// Package drafts provides single-process local draft persistence, not production orchestration storage.
package drafts

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"sathonate.local/fabric-control-plane/internal/domain"
	"sathonate.local/fabric-control-plane/internal/planner"
)

var ErrNotFound = errors.New("network draft not found")
var ErrConflict = errors.New("draft has changed; reload before saving")
var ErrInvalid = errors.New("network draft has validation errors")
var ErrDuplicate = errors.New("an active draft already uses this network identifier")
var ErrIdentity = errors.New("network identifier cannot change")

type Event struct {
	ID       string    `json:"id"`
	Kind     string    `json:"kind"`
	Message  string    `json:"message"`
	At       time.Time `json:"at"`
	Revision int       `json:"revision"`
}
type Draft struct {
	ID        string             `json:"id"`
	Revision  int                `json:"revision"`
	Status    string             `json:"status"`
	Desired   domain.NetworkSpec `json:"desired"`
	Issues    []domain.Issue     `json:"issues"`
	Plan      *planner.Plan      `json:"plan,omitempty"`
	Events    []Event            `json:"events"`
	CreatedAt time.Time          `json:"createdAt"`
	UpdatedAt time.Time          `json:"updatedAt"`
	Archived  bool               `json:"archived"`
}
type snapshot struct {
	Version int              `json:"version"`
	Drafts  map[string]Draft `json:"drafts"`
}
type Store struct {
	mu   sync.Mutex
	path string
	data map[string]Draft
}

func Open(path string) (*Store, error) {
	s := &Store{path: path, data: map[string]Draft{}}
	b, e := os.ReadFile(path)
	if errors.Is(e, os.ErrNotExist) {
		return s, nil
	}
	if e != nil {
		return nil, e
	}
	var saved snapshot
	if e = json.Unmarshal(b, &saved); e != nil {
		return nil, e
	}
	if saved.Version != 1 || saved.Drafts == nil {
		return nil, errors.New("unsupported or invalid local draft store")
	}
	s.data = saved.Drafts
	for key, draft := range s.data {
		if draft.Desired.Spec.Ordering.Consensus == "" {
			draft.Desired = domain.WithDefaults(draft.Desired)
			draft.Issues = domain.Validate(draft.Desired)
			draft.Plan = nil
			draft.Status = "draft"
			s.data[key] = draft
		}
	}
	return s, nil
}
func id() string {
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b[:])
}
func clone[T any](v T) T { b, _ := json.Marshal(v); var c T; _ = json.Unmarshal(b, &c); return c }
func (s *Store) List() []Draft {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []Draft{}
	for _, d := range s.data {
		if !d.Archived {
			out = append(out, clone(d))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out
}
func (s *Store) Get(key string) (Draft, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.data[key]
	if !ok {
		return Draft{}, ErrNotFound
	}
	return clone(d), nil
}

// commit persists a new snapshot before exposing it to readers. One API process owns this file.
func (s *Store) commit(d Draft) error {
	next := make(map[string]Draft, len(s.data)+1)
	for k, v := range s.data {
		next[k] = v
	}
	next[d.ID] = d
	if e := os.MkdirAll(filepath.Dir(s.path), 0700); e != nil {
		return e
	}
	b, e := json.Marshal(snapshot{Version: 1, Drafts: next})
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(s.path), ".drafts-*.tmp")
	if e != nil {
		return e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	if e = os.Rename(tmp, s.path); e != nil {
		return e
	}
	s.data = next
	return nil
}
func (s *Store) Save(spec domain.NetworkSpec, key string, revision int) (Draft, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var previous *Draft
	if key != "" {
		old, ok := s.data[key]
		if !ok {
			return Draft{}, ErrNotFound
		}
		previous = &old
	} else {
		for _, old := range s.data {
			if !old.Archived && old.Desired.Metadata.ID == spec.Metadata.ID {
				return Draft{}, ErrDuplicate
			}
		}
	}
	d, e := BuildDraft(spec, previous, revision)
	if e != nil {
		return Draft{}, e
	}
	if e = s.commit(d); e != nil {
		return Draft{}, e
	}
	return clone(d), nil
}
func (s *Store) Review(key string, revision int) (Draft, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.data[key]
	if !ok {
		return Draft{}, ErrNotFound
	}
	d, e := BuildReview(old, revision)
	if e != nil {
		return Draft{}, e
	}
	if old.Plan != nil && d.Plan != nil && old.Plan.ID == d.Plan.ID {
		return clone(d), nil
	}
	if e = s.commit(d); e != nil {
		return Draft{}, e
	}
	return clone(d), nil
}
func (s *Store) Archive(key string, revision int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.data[key]
	if !ok {
		return ErrNotFound
	}
	d, e := BuildArchive(old, revision)
	if e != nil {
		return e
	}
	if old.Archived {
		return nil
	}
	return s.commit(d)
}
