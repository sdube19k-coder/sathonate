package deployment

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"sathonate.local/fabric-control-plane/internal/localfabric"
	"sathonate.local/fabric-control-plane/internal/operations"
	"testing"
	"time"
)

func TestHistorySnapshotCannotWriteOrClaimLiveHealth(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	op := operations.Operation{ID: "op_12345678", TenantID: "tenant", NetworkID: "network", Status: "succeeded"}
	when := time.Now().Add(-time.Hour)
	raw, _ := json.Marshal(Result{Ready: true, Verified: true, Phase: "succeeded", Health: &localfabric.Health{Ready: true, CheckedAt: when}})
	m := &Manager{ctx: ctx, cancel: cancel, runner: historyRunner{}, jobs: operations.ReadOnlySnapshot([]operations.Operation{op}), history: map[string][]byte{"tenant\x00" + op.ID: raw}}
	defer m.Close()
	if _, err := m.Status("foreign", op.ID); err == nil {
		t.Fatal("foreign history read")
	}
	result, err := m.Status("tenant", op.ID)
	if err != nil || result.Ready || result.Verified || result.Mode != "history-only" || result.Health.Ready || !result.Health.CheckedAt.Equal(when) {
		t.Fatal("recorded history claimed live health", err)
	}
	if _, err = m.Start("tenant", "actor", "network", fixture(t), 1, "valid-request-key", "", ""); !errors.Is(err, operations.ErrReadOnly) {
		t.Fatal("history started deployment", err)
	}
	if _, err = m.Retry("tenant", "actor", op.ID, "valid-retry-key"); !errors.Is(err, operations.ErrReadOnly) {
		t.Fatal("history retried", err)
	}
	if err = m.Cancel(ctx, "tenant", op.ID); !errors.Is(err, operations.ErrReadOnly) {
		t.Fatal("history cancelled", err)
	}
	if err = m.Cleanup(ctx, "tenant", op.ID); !errors.Is(err, operations.ErrReadOnly) {
		t.Fatal("history deleted", err)
	}
	if _, _, err = m.jobs.Begin("tenant", "actor", "network", "kind", "valid-key-123", "digest", "2.5.15", "", Stages); !errors.Is(err, operations.ErrReadOnly) {
		t.Fatal("snapshot journal mutation", err)
	}
}
func TestPostgresHistorySnapshotDoesNotRecoverJobsOrCrossScope(t *testing.T) {
	connection := os.Getenv("SATHONATE_TEST_DATABASE_URL")
	if connection == "" {
		t.Skip("requires dedicated PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, connection)
	if err != nil {
		t.Fatal("database unavailable")
	}
	defer pool.Close()
	suffix := time.Now().Format("150405.000000000")
	scope := "history-" + suffix
	foreign := "foreign-" + suffix
	id := "op_history_" + suffix
	defer func() {
		_, _ = pool.Exec(context.Background(), "DELETE FROM sathonate_deployments WHERE tenant_id=ANY($1)", []string{scope, foreign})
		_, _ = pool.Exec(context.Background(), "DELETE FROM sathonate_operations WHERE tenant_id=ANY($1)", []string{scope, foreign})
	}()
	for _, tenant := range []string{scope, foreign} {
		op := operations.Operation{ID: id, TenantID: tenant, NetworkID: "network", Status: "running"}
		raw, _ := json.Marshal(op)
		if _, err = pool.Exec(ctx, "INSERT INTO sathonate_operations(tenant_id,id,network_id,status,payload) VALUES($1,$2,'network','running',$3)", tenant, id, raw); err != nil {
			t.Fatal("fixture operation")
		}
		result, _ := json.Marshal(Result{Phase: "running", Network: fixture(t)})
		if _, err = pool.Exec(ctx, "INSERT INTO sathonate_deployments(tenant_id,operation_id,payload) VALUES($1,$2,$3)", tenant, id, result); err != nil {
			t.Fatal("fixture snapshot")
		}
	}
	snapshot, err := OpenHistoryDatabase(ctx, pool, []string{scope})
	if err != nil {
		t.Fatal("history startup", err)
	}
	defer snapshot.Close()
	if len(snapshot.Operations().List(scope)) != 1 || len(snapshot.Operations().List(foreign)) != 0 {
		t.Fatal("history scope isolation")
	}
	if _, err = snapshot.Status(foreign, id); err == nil {
		t.Fatal("foreign snapshot access")
	}
	var status string
	if err = pool.QueryRow(ctx, "SELECT status FROM sathonate_operations WHERE tenant_id=$1 AND id=$2", scope, id).Scan(&status); err != nil || status != "running" {
		t.Fatal("history startup recovered or modified a job")
	}
}
