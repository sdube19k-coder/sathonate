package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sathonate.local/fabric-control-plane/internal/identity"
	"sathonate.local/fabric-control-plane/internal/operations"
	"time"
)

func (a *API) rotationPlan(w http.ResponseWriter, r *http.Request) {
	p := identity.FromContext(r.Context())
	if !p.CanAdminister() {
		problem(w, 403, "administrator_required", "Administrator scope and role required", nil)
		return
	}
	if a.options.Deployments == nil {
		problem(w, 404, "unavailable", "Deployment runner unavailable", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	if r.Method == http.MethodGet {
		result, err := a.options.Deployments.RotationPlan(ctx, p, r.PathValue("operationId"), r.PathValue("planId"))
		if err != nil {
			problem(w, 404, "not_found", "Maintenance plan unavailable", nil)
			return
		}
		respond(w, 200, result)
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if !operations.ValidKey(key) {
		problem(w, 400, "idempotency_key", "Provide a valid maintenance idempotency key", nil)
		return
	}
	var input struct {
		IdentityRef string `json:"identityRef"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var extra any
	if decoder.Decode(&input) != nil || decoder.Decode(&extra) != io.EOF {
		problem(w, 400, "request", "Use one identity reference object; credential material is forbidden", nil)
		return
	}
	result, err := a.options.Deployments.PlanRotation(ctx, p, r.PathValue("operationId"), input.IdentityRef, key)
	if err != nil {
		problem(w, 409, "maintenance", "Cannot create the bound maintenance plan; no identity was changed", nil)
		return
	}
	respond(w, 201, result)
}
