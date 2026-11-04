package deployment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sathonate.local/fabric-control-plane/internal/kubernetesfabric"
	"sathonate.local/fabric-control-plane/internal/operations"
)

type ChangePreview struct {
	Parent            string                          `json:"parentOperationId"`
	Action            string                          `json:"action"`
	Digest            string                          `json:"digest"`
	Before            kubernetesfabric.LifecycleState `json:"before"`
	After             kubernetesfabric.LifecycleState `json:"after"`
	Impact            []string                        `json:"impact"`
	Recovery          string                          `json:"recovery"`
	LiveHealthChecked bool                            `json:"liveHealthChecked"`
}

func nextLifecycle(previous *kubernetesfabric.LifecycleState, action string) (kubernetesfabric.LifecycleState, []string, error) {
	state := kubernetesfabric.LifecycleState{Sequence: 1}
	if previous != nil {
		state = *previous
	}
	if state.Validate() != nil {
		return state, nil, errors.New("invalid recorded lifecycle state")
	}
	switch action {
	case "verify-network":
		return state, []string{"Reconcile the current approved member set without changing membership", "Submit a NEW qualification Put and verify Get and equal ledger state on every managed peer"}, nil
	case "restart-peer":
		return state, []string{"Restart org1-peer-1 through an owned UID-checked pod-template update", "Keep identities and persistent claims; wait for rollout, then commit and query a new transaction on every managed peer"}, nil
	case "add-peer":
		if state.PeerAdded {
			return state, nil, errors.New("org1-peer-2 already belongs to the managed topology")
		}
		state.PeerAdded = true
		return state, []string{"Enroll independent org1-peer-2 signing/TLS identities using Org1 CA", "Create one peer and persistent claim in the owned namespace", "Join smpchannel, install the approved package and verify synchronized ledgers on three peers", "Existing Raft membership, organization membership and endorsement policy stay unchanged"}, nil
	case "upgrade-chaincode":
		if state.Sequence != 1 {
			return state, nil, errors.New("only sequence 1 to 2 upgrade is currently supported")
		}
		state.Sequence = 2
		return state, []string{"Approve smp_smoke version 2.0 sequence 2 with both organization administrators", "Check commit readiness and commit with Org1 AND Org2 endorsements", "Retain the qualified package/image and existing ledger; this upgrades the definition, not arbitrary code", "Verify a new committed transaction on every managed peer"}, nil
	default:
		return state, nil, errors.New("unsupported lifecycle action; organization/Raft/configuration changes are not enabled")
	}
}

func (m *Manager) PreviewLifecycle(ctx context.Context, tenant, actor, parent, action string) (ChangePreview, error) {
	fail := errors.New("a current verified protected local deployment and healthy workloads are required")
	if m.history != nil || m.Mode() != "secure-local" {
		return ChangePreview{}, fail
	}
	before, e := m.Status(tenant, parent)
	if e != nil || !before.Verified || !before.Ready || before.Phase != "running" || before.Simulated {
		return ChangePreview{}, fail
	}
	after, impact, e := nextLifecycle(before.Lifecycle, action)
	if e != nil {
		return ChangePreview{}, e
	}
	health, e := m.Health(ctx, tenant, parent)
	if e != nil || !health.Ready {
		return ChangePreview{}, fail
	}
	if preflight, ok := m.runner.(interface {
		LifecyclePreflight(context.Context, Request, string) error
	}); ok {
		op, e := m.jobs.Get(tenant, parent)
		if e != nil {
			return ChangePreview{}, fail
		}
		if e = preflight.LifecyclePreflight(ctx, Request{TenantID: tenant, NetworkID: op.NetworkID, Network: before.Network}, action); e != nil {
			return ChangePreview{}, e
		}
	}
	state := kubernetesfabric.LifecycleState{Sequence: 1}
	if before.Lifecycle != nil {
		state = *before.Lifecycle
	}
	raw, _ := json.Marshal([]any{tenant, actor, parent, action, before.Network, before.TargetSHA256, state, after})
	sum := sha256.Sum256(raw)
	return ChangePreview{Parent: parent, Action: action, Digest: hex.EncodeToString(sum[:]), Before: state, After: after, Impact: impact, Recovery: "Retry the recorded operation to reconcile the same identities and resources. Do not delete PVCs or roll back committed Fabric definitions.", LiveHealthChecked: true}, nil
}

func (m *Manager) ApplyLifecycle(ctx context.Context, tenant, actor, parent, action, digest, key string) (operations.Operation, error) {
	if key == "" || !operations.ValidKey(key) {
		return operations.Operation{}, errors.New("valid idempotency key required")
	}
	// Resolve a retried HTTP acknowledgement before requiring the superseded
	// parent to remain current. Never reuse a key for another action or actor.
	for _, op := range m.jobs.List(tenant) {
		if op.IdempotencyKey == key {
			state, e := m.Status(tenant, op.ID)
			if e != nil || op.Actor != actor || state.LifecycleParent != parent || state.LifecycleAction != action || state.LifecycleApproval != digest {
				return operations.Operation{}, operations.ErrIdempotency
			}
			return op, nil
		}
	}
	preview, e := m.PreviewLifecycle(ctx, tenant, actor, parent, action)
	if e != nil {
		return operations.Operation{}, e
	}
	if digest == "" || digest != preview.Digest {
		return operations.Operation{}, errors.New("review and explicitly approve the current change preview")
	}
	previous, e := m.Status(tenant, parent)
	if e != nil {
		return operations.Operation{}, e
	}
	op, e := m.start(tenant, actor, mustNetwork(m, tenant, parent), previous.Network, previous.Revision, key, "", "", &preview.After, parent, action, parent, digest)
	return op, e
}
func mustNetwork(m *Manager, tenant, id string) string {
	op, _ := m.jobs.Get(tenant, id)
	return op.NetworkID
}
