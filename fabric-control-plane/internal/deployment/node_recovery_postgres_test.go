package deployment

import (
	"context"
	"fmt"
	"os"
	"sathonate.local/fabric-control-plane/internal/persistence"
	"strings"
	"testing"
	"time"
)

type recoveryFixtureRunner struct{ dynamicFixtureRunner }

func (recoveryFixtureRunner) RecoveryPreflight(context.Context, Request, string) (NodeRecovery, error) {
	return NodeRecovery{Node: "peer-a", Kind: "peer", DeploymentUID: "deployment-uid", ResourceUID: "cr-uid", TemplateHash: "template-hash", PodUIDs: []string{}, Claims: map[string]string{"data": "claim-uid"}, Materials: map[string]materialVersion{"tls": {UID: "secret-uid", Version: "42"}}}, nil
}

// This proves PostgreSQL durability and approval isolation, not live Fabric
// recovery. The disposable browser fixture independently executes the latter.
func TestPostgresNodeRecoveryApprovalDurabilityAndIsolation(t *testing.T) {
	connection := os.Getenv("SATHONATE_TEST_DATABASE_URL")
	if connection == "" {
		t.Skip("requires dedicated PostgreSQL")
	}
	if !strings.Contains(connection, "/fabric_smp_tests?") {
		t.Fatal("dedicated test database required")
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
	m, e := OpenDatabase(ctx, db.Pool(), recoveryFixtureRunner{})
	if e != nil {
		t.Fatal(e)
	}
	defer func() { m.Close() }()
	tenant := fmt.Sprintf("recovery-%x", time.Now().UnixNano())
	s := dynamicDesign(t)
	op, _, e := m.jobs.Begin(tenant, "operator-a", "network-a", "secure-local-deploy", "base-key", "digest", "2.5.15", "", Stages)
	if e != nil {
		t.Fatal(e)
	}
	for _, stage := range Stages {
		if m.jobs.Advance(op.ID, stage.Key) != nil {
			t.Fatal("fixture stages")
		}
	}
	if m.write(Result{TenantID: tenant, OperationID: op.ID, Network: s, Revision: 1, Mode: "secure-local", Ready: true, Verified: true, Phase: "running"}) != nil || m.jobs.Finish(op.ID, nil) != nil {
		t.Fatal("fixture persistence")
	}
	preview, e := m.PreviewNodeRecovery(ctx, tenant, "operator-a", op.ID, "peer-a")
	if e != nil {
		t.Fatal(e)
	}
	other, e := m.PreviewNodeRecovery(ctx, tenant, "operator-b", op.ID, "peer-a")
	if e != nil || other.Plan.Digest == preview.Plan.Digest {
		t.Fatal("approval not actor-bound")
	}
	if _, e = m.PreviewNodeRecovery(ctx, tenant+"-foreign", "operator-a", op.ID, "peer-a"); e == nil {
		t.Fatal("foreign tenant preview")
	}
	if _, e = m.ApplyNodeRecovery(ctx, tenant, "operator-a", op.ID, "peer-a", "stale", "recovery-key"); e == nil {
		t.Fatal("stale approval")
	}
	change, e := m.ApplyNodeRecovery(ctx, tenant, "operator-a", op.ID, "peer-a", preview.Plan.Digest, "recovery-key")
	if e != nil {
		t.Fatal(e)
	}
	ack, e := m.ApplyNodeRecovery(ctx, tenant, "operator-a", op.ID, "peer-a", preview.Plan.Digest, "recovery-key")
	if e != nil || ack.ID != change.ID {
		t.Fatal("duplicate recovery", e)
	}
	if _, e = m.ApplyNodeRecovery(ctx, tenant, "operator-b", op.ID, "peer-a", preview.Plan.Digest, "recovery-key"); e == nil {
		t.Fatal("actor escalation")
	}
	if _, e = m.ApplyNodeRecovery(ctx, tenant, "operator-a", op.ID, "different-node", preview.Plan.Digest, "recovery-key"); e == nil {
		t.Fatal("node substitution")
	}
	m.Close()
	m, e = OpenDatabase(ctx, db.Pool(), recoveryFixtureRunner{})
	if e != nil {
		t.Fatal(e)
	}
	state, e := m.Status(tenant, change.ID)
	if e != nil || state.Recovery == nil || state.PreviousNetwork == nil || state.Recovery.Digest != preview.Plan.Digest || state.Recovery.Claims["data"] != "claim-uid" || state.Recovery.Materials["tls"].Version != "42" {
		t.Fatal("durable recovery approval/storage/material lost", e)
	}
	ack, e = m.ApplyNodeRecovery(ctx, tenant, "operator-a", op.ID, "peer-a", preview.Plan.Digest, "recovery-key")
	if e != nil || ack.ID != change.ID {
		t.Fatal("restart acknowledgement not adopted", e)
	}
}
