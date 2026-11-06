package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"sathonate.local/fabric-control-plane/internal/identity"
)

func (a *API) dynamicNetwork(w http.ResponseWriter, r *http.Request) {
	if !identity.IsProjectAuthenticated(r.Context()) || a.options.Deployments == nil || a.options.Deployments.Mode() != "secure-local" || (a.options.Auth.Mode != "local-token" && a.options.Auth.Mode != "okta") {
		problem(w, 403, "dynamic_unavailable", "Authenticated explicit local dynamic execution required", nil)
		return
	}
	d, e := a.repository(r).Get(r.PathValue("id"))
	if e != nil {
		storeError(w, e)
		return
	}
	rev, ok := revision(w, r)
	if !ok {
		return
	}
	if rev != d.Revision {
		problem(w, 409, "revision_conflict", "Reload the latest saved desired configuration", nil)
		return
	}
	parent := r.URL.Query().Get("parent")
	var input struct {
		Parent   string `json:"parentOperationId"`
		Approval string `json:"approval"`
	}
	if r.Method == "POST" {
		r.Body = http.MaxBytesReader(w, r.Body, 2048)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		var extra any
		if decoder.Decode(&input) != nil || decoder.Decode(&extra) != io.EOF {
			problem(w, 400, "request", "Use one approved dynamic-change request", nil)
			return
		}
		parent = input.Parent
	}
	op, e := a.options.Deployments.Operations().Get(tenant(r), parent)
	if e != nil || op.NetworkID != d.ID {
		problem(w, 404, "not_found", "Deployment not found", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 35*time.Second)
	defer cancel()
	if r.Method == "GET" {
		preview, e := a.options.Deployments.PreviewDynamic(ctx, tenant(r), actor(r), parent, d.ID, d.Desired, d.Revision)
		if e != nil {
			problem(w, 409, "dynamic_preflight", e.Error(), nil)
			return
		}
		respond(w, 200, preview)
		return
	}
	change, e := a.options.Deployments.ApplyDynamic(ctx, tenant(r), actor(r), parent, d.ID, d.Desired, d.Revision, input.Approval, r.Header.Get("Idempotency-Key"))
	if e != nil {
		problem(w, 409, "dynamic_change", e.Error(), nil)
		return
	}
	w.Header().Set("Location", "/api/v1/operations/"+change.ID)
	respond(w, 202, change)
}
