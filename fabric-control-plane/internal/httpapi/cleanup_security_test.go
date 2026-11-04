package httpapi

import (
	"net/http"
	"net/http/httptest"
	"sathonate.local/fabric-control-plane/internal/identity"
	"testing"
)

func TestDestructiveRoutesRequireAdministrator(t *testing.T) {
	for _, path := range []string{"/api/v1/deployments/unknown", "/api/v1/deployments/unknown/cleanup-preview"} {
		for _, roles := range [][]string{nil, {"viewer"}, {"operator"}} {
			method := http.MethodDelete
			if path[len(path)-7:] == "preview" {
				method = http.MethodGet
			}
			request := httptest.NewRequest(method, "http://localhost:8080"+path, nil)
			request = request.WithContext(identity.WithPrincipal(request.Context(), identity.Principal{Subject: "person", TenantID: "tenant-a", Roles: roles, Scopes: []string{"fabric.read", "fabric.write"}}))
			response := httptest.NewRecorder()
			handler(t).ServeHTTP(response, request)
			if response.Code != 403 {
				t.Fatal("destructive route did not reject non-admin", response.Code)
			}
		}
	}
}
