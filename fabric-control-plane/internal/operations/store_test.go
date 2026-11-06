package operations

import (
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func begin(t *testing.T, s *Store, key string) Operation {
	t.Helper()
	o, _, e := s.Begin("tenant-a", "operator", "network", "start", key, "digest", "version", "", []Stage{{Key: "validate", Label: "Validate"}, {Key: "deploy", Label: "Deploy"}})
	if e != nil {
		t.Fatal(e)
	}
	return o
}
func TestDurableLifecycleReplayAndTenantIsolation(t *testing.T) {
	root := t.TempDir()
	s, e := Open(root)
	if e != nil {
		t.Fatal(e)
	}
	o := begin(t, s, "same-request")
	again, replay, e := s.Begin("tenant-a", "operator", "network", "start", "same-request", "digest", "version", "", nil)
	if e == nil || replay {
		t.Fatal("invalid empty stages accepted", again)
	}
	again, replay, e = s.Begin("tenant-a", "operator", "network", "start", "same-request", "digest", "version", "", []Stage{{Key: "validate"}})
	if e != nil || !replay || again.ID != o.ID {
		t.Fatal("request replay lost identity", e)
	}
	if _, _, e = s.Begin("tenant-a", "operator", "network", "start", "same-request", "different", "version", "", []Stage{{Key: "validate"}}); !errors.Is(e, ErrIdempotency) {
		t.Fatal("key reused for changed request")
	}
	if _, e = s.Get("tenant-b", o.ID); !errors.Is(e, ErrNotFound) {
		t.Fatal("cross-tenant read")
	}
	if len(s.List("tenant-b")) != 0 {
		t.Fatal("cross-tenant list")
	}
	if e = s.Advance(o.ID, "deploy"); e == nil {
		t.Fatal("skipped validation")
	}
	if e = s.Advance(o.ID, "validate"); e != nil {
		t.Fatal(e)
	}
	if e = s.Finish(o.ID, nil); e == nil {
		t.Fatal("succeeded with pending deployment")
	}
	if e = s.Advance(o.ID, "deploy"); e != nil {
		t.Fatal(e)
	}
	if e = s.Event(o.ID, "Bearer hidden-token"); e != nil {
		t.Fatal(e)
	}
	if e = s.Finish(o.ID, nil); e != nil {
		t.Fatal(e)
	}
	reopened, e := Open(root)
	if e != nil {
		t.Fatal(e)
	}
	saved, e := reopened.Get("tenant-a", o.ID)
	if e != nil || saved.Status != "succeeded" || saved.Stages[1].Status != "succeeded" {
		t.Fatal("completion not durable", e)
	}
	for _, event := range saved.Events {
		if strings.Contains(event.Message, "hidden-token") {
			t.Fatal("token in journal")
		}
	}
}
func TestRecoveryAndRetryLink(t *testing.T) {
	root := t.TempDir()
	s, _ := Open(root)
	o := begin(t, s, "first-request")
	_ = s.Advance(o.ID, "validate")
	reopened, e := Open(root)
	if e != nil {
		t.Fatal(e)
	}
	old, _ := reopened.Get("tenant-a", o.ID)
	if old.Status != "interrupted" || old.Stages[0].Status != "interrupted" {
		t.Fatal("running job survived executor restart")
	}
	retry, _, e := reopened.Begin("tenant-a", "operator", "network", "start", "retry-request", "digest", "version", o.ID, old.Stages)
	if e != nil || retry.RetryOf != o.ID || retry.ID == o.ID {
		t.Fatal("invalid retry linkage", e)
	}
	if _, _, e = reopened.Begin("tenant-b", "operator", "network", "start", "foreign-retry", "digest", "version", o.ID, old.Stages); !errors.Is(e, ErrNotFound) {
		t.Fatal("cross-tenant retry accepted")
	}
}
func TestOnlyOneConcurrentOperationWins(t *testing.T) {
	s, _ := Open(t.TempDir())
	var wg sync.WaitGroup
	var mu sync.Mutex
	wins := 0
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, e := s.Begin("tenant-a", "operator", "network", "start", "", "digest", "version", "", []Stage{{Key: "validate"}})
			if e == nil {
				mu.Lock()
				wins++
				mu.Unlock()
			} else if !errors.Is(e, ErrConflict) {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	if wins != 1 {
		t.Fatal("conflicting jobs accepted", wins)
	}
}
func TestFailedPersistenceDoesNotLeaseNetwork(t *testing.T) {
	s, _ := Open(t.TempDir())
	original := s.root
	s.root = filepath.Join(original, "missing")
	if _, _, e := s.Begin("tenant", "operator", "network", "start", "", "digest", "version", "", []Stage{{Key: "validate"}}); e == nil {
		t.Fatal("unpersisted operation accepted")
	}
	if len(s.List("tenant")) != 0 {
		t.Fatal("failed write mutated state")
	}
	s.root = original
	if _, _, e := s.Begin("tenant", "operator", "network", "start", "", "digest", "version", "", []Stage{{Key: "validate"}}); e != nil {
		t.Fatal("failed write retained lease", e)
	}
}

func TestMonitoringHistoryPreservesTerminalExecutionAndDeduplicates(t *testing.T) {
	root := t.TempDir()
	s, e := Open(root)
	if e != nil {
		t.Fatal(e)
	}
	op := begin(t, s, "monitoring")
	if s.Advance(op.ID, "validate") != nil || s.Advance(op.ID, "deploy") != nil || s.Finish(op.ID, nil) != nil {
		t.Fatal("terminal fixture")
	}
	before, _ := s.Get("tenant-a", op.ID)
	fp := strings.Repeat("a", 64)
	if s.Observe("foreign", op.ID, false, 1, fp) == nil {
		t.Fatal("foreign observation")
	}
	if s.Observe("tenant-a", op.ID, false, 1, "private-arbitrary-text") == nil {
		t.Fatal("untyped diagnostics accepted")
	}
	if s.Observe("tenant-a", op.ID, false, 1, fp) != nil || s.Observe("tenant-a", op.ID, false, 1, fp) != nil {
		t.Fatal("terminal monitoring rejected")
	}
	after, _ := s.Get("tenant-a", op.ID)
	if after.Status != before.Status || after.CurrentStage != before.CurrentStage || !after.FinishedAt.Equal(*before.FinishedAt) || len(after.Events) != len(before.Events)+1 {
		t.Fatal("monitoring changed execution state or duplicated checkpoint")
	}
	if s.Advance(op.ID, "deploy") == nil || s.Finish(op.ID, nil) == nil {
		t.Fatal("monitoring reopened terminal execution")
	}
	reopened, e := Open(root)
	if e != nil {
		t.Fatal(e)
	}
	again, _ := reopened.Get("tenant-a", op.ID)
	if len(again.Events) != len(after.Events) || again.Status != "succeeded" {
		t.Fatal("observation not durable")
	}
}
