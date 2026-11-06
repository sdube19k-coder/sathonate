package httpapi

import (
	"context"
	"net/http"
	"time"

	"sathonate.local/fabric-control-plane/internal/identity"
)

func (a *API) deploymentWorkloads(w http.ResponseWriter, r *http.Request) {
	if !identity.IsProjectAuthenticated(r.Context()) {
		problem(w, 403, "project_grant_required", "Authenticated project grant required", nil)
		return
	}
	if _, e := a.findOperation(r, r.PathValue("operationId")); e != nil {
		problem(w, 404, "not_found", "Operation not found", nil)
		return
	}
	if a.options.Deployments == nil || a.options.Deployments.Mode() != "secure-local" {
		problem(w, 409, "workloads_unavailable", "Current workload inspection requires an approved local target", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	result, e := a.options.Deployments.Workloads(ctx, tenant(r), r.PathValue("operationId"))
	if e != nil {
		problem(w, 503, "workloads_unavailable", "Could not read current workloads from this approved local target. No changes were made.", nil)
		return
	}
	respond(w, 200, result)
}
