package operations

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"time"
)

const SQLSchema = `CREATE TABLE IF NOT EXISTS sathonate_operations(
 tenant_id text NOT NULL,id text NOT NULL,network_id text NOT NULL,status text NOT NULL,
 idempotency_key text NOT NULL DEFAULT '',payload jsonb NOT NULL,updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(tenant_id,id));
 CREATE UNIQUE INDEX IF NOT EXISTS sathonate_operation_active ON sathonate_operations(tenant_id,network_id) WHERE status='running';
 CREATE UNIQUE INDEX IF NOT EXISTS sathonate_operation_key ON sathonate_operations(tenant_id,idempotency_key) WHERE idempotency_key<>'';
 CREATE TABLE IF NOT EXISTS sathonate_deployments(tenant_id text NOT NULL,operation_id text NOT NULL,payload jsonb NOT NULL,updated_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(tenant_id,operation_id));
ALTER TABLE sathonate_operations ENABLE ROW LEVEL SECURITY;
ALTER TABLE sathonate_operations FORCE ROW LEVEL SECURITY;
ALTER TABLE sathonate_deployments ENABLE ROW LEVEL SECURITY;
ALTER TABLE sathonate_deployments FORCE ROW LEVEL SECURITY;
DO $$ DECLARE table_name text; BEGIN
 FOREACH table_name IN ARRAY ARRAY['sathonate_operations','sathonate_deployments'] LOOP
 IF NOT EXISTS(SELECT 1 FROM pg_policies WHERE tablename=table_name AND policyname='sathonate_tenant_executor') THEN
 EXECUTE format('CREATE POLICY sathonate_tenant_executor ON %I USING (tenant_id=current_setting(''sathonate.tenant_id'',true) OR EXISTS(SELECT 1 FROM pg_roles r WHERE r.rolname=''sathonate_executor'' AND pg_has_role(current_user,r.oid,''member''))) WITH CHECK (tenant_id=current_setting(''sathonate.tenant_id'',true) OR EXISTS(SELECT 1 FROM pg_roles r WHERE r.rolname=''sathonate_executor'' AND pg_has_role(current_user,r.oid,''member'')))',table_name);
 END IF;
 END LOOP;
END $$;`

// OpenDatabase is the single-process local executor journal. API and worker
// callbacks share this instance; it is not a distributed queue claim.
func OpenDatabase(ctx context.Context, pool *pgxpool.Pool) (*Store, error) {
	return openDatabase(ctx, pool, false)
}

// OpenRecoverableDatabase retains cloud operations for adoption of the same
// Kubernetes Job. Caller must hold the executor advisory lock before opening.
func OpenRecoverableDatabase(ctx context.Context, pool *pgxpool.Pool) (*Store, error) {
	return openDatabase(ctx, pool, true)
}
func openDatabase(ctx context.Context, pool *pgxpool.Pool, recoverCloud bool) (*Store, error) {
	// Recovery must see the complete journal, never a silently filtered subset.
	var allowed bool
	err := pool.QueryRow(ctx, `SELECT rolsuper OR rolbypassrls OR EXISTS(SELECT 1 FROM pg_roles x WHERE x.rolname='sathonate_executor' AND pg_has_role(current_user,x.oid,'member')) FROM pg_roles WHERE rolname=current_user`).Scan(&allowed)
	if err != nil || !allowed {
		return nil, errors.New("executor database role is not explicitly authorized for journal recovery")
	}
	s := &Store{items: map[string]Operation{}, pool: pool}
	rows, err := pool.Query(ctx, "SELECT payload FROM sathonate_operations")
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var raw []byte
		var d diskRecord
		if err = rows.Scan(&raw); err != nil {
			rows.Close()
			return nil, err
		}
		if err = json.Unmarshal(raw, &d); err != nil {
			rows.Close()
			return nil, err
		}
		o := d.Operation
		o.IdempotencyKey = d.Key
		s.items[o.ID] = o
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for id, o := range s.items {
		if o.Status == "running" && !(recoverCloud && (o.Kind == "aws-eks-deploy" || o.Kind == "secure-local-deploy" || o.Kind == "secure-eks-deploy")) {
			now := time.Now().UTC()
			o.Status = "interrupted"
			o.Error = "Local executor restarted. Retry cancels the old worker Job before reconciling retained resources."
			o.FinishedAt = &now
			for i := range o.Stages {
				if o.Stages[i].Status == "running" {
					o.Stages[i].Status = "interrupted"
					o.Stages[i].FinishedAt = &now
				}
			}
			appendEvent(&o, o.Error)
			if err = s.persist(o); err != nil {
				return nil, err
			}
			s.items[id] = o
		}
	}
	return s, nil
}
func (s *Store) persistDatabase(o Operation) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	raw, err := json.Marshal(diskRecord{o, o.IdempotencyKey})
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO sathonate_operations(tenant_id,id,network_id,status,idempotency_key,payload) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(tenant_id,id) DO UPDATE SET status=EXCLUDED.status,payload=EXCLUDED.payload,updated_at=now()`, o.TenantID, o.ID, o.NetworkID, o.Status, o.IdempotencyKey, raw)
	return err
}
