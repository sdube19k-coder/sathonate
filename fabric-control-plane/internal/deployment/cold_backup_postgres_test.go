package deployment

import (
	"context"
	"os"
	"path/filepath"
	"sathonate.local/fabric-control-plane/internal/persistence"
	"strings"
	"testing"
	"time"
)

func TestPostgresColdBackupRequiresStoppedExecutorAndExactScope(t *testing.T) {
	connection := os.Getenv("SATHONATE_TEST_DATABASE_URL")
	if connection == "" {
		t.Skip("requires dedicated PostgreSQL")
	}
	if !strings.Contains(connection, "/fabric_smp_tests?") {
		t.Fatal("dedicated database required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db, e := persistence.Open(ctx, connection, false)
	if e != nil {
		t.Fatal("test database unavailable")
	}
	defer db.Close()
	lease, e := db.Pool().Acquire(ctx)
	if e != nil {
		t.Fatal("lease unavailable")
	}
	var held bool
	if lease.QueryRow(ctx, "SELECT pg_try_advisory_lock(718481029)").Scan(&held) != nil || !held {
		lease.Release()
		t.Fatal("test executor lock unavailable")
	}
	runner := SecureRunner{binding: &secureBinding{}}
	artifact := filepath.Join(t.TempDir(), "never-created.encrypted")
	_, e = runner.ColdBackup(ctx, db.Pool(), ColdOptions{Tenant: "foreign", Operation: "unowned", Action: "capture", Artifact: artifact, Key: make([]byte, 32)})
	lease.Exec(ctx, "SELECT pg_advisory_unlock(718481029)")
	lease.Release()
	if e == nil || !strings.Contains(e.Error(), "stop the local API/executor") {
		t.Fatal("active executor was not rejected")
	}
	if _, err := os.Stat(artifact); !os.IsNotExist(err) {
		t.Fatal("blocked capture wrote an artifact")
	}
	_, e = runner.ColdBackup(ctx, db.Pool(), ColdOptions{Tenant: "unowned", Operation: "missing", Action: "capture", Artifact: artifact, Key: make([]byte, 32)})
	if e == nil {
		t.Fatal("missing tenant/operation scope accepted")
	}
	if _, err := os.Stat(artifact); !os.IsNotExist(err) {
		t.Fatal("foreign capture wrote an artifact")
	}
}
