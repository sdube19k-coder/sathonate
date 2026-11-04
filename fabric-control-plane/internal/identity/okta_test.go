package identity

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	jose "github.com/go-jose/go-jose/v4"
)

func TestAccessTokenVerificationAndAuthorization(t *testing.T) {
	key, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		t.Fatal(e)
	}
	keys := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, Use: "sig", Algorithm: "RS256", KeyID: "test"}}})
	}))
	defer keys.Close()
	config := Config{Mode: "okta", Environment: "production", Issuer: "https://company.okta.example/oauth2/default", ClientID: "spa-client", Audience: "api://sathonate", TenantClaim: "sathonate_tenant", ProjectClaim: "sathonate_project"}
	auth := &Authenticator{memberships: []Membership{{Environment: "production", Issuer: config.Issuer, Subject: "person", Tenant: "business-a", Project: "project-a", Role: "operator", Permissions: []string{"read", "write"}, NotBefore: time.Now().Add(-time.Hour), ExpiresAt: time.Now().Add(time.Hour)}, {Environment: "production", Issuer: config.Issuer, Subject: "viewer", Tenant: "business-a", Project: "project-a", Role: "viewer", Permissions: []string{"read"}, NotBefore: time.Now().Add(-time.Hour), ExpiresAt: time.Now().Add(time.Hour)}}, config: config, verifier: oidc.NewVerifier(config.Issuer, oidc.NewRemoteKeySet(context.Background(), keys.URL), &oidc.Config{ClientID: config.Audience, SupportedSigningAlgs: []string{"RS256"}})}
	signer, e := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, (&jose.SignerOptions{}).WithHeader("kid", "test"))
	if e != nil {
		t.Fatal(e)
	}
	makeToken := func(change func(map[string]any)) string {
		claims := map[string]any{"iss": config.Issuer, "sub": "person", "aud": config.Audience, "cid": config.ClientID, "exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(), "sathonate_tenant": "business-a", "sathonate_project": "project-a", "groups": []string{"sathonate-admin"}, "scp": []string{"fabric.read", "fabric.write"}}
		if change != nil {
			change(claims)
		}
		b, _ := json.Marshal(claims)
		signed, e := signer.Sign(b)
		if e != nil {
			t.Fatal(e)
		}
		raw, e := signed.CompactSerialize()
		if e != nil {
			t.Fatal(e)
		}
		return raw
	}
	raw := makeToken(nil)
	principal, e := auth.authenticate(context.Background(), raw)
	if e != nil || principal.TenantID != "business-a" || !principal.CanWrite() {
		t.Fatal("valid access token rejected", e)
	}
	for _, tc := range []struct {
		name   string
		change func(map[string]any)
	}{
		{"ID token audience", func(c map[string]any) { c["aud"] = config.ClientID }},
		{"wrong issuer", func(c map[string]any) { c["iss"] = "https://other.example" }},
		{"expired token", func(c map[string]any) { c["exp"] = time.Now().Add(-time.Hour).Unix() }},
		{"wrong client", func(c map[string]any) { c["cid"] = "other-client" }},
		{"missing tenant", func(c map[string]any) { delete(c, "sathonate_tenant") }},
		{"unknown subject", func(c map[string]any) { c["sub"] = "intruder" }},
		{"cross project", func(c map[string]any) { c["sathonate_project"] = "foreign" }},
		{"cross tenant", func(c map[string]any) { c["sathonate_tenant"] = "foreign" }},
		{"future issuance", func(c map[string]any) { c["iat"] = time.Now().Add(time.Hour).Unix() }},
		{"future nbf", func(c map[string]any) { c["nbf"] = time.Now().Add(time.Hour).Unix() }},
		{"missing iat", func(c map[string]any) { delete(c, "iat") }},
		{"excess lifetime", func(c map[string]any) { c["exp"] = time.Now().Add(2 * time.Hour).Unix() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, e := auth.authenticate(context.Background(), makeToken(tc.change)); e == nil {
				t.Fatal("unauthorized token accepted")
			}
		})
	}
	viewer := makeToken(func(c map[string]any) {
		c["sub"] = "viewer"
		c["groups"] = []string{"sathonate-admin"}
		c["permissions"] = []string{"write", "deploy", "cleanup"}
	})
	handler := auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if FromContext(r.Context()).TenantID == "" {
			t.Fatal("identity not propagated")
		}
		w.WriteHeader(200)
	}))
	for _, tc := range []struct {
		method, token string
		status        int
	}{{"GET", viewer, 200}, {"POST", viewer, 403}, {"POST", raw, 200}, {"GET", "not-a-jwt", 401}} {
		r := httptest.NewRequest(tc.method, "http://api.test/api/v1/networks", nil)
		r.Header.Set("Authorization", "Bearer "+tc.token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("%s got %d", tc.method, w.Code)
		}
	}
}
func TestInvalidProviderConfigurationFailsBeforeDiscovery(t *testing.T) {
	for _, issuer := range []string{"http://insecure.example/oauth2/default", "https://company.okta.example", "https://user:password@company.okta.example/oauth2/default"} {
		_, e := New(context.Background(), Config{Mode: "okta", Environment: "production", Issuer: issuer, ClientID: "spa", Audience: "api://sathonate"})
		if e == nil {
			t.Fatal("unsafe issuer configuration accepted")
		}
	}
	if _, e := New(context.Background(), Config{Mode: "disabled"}); e == nil || strings.Contains(e.Error(), "password") {
		t.Fatal("invalid auth mode accepted")
	}
}

// Exercises actual HTTPS discovery and remote JWKS through the public constructor.
// The fixture certificate is trusted only in this test client, never globally.
func TestConfiguredOIDCDiscoveryAndServerMembership(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	var issuer string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/keys") {
			_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, Use: "sig", Algorithm: "RS256", KeyID: "fixture"}}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"issuer": issuer, "jwks_uri": issuer + "/keys", "authorization_endpoint": issuer + "/authorize", "token_endpoint": issuer + "/token", "response_types_supported": []string{"code"}, "subject_types_supported": []string{"public"}, "id_token_signing_alg_values_supported": []string{"RS256"}})
	}))
	defer server.Close()
	issuer = server.URL + "/oauth2/test"
	now := time.Now().Truncate(time.Second)
	path := filepath.Join(t.TempDir(), "memberships.json")
	grants := []Membership{{Environment: "production", Issuer: issuer, Subject: "person", Tenant: "tenant", Project: "project", Role: "operator", Permissions: []string{"read", "deploy"}, NotBefore: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour)}}
	b, _ := json.Marshal(grants)
	if os.WriteFile(path, b, 0600) != nil {
		t.Fatal("write fixture")
	}
	config := Config{Mode: "okta", Environment: "production", Issuer: issuer, ClientID: "spa", Audience: "api://fabric", MembershipsFile: path}
	auth, err := New(oidc.ClientContext(context.Background(), server.Client()), config)
	if err != nil {
		t.Fatal(err)
	}
	signer, _ := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, (&jose.SignerOptions{}).WithHeader("kid", "fixture"))
	token := func(change func(map[string]any)) string {
		c := map[string]any{"iss": issuer, "sub": "person", "aud": "api://fabric", "cid": "spa", "iat": now.Unix(), "exp": now.Add(30 * time.Minute).Unix(), "sathonate_tenant": "tenant", "sathonate_project": "project", "scp": []string{"fabric.read", "fabric.write"}, "groups": []string{"sathonate-admin"}}
		if change != nil {
			change(c)
		}
		b, _ := json.Marshal(c)
		s, _ := signer.Sign(b)
		raw, _ := s.CompactSerialize()
		return raw
	}
	handler := auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := FromContext(r.Context())
		if p.CanAdminister() || p.ProjectID != "project" {
			t.Error("untrusted role or project")
		}
		w.WriteHeader(204)
	}))
	for _, tc := range []struct {
		name, method, path string
		change             func(map[string]any)
		status             int
	}{
		{"allowed deploy", "POST", "/api/v1/networks/n/deploy", nil, 204},
		{"status", "GET", "/api/v1/deployments/op", nil, 204},
		{"retry denied", "POST", "/api/v1/deployments/op/retry", nil, 403},
		{"cancel denied", "POST", "/api/v1/deployments/op/cancel", nil, 403},
		{"cleanup denied", "DELETE", "/api/v1/networks/n", nil, 403},
		{"tenant change", "GET", "/api/v1/networks", func(c map[string]any) { c["sathonate_tenant"] = "other" }, 401},
		{"project change", "GET", "/api/v1/networks", func(c map[string]any) { c["sathonate_project"] = "other" }, 401},
		{"revoked old issuance", "GET", "/api/v1/networks", func(c map[string]any) { c["iat"] = now.Add(-2 * time.Minute).Unix() }, 401},
		{"missing scope", "GET", "/api/v1/networks", func(c map[string]any) { c["scp"] = []string{"fabric.write"} }, 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, nil)
			r.Header.Set("Authorization", "Bearer "+token(tc.change))
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("got %d want %d", w.Code, tc.status)
			}
		})
	}
	auth.config.ServerBoundProject = true
	noScope := token(func(c map[string]any) { delete(c, "sathonate_tenant"); delete(c, "sathonate_project") })
	if p, err := auth.authenticate(context.Background(), noScope); err != nil || p.TenantID != "tenant" || p.ProjectID != "project" {
		t.Fatal("unique server-bound scope rejected", err)
	}
	if _, err := auth.authenticate(context.Background(), token(func(c map[string]any) { c["sathonate_project"] = "other" })); err == nil {
		t.Fatal("token selected a foreign server-bound project")
	}
	duplicate := auth.memberships[0]
	duplicate.Project = "other"
	auth.memberships = append(auth.memberships, duplicate)
	if _, err := auth.authenticate(context.Background(), noScope); err == nil {
		t.Fatal("ambiguous project granted")
	}
	auth.memberships = auth.memberships[:1]
	auth.config.LocalIdentityProbe = true
	auth.memberships = nil
	identityHandler := auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !IsVerifiedIdentity(r.Context()) || IsProjectAuthenticated(r.Context()) || FromContext(r.Context()).TenantID != "" {
			t.Error("identity probe leaked project authority")
		}
		w.WriteHeader(204)
	}))
	req := httptest.NewRequest("GET", "/api/v1/auth/identity", nil)
	req.Header.Set("Authorization", "Bearer "+noScope)
	response := httptest.NewRecorder()
	identityHandler.ServeHTTP(response, req)
	if response.Code != 204 {
		t.Fatal("signed identity probe failed", response.Code)
	}
	req = httptest.NewRequest("GET", "/api/v1/projects", nil)
	req.Header.Set("Authorization", "Bearer "+noScope)
	response = httptest.NewRecorder()
	identityHandler.ServeHTTP(response, req)
	if response.Code != 401 {
		t.Fatal("identity probe bypassed project membership")
	}
	// Bearer reuse is permitted while active; revocation is server policy, not a
	// fabricated one-time-token guarantee. Removing membership fails closed.
	auth.memberships = nil
	if _, err := auth.authenticate(context.Background(), token(nil)); err == nil {
		t.Fatal("revoked membership accepted")
	}
	wrongEnvironment := config
	wrongEnvironment.Environment = "local-development"
	if _, err := New(oidc.ClientContext(context.Background(), server.Client()), wrongEnvironment); err == nil {
		t.Fatal("production membership was silently reused in local development")
	}
	config.MembershipsFile = ""
	if _, err := New(context.Background(), config); err == nil {
		t.Fatal("missing policy accepted")
	}
}
