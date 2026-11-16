package httpapi

import (
	"net/http"

	"sathonate.local/fabric-control-plane/internal/compatibility"
	"sathonate.local/fabric-control-plane/internal/identity"
)

func compatibilityCatalog(w http.ResponseWriter, r *http.Request) {
	if !identity.IsProjectAuthenticated(r.Context()) || identity.FromContext(r.Context()).ProjectID == "" {
		problem(w, http.StatusForbidden, "project_grant_required", "Authenticated server-owned project grant required", nil)
		return
	}
	respond(w, http.StatusOK, compatibility.Snapshot())
}
