package maintenance

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"testing"
	"time"
)

func TestPostgresMaintenanceRecoveryAndCAS(t *testing.T) {
	connection := os.Getenv("SATHONATE_TEST_DATABASE_URL")
	if connection == "" {
		t.Skip("dedicated PostgreSQL required; not a mock")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, connection)
	if err != nil {
		t.Fatal("test database unavailable")
	}
	defer pool.Close()
	if _, err = pool.Exec(ctx, SQLSchema); err != nil {
		t.Fatal("maintenance migration failed")
	}
	p, b := fixture()
	p.TenantID = fmt.Sprintf("maintenance-%d", time.Now().UnixNano())
	b.TenantID = p.TenantID
	defer pool.Exec(context.Background(), "DELETE FROM sathonate_maintenance WHERE tenant_id=$1", p.TenantID)
	intent, _ := NewIntent(p, b, "org1/peer1", "request-one", time.Now())
	store := Store{pool}
	created, err := store.Create(ctx, intent)
	if err != nil {
		t.Fatal(err)
	}
	again, err := store.Create(ctx, intent)
	if err != nil || again.ID != created.ID {
		t.Fatal("duplicate intent", err)
	}
	wrong := intent
	wrong.BindingSHA256 = "wrong"
	if _, err = store.Create(ctx, wrong); err == nil {
		t.Fatal("idempotency binding changed")
	}
	if _, err = store.Get(ctx, "foreign", intent.ID); err == nil {
		t.Fatal("foreign maintenance read")
	}
	recovered, err := (Store{pool}).Get(ctx, p.TenantID, intent.ID)
	if err != nil || recovered.Revision != 1 {
		t.Fatal("recovery failed")
	}
	now := time.Now()
	next, err := Advance(ctx, p, recovered, 1, "checkpoint-one", StepApproval{intent.ID, intent.BindingSHA256, "inventory", "change-one", now.Add(time.Minute)}, &simulatedDriver{}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.SaveCheckpoint(ctx, next, 1); err != nil {
		t.Fatal(err)
	}
	if err = store.SaveCheckpoint(ctx, next, 1); err == nil {
		t.Fatal("stale update accepted")
	}
	saved, err := store.Get(ctx, p.TenantID, intent.ID)
	if err != nil || saved.Revision != 2 || !saved.Simulated {
		t.Fatal("durable checkpoint not recovered")
	}
}
