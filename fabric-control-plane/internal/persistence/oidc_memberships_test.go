package persistence

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"sathonate.local/fabric-control-plane/internal/domain"
	"sathonate.local/fabric-control-plane/internal/identity"
	"strings"
	"testing"
	"time"
)

func TestPostgresOIDCMembershipAuthorityAndOwnership(t *testing.T) {
	connection := os.Getenv("SATHONATE_TEST_DATABASE_URL")
	if connection == "" {
		t.Skip("requires dedicated PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	db, err := Open(ctx, connection, false)
	if err != nil {
		t.Fatal("database unavailable")
	}
	defer db.Close()
	if db.Migrate(ctx) != nil {
		t.Fatal("membership schema migration")
	}
	suffix := strings.ReplaceAll(time.Now().Format("150405.000000000"), ".", "")
	tenant, project := "oidc-fixture-"+suffix, "project-a"
	scope := (identity.Principal{TenantID: tenant, ProjectID: project}).StorageTenant()
	file, err := os.Open("../../examples/secure-local-network.json")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	spec, err := domain.Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	network, err := db.ForTenant(ctx, scope).Save(spec, "", 0)
	if err != nil {
		t.Fatal("existing project fixture")
	}
	role := "oidc_reader_" + suffix
	defer func() {
		_, _ = db.pool.Exec(context.Background(), "DELETE FROM sathonate_oidc_memberships WHERE storage_scope=$1", scope)
		_, _ = db.pool.Exec(context.Background(), "DELETE FROM sathonate_network_revisions WHERE tenant_id=$1", scope)
		_, _ = db.pool.Exec(context.Background(), "DELETE FROM sathonate_networks WHERE tenant_id=$1", scope)
		_, _ = db.pool.Exec(context.Background(), "DROP OWNED BY "+pgx.Identifier{role}.Sanitize())
		_, _ = db.pool.Exec(context.Background(), "DROP ROLE "+pgx.Identifier{role}.Sanitize())
	}()
	now := time.Now()
	g := identity.Membership{Environment: "local-development", Issuer: "https://oidc-fixture.example/oauth2/default", Subject: "verified-fixture-subject-" + suffix, Tenant: tenant, Project: project, Role: "viewer", Permissions: []string{"read"}, NotBefore: now.Add(-time.Hour), ExpiresAt: now.Add(time.Hour)}
	if db.GrantDevelopmentReadOnly(ctx, g, network.ID) != nil {
		t.Fatal("approved existing development scope rejected")
	}
	for _, tc := range []struct {
		name   string
		change func(*identity.Membership)
	}{
		{"production", func(g *identity.Membership) { g.Environment = "production" }},
		{"administrator", func(g *identity.Membership) { g.Role = "admin" }},
		{"write", func(g *identity.Membership) { g.Permissions = []string{"read", "write"} }},
		{"foreign tenant", func(g *identity.Membership) { g.Tenant = "other-tenant" }},
		{"foreign project", func(g *identity.Membership) { g.Project = "other-project" }},
		{"long lived", func(g *identity.Membership) { g.ExpiresAt = now.Add(5 * time.Hour) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := g
			tc.change(&changed)
			if db.GrantDevelopmentReadOnly(ctx, changed, network.ID) == nil {
				t.Fatal("unauthorized development membership accepted")
			}
		})
	}
	if _, err = db.pool.Exec(ctx, "CREATE ROLE "+pgx.Identifier{role}.Sanitize()+" NOLOGIN; GRANT USAGE ON SCHEMA public TO "+pgx.Identifier{role}.Sanitize()+"; GRANT SELECT ON sathonate_oidc_memberships TO "+pgx.Identifier{role}.Sanitize()); err != nil {
		t.Fatal("restricted reader fixture")
	}
	cfg, err := pgxpool.ParseConfig(connection)
	if err != nil {
		t.Fatal("reader config")
	}
	cfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		_, e := c.Exec(ctx, "SET ROLE "+pgx.Identifier{role}.Sanitize())
		return e
	}
	reader, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal("reader connection")
	}
	defer reader.Close()
	source := MembershipSource{Pool: reader}
	if source.Ready(ctx) != nil {
		t.Fatal("forced RLS metadata")
	}
	grants, err := source.Lookup(ctx, g.Issuer, g.Subject, g.Environment)
	if err != nil || len(grants) != 1 || grants[0].Tenant != tenant || grants[0].Project != project || grants[0].Role != "viewer" {
		t.Fatal("verified subject lookup failed")
	}
	for _, binding := range [][3]string{{"https://other.example/oauth2/default", g.Subject, g.Environment}, {g.Issuer, "other-subject", g.Environment}, {g.Issuer, g.Subject, "production"}} {
		found, err := source.Lookup(ctx, binding[0], binding[1], binding[2])
		if err != nil || len(found) != 0 {
			t.Fatal("cross-issuer/subject/environment membership leaked")
		}
	}
	var count int
	if reader.QueryRow(ctx, "SELECT count(*) FROM sathonate_oidc_memberships").Scan(&count) != nil || count != 0 {
		t.Fatal("membership context leaked from transaction")
	}
	if _, err = reader.Exec(ctx, "DELETE FROM sathonate_oidc_memberships"); err == nil {
		t.Fatal("API reader modified membership policy")
	}
}
