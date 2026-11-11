package httpapi

import (
	"context"
	"net/http"
	"time"

	"sathonate.local/fabric-control-plane/internal/audit"
	"sathonate.local/fabric-control-plane/internal/cloud"
	"sathonate.local/fabric-control-plane/internal/customer"
	"sathonate.local/fabric-control-plane/internal/deployment"
	"sathonate.local/fabric-control-plane/internal/identity"
	"sathonate.local/fabric-control-plane/internal/localfabric"
	"sathonate.local/fabric-control-plane/internal/readiness"
)

type Options struct {
	Customer    *customer.Service
	AuditRecent func(context.Context, string) ([]audit.Event, error)
	ReadHistory func(context.Context, string) (*deployment.Manager, error)
	Auth        identity.Config
	Registry    cloud.Registry
	Commander   cloud.Commander
	Storage     string
	Local       *localfabric.Manager
	Deployments *deployment.Manager
	Readiness   readiness.Config
}

func (a *API) deploymentPackage(w http.ResponseWriter, r *http.Request) {
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
		problem(w, 409, "revision_conflict", "Save and reload the current design before preparing deployment", nil)
		return
	}
	p, err := cloud.Prepare(d.Desired, d.Revision, tenant(r), a.options.Registry)
	if err != nil {
		problem(w, 422, "deployment_package", err.Error(), nil)
		return
	}
	respond(w, 200, p)
}

func (a *API) cloudTargets(w http.ResponseWriter, r *http.Request) {
	items := a.options.Registry.Summaries(tenant(r))
	if a.options.Deployments != nil && a.options.Deployments.Mode() == "local-kubernetes" {
		items = append(items, cloud.Summary{ID: "smp-qualification", Name: "Local kind — real Kubernetes / Bevel", Region: "this-computer", ClusterName: "smp-qualification", Provider: "local", ExecutionEnabled: true})
	}
	respond(w, 200, map[string]any{"items": items, "executionEnabled": false})
}
func tenant(r *http.Request) string {
	p := identity.FromContext(r.Context())
	if p.TenantID == "" {
		return "local-development"
	}
	return p.StorageTenant()
}
func (a *API) cloudPreflight(w http.ResponseWriter, r *http.Request) {
	rev, ok := revision(w, r)
	if !ok {
		return
	}
	d, e := a.repository(r).Get(r.PathValue("id"))
	if e != nil {
		storeError(w, e)
		return
	}
	if d.Revision != rev {
		problem(w, 409, "revision_conflict", "Reload the current draft before checking cloud readiness", nil)
		return
	}
	target, ok := a.options.Registry.Find(tenant(r), d.Desired.Spec.Deployment.TargetRef)
	if !ok {
		respond(w, 200, cloud.Report{TargetID: d.Desired.Spec.Deployment.TargetRef, CheckedAt: time.Now().UTC(), Ready: false, Checks: []cloud.Check{{ID: "registered-target", Status: "blocked", Message: "This target is not registered for your authenticated organization. Configure the existing AWS EKS cluster on the server."}}})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()
	commander := a.options.Commander
	if commander == nil {
		commander = cloud.OSCommander{}
	}
	if target.Operator != nil {
		report := cloud.Report{TargetID: target.ID, CheckedAt: time.Now().UTC(), Checks: []cloud.Check{}}
		_, err := cloud.CompileOperator(d.Desired, target, deployment.Namespace(tenant(r), d.ID))
		if err == nil {
			err = cloud.OperatorPreflight(ctx, target, commander)
		}
		if err != nil {
			report.Checks = append(report.Checks, cloud.Check{ID: "operator-preflight", Status: "blocked", Message: err.Error()})
		} else {
			report.Checks = append(report.Checks, cloud.Check{ID: "operator-preflight", Status: "passed", Message: "Read-only EKS operator checks passed; qualification approval is checked separately when submitting a job."})
		}
		report.Checks = append(report.Checks, cloud.Check{ID: "production-release", Status: "blocked", Message: "Production cloud execution awaits actual EKS and enterprise acceptance."})
		respond(w, 200, report)
		return
	}
	report := cloud.Preflight(ctx, d.Desired, target, commander)
	respond(w, 200, report)
}
func (a *API) cloudBundle(w http.ResponseWriter, r *http.Request) {
	rev, ok := revision(w, r)
	if !ok {
		return
	}
	d, e := a.repository(r).Get(r.PathValue("id"))
	if e != nil {
		storeError(w, e)
		return
	}
	if d.Revision != rev {
		problem(w, 409, "revision_conflict", "The draft has changed", nil)
		return
	}
	target, ok := a.options.Registry.Find(tenant(r), d.Desired.Spec.Deployment.TargetRef)
	if !ok {
		problem(w, 422, "target_unregistered", "Register an EKS target for this organization first", nil)
		return
	}
	if target.Operator != nil {
		plan, err := cloud.CompileOperator(d.Desired, target, deployment.Namespace(tenant(r), d.ID))
		if err != nil {
			problem(w, 422, "operator_contract", err.Error(), nil)
			return
		}
		respond(w, 200, map[string]any{"executable": false, "operatorPlan": plan, "notice": "Use the approved EKS qualification job endpoint; legacy Ansible/Flux artifacts are not used."})
		return
	}
	bundle, e := cloud.Compile(d.Desired, target)
	if e != nil {
		problem(w, 422, "cloud_contract", e.Error(), nil)
		return
	}
	manifests, e := cloud.Manifests(bundle, target)
	if e != nil {
		problem(w, 500, "compile", "Could not compile staging resources", nil)
		return
	}
	respond(w, 200, map[string]any{"executable": false, "bundle": bundle, "kubernetesManifests": manifests, "notice": "Candidate artifacts only. No Kubernetes resources were created; production release and scheduling gates remain unsatisfied."})
}
