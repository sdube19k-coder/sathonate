package operations

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"testing"
	"time"
)

// Real database enforcement under a disposable NOLOGIN/NOBYPASSRLS role, not
// the superuser used to migrate the dedicated test database.
func TestOperationDeploymentRLSUnderRestrictedRole(t *testing.T) {
	connection := os.Getenv("SATHONATE_TEST_DATABASE_URL")
	if connection == "" {
		t.Skip("dedicated PostgreSQL required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, connection)
	if err != nil {
		t.Fatal("database unavailable")
	}
	defer pool.Close()
	if _, err = pool.Exec(ctx, SQLSchema); err != nil {
		t.Fatal("RLS migration failed", err)
	}
	role := fmt.Sprintf("smp_rls_%d", time.Now().UnixNano())
	if _, err = pool.Exec(ctx, "CREATE ROLE "+role+" NOLOGIN NOSUPERUSER NOBYPASSRLS"); err != nil {
		t.Fatal("test role creation failed")
	}
	defer func() {
		pool.Exec(context.Background(), "DROP OWNED BY "+role)
		pool.Exec(context.Background(), "DROP ROLE "+role)
	}()
	if _, err = pool.Exec(ctx, "GRANT SELECT,INSERT,UPDATE,DELETE ON sathonate_operations,sathonate_deployments TO "+role); err != nil {
		t.Fatal("test grants failed")
	}
	cfg, e := pgxpool.ParseConfig(connection)
	if e != nil {
		t.Fatal("private test database configuration invalid")
	}
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, "SET ROLE "+role)
		return err
	}
	restricted, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal("restricted pool unavailable")
	}
	defer restricted.Close()
	if _, e = OpenRecoverableDatabase(ctx, restricted); e == nil {
		t.Fatal("unauthorized journal recovery accepted")
	}
	tenant := fmt.Sprintf("rls-%d", time.Now().UnixNano())
	defer pool.Exec(context.Background(), "DELETE FROM sathonate_operations WHERE tenant_id=$1", tenant)
	defer pool.Exec(context.Background(), "DELETE FROM sathonate_deployments WHERE tenant_id=$1", tenant)
	for _, table := range []string{"sathonate_operations", "sathonate_deployments"} {
		tx, e := pool.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = tx.Exec(ctx, "SET LOCAL ROLE "+role); e != nil {
			tx.Rollback(ctx)
			t.Fatal(e)
		}
		var missingContextRows int
		if e = tx.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&missingContextRows); e != nil || missingContextRows != 0 {
			tx.Rollback(ctx)
			t.Fatal("missing tenant context exposed rows")
		}
		if _, e = tx.Exec(ctx, "SELECT set_config('sathonate.tenant_id',$1,true)", tenant); e != nil {
			tx.Rollback(ctx)
			t.Fatal("tenant test context failed")
		}
		insert := "INSERT INTO sathonate_operations(tenant_id,id,network_id,status,payload) VALUES($1,'rls-op','net','succeeded','{}')"
		if table == "sathonate_deployments" {
			insert = "INSERT INTO sathonate_deployments(tenant_id,operation_id,payload) VALUES($1,'rls-op','{}')"
		}
		if _, e = tx.Exec(ctx, insert, tenant); e != nil {
			tx.Rollback(ctx)
			t.Fatal("own-tenant insert denied", e)
		}
		tx.Exec(ctx, "SELECT set_config('sathonate.tenant_id','foreign',true)")
		var count int
		if e = tx.QueryRow(ctx, "SELECT count(*) FROM "+table+" WHERE tenant_id=$1", tenant).Scan(&count); e != nil || count != 0 {
			tx.Rollback(ctx)
			t.Fatal("foreign rows visible")
		}
		tag, e := tx.Exec(ctx, "UPDATE "+table+" SET payload='{}' WHERE tenant_id=$1", tenant)
		if e != nil || tag.RowsAffected() != 0 {
			tx.Rollback(ctx)
			t.Fatal("foreign update permitted")
		}
		tag, e = tx.Exec(ctx, "DELETE FROM "+table+" WHERE tenant_id=$1", tenant)
		if e != nil || tag.RowsAffected() != 0 {
			tx.Rollback(ctx)
			t.Fatal("foreign delete permitted")
		}
		if _, e = tx.Exec(ctx, insert, tenant); e == nil {
			tx.Rollback(ctx)
			t.Fatal("foreign insert permitted")
		}
		tx.Rollback(ctx)
	}
}
