package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sathonate.local/fabric-control-plane/internal/customer"
	"sathonate.local/fabric-control-plane/internal/deployment"
	"sathonate.local/fabric-control-plane/internal/domain"
	"sathonate.local/fabric-control-plane/internal/drafts"
	"sathonate.local/fabric-control-plane/internal/identity"
	"sathonate.local/fabric-control-plane/internal/topology"
)

type customerHTTPStore struct{ account customer.Account }

func (s *customerHTTPStore) Read(_ context.Context, tenant string) (customer.Account, error) {
	if tenant != "customer-a" {
		return customer.Account{}, customer.ErrDenied
	}
	return s.account, nil
}
func (s *customerHTTPStore) Update(ctx context.Context, tenant string, _ customer.Change, action func(*customer.Account) error) error {
	account, err := s.Read(ctx, tenant)
	if err != nil {
		return err
	}
	if err := action(&account); err != nil {
		return err
	}
	s.account = account
	return nil
}

func TestCustomerPilotOIDCAuthorization(t *testing.T) {
	auth, tokens := secureOIDCFixture(t, []identity.LocalGrant{
		{Tenant: "customer-a", Project: "current", Permissions: []string{"read", "write", "deploy"}},
		{Tenant: "customer-a", Project: "other", Permissions: []string{"read"}},
		{Tenant: "customer-b", Project: "current", Permissions: []string{"read", "write", "deploy"}},
	})
	issuer := auth.Config().Issuer
	store := &customerHTTPStore{account: customer.Account{Name: "Pilot account",
		Owner:       customer.Identity{Issuer: issuer, Subject: "fixture-developer-0"},
		Invoices:    []customer.Invoice{{ID: "invoice-only-owner"}},
		Invitations: []customer.Invitation{{ID: "pending", Recipient: customer.Identity{Issuer: issuer, Subject: "fixture-developer-1"}, State: "pending", ExpiresAt: time.Now().Add(time.Hour)}},
	}}
	service := customer.Service{Store: store}
	handler := auth.Middleware(NewConfigured(nil, []string{"api.test"}, []string{"https://api.test"}, Options{Auth: auth.Config(), Customer: &service}))
	call := func(index int, method, path, body string, status int) string {
		t.Helper()
		request := httptest.NewRequest(method, "https://api.test"+path, strings.NewReader(body))
		request.Header.Set("X-Tenant-ID", "customer-a")
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", "test-request-key-001")
		if index >= 0 {
			request.Header.Set("Authorization", "Bearer "+tokens[index])
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != status {
			t.Fatalf("%s %s returned %d, want %d", method, path, response.Code, status)
		}
		return response.Body.String()
	}
	call(-1, "GET", "/api/v1/customer", "", 401)
	call(2, "GET", "/api/v1/customer", "", 403)
	call(0, "POST", "/api/v1/networks/fixture/dynamic/deploy", "{}", 403)
	body := call(1, "GET", "/api/v1/customer", "", 200)
	if strings.Contains(body, "invoice-only-owner") || strings.Contains(body, "Pilot account") || !strings.Contains(body, "pending") {
		t.Fatal("pending invite data overexposed or missing")
	}
	call(1, "POST", "/api/v1/customer/actions", `{"action":"start-trial"}`, 403)
	call(1, "POST", identity.CustomerInvitationAcceptance, `{"action":"start-trial"}`, 400)
	call(1, "POST", identity.CustomerInvitationAcceptance, `{"action":"accept-invitation","reference":"pending"}`, 204)
	body = call(1, "GET", "/api/v1/customer", "", 200)
	if !strings.Contains(body, "Pilot account") || strings.Contains(body, "invoice-only-owner") {
		t.Fatal("member projection invalid")
	}
	body = call(0, "GET", "/api/v1/customer", "", 200)
	if !strings.Contains(body, "invoice-only-owner") || !strings.Contains(body, `"canManage":true`) {
		t.Fatal("owner projection missing")
	}
	call(0, "POST", "/api/v1/customer/actions", `{"action":"start-trial","tenant":"customer-b"}`, 400)
	call(0, "POST", "/api/v1/customer/actions", `{"action":"start-trial"}`, 204)
	call(0, "POST", "/api/v1/customer/webhook", `{}`, 404)
}

func TestCustomerPreviewAndRetryEntitlement(t *testing.T) {
	auth, tokens := secureOIDCFixture(t, []identity.LocalGrant{{Tenant: "customer-a", Project: "current", Permissions: []string{"read", "write", "retry"}}})
	actor := identity.Principal{TenantID: "customer-a", ProjectID: "current"}
	owner := customer.Identity{Issuer: auth.Config().Issuer, Subject: "fixture-developer-0"}
	store := &customerHTTPStore{account: customer.Account{Owner: owner, Projects: []customer.Project{{ID: "current", State: "active"}},
		Subscription: customer.Subscription{Plan: "team", Status: "active", Until: time.Now().Add(time.Hour)}}}
	service := customer.Service{Store: store}
	draftsStore, err := drafts.Open(filepath.Join(t.TempDir(), "drafts.json"))
	if err != nil {
		t.Fatal(err)
	}
	file, err := os.Open("../../examples/dynamic/a.json")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	spec, err := domain.Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	spec.Spec.Deployment.Provider = domain.CustomerKubernetesProvider
	spec.Spec.Deployment.TargetRef = "rc1-customer"
	draft, err := draftsStore.Save(spec, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	class := topology.StorageCapability{Provisioner: "fixture.test/csi", Encrypted: true, WaitForFirstConsumer: true, MaxCapacityGiB: 100}
	target := customer.Target{Ref: "rc1-customer", Tenant: "customer-a", Project: "current", ClusterSHA256: strings.Repeat("a", 64),
		Namespace: "rc1-customer", NamespaceUID: "fixture-uid", Storage: map[string]topology.StorageCapability{}}
	for _, node := range spec.Spec.Configuration.Nodes {
		target.Storage[node.Storage.Class] = class
	}
	for _, authority := range spec.Spec.Configuration.Authorities {
		target.Storage[authority.Storage.Class] = class
	}
	manager, err := deployment.Open(t.TempDir(), deployment.MockRunner{Delay: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	if _, err = manager.Start(actor.StorageTenant(), owner.Subject, draft.ID, spec, 1, "blocked-fixture", "", ""); err == nil {
		t.Fatal("customer execution started")
	}
	local, err := os.Open("../../examples/local-kubernetes-network.json")
	if err != nil {
		t.Fatal(err)
	}
	defer local.Close()
	localSpec, err := domain.Decode(local)
	if err != nil {
		t.Fatal(err)
	}
	existing, err := manager.Start(actor.StorageTenant(), owner.Subject, "fixture-network", localSpec, 1, "existing-fixture", "", "")
	if err != nil {
		t.Fatal(err)
	}
	var decisions []string
	handler := auth.Middleware(NewConfigured(func(*http.Request) Repository { return fileRepository{draftsStore} }, []string{"api.test"}, nil, Options{
		Auth: auth.Config(), Customer: &service, CustomerTargets: customer.Targets{target}, Deployments: manager, CustomerDecision: func(v string) { decisions = append(decisions, v) },
	}))
	call := func(path, revision string, want int) {
		t.Helper()
		req := httptest.NewRequest("POST", "https://api.test"+path, strings.NewReader("{}"))
		req.Header.Set("Authorization", "Bearer "+tokens[0])
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("If-Match", revision)
		req.Header.Set("Idempotency-Key", "retry-fixture")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code != want {
			t.Fatalf("%s returned %d expected %d: %s", path, response.Code, want, response.Body.String())
		}
		if want == 200 && !strings.Contains(response.Body.String(), `"executionEnabled":false`) {
			t.Fatal("preview execution flag")
		}
	}
	preview := "/api/v1/networks/" + draft.ID + "/customer-preflight"
	call(preview, `"999"`, 409)
	call(preview, `"1"`, 200)
	store.account.Subscription.Until = time.Now().Add(-time.Hour)
	call(preview, `"1"`, 403)
	call("/api/v1/operations/"+existing.ID+"/retry", "", 403)
	if len(decisions) != 1 || decisions[0] != "denied" {
		t.Fatal("denial metric not observed")
	}
}
