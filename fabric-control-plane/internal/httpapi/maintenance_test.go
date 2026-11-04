package httpapi

import (
	"net/http/httptest"
	"sathonate.local/fabric-control-plane/internal/identity"
	"strings"
	"testing"
)

func TestRotationPlanRoutesDoNotAuthorizeOperators(t *testing.T) {
	for _, path := range []string{"/api/v1/deployments/unknown/rotation-plans", "/api/v1/deployments/unknown/rotation-plans/unknown"} {
		method := "POST"
		if strings.HasSuffix(path, "/unknown") {
			method = "GET"
		}
		r := httptest.NewRequest(method, "http://localhost:8080"+path, strings.NewReader(`{"identityRef":"org1/peer1"}`))
		r.Header.Set("Content-Type", "application/json")
		r = r.WithContext(identity.WithPrincipal(r.Context(), identity.Principal{Subject: "person", TenantID: "tenant-a", Roles: []string{"operator"}, Scopes: []string{"fabric.read", "fabric.write"}}))
		w := httptest.NewRecorder()
		handler(t).ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal("operator reached maintenance planning", w.Code)
		}
	}
}
