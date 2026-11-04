package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"sathonate.local/fabric-control-plane/internal/identity"
	"time"
)

const OIDCMembershipSchema = `
CREATE TABLE IF NOT EXISTS sathonate_oidc_memberships(
 environment text NOT NULL CHECK(environment IN ('local-development','production')),
 issuer text NOT NULL, subject text NOT NULL, tenant_id text NOT NULL, project_id text NOT NULL,
 storage_scope text NOT NULL, anchor_network_id text NOT NULL,
 granted_by text NOT NULL DEFAULT current_user, granted_at timestamptz NOT NULL DEFAULT now(),
 role text NOT NULL CHECK(role IN ('viewer','operator','admin')), permissions jsonb NOT NULL CHECK(jsonb_typeof(permissions)='array'),
 not_before timestamptz NOT NULL, expires_at timestamptz NOT NULL CHECK(expires_at>not_before),
 PRIMARY KEY(environment,issuer,subject,tenant_id,project_id),
 FOREIGN KEY(storage_scope,anchor_network_id) REFERENCES sathonate_networks(tenant_id,id));
REVOKE ALL ON sathonate_oidc_memberships FROM PUBLIC;
ALTER TABLE sathonate_oidc_memberships ENABLE ROW LEVEL SECURITY;
ALTER TABLE sathonate_oidc_memberships FORCE ROW LEVEL SECURITY;
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_policies WHERE tablename='sathonate_oidc_memberships' AND policyname='oidc_verified_subject') THEN
 CREATE POLICY oidc_verified_subject ON sathonate_oidc_memberships FOR SELECT USING(
 issuer=current_setting('sathonate.oidc_issuer',true) AND subject=current_setting('sathonate.oidc_subject',true) AND environment=current_setting('sathonate.oidc_environment',true));
 END IF;
 IF NOT EXISTS(SELECT 1 FROM pg_policies WHERE tablename='sathonate_oidc_memberships' AND policyname='oidc_membership_administrator') THEN
 CREATE POLICY oidc_membership_administrator ON sathonate_oidc_memberships FOR ALL USING(
 EXISTS(SELECT 1 FROM pg_roles r WHERE r.rolname='sathonate_identity_admin' AND pg_has_role(current_user,r.oid,'member')))
 WITH CHECK(EXISTS(SELECT 1 FROM pg_roles r WHERE r.rolname='sathonate_identity_admin' AND pg_has_role(current_user,r.oid,'member')));
 END IF;
END $$;`

// MembershipSource is a read-only identity boundary, separate from tenant data
// repositories and the privileged administrative membership writer.
type MembershipSource struct{ Pool *pgxpool.Pool }

func (s MembershipSource) Ready(ctx context.Context) error {
	var ready bool
	err := s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_class WHERE oid=to_regclass('public.sathonate_oidc_memberships') AND relrowsecurity AND relforcerowsecurity)`).Scan(&ready)
	if err != nil || !ready {
		return errors.New("OIDC membership schema/RLS is unavailable; explicit administrative migration required")
	}
	return nil
}
func (s MembershipSource) Lookup(ctx context.Context, issuer, subject, environment string) ([]identity.Membership, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, errors.New("membership lookup unavailable")
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SELECT set_config('sathonate.oidc_issuer',$1,true),set_config('sathonate.oidc_subject',$2,true),set_config('sathonate.oidc_environment',$3,true)", issuer, subject, environment); err != nil {
		return nil, errors.New("membership lookup unavailable")
	}
	rows, err := tx.Query(ctx, "SELECT issuer,subject,environment,tenant_id,project_id,role,permissions,not_before,expires_at,storage_scope FROM sathonate_oidc_memberships WHERE issuer=$1 AND subject=$2 AND environment=$3 ORDER BY tenant_id,project_id LIMIT 101", issuer, subject, environment)
	if err != nil {
		return nil, errors.New("membership lookup unavailable")
	}
	defer rows.Close()
	out := []identity.Membership{}
	for rows.Next() {
		var g identity.Membership
		var permissions []byte
		var scope string
		if rows.Scan(&g.Issuer, &g.Subject, &g.Environment, &g.Tenant, &g.Project, &g.Role, &permissions, &g.NotBefore, &g.ExpiresAt, &scope) != nil || json.Unmarshal(permissions, &g.Permissions) != nil {
			return nil, errors.New("invalid membership record")
		}
		if scope != (identity.Principal{TenantID: g.Tenant, ProjectID: g.Project}).StorageTenant() {
			return nil, errors.New("membership scope ownership mismatch")
		}
		out = append(out, g)
	}
	if rows.Err() != nil || len(out) > 100 {
		return nil, errors.New("membership lookup unavailable or ambiguous")
	}
	return out, nil
}

// GrantDevelopmentReadOnly is an administrative path, never called by an HTTP
// handler. It cannot create a tenant/project, and anchors existing scope ownership.
func (d *Database) GrantDevelopmentReadOnly(ctx context.Context, g identity.Membership, network string) error {
	now := time.Now()
	if g.Environment != "local-development" || g.Role != "viewer" || len(g.Permissions) != 1 || g.Permissions[0] != "read" || g.Subject == "" || g.Issuer == "" || g.Tenant == "" || g.Project == "" || g.NotBefore.IsZero() || !g.ExpiresAt.After(now) || g.ExpiresAt.After(now.Add(4*time.Hour)) || network == "" {
		return errors.New("only an explicit expiring read-only development membership may be granted")
	}
	scope := (identity.Principal{TenantID: g.Tenant, ProjectID: g.Project}).StorageTenant()
	permissions, _ := json.Marshal(g.Permissions)
	tag, err := d.pool.Exec(ctx, `INSERT INTO sathonate_oidc_memberships(environment,issuer,subject,tenant_id,project_id,storage_scope,anchor_network_id,role,permissions,not_before,expires_at)
 SELECT $1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11 WHERE EXISTS(SELECT 1 FROM sathonate_networks WHERE tenant_id=$6 AND id=$7 AND NOT archived)
 ON CONFLICT(environment,issuer,subject,tenant_id,project_id) DO UPDATE SET storage_scope=EXCLUDED.storage_scope,anchor_network_id=EXCLUDED.anchor_network_id,role=EXCLUDED.role,permissions=EXCLUDED.permissions,not_before=EXCLUDED.not_before,expires_at=EXCLUDED.expires_at,granted_by=current_user,granted_at=now()`, g.Environment, g.Issuer, g.Subject, g.Tenant, g.Project, scope, network, g.Role, permissions, g.NotBefore, g.ExpiresAt)
	if err != nil || tag.RowsAffected() != 1 {
		return errors.New("development membership was not granted; existing scope/network ownership is required")
	}
	return nil
}
