package deployment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

func (r SecureEKSRunner) PreviewCleanup(ctx context.Context, q Request) (CleanupPreview, error) {
	p := CleanupPreview{Schema: "sathonate.secure-eks-cleanup/v1", ObservedAt: time.Now().UTC(), Delete: []CleanupRef{}, Retain: []string{"namespace", "secrets", "persistentvolumeclaims", "external-backups-and-AWS-infrastructure"}, Notice: "Exact workload cleanup only; namespace/data/cloud resources require separately verified teardown. Operator cascading retention is not assumed."}
	execution, e := r.execution(q, false)
	if e != nil {
		return p, e
	}
	if e = r.secure.owned(ctx, q.TenantID, q.NetworkID); e != nil {
		return p, e
	}
	b := r.secure.base()
	ns := b.namespace(q.TenantID, q.NetworkID)
	p.TargetSHA256 = r.BindingDigest(q)
	p.NetworkSHA256 = r.secure.binding.remote.config.NetworkSHA256
	h := sha256.Sum256([]byte(q.TenantID + "/" + q.NetworkID + "/" + p.TargetSHA256 + "/" + p.NetworkSHA256))
	p.BindingSHA256 = hex.EncodeToString(h[:])
	raw, e := b.run(ctx, "get", "namespace", ns, "-o", "jsonpath={.metadata.uid}")
	if e != nil || len(raw) == 0 {
		return p, errSecureEKS
	}
	p.NamespaceUID = strings.TrimSpace(string(raw))
	name, e := jobName(q.OperationID)
	if e != nil {
		return p, e
	}
	refs := []CleanupRef{{Resource: "jobs", Namespace: ns, Name: name}, {Resource: "fabricchaincodes", Namespace: ns, Name: "smp-smoke"}}
	for _, node := range execution.Network.Spec.Nodes {
		kind := "fabricpeers"
		if node.Role == "orderer" {
			kind = "fabricorderernodes"
		}
		refs = append(refs, CleanupRef{Resource: kind, Namespace: ns, Name: node.ID})
	}
	for _, org := range execution.Network.Spec.Organizations {
		refs = append(refs, CleanupRef{Resource: "fabriccas", Namespace: ns, Name: org.ID + "-ca"})
	}
	refs = append(refs, CleanupRef{Resource: "deployments", Namespace: ns, Name: "secure-controller"})
	for _, kind := range []string{"rolebindings", "roles", "serviceaccounts"} {
		for _, name := range []string{"secure-controller", "secure-issuer"} {
			refs = append(refs, CleanupRef{Resource: kind, Namespace: ns, Name: name})
		}
	}
	for _, ref := range refs {
		raw, e = b.run(ctx, "-n", ns, "get", ref.Resource, ref.Name, "--ignore-not-found", "-o", "json")
		if e != nil {
			return p, e
		}
		if len(strings.TrimSpace(string(raw))) == 0 {
			continue
		}
		var meta resourceMetadata
		if json.Unmarshal(raw, &meta) != nil || meta.Metadata.Name != ref.Name || meta.Metadata.Namespace != ns || meta.Metadata.UID == "" {
			return p, errSecureEKS
		}
		labels := meta.Metadata.Labels
		if strings.HasPrefix(ref.Resource, "fabric") {
			if labels["sathonate.io/namespace"] != ns {
				return p, errSecureEKS
			}
		} else if labels["fabric.sathonate.io/tenant"] != q.TenantID || labels["fabric.sathonate.io/network"] != q.NetworkID {
			return p, errSecureEKS
		}
		ref.UID = meta.Metadata.UID
		p.Delete = append(p.Delete, ref)
	}
	p.Digest = cleanupDigest(p)
	return p, nil
}

func (r SecureEKSRunner) CleanupApproved(ctx context.Context, q Request, digest string) error {
	plan, e := r.PreviewCleanup(ctx, q)
	if e != nil {
		return e
	}
	if len(digest) != 64 || plan.Digest != digest {
		return errors.New("secure EKS cleanup requires the current exact preview approval")
	}
	b := r.secure.base()
	ns := b.namespace(q.TenantID, q.NetworkID)
	for _, ref := range plan.Delete {
		if e = r.secure.owned(ctx, q.TenantID, q.NetworkID); e != nil {
			return e
		}
		uid, e := b.run(ctx, "get", "namespace", ns, "-o", "jsonpath={.metadata.uid}")
		if e != nil || strings.TrimSpace(string(uid)) != plan.NamespaceUID {
			return errSecureEKS
		}
		if e = b.deleteCleanupRef(ctx, ref); e != nil {
			return e
		}
	}
	after, e := r.PreviewCleanup(ctx, q)
	if e != nil || after.NamespaceUID != plan.NamespaceUID || len(after.Delete) != 0 {
		return errors.New("secure EKS workload cleanup remains unverified")
	}
	return nil
}
