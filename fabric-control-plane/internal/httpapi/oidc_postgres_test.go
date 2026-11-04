package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sathonate.local/fabric-control-plane/internal/deployment"
	"sathonate.local/fabric-control-plane/internal/domain"
	"sathonate.local/fabric-control-plane/internal/identity"
	"sathonate.local/fabric-control-plane/internal/localfabric"
	"sathonate.local/fabric-control-plane/internal/operations"
	"sathonate.local/fabric-control-plane/internal/persistence"
	"testing"
	"time"
)

// Actual JWT verification, PostgreSQL authority/RLS, scoped API repositories and
// immutable history reads. The issuer is controlled; this is not real Okta PASS.
func TestPostgresOIDCReadOnlyAuthorization(t *testing.T) {
	connection := os.Getenv("SATHONATE_TEST_DATABASE_URL")
	if connection == "" {
		t.Skip("requires dedicated PostgreSQL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := persistence.Open(ctx, connection, false)
	if err != nil {
		t.Fatal("database unavailable")
	}
	defer db.Close()
	if db.Migrate(ctx) != nil {
		t.Fatal("schema migration")
	}
	suffix := fmt.Sprintf("%x", time.Now().UnixNano())
	tenant, project := "oidc-http-"+suffix, "project-a"
	scope := (identity.Principal{TenantID: tenant, ProjectID: project}).StorageTenant()
	foreignScope := (identity.Principal{TenantID: tenant, ProjectID: "project-b"}).StorageTenant()
	file, err := os.Open("../../examples/secure-local-network.json")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	spec, err := domain.Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	owned, err := db.ForTenant(ctx, scope).Save(spec, "", 0)
	if err != nil {
		t.Fatal("owned fixture")
	}
	foreign, err := db.ForTenant(ctx, foreignScope).Save(spec, "", 0)
	if err != nil {
		t.Fatal("foreign fixture")
	}
	defer func() {
		_, _ = db.Pool().Exec(context.Background(), "DELETE FROM sathonate_oidc_memberships WHERE storage_scope=$1", scope)
		for _, table := range []string{"sathonate_deployments", "sathonate_operations", "sathonate_network_revisions", "sathonate_networks"} {
			_, _ = db.Pool().Exec(context.Background(), "DELETE FROM "+table+" WHERE tenant_id=ANY($1)", []string{scope, foreignScope})
		}
	}()
	source := persistence.MembershipSource{Pool: db.Pool()}
	auth, tokens := secureOIDCFixture(t, []identity.LocalGrant{{Tenant: tenant, Project: project, Permissions: []string{"read"}}, {Tenant: tenant, Project: project, Permissions: []string{"read"}}}, func(c *identity.Config) {
		c.MembershipsFile = ""
		c.MembershipSource = source
		c.Environment = "local-development"
		c.ServerBoundProject = true
		c.LocalIdentityProbe = true
	})
	now := time.Now()
	grant := identity.Membership{Issuer: auth.Config().Issuer, Environment: "local-development", Subject: "fixture-developer-0", Tenant: tenant, Project: project, Role: "viewer", Permissions: []string{"read"}, NotBefore: now.Add(-time.Hour), ExpiresAt: now.Add(time.Hour)}
	if db.GrantDevelopmentReadOnly(ctx, grant, owned.ID) != nil {
		t.Fatal("approved membership")
	}
	id := fmt.Sprintf("op_%032x", time.Now().UnixNano())
	op := operations.Operation{ID: id, TenantID: scope, NetworkID: owned.ID, Actor: "fixture", Status: "succeeded", Kind: "secure-local-deploy"}
	opData, _ := json.Marshal(op)
	recorded, _ := json.Marshal(deployment.Result{OperationID: id, Network: spec, Phase: "succeeded", Ready: true, Verified: true, Health: &localfabric.Health{Ready: true, CheckedAt: now.Add(-time.Hour)}})
	if _, err = db.Pool().Exec(ctx, "INSERT INTO sathonate_operations(tenant_id,id,network_id,status,payload) VALUES($1,$2,$3,'succeeded',$4)", scope, id, owned.ID, opData); err != nil {
		t.Fatal("operation fixture")
	}
	if _, err = db.Pool().Exec(ctx, "INSERT INTO sathonate_deployments(tenant_id,operation_id,payload) VALUES($1,$2,$3)", scope, id, recorded); err != nil {
		t.Fatal("history fixture")
	}
	server := httptest.NewUnstartedServer(nil)
	host := server.Listener.Addr().String()
	handler := NewConfigured(func(r *http.Request) Repository {
		return db.ForTenant(r.Context(), identity.FromContext(r.Context()).StorageTenant())
	}, []string{host}, nil, Options{Auth: auth.Config(), ReadHistory: func(ctx context.Context, scope string) (*deployment.Manager, error) {
		return deployment.OpenHistoryDatabase(ctx, db.Pool(), []string{scope})
	}})
	server.Config.Handler = auth.Middleware(handler)
	server.Start()
	defer server.Close()
	call := func(method, path, token string) (int, []byte) {
		t.Helper()
		r, _ := http.NewRequestWithContext(ctx, method, server.URL+path, bytes.NewBufferString("{}"))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("If-Match", "\"0\"")
		r.Header.Set("X-Tenant-ID", foreignScope)
		response, err := server.Client().Do(r)
		if err != nil {
			t.Fatal("request failed")
		}
		defer response.Body.Close()
		var body bytes.Buffer
		_, _ = body.ReadFrom(response.Body)
		return response.StatusCode, body.Bytes()
	}
	for _, tc := range []struct {
		method, path string
		want         int
	}{
		{"GET", "/api/v1/operations/" + id + "/diagnostics", 200},
		{"GET", "/api/v1/projects", 200}, {"GET", "/api/v1/networks/" + owned.ID, 200}, {"GET", "/api/v1/networks/" + foreign.ID, 404}, {"GET", "/api/v1/operations", 200}, {"GET", "/api/v1/operations/" + id, 200}, {"GET", "/api/v1/deployments/" + id, 200}, {"GET", "/api/v1/deployments/" + id + "/health", 200},
		{"POST", "/api/v1/networks", 403}, {"DELETE", "/api/v1/networks/" + owned.ID, 403}, {"POST", "/api/v1/networks/" + owned.ID + "/deploy", 403}, {"POST", "/api/v1/operations/" + id + "/retry", 403}, {"POST", "/api/v1/deployments/" + id + "/cancel", 403}, {"POST", "/api/v1/deployments/" + id + "/rotation-plans", 403}, {"POST", "/api/v1/memberships", 403},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			status, body := call(tc.method, tc.path, tokens[0])
			if status != tc.want {
				t.Fatalf("status %d want %d", status, tc.want)
			}
			if tc.path == "/api/v1/deployments/"+id && tc.method == "GET" {
				var result deployment.Result
				if json.Unmarshal(body, &result) != nil || result.Ready || result.Verified || result.Mode != "history-only" {
					t.Fatal("snapshot claimed live execution")
				}
			}
		})
	}
	if status, _ := call("GET", "/api/v1/projects", tokens[1]); status != 401 {
		t.Fatal("unassigned verified subject accepted")
	}
	if status, _ := call("GET", "/api/v1/projects", "invalid-token"); status != 401 {
		t.Fatal("invalid signature accepted")
	}
	if _, err = db.Pool().Exec(ctx, "UPDATE sathonate_oidc_memberships SET expires_at=$1 WHERE issuer=$2 AND subject=$3", now.Add(-time.Second), grant.Issuer, grant.Subject); err != nil {
		t.Fatal("revocation fixture")
	}
	if status, _ := call("GET", "/api/v1/projects", tokens[0]); status != 401 {
		t.Fatal("database revocation was not immediate")
	}
	if status, _ := call("GET", "/api/v1/operations/"+id+"/diagnostics", tokens[0]); status != 401 {
		t.Fatal("diagnostics ignored membership revocation")
	}
	if status, _ := call("GET", "/api/v1/auth/identity", tokens[0]); status != 200 {
		t.Fatal("signed identity probe failed after grant removal")
	}
}
