package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"sathonate.local/fabric-control-plane/internal/deployment"
	"sathonate.local/fabric-control-plane/internal/identity"
	"sathonate.local/fabric-control-plane/internal/operations"
)

func TestDiagnosticsAuthorizationAndPayloadAllowlist(t *testing.T) {
	m, err := deployment.Open(t.TempDir(), deployment.MockRunner{})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	var grants []identity.LocalGrant
	var tokens []string
	for _, scope := range [][2]string{{"tenant-a", "project-a"}, {"tenant-b", "project-a"}, {"tenant-a", "project-b"}} {
		token, digest, err := identity.GenerateLocalToken()
		if err != nil {
			t.Fatal(err)
		}
		tokens = append(tokens, token)
		grants = append(grants, identity.LocalGrant{TokenSHA256: digest, Developer: "viewer", Tenant: scope[0], Project: scope[1], Permissions: []string{"read"}, ExpiresAt: time.Now().Add(time.Hour)})
	}
	auth, err := identity.NewLocalTokens("local-development", grants)
	if err != nil {
		t.Fatal(err)
	}
	scope := (identity.Principal{TenantID: "tenant-a", ProjectID: "project-a"}).StorageTenant()
	op, _, err := m.Operations().Begin(scope, "sensitive-actor-canary", "network-a", "deploy", "sensitive-key-canary", "digest", "version", "", []operations.Stage{{Key: "sensitive-label-canary", Label: "sensitive-label-canary", Status: "pending"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Operations().Event(op.ID, "sensitive-event-canary"); err != nil {
		t.Fatal(err)
	}
	h := auth.Middleware(NewConfigured(nil, []string{"localhost:8080"}, nil, Options{Auth: auth.Config(), Deployments: m}))
	path := "/api/v1/operations/" + op.ID + "/diagnostics"
	for i, token := range append(tokens, "") {
		w := request(h, http.MethodGet, path, nil, map[string]string{"Authorization": "Bearer " + token})
		want := 404
		if i == 0 {
			want = 200
		}
		if i == 3 {
			want = 401
		}
		if w.Code != want {
			t.Fatalf("scope %d status %d want %d", i, w.Code, want)
		}
		if i == 0 {
			var report map[string]any
			if json.Unmarshal(w.Body.Bytes(), &report) != nil || report["liveHealthChecked"] != false || report["evidenceType"] != "recorded-operation" {
				t.Fatal("report confuses recorded and live state")
			}
			for _, secret := range append(tokens, "sensitive-", scope) {
				if strings.Contains(w.Body.String(), secret) {
					t.Fatal("diagnostic payload exceeded allowlist")
				}
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("diagnostics cached")
			}
		}
	}
}
