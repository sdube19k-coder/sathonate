package deployment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	"sathonate.local/fabric-control-plane/internal/cloud"
	"sathonate.local/fabric-control-plane/internal/kubernetesfabric"
	"sathonate.local/fabric-control-plane/internal/localfabric"
)

// EKSRunner is a qualification adapter, not infrastructure provisioning. The
// registry and approval file are supplied by the administrator, never the API.
type EKSRunner struct {
	Registry  cloud.Registry
	Approvals []cloud.Approval
	Commands  cloud.Commander
}
type eksBinding struct {
	target   cloud.Target
	commands cloud.Commander
	plan     cloud.OperatorPlan
}

func (EKSRunner) Mode() string                     { return "aws-eks" }
func (r EKSRunner) Validate(request Request) error { _, err := r.bind(request); return err }
func (r EKSRunner) BindingDigest(request Request) string {
	t, ok := r.Registry.Find(request.TenantID, request.Network.Spec.Deployment.TargetRef)
	if !ok {
		return ""
	}
	return cloud.TargetDigest(t)
}
func (r EKSRunner) bind(request Request) (KubernetesRunner, error) {
	target, ok := r.Registry.Find(request.TenantID, request.Network.Spec.Deployment.TargetRef)
	if !ok {
		return KubernetesRunner{}, errors.New("target is not approved for this tenant")
	}
	if request.TargetSHA256 != "" && request.TargetSHA256 != cloud.TargetDigest(target) {
		return KubernetesRunner{}, errors.New("target configuration changed since job submission; restore the original approved binding before recovery")
	}
	approved := false
	for _, a := range r.Approvals {
		if a.Check(target, time.Now().UTC()) == nil {
			approved = true
		}
	}
	if !approved {
		return KubernetesRunner{}, errors.New("EKS qualification requires a current approval for this exact target")
	}
	plan, err := cloud.CompileOperator(request.Network, target, Namespace(request.TenantID, request.NetworkID))
	if err != nil {
		return KubernetesRunner{}, err
	}
	commands := r.Commands
	if commands == nil {
		commands = cloud.OSCommander{}
	}
	return KubernetesRunner{Kubeconfig: target.Operator.Kubeconfig, Image: target.RunnerImage, eks: &eksBinding{target: target, commands: commands, plan: plan}}, nil
}
func (r EKSRunner) Run(ctx context.Context, request Request, emit func(string, string) error) (Outcome, error) {
	worker, err := r.bind(request)
	if err != nil {
		return Outcome{}, err
	}
	outcome, err := worker.Run(ctx, request, emit)
	if err != nil {
		return Outcome{}, err
	}
	// A recovered, already-completed Job carries historical transaction proof.
	// Check current workloads before promoting that proof to a ready deployment.
	health, err := worker.Health(ctx, request.TenantID, request.NetworkID, outcome.Health)
	if err != nil {
		return Outcome{}, err
	}
	if !health.Ready {
		return Outcome{}, errors.New("Fabric transaction evidence exists, but current Kubernetes workloads are not ready; recover workloads and retry verification")
	}
	outcome.Health = &health
	return outcome, nil
}
func (r EKSRunner) existing(tenant, record string) (KubernetesRunner, error) {
	for _, t := range r.Registry.Targets {
		if t.TenantID == tenant && t.RunnerNamespace == Namespace(tenant, record) && t.Operator != nil {
			c := r.Commands
			if c == nil {
				c = cloud.OSCommander{}
			}
			return KubernetesRunner{Kubeconfig: t.Operator.Kubeconfig, Image: t.RunnerImage, eks: &eksBinding{target: t, commands: c}}, nil
		}
	}
	return KubernetesRunner{}, errors.New("no approved target binding for this network")
}
func (r EKSRunner) Health(ctx context.Context, tenant, record string, previous *localfabric.Health) (localfabric.Health, error) {
	w, e := r.existing(tenant, record)
	if e != nil {
		return localfabric.Health{}, e
	}
	if e = w.validateAccess(ctx); e != nil {
		return localfabric.Health{}, e
	}
	if e = w.ownedNamespace(ctx, tenant, record); e != nil {
		return localfabric.Health{}, e
	}
	return w.Health(ctx, tenant, record, previous)
}
func (r EKSRunner) Logs(ctx context.Context, tenant, record, node string) (string, error) {
	w, e := r.existing(tenant, record)
	if e != nil {
		return "", e
	}
	if e = w.validateAccess(ctx); e != nil {
		return "", e
	}
	if e = w.ownedNamespace(ctx, tenant, record); e != nil {
		return "", e
	}
	return w.Logs(ctx, tenant, record, node)
}
func (r EKSRunner) Cleanup(ctx context.Context, tenant, record string) error {
	return errors.New("EKS cleanup requires an administrator-approved current resource preview")
}

func (r EKSRunner) CancelDeployment(ctx context.Context, request Request) error {
	w, e := r.existing(request.TenantID, request.NetworkID)
	if e != nil {
		return e
	}
	if e = w.validateAccess(ctx); e != nil {
		return e
	}
	if e = w.ownedNamespace(ctx, request.TenantID, request.NetworkID); e != nil {
		return e
	}
	return w.Cancel(ctx, request.TenantID, request.OperationID)
}

func (r KubernetesRunner) ownedNamespace(ctx context.Context, tenant, record string) error {
	raw, err := r.run(ctx, "get", "namespace", Namespace(tenant, record), "-o", "json")
	if err != nil {
		return err
	}
	var value struct {
		Metadata struct{ Labels map[string]string }
	}
	if json.Unmarshal(raw, &value) != nil || value.Metadata.Labels["fabric.sathonate.io/network"] != record || value.Metadata.Labels["fabric.sathonate.io/tenant"] != tenant || value.Metadata.Labels["fabric.sathonate.io/target"] != r.eks.target.ID {
		return errors.New("namespace ownership no longer matches approved target")
	}
	return nil
}

func (r KubernetesRunner) eksManifest(request Request, name string) (map[string]any, error) {
	type M = map[string]any
	t := r.eks.target
	ns := t.RunnerNamespace
	raw, err := json.Marshal(request.Network)
	if err != nil {
		return nil, err
	}
	runtime, _ := json.Marshal(t.Operator.Runtime)
	labels := M{"fabric.sathonate.io/network": request.NetworkID, "fabric.sathonate.io/operation": name, "fabric.sathonate.io/tenant": request.TenantID, "fabric.sathonate.io/managed-by": "fabric-smp-eks"}
	channelNames := []string{ns + "-" + request.Network.Spec.Channels[0].ID}
	for _, o := range request.Network.Spec.Organizations {
		channelNames = append(channelNames, ns+"-"+o.ID+"-"+request.Network.Spec.Channels[0].ID)
	}
	rules := []any{
		M{"apiGroups": []string{""}, "resources": []string{"secrets"}, "verbs": []string{"get", "create", "patch", "update"}},
		M{"apiGroups": []string{""}, "resources": []string{"services", "pods"}, "verbs": []string{"get", "list", "watch"}},
		M{"apiGroups": []string{"apps"}, "resources": []string{"deployments"}, "verbs": []string{"get", "list", "watch"}},
		M{"apiGroups": []string{"hlf.kungfusoftware.es"}, "resources": []string{"fabriccas", "fabricpeers", "fabricorderernodes", "fabricchaincodes"}, "verbs": []string{"get", "list", "watch", "create", "patch", "update"}},
	}
	subject := []any{M{"kind": "ServiceAccount", "name": "smp-worker", "namespace": ns}}
	items := []any{
		M{"apiVersion": "v1", "kind": "ServiceAccount", "metadata": M{"name": "smp-worker", "namespace": ns, "labels": labels}},
		M{"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "Role", "metadata": M{"name": "smp-worker", "namespace": ns, "labels": labels}, "rules": rules},
		M{"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "RoleBinding", "metadata": M{"name": "smp-worker", "namespace": ns, "labels": labels}, "roleRef": M{"apiGroup": "rbac.authorization.k8s.io", "kind": "Role", "name": "smp-worker"}, "subjects": subject},
		M{"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "ClusterRole", "metadata": M{"name": ns + "-channels", "labels": labels}, "rules": []any{M{"apiGroups": []string{"hlf.kungfusoftware.es"}, "resources": []string{"fabricmainchannels", "fabricfollowerchannels"}, "resourceNames": channelNames, "verbs": []string{"get", "list", "watch"}}}},
		M{"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "ClusterRoleBinding", "metadata": M{"name": ns + "-channels", "labels": labels}, "roleRef": M{"apiGroup": "rbac.authorization.k8s.io", "kind": "ClusterRole", "name": ns + "-channels"}, "subjects": subject},
		M{"apiVersion": "v1", "kind": "ConfigMap", "metadata": M{"name": name, "namespace": ns, "labels": labels}, "immutable": true, "data": M{"network.json": string(raw), "runtime.json": string(runtime)}},
	}
	container := M{"name": "worker", "image": t.RunnerImage, "imagePullPolicy": "IfNotPresent", "workingDir": "/workspace", "args": []string{"--namespace", ns, "--runtime", "/inputs/runtime.json"}, "securityContext": M{"allowPrivilegeEscalation": false, "readOnlyRootFilesystem": true, "capabilities": M{"drop": []string{"ALL"}}}, "resources": M{"requests": M{"cpu": "100m", "memory": "256Mi"}, "limits": M{"cpu": "2", "memory": "1Gi"}}, "env": []any{M{"name": "HOME", "value": "/workspace"}, M{"name": "TMPDIR", "value": "/workspace"}}, "volumeMounts": []any{M{"name": "input", "mountPath": "/inputs", "readOnly": true}, M{"name": "workspace", "mountPath": "/workspace"}, M{"name": "tmp", "mountPath": "/tmp"}}}
	pod := M{"serviceAccountName": "smp-worker", "restartPolicy": "Never", "securityContext": M{"runAsNonRoot": true, "runAsUser": 10001, "runAsGroup": 10001, "fsGroup": 10001, "seccompProfile": M{"type": "RuntimeDefault"}}, "containers": []any{container}, "volumes": []any{M{"name": "input", "configMap": M{"name": name}}, M{"name": "workspace", "emptyDir": M{"medium": "Memory", "sizeLimit": "128Mi"}}, M{"name": "tmp", "emptyDir": M{"medium": "Memory", "sizeLimit": "64Mi"}}}}
	job := M{"apiVersion": "batch/v1", "kind": "Job", "metadata": M{"name": name, "namespace": ns, "labels": labels}, "spec": M{"backoffLimit": 0, "activeDeadlineSeconds": 1800, "template": M{"metadata": M{"labels": labels}, "spec": pod}}}
	items = append(items, job)
	return M{"apiVersion": "v1", "kind": "List", "items": items}, nil
}

func (r KubernetesRunner) broker(ctx context.Context, request Request, v kubernetesfabric.Object) error {
	ns := Namespace(request.TenantID, request.NetworkID)
	if err := kubernetesfabric.ValidateChannelRequest(v, ns, request.Network); err != nil {
		return err
	}
	resource := "fabricmainchannels"
	if v["kind"] == "FabricFollowerChannel" {
		resource = "fabricfollowerchannels"
	}
	metadata := v["metadata"].(map[string]any)
	name := metadata["name"].(string)
	raw, err := r.run(ctx, "get", resource, name, "--ignore-not-found", "-o", "json")
	if err != nil {
		return err
	}
	if len(raw) > 0 {
		var existing struct {
			Spec     map[string]any
			Metadata struct{ Labels map[string]string }
		}
		if json.Unmarshal(raw, &existing) != nil || existing.Metadata.Labels["sathonate.io/namespace"] != ns {
			return fmt.Errorf("channel broker refuses an unowned resource")
		}
		if !desiredFieldsMatch(existing.Spec, v["spec"]) {
			return errors.New("existing channel configuration differs from the recorded worker request; inspect drift before retry")
		}
		// Admission/defaulting and the operator may own other spec fields. A
		// matching existing channel is adopted, never reapplied during log replay.
		return nil
	}
	return r.apply(ctx, v)
}

func desiredFieldsMatch(actual, desired any) bool {
	switch want := desired.(type) {
	case map[string]any:
		have, ok := actual.(map[string]any)
		if !ok {
			return false
		}
		for key, value := range want {
			if !desiredFieldsMatch(have[key], value) {
				return false
			}
		}
		return true
	case []any:
		have, ok := actual.([]any)
		if len(want) == 0 && actual == nil {
			return true
		}
		if !ok || len(have) != len(want) {
			return false
		}
		for i := range want {
			if !desiredFieldsMatch(have[i], want[i]) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(actual, desired)
	}
}
