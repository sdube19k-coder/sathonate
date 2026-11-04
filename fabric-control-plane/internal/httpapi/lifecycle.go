package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sathonate.local/fabric-control-plane/internal/identity"
	"time"
)

func (a *API) lifecycle(w http.ResponseWriter, r *http.Request) {
	if !identity.IsProjectAuthenticated(r.Context()) || a.options.Deployments == nil || a.options.Deployments.Mode() != "secure-local" {
		problem(w, 403, "lifecycle_unavailable", "Authenticated protected local lifecycle execution required", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 35*time.Second)
	defer cancel()
	parent, action := r.PathValue("operationId"), r.PathValue("action")
	// Resolve ownership first; foreign IDs must never reveal change capabilities.
	if _, e := a.options.Deployments.Operations().Get(tenant(r), parent); e != nil {
		problem(w, 404, "not_found", "Operation not found", nil)
		return
	}
	if r.Method == "GET" {
		preview, e := a.options.Deployments.PreviewLifecycle(ctx, tenant(r), actor(r), parent, action)
		if e != nil {
			problem(w, 409, "lifecycle_preflight", e.Error(), nil)
			return
		}
		respond(w, 200, preview)
		return
	}
	var input struct {
		Approval string `json:"approval"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2048)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	var extra any
	if d.Decode(&input) != nil || d.Decode(&extra) != io.EOF {
		problem(w, 400, "request", "Use one approved lifecycle request", nil)
		return
	}
	op, e := a.options.Deployments.ApplyLifecycle(ctx, tenant(r), actor(r), parent, action, input.Approval, r.Header.Get("Idempotency-Key"))
	if e != nil {
		problem(w, 409, "lifecycle", e.Error(), nil)
		return
	}
	w.Header().Set("Location", "/api/v1/operations/"+op.ID)
	respond(w, 202, op)
}
