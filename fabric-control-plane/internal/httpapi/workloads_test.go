package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"

	"strings"
	"testing"
	"time"

	"sathonate.local/fabric-control-plane/internal/deployment"
	"sathonate.local/fabric-control-plane/internal/identity"
	"sathonate.local/fabric-control-plane/internal/operations"
	"sathonate.local/fabric-control-plane/internal/persistence"
)

type workloadReaderFixture struct {
	deployment.MockRunner
	calls   *int
	failure bool
}

func (workloadReaderFixture) Mode() string { return "secure-local" }
func (f workloadReaderFixture) InspectWorkloads(_ context.Context, q deployment.Request) (deployment.WorkloadSnapshot, error) {
	*f.calls++
	if f.failure {
		return deployment.WorkloadSnapshot{}, errors.New("private-provider-error-canary")
	}
	return deployment.WorkloadSnapshot{ReadOnly: true, CheckedAt: time.Now().UTC(), WorkloadsReady: true, Pods: []deployment.WorkloadObservation{}, Volumes: []deployment.VolumeObservation{}, Missing: []string{}, Guidance: []string{}}, nil
}

func TestPostgresCurrentWorkloadsReadGrantScopeAndFailureRedaction(t *testing.T) {
	connection := os.Getenv("SATHONATE_TEST_DATABASE_URL")
	if connection == "" {
		t.Skip("requires dedicated PostgreSQL")
	}
	if !strings.Contains(connection, "/fabric_smp_tests?") {
		t.Fatal("dedicated test database required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "successful-read", true: "provider-failure"}[fail], func(t *testing.T) {
			calls := 0
			db, e := persistence.Open(ctx, connection, false)
			if e != nil {
				t.Fatal("test database unavailable")
			}
			defer db.Close()
			if db.Migrate(ctx) != nil {
				t.Fatal("test schema unavailable")
			}
			m, e := deployment.OpenDatabase(ctx, db.Pool(), workloadReaderFixture{calls: &calls, failure: fail})
			if e != nil {
				db.Close()
				t.Fatal(e)
			}
			defer m.Close()
			tenantID := "workload-" + time.Now().UTC().Format("20060102150405.000000000")
			var grants []identity.LocalGrant
			tokens := []string{}
			for _, scope := range [][2]string{{tenantID, "project-a"}, {tenantID + "-foreign", "project-a"}, {tenantID, "project-b"}} {
				token, digest, e := identity.GenerateLocalToken()
				if e != nil {
					t.Fatal(e)
				}
				tokens = append(tokens, token)
				grants = append(grants, identity.LocalGrant{TokenSHA256: digest, Developer: "viewer", Tenant: scope[0], Project: scope[1], Permissions: []string{"read"}, ExpiresAt: time.Now().Add(time.Hour)})
			}
			auth, e := identity.NewLocalTokens("local-development", grants)
			if e != nil {
				t.Fatal(e)
			}
			scope := (identity.Principal{TenantID: tenantID, ProjectID: "project-a"}).StorageTenant()
			op, _, e := m.Operations().Begin(scope, "viewer", "network-a", "deploy", "read-fixture-key", "digest", "version", "", []operations.Stage{{Key: "inspect", Label: "Fixture"}})
			if e != nil {
				t.Fatal(e)
			}
			defer db.Pool().Exec(context.Background(), "DELETE FROM sathonate_operations WHERE tenant_id=$1 AND id=$2", scope, op.ID)
			defer db.Pool().Exec(context.Background(), "DELETE FROM sathonate_deployments WHERE tenant_id=$1 AND operation_id=$2", scope, op.ID)
			payload, _ := json.Marshal(deployment.Result{Mode: "secure-local", OperationID: op.ID, Revision: 3})
			if _, e = db.Pool().Exec(ctx, "INSERT INTO sathonate_deployments(tenant_id,operation_id,payload) VALUES($1,$2,$3)", scope, op.ID, payload); e != nil {
				t.Fatal("fixture persistence failed")
			}
			h := auth.Middleware(NewConfigured(nil, []string{"localhost:8080"}, nil, Options{Auth: auth.Config(), Deployments: m}))
			path := "/api/v1/deployments/" + op.ID + "/workloads"
			for i, token := range append(tokens, "", tokens[0]+"invalid") {
				w := request(h, http.MethodGet, path, nil, map[string]string{"Authorization": "Bearer " + token})
				want := 404
				if i == 0 {
					want = 200
					if fail {
						want = 503
					}
				}
				if i >= 3 {
					want = 401
				}
				if w.Code != want {
					t.Fatalf("scope %d got %d want %d", i, w.Code, want)
				}
				if strings.Contains(w.Body.String(), "private-provider") {
					t.Fatal("provider diagnostics exposed")
				}
				if i == 0 && !fail {
					var s deployment.WorkloadSnapshot
					if json.Unmarshal(w.Body.Bytes(), &s) != nil || !s.ReadOnly || s.LedgerChecked || s.OperationID != op.ID || s.Revision != 3 {
						t.Fatal("current workload observation confused with ledger verification")
					}
					if w.Header().Get("Cache-Control") != "no-store" {
						t.Fatal("workload data cached")
					}
				}
			}
			if calls != 1 {
				t.Fatal("foreign or unauthenticated request reached workload provider", calls)
			}
		})
	}
}

func TestWorkloadRouteRejectsSimulationAndImplicitDevelopmentIdentity(t *testing.T) {
	m, e := deployment.Open(t.TempDir(), deployment.MockRunner{})
	if e != nil {
		t.Fatal(e)
	}
	defer m.Close()
	token, digest, e := identity.GenerateLocalToken()
	if e != nil {
		t.Fatal(e)
	}
	grant := identity.LocalGrant{TokenSHA256: digest, Developer: "viewer", Tenant: "tenant", Project: "project", Permissions: []string{"read"}, ExpiresAt: time.Now().Add(time.Hour)}
	auth, e := identity.NewLocalTokens("local-development", []identity.LocalGrant{grant})
	if e != nil {
		t.Fatal(e)
	}
	scope := (identity.Principal{TenantID: grant.Tenant, ProjectID: grant.Project}).StorageTenant()
	op, _, e := m.Operations().Begin(scope, "viewer", "network", "deploy", "workload-mock-key", "digest", "version", "", []operations.Stage{{Key: "inspect", Label: "Fixture"}})
	if e != nil {
		t.Fatal(e)
	}
	path := "/api/v1/deployments/" + op.ID + "/workloads"
	h := auth.Middleware(NewConfigured(nil, []string{"localhost:8080"}, nil, Options{Auth: auth.Config(), Deployments: m}))
	if w := request(h, http.MethodGet, path, nil, map[string]string{"Authorization": "Bearer " + token}); w.Code != 409 {
		t.Fatal("simulation was exposed as current Kubernetes state", w.Code)
	}
	legacy := NewConfigured(nil, []string{"localhost:8080"}, nil, Options{Deployments: m})
	if w := request(legacy, http.MethodGet, path, nil, nil); w.Code != 403 {
		t.Fatal("implicit developer identity bypassed authentication", w.Code)
	}
}
