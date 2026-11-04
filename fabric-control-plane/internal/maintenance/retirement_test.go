package maintenance

import (
	"context"
	"testing"
	"time"
)

func TestRetirementCannotSkipVerificationOrExpiredApproval(t *testing.T) {
	p, b := fixture()
	now := time.Now()
	intent, _ := NewIntent(p, b, "org1/peer1", "request-one", now)
	approval := StepApproval{intent.ID, intent.BindingSHA256, "retirement", "change-one", now.Add(time.Minute)}
	driver := &simulatedDriver{}
	if _, err := Advance(context.Background(), p, intent, 1, "checkpoint-one", approval, driver, now); err == nil || driver.calls != 0 {
		t.Fatal("retirement bypassed verification")
	}
	approval.Step = "inventory"
	approval.ExpiresAt = now.Add(-time.Second)
	if _, err := Advance(context.Background(), p, intent, 1, "checkpoint-one", approval, driver, now); err == nil {
		t.Fatal("expired approval accepted")
	}
}
