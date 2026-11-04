// Package engine defines the future execution boundary; no adapters execute yet.
package engine

import (
	"context"
	"errors"
)

var ErrNotImplemented = errors.New("deployment execution is not implemented")

// Execution is a future authorized, digest-bound request. The control plane must
// validate tenant ownership, live lease, revision, signature and artifacts before dispatch.
type Execution struct {
	TenantID         string
	NetworkID        string
	OperationID      string
	TargetID         string
	PlanDigest       string
	LeaseFence       uint64
	ExpectedRevision uint64
}

type Observation struct {
	ResourceID  string
	State       string
	EvidenceRef string
}

type Adapter interface {
	Name() string
	Observe(context.Context, Execution) ([]Observation, error)
	Apply(context.Context, Execution) error
	Verify(context.Context, Execution) ([]Observation, error)
}
