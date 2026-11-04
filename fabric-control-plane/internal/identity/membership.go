package identity

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"time"
)

// Membership is server-owned authorization configuration, never token or request
// input. Revocation takes effect after a process restart; NotBefore invalidates
// older bearer tokens without treating legitimate token reuse as an attack.
type Membership struct {
	Environment string    `json:"environment"`
	Issuer      string    `json:"issuer"`
	Subject     string    `json:"subject"`
	Tenant      string    `json:"tenant"`
	Project     string    `json:"project"`
	Role        string    `json:"role"`
	Permissions []string  `json:"permissions"`
	NotBefore   time.Time `json:"notBefore"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

func loadMemberships(path string) ([]Membership, error) {
	denied := errors.New("valid server-owned OIDC memberships are required")
	f, err := os.Open(path)
	if err != nil {
		return nil, denied
	}
	defer f.Close()
	d := json.NewDecoder(io.LimitReader(f, 1<<20))
	d.DisallowUnknownFields()
	var grants []Membership
	if d.Decode(&grants) != nil {
		return nil, denied
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return nil, denied
	}
	return validateMemberships(grants)
}
func validateMemberships(grants []Membership) ([]Membership, error) {
	denied := errors.New("valid server-owned OIDC memberships are required")
	if len(grants) == 0 || len(grants) > 10000 {
		return nil, denied
	}
	seen := map[[3]string]bool{}
	for _, g := range grants {
		for _, s := range []string{g.Subject, g.Tenant, g.Project} {
			if s == "" || len(s) > 128 || strings.TrimSpace(s) != s || strings.ContainsAny(s, "\r\n\x00") {
				return nil, denied
			}
		}
		key := [3]string{g.Subject, g.Tenant, g.Project}
		if (g.Environment != "production" && g.Environment != "local-development") || g.Issuer == "" || seen[key] || !g.ExpiresAt.After(g.NotBefore) || g.NotBefore.IsZero() {
			return nil, denied
		}
		seen[key] = true
		if g.Role != "viewer" && g.Role != "operator" && g.Role != "admin" {
			return nil, denied
		}
		read := false
		for _, p := range g.Permissions {
			switch p {
			case "read":
				read = true
			case "write", "deploy", "retry", "cancel":
				if g.Role == "viewer" {
					return nil, denied
				}
			case "cleanup":
				if g.Role != "admin" {
					return nil, denied
				}
			default:
				return nil, denied
			}
		}
		if !read {
			return nil, denied
		}
	}
	return grants, nil
}

func (a *Authenticator) membership(subject, tenant, project string, issued time.Time) (Membership, error) {
	now := time.Now()
	for _, g := range a.memberships {
		if g.Environment == a.config.Environment && g.Issuer == a.config.Issuer && g.Subject == subject && g.Tenant == tenant && g.Project == project && !issued.Before(g.NotBefore) && now.Before(g.ExpiresAt) && !now.Before(g.NotBefore) {
			return g, nil
		}
	}
	return Membership{}, errors.New("no active server-owned membership")
}

// Server-bound mode selects one active scope from trusted policy. A token cannot
// select a different project, and multiple memberships fail closed.
func (a *Authenticator) boundMembership(subject string, issued time.Time) (Membership, error) {
	var selected Membership
	count := 0
	for _, g := range a.memberships {
		if g.Subject == subject {
			if active, err := a.membership(subject, g.Tenant, g.Project, issued); err == nil {
				selected = active
				count++
			}
		}
	}
	if count != 1 {
		return Membership{}, errors.New("one active server-bound project membership is required")
	}
	return selected, nil
}
func (a *Authenticator) AuthorizedStorageScopes() []string {
	scopes := []string{}
	seen := map[string]bool{}
	now := time.Now()
	for _, g := range a.memberships {
		if g.Environment != a.config.Environment || g.Issuer != a.config.Issuer || now.Before(g.NotBefore) || !now.Before(g.ExpiresAt) {
			continue
		}
		scope := (Principal{TenantID: g.Tenant, ProjectID: g.Project}).StorageTenant()
		if !seen[scope] {
			scopes = append(scopes, scope)
			seen[scope] = true
		}
	}
	return scopes
}

type MembershipSource interface {
	Lookup(context.Context, string, string, string) ([]Membership, error)
}
