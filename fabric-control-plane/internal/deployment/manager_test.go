package deployment

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"path/filepath"
	"sathonate.local/fabric-control-plane/internal/domain"
	"sathonate.local/fabric-control-plane/internal/operations"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T) domain.NetworkSpec {
	t.Helper()
	f, err := os.Open("../../examples/local-kubernetes-network.json")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	s, err := domain.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func terminal(t *testing.T, m *Manager, id string) operations.Operation {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		op, err := m.Operations().Get("test-a", id)
		if err != nil {
			t.Fatal(err)
		}
		if op.Status != "running" {
			return op
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("operation did not terminate")
	return operations.Operation{}
}
func TestMockIsolationFailureRetryAndRestart(t *testing.T) {
	root := t.TempDir()
	m, err := Open(root, MockRunner{Delay: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	op, err := m.Start("test-a", "operator", "network", fixture(t), 1, "failure-key", "peers", "")
	if err != nil {
		t.Fatal(err)
	}
	again, err := m.Start("test-a", "operator", "network", fixture(t), 1, "failure-key", "peers", "")
	if err != nil || again.ID != op.ID {
		t.Fatal("idempotency lost", err)
	}
	failed := terminal(t, m, op.ID)
	if failed.Status != "failed" {
		t.Fatal("injected failure succeeded")
	}
	if _, err = m.Status("test-b", op.ID); err == nil {
		t.Fatal("cross tenant status access")
	}
	retry, err := m.Retry("test-a", "operator", op.ID, "retry-key")
	if err != nil {
		t.Fatal(err)
	}
	if terminal(t, m, retry.ID).Status != "succeeded" {
		t.Fatal("retry failed")
	}
	result, err := m.Status("test-a", retry.ID)
	if err != nil || !result.Simulated || result.Verified || !result.Ready {
		t.Fatal("mock result misrepresented real evidence", result, err)
	}
	old, err := m.Status("test-a", op.ID)
	if err != nil || old.Phase != "superseded" || old.Ready || old.Verified {
		t.Fatal("historical failure displayed as current deployment", err)
	}
	if _, err = m.Retry("test-a", "operator", op.ID, "historical-retry-key"); err == nil {
		t.Fatal("historical input retried after a newer deployment")
	}
	m.Close()
	m, err = Open(root, MockRunner{})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	persisted, err := m.Status("test-a", retry.ID)
	if err != nil || persisted.OperationID != retry.ID {
		t.Fatal("result was not durable")
	}
}
func TestSnapshotFailureCannotDispatch(t *testing.T) {
	m, err := Open(t.TempDir(), MockRunner{})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	m.root = filepath.Join(t.TempDir(), "file")
	if err = os.WriteFile(m.root, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = m.Start("test-a", "operator", "network", fixture(t), 1, "persist-failure", "", ""); err == nil {
		t.Fatal("persist failure accepted")
	}
	for _, op := range m.Operations().List("test-a") {
		if op.Status != "failed" {
			t.Fatal("operation dispatched without persisted input")
		}
	}
}
func TestPostgresJobAndNetworkStateSurviveRestart(t *testing.T) {
	connection := os.Getenv("SATHONATE_TEST_DATABASE_URL")
	if connection == "" {
		t.Skip("requires a dedicated real PostgreSQL test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, connection)
	if err != nil {
		t.Fatal("test database unavailable")
	}
	defer pool.Close()
	if _, err = pool.Exec(ctx, operations.SQLSchema); err != nil {
		t.Fatal(err)
	}
	m, err := OpenDatabase(ctx, pool, MockRunner{})
	if err != nil {
		t.Fatal(err)
	}
	key := "postgres-" + time.Now().Format("150405.000000000")
	key = strings.ReplaceAll(key, ".", "-")
	op, err := m.Start("test-a", "operator", key, fixture(t), 1, key, "", "")
	if err != nil {
		m.Close()
		t.Fatal(err)
	}
	if terminal(t, m, op.ID).Status != "succeeded" {
		m.Close()
		t.Fatal("job failed")
	}
	m.Close()
	m, err = OpenDatabase(ctx, pool, MockRunner{})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	result, err := m.Status("test-a", op.ID)
	if err != nil || !result.Ready || result.Verified {
		t.Fatal("PostgreSQL state lost or promoted to real", err)
	}
	if _, err = m.Status("test-b", op.ID); err == nil {
		t.Fatal("cross tenant SQL status access")
	}
	_, _ = pool.Exec(ctx, "DELETE FROM sathonate_deployments WHERE tenant_id=$1 AND operation_id=$2", "test-a", op.ID)
	_, _ = pool.Exec(ctx, "DELETE FROM sathonate_operations WHERE tenant_id=$1 AND id=$2", "test-a", op.ID)
}
