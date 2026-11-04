// Package operations records tenant-scoped local jobs before execution begins.
// This file store coordinates one API process; it is not a distributed scheduler.
package operations

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"path/filepath"
	"regexp"
	"sathonate.local/fabric-control-plane/internal/redact"
	"sort"
	"sync"
	"time"
)

var ErrReadOnly = errors.New("recorded history is read-only; execution is disabled")
var ErrNotFound = errors.New("operation not found")
var ErrConflict = errors.New("another operation is active for this network")
var ErrIdempotency = errors.New("idempotency key was used for a different request")

type Stage struct {
	Key        string     `json:"key"`
	Label      string     `json:"label"`
	Status     string     `json:"status"`
	StartedAt  *time.Time `json:"startedAt,omitempty"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
}
type Event struct {
	Sequence int       `json:"sequence"`
	Time     time.Time `json:"time"`
	Stage    string    `json:"stage,omitempty"`
	Message  string    `json:"message"`
}
type Operation struct {
	ID              string     `json:"operationId"`
	TenantID        string     `json:"tenantId"`
	Actor           string     `json:"actor"`
	NetworkID       string     `json:"networkId"`
	Kind            string     `json:"kind"`
	Status          string     `json:"status"`
	CurrentStage    string     `json:"currentStage,omitempty"`
	ProfileDigest   string     `json:"profileDigest"`
	RuntimeVersion  string     `json:"runtimeVersion"`
	IdempotencyKey  string     `json:"-"`
	RetryOf         string     `json:"retryOf,omitempty"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
	FinishedAt      *time.Time `json:"finishedAt,omitempty"`
	CancelRequested bool       `json:"cancelRequested,omitempty"`
	Error           string     `json:"error,omitempty"`
	Stages          []Stage    `json:"stages"`
	Events          []Event    `json:"events"`
}

// diskRecord retains the deduplication key without exposing it in HTTP responses.
type diskRecord struct {
	Operation
	Key string `json:"idempotencyKey"`
}
type Store struct {
	readOnly bool
	mu       sync.Mutex
	root     string
	items    map[string]Operation
	pool     *pgxpool.Pool
}

func Open(root string) (*Store, error) {
	if e := os.MkdirAll(root, 0700); e != nil {
		return nil, e
	}
	s := &Store{root: root, items: map[string]Operation{}}
	files, e := filepath.Glob(filepath.Join(root, "op_*.json"))
	if e != nil {
		return nil, e
	}
	for _, f := range files {
		b, e := os.ReadFile(f)
		if e != nil {
			return nil, e
		}
		var d diskRecord
		if e = json.Unmarshal(b, &d); e != nil {
			return nil, errors.New("invalid operation journal")
		}
		o := d.Operation
		o.IdempotencyKey = d.Key
		if !idPattern.MatchString(o.ID) || o.TenantID == "" || o.NetworkID == "" {
			return nil, errors.New("invalid operation identity")
		}
		if o.Status == "running" {
			now := time.Now().UTC()
			o.Status = "interrupted"
			o.Error = "Executor restarted during this operation. Retry reconciles resources and rechecks completed stages."
			o.UpdatedAt = now
			o.FinishedAt = &now
			for i := range o.Stages {
				if o.Stages[i].Status == "running" {
					o.Stages[i].Status = "interrupted"
					o.Stages[i].FinishedAt = &now
				}
			}
			appendEvent(&o, o.Error)
			if e = s.persist(o); e != nil {
				return nil, e
			}
		}
		s.items[o.ID] = o
	}
	return s, nil
}

var idPattern = regexp.MustCompile(`^op_[a-f0-9]{32}$`)
var keyPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{8,128}$`)

func ValidKey(key string) bool { return key == "" || keyPattern.MatchString(key) }
func clone(o Operation) Operation {
	b, _ := json.Marshal(o)
	var c Operation
	_ = json.Unmarshal(b, &c)
	c.IdempotencyKey = o.IdempotencyKey
	return c
}
func appendEvent(o *Operation, message string) {
	o.Events = append(o.Events, Event{len(o.Events) + 1, time.Now().UTC(), o.CurrentStage, redact.Text(message)})
	o.UpdatedAt = time.Now().UTC()
}
func (s *Store) persist(o Operation) error {
	if s.readOnly {
		return ErrReadOnly
	}
	if s.pool != nil {
		return s.persistDatabase(o)
	}
	b, e := json.MarshalIndent(diskRecord{o, o.IdempotencyKey}, "", "  ")
	if e != nil {
		return e
	}
	p := filepath.Join(s.root, o.ID+".json")
	f, e := os.OpenFile(p+".tmp", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if e != nil {
		return e
	}
	if _, e = f.Write(b); e != nil {
		_ = f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		_ = f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Rename(p+".tmp", p)
}
func (s *Store) Begin(tenant, actor, network, kind, key, digest, version, retryOf string, stages []Stage) (Operation, bool, error) {
	return s.begin(tenant, actor, network, kind, key, digest, version, retryOf, stages, nil)
}

// BeginPrepared atomically records durable cloud input with the operation.
func (s *Store) BeginPrepared(tenant, actor, network, kind, key, digest, version, retryOf string, stages []Stage, prepare func(Operation) ([]byte, error)) (Operation, bool, error) {
	if s.pool == nil {
		return Operation{}, false, errors.New("durable scheduling requires PostgreSQL")
	}
	return s.begin(tenant, actor, network, kind, key, digest, version, retryOf, stages, prepare)
}
func (s *Store) begin(tenant, actor, network, kind, key, digest, version, retryOf string, stages []Stage, prepare func(Operation) ([]byte, error)) (Operation, bool, error) {
	if s.readOnly {
		return Operation{}, false, ErrReadOnly
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if tenant == "" || actor == "" || network == "" || digest == "" || !ValidKey(key) || len(stages) == 0 {
		return Operation{}, false, errors.New("invalid operation request")
	}
	seen := map[string]bool{}
	for _, stage := range stages {
		if stage.Key == "" || seen[stage.Key] {
			return Operation{}, false, errors.New("operation stages require unique keys")
		}
		seen[stage.Key] = true
	}
	for _, o := range s.items {
		if o.TenantID == tenant && key != "" && o.IdempotencyKey == key {
			if o.NetworkID != network || o.Kind != kind || o.ProfileDigest != digest || o.RetryOf != retryOf {
				return Operation{}, false, ErrIdempotency
			}
			return clone(o), true, nil
		}
	}
	for _, o := range s.items {
		if o.TenantID == tenant && o.NetworkID == network && o.Status == "running" {
			return Operation{}, false, ErrConflict
		}
	}
	if retryOf != "" {
		old, ok := s.items[retryOf]
		if !ok || old.TenantID != tenant {
			return Operation{}, false, ErrNotFound
		}
		if old.NetworkID != network || old.Kind != kind || old.ProfileDigest != digest || (old.Status != "failed" && old.Status != "interrupted") {
			return Operation{}, false, errors.New("only a failed or interrupted operation with the same profile can be retried")
		}
	}
	raw := make([]byte, 16)
	if _, e := rand.Read(raw); e != nil {
		return Operation{}, false, e
	}
	now := time.Now().UTC()
	o := Operation{ID: "op_" + hex.EncodeToString(raw), TenantID: tenant, Actor: actor, NetworkID: network, Kind: kind, Status: "running", ProfileDigest: digest, RuntimeVersion: version, IdempotencyKey: key, RetryOf: retryOf, CreatedAt: now, UpdatedAt: now, Events: []Event{}, Stages: append([]Stage{}, stages...)}
	for i := range o.Stages {
		o.Stages[i].Status = "pending"
		o.Stages[i].StartedAt = nil
		o.Stages[i].FinishedAt = nil
	}
	appendEvent(&o, "Operation recorded; execution queued in the local worker")
	var persistErr error
	if prepare != nil {
		var payload []byte
		payload, persistErr = prepare(o)
		if persistErr == nil {
			persistErr = s.persistPrepared(o, payload)
		}
	} else {
		persistErr = s.persist(o)
	}
	if e := persistErr; e != nil {
		return Operation{}, false, e
	}
	s.items[o.ID] = o
	return clone(o), false, nil
}
func (s *Store) Get(tenant, id string) (Operation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.items[id]
	if !ok || o.TenantID != tenant {
		return Operation{}, ErrNotFound
	}
	return clone(o), nil
}
func (s *Store) List(tenant string) []Operation {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []Operation{}
	for _, o := range s.items {
		if o.TenantID == tenant {
			out = append(out, clone(o))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}
func (s *Store) mutate(id string, change func(*Operation) error) error {
	if s.readOnly {
		return ErrReadOnly
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.items[id]
	if !ok {
		return ErrNotFound
	}
	o = clone(o)
	if o.Status != "running" {
		return errors.New("operation is terminal")
	}
	if e := change(&o); e != nil {
		return e
	}
	if e := s.persist(o); e != nil {
		return e
	}
	s.items[id] = o
	return nil
}
func (s *Store) Advance(id, key string) error {
	return s.mutate(id, func(o *Operation) error {
		index := -1
		for i := range o.Stages {
			if o.Stages[i].Key == key {
				index = i
			}
		}
		if index < 0 {
			return errors.New("unknown operation stage")
		}
		if o.CurrentStage == key {
			return nil
		}
		now := time.Now().UTC()
		for i := 0; i < index; i++ {
			if o.Stages[i].Status == "running" {
				o.Stages[i].Status = "succeeded"
				o.Stages[i].FinishedAt = &now
			} else if o.Stages[i].Status != "succeeded" {
				return errors.New("cannot skip an operation stage")
			}
		}
		if o.Stages[index].Status != "pending" {
			return errors.New("cannot repeat or rewind a stage")
		}
		o.Stages[index].Status = "running"
		o.Stages[index].StartedAt = &now
		o.CurrentStage = key
		appendEvent(o, o.Stages[index].Label)
		return nil
	})
}
func (s *Store) Event(id, message string) error {
	return s.mutate(id, func(o *Operation) error { appendEvent(o, message); return nil })
}
func (s *Store) Finish(id string, executionError error) error {
	return s.mutate(id, func(o *Operation) error {
		now := time.Now().UTC()
		if executionError == nil {
			for _, stage := range o.Stages {
				if stage.Status == "pending" {
					return errors.New("cannot succeed with pending stages")
				}
			}
			o.Status = "succeeded"
		} else {
			o.Status = "failed"
			o.Error = redact.Text(executionError.Error())
		}
		for i := range o.Stages {
			if o.Stages[i].Status == "running" {
				if executionError == nil {
					o.Stages[i].Status = "succeeded"
				} else {
					o.Stages[i].Status = "failed"
				}
				o.Stages[i].FinishedAt = &now
			}
		}
		o.FinishedAt = &now
		if executionError == nil {
			appendEvent(o, "Operation completed and verification passed")
		} else {
			appendEvent(o, "Operation failed: "+o.Error)
		}
		return nil
	})
}

func ReadOnlySnapshot(items []Operation) *Store {
	s := &Store{readOnly: true, items: map[string]Operation{}}
	for _, op := range items {
		s.items[op.ID] = clone(op)
	}
	return s
}
