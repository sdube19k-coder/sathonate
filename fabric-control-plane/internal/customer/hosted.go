package customer

import (
	"context"
	"net/url"
	"strings"
)

type HostedRequest struct {
	ProviderCustomer string
	Plan             string
	IdempotencyKey   string
}

type HostedProvider interface {
	// CreateSession sends only an opaque provider reference and catalog plan.
	// Implementations must use the idempotency key for provider-side retry.
	CreateSession(context.Context, HostedRequest) (string, error)
}

// HostedSession is a provider adapter boundary for checkout or account
// management. Production HTTP routing remains disabled.
func (s Service) HostedSession(ctx context.Context, actor Actor, key, plan string, provider HostedProvider, approvedOrigin string) (string, error) {
	if provider == nil {
		return "", ErrUnavailable
	}
	if !actor.valid() {
		return "", ErrDenied
	}
	if !idPattern.MatchString(key) || len(key) < 16 {
		return "", ErrInvalid
	}
	if plan != "" {
		selected, ok := planFor(plan)
		if !ok || selected.MonthlyMinor <= 0 {
			return "", ErrInvalid
		}
	}
	origin, err := url.Parse(approvedOrigin)
	if err != nil || origin.Scheme != "https" || origin.Hostname() == "" || origin.User != nil || origin.Path != "" ||
		origin.RawQuery != "" || origin.Fragment != "" {
		return "", ErrInvalid
	}
	account, err := s.Store.Read(ctx, actor.Tenant)
	if err != nil {
		return "", err
	}
	if account.Owner != actor.identity() || !idPattern.MatchString(account.ProviderRef) {
		return "", ErrDenied
	}
	request := HostedRequest{account.ProviderRef, plan, digest([]string{actor.Tenant, actor.Issuer, actor.Subject, key})}
	change := Change{ID: "hosted:" + request.IdempotencyKey, Action: "billing.hosted-session", Digest: digest(request),
		ActorSHA256: digest(actor.identity()), At: s.now()}
	if err := s.Store.Update(ctx, actor.Tenant, change, func(current *Account) error {
		if current.Owner != actor.identity() || current.ProviderRef != request.ProviderCustomer {
			return ErrDenied
		}
		return nil
	}); err != nil {
		return "", err
	}
	location, err := provider.CreateSession(ctx, request)
	if err != nil || len(location) > 4096 {
		return "", ErrUnavailable
	}
	target, err := url.Parse(location)
	if err != nil || target.Scheme != origin.Scheme || !strings.EqualFold(target.Host, origin.Host) ||
		target.User != nil || target.Opaque != "" || target.Path == "" || target.Fragment != "" {
		return "", ErrUnavailable
	}
	return target.String(), nil
}
