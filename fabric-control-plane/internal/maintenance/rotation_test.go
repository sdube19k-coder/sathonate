package maintenance

import (
	"context"
	"encoding/json"
	"sathonate.local/fabric-control-plane/internal/identity"
	"strings"
	"testing"
	"time"
)

func fixture() (identity.Principal, Binding) {
	return identity.Principal{Subject: "operator", TenantID: "tenant-a", Roles: []string{"admin"}, Scopes: []string{"fabric.write"}}, Binding{"tenant-a", "record", strings.Repeat("a", 64), strings.Repeat("b", 64)}
}

type simulatedDriver struct {
	calls            int
	fail, unverified bool
}

func (d *simulatedDriver) Reconcile(_ context.Context, i Intent, stage, key string) (Receipt, error) {
	d.calls++
	if d.fail {
		return Receipt{}, ErrConflict
	}
	return Receipt{i.BindingSHA256, stage, strings.Repeat("c", 64), !d.unverified, true}, nil
}
func TestRotationInterruptedRetryAndVerification(t *testing.T) {
	p, b := fixture()
	now := time.Now()
	intent, err := NewIntent(p, b, "org1/peer1", "request-one", now)
	if err != nil {
		t.Fatal(err)
	}
	approval := StepApproval{intent.ID, intent.BindingSHA256, "inventory", "change-one", now.Add(time.Hour / 2)}
	driver := &simulatedDriver{fail: true}
	next, err := Advance(context.Background(), p, intent, 1, "checkpoint-one", approval, driver, now)
	if err == nil || next.Revision != 1 {
		t.Fatal("failed step advanced")
	}
	driver.fail = false
	driver.unverified = true
	if _, err = Advance(context.Background(), p, intent, 1, "checkpoint-one", approval, driver, now); err == nil {
		t.Fatal("unverified replacement accepted")
	}
	driver.unverified = false
	next, err = Advance(context.Background(), p, intent, 1, "checkpoint-one", approval, driver, now)
	if err != nil || !next.Simulated {
		t.Fatal("simulated receipt not labelled", err)
	}
	calls := driver.calls
	again, err := Advance(context.Background(), p, next, 1, "checkpoint-one", approval, driver, now)
	if err != nil || again.Revision != 2 || driver.calls != calls {
		t.Fatal("duplicate step re-executed")
	}
	raw, _ := json.Marshal(next)
	var recovered Intent
	if json.Unmarshal(raw, &recovered) != nil {
		t.Fatal("cannot recover")
	}
	approval.Step = "backup"
	recovered, err = Advance(context.Background(), p, recovered, 2, "checkpoint-two", approval, driver, now)
	if err != nil || recovered.Phase != "backup" {
		t.Fatal("checkpoint recovery failed", err)
	}
}
func TestRotationScopeAndDisabledLiveAdapter(t *testing.T) {
	p, b := fixture()
	now := time.Now()
	intent, _ := NewIntent(p, b, "org1/peer1", "request-one", now)
	approval := StepApproval{intent.ID, intent.BindingSHA256, "inventory", "change-one", now.Add(time.Minute)}
	if _, err := Advance(context.Background(), p, intent, 1, "checkpoint-one", approval, nil, now); err == nil {
		t.Fatal("missing adapter accepted")
	}
	other := p
	other.TenantID = "foreign"
	if _, err := NewIntent(other, b, "org1/peer1", "request-one", now); err == nil {
		t.Fatal("cross tenant plan accepted")
	}
	other = p
	other.Roles = []string{"operator"}
	if _, err := Advance(context.Background(), other, intent, 1, "checkpoint-one", approval, &simulatedDriver{}, now); err == nil {
		t.Fatal("operator rotated credentials")
	}
	approval.BindingSHA256 = strings.Repeat("f", 64)
	if _, err := Advance(context.Background(), p, intent, 1, "checkpoint-one", approval, &simulatedDriver{}, now); err == nil {
		t.Fatal("wrong binding accepted")
	}
	if _, err := NewIntent(p, b, "password=private", "request-one", now); err == nil {
		t.Fatal("arbitrary identity input accepted")
	}
}
