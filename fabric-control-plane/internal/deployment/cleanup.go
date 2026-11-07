package deployment

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"net/url"
	"sort"
	"strings"
	"time"

	"sathonate.local/fabric-control-plane/internal/cloud"
)

type CleanupRef struct {
	Resource  string `json:"resource"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
	UID       string `json:"uid"`
}
type RetainedRef struct {
	Resource string `json:"resource"`
	Name     string `json:"name"`
	UID      string `json:"uid"`
}
type CleanupPreview struct {
	Schema                       string        `json:"schema"`
	TargetSHA256                 string        `json:"targetSHA256"`
	NetworkSHA256                string        `json:"networkSHA256"`
	BindingSHA256                string        `json:"bindingSHA256"`
	NamespaceUID                 string        `json:"namespaceUID"`
	Digest                       string        `json:"digest"`
	ObservedAt                   time.Time     `json:"observedAt"`
	Delete                       []CleanupRef  `json:"delete"`
	Retain                       []string      `json:"retain"`
	RetainedObjects              []RetainedRef `json:"retainedObjects"`
	RetentionVerified            bool          `json:"retentionVerified"`
	CloudVolumeInventoryVerified bool          `json:"cloudVolumeInventoryVerified"`
	Notice                       string        `json:"notice"`
}
type resourceMetadata struct {
	Metadata struct {
		Name, Namespace, UID string
		Labels               map[string]string
		OwnerReferences      []struct{ UID string } `json:"ownerReferences"`
	}
}

func cleanupNames(request Request) []CleanupRef {
	ns := Namespace(request.TenantID, request.NetworkID)
	out := []CleanupRef{}
	// Stop channel reconciliation before deleting its Fabric participants.
	for _, c := range request.Network.Spec.Channels {
		out = append(out, CleanupRef{Resource: "fabricmainchannels", Name: ns + "-" + c.ID})
		for _, o := range request.Network.Spec.Organizations {
			out = append(out, CleanupRef{Resource: "fabricfollowerchannels", Name: ns + "-" + o.ID + "-" + c.ID})
		}
	}
	out = append(out, CleanupRef{Resource: "fabricchaincodes", Namespace: ns, Name: "smp-smoke"})
	for _, n := range request.Network.Spec.Nodes {
		resource := "fabricpeers"
		if n.Role == "orderer" {
			resource = "fabricorderernodes"
		}
		out = append(out, CleanupRef{Resource: resource, Namespace: ns, Name: n.ID})
	}
	for _, o := range request.Network.Spec.Organizations {
		out = append(out, CleanupRef{Resource: "fabriccas", Namespace: ns, Name: o.ID + "-ca"})
	}
	return out
}
func cleanupOwnership(ref CleanupRef, meta resourceMetadata, request Request) error {
	ns := Namespace(request.TenantID, request.NetworkID)
	if meta.Metadata.Name != ref.Name || meta.Metadata.Namespace != ref.Namespace || meta.Metadata.UID == "" {
		return errors.New("cleanup resource identity is not verified")
	}
	if strings.HasPrefix(ref.Resource, "fabric") {
		if meta.Metadata.Labels["sathonate.io/namespace"] != ns {
			return errors.New("cleanup refuses a Fabric resource without matching ownership label")
		}
	} else if meta.Metadata.Labels["fabric.sathonate.io/network"] != request.NetworkID || meta.Metadata.Labels["fabric.sathonate.io/tenant"] != request.TenantID {
		return errors.New("cleanup refuses foreign job or permission ownership")
	}
	return nil
}
func cleanupDigest(plan CleanupPreview) string {
	plan.Digest = ""
	plan.ObservedAt = time.Time{}
	raw, _ := json.Marshal(plan)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func (r KubernetesRunner) previewCleanup(ctx context.Context, request Request) (CleanupPreview, error) {
	ns := Namespace(request.TenantID, request.NetworkID)
	p := CleanupPreview{Schema: "sathonate.cleanup/v1", ObservedAt: time.Now().UTC(), Delete: []CleanupRef{}, Retain: []string{"namespace", "persistentvolumeclaims", "enrollment-and-wallet-secrets", "helm-release-records", "external-snapshots-and-backups"}, Notice: "Retention is requested, not secure erasure; operator cascading and external storage must be independently verified"}
	if r.eks == nil {
		return p, errors.New("approved target binding required")
	}
	p.TargetSHA256 = cloud.TargetDigest(r.eks.target)
	raw, _ := json.Marshal(request.Network)
	h := sha256.Sum256(raw)
	p.NetworkSHA256 = hex.EncodeToString(h[:])
	raw, _ = json.Marshal([]string{request.TenantID, request.NetworkID, p.TargetSHA256, p.NetworkSHA256})
	h = sha256.Sum256(raw)
	p.BindingSHA256 = hex.EncodeToString(h[:])
	raw, err := r.run(ctx, "get", "namespace", ns, "-o", "json")
	var namespace resourceMetadata
	if err != nil || json.Unmarshal(raw, &namespace) != nil || namespace.Metadata.UID == "" {
		return p, errors.New("namespace identity unavailable")
	}
	if namespace.Metadata.Labels["fabric.sathonate.io/tenant"] != request.TenantID || namespace.Metadata.Labels["fabric.sathonate.io/network"] != request.NetworkID || namespace.Metadata.Labels["fabric.sathonate.io/target"] != r.eks.target.ID {
		return p, errors.New("namespace ownership no longer matches approved binding")
	}
	p.NamespaceUID = namespace.Metadata.UID
	// Print metadata only: never export Secret data, annotations or Helm payloads.
	raw, err = r.run(ctx, "-n", ns, "get", "secrets,persistentvolumeclaims", "-o", `jsonpath={range .items[*]}{.kind}{"\t"}{.metadata.name}{"\t"}{.metadata.uid}{"\n"}{end}`)
	if err != nil {
		return p, errors.New("retained-data metadata inventory unavailable")
	}
	p.RetainedObjects = []RetainedRef{}
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 3 || !safeCleanupName(fields[1]) || fields[2] == "" {
			return p, errors.New("invalid retained-data metadata")
		}
		resource := "secrets"
		if fields[0] == "PersistentVolumeClaim" {
			resource = "persistentvolumeclaims"
		} else if fields[0] != "Secret" {
			return p, errors.New("unexpected retained-data resource")
		}
		p.RetainedObjects = append(p.RetainedObjects, RetainedRef{resource, fields[1], fields[2]})
	}
	sort.Slice(p.RetainedObjects, func(i, j int) bool {
		a, b := p.RetainedObjects[i], p.RetainedObjects[j]
		return a.Resource+"/"+a.Name < b.Resource+"/"+b.Name
	})

	refs := cleanupNames(request)
	for _, resource := range []string{"jobs", "configmaps"} {
		raw, err = r.run(ctx, "-n", ns, "get", resource, "-l", "fabric.sathonate.io/network="+request.NetworkID, "-o", "json")
		var list struct{ Items []resourceMetadata }
		if err != nil || json.Unmarshal(raw, &list) != nil {
			return p, errors.New("cannot inventory owned cleanup resources")
		}
		sort.Slice(list.Items, func(i, j int) bool { return list.Items[i].Metadata.Name < list.Items[j].Metadata.Name })
		for _, v := range list.Items {
			refs = append(refs, CleanupRef{Resource: resource, Namespace: ns, Name: v.Metadata.Name})
		}
	}
	// Revoke only this worker's access after its Jobs and Fabric CRs are removed.
	for _, resource := range []string{"rolebindings", "roles", "serviceaccounts"} {
		refs = append(refs, CleanupRef{Resource: resource, Namespace: ns, Name: "smp-worker"})
	}
	refs = append(refs, CleanupRef{Resource: "clusterrolebindings", Name: ns + "-channels"}, CleanupRef{Resource: "clusterroles", Name: ns + "-channels"})
	for _, ref := range refs {
		args := []string{"get", ref.Resource, ref.Name, "--ignore-not-found", "-o", "json"}
		if ref.Namespace != "" {
			args = append([]string{"-n", ref.Namespace}, args...)
		}
		raw, err = r.run(ctx, args...)
		if err != nil {
			return p, errors.New("cleanup inventory read denied")
		}
		if len(bytes.TrimSpace(raw)) == 0 {
			continue
		}
		var meta resourceMetadata
		if json.Unmarshal(raw, &meta) != nil {
			return p, errors.New("invalid cleanup metadata")
		}
		if err = cleanupOwnership(ref, meta, request); err != nil {
			return p, err
		}
		ref.UID = meta.Metadata.UID
		p.Delete = append(p.Delete, ref)
	}
	p.Digest = cleanupDigest(p)
	return p, nil
}
func (r EKSRunner) cleanupRunner(ctx context.Context, request Request) (KubernetesRunner, error) {
	w, err := r.existing(request.TenantID, request.NetworkID)
	if err != nil {
		return w, err
	}
	if request.TargetSHA256 == "" || request.TargetSHA256 != cloud.TargetDigest(w.eks.target) {
		return w, errors.New("cleanup target binding changed or was not recorded")
	}
	if _, err = cloud.CompileOperator(request.Network, w.eks.target, Namespace(request.TenantID, request.NetworkID)); err != nil {
		return w, errors.New("cleanup desired network differs from approved target")
	}
	if err = w.validateAccess(ctx); err != nil {
		return w, err
	}
	return w, nil
}
func (r EKSRunner) PreviewCleanup(ctx context.Context, request Request) (CleanupPreview, error) {
	w, err := r.cleanupRunner(ctx, request)
	if err != nil {
		return CleanupPreview{}, err
	}
	return w.previewCleanup(ctx, request)
}
func cleanupPath(ref CleanupRef) (string, error) {
	groups := map[string]string{"fabricmainchannels": "/apis/hlf.kungfusoftware.es/v1alpha1", "fabricfollowerchannels": "/apis/hlf.kungfusoftware.es/v1alpha1", "fabricchaincodes": "/apis/hlf.kungfusoftware.es/v1alpha1", "fabricpeers": "/apis/hlf.kungfusoftware.es/v1alpha1", "fabricorderernodes": "/apis/hlf.kungfusoftware.es/v1alpha1", "fabriccas": "/apis/hlf.kungfusoftware.es/v1alpha1", "jobs": "/apis/batch/v1", "configmaps": "/api/v1", "pods": "/api/v1", "serviceaccounts": "/api/v1", "roles": "/apis/rbac.authorization.k8s.io/v1", "rolebindings": "/apis/rbac.authorization.k8s.io/v1", "clusterroles": "/apis/rbac.authorization.k8s.io/v1", "clusterrolebindings": "/apis/rbac.authorization.k8s.io/v1"}
	base, ok := groups[ref.Resource]
	if ref.Resource == "deployments" {
		base = "/apis/apps/v1"
		ok = true
	}
	if !ok || !safeCleanupName(ref.Name) || ref.UID == "" || ref.Namespace != "" && !safeCleanupName(ref.Namespace) {
		return "", errors.New("invalid exact cleanup reference")
	}
	if ref.Namespace != "" {
		base += "/namespaces/" + url.PathEscape(ref.Namespace)
	}
	return base + "/" + ref.Resource + "/" + url.PathEscape(ref.Name), nil
}
func safeCleanupName(value string) bool {
	if value == "" || len(value) > 253 {
		return false
	}
	for _, c := range value {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '.') {
			return false
		}
	}
	return true
}
func (r KubernetesRunner) deleteCleanupRef(ctx context.Context, ref CleanupRef) error {
	path, err := cleanupPath(ref)
	if err != nil {
		return err
	}
	raw, _ := json.Marshal(map[string]any{"apiVersion": "v1", "kind": "DeleteOptions", "propagationPolicy": "Foreground", "preconditions": map[string]string{"uid": ref.UID}})
	cmd := r.command(ctx, "delete", "--raw", path, "-f", "-")
	cmd.Stdin = bytes.NewReader(raw)
	if cmd.Run() != nil {
		return errors.New("conditional cleanup delete failed; re-inventory without bypassing UID preconditions")
	}
	deadline, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		args := []string{"get", ref.Resource, ref.Name, "--ignore-not-found", "-o", "name"}
		if ref.Namespace != "" {
			args = append([]string{"-n", ref.Namespace}, args...)
		}
		observed, e := r.run(deadline, args...)
		if e != nil {
			return errors.New("cleanup resource deletion observation failed")
		}
		if len(bytes.TrimSpace(observed)) == 0 {
			break
		}
		select {
		case <-deadline.Done():
			return errors.New("cleanup resource deletion not yet verified")
		case <-ticker.C:
		}
	}
	return nil
}
func (r EKSRunner) CleanupApproved(ctx context.Context, request Request, digest string) error {
	w, err := r.cleanupRunner(ctx, request)
	if err != nil {
		return err
	}
	plan, err := w.previewCleanup(ctx, request)
	if err != nil {
		return err
	}
	if len(digest) != 64 || digest != plan.Digest {
		return errors.New("current cleanup preview digest must be explicitly approved")
	}
	for _, ref := range plan.Delete {
		if err = w.ownedNamespace(ctx, request.TenantID, request.NetworkID); err != nil {
			return err
		}
		raw, e := w.run(ctx, "get", "namespace", w.eks.target.RunnerNamespace, "-o", "jsonpath={.metadata.uid}")
		if e != nil || strings.TrimSpace(string(raw)) != plan.NamespaceUID {
			return errors.New("namespace was replaced after cleanup approval")
		}
		if err = w.deleteCleanupRef(ctx, ref); err != nil {
			return err
		}
	}
	// Do not claim deletion without observing a fresh empty deletion inventory.
	after, err := w.previewCleanup(ctx, request)
	if err != nil || after.NamespaceUID != plan.NamespaceUID || len(after.Delete) != 0 {
		return errors.New("cleanup verification incomplete; inspect retained resources")
	}
	return nil
}
