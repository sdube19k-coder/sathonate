package httpapi

import (
	"net/http"
	"time"

	"sathonate.local/fabric-control-plane/internal/identity"
)

// Diagnostics deliberately uses an allowlist, rather than serializing a runtime
// result, desired design, event payload, Kubernetes object or credential store.
// It describes recorded execution, never current infrastructure health.
func (a *API) operationDiagnostics(w http.ResponseWriter, r *http.Request) {
	if !identity.IsProjectAuthenticated(r.Context()) {
		problem(w, 403, "project_grant_required", "Authenticated project grant required", nil)
		return
	}
	op, err := a.findOperation(r, r.PathValue("operationId"))
	if err != nil {
		problem(w, 404, "not_found", "Operation not found", nil)
		return
	}
	type stage struct {
		Status     string     `json:"status"`
		StartedAt  *time.Time `json:"startedAt,omitempty"`
		FinishedAt *time.Time `json:"finishedAt,omitempty"`
	}
	stages := make([]stage, 0, len(op.Stages))
	for _, s := range op.Stages {
		stages = append(stages, stage{s.Status, s.StartedAt, s.FinishedAt})
	}
	respond(w, 200, map[string]any{
		"schema":      "sathonate.operation-diagnostics/v1",
		"generatedAt": time.Now().UTC(), "evidenceType": "recorded-operation",
		"liveHealthChecked": false, "operationId": op.ID, "networkId": op.NetworkID,
		"status": op.Status, "createdAt": op.CreatedAt, "updatedAt": op.UpdatedAt,
		"finishedAt": op.FinishedAt, "retryOf": op.RetryOf,
		"cancelRequested": op.CancelRequested, "stages": stages,
		"eventCount": len(op.Events),
		"nextStep":   diagnosticNextStep(op.Status),
	})
}

func diagnosticNextStep(status string) string {
	switch status {
	case "running", "pending":
		return "Refresh operation status. Do not start a duplicate deployment."
	case "failed", "interrupted":
		return "Review the authenticated operation events and node logs. An authorized retry reconciles existing resources."
	case "succeeded":
		return "Refresh network health to check current workloads; this report records a completed operation."
	default:
		return "Review operation history. Cancellation does not remove workloads or persistent data."
	}
}
