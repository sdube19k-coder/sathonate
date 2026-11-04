package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"sathonate.local/fabric-control-plane/internal/deployment"
	"sathonate.local/fabric-control-plane/internal/domain"
	"sathonate.local/fabric-control-plane/internal/drafts"
	"sathonate.local/fabric-control-plane/internal/identity"
	"sathonate.local/fabric-control-plane/internal/planner"
	"sathonate.local/fabric-control-plane/internal/redact"
)

type API struct {
	repository     RepositoryResolver
	options        Options
	allowedHosts   map[string]bool
	allowedOrigins map[string]bool
}

func New(store *drafts.Store, hosts, origins []string) http.Handler {
	return NewConfigured(func(*http.Request) Repository { return fileRepository{store} }, hosts, origins, Options{Auth: identity.Config{Mode: "local-development"}, Storage: "single-process local drafts"})
}

func NewConfigured(resolve RepositoryResolver, hosts, origins []string, options Options) http.Handler {
	a := &API{repository: resolve, options: options, allowedHosts: map[string]bool{}, allowedOrigins: map[string]bool{}}
	for _, v := range hosts {
		a.allowedHosts[v] = true
	}
	for _, v := range origins {
		a.allowedOrigins[v] = true
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/local-runtime", a.localStatus)
	mux.HandleFunc("GET /api/v1/local-runtime/health", a.localHealth)
	mux.HandleFunc("GET /api/v1/operations", a.localOperations)
	mux.HandleFunc("GET /api/v1/operations/{operationId}", a.localOperation)
	mux.HandleFunc("GET /api/v1/operations/{operationId}/events", a.localOperationEvents)
	mux.HandleFunc("GET /api/v1/operations/{operationId}/diagnostics", a.operationDiagnostics)
	mux.HandleFunc("POST /api/v1/operations/{operationId}/retry", a.localRetry)
	mux.HandleFunc("POST /api/v1/local-runtime/start", a.localStart)
	mux.HandleFunc("POST /api/v1/local-runtime/stop", a.localStop)
	mux.HandleFunc("POST /api/v1/local-runtime/test", a.localTest)
	mux.HandleFunc("GET /api/v1/local-runtime/logs/{node}", a.localLogs)
	mux.HandleFunc("GET /api/v1/projects", func(w http.ResponseWriter, r *http.Request) {
		p := identity.FromContext(r.Context())
		if !identity.IsProjectAuthenticated(r.Context()) || p.ProjectID == "" {
			problem(w, 403, "project_grant_required", "Authenticated server-owned project grant required", nil)
			return
		}
		respond(w, 200, map[string]any{"items": []any{map[string]string{"id": p.ProjectID, "tenantId": p.TenantID, "developer": p.Subject}}})
	})
	mux.HandleFunc("GET /api/v1/auth/identity", func(w http.ResponseWriter, r *http.Request) {
		if !identity.IsVerifiedIdentity(r.Context()) || identity.FromContext(r.Context()).Subject == "" {
			problem(w, 404, "unavailable", "Local identity qualification is disabled", nil)
			return
		}
		respond(w, 200, map[string]string{"issuer": a.options.Auth.Issuer, "subject": identity.FromContext(r.Context()).Subject})
	})
	mux.HandleFunc("GET /api/v1/auth/config", func(w http.ResponseWriter, r *http.Request) { respond(w, 200, a.options.Auth) })
	mux.HandleFunc("GET /api/v1/cloud/targets", a.cloudTargets)
	mux.HandleFunc("GET /api/v1/readiness", a.launchReadiness)
	mux.HandleFunc("POST /api/v1/readiness/probe", a.launchReadiness)
	mux.HandleFunc("POST /api/v1/networks/{id}/cloud-preflight", a.cloudPreflight)
	mux.HandleFunc("POST /api/v1/networks/{id}/cloud-bundle", a.cloudBundle)
	mux.HandleFunc("POST /api/v1/networks/{id}/deployment-package", a.deploymentPackage)
	mux.HandleFunc("POST /api/v1/networks/{id}/deploy", a.deployNetwork)
	mux.HandleFunc("POST /api/v1/deployments/{operationId}/cancel", a.deploymentCancel)
	mux.HandleFunc("GET /api/v1/deployments/{operationId}/lifecycle/{action}/preview", a.lifecycle)
	mux.HandleFunc("POST /api/v1/deployments/{operationId}/lifecycle/{action}/deploy", a.lifecycle)
	mux.HandleFunc("GET /api/v1/deployments/{operationId}", a.deploymentStatus)
	mux.HandleFunc("GET /api/v1/deployments/{operationId}/health", a.deploymentHealth)
	mux.HandleFunc("GET /api/v1/deployments/{operationId}/logs/{node}", a.deploymentLogs)
	mux.HandleFunc("POST /api/v1/deployments/{operationId}/rotation-plans", a.rotationPlan)
	mux.HandleFunc("GET /api/v1/deployments/{operationId}/rotation-plans/{planId}", a.rotationPlan)
	mux.HandleFunc("GET /api/v1/deployments/{operationId}/cleanup-preview", a.deploymentCleanupPreview)
	mux.HandleFunc("DELETE /api/v1/deployments/{operationId}", a.deploymentCleanup)
	mux.HandleFunc("GET /api/v1/meta", func(w http.ResponseWriter, r *http.Request) {
		deploymentMode := "disabled"
		if a.options.ReadHistory != nil {
			deploymentMode = "history-only"
		}
		if a.options.Deployments != nil {
			deploymentMode = a.options.Deployments.Mode()
		}
		respond(w, 200, map[string]any{"name": "Fabric SMP", "company": "Sathonate", "mode": a.options.Auth.Mode, "deploymentMode": deploymentMode, "mockExecutionEnabled": deploymentMode == "mock", "executionEnabled": false, "localRuntimeEnabled": a.options.Local != nil && a.options.Auth.Mode == "local-development", "schemaVersion": domain.APIVersion, "storage": a.options.Storage, "engines": []string{"bevel", "native"}})
	})
	mux.HandleFunc("GET /api/v1/networks", func(w http.ResponseWriter, r *http.Request) {
		items, e := a.repository(r).List()
		if e != nil {
			storeError(w, e)
			return
		}
		respond(w, 200, map[string]any{"items": items})
	})
	mux.HandleFunc("POST /api/v1/networks", a.create)
	mux.HandleFunc("POST /api/v1/validate", a.validate)
	mux.HandleFunc("POST /api/v1/plans", a.plan)
	mux.HandleFunc("GET /api/v1/networks/{id}", a.get)
	mux.HandleFunc("PUT /api/v1/networks/{id}", a.update)
	mux.HandleFunc("DELETE /api/v1/networks/{id}", a.archive)
	mux.HandleFunc("POST /api/v1/networks/{id}/review", a.review)
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { problem(w, 404, "not_found", "API route not found", nil) })
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if !a.allowedHosts[r.Host] {
			problem(w, 403, "invalid_host", "Host is not allowed", nil)
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			if origin := r.Header.Get("Origin"); origin != "" && !a.allowedOrigins[origin] {
				problem(w, 403, "invalid_origin", "Origin is not allowed", nil)
				return
			}
			if r.Method != "DELETE" {
				kind, _, e := mime.ParseMediaType(r.Header.Get("Content-Type"))
				if e != nil || kind != "application/json" {
					problem(w, 415, "content_type", "Use application/json", nil)
					return
				}
			}
		}
		if a.options.ReadHistory != nil && (r.Method == "GET" || r.Method == "HEAD") && identity.IsProjectAuthenticated(r.Context()) && (strings.HasPrefix(r.URL.Path, "/api/v1/operations") || strings.HasPrefix(r.URL.Path, "/api/v1/deployments")) {
			history, err := a.options.ReadHistory(r.Context(), tenant(r))
			if err != nil {
				problem(w, 503, "history_unavailable", "Recorded project history is unavailable", nil)
				return
			}
			defer history.Close()
			r = r.WithContext(context.WithValue(r.Context(), historyContextKey{}, history))
		}
		mux.ServeHTTP(w, r)
	})
}
func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func problem(w http.ResponseWriter, status int, code, message string, issues []domain.Issue) {
	message = redact.Text(message)
	safeIssues := append([]domain.Issue(nil), issues...)
	for i := range safeIssues {
		safeIssues[i].Message = redact.Text(safeIssues[i].Message)
		safeIssues[i].Path = redact.Text(safeIssues[i].Path)
	}
	issues = safeIssues
	respond(w, status, map[string]any{"error": map[string]any{"code": code, "message": message, "issues": issues}})
}
func readSpec(w http.ResponseWriter, r *http.Request) (domain.NetworkSpec, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	s, e := domain.Decode(r.Body)
	if e != nil {
		problem(w, 400, "invalid_json", "Input must match the network schema and be at most 1 MiB", nil)
		return s, false
	}
	return s, true
}
func validated(w http.ResponseWriter, r *http.Request) (domain.NetworkSpec, bool) {
	s, ok := readSpec(w, r)
	if !ok {
		return s, false
	}
	issues := domain.Validate(s)
	if domain.HasErrors(issues) {
		problem(w, 422, "validation", "Some network settings need attention", issues)
		return s, false
	}
	return s, true
}
func revision(w http.ResponseWriter, r *http.Request) (int, bool) {
	v := strings.Trim(r.Header.Get("If-Match"), "\"")
	n, e := strconv.Atoi(v)
	if e != nil || n < 1 {
		problem(w, 428, "revision_required", "A current draft revision is required", nil)
		return 0, false
	}
	return n, true
}
func draftResponse(w http.ResponseWriter, status int, d drafts.Draft) {
	w.Header().Set("ETag", strconv.Quote(strconv.Itoa(d.Revision)))
	respond(w, status, d)
}
func storeError(w http.ResponseWriter, e error) {
	switch {
	case errors.Is(e, drafts.ErrNotFound):
		problem(w, 404, "not_found", e.Error(), nil)
	case errors.Is(e, drafts.ErrDuplicate):
		problem(w, 409, "duplicate_identifier", e.Error(), nil)
	case errors.Is(e, drafts.ErrIdentity):
		problem(w, 400, "immutable_identifier", e.Error(), nil)
	case errors.Is(e, drafts.ErrConflict):
		problem(w, 409, "revision_conflict", e.Error(), nil)
	case errors.Is(e, drafts.ErrInvalid):
		problem(w, 422, "validation", e.Error(), nil)
	default:
		problem(w, 500, "storage", "Could not save the draft. Please try again.", nil)
	}
}
func (a *API) create(w http.ResponseWriter, r *http.Request) {
	s, ok := validated(w, r)
	if !ok {
		return
	}
	d, e := a.repository(r).Save(s, "", 0)
	if e != nil {
		storeError(w, e)
		return
	}
	w.Header().Set("Location", "/api/v1/networks/"+d.ID)
	draftResponse(w, 201, d)
}
func (a *API) get(w http.ResponseWriter, r *http.Request) {
	d, e := a.repository(r).Get(r.PathValue("id"))
	if e != nil {
		storeError(w, e)
		return
	}
	draftResponse(w, 200, d)
}
func (a *API) update(w http.ResponseWriter, r *http.Request) {
	rev, ok := revision(w, r)
	if !ok {
		return
	}
	s, ok := validated(w, r)
	if !ok {
		return
	}
	d, e := a.repository(r).Save(s, r.PathValue("id"), rev)
	if e != nil {
		storeError(w, e)
		return
	}
	draftResponse(w, 200, d)
}
func (a *API) review(w http.ResponseWriter, r *http.Request) {
	rev, ok := revision(w, r)
	if !ok {
		return
	}
	d, e := a.repository(r).Review(r.PathValue("id"), rev)
	if e != nil {
		storeError(w, e)
		return
	}
	draftResponse(w, 200, d)
}
func (a *API) archive(w http.ResponseWriter, r *http.Request) {
	rev, ok := revision(w, r)
	if !ok {
		return
	}
	if e := a.repository(r).Archive(r.PathValue("id"), rev); e != nil {
		storeError(w, e)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (a *API) validate(w http.ResponseWriter, r *http.Request) {
	s, ok := readSpec(w, r)
	if !ok {
		return
	}
	issues := domain.Validate(s)
	result := map[string]any{"valid": !domain.HasErrors(issues), "issues": issues}
	if s.Spec.Deployment.TargetRef == domain.SecureLocalProfile {
		profile := deployment.SecureLocalProfile()
		if a.options.Deployments != nil && a.options.Deployments.Mode() == "secure-local" && (a.options.Auth.Mode == "local-token" || a.options.Auth.Mode == "okta") && identity.IsProjectAuthenticated(r.Context()) {
			profile.ExecutionEnabled = true
			profile.Blockers = []string{}
		}
		result["deploymentProfile"] = profile
	}
	respond(w, 200, result)
}
func (a *API) plan(w http.ResponseWriter, r *http.Request) {
	s, ok := validated(w, r)
	if !ok {
		return
	}
	p, e := planner.Create(s)
	if e != nil {
		problem(w, 422, "validation", "Could not generate a review plan", nil)
		return
	}
	respond(w, 200, p)
}

type historyContextKey struct{}

func (a *API) deploymentForRead(r *http.Request) *deployment.Manager {
	if a.options.Deployments != nil {
		return a.options.Deployments
	}
	m, _ := r.Context().Value(historyContextKey{}).(*deployment.Manager)
	return m
}
