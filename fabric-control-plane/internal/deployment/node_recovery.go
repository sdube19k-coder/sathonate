package deployment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strconv"

	"sathonate.local/fabric-control-plane/internal/domain"
	"sathonate.local/fabric-control-plane/internal/operations"
)

type transitionInput struct {
	Previous *domain.NetworkSpec
	Recovery *NodeRecovery
}
type materialVersion struct {
	UID     string `json:"uid"`
	Version string `json:"version"`
}
type NodeRecovery struct {
	Node          string                     `json:"node"`
	Kind          string                     `json:"kind"`
	DeploymentUID string                     `json:"deploymentUID,omitempty"`
	ResourceUID   string                     `json:"resourceUID,omitempty"`
	TemplateHash  string                     `json:"templateHash,omitempty"`
	Replicas      int                        `json:"replicas"`
	PodUIDs       []string                   `json:"podUIDs"`
	Claims        map[string]string          `json:"claims"`
	Materials     map[string]materialVersion `json:"materials"`
	Digest        string                     `json:"digest"`
}
type RecoveryPreview struct {
	Parent   string       `json:"parentOperationId"`
	Plan     NodeRecovery `json:"plan"`
	Impact   string       `json:"impact"`
	Recovery string       `json:"recovery"`
}

func (m *Manager) PreviewNodeRecovery(ctx context.Context, tenant, actor, parent, node string) (RecoveryPreview, error) {
	fail := errors.New("a current verified dynamic local intent and intact owned resources/storage/material are required")
	if m.history != nil || m.Mode() != "secure-local" {
		return RecoveryPreview{}, fail
	}
	op, e := m.jobs.Get(tenant, parent)
	if e != nil {
		return RecoveryPreview{}, e
	}
	before, e := m.Status(tenant, parent)
	if e != nil || !before.Verified || before.Phase != "running" || before.Network.Spec.Deployment.TargetRef != domain.DynamicLocalProfile {
		return RecoveryPreview{}, fail
	}
	r, ok := m.runner.(interface {
		RecoveryPreflight(context.Context, Request, string) (NodeRecovery, error)
	})
	if !ok {
		return RecoveryPreview{}, fail
	}
	plan, e := r.RecoveryPreflight(ctx, Request{TenantID: tenant, NetworkID: op.NetworkID, Network: before.Network, TargetSHA256: before.TargetSHA256}, node)
	if e != nil {
		return RecoveryPreview{}, e
	}
	raw, _ := json.Marshal([]any{tenant, actor, parent, op.NetworkID, before.Revision, before.TargetSHA256, before.Network, plan})
	h := sha256.Sum256(raw)
	plan.Digest = hex.EncodeToString(h[:])
	return RecoveryPreview{Parent: parent, Plan: plan, Impact: "Restart only the selected owned node (or reconcile without a restart). Restore its approved single replica; preserve identities and PVCs. Endorsement/enrollment may pause briefly.", Recovery: "The immutable approval is reused after retry/restart. Committed configuration recovers forward. Verify every channel with NEW committed transactions; never delete a PVC to repair storage."}, nil
}
func (m *Manager) ApplyNodeRecovery(ctx context.Context, tenant, actor, parent, node, digest, key string) (operations.Operation, error) {
	if !operations.ValidKey(key) {
		return operations.Operation{}, operations.ErrIdempotency
	}
	for _, op := range m.jobs.List(tenant) {
		if op.IdempotencyKey == key {
			s, e := m.Status(tenant, op.ID)
			if e != nil || op.Actor != actor || s.Recovery == nil || s.Recovery.Node != node || s.Recovery.Digest != digest || s.LifecycleParent != parent {
				return operations.Operation{}, operations.ErrIdempotency
			}
			return op, nil
		}
	}
	p, e := m.PreviewNodeRecovery(ctx, tenant, actor, parent, node)
	if e != nil {
		return operations.Operation{}, e
	}
	if digest == "" || digest != p.Plan.Digest {
		return operations.Operation{}, errors.New("recovery preview changed; review the current node and approve again")
	}
	before, e := m.Status(tenant, parent)
	if e != nil {
		return operations.Operation{}, e
	}
	return m.start(tenant, actor, mustNetwork(m, tenant, parent), before.Network, before.Revision, key, "", "", nil, parent, "node-recovery", parent, digest, transitionInput{Previous: &before.Network, Recovery: &p.Plan})
}

const recoveryAnnotation = "fabric.sathonate.io/node-recovery"

type nodeState struct {
	Plan        NodeRecovery
	Version     string
	Annotations map[string]string
}

func (r SecureRunner) RecoveryPreflight(ctx context.Context, q Request, node string) (NodeRecovery, error) {
	s, e := r.nodeState(ctx, q, node)
	return s.Plan, e
}
func (r SecureRunner) nodeState(ctx context.Context, q Request, node string) (nodeState, error) {
	fail := errors.New("recovery blocked: owned UID, approved image, quorum, bound PVCs and available credential material must be intact")
	if r.binding == nil || r.binding.remote != nil || r.Validate(q) != nil || q.Network.Spec.Configuration == nil || r.owned(ctx, q.TenantID, q.NetworkID) != nil {
		return nodeState{}, fail
	}
	s := nodeState{Plan: NodeRecovery{Node: node, Kind: "reconcile", PodUIDs: []string{}, Claims: map[string]string{}, Materials: map[string]materialVersion{}}}
	if node == "" {
		return s, nil
	}
	kind, resource := "", ""
	for _, n := range q.Network.Spec.Nodes {
		if n.ID == node {
			kind = n.Role
			resource = "fabricpeers"
			if kind == "orderer" {
				resource = "fabricorderernodes"
			}
		}
	}
	for _, a := range q.Network.Spec.Configuration.Authorities {
		if a.ID == node {
			kind = "authority"
			resource = "fabriccas"
		}
	}
	if resource == "" {
		return s, fail
	}
	s.Plan.Kind = kind
	b := r.base()
	ns := b.namespace(q.TenantID, q.NetworkID)
	get := func(resource, name string, v any) bool {
		raw, e := b.run(ctx, "-n", ns, "get", resource, name, "-o", "json")
		return e == nil && json.Unmarshal(raw, v) == nil
	}
	var cr struct {
		Metadata struct {
			UID    string
			Labels map[string]string
		}
	}
	if !get(resource, node, &cr) || cr.Metadata.UID == "" || cr.Metadata.Labels["sathonate.io/namespace"] != ns {
		return s, fail
	}
	s.Plan.ResourceUID = cr.Metadata.UID
	var d struct {
		Metadata struct {
			UID, ResourceVersion, Namespace string
			Labels, Annotations             map[string]string
		}
		Spec struct {
			Replicas int
			Template json.RawMessage
		}
	}
	var templateView struct {
		Metadata struct{ Annotations map[string]string }
		Spec     struct {
			Containers []struct{ Image string }
			Volumes    []struct {
				Secret                *struct{ SecretName string }
				PersistentVolumeClaim *struct{ ClaimName string }
			}
		}
	}
	if !get("deployment", node, &d) || d.Metadata.UID == "" || !recoveryDeploymentOwned(ns, node, d.Metadata.Namespace, d.Metadata.Labels, d.Metadata.Annotations) || d.Spec.Replicas < 0 || d.Spec.Replicas > 1 {
		return s, fail
	}
	// The qualified Helm client leaves release-namespace empty on rendered
	// objects. Require the actual scoped namespace and a deployed, named Helm
	// storage record, never an absent annotation as proof of ownership.
	rawReleases, e := b.run(ctx, "-n", ns, "get", "secrets", "-l", "owner=helm,name="+node, "-o", "json")
	var releases struct {
		Items []struct {
			Metadata struct {
				Name, Namespace, UID, ResourceVersion string
				Labels                                map[string]string
			}
			Type string
		}
	}
	if e != nil || json.Unmarshal(rawReleases, &releases) != nil {
		return s, errors.New("recovery blocked: Helm ownership inventory unavailable")
	}
	best := 0
	for _, item := range releases.Items {
		m := item.Metadata
		v, err := strconv.Atoi(m.Labels["version"])
		if err == nil && v > best && m.Namespace == ns && m.UID != "" && m.Labels["owner"] == "helm" && m.Labels["name"] == node && m.Labels["status"] == "deployed" && item.Type == "helm.sh/release.v1" && m.Name == "sh.helm.release.v1."+node+".v"+strconv.Itoa(v) {
			best = v
		}
	}
	if best == 0 {
		return s, errors.New("recovery blocked: deployed Helm ownership record missing")
	}
	if json.Unmarshal(d.Spec.Template, &templateView) != nil {
		return s, fail
	}
	imageKind := map[string]string{"peer": "FabricPeer", "orderer": "FabricOrdererNode", "authority": "FabricCA"}[kind]
	if len(templateView.Spec.Containers) != 1 || templateView.Spec.Containers[0].Image != r.binding.config.Runtime.Images[imageKind] {
		return s, fail
	}
	s.Plan.DeploymentUID = d.Metadata.UID
	s.Plan.Replicas = d.Spec.Replicas
	s.Version = d.Metadata.ResourceVersion
	s.Annotations = templateView.Metadata.Annotations
	if s.Annotations == nil {
		s.Annotations = map[string]string{}
	}
	hash, e := recoveryTemplateHash(d.Spec.Template)
	if e != nil {
		return s, fail
	}
	s.Plan.TemplateHash = hash
	ordererConfig := map[string]string{}
	if kind == "orderer" {
		var config struct {
			Metadata struct{ UID, ResourceVersion string }
			Data     map[string]string
		}
		if !get("configmap", node+"--ord", &config) || config.Metadata.UID == "" {
			return s, errors.New("recovery blocked: orderer TLS configuration unavailable")
		}
		ordererConfig = config.Data
		s.Plan.Materials["configmap:"+node+"--ord"] = materialVersion{UID: config.Metadata.UID, Version: config.Metadata.ResourceVersion}
	}
	for _, v := range templateView.Spec.Volumes {
		if v.PersistentVolumeClaim != nil {
			var pvc struct {
				Metadata struct{ UID string }
				Status   struct{ Phase string }
			}
			name := v.PersistentVolumeClaim.ClaimName
			if !get("pvc", name, &pvc) || pvc.Metadata.UID == "" || pvc.Status.Phase != "Bound" {
				return s, fail
			}
			s.Plan.Claims[name] = pvc.Metadata.UID
		}
		if v.Secret != nil {
			var secret struct {
				Metadata struct{ UID, ResourceVersion string }
				Data     map[string]string
			}
			name := v.Secret.SecretName
			if !get("secret", name, &secret) || secret.Metadata.UID == "" || (len(secret.Data) == 0 && !optionalRecoveryMaterial(kind, node, name, ordererConfig)) {
				return s, errors.New("recovery blocked: required credential material missing or empty")
			}
			s.Plan.Materials[name] = materialVersion{UID: secret.Metadata.UID, Version: secret.Metadata.ResourceVersion}
		}
	}
	if len(s.Plan.Claims) == 0 || len(s.Plan.Materials) == 0 {
		return s, fail
	}
	raw, e := b.run(ctx, "-n", ns, "get", "pods", "-l", "release="+node, "-o", "json")
	var pods struct {
		Items []struct{ Metadata struct{ UID string } }
	}
	if e != nil || json.Unmarshal(raw, &pods) != nil {
		return s, fail
	}
	for _, p := range pods.Items {
		s.Plan.PodUIDs = append(s.Plan.PodUIDs, p.Metadata.UID)
	}
	sort.Strings(s.Plan.PodUIDs)
	if kind == "orderer" {
		for _, c := range q.Network.Spec.Channels {
			member := false
			for _, id := range c.OrdererRefs {
				member = member || id == node
			}
			if !member {
				continue
			}
			ready := 0
			for _, id := range c.OrdererRefs {
				if id == node {
					continue
				}
				var dep struct {
					Metadata struct{ Generation int64 }
					Spec     struct{ Replicas int }
					Status   struct {
						ReadyReplicas, AvailableReplicas int
						ObservedGeneration               int64
					}
				}
				if !get("deployment", id, &dep) {
					return s, fail
				}
				if dep.Spec.Replicas == 1 && dep.Status.ReadyReplicas == 1 && dep.Status.AvailableReplicas == 1 && dep.Status.ObservedGeneration >= dep.Metadata.Generation {
					ready++
				}
			}
			if ready < len(c.OrdererRefs)/2+1 {
				return s, fail
			}
		}
	}
	return s, nil
}
func sameRecoveryResource(a, b NodeRecovery, already bool) bool {
	if a.Node != b.Node || a.Kind != b.Kind || a.DeploymentUID != b.DeploymentUID || a.ResourceUID != b.ResourceUID || a.TemplateHash != b.TemplateHash || !reflect.DeepEqual(a.Claims, b.Claims) || !reflect.DeepEqual(a.Materials, b.Materials) {
		return false
	}
	return already || (a.Replicas == b.Replicas && reflect.DeepEqual(a.PodUIDs, b.PodUIDs))
}
func (r SecureRunner) recoverNode(ctx context.Context, q Request, ns string) error {
	if q.Recovery == nil {
		return ErrWorkloadsUnavailable
	}
	if q.Recovery.Node == "" {
		return nil
	}
	state, e := r.nodeState(ctx, q, q.Recovery.Node)
	if e != nil {
		return e
	}
	already := state.Annotations[recoveryAnnotation] == q.Recovery.Digest
	if !sameRecoveryResource(*q.Recovery, state.Plan, already) {
		return errors.New("recovery resource or material changed; no mutation performed")
	}
	if !already {
		state.Annotations[recoveryAnnotation] = q.Recovery.Digest
		patch, _ := json.Marshal([]map[string]any{{"op": "test", "path": "/metadata/uid", "value": state.Plan.DeploymentUID}, {"op": "test", "path": "/metadata/resourceVersion", "value": state.Version}, {"op": "add", "path": "/spec/template/metadata/annotations", "value": state.Annotations}, {"op": "add", "path": "/spec/replicas", "value": 1}})
		if _, e = r.base().run(ctx, "-n", ns, "patch", "deployment", q.Recovery.Node, "--type=json", "-p", string(patch)); e != nil {
			return errors.New("owned recovery patch not acknowledged; retry the recorded operation")
		}
	}
	_, e = r.base().run(ctx, "-n", ns, "rollout", "status", "deployment/"+q.Recovery.Node, "--timeout=180s")
	return e
}

func recoveryTemplateHash(raw []byte) (string, error) {
	var template map[string]any
	if json.Unmarshal(raw, &template) != nil {
		return "", ErrWorkloadsUnavailable
	}
	meta, ok := template["metadata"].(map[string]any)
	if !ok {
		return "", ErrWorkloadsUnavailable
	}
	if annotations, ok := meta["annotations"].(map[string]any); ok {
		delete(annotations, recoveryAnnotation)
		if len(annotations) == 0 {
			delete(meta, "annotations")
		}
	}
	data, e := json.Marshal(template)
	if e != nil {
		return "", e
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

func recoveryDeploymentOwned(namespace, node, actualNamespace string, labels, annotations map[string]string) bool {
	helmNamespace := annotations["meta.helm.sh/release-namespace"]
	return actualNamespace == namespace && labels["release"] == node && labels["app.kubernetes.io/managed-by"] == "Helm" && annotations["meta.helm.sh/release-name"] == node && (helmNamespace == namespace || helmNamespace == "")
}

// Host-side recovery precedes worker identity/Kubernetes stages. Checkpoint the
// already-active validation stage; never skip or relax the durable stage order.
func emitRecoveryCheckpoint(emit func(string, string) error, node string) error {
	message := "Recovery checkpoint: owned node rollout acknowledged; retained material and storage verified"
	if node == "" {
		message = "Reconciliation checkpoint: no node restart requested; verifying recorded configuration"
	}
	return emit("validate", message)
}

// The chart always mounts the client-auth placeholder. It may be empty ONLY
// when that client-auth mode is explicitly disabled and cluster/admin TLS keep
// their qualified independent trust roots. No TLS setting is changed here.
func optionalRecoveryMaterial(kind, node, name string, config map[string]string) bool {
	return kind == "orderer" && name == node+"-peer-tlsrootcert" &&
		config["ORDERER_GENERAL_TLS_ENABLED"] == "true" && config["ORDERER_GENERAL_TLS_CLIENTAUTHREQUIRED"] == "false" &&
		config["ORDERER_GENERAL_CLUSTER_ROOTCAS"] == "/var/hyperledger/tls/server/cert/cacert.pem" &&
		config["ORDERER_ADMIN_TLS_ENABLED"] == "true" && config["ORDERER_ADMIN_TLS_CLIENTROOTCAS"] == "[/var/hyperledger/admin/clientcacert.crt]"
}
