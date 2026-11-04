package kubernetesfabric

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestReadbackWaitsForDeliveryWithoutRelaxingTrust(t *testing.T) {
	count := 0
	raw, e := awaitLedgerReadback(context.Background(), time.Millisecond, func() ([]byte, error) {
		count++
		if count < 3 {
			return nil, securePeerFailure([]byte("endorsement failure: key not found"))
		}
		return []byte("exact-expected-value"), nil
	})
	if e != nil || string(raw) != "exact-expected-value" || count != 3 {
		t.Fatal("lagging peer readback failed")
	}
	for _, message := range []string{"certificate signed by unknown authority", "authorization failure", "endorsement failure", "authorization failure key not found"} {
		count = 0
		_, e = awaitLedgerReadback(context.Background(), time.Millisecond, func() ([]byte, error) { count++; return nil, securePeerFailure([]byte(message)) })
		if e == nil || count != 1 {
			t.Fatal("non-delivery error was retried", message)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, e = awaitLedgerReadback(ctx, time.Millisecond, func() ([]byte, error) { return nil, errors.New("must not execute") })
	if !errors.Is(e, context.Canceled) {
		t.Fatal("cancelled readback accepted")
	}
}
