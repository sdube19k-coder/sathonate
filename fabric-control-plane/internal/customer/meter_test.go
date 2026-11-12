package customer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"sathonate.local/fabric-control-plane/internal/identity"
)

type testPage func(context.Context, string, string, time.Time, time.Time) ([]VerifiedOperation, error)

func (f testPage) Page(c context.Context, s, a string, b, e time.Time) ([]VerifiedOperation, error) {
	return f(c, s, a, b, e)
}

func TestMeterRestartsAfterPartialPageCommit(t *testing.T) {
	now := time.Now().UTC()
	store := &fixtureStore{map[string]Account{"tenant": {Projects: []Project{{ID: "project", State: "active"}}}}, map[string]Change{}}
	svc := Service{Store: store, Now: func() time.Time { return now }}
	expected := (identity.Principal{TenantID: "tenant", ProjectID: "project"}).StorageTenant()
	page := make([]VerifiedOperation, 101)
	for i := range page {
		page[i] = VerifiedOperation{ID: fmt.Sprintf("op_%032x", i+1), Scope: expected, Mode: "customer-kubernetes", Status: "succeeded",
			TransactionID: strings.Repeat("a", 64), Ready: true, Verified: true, FinishedAt: now.Add(-time.Hour)}
	}
	fail := true
	source := testPage(func(_ context.Context, scope, cursor string, since, until time.Time) ([]VerifiedOperation, error) {
		if scope != expected || !since.Equal(now.Add(-24*time.Hour)) || !until.Equal(now) {
			t.Fatal("wrong source scope or window")
		}
		if cursor == "" {
			return page[:100], nil
		}
		if cursor != page[99].ID {
			t.Fatal("incorrect cursor")
		}
		if fail {
			return nil, errors.New("fixture interruption")
		}
		return page[100:], nil
	})
	m := Meter{Service: svc, Source: source}
	result, err := m.Sweep(context.Background(), "tenant", "project", 24*time.Hour)
	if err != ErrUnavailable || result.Acknowledged != 100 {
		t.Fatalf("interruption result %+v %v", result, err)
	}
	fail = false
	result, err = m.Sweep(context.Background(), "tenant", "project", 24*time.Hour)
	if err != nil || result.Acknowledged != 101 || store.accounts["tenant"].Usage[0].VerifiedOperations != 101 || len(store.audit) != 101 {
		t.Fatalf("replay double counted or lost usage %+v %v", result, err)
	}
	if _, err = m.Sweep(context.Background(), "tenant", "unapproved", 24*time.Hour); err != ErrDenied {
		t.Fatal("unapproved metering scope")
	}
}
