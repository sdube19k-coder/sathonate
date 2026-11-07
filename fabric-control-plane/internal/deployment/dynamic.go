package deployment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"sathonate.local/fabric-control-plane/internal/domain"
	"sathonate.local/fabric-control-plane/internal/kubernetesfabric"
	"sathonate.local/fabric-control-plane/internal/operations"
)

type DynamicPreview struct {
	Parent   string             `json:"parentOperationId"`
	Digest   string             `json:"digest"`
	Revision int                `json:"revision"`
	Before   domain.NetworkSpec `json:"before"`
	After    domain.NetworkSpec `json:"after"`
	Recovery string             `json:"recovery"`
}

func (m *Manager) DynamicCapabilities() map[string]any {
	caps := map[string]any{"enabled": false, "developmentOnly": true, "productionEnabled": false}
	r, ok := m.runner.(SecureRunner)
	if !ok || r.binding == nil || r.binding.remote != nil || !r.binding.config.Runtime.DynamicEnabled {
		return caps
	}
	caps["enabled"] = true
	caps["artifacts"] = r.binding.config.Runtime.CodeArtifacts
	caps["placements"] = r.binding.config.Runtime.NodeSelectors
	caps["storageClass"] = r.binding.config.Runtime.StorageClass
	caps["maximumVolumeSize"] = r.binding.config.Runtime.Capacity
	return caps
}

func (m *Manager) PreviewDynamic(ctx context.Context, tenant, actor, parent, record string, desired domain.NetworkSpec, revision int) (DynamicPreview, error) {
	fail := errors.New("a current verified dynamic local deployment, valid additive design and healthy workloads are required")
	if m.history != nil || m.Mode() != "secure-local" || revision < 1 {
		return DynamicPreview{}, fail
	}
	op, e := m.jobs.Get(tenant, parent)
	if e != nil || op.NetworkID != record {
		return DynamicPreview{}, fail
	}
	before, e := m.Status(tenant, parent)
	if e != nil || !before.Verified || !before.Ready || before.Simulated || before.Phase != "running" || before.Network.Spec.Deployment.TargetRef != domain.DynamicLocalProfile || desired.Spec.Deployment.TargetRef != domain.DynamicLocalProfile || kubernetesfabric.ValidateDynamicTransition(before.Network, desired) != nil {
		return DynamicPreview{}, fail
	}
	validator, ok := m.runner.(interface{ Validate(Request) error })
	if !ok || validator.Validate(Request{TenantID: tenant, NetworkID: record, Network: desired, PreviousNetwork: &before.Network, TargetSHA256: before.TargetSHA256}) != nil {
		return DynamicPreview{}, fail
	}
	health, e := m.Health(ctx, tenant, parent)
	if e != nil || !health.Ready {
		return DynamicPreview{}, fail
	}
	raw, _ := json.Marshal([]any{tenant, actor, parent, record, revision, before.Network, before.TargetSHA256, desired})
	sum := sha256.Sum256(raw)
	return DynamicPreview{Parent: parent, Digest: hex.EncodeToString(sum[:]), Revision: revision, Before: before.Network, After: desired, Recovery: "Retry the recorded operation. Committed channel membership and code definitions recover forward; identities, PVCs and previous code services are preserved."}, nil
}

func (m *Manager) ApplyDynamic(ctx context.Context, tenant, actor, parent, record string, desired domain.NetworkSpec, revision int, digest, key string) (operations.Operation, error) {
	if !operations.ValidKey(key) {
		return operations.Operation{}, errors.New("valid idempotency key required")
	}
	for _, op := range m.jobs.List(tenant) {
		if op.IdempotencyKey == key {
			state, e := m.Status(tenant, op.ID)
			if e != nil || op.Actor != actor || op.NetworkID != record || state.LifecycleParent != parent || state.LifecycleAction != "dynamic-reconcile" || state.LifecycleApproval != digest || state.PreviousNetwork == nil || state.Revision != revision {
				return operations.Operation{}, operations.ErrIdempotency
			}
			want, _ := json.Marshal(desired)
			got, _ := json.Marshal(state.Network)
			if string(want) != string(got) {
				return operations.Operation{}, operations.ErrIdempotency
			}
			return op, nil
		}
	}
	preview, e := m.PreviewDynamic(ctx, tenant, actor, parent, record, desired, revision)
	if e != nil {
		return operations.Operation{}, e
	}
	if digest == "" || digest != preview.Digest {
		return operations.Operation{}, errors.New("review and explicitly approve the current dynamic change preview")
	}
	return m.start(tenant, actor, record, desired, revision, key, "", "", nil, parent, "dynamic-reconcile", parent, digest, transitionInput{Previous: &preview.Before})
}
