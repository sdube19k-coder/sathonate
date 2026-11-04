package identity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// LocalGrant is trusted server configuration. It contains only a SHA256 digest,
// never a usable bearer credential. Each grant selects exactly one project.
type LocalGrant struct {
	TokenSHA256 string    `json:"tokenSHA256"`
	Developer   string    `json:"developer"`
	Tenant      string    `json:"tenant"`
	Project     string    `json:"project"`
	Permissions []string  `json:"permissions"`
	ExpiresAt   time.Time `json:"expiresAt"`
}
type LocalTokens struct {
	grants map[[32]byte]LocalGrant
	now    func() time.Time
}

func GenerateLocalToken() (string, string, error) {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		return "", "", e
	}
	token := hex.EncodeToString(b)
	sum := sha256.Sum256([]byte(token))
	return token, hex.EncodeToString(sum[:]), nil
}
func LoadLocalTokens(environment, path string) (*LocalTokens, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, errors.New("local token grants unavailable")
	}
	defer f.Close()
	d := json.NewDecoder(io.LimitReader(f, 1<<20))
	d.DisallowUnknownFields()
	var grants []LocalGrant
	if d.Decode(&grants) != nil {
		return nil, errors.New("invalid local token grant configuration")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return nil, errors.New("invalid local token grant configuration")
	}
	return NewLocalTokens(environment, grants)
}
func NewLocalTokens(environment string, grants []LocalGrant) (*LocalTokens, error) {
	if environment != "local-development" || len(grants) == 0 || len(grants) > 100 {
		return nil, errors.New("local bearer authentication requires explicit local-development and nonempty server grants")
	}
	a := &LocalTokens{grants: map[[32]byte]LocalGrant{}, now: time.Now}
	for _, g := range grants {
		raw, e := hex.DecodeString(g.TokenSHA256)
		if e != nil || len(raw) != 32 || g.Developer == "" || g.Tenant == "" || g.Project == "" || len(g.Tenant) > 128 || len(g.Project) > 128 || len(g.Developer) > 128 || !g.ExpiresAt.After(time.Now()) || g.ExpiresAt.After(time.Now().Add(24*time.Hour)) {
			return nil, errors.New("invalid or expired local grant; maximum lifetime is 24 hours")
		}
		var key [32]byte
		copy(key[:], raw)
		if _, exists := a.grants[key]; exists {
			return nil, errors.New("duplicate local grant digest")
		}
		allowed := map[string]bool{}
		for _, p := range g.Permissions {
			switch p {
			case "read", "write", "deploy", "retry", "cancel", "cleanup":
			default:
				return nil, errors.New("unsupported local grant permission")
			}
			allowed[p] = true
		}
		if !allowed["read"] {
			return nil, errors.New("local grant must permit read")
		}
		g.Permissions = append([]string(nil), g.Permissions...)
		a.grants[key] = g
	}
	return a, nil
}
func (a *LocalTokens) Config() Config { return Config{Mode: "local-token"} }

// StorageTenant reuses the existing PostgreSQL RLS key without conflating two
// projects of the same tenant. No client-selected header changes this scope.
func (p Principal) StorageTenant() string {
	if p.ProjectID == "" {
		return p.TenantID
	}
	raw, _ := json.Marshal([2]string{p.TenantID, p.ProjectID})
	sum := sha256.Sum256(raw)
	return "lp-" + hex.EncodeToString(sum[:24])
}
func localPermission(r *http.Request) string {
	if r.Method == "GET" || r.Method == "HEAD" || r.URL.Path == "/api/v1/validate" || r.URL.Path == "/api/v1/plans" {
		return "read"
	}
	if strings.HasSuffix(r.URL.Path, "/retry") {
		return "retry"
	}
	if strings.HasSuffix(r.URL.Path, "/cancel") {
		return "cancel"
	}
	if r.Method == "DELETE" || strings.HasSuffix(r.URL.Path, "/cleanup-preview") {
		return "cleanup"
	}
	if strings.HasSuffix(r.URL.Path, "/deploy") {
		return "deploy"
	}
	return "write"
}
func (a *LocalTokens) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/api/v1/auth/config" {
			next.ServeHTTP(w, r)
			return
		}
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			http.Error(w, "authentication required", 401)
			return
		}
		token := strings.TrimPrefix(header, "Bearer ")
		if len(token) != 64 {
			http.Error(w, "invalid local token", 401)
			return
		}
		if _, e := hex.DecodeString(token); e != nil {
			http.Error(w, "invalid local token", 401)
			return
		}
		sum := sha256.Sum256([]byte(token))
		g, ok := a.grants[sum]
		if !ok || !g.ExpiresAt.After(a.now()) {
			http.Error(w, "invalid or expired local token", 401)
			return
		}
		permitted := map[string]bool{}
		for _, p := range g.Permissions {
			permitted[p] = true
		}
		if !permitted[localPermission(r)] {
			http.Error(w, "operation not permitted by local grant", 403)
			return
		}
		role := "viewer"
		scopes := []string{"fabric.read"}
		if permitted["write"] || permitted["deploy"] || permitted["retry"] || permitted["cancel"] || permitted["cleanup"] {
			role = "operator"
			scopes = append(scopes, "fabric.write")
		}
		if permitted["cleanup"] {
			role = "admin"
		}
		p := Principal{Subject: g.Developer, TenantID: g.Tenant, ProjectID: g.Project, Roles: []string{role}, Scopes: scopes}
		next.ServeHTTP(w, r.WithContext(WithPrincipal(context.WithValue(r.Context(), localAuthenticatedKey{}, true), p)))
	})
}

type localAuthenticatedKey struct{}

func IsLocalAuthenticated(ctx context.Context) bool {
	v, _ := ctx.Value(localAuthenticatedKey{}).(bool)
	return v
}
