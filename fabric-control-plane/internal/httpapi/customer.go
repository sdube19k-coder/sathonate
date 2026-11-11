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
		problem(w, 403, "customer_entitlement", "Active project entitlement required", nil)
		return false
	}
	err := a.options.Customer.AllowProject(r.Context(), actor)
	if err != nil {
		problem(w, customer.Status(err), "customer_entitlement", "Active project entitlement required", nil)
		return false
	}
	return true
}
