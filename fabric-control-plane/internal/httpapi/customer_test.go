package httpapi

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"sathonate.local/fabric-control-plane/internal/customer"
	"sathonate.local/fabric-control-plane/internal/identity"
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
