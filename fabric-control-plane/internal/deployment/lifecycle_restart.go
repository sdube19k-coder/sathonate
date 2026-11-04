package deployment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"regexp"
)

func (r SecureRunner) LifecyclePreflight(ctx context.Context, q Request, action string) error {
	if e := r.owned(ctx, q.TenantID, q.NetworkID); e != nil {
		return e
	}
	if action == "add-peer" {
		ns := r.base().namespace(q.TenantID, q.NetworkID)
		raw, e := r.base().run(ctx, "-n", ns, "get", "fabricpeers", "org1-peer-2", "--ignore-not-found", "-o", "json")
		if e != nil || len(bytes.TrimSpace(raw)) != 0 {
			return errors.New("new peer name is already occupied or cannot be safely inspected")
		}
	}
	return nil
}

func (r SecureRunner) restartPeer(ctx context.Context, q Request, ns string) error {
	denied := errors.New("owned peer restart preflight failed")
	if q.Lifecycle == nil || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(q.LifecycleApproval) {
		return denied
	}
	if r.owned(ctx, q.TenantID, q.NetworkID) != nil {
		return denied
	}
	b := r.base()
	raw, e := b.run(ctx, "-n", ns, "get", "fabricpeers", "org1-peer-1", "-o", "json")
	var cr struct {
		Metadata struct{ Labels map[string]string }
	}
	if e != nil || json.Unmarshal(raw, &cr) != nil || cr.Metadata.Labels["sathonate.io/namespace"] != ns {
		return denied
	}
	raw, e = b.run(ctx, "-n", ns, "get", "deployment", "org1-peer-1", "-o", "json")
	var deployment struct {
		Metadata struct{ UID string }
		Spec     struct {
			Template struct {
				Metadata struct{ Annotations map[string]string }
			}
		}
	}
	if e != nil || json.Unmarshal(raw, &deployment) != nil || deployment.Metadata.UID == "" {
		return denied
	}
	annotations := deployment.Spec.Template.Metadata.Annotations
	if annotations == nil {
		annotations = map[string]string{}
	}
	// The immutable approval digest is stable across controller adoption/retry.
	// An ambiguous patch acknowledgement therefore does not trigger a new restart.
	if annotations["fabric.sathonate.io/restart"] != q.LifecycleApproval {
		annotations["fabric.sathonate.io/restart"] = q.LifecycleApproval
		patch, _ := json.Marshal([]map[string]any{{"op": "test", "path": "/metadata/uid", "value": deployment.Metadata.UID}, {"op": "add", "path": "/spec/template/metadata/annotations", "value": annotations}})
		if _, e = b.run(ctx, "-n", ns, "patch", "deployment", "org1-peer-1", "--type=json", "-p", string(patch)); e != nil {
			return denied
		}
	}
	_, e = b.run(ctx, "-n", ns, "rollout", "status", "deployment/org1-peer-1", "--timeout=180s")
	return e
}
