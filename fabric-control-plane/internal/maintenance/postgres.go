package maintenance

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const SQLSchema = `
CREATE TABLE IF NOT EXISTS sathonate_maintenance (
 tenant_id text NOT NULL,id text NOT NULL,key_sha256 text NOT NULL,binding_sha256 text NOT NULL,
 revision bigint NOT NULL CHECK(revision>0),payload jsonb NOT NULL,updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(tenant_id,id),UNIQUE(tenant_id,key_sha256));
ALTER TABLE sathonate_maintenance ENABLE ROW LEVEL SECURITY;
ALTER TABLE sathonate_maintenance FORCE ROW LEVEL SECURITY;
DO $$ BEGIN IF NOT EXISTS(SELECT 1 FROM pg_policies WHERE tablename='sathonate_maintenance' AND policyname='sathonate_tenant') THEN
 CREATE POLICY sathonate_tenant ON sathonate_maintenance USING(tenant_id=current_setting('sathonate.tenant_id',true)) WITH CHECK(tenant_id=current_setting('sathonate.tenant_id',true));
END IF; END $$;
`

type Store struct{ Pool *pgxpool.Pool }

func (s Store) transaction(ctx context.Context, tenant string) (pgx.Tx, error) {
	if s.Pool == nil || tenant == "" {
		return nil, ErrDenied
	}
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return nil, errors.New("maintenance storage unavailable")
	}
	if _, e = tx.Exec(ctx, "SELECT set_config('sathonate.tenant_id',$1,true)", tenant); e != nil {
		tx.Rollback(ctx)
		return nil, errors.New("maintenance tenant scope unavailable")
	}
	return tx, nil
}
func (s Store) Create(ctx context.Context, intent Intent) (Intent, error) {
	if !intent.Binding.valid() || intent.Revision != 1 || intent.Phase != "planned" || intent.KeySHA256 == "" || intent.BindingSHA256 != hash([]string{intent.Binding.Digest(), intent.IdentityRef}) {
		return Intent{}, ErrDenied
	}
	tx, e := s.transaction(ctx, intent.Binding.TenantID)
	if e != nil {
		return Intent{}, e
	}
	defer tx.Rollback(ctx)
	raw, _ := json.Marshal(intent)
	_, e = tx.Exec(ctx, `INSERT INTO sathonate_maintenance(tenant_id,id,key_sha256,binding_sha256,revision,payload) VALUES($1,$2,$3,$4,1,$5) ON CONFLICT(tenant_id,key_sha256) DO NOTHING`, intent.Binding.TenantID, intent.ID, intent.KeySHA256, intent.BindingSHA256, raw)
	if e != nil {
		return Intent{}, errors.New("cannot persist maintenance intent")
	}
	var existing []byte
	var key string
	if e = tx.QueryRow(ctx, "SELECT payload,key_sha256 FROM sathonate_maintenance WHERE tenant_id=$1 AND key_sha256=$2", intent.Binding.TenantID, intent.KeySHA256).Scan(&existing, &key); e != nil {
		return Intent{}, errors.New("cannot read durable maintenance intent")
	}
	var result Intent
	if json.Unmarshal(existing, &result) != nil || result.BindingSHA256 != intent.BindingSHA256 {
		return Intent{}, ErrConflict
	}
	result.KeySHA256 = key
	if tx.Commit(ctx) != nil {
		return Intent{}, errors.New("cannot commit maintenance intent")
	}
	return result, nil
}
func (s Store) Get(ctx context.Context, tenant, id string) (Intent, error) {
	tx, e := s.transaction(ctx, tenant)
	if e != nil {
		return Intent{}, e
	}
	defer tx.Rollback(ctx)
	var raw []byte
	var key string
	if tx.QueryRow(ctx, "SELECT payload,key_sha256 FROM sathonate_maintenance WHERE tenant_id=$1 AND id=$2", tenant, id).Scan(&raw, &key) != nil {
		return Intent{}, ErrDenied
	}
	var result Intent
	if json.Unmarshal(raw, &result) != nil {
		return Intent{}, errors.New("invalid maintenance record")
	}
	result.KeySHA256 = key
	return result, nil
}

// SaveCheckpoint is a compare-and-swap commit after a trusted driver's receipt.
// No HTTP endpoint accepts arbitrary Intent payloads or caller verification flags.
func (s Store) SaveCheckpoint(ctx context.Context, next Intent, expected int) error {
	if next.Revision != expected+1 || !next.Binding.valid() {
		return ErrConflict
	}
	tx, e := s.transaction(ctx, next.Binding.TenantID)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	raw, _ := json.Marshal(next)
	tag, e := tx.Exec(ctx, "UPDATE sathonate_maintenance SET payload=$1,revision=$2,updated_at=now() WHERE tenant_id=$3 AND id=$4 AND revision=$5 AND binding_sha256=$6", raw, next.Revision, next.Binding.TenantID, next.ID, expected, next.BindingSHA256)
	if e != nil || tag.RowsAffected() != 1 {
		return ErrConflict
	}
	if tx.Commit(ctx) != nil {
		return errors.New("cannot commit maintenance checkpoint")
	}
	return nil
}
