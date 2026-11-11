// Package identity authenticates API access tokens; ID tokens are never API credentials.
package identity

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

const ProductionACR = "urn:okta:loa:2fa:any"

type Config struct {
	RequiredACR        string           `json:"requiredACR,omitempty"`
	MembershipSource   MembershipSource `json:"-"`
	Mode               string           `json:"mode"`
	Issuer             string           `json:"issuer,omitempty"`
	ClientID           string           `json:"clientId,omitempty"`
	Audience           string           `json:"audience,omitempty"`
	TenantClaim        string           `json:"-"`
	ProjectClaim       string           `json:"-"`
	MembershipsFile    string           `json:"-"`
	Environment        string           `json:"-"`
	ServerBoundProject bool             `json:"-"`
	LocalIdentityProbe bool             `json:"-"`
}
type Principal struct {
	Subject     string   `json:"subject"`
	TenantID    string   `json:"tenantId"`
	ProjectID   string   `json:"projectId,omitempty"`
	Roles       []string `json:"roles"`
	Scopes      []string `json:"scopes"`
	permissions []string
}
type contextKey struct{}

func FromContext(ctx context.Context) Principal {
	p, _ := ctx.Value(contextKey{}).(Principal)
	return p
}
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, contextKey{}, p)
}
func (p Principal) CanWrite() bool {
	writeScope := false
	for _, scope := range p.Scopes {
		if scope == "fabric.write" {
			writeScope = true
		}
	}
	if !writeScope {
		return false
	}
	for _, role := range p.Roles {
		if role == "operator" || role == "admin" {
			return true
		}
	}
	return false
}

func (p Principal) CanAdminister() bool {
	if p.Subject == "" || p.TenantID == "" || !p.CanWrite() {
		return false
	}
	for _, role := range p.Roles {
		if role == "admin" {
			return true
		}
	}
	return false
}

type Verifier interface {
	Verify(context.Context, string) (*oidc.IDToken, error)
}
type Authenticator struct {
	config      Config
	verifier    Verifier
	memberships []Membership
}

func New(ctx context.Context, c Config) (*Authenticator, error) {
	if c.Mode == "local-development" {
		return &Authenticator{config: c}, nil
	}
	if c.Mode != "okta" {
		return nil, errors.New("authentication mode must be okta or local-development")
	}
	if c.Environment == "" {
		c.Environment = "production"
	}
	if c.Environment != "production" && c.Environment != "local-development" {
		return nil, errors.New("explicit supported OIDC environment required")
	}
	if c.Environment == "production" {
		if c.RequiredACR == "" {
			c.RequiredACR = ProductionACR
		}
		if c.RequiredACR != ProductionACR {
			return nil, errors.New("production requires the approved two-factor assurance policy")
		}
	} else if c.RequiredACR != "" && c.RequiredACR != ProductionACR {
		return nil, errors.New("unsupported authentication assurance policy")
	}
	parsed, e := url.Parse(c.Issuer)
	if e != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || !strings.Contains(parsed.Path, "/oauth2/") || c.ClientID == "" || c.Audience == "" || c.Audience == c.ClientID {
		return nil, errors.New("Okta requires a HTTPS custom authorization-server issuer, SPA client ID and a distinct API audience")
	}
	if c.TenantClaim == "" {
		c.TenantClaim = "sathonate_tenant"
	}
	if c.ProjectClaim == "" {
		c.ProjectClaim = "sathonate_project"
	}
	var grants []Membership
	if c.MembershipSource != nil {
		if c.MembershipsFile != "" {
			return nil, errors.New("select one membership authority")
		}
	} else {
		grants, e = loadMemberships(c.MembershipsFile)
		if e != nil {
			return nil, e
		}
		for _, g := range grants {
			if g.Issuer != c.Issuer || g.Environment != c.Environment {
				return nil, errors.New("membership issuer and environment must match the configured service")
			}
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	client := http.DefaultClient
	if configured, ok := ctx.Value(oauth2.HTTPClient).(*http.Client); ok {
		client = configured
	}
	bounded := *client
	bounded.Timeout = 5 * time.Second
	bounded.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	ctx = oidc.ClientContext(ctx, &bounded)
	provider, e := oidc.NewProvider(ctx, c.Issuer)
	if e != nil {
		return nil, errors.New("could not discover the configured Okta authorization server")
	}
	var discovery struct {
		JWKS string `json:"jwks_uri"`
	}
	if provider.Claims(&discovery) != nil {
		return nil, errors.New("invalid provider metadata")
	}
	jwks, err := url.Parse(discovery.JWKS)
	if err != nil || jwks.Scheme != "https" || jwks.Host != parsed.Host || jwks.User != nil || jwks.RawQuery != "" || jwks.Fragment != "" {
		return nil, errors.New("JWKS must use the configured HTTPS issuer origin")
	}
	return &Authenticator{config: c, memberships: grants, verifier: provider.Verifier(&oidc.Config{ClientID: c.Audience, SupportedSigningAlgs: []string{"RS256"}})}, nil
}
func (a *Authenticator) Config() Config { return a.config }
func (a *Authenticator) verifiedToken(ctx context.Context, raw string) (*oidc.IDToken, map[string]any, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	token, e := a.verifier.Verify(ctx, raw)
	if e != nil {
		return nil, nil, errors.New("invalid access token")
	}
	var claims map[string]any
	if e = token.Claims(&claims); e != nil {
		return nil, nil, errors.New("invalid claims")
	}
	if a.config.RequiredACR != "" {
		acr, ok := claims["acr"].(string)
		if !ok || acr != a.config.RequiredACR {
			return nil, nil, errors.New("required authentication assurance not satisfied")
		}
	}
	client, _ := claims["cid"].(string)
	if token.Subject == "" || client != a.config.ClientID {
		return nil, nil, errors.New("missing authorized client or subject")
	}
	issued, ok := claims["iat"].(float64)
	now := time.Now()
	if !ok || issued <= 0 || issued > float64(now.Unix()+30) || token.Expiry.Sub(time.Unix(int64(issued), 0)) > time.Hour || !token.Expiry.After(time.Unix(int64(issued), 0)) {
		return nil, nil, errors.New("invalid token issuance or lifetime")
	}
	if nbf, exists := claims["nbf"]; exists {
		n, ok := nbf.(float64)
		if !ok || n > float64(now.Unix()) {
			return nil, nil, errors.New("token not yet valid")
		}
	}
	return token, claims, nil
}
func (a *Authenticator) authenticate(ctx context.Context, raw string) (Principal, error) {
	token, claims, e := a.verifiedToken(ctx, raw)
	if e != nil {
		return Principal{}, e
	}
	policy := a
	if a.config.MembershipSource != nil {
		grants, err := a.config.MembershipSource.Lookup(ctx, a.config.Issuer, token.Subject, a.config.Environment)
		if err != nil || len(grants) == 0 {
			return Principal{}, errors.New("no authorized membership")
		}
		grants, err = validateMemberships(grants)
		if err != nil {
			return Principal{}, err
		}
		for _, g := range grants {
			if g.Issuer != a.config.Issuer || g.Subject != token.Subject || g.Environment != a.config.Environment {
				return Principal{}, errors.New("membership authority mismatch")
			}
		}
		snapshot := *a
		snapshot.memberships = grants
		policy = &snapshot
	}
	tenant, _ := claims[a.config.TenantClaim].(string)
	project, _ := claims[a.config.ProjectClaim].(string)
	issued := claims["iat"].(float64)
	var grant Membership
	var err error
	if a.config.ServerBoundProject {
		grant, err = policy.boundMembership(token.Subject, time.Unix(int64(issued), 0))
		if err == nil && ((tenant != "" && tenant != grant.Tenant) || (project != "" && project != grant.Project)) {
			return Principal{}, errors.New("token scope conflicts with server-owned membership")
		}
		tenant, project = grant.Tenant, grant.Project
	} else {
		grant, err = policy.membership(token.Subject, tenant, project, time.Unix(int64(issued), 0))
	}
	if err != nil {
		return Principal{}, err
	}
	roles := []string{grant.Role}
	scopes := []string{}
	if values, ok := claims["scp"].([]any); ok {
		for _, v := range values {
			if scope, ok := v.(string); ok {
				scopes = append(scopes, scope)
			}
		}
	}
	readScope := false
	for _, scope := range scopes {
		if scope == "fabric.read" {
			readScope = true
		}
	}
	if !readScope {
		return Principal{}, errors.New("missing API read scope")
	}
	return Principal{Subject: token.Subject, TenantID: tenant, ProjectID: project, Roles: roles, Scopes: scopes, permissions: grant.Permissions}, nil
}
func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/config" && r.Method == "GET" {
			next.ServeHTTP(w, r)
			return
		}
		if a.config.Mode == "okta" && a.config.LocalIdentityProbe && r.Method == "GET" && r.URL.Path == "/api/v1/auth/identity" {
			header := r.Header.Get("Authorization")
			if !strings.HasPrefix(header, "Bearer ") {
				http.Error(w, "authentication required", 401)
				return
			}
			token, claims, err := a.verifiedToken(r.Context(), strings.TrimPrefix(header, "Bearer "))
			if err != nil {
				http.Error(w, "invalid access token", 401)
				return
			}
			read := false
			if scopes, ok := claims["scp"].([]any); ok {
				for _, scope := range scopes {
					if scope == "fabric.read" {
						read = true
					}
				}
			}
			if !read {
				http.Error(w, "missing API read scope", 401)
				return
			}
			ctx := WithPrincipal(r.Context(), Principal{Subject: token.Subject})
			next.ServeHTTP(w, r.WithContext(context.WithValue(ctx, verifiedIdentityKey{}, true)))
			return
		}
		var p Principal
		if a.config.Mode == "local-development" {
			p = Principal{Subject: "local-developer", TenantID: "local-development", Roles: []string{"admin"}, Scopes: []string{"fabric.read", "fabric.write"}}
		} else {
			header := r.Header.Get("Authorization")
			if !strings.HasPrefix(header, "Bearer ") {
				http.Error(w, "authentication required", http.StatusUnauthorized)
				return
			}
			var e error
			p, e = a.authenticate(r.Context(), strings.TrimPrefix(header, "Bearer "))
			if e != nil {
				http.Error(w, "invalid or unauthorized access token", http.StatusUnauthorized)
				return
			}
		}
		if a.config.Mode == "okta" {
			allowed := false
			permission := localPermission(r)
			for _, v := range p.permissions {
				if v == permission {
					allowed = true
				}
			}
			if !allowed || (permission != "read" && !p.CanWrite()) {
				http.Error(w, "operation not permitted by server membership", http.StatusForbidden)
				return
			}
		}
		// Validation/unsaved plan generation are read-like POSTs. Persistent reviews are writes.
		readOnly := r.Method == "GET" || r.Method == "HEAD" || r.URL.Path == "/api/v1/validate" || r.URL.Path == "/api/v1/plans" || strings.HasSuffix(r.URL.Path, "/cloud-preflight")
		if !readOnly && !p.CanWrite() {
			http.Error(w, "operator role required", http.StatusForbidden)
			return
		}
		ctx := WithPrincipal(r.Context(), p)
		if a.config.Mode == "okta" {
			ctx = context.WithValue(ctx, projectAuthenticatedKey{}, true)
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type projectAuthenticatedKey struct{}

func IsProjectAuthenticated(ctx context.Context) bool {
	v, _ := ctx.Value(projectAuthenticatedKey{}).(bool)
	return v || IsLocalAuthenticated(ctx)
}

type verifiedIdentityKey struct{}

func IsVerifiedIdentity(ctx context.Context) bool {
	verified, _ := ctx.Value(verifiedIdentityKey{}).(bool)
	return verified
}
