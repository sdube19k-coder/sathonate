package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"github.com/coreos/go-oidc/v3/oidc"
	jose "github.com/go-jose/go-jose/v4"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sathonate.local/fabric-control-plane/internal/identity"
	"strconv"
	"strings"
	"testing"
	"time"
)

func secureOIDCFixture(t *testing.T, grants []identity.LocalGrant, options ...func(*identity.Config)) (*identity.Authenticator, []string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal("OIDC fixture key generation")
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
	t.Cleanup(server.Close)
	issuer = server.URL + "/oauth2/qualification"
	now := time.Now().Truncate(time.Second)
	members := []identity.Membership{}
	tokens := []string{}
	signer, _ := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, (&jose.SignerOptions{}).WithHeader("kid", "fixture"))
	for i, g := range grants {
		subject := "fixture-developer-" + strconv.Itoa(i)
		role := "admin"
		if len(g.Permissions) == 1 {
			role = "viewer"
		}
		members = append(members, identity.Membership{Environment: "production", Issuer: issuer, Subject: subject, Tenant: g.Tenant, Project: g.Project, Role: role, Permissions: g.Permissions, NotBefore: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour)})
		b, _ := json.Marshal(map[string]any{"iss": issuer, "sub": subject, "aud": "api://sathonate-test", "cid": "fixture-spa", "acr": identity.ProductionACR, "iat": now.Unix(), "exp": now.Add(time.Hour).Unix(), "sathonate_tenant": g.Tenant, "sathonate_project": g.Project, "scp": []string{"fabric.read", "fabric.write"}, "groups": []string{"sathonate-admin"}})
		signed, _ := signer.Sign(b)
		raw, _ := signed.CompactSerialize()
		tokens = append(tokens, raw)
	}
	b, _ := json.Marshal(members)
	path := filepath.Join(t.TempDir(), "memberships.json")
	if os.WriteFile(path, b, 0600) != nil {
		t.Fatal("membership fixture write")
	}
	config := identity.Config{Mode: "okta", Issuer: issuer, ClientID: "fixture-spa", Audience: "api://sathonate-test", MembershipsFile: path}
	for _, option := range options {
		option(&config)
	}
	auth, err := identity.New(oidc.ClientContext(context.Background(), server.Client()), config)
	if err != nil {
		t.Fatal("OIDC fixture configuration", err)
	}
	return auth, tokens
}
