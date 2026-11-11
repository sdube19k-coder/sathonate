package httpapi

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"sathonate.local/fabric-control-plane/internal/audit"
	"sathonate.local/fabric-control-plane/internal/identity"
	"sathonate.local/fabric-control-plane/internal/persistence"
)

func TestPostgresProjectAuditIsolation(t *testing.T) {
	url := os.Getenv("SATHONATE_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires disposable PostgreSQL test database")
	}
	if !strings.Contains(url, "/fabric_smp_tests?") {
		t.Fatal("dedicated PostgreSQL required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := persistence.Open(ctx, url, false)
	if err != nil {
		t.Fatal("database unavailable")
	}
	defer db.Close()
	if db.Migrate(ctx) != nil {
		t.Fatal("migration failed")
	}
	suffix := time.Now().UTC().Format("20060102150405000000000")
	tenant := "audit-" + suffix
	var grants []identity.LocalGrant
	var tokens []string
	for i, project := range []string{"a", "b", "a", "a"} {
		token, digest, err := identity.GenerateLocalToken()
		if err != nil {
			t.Fatal(err)
		}
		permissions := []string{"read", "write", "cleanup"}
		if i == 2 {
			permissions = []string{"read"}
		}
		grantTenant := tenant
		if i == 3 {
			grantTenant += "-foreign"
		}
		grants = append(grants, identity.LocalGrant{TokenSHA256: digest, Developer: "audit-actor", Tenant: grantTenant, Project: project, Permissions: permissions, ExpiresAt: time.Now().Add(time.Hour)})
		tokens = append(tokens, token)
	}
	auth, err := identity.NewLocalTokens("local-development", grants)
	if err != nil {
		t.Fatal(err)
	}
	store := audit.Postgres{Pool: db.Pool()}
	if store.Ready(ctx, false) != nil {
		t.Fatal("journal not ready")
	}
	handler := func() http.Handler {
		return auth.Middleware(store.Journal().WrapProject(NewConfigured(nil, []string{"localhost:8080"}, nil, Options{Auth: auth.Config(), AuditRecent: store.Recent}), func(ctx context.Context) (string, string) {
			p := identity.FromContext(ctx)
			if !identity.IsProjectAuthenticated(ctx) {
				return "", ""
			}
			return p.StorageTenant(), p.Subject
		}))
	}
	get := func(h http.Handler, i int, path string) (int, string) {
		r := request(h, http.MethodGet, path, nil, map[string]string{"Authorization": "Bearer " + tokens[i], "X-Project-ID": "a"})
		return r.Code, r.Body.String()
	}
	scope := (identity.Principal{TenantID: tenant, ProjectID: "a"}).StorageTenant()
	other := (identity.Principal{TenantID: tenant, ProjectID: "b"}).StorageTenant()
	foreign := (identity.Principal{TenantID: tenant + "-foreign", ProjectID: "a"}).StorageTenant()
	defer db.Pool().Exec(context.Background(), "DELETE FROM public.sathonate_audit_events WHERE scope=ANY($1)", []string{scope, other, foreign})
	if code, _ := get(handler(), 0, "/api/v1/projects?private-query=ignored"); code != 200 {
		t.Fatalf("project read %d", code)
	}
	events, err := store.Recent(ctx, scope)
	if err != nil || len(events) != 2 || events[0].RequestID != events[1].RequestID {
		t.Fatal("durable attempt/completion missing")
	}
	marker := events[0].RequestID
	if code, body := get(handler(), 0, "/api/v1/audit"); code != 200 || !strings.Contains(body, marker) || strings.Contains(body, "private-query") || strings.Contains(body, "audit-actor") {
		t.Fatal("scoped audit read failed")
	}
	for _, i := range []int{1, 3} {
		if code, body := get(handler(), i, "/api/v1/audit?tenant="+tenant); code != 200 || strings.Contains(body, marker) {
			t.Fatal("foreign audit event disclosed")
		}
	}
	if code, _ := get(handler(), 2, "/api/v1/audit"); code != 403 {
		t.Fatal("viewer received audit access")
	}
	role := "smp_audit_" + suffix
	if _, err := db.Pool().Exec(ctx, "CREATE ROLE "+role+" NOLOGIN"); err != nil {
		t.Fatal("audit RLS test role unavailable")
	}
	defer db.Pool().Exec(context.Background(), "DROP ROLE "+role)
	defer db.Pool().Exec(context.Background(), "REVOKE SELECT,INSERT ON public.sathonate_audit_events FROM "+role)
	if _, err := db.Pool().Exec(ctx, "GRANT SELECT,INSERT ON public.sathonate_audit_events TO "+role); err != nil {
		t.Fatal("audit RLS grants unavailable")
	}
	tx, err := db.Pool().Begin(ctx)
	if err != nil {
		t.Fatal("audit RLS transaction unavailable")
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "SET LOCAL ROLE "+role); err != nil {
		t.Fatal("could not select restricted audit role")
	}
	if _, err := tx.Exec(ctx, "SELECT set_config('sathonate.audit_scope',$1,true)", scope); err != nil {
		t.Fatal("could not set audit scope")
	}
	var visible int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM public.sathonate_audit_events WHERE scope=$1", foreign).Scan(&visible); err != nil || visible != 0 {
		t.Fatal("database RLS disclosed foreign audit rows")
	}
	if _, err := tx.Exec(ctx, "INSERT INTO public.sathonate_audit_events VALUES($1,$2,'attempt',now(),'{}')", foreign, strings.Repeat("b", 32)); err == nil {
		t.Fatal("database RLS accepted a foreign audit append")
	}
}
