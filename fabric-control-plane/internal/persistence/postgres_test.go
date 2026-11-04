package persistence

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"sathonate.local/fabric-control-plane/internal/domain"
	"sathonate.local/fabric-control-plane/internal/drafts"
)

func TestPostgresTenantIsolationAndRevisions(t *testing.T) {
	connection := os.Getenv("SATHONATE_TEST_DATABASE_URL")
	if connection == "" {
		t.Skip("real PostgreSQL integration requires SATHONATE_TEST_DATABASE_URL; not validated by a mock")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, e := Open(ctx, connection, false)
	if e != nil {
		t.Fatal("test PostgreSQL unavailable")
	}
	defer db.Close()
	if e = db.Migrate(ctx); e != nil {
		t.Fatal(e)
	}
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	a, b := "test-a-"+suffix, "test-b-"+suffix
	defer func() {
		_, _ = db.pool.Exec(context.Background(), "DELETE FROM sathonate_network_revisions WHERE tenant_id=ANY($1)", []string{a, b})
		_, _ = db.pool.Exec(context.Background(), "DELETE FROM sathonate_networks WHERE tenant_id=ANY($1)", []string{a, b})
	}()
	f, e := os.Open("../../examples/cloud-network.json")
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	spec, e := domain.Decode(f)
	if e != nil {
		t.Fatal(e)
	}
	ar, br := db.ForTenant(ctx, a), db.ForTenant(ctx, b)
	saved, e := ar.Save(spec, "", 0)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = br.Get(saved.ID); !errors.Is(e, drafts.ErrNotFound) {
		t.Fatal("tenant B read tenant A")
	}
	if _, e = br.Save(spec, "", 0); e != nil {
		t.Fatal("network keys must be tenant scoped", e)
	}
	if _, e = ar.Save(spec, saved.ID, 1); e != nil {
		t.Fatal(e)
	}
	if _, e = ar.Save(spec, saved.ID, 1); !errors.Is(e, drafts.ErrConflict) {
		t.Fatal("stale update accepted")
	}
}
