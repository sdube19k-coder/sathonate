package customer

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

type fixtureStore struct {
	accounts map[string]Account
	audit    map[string]Change
}

func (s *fixtureStore) Read(_ context.Context, tenant string) (Account, error) {
	account, ok := s.accounts[tenant]
	if !ok {
		return Account{}, ErrDenied
	}
	raw, _ := json.Marshal(account)
	var copy Account
	_ = json.Unmarshal(raw, &copy)
	return copy, nil
}
func (s *fixtureStore) Update(ctx context.Context, tenant string, change Change, apply func(*Account) error) error {
	account, err := s.Read(ctx, tenant)
	if err != nil {
		return err
	}
	key := tenant + "/" + change.ID
	if existing, ok := s.audit[key]; ok {
		if existing.Digest != change.Digest {
			return ErrConflict
		}
		return nil
	}
	if err = apply(&account); err != nil {
		return err
	}
	s.accounts[tenant], s.audit[key] = account, change
	return nil
}

func TestPilotLifecycle(t *testing.T) {
	now := time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC)
	owner := Actor{"https://id.example.test/oauth2/default", "owner", "tenant-a", "initial"}
	invitee := Actor{owner.Issuer, "invitee", owner.Tenant, owner.Project}
	other := Actor{owner.Issuer, invitee.Subject, "tenant-b", owner.Project}
	store := &fixtureStore{map[string]Account{"tenant-a": {Name: "Customer", Owner: owner.identity(), ProviderRef: "fixture-customer"}, "tenant-b": {Name: "Foreign", Owner: other.identity(), ProviderRef: "foreign"}}, map[string]Change{}}
	svc := Service{Store: store, Now: func() time.Time { return now }}
	ctx := context.Background()
	expect := func(got, want error) {
		t.Helper()
		if !errors.Is(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	execute := func(actor Actor, key, action, name, ref string) error {
		return svc.Execute(ctx, actor, "fixture-key-0000"+key, Command{action, name, ref})
	}
	expect(execute(owner, "1", "request-project", "Pilot", ""), ErrDenied)
	expect(execute(invitee, "2", "start-trial", "", ""), ErrDenied)
	expect(execute(owner, "2", "start-trial", "", ""), nil)
	expect(execute(owner, "2", "start-trial", "", ""), nil)
	expect(execute(owner, "2", "request-project", "Pilot", ""), ErrConflict)
	expect(execute(owner, "3", "invite", "", invitee.Subject), nil)
	pending, err := svc.View(ctx, invitee)
	expect(err, nil)
	if pending.Name != "" || len(pending.PendingInvitations) != 1 {
		t.Fatal("invitee saw account data")
	}
	invitation := pending.PendingInvitations[0].ID
	expect(execute(other, "4", "accept-invitation", "", invitation), ErrDenied)
	wrongIssuer := invitee
	wrongIssuer.Issuer = "https://foreign.test"
	expect(execute(wrongIssuer, "4", "accept-invitation", "", invitation), ErrDenied)
	expect(execute(invitee, "4", "accept-invitation", "", invitation), nil)
	expect(execute(invitee, "5", "rename-organization", "Foreign", ""), ErrDenied)
	expired := invitee
	expired.Subject = "expires"
	expect(execute(owner, "expiry-invite", "invite", "", expired.Subject), nil)
	pending, err = svc.View(ctx, expired)
	expect(err, nil)
	expiry := pending.PendingInvitations[0].ID
	now = now.Add(48 * time.Hour)
	expect(execute(expired, "accept-expired", "accept-invitation", "", expiry), ErrConflict)
	if _, err = svc.View(ctx, expired); err != ErrDenied {
		t.Fatal("expired invite visible")
	}
	expect(execute(owner, "revoke-invite", "invite", "", expired.Subject), nil)
	pending, err = svc.View(ctx, expired)
	expect(err, nil)
	revocation := pending.PendingInvitations[0].ID
	expect(execute(owner, "revoked", "revoke-invitation", "", revocation), nil)
	expect(execute(expired, "accept-revoked", "accept-invitation", "", revocation), ErrConflict)

	expect(execute(owner, "6", "request-project", "Pilot", ""), nil)
	expect(execute(owner, "6", "request-project", "Pilot", ""), nil)
	expect(execute(owner, "7", "request-project", "Two", ""), nil)
	expect(execute(owner, "8", "request-project", "Three", ""), nil)
	expect(execute(owner, "9", "request-project", "Four", ""), ErrConflict)
	view, err := svc.View(ctx, owner)
	expect(err, nil)
	project := owner
	project.Project = view.Projects[0].ID
	expect(svc.AllowProject(ctx, project), ErrDenied)
	expect(svc.ApproveProject(ctx, owner.Tenant, project.Project, "approved-fixture"), nil)
	expect(svc.AllowProject(ctx, project), nil)
	receipt := UsageReceipt{"verified-receipt", project.Project, "operation-verified", now}
	expect(svc.recordUsage(ctx, owner.Tenant, receipt), nil)
	expect(svc.recordUsage(ctx, owner.Tenant, receipt), nil)
	expect(svc.recordUsage(ctx, other.Tenant, receipt), ErrDenied)
	view, err = svc.View(ctx, owner)
	expect(err, nil)
	if len(view.Usage) != 1 || view.Usage[0].VerifiedOperations != 1 {
		t.Fatal("duplicate usage charged")
	}
	now = now.Add(14 * 24 * time.Hour)
	expect(svc.AllowProject(ctx, project), ErrDenied)
	expect(execute(owner, "10", "start-trial", "", ""), ErrConflict)
}

func TestWebhookStateAndReplay(t *testing.T) {
	now := time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC)
	owner := Actor{"https://id.example.test/oauth2/default", "owner", "tenant-a", "initial"}
	store := &fixtureStore{map[string]Account{"tenant-a": {Name: "Customer", Owner: owner.identity(), ProviderRef: "fixture-customer"}}, map[string]Change{}}
	svc := Service{Store: store, Now: func() time.Time { return now }}
	key := bytes.Repeat([]byte{91}, 32)
	h := svc.Webhook(FixtureVerifier{Key: key})
	delivery := now
	send := func(event PaymentEvent, signed bool, want int) {
		t.Helper()
		body, _ := json.Marshal(event)
		r := httptest.NewRequest(http.MethodPost, "/fixture/payment", bytes.NewReader(body))
		mac := hmac.New(sha256.New, key)
		stamp := strconv.FormatInt(delivery.Unix(), 10)
		mac.Write([]byte(stamp + "."))
		mac.Write(body)
		if signed {
			r.Header.Set("Payment-Signature", stamp+"."+hex.EncodeToString(mac.Sum(nil)))
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s got %d, want %d", event.Type, w.Code, want)
		}
	}
	active := PaymentEvent{ID: "event1", Tenant: owner.Tenant, Customer: "fixture-customer", Type: "subscription.active", Plan: "team", At: now, Until: now.Add(30 * 24 * time.Hour)}
	send(active, false, 403)
	wrong := active
	wrong.Customer = "other"
	send(wrong, true, 403)
	send(active, true, 204)
	send(active, true, 204)
	wrong = active
	wrong.Plan = "pilot"
	send(wrong, true, 409)
	now = now.Add(time.Second)
	open := PaymentEvent{ID: "invoice-event", Tenant: owner.Tenant, Customer: "fixture-customer", At: now, Type: "invoice.open", InvoiceID: "invoice1", Currency: "USD", AmountMinor: 4900}
	send(open, true, 204)
	now = now.Add(time.Second)
	paid := open
	paid.ID, paid.Type, paid.At = "paid", "invoice.paid", now
	send(paid, true, 204)
	tampered := paid
	tampered.ID, tampered.AmountMinor, tampered.Type = "tampered", 1, "invoice.void"
	send(tampered, true, 409)
	now = now.Add(time.Second)
	send(PaymentEvent{ID: "ended", Tenant: owner.Tenant, Customer: "fixture-customer", At: now, Type: "subscription.past_due"}, true, 204)
	view, err := svc.View(context.Background(), owner)
	if err != nil || view.ActiveEntitlement || view.Invoices[0].Status != "paid" || len(store.audit) != 4 {
		t.Fatal("unexpected account transition or replay record")
	}
	stale := active
	stale.ID = "stale-active"
	send(stale, true, 409)
	now = now.Add(6 * time.Minute)
	send(active, true, 403)
	delivery = now
	send(active, true, 204)
}
