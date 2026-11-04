package kubernetesfabric

import (
	"context"
	"strings"
	"time"
)

// Commit events on endorsing peers do not guarantee delivery to every peer.
// Retry only the sample contract's explicit absent-key result. All trust,
// authorization, execution and other errors remain terminal. The caller still
// requires exact value equality and the manager verifies every ledger hash.
func awaitLedgerReadback(ctx context.Context, interval time.Duration, query func() ([]byte, error)) ([]byte, error) {
	for {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		raw, e := query()
		if e == nil || !strings.Contains(e.Error(), "(ledger-value-unavailable)") {
			return raw, e
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(interval):
		}
	}
}
