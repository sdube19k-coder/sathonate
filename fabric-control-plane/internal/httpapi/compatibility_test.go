package httpapi

import (
	"net/http"
	"testing"

	"sathonate.local/fabric-control-plane/internal/identity"
)

func TestCompatibilityCatalogIsOptInAndProjectAuthorized(t *testing.T) {
	const path = "/api/v1/compatibility"
	auth, tokens := secureOIDCFixture(t, []identity.LocalGrant{
		{Tenant: "tenant-a", Project: "project-a", Permissions: []string{"read"}},
		{Tenant: "tenant-b", Project: "project-b", Permissions: []string{"read"}},
	})
	makeHandler := func(enabled bool) http.Handler {
		return auth.Middleware(NewConfigured(func(*http.Request) Repository {
			t.Fatal("catalog route must not access a repository")
			return nil
		}, []string{"localhost:8080"}, nil, Options{CompatibilityCatalog: enabled}))
	}
	headers := map[string]string{"Authorization": "Bearer " + tokens[0]}
	if response := request(makeHandler(false), "GET", path, nil, headers); response.Code != 404 {
		t.Fatalf("disabled route returned %d", response.Code)
	}
	handler := makeHandler(true)
	if response := request(handler, "GET", path, nil, nil); response.Code != 401 {
		t.Fatalf("unauthenticated request returned %d", response.Code)
	}
	response := request(handler, "GET", path, nil, headers)
	other := request(handler, "GET", path, nil, map[string]string{"Authorization": "Bearer " + tokens[1]})
	if response.Code != 200 || other.Code != 200 || response.Body.String() != other.Body.String() {
		t.Fatalf("expected the same static catalog for both projects, got %d and %d", response.Code, other.Code)
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("authenticated response must not be cached")
	}
	if response := request(handler, "POST", path, []byte("{}"), headers); response.Code < 400 {
		t.Fatal("compatibility catalog accepted a write")
	}
}
