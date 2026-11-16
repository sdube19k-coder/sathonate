package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"time"

	"sathonate.local/fabric-control-plane/internal/domain"
	"sathonate.local/fabric-control-plane/internal/identity"
	"sathonate.local/fabric-control-plane/internal/operations"
)

func (a *API) deploymentAuthorization(w http.ResponseWriter, r *http.Request) {
	principal := identity.FromContext(r.Context())
	if a.options.Auth.Mode != "okta" || !identity.IsProjectAuthenticated(r.Context()) || principal.ProjectID == "" || !principal.CanDeploy() {
		problem(w, 403, "deploy_permission_required", "Okta write scope and project deployment permission are required", nil)
		return
	}
	rev, ok := revision(w, r)
	if !ok {
		return
	}
	d, err := a.repository(r).Get(r.PathValue("id"))
	if err != nil {
		storeError(w, err)
		return
	}
	if d.Archived || d.Revision != rev || d.Desired.Spec.Deployment.TargetRef != domain.SecureEKSProfile {
		problem(w, 409, "deployment_design", "The saved EKS design is unavailable or its revision changed", nil)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	respond(w, 200, map[string]any{"authorized": true, "revision": d.Revision})
}

func (a *API) deployNetwork(w http.ResponseWriter, r *http.Request) {
	if a.options.Deployments == nil {
		problem(w, 503, "execution_disabled", "Deployment execution is disabled. Configure the documented local Kubernetes runner; AWS/EKS execution remains disabled.", nil)
		return
	}
	rev, ok := revision(w, r)
	if !ok {
		return
	}
	d, err := a.repository(r).Get(r.PathValue("id"))
	if err != nil {
		storeError(w, err)
		return
	}
	if d.Revision != rev {
		problem(w, 409, "revision_conflict", "Save and reload the latest network design", nil)
		return
	}
	if (d.Desired.Spec.Deployment.TargetRef == domain.SecureLocalProfile || d.Desired.Spec.Deployment.TargetRef == domain.SecureEKSProfile || d.Desired.Spec.Deployment.TargetRef == domain.DynamicLocalProfile) && (!identity.IsProjectAuthenticated(r.Context()) || identity.FromContext(r.Context()).ProjectID == "" || (a.options.Auth.Mode != "local-token" && a.options.Auth.Mode != "okta")) {
		problem(w, 403, "secure_auth_required", "Secure local deployments require authenticated server-bound tenant/project grants", nil)
		return
	}
	var input struct {
		FailureStage string `json:"failureStage,omitempty"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&input); err != nil {
		problem(w, 400, "request", "Use a deployment options object", nil)
		return
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		problem(w, 400, "request", "Use one JSON object", nil)
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if key == "" || !operations.ValidKey(key) {
		problem(w, 400, "idempotency_key", "Provide an 8–128 character idempotency key", nil)
		return
	}
	op, err := a.options.Deployments.Start(tenant(r), actor(r), d.ID, d.Desired, d.Revision, key, input.FailureStage, "")
	if err != nil {
		problem(w, 409, "deployment", err.Error(), nil)
		return
	}
	w.Header().Set("Location", "/api/v1/operations/"+op.ID)
	respond(w, 202, op)
}

func (a *API) deploymentHealth(w http.ResponseWriter, r *http.Request) {
	manager := a.deploymentForRead(r)
	if manager == nil {
		problem(w, 404, "unavailable", "Local Kubernetes runner is disabled", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	health, err := manager.Health(ctx, tenant(r), r.PathValue("operationId"))
	if err != nil {
		problem(w, 422, "health", "Could not verify local Kubernetes health", nil)
		return
	}
	respond(w, 200, health)
}
func (a *API) deploymentLogs(w http.ResponseWriter, r *http.Request) {
	manager := a.deploymentForRead(r)
	if manager == nil {
		problem(w, 404, "unavailable", "Local Kubernetes runner is disabled", nil)
		return
	}
	logs, err := manager.Logs(r.Context(), tenant(r), r.PathValue("operationId"), r.PathValue("node"))
	if err != nil {
		problem(w, 422, "logs", "Could not read this network's Fabric node logs", nil)
		return
	}
	respond(w, 200, map[string]string{"logs": logs})
}
func (a *API) deploymentCleanup(w http.ResponseWriter, r *http.Request) {
	if !identity.FromContext(r.Context()).CanAdminister() {
		problem(w, 403, "administrator_required", "Administrator scope and role required for destructive operations", nil)
		return
	}
	if a.options.Deployments == nil {
		problem(w, 404, "unavailable", "Local Kubernetes runner is disabled", nil)
		return
	}
	timeout := 2 * time.Minute
	if a.options.Deployments.Mode() == "secure-local" {
		timeout = 5 * time.Minute
		if err := cleanupResponseDeadline(w, time.Now().Add(timeout+10*time.Second)); err != nil {
			problem(w, 503, "cleanup_transport", "Bounded cleanup response deadline unavailable; no deletion started", nil)
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	var err error
	if a.options.Deployments.Mode() == "aws-eks" || a.options.Deployments.Mode() == "secure-eks" || a.options.Deployments.Mode() == "secure-local" {
		err = a.options.Deployments.CleanupApproved(ctx, tenant(r), r.PathValue("operationId"), actor(r), r.Header.Get("X-Sathonate-Cleanup-Approval"))
	} else {
		err = a.options.Deployments.Cleanup(ctx, tenant(r), r.PathValue("operationId"))
	}
	if err != nil {
		problem(w, 409, "cleanup", err.Error(), nil)
		return
	}
	respond(w, 200, map[string]bool{"removed": true})
}
func actor(r *http.Request) string {
	if p := identity.FromContext(r.Context()); p.Subject != "" {
		return p.Subject
	}
	return "local-developer"
}
func (a *API) deploymentStatus(w http.ResponseWriter, r *http.Request) {
	manager := a.deploymentForRead(r)
	if manager == nil {
		problem(w, 404, "unavailable", "Deployment runner is disabled", nil)
		return
	}
	result, err := manager.Status(tenant(r), r.PathValue("operationId"))
	if err != nil {
		problem(w, 404, "not_found", "Deployment not found", nil)
		return
	}
	respond(w, 200, result)
}

func (a *API) deploymentCancel(w http.ResponseWriter, r *http.Request) {
	if a.options.Deployments == nil {
		problem(w, 404, "unavailable", "Deployment runner is disabled", nil)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var options *struct{}
	if err := decoder.Decode(&options); err != nil || options == nil {
		problem(w, 400, "request", "Use an empty cancellation options object", nil)
		return
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		problem(w, 400, "request", "Use one cancellation options object", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	if err := a.options.Deployments.Cancel(ctx, tenant(r), r.PathValue("operationId")); err != nil {
		problem(w, 409, "cancel", err.Error(), nil)
		return
	}
	respond(w, 200, map[string]bool{"cancelled": true})
}
func (a *API) findOperation(r *http.Request, id string) (operations.Operation, error) {
	manager := a.deploymentForRead(r)
	if manager != nil {
		if op, err := manager.Operations().Get(tenant(r), id); err == nil {
			return op, nil
		}
	}
	if a.options.Local != nil && a.options.Auth.Mode == "local-development" {
		return a.options.Local.Operations().Get(tenant(r), id)
	}
	return operations.Operation{}, operations.ErrNotFound
}
func (a *API) allOperations(r *http.Request) []operations.Operation {
	manager := a.deploymentForRead(r)
	items := []operations.Operation{}
	if manager != nil {
		items = append(items, manager.Operations().List(tenant(r))...)
	}
	if a.options.Local != nil && a.options.Auth.Mode == "local-development" {
		items = append(items, a.options.Local.Operations().List(tenant(r))...)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return items
}

func (a *API) deploymentCleanupPreview(w http.ResponseWriter, r *http.Request) {
	if !identity.FromContext(r.Context()).CanAdminister() {
		problem(w, 403, "administrator_required", "Administrator scope and role required for cleanup preview", nil)
		return
	}
	if a.options.Deployments == nil {
		problem(w, 404, "unavailable", "Deployment runner unavailable", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	plan, err := a.options.Deployments.CleanupPreview(ctx, tenant(r), r.PathValue("operationId"))
	if err != nil {
		problem(w, 409, "cleanup_preview", "Cannot establish an approved cleanup inventory", nil)
		return
	}
	respond(w, 200, plan)
}

// Keep the response alive for the already bounded, authenticated cleanup only.
// Ordinary requests retain the server's shorter write deadline. Recorders do
// not implement deadlines; the native server and unwrapped writers do.
func cleanupResponseDeadline(w http.ResponseWriter, deadline time.Time) error {
	err := http.NewResponseController(w).SetWriteDeadline(deadline)
	if errors.Is(err, http.ErrNotSupported) {
		return nil
	}
	return err
}
