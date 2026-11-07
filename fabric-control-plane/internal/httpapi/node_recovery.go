package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sathonate.local/fabric-control-plane/internal/identity"
	"time"
)

func (a *API) nodeRecovery(w http.ResponseWriter, r *http.Request) {
	if !identity.IsProjectAuthenticated(r.Context()) || a.options.Deployments == nil || a.options.Deployments.Mode() != "secure-local" {
		problem(w, 403, "recovery_unavailable", "Authenticated approved local recovery required", nil)
		return
	}
	parent := r.PathValue("operationId")
	if _, e := a.options.Deployments.Operations().Get(tenant(r), parent); e != nil {
		problem(w, 404, "not_found", "Operation not found", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 35*time.Second)
	defer cancel()
	if r.Method == "GET" {
		p, e := a.options.Deployments.PreviewNodeRecovery(ctx, tenant(r), actor(r), parent, r.URL.Query().Get("node"))
		if e != nil {
			problem(w, 409, "recovery_preflight", e.Error(), nil)
			return
		}
		respond(w, 200, p)
		return
	}
	var input struct {
		Node     string `json:"node"`
		Approval string `json:"approval"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2048)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	var extra any
	if d.Decode(&input) != nil || d.Decode(&extra) != io.EOF {
		problem(w, 400, "request", "Use one reviewed recovery request", nil)
		return
	}
	op, e := a.options.Deployments.ApplyNodeRecovery(ctx, tenant(r), actor(r), parent, input.Node, input.Approval, r.Header.Get("Idempotency-Key"))
	if e != nil {
		problem(w, 409, "recovery", e.Error(), nil)
		return
	}
	w.Header().Set("Location", "/api/v1/operations/"+op.ID)
	respond(w, 202, op)
}
