package operations

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPreparedCloudSubmissionIsAtomic(t *testing.T) {
	url := os.Getenv("SATHONATE_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires dedicated PostgreSQL test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, e := pgxpool.New(ctx, url)
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	if _, e = pool.Exec(ctx, SQLSchema); e != nil {
		t.Fatal(e)
	}
	s, e := OpenRecoverableDatabase(ctx, pool)
	if e != nil {
		t.Fatal(e)
	}
	key := fmt.Sprintf("atomic-%d", time.Now().UnixNano())
	_, _, e = s.BeginPrepared("atomic-test", "operator", key, "aws-eks-deploy", key, "digest", "2.5.15", "", []Stage{{Key: "validate", Label: "Validate"}}, func(Operation) ([]byte, error) { return []byte("invalid JSON"), nil })
	if e == nil {
		t.Fatal("invalid deployment payload committed")
	}
	var count int
	if e = pool.QueryRow(ctx, "SELECT count(*) FROM sathonate_operations WHERE tenant_id='atomic-test' AND idempotency_key=$1", key).Scan(&count); e != nil || count != 0 {
		t.Fatal("operation committed without its input", e)
	}
}

func TestSecureEKSPostgresDurableRecoveryAndIdempotency(t *testing.T) {
	url := os.Getenv("SATHONATE_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires dedicated PostgreSQL test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, e := pgxpool.New(ctx, url)
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	if _, e = pool.Exec(ctx, SQLSchema); e != nil {
		t.Fatal(e)
	}
	first, e := OpenRecoverableDatabase(ctx, pool)
	if e != nil {
		t.Fatal(e)
	}
	tenant := fmt.Sprintf("secure-eks-recovery-%d", time.Now().UnixNano())
	key := "qualification-recovery-key"
	defer pool.Exec(context.Background(), "DELETE FROM sathonate_deployments WHERE tenant_id=$1", tenant)
	defer pool.Exec(context.Background(), "DELETE FROM sathonate_operations WHERE tenant_id=$1", tenant)
	payload := []byte(`{"mode":"secure-eks","targetSHA256":"test-binding","phase":"deploying"}`)
	create := func(store *Store) (Operation, bool, error) {
		return store.BeginPrepared(tenant, "fixture", "network", "secure-eks-deploy", key, "network-digest", "2.5.15", "", []Stage{{Key: "validate", Label: "Validate"}}, func(Operation) ([]byte, error) { return payload, nil })
	}
	op, reused, e := create(first)
	if e != nil || reused {
		t.Fatal("durable secure EKS submission failed", e)
	}
	again, reused, e := create(first)
	if e != nil || !reused || again.ID != op.ID {
		t.Fatal("idempotent secure EKS submission failed", e)
	}
	second, e := OpenRecoverableDatabase(ctx, pool)
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, running := range second.RunningCloud() {
		if running.ID == op.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("secure EKS operation lost after process restart")
	}
	if e = second.DiscoverCloud(ctx); e != nil {
		t.Fatal(e)
	}
	if e = second.Replay(op.ID); e != nil {
		t.Fatal("secure EKS adoption rejected", e)
	}
	if _, e = second.Get("foreign-tenant", op.ID); e == nil {
		t.Fatal("cross tenant operation visible")
	}
	var saved []byte
	if e = pool.QueryRow(ctx, "SELECT payload FROM sathonate_deployments WHERE tenant_id=$1 AND operation_id=$2", tenant, op.ID).Scan(&saved); e != nil || len(saved) == 0 {
		t.Fatal("bound desired state missing", e)
	}
}
