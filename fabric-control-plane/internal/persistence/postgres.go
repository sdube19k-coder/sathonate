// Package persistence provides tenant-scoped PostgreSQL repositories. No tenant comes from request JSON.
package persistence

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"sathonate.local/fabric-control-plane/internal/audit"
	"sathonate.local/fabric-control-plane/internal/domain"
	"sathonate.local/fabric-control-plane/internal/drafts"
	"sathonate.local/fabric-control-plane/internal/maintenance"
	"sathonate.local/fabric-control-plane/internal/operations"
)

const schema = `
CREATE TABLE IF NOT EXISTS sathonate_networks (
 tenant_id text NOT NULL, id text NOT NULL, network_key text NOT NULL,
 revision bigint NOT NULL CHECK(revision > 0), archived boolean NOT NULL DEFAULT false,
 updated_at timestamptz NOT NULL DEFAULT now(), payload jsonb NOT NULL,
 PRIMARY KEY(tenant_id,id)
);
CREATE UNIQUE INDEX IF NOT EXISTS sathonate_network_key_active ON sathonate_networks(tenant_id,network_key) WHERE NOT archived;
CREATE TABLE IF NOT EXISTS sathonate_network_revisions (
 tenant_id text NOT NULL, network_id text NOT NULL, revision bigint NOT NULL,
 desired jsonb NOT NULL, recorded_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(tenant_id,network_id,revision),
 FOREIGN KEY(tenant_id,network_id) REFERENCES sathonate_networks(tenant_id,id)
);
ALTER TABLE sathonate_networks ENABLE ROW LEVEL SECURITY;
ALTER TABLE sathonate_networks FORCE ROW LEVEL SECURITY;
ALTER TABLE sathonate_network_revisions ENABLE ROW LEVEL SECURITY;
ALTER TABLE sathonate_network_revisions FORCE ROW LEVEL SECURITY;
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_policies WHERE tablename='sathonate_networks' AND policyname='sathonate_tenant') THEN
  CREATE POLICY sathonate_tenant ON sathonate_networks USING(tenant_id=current_setting('sathonate.tenant_id',true)) WITH CHECK(tenant_id=current_setting('sathonate.tenant_id',true));
 END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_policies WHERE tablename='sathonate_network_revisions' AND policyname='sathonate_tenant') THEN
  CREATE POLICY sathonate_tenant ON sathonate_network_revisions USING(tenant_id=current_setting('sathonate.tenant_id',true)) WITH CHECK(tenant_id=current_setting('sathonate.tenant_id',true));
 END IF;
END $$;
`

type Database struct{ pool *pgxpool.Pool }

func Open(ctx context.Context, connection string, requireTLS bool) (*Database, error) {
	cfg, e := pgxpool.ParseConfig(connection)
	if e != nil {
		return nil, e
	}
	cfg.MaxConns = 12
	if requireTLS && !verifiedPostgresTLS(cfg) {
		return nil, errors.New("production PostgreSQL requires verified TLS (sslmode=verify-full)")
	}
	pool, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		return nil, e
	}
	if e = pool.Ping(ctx); e != nil {
		pool.Close()
		return nil, e
	}
	if requireTLS {
		var unsafe bool
		if e = pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_roles WHERE pg_has_role(current_user,oid,'MEMBER') AND (rolsuper OR rolbypassrls OR rolcreaterole OR rolcreatedb OR rolreplication OR left(rolname,3)='pg_' OR rolname='sathonate_identity_admin')) OR EXISTS (SELECT 1 FROM pg_class WHERE relnamespace='public'::regnamespace AND relname LIKE 'sathonate_%' AND pg_has_role(current_user,relowner,'MEMBER')) OR EXISTS (SELECT 1 FROM pg_class WHERE oid=to_regclass('public.sathonate_oidc_memberships') AND has_table_privilege(current_user,oid,'INSERT,UPDATE,DELETE,TRUNCATE,REFERENCES,TRIGGER'))").Scan(&unsafe); e != nil || unsafe {
			pool.Close()
			return nil, errors.New("production API database role must not own application tables or inherit privileged roles")
		}
	}
	return &Database{pool: pool}, nil
}

// Migrate is an explicit administrative command, not implicitly invoked by each API process.
func (d *Database) Migrate(ctx context.Context) error {
	_, e := d.pool.Exec(ctx, schema+operations.SQLSchema+maintenance.SQLSchema+OIDCMembershipSchema+audit.SQLSchema)
	return e
}
func (d *Database) Pool() *pgxpool.Pool { return d.pool }
func (d *Database) Close()              { d.pool.Close() }
func (d *Database) ForTenant(ctx context.Context, tenant string) *Repository {
	return &Repository{ctx: ctx, tenant: tenant, pool: d.pool}
}

type Repository struct {
	ctx    context.Context
	tenant string
	pool   *pgxpool.Pool
}

func (r *Repository) transaction(readOnly bool) (pgx.Tx, error) {
	if r.tenant == "" {
		return nil, errors.New("tenant identity required")
	}
	options := pgx.TxOptions{}
	if readOnly {
		options.AccessMode = pgx.ReadOnly
	}
	tx, e := r.pool.BeginTx(r.ctx, options)
	if e != nil {
		return nil, e
	}
	if _, e = tx.Exec(r.ctx, "SELECT set_config('sathonate.tenant_id',$1,true)", r.tenant); e != nil {
		_ = tx.Rollback(r.ctx)
		return nil, e
	}
	return tx, nil
}

func (r *Repository) List() ([]drafts.Draft, error) {
	if r.tenant == "" {
		return nil, errors.New("tenant identity required")
	}
	tx, e := r.transaction(true)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(r.ctx)
	rows, e := tx.Query(r.ctx, "SELECT payload FROM sathonate_networks WHERE tenant_id=$1 AND NOT archived ORDER BY updated_at DESC,id", r.tenant)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []drafts.Draft{}
	for rows.Next() {
		var b []byte
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		var d drafts.Draft
		if e = json.Unmarshal(b, &d); e != nil {
			return nil, e
		}
		out = append(out, d)
	}
	if e = rows.Err(); e != nil {
		return nil, e
	}
	rows.Close()
	if e = tx.Commit(r.ctx); e != nil {
		return nil, e
	}
	return out, nil
}
func read(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, tenant, id string, lock bool) (drafts.Draft, error) {
	if tenant == "" {
		return drafts.Draft{}, errors.New("tenant identity required")
	}
	sql := "SELECT payload FROM sathonate_networks WHERE tenant_id=$1 AND id=$2"
	if lock {
		sql += " FOR UPDATE"
	}
	var b []byte
	e := q.QueryRow(ctx, sql, tenant, id).Scan(&b)
	if errors.Is(e, pgx.ErrNoRows) {
		return drafts.Draft{}, drafts.ErrNotFound
	}
	if e != nil {
		return drafts.Draft{}, e
	}
	var d drafts.Draft
	e = json.Unmarshal(b, &d)
	return d, e
}
func (r *Repository) Get(id string) (drafts.Draft, error) {
	tx, e := r.transaction(true)
	if e != nil {
		return drafts.Draft{}, e
	}
	defer tx.Rollback(r.ctx)
	d, e := read(r.ctx, tx, r.tenant, id, false)
	if e != nil {
		return drafts.Draft{}, e
	}
	if e = tx.Commit(r.ctx); e != nil {
		return drafts.Draft{}, e
	}
	return d, nil
}
func (r *Repository) change(id string, transform func(*drafts.Draft) (drafts.Draft, error)) (drafts.Draft, error) {
	if r.tenant == "" {
		return drafts.Draft{}, errors.New("tenant identity required")
	}
	tx, e := r.transaction(false)
	if e != nil {
		return drafts.Draft{}, e
	}
	defer tx.Rollback(r.ctx)
	var old *drafts.Draft
	if id != "" {
		d, e := read(r.ctx, tx, r.tenant, id, true)
		if e != nil {
			return drafts.Draft{}, e
		}
		old = &d
	}
	d, e := transform(old)
	if e != nil {
		return drafts.Draft{}, e
	}
	b, e := json.Marshal(d)
	if e != nil {
		return drafts.Draft{}, e
	}
	_, e = tx.Exec(r.ctx, `INSERT INTO sathonate_networks(tenant_id,id,network_key,revision,archived,updated_at,payload) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(tenant_id,id) DO UPDATE SET revision=EXCLUDED.revision,archived=EXCLUDED.archived,updated_at=EXCLUDED.updated_at,payload=EXCLUDED.payload`, r.tenant, d.ID, d.Desired.Metadata.ID, d.Revision, d.Archived, d.UpdatedAt, b)
	if e != nil {
		var pgErr *pgconn.PgError
		if errors.As(e, &pgErr) && pgErr.Code == "23505" {
			return drafts.Draft{}, drafts.ErrDuplicate
		}
		return drafts.Draft{}, e
	}
	desired, e := json.Marshal(d.Desired)
	if e != nil {
		return drafts.Draft{}, e
	}
	if _, e = tx.Exec(r.ctx, `INSERT INTO sathonate_network_revisions(tenant_id,network_id,revision,desired) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, r.tenant, d.ID, d.Revision, desired); e != nil {
		return drafts.Draft{}, e
	}
	if e = tx.Commit(r.ctx); e != nil {
		return drafts.Draft{}, e
	}
	return d, nil
}
func (r *Repository) Save(s domain.NetworkSpec, id string, revision int) (drafts.Draft, error) {
	return r.change(id, func(old *drafts.Draft) (drafts.Draft, error) { return drafts.BuildDraft(s, old, revision) })
}
func (r *Repository) Review(id string, revision int) (drafts.Draft, error) {
	return r.change(id, func(old *drafts.Draft) (drafts.Draft, error) { return drafts.BuildReview(*old, revision) })
}
func (r *Repository) Archive(id string, revision int) error {
	_, e := r.change(id, func(old *drafts.Draft) (drafts.Draft, error) { return drafts.BuildArchive(*old, revision) })
	return e
}

// Refuse every plaintext/weak fallback, not just the preferred connection.
func verifiedPostgresTLS(cfg *pgxpool.Config) bool {
	valid := func(c *tls.Config) bool { return c != nil && !c.InsecureSkipVerify && c.ServerName != "" }
	if !valid(cfg.ConnConfig.TLSConfig) {
		return false
	}
	if cfg.ConnConfig.TLSConfig.MinVersion < tls.VersionTLS12 {
		cfg.ConnConfig.TLSConfig.MinVersion = tls.VersionTLS12
	}
	for _, fallback := range cfg.ConnConfig.Fallbacks {
		if !valid(fallback.TLSConfig) {
			return false
		}
		if fallback.TLSConfig.MinVersion < tls.VersionTLS12 {
			fallback.TLSConfig.MinVersion = tls.VersionTLS12
		}
	}
	return true
}
