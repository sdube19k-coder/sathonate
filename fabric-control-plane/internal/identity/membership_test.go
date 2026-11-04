package identity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestServerMembershipConfigurationFailsClosed(t *testing.T) {
	now := time.Now()
	base := Membership{Environment: "production", Issuer: "https://fixture.example/oauth2/default", Subject: "subject", Tenant: "tenant", Project: "project", Role: "viewer", Permissions: []string{"read"}, NotBefore: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour)}
	for _, tc := range []struct {
		name   string
		change func(*Membership)
	}{{"viewer escalation", func(g *Membership) { g.Permissions = []string{"read", "deploy"} }}, {"operator cleanup", func(g *Membership) { g.Role = "operator"; g.Permissions = []string{"read", "cleanup"} }}, {"unknown operation", func(g *Membership) { g.Permissions = []string{"read", "superuser"} }}, {"empty project", func(g *Membership) { g.Project = "" }}, {"unknown role", func(g *Membership) { g.Role = "root" }}, {"missing read", func(g *Membership) { g.Permissions = nil }}, {"invalid validity", func(g *Membership) { g.ExpiresAt = g.NotBefore }}} {
		t.Run(tc.name, func(t *testing.T) {
			g := base
			tc.change(&g)
			raw, _ := json.Marshal([]Membership{g})
			path := filepath.Join(t.TempDir(), "policy.json")
			if os.WriteFile(path, raw, 0600) != nil {
				t.Fatal("fixture")
			}
			if _, err := loadMemberships(path); err == nil {
				t.Fatal("invalid authorization policy accepted")
			}
		})
	}
	for _, data := range []string{`[]`, `[{"password":"forbidden"}]`, `[] {}`} {
		path := filepath.Join(t.TempDir(), "invalid.json")
		_ = os.WriteFile(path, []byte(data), 0600)
		if _, err := loadMemberships(path); err == nil {
			t.Fatal("invalid policy accepted")
		}
	}
}
