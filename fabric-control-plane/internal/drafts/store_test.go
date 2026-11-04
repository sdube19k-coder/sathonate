package drafts

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"sathonate.local/fabric-control-plane/internal/domain"
)

func TestLegacyOrderingDefaultsInvalidateOldReview(t *testing.T) {
	path := filepath.Join(t.TempDir(), "drafts.json")
	store, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	d, e := store.Save(fixture(t), "", 0)
	if e != nil {
		t.Fatal(e)
	}
	d, e = store.Review(d.ID, 1)
	if e != nil {
		t.Fatal(e)
	}
	d.Desired.Spec.Ordering = domain.Ordering{}
	b, e := json.Marshal(snapshot{Version: 1, Drafts: map[string]Draft{d.ID: d}})
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(path, b, 0600); e != nil {
		t.Fatal(e)
	}
	reopened, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	restored, e := reopened.Get(d.ID)
	if e != nil {
		t.Fatal(e)
	}
	if restored.Desired.Spec.Ordering.Consensus != "raft" || restored.Plan != nil || restored.Status != "draft" || restored.Revision != 1 {
		t.Fatal("legacy draft was not safely defaulted and old review invalidated")
	}
	if len(restored.Events) != len(d.Events) {
		t.Fatal("migration discarded design history")
	}
}

func fixture(t *testing.T) domain.NetworkSpec {
	t.Helper()
	f, e := os.Open("../../examples/native-network.json")
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	s, e := domain.Decode(f)
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func TestPersistenceRevisionAndReviewLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "drafts.json")
	store, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	draft, e := store.Save(fixture(t), "", 0)
	if e != nil {
		t.Fatal(e)
	}
	reviewed, e := store.Review(draft.ID, 1)
	if e != nil {
		t.Fatal(e)
	}
	if reviewed.Plan == nil || reviewed.Plan.Executable || reviewed.Status != "reviewed" {
		t.Fatal("incorrect review state")
	}
	again, e := store.Review(draft.ID, 1)
	if e != nil {
		t.Fatal(e)
	}
	if len(again.Events) != 2 {
		t.Fatal("repeated review duplicated events")
	}
	reopened, e := Open(path)
	if e != nil {
		t.Fatal(e)
	}
	saved, e := reopened.Get(draft.ID)
	if e != nil || saved.Plan == nil {
		t.Fatal("review not persisted")
	}
	spec := fixture(t)
	spec.Metadata.Name = "Revised design"
	updated, e := reopened.Save(spec, draft.ID, 1)
	if e != nil {
		t.Fatal(e)
	}
	if updated.Plan != nil || updated.Status != "draft" || updated.Revision != 2 {
		t.Fatal("editing must clear review and advance revision")
	}
	if _, e = reopened.Save(spec, draft.ID, 1); !errors.Is(e, ErrConflict) {
		t.Fatal("stale update was accepted")
	}
	if e = reopened.Archive(draft.ID, 1); !errors.Is(e, ErrConflict) {
		t.Fatal("stale archive was accepted")
	}
	if e = reopened.Archive(draft.ID, 2); e != nil {
		t.Fatal(e)
	}
	if len(reopened.List()) != 0 {
		t.Fatal("archived draft still active")
	}
}
func TestConflictIsolationAndWriteFailure(t *testing.T) {
	store, e := Open(filepath.Join(t.TempDir(), "drafts.json"))
	if e != nil {
		t.Fatal(e)
	}
	spec := fixture(t)
	d, e := store.Save(spec, "", 0)
	if e != nil {
		t.Fatal(e)
	}
	spec.Metadata.Name = "outside mutation"
	saved, _ := store.Get(d.ID)
	if saved.Desired.Metadata.Name == spec.Metadata.Name {
		t.Fatal("input alias changes stored draft")
	}
	if _, e = store.Save(fixture(t), "", 0); !errors.Is(e, ErrDuplicate) {
		t.Fatal("duplicate network identifier accepted")
	}
	blocked := filepath.Join(t.TempDir(), "blocked")
	if e = os.Mkdir(blocked, 0700); e != nil {
		t.Fatal(e)
	}
	store.path = blocked
	spec = fixture(t)
	spec.Metadata.Name = "Must not become visible"
	if _, e = store.Save(spec, d.ID, 1); e == nil {
		t.Fatal("write failure ignored")
	}
	saved, _ = store.Get(d.ID)
	if saved.Revision != 1 || saved.Desired.Metadata.Name == spec.Metadata.Name {
		t.Fatal("failed write changed in-memory state")
	}
}
func TestConcurrentEditorsOnlyOneWins(t *testing.T) {
	store, e := Open(filepath.Join(t.TempDir(), "drafts.json"))
	if e != nil {
		t.Fatal(e)
	}
	spec := fixture(t)
	d, e := store.Save(spec, "", 0)
	if e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	var winners atomic.Int32
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := store.Save(spec, d.ID, 1); err == nil {
				winners.Add(1)
			} else if !errors.Is(err, ErrConflict) {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatalf("expected one successful editor, got %d", winners.Load())
	}
}
func TestCorruptStoreFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "drafts.json")
	if e := os.WriteFile(path, []byte("not json"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := Open(path); e == nil {
		t.Fatal("corrupt store silently discarded")
	}
}
