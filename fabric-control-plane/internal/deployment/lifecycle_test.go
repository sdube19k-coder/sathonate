package deployment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sathonate.local/fabric-control-plane/internal/domain"
	"sathonate.local/fabric-control-plane/internal/localfabric"
	"sathonate.local/fabric-control-plane/internal/persistence"
	"testing"
	"time"
)

type lifecycleFixtureRunner struct{ degraded bool }

func (lifecycleFixtureRunner) Mode() string { return "secure-local" }
func (lifecycleFixtureRunner) Run(ctx context.Context, _ Request, _ func(string, string) error) (Outcome, error) {
	<-ctx.Done()
	return Outcome{}, ctx.Err()
}
func (r lifecycleFixtureRunner) Health(context.Context, string, string, *localfabric.Health) (localfabric.Health, error) {
	return localfabric.Health{Ready: !r.degraded}, nil
}
func (lifecycleFixtureRunner) Logs(context.Context, string, string, string) (string, error) {
	return "", errors.New("unused fixture")
}
func (lifecycleFixtureRunner) Cleanup(context.Context, string, string) error {
	return errors.New("unused fixture")
}

func TestPostgresLifecycleApprovalScopeStalenessAndIdempotency(t *testing.T) {
	connection := os.Getenv("SATHONATE_TEST_DATABASE_URL")
	if connection == "" {
		t.Skip("requires dedicated PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, e := persistence.Open(ctx, connection, false)
	if e != nil {
		t.Fatal("test database unavailable")
	}
	defer db.Close()
	if db.Migrate(ctx) != nil {
		t.Fatal("test schema unavailable")
	}
	m, e := OpenDatabase(ctx, db.Pool(), lifecycleFixtureRunner{})
	if e != nil {
		t.Fatal(e)
	}
	defer m.Close()
	tenant := fmt.Sprintf("lifecycle-%x", time.Now().UnixNano())
	raw, e := os.ReadFile("../../examples/secure-local-network.json")
	if e != nil {
		t.Fatal(e)
	}
	var spec domain.NetworkSpec
	if json.Unmarshal(raw, &spec) != nil {
		t.Fatal("fixture")
	}
	op, _, e := m.jobs.Begin(tenant, "operator-a", "network-a", "secure-local-deploy", "base-key", "digest", "2.5.15", "", Stages)
	if e != nil {
		t.Fatal(e)
	}
	for _, stage := range Stages {
		if m.jobs.Advance(op.ID, stage.Key) != nil {
			t.Fatal("fixture stages")
		}
	}
	if m.write(Result{TenantID: tenant, OperationID: op.ID, Network: spec, Revision: 1, Mode: "secure-local", Ready: true, Verified: true, Phase: "running"}) != nil || m.jobs.Finish(op.ID, nil) != nil {
		t.Fatal("fixture persistence")
	}
	preview, e := m.PreviewLifecycle(ctx, tenant, "operator-a", op.ID, "add-peer")
	if e != nil || !preview.LiveHealthChecked || !preview.After.PeerAdded {
		t.Fatal("valid preview rejected", e)
	}
	other, e := m.PreviewLifecycle(ctx, tenant, "operator-b", op.ID, "add-peer")
	if e != nil || other.Digest == preview.Digest {
		t.Fatal("approval is not actor bound")
	}
	if _, e = m.PreviewLifecycle(ctx, tenant+"-foreign", "operator-a", op.ID, "add-peer"); e == nil {
		t.Fatal("foreign preview accepted")
	}
	for _, action := range []string{"add-orderer", "add-organization", "remove-peer", "shell-command"} {
		if _, e = m.PreviewLifecycle(ctx, tenant, "operator-a", op.ID, action); e == nil {
			t.Fatal("unsupported mutation accepted")
		}
	}
	m.runner = lifecycleFixtureRunner{degraded: true}
	if _, e = m.PreviewLifecycle(ctx, tenant, "operator-a", op.ID, "add-peer"); e == nil {
		t.Fatal("degraded preflight accepted")
	}
	m.runner = lifecycleFixtureRunner{}
	if _, e = m.ApplyLifecycle(ctx, tenant, "operator-a", op.ID, "add-peer", "stale", "apply-key"); e == nil {
		t.Fatal("stale digest accepted")
	}
	applied, e := m.ApplyLifecycle(ctx, tenant, "operator-a", op.ID, "add-peer", preview.Digest, "apply-key")
	if e != nil {
		t.Fatal(e)
	}
	again, e := m.ApplyLifecycle(ctx, tenant, "operator-a", op.ID, "add-peer", preview.Digest, "apply-key")
	if e != nil || again.ID != applied.ID {
		t.Fatal("acknowledgement retry created another operation", e)
	}
	if _, e = m.ApplyLifecycle(ctx, tenant, "operator-b", op.ID, "add-peer", preview.Digest, "apply-key"); e == nil {
		t.Fatal("another actor reused approval")
	}
	if _, e = m.PreviewLifecycle(ctx, tenant, "operator-a", op.ID, "upgrade-chaincode"); e == nil {
		t.Fatal("superseded parent accepted")
	}
	if _, e = m.Start(tenant, "operator-a", "network-a", spec, 1, "base-redeploy", "", ""); e == nil {
		t.Fatal("base redeployment discarded lifecycle state")
	}
	result, e := m.Status(tenant, applied.ID)
	if e != nil || result.LifecycleApproval != preview.Digest || result.LifecycleParent != op.ID || result.Lifecycle == nil || !result.Lifecycle.PeerAdded {
		t.Fatal("durable lifecycle intent missing")
	}
}
