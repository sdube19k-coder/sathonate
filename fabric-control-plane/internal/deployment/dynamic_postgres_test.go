package deployment

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"sathonate.local/fabric-control-plane/internal/domain"
	"sathonate.local/fabric-control-plane/internal/kubernetesfabric"
	"sathonate.local/fabric-control-plane/internal/persistence"
)

type dynamicFixtureRunner struct{ lifecycleFixtureRunner }

func (dynamicFixtureRunner) Validate(q Request) error {
	if q.PreviousNetwork != nil {
		return kubernetesfabric.ValidateDynamicTransition(*q.PreviousNetwork, q.Network)
	}
	if domain.HasErrors(domain.Validate(q.Network)) {
		return fmt.Errorf("invalid fixture network")
	}
	return nil
}

// This tests durable approval and authorization, not Fabric execution. The live
// disposable API acceptance is independently required for runtime qualification.
func TestPostgresDynamicApprovalRecoveryAndIsolation(t *testing.T) {
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
	m, e := OpenDatabase(ctx, db.Pool(), dynamicFixtureRunner{})
	if e != nil {
		t.Fatal(e)
	}
	defer func() { m.Close() }()
	tenant := fmt.Sprintf("dynamic-%x", time.Now().UnixNano())
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
	raw, _ := json.Marshal(s)
	var desired domain.NetworkSpec
	_ = json.Unmarshal(raw, &desired)
	desired.Spec.Configuration.Chaincodes[0].Sequence = 2
	desired.Spec.Configuration.Chaincodes[0].Version = "2.0"
	preview, e := m.PreviewDynamic(ctx, tenant, "operator-a", op.ID, "network-a", desired, 2)
	if e != nil {
		t.Fatal(e)
	}
	other, e := m.PreviewDynamic(ctx, tenant, "operator-b", op.ID, "network-a", desired, 2)
	if e != nil || other.Digest == preview.Digest {
		t.Fatal("preview not actor-bound")
	}
	if _, e = m.PreviewDynamic(ctx, tenant+"-foreign", "operator-a", op.ID, "network-a", desired, 2); e == nil {
		t.Fatal("foreign tenant read preview")
	}
	if _, e = m.PreviewDynamic(ctx, tenant, "operator-a", op.ID, "other-record", desired, 2); e == nil {
		t.Fatal("foreign project/network preview")
	}
	if _, e = m.ApplyDynamic(ctx, tenant, "operator-a", op.ID, "network-a", desired, 2, "stale", "apply-key"); e == nil {
		t.Fatal("stale approval accepted")
	}
	change, e := m.ApplyDynamic(ctx, tenant, "operator-a", op.ID, "network-a", desired, 2, preview.Digest, "apply-key")
	if e != nil {
		t.Fatal(e)
	}
	again, e := m.ApplyDynamic(ctx, tenant, "operator-a", op.ID, "network-a", desired, 2, preview.Digest, "apply-key")
	if e != nil || again.ID != change.ID {
		t.Fatal("acknowledgement duplicate", e)
	}
	if _, e = m.ApplyDynamic(ctx, tenant, "operator-b", op.ID, "network-a", desired, 2, preview.Digest, "apply-key"); e == nil {
		t.Fatal("actor approval escalation")
	}
	if _, e = m.PreviewDynamic(ctx, tenant, "operator-a", op.ID, "network-a", desired, 2); e == nil {
		t.Fatal("superseded parent preview")
	}
	if _, e = m.Start(tenant, "operator-a", "network-a", s, 1, "discard-change", "", ""); e == nil {
		t.Fatal("plain deploy discarded dynamic transition")
	}
	desired.Spec.Configuration.Chaincodes[0].Version = "caller-mutated"
	m.Close()
	m, e = OpenDatabase(ctx, db.Pool(), dynamicFixtureRunner{})
	if e != nil {
		t.Fatal(e)
	}
	state, e := m.Status(tenant, change.ID)
	if e != nil || state.PreviousNetwork == nil || state.Network.Spec.Configuration.Chaincodes[0].Version != "2.0" || state.PreviousNetwork.Spec.Configuration.Chaincodes[0].Version != "1.0" || state.LifecycleApproval != preview.Digest {
		t.Fatal("durable immutable previous/desired input lost", e)
	}
}
