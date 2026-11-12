package httpapi

import (
	"net/http"
	"sathonate.local/fabric-control-plane/internal/identity"
	"sathonate.local/fabric-control-plane/internal/operations"
	"strconv"
)

func (a *API) runLocal(w http.ResponseWriter, r *http.Request, kind, retryOf string) {
	if !a.localAllowed(w) {
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if !operations.ValidKey(key) {
		problem(w, 400, "idempotency_key", "Use an 8–128 character idempotency key with letters, digits, underscores or hyphens", nil)
		return
	}
	actor := identity.FromContext(r.Context()).Subject
	if actor == "" {
		actor = "local-developer"
	}
	op, e := a.options.Local.Run(kind, key, retryOf, actor)
	if e != nil {
		problem(w, 409, "local_operation", e.Error(), nil)
		return
	}
	w.Header().Set("Location", "/api/v1/operations/"+op.ID)
	respond(w, 202, op)
}

func (a *API) localOperations(w http.ResponseWriter, r *http.Request) {
	if a.deploymentForRead(r) == nil && a.options.Auth.Mode != "local-development" {
		problem(w, 404, "unavailable", "Deployment runner is unavailable", nil)
		return
	}
	respond(w, 200, map[string]any{"items": a.allOperations(r)})
}
func (a *API) localOperation(w http.ResponseWriter, r *http.Request) {
	op, e := a.findOperation(r, r.PathValue("operationId"))
	if e != nil {
		problem(w, 404, "not_found", "Operation not found", nil)
		return
	}
	respond(w, 200, op)
}
func (a *API) localOperationEvents(w http.ResponseWriter, r *http.Request) {
	op, e := a.findOperation(r, r.PathValue("operationId"))
	if e != nil {
		problem(w, 404, "not_found", "Operation not found", nil)
		return
	}
	after := 0
	if v := r.URL.Query().Get("after"); v != "" {
		after, e = strconv.Atoi(v)
		if e != nil || after < 0 {
			problem(w, 400, "cursor", "Invalid event cursor", nil)
			return
		}
	}
	events := []operations.Event{}
	for _, event := range op.Events {
		if event.Sequence > after {
			events = append(events, event)
		}
	}
	respond(w, 200, map[string]any{"items": events, "nextCursor": len(op.Events), "status": op.Status})
}
func (a *API) localRetry(w http.ResponseWriter, r *http.Request) {
	if a.options.Deployments != nil {
		if previous, err := a.options.Deployments.Operations().Get(tenant(r), r.PathValue("operationId")); err == nil {
			if a.options.Customer != nil {
				snapshot, err := a.options.Deployments.Status(tenant(r), previous.ID)
				if err != nil {
					problem(w, 503, "retry_unavailable", "Deployment retry unavailable", nil)
					return
				}
				// Restoring an existing workload remains independent of billing state.
				if snapshot.Recovery == nil && !a.customerDeploymentAllowed(w, r) {
					return
				}
			}
			key := r.Header.Get("Idempotency-Key")
			if key == "" || !operations.ValidKey(key) {
				problem(w, 400, "idempotency_key", "Provide an idempotency key", nil)
				return
			}
			op, err := a.options.Deployments.Retry(tenant(r), actor(r), previous.ID, key)
			if err != nil {
				problem(w, 409, "deployment", err.Error(), nil)
				return
			}
			respond(w, 202, op)
			return
		}
	}
	if !a.localAllowed(w) {
		return
	}
	op, e := a.options.Local.Operations().Get(tenant(r), r.PathValue("operationId"))
	if e != nil {
		problem(w, 404, "not_found", "Operation not found", nil)
		return
	}
	a.runLocal(w, r, op.Kind, op.ID)
}

func (a *API) localTest(w http.ResponseWriter, r *http.Request) {
	a.runLocal(w, r, "verify", "")
}

func (a *API) localAllowed(w http.ResponseWriter) bool {
	if a.options.Auth.Mode != "local-development" || a.options.Local == nil {
		problem(w, 404, "unavailable", "Local Docker runtime is available only in loopback development mode", nil)
		return false
	}
	return true
}
func (a *API) localStatus(w http.ResponseWriter, r *http.Request) {
	if !a.localAllowed(w) {
		return
	}
	respond(w, 200, a.options.Local.Snapshot(r.Context()))
}
func (a *API) localHealth(w http.ResponseWriter, r *http.Request) {
	if !a.localAllowed(w) {
		return
	}
	respond(w, 200, a.options.Local.Health(r.Context()))
}
func (a *API) localStart(w http.ResponseWriter, r *http.Request) {
	a.runLocal(w, r, "start", "")
}
func (a *API) localStop(w http.ResponseWriter, r *http.Request) {
	a.runLocal(w, r, "stop", "")
}
func (a *API) localLogs(w http.ResponseWriter, r *http.Request) {
	if !a.localAllowed(w) {
		return
	}
	logs, e := a.options.Local.Logs(r.Context(), r.PathValue("node"))
	if e != nil {
		problem(w, 422, "logs", "Could not read logs for that Fabric node", nil)
		return
	}
	respond(w, 200, map[string]string{"logs": logs})
}
