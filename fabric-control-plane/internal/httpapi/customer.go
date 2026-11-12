package httpapi

import (
	"net/http"

	"sathonate.local/fabric-control-plane/internal/customer"
	"sathonate.local/fabric-control-plane/internal/identity"
)

func (a *API) customerActor(r *http.Request) (customer.Actor, bool) {
	p := identity.FromContext(r.Context())
	return customer.Actor{Issuer: a.options.Auth.Issuer, Subject: p.Subject, Tenant: p.TenantID, Project: p.ProjectID},
		a.options.Customer != nil && a.options.Auth.Mode == "okta" && !identity.IsLocalAuthenticated(r.Context()) && identity.IsProjectAuthenticated(r.Context())
}

func (a *API) customerAccount(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.customerActor(r)
	if !ok {
		problem(w, 404, "unavailable", "Customer pilot unavailable", nil)
		return
	}
	view, err := a.options.Customer.View(r.Context(), actor)
	if err != nil {
		problem(w, customer.Status(err), "customer_account", "Customer account unavailable", nil)
		return
	}
	respond(w, 200, map[string]any{"account": view, "canManage": view.IsOwner && identity.FromContext(r.Context()).CanAdminister(), "plans": customer.Plans(), "paymentsEnabled": false, "infrastructureEnabled": false})
}

func (a *API) customerAction(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.customerActor(r)
	if !ok {
		problem(w, 404, "unavailable", "Customer pilot unavailable", nil)
		return
	}
	command, err := customer.DecodeCommand(http.MaxBytesReader(w, r.Body, 4096))
	if err != nil {
		problem(w, 400, "customer_request", "Invalid customer operation", nil)
		return
	}
	if r.URL.Path == identity.CustomerInvitationAcceptance {
		if command.Action != "accept-invitation" {
			problem(w, 400, "customer_request", "Invitation acceptance required", nil)
			return
		}
	} else if command.Action == "accept-invitation" || !identity.FromContext(r.Context()).CanAdminister() {
		problem(w, 403, "customer_denied", "Customer administration denied", nil)
		return
	}
	keys := r.Header.Values("Idempotency-Key")
	if len(keys) != 1 {
		problem(w, 400, "customer_idempotency", "One idempotency key is required", nil)
		return
	}
	if err = a.options.Customer.Execute(r.Context(), actor, keys[0], command); err != nil {
		problem(w, customer.Status(err), "customer_action", "Customer operation could not be applied", nil)
		return
	}
	w.WriteHeader(204)
}

func (a *API) customerDeploymentAllowed(w http.ResponseWriter, r *http.Request) bool {
	if a.options.Customer == nil {
		return true
	}
	actor, ok := a.customerActor(r)
	if !ok {
		a.observeCustomerDecision("denied")
		problem(w, 403, "customer_entitlement", "Active project entitlement required", nil)
		return false
	}
	err := a.options.Customer.AllowProject(r.Context(), actor)
	if err != nil {
		outcome := "denied"
		if customer.Status(err) == 503 {
			outcome = "unavailable"
		}
		a.observeCustomerDecision(outcome)
		problem(w, customer.Status(err), "customer_entitlement", "Active project entitlement required", nil)
		return false
	}
	a.observeCustomerDecision("allowed")
	return true
}

func (a *API) observeCustomerDecision(outcome string) {
	if a.options.CustomerDecision != nil {
		a.options.CustomerDecision(outcome)
	}
}

func (a *API) customerProvisionPreview(w http.ResponseWriter, r *http.Request) {
	actor, ok := a.customerActor(r)
	if !ok || len(a.options.CustomerTargets) == 0 {
		problem(w, 404, "unavailable", "Customer target preview unavailable", nil)
		return
	}
	revision, ok := revision(w, r)
	if !ok {
		return
	}
	draft, err := a.repository(r).Get(r.PathValue("id"))
	if err != nil {
		storeError(w, err)
		return
	}
	if draft.Revision != revision {
		problem(w, 409, "revision_conflict", "Save and reload the current design before preview", nil)
		return
	}
	preview, err := a.options.Customer.PreviewProvision(r.Context(), actor, a.options.CustomerTargets, draft.ID, draft.Revision, draft.Desired)
	if err != nil {
		problem(w, customer.Status(err), "customer_preflight", "Customer target preflight refused", nil)
		return
	}
	respond(w, 200, preview)
}
