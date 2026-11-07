package deployment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"sathonate.local/fabric-control-plane/internal/domain"
	"sathonate.local/fabric-control-plane/internal/kubernetesfabric"
	"sathonate.local/fabric-control-plane/internal/operations"
)

const retiredLabel = "fabric.sathonate.io/retired"

type LocalDeletion struct {
	Plan              CleanupPreview `json:"plan"`
	ActorSHA256       string         `json:"actorSHA256"`
	Completed         bool           `json:"completed"`
	RetentionVerified bool           `json:"retentionVerified"`
}

// Retained-data deletion removes compute and execution authority, NOT Fabric
// CRs: their Helm uninstall finalizers can destroy storage. The namespace and
// its data remain fenced until an explicit, separately qualified restore.
func (r SecureRunner) PreviewCleanup(ctx context.Context, q Request) (CleanupPreview, error) {
	p := CleanupPreview{Schema: "sathonate.local-retained-deletion/v1", ObservedAt: time.Now().UTC(), Delete: []CleanupRef{}, RetainedObjects: []RetainedRef{}, Retain: []string{"namespace", "Fabric-CRs-and-channel-configuration", "Secrets-and-Helm-history", "persistentvolumeclaims", "services-configmaps-and-network-isolation"}, Notice: "Remove owned workloads and execution access; permanently fence this namespace against automatic redeployment. Preserve Fabric CRs to avoid Helm uninstall finalizers deleting PVCs. No data erasure or backup is performed."}
	if r.binding == nil || r.binding.remote != nil || q.Network.Spec.Deployment.TargetRef != domain.DynamicLocalProfile || r.Validate(q) != nil {
		return p, errors.New("dynamic local deletion unavailable")
	}
	b := r.base()
	ns := b.namespace(q.TenantID, q.NetworkID)
	if b.validateSecure(ctx) != nil {
		return p, errors.New("local target unavailable")
	}
	var namespace struct {
		Metadata struct {
			UID    string
			Labels map[string]string
		}
	}
	raw, e := b.run(ctx, "get", "namespace", ns, "-o", "json")
	if e != nil || json.Unmarshal(raw, &namespace) != nil || namespace.Metadata.UID == "" || ValidateSecureNamespace(namespace.Metadata.Labels, q.TenantID, "token-bound-project", q.NetworkID) != nil {
		return p, errors.New("namespace ownership unavailable")
	}
	if namespace.Metadata.Labels[coldLabel] != "" {
		return p, errors.New("offline cold maintenance prevents workload deletion")
	}
	p.NamespaceUID = namespace.Metadata.UID
	p.TargetSHA256 = r.BindingDigest(q)
	spec, _ := json.Marshal(q.Network)
	h := sha256.Sum256(spec)
	p.NetworkSHA256 = hex.EncodeToString(h[:])
	binding, _ := json.Marshal([]string{q.TenantID, q.NetworkID, p.TargetSHA256, p.NetworkSHA256})
	h = sha256.Sum256(binding)
	p.BindingSHA256 = hex.EncodeToString(h[:])
	releases := localDeletionReleases(q.Network)
	retainedOwners := map[string][]string{}
	// The controller must disappear before any managed Deployment; it must not
	// race to recreate the resources being retired.
	for _, resource := range []string{"deployments", "jobs", "rolebindings", "roles", "serviceaccounts", "secrets", "persistentvolumeclaims", "fabricpeers", "fabricorderernodes", "fabriccas", "fabricchaincodes"} {
		raw, e = b.run(ctx, "-n", ns, "get", resource, "-o", "json")
		var list struct{ Items []resourceMetadata }
		if e != nil || json.Unmarshal(raw, &list) != nil {
			return p, errors.New("deletion inventory unavailable")
		}
		for _, item := range list.Items {
			m := item.Metadata
			if m.Namespace != ns || m.UID == "" || !safeCleanupName(m.Name) {
				return p, errors.New("invalid deletion resource identity")
			}
			destroy := false
			switch resource {
			case "deployments":
				if !localDeletionWorkloadOwned(q, releases, item) {
					return p, errors.New("foreign workload prevents retained-data deletion")
				}
				destroy = true
			case "jobs":
				if m.Labels["fabric.sathonate.io/tenant"] != q.TenantID || m.Labels["fabric.sathonate.io/network"] != q.NetworkID {
					return p, errors.New("foreign job prevents retained-data deletion")
				}
				destroy = true
			case "rolebindings", "roles", "serviceaccounts":
				if m.Name == "secure-controller" || m.Name == "secure-issuer" {
					if m.Labels["fabric.sathonate.io/tenant"] != q.TenantID || m.Labels["fabric.sathonate.io/network"] != q.NetworkID {
						return p, errors.New("foreign execution permission prevents deletion")
					}
					destroy = true
				}
			}
			if destroy {
				p.Delete = append(p.Delete, CleanupRef{Resource: resource, Namespace: ns, Name: m.Name, UID: m.UID})
			} else {
				for _, owner := range m.OwnerReferences {
					retainedOwners[m.UID] = append(retainedOwners[m.UID], owner.UID)
				}
				p.RetainedObjects = append(p.RetainedObjects, RetainedRef{Resource: resource, Name: m.Name, UID: m.UID})
			}
		}
	}
	sort.Slice(p.Delete, func(i, j int) bool {
		a, b := p.Delete[i], p.Delete[j]
		first := a.Name == "secure-controller" && a.Resource == "deployments"
		second := b.Name == "secure-controller" && b.Resource == "deployments"
		if first != second {
			return first
		}
		return a.Resource+"/"+a.Name < b.Resource+"/"+b.Name
	})
	sort.Slice(p.RetainedObjects, func(i, j int) bool {
		a, b := p.RetainedObjects[i], p.RetainedObjects[j]
		return a.Resource+"/"+a.Name < b.Resource+"/"+b.Name
	})
	for _, ref := range p.Delete {
		for _, owners := range retainedOwners {
			for _, uid := range owners {
				if uid == ref.UID {
					return p, errors.New("retained data has a garbage-collection owner in the deletion plan")
				}
			}
		}
	}
	p.Digest = cleanupDigest(p)
	return p, nil
}
func localDeletionReleases(s domain.NetworkSpec) map[string]bool {
	names := map[string]bool{}
	for _, n := range s.Spec.Nodes {
		names[n.ID] = true
	}
	for _, a := range s.Spec.Configuration.Authorities {
		names[a.ID] = true
	}
	for _, cc := range s.Spec.Configuration.Chaincodes {
		for seq := 1; seq <= cc.Sequence; seq++ {
			names[kubernetesfabric.DynamicService(cc.ChannelRef, cc.Name, seq)] = true
		}
	}
	return names
}
func localDeletionWorkloadOwned(q Request, releases map[string]bool, m resourceMetadata) bool {
	meta := m.Metadata
	if meta.Name == "secure-controller" {
		return meta.Labels["fabric.sathonate.io/tenant"] == q.TenantID && meta.Labels["fabric.sathonate.io/network"] == q.NetworkID
	}
	return releases[meta.Name] && (meta.Labels["release"] == meta.Name || meta.Labels["chaincode"] == meta.Name)
}
func (r SecureRunner) deleteRetainingData(ctx context.Context, q Request, plan CleanupPreview) error {
	if plan.Schema != "sathonate.local-retained-deletion/v1" || plan.Digest != cleanupDigest(plan) || plan.TargetSHA256 != r.BindingDigest(q) {
		return errors.New("invalid retained deletion intent")
	}
	b := r.base()
	ns := b.namespace(q.TenantID, q.NetworkID)
	checkNamespace := func() (map[string]string, string, error) {
		if b.validateSecure(ctx) != nil {
			return nil, "", errors.New("target ownership unavailable")
		}
		raw, e := b.run(ctx, "get", "namespace", ns, "-o", "json")
		var obj struct {
			Metadata struct {
				UID, ResourceVersion string
				Labels               map[string]string
			}
		}
		if e != nil || json.Unmarshal(raw, &obj) != nil || obj.Metadata.UID != plan.NamespaceUID || ValidateSecureNamespace(obj.Metadata.Labels, q.TenantID, "token-bound-project", q.NetworkID) != nil {
			return nil, "", errors.New("namespace replaced during deletion")
		}
		return obj.Metadata.Labels, obj.Metadata.ResourceVersion, nil
	}
	labels, version, e := checkNamespace()
	if e != nil {
		return e
	}
	if existing := labels[retiredLabel]; existing != "" && existing != plan.Digest[:60] {
		return errors.New("different retirement intent owns namespace")
	}
	if labels[retiredLabel] == "" {
		labels[retiredLabel] = plan.Digest[:60]
		patch, _ := json.Marshal([]map[string]any{{"op": "test", "path": "/metadata/uid", "value": plan.NamespaceUID}, {"op": "test", "path": "/metadata/resourceVersion", "value": version}, {"op": "add", "path": "/metadata/labels", "value": labels}})
		if _, e = b.run(ctx, "patch", "namespace", ns, "--type=json", "-p", string(patch)); e != nil {
			return errors.New("namespace fence not acknowledged; retry recorded deletion")
		}
	}
	for _, ref := range plan.Delete {
		if _, _, e = checkNamespace(); e != nil {
			return e
		}
		raw, err := b.run(ctx, "-n", ns, "get", ref.Resource, ref.Name, "--ignore-not-found", "-o", "json")
		if err != nil {
			return errors.New("deletion observation unavailable")
		}
		if len(strings.TrimSpace(string(raw))) == 0 {
			continue
		}
		var meta resourceMetadata
		if json.Unmarshal(raw, &meta) != nil || meta.Metadata.UID != ref.UID {
			return errors.New("resource replaced; deletion refused")
		}
		if e = b.deleteCleanupRef(ctx, ref); e != nil {
			return e
		}
	}
	for _, keep := range plan.RetainedObjects {
		raw, e := b.run(ctx, "-n", ns, "get", keep.Resource, keep.Name, "-o", "json")
		var meta resourceMetadata
		if e != nil || json.Unmarshal(raw, &meta) != nil || meta.Metadata.UID != keep.UID {
			return errors.New("retained object preservation not verified")
		}
	}
	after, e := r.PreviewCleanup(ctx, q)
	if e != nil || len(after.Delete) != 0 || after.NamespaceUID != plan.NamespaceUID {
		return errors.New("workload removal verification incomplete")
	}
	return nil
}
func (m *Manager) deleteLocal(ctx context.Context, tenant, id, actor, digest string, s Result, op operations.Operation) error {
	r, ok := m.runner.(interface {
		PreviewCleanup(context.Context, Request) (CleanupPreview, error)
		deleteRetainingData(context.Context, Request, CleanupPreview) error
	})
	if !ok || actor == "" || len(digest) != 64 {
		return errors.New("administrator current deletion preview approval required")
	}
	q := Request{TenantID: tenant, NetworkID: op.NetworkID, OperationID: id, Network: s.Network, TargetSHA256: s.TargetSHA256}
	if s.LocalDeletion == nil {
		plan, e := r.PreviewCleanup(ctx, q)
		if e != nil {
			return e
		}
		if plan.Digest != digest {
			return errors.New("deletion preview changed; review again")
		}
		sum := sha256.Sum256([]byte(actor))
		s.LocalDeletion = &LocalDeletion{Plan: plan, ActorSHA256: hex.EncodeToString(sum[:])}
		s.TenantID = tenant
		s.Phase = "retiring"
		s.Ready = false
		s.Verified = false
		if e = m.write(s); e != nil {
			return e
		}
	} else {
		sum := sha256.Sum256([]byte(actor))
		if s.LocalDeletion.Plan.Digest != digest || s.LocalDeletion.ActorSHA256 != hex.EncodeToString(sum[:]) {
			return errors.New("deletion recovery approval/actor mismatch")
		}
	}
	if s.LocalDeletion.Completed {
		return nil
	}
	if e := r.deleteRetainingData(ctx, q, s.LocalDeletion.Plan); e != nil {
		return e
	}
	s.LocalDeletion.Completed = true
	s.TenantID = tenant
	s.Phase = "removed"
	s.Ready = false
	s.Verified = false
	s.LocalDeletion.RetentionVerified = true
	// Preserve the approval digest; retentionVerified is an outcome, not a new approval.
	s.Notice = "Workloads and execution permissions removed. Namespace, Fabric CRs, identities, Helm history and PVC UIDs preserved. Data erasure and restart remain disabled."
	for i := range s.Resources {
		s.Resources[i].Status = "removed"
	}
	if s.Health != nil {
		s.Health.Ready = false
	}
	return m.write(s)
}
