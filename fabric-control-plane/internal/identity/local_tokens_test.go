package identity

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLocalTokensAuthenticationExpiryPermissionsAndProjectBinding(t *testing.T) {
	token, digest, e := GenerateLocalToken()
	if e != nil {
		t.Fatal(e)
	}
	grant := LocalGrant{TokenSHA256: digest, Developer: "developer-a", Tenant: "tenant-a", Project: "project-a", Permissions: []string{"read", "deploy"}, ExpiresAt: time.Now().Add(time.Hour)}
	a, e := NewLocalTokens("local-development", []LocalGrant{grant})
	if e != nil {
		t.Fatal(e)
	}
	observed := Principal{}
	handler := a.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { observed = FromContext(r.Context()); w.WriteHeader(204) }))
	invoke := func(method, path, credential string) int {
		r := httptest.NewRequest(method, path, nil)
		if credential != "" {
			r.Header.Set("Authorization", "Bearer "+credential)
		}
		r.Header.Set("X-Tenant", "foreign")
		r.Header.Set("X-Project", "foreign")
		r.Header.Set("X-Role", "admin")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if strings.Contains(w.Body.String(), token) {
			t.Fatal("credential exposed")
		}
		return w.Code
	}
	if invoke("GET", "/api/v1/networks", "") != 401 || invoke("GET", "/api/v1/networks", strings.Repeat("0", 64)) != 401 {
		t.Fatal("unauthorized token accepted")
	}
	if invoke("GET", "/api/v1/networks", token) != 204 || observed.TenantID != "tenant-a" || observed.ProjectID != "project-a" || observed.Subject != "developer-a" || observed.CanAdminister() {
		t.Fatal("server grant altered by client")
	}
	if invoke("POST", "/api/v1/networks/id/deploy", token) != 204 || invoke("POST", "/api/v1/operations/id/retry", token) != 403 || invoke("DELETE", "/api/v1/deployments/id", token) != 403 || invoke("POST", "/api/v1/networks", token) != 403 {
		t.Fatal("local permission escalation")
	}
	scope := observed.StorageTenant()
	foreign := observed
	foreign.ProjectID = "project-b"
	if scope == foreign.StorageTenant() {
		t.Fatal("cross-project scope alias")
	}
	foreign = observed
	foreign.TenantID = "tenant-b"
	if scope == foreign.StorageTenant() {
		t.Fatal("cross-tenant scope alias")
	}
	a.now = func() time.Time { return grant.ExpiresAt.Add(time.Second) }
	if invoke("GET", "/api/v1/networks", token) != 401 {
		t.Fatal("expired token accepted")
	}
	for _, environment := range []string{"", "staging", "production", "development"} {
		if _, e = NewLocalTokens(environment, []LocalGrant{grant}); e == nil {
			t.Fatal("ambiguous environment enabled local auth")
		}
	}
	grant.ExpiresAt = time.Now().Add(-time.Second)
	if _, e = NewLocalTokens("local-development", []LocalGrant{grant}); e == nil {
		t.Fatal("expired grant loaded")
	}
}
