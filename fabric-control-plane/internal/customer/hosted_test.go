package customer

import (
	"context"
	"testing"
)

type hostedFixture func(context.Context, HostedRequest) (string, error)

func (h hostedFixture) CreateSession(ctx context.Context, r HostedRequest) (string, error) {
	return h(ctx, r)
}

func TestHostedProviderExactOriginAndIdempotency(t *testing.T) {
	actor := Actor{"https://id.test", "owner", "tenant", "project"}
	store := &fixtureStore{map[string]Account{"tenant": {Owner: actor.identity(), ProviderRef: "opaque-customer"}}, map[string]Change{}}
	svc := Service{Store: store}
	ctx := context.Background()
	count := 0
	var request HostedRequest
	provider := hostedFixture(func(_ context.Context, next HostedRequest) (string, error) {
		count++
		if count > 1 && request != next {
			t.Fatal("provider retry identity changed")
		}
		request = next
		if count == 1 {
			return "https://billing.test.evil.test/session", nil
		}
		return "https://billing.test/session/fixture", nil
	})
	key := "retry-fixture-0001"
	if _, err := svc.HostedSession(ctx, actor, key, "team", nil, "https://billing.test"); err != ErrUnavailable {
		t.Fatal("nil provider enabled")
	}
	if _, err := svc.HostedSession(ctx, actor, key, "team", provider, "https://billing.test"); err != ErrUnavailable {
		t.Fatal("unapproved redirect accepted")
	}
	if _, err := svc.HostedSession(ctx, actor, key, "team", provider, "https://billing.test"); err != nil {
		t.Fatal(err)
	}
	if request.ProviderCustomer != "opaque-customer" || request.Plan != "team" || count != 2 || len(store.audit) != 1 {
		t.Fatal("unexpected provider or audit input")
	}
	if _, err := svc.HostedSession(ctx, actor, key, "", provider, "https://billing.test"); err != ErrConflict {
		t.Fatal("conflicting retry accepted")
	}
	actor.Subject = "foreign"
	if _, err := svc.HostedSession(ctx, actor, key, "team", provider, "https://billing.test"); err != ErrDenied {
		t.Fatal("foreign hosted session allowed")
	}
}
