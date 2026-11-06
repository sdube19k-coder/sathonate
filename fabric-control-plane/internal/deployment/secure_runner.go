package deployment

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"regexp"
	"sathonate.local/fabric-control-plane/internal/domain"
	"sathonate.local/fabric-control-plane/internal/kubernetesfabric"
	"sathonate.local/fabric-control-plane/internal/localfabric"
	"strings"
	"time"
)

type SecureLocalConfig struct {
	Kubeconfig  string                   `json:"kubeconfig"`
	ClusterName string                   `json:"clusterName"`
	WorkerImage string                   `json:"workerImage"`
	Runtime     kubernetesfabric.Runtime `json:"runtime"`
}
type secureBinding struct {
	config  SecureLocalConfig
	kubeSHA string
	remote  *secureEKSBinding
}
type SecureRunner struct{ binding *secureBinding }

func LoadSecureRunner(path string) (SecureRunner, error) {
	raw, e := os.ReadFile(path)
	if e != nil {
		return SecureRunner{}, errors.New("secure local configuration unavailable")
	}
	var c SecureLocalConfig
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if len(raw) > 1<<20 || d.Decode(&c) != nil {
		return SecureRunner{}, errors.New("invalid secure local configuration")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return SecureRunner{}, errors.New("invalid secure local configuration")
	}
	return NewSecureRunner(c)
}
func NewSecureRunner(c SecureLocalConfig) (SecureRunner, error) {
	if !regexp.MustCompile(`^smp-sprint1-[a-f0-9]{8}$`).MatchString(c.ClusterName) || !regexp.MustCompile(`^[a-zA-Z0-9./:_-]+@sha256:[a-f0-9]{64}$`).MatchString(c.WorkerImage) || c.Kubeconfig == "" {
		return SecureRunner{}, errors.New("secure runner requires an explicit disposable Sprint 1 cluster and digest-pinned worker")
	}
	raw, e := os.ReadFile(c.Kubeconfig)
	if e != nil {
		return SecureRunner{}, errors.New("secure kubeconfig unavailable")
	}
	sum := sha256.Sum256(raw)
	// Deep copy the trusted config: a caller cannot mutate an active binding.
	data, _ := json.Marshal(c)
	var copy SecureLocalConfig
	json.Unmarshal(data, &copy)
	return SecureRunner{&secureBinding{config: copy, kubeSHA: hex.EncodeToString(sum[:])}}, nil
}
func (SecureRunner) Mode() string { return "secure-local" }
func (r SecureRunner) base() KubernetesRunner {
	if r.binding == nil {
		return KubernetesRunner{}
	}
	return KubernetesRunner{Kubeconfig: r.binding.config.Kubeconfig, Image: r.binding.config.WorkerImage, secure: r.binding}
}
func (r SecureRunner) Validate(q Request) error {
	dynamic := q.Network.Spec.Deployment.TargetRef == domain.DynamicLocalProfile
	if q.PreviousNetwork != nil && (!dynamic || kubernetesfabric.ValidateDynamicTransition(*q.PreviousNetwork, q.Network) != nil) {
		return errors.New("invalid approved dynamic transition")
	}
	if dynamic && (r.binding == nil || r.binding.remote != nil || q.Lifecycle != nil) {
		return errors.New("dynamic execution is restricted to an explicitly enabled disposable local target")
	}
	if q.Lifecycle != nil && (r.binding == nil || r.binding.remote != nil || q.Lifecycle.Validate() != nil) {
		return errors.New("lifecycle changes are local-only")
	}
	if r.binding == nil || !regexp.MustCompile(`^lp-[a-f0-9]{48}$`).MatchString(q.TenantID) || q.NetworkID == "" || domain.HasErrors(domain.Validate(q.Network)) || (!dynamic && q.Network.Spec.Deployment.TargetRef != domain.SecureLocalProfile) || r.binding.config.Runtime.Validate(q.Network) != nil || q.FailureStage != "" {
		return errors.New("secure local deployment requires an authenticated project scope and exact approved profile")
	}
	if q.TargetSHA256 != "" && q.TargetSHA256 != r.BindingDigest(q) {
		return errors.New("secure runtime binding changed; restore original target before retry")
	}
	return nil
}
func (r SecureRunner) BindingDigest(Request) string {
	if r.binding == nil {
		return ""
	}
	if r.binding.remote != nil {
		raw, _ := json.Marshal(struct {
			Config     SecureEKSConfig
			KubeSHA    string
			Controller SecureProfile
		}{r.binding.remote.config, r.binding.kubeSHA, SecureLocalProfile()})
		sum := sha256.Sum256(raw)
		return hex.EncodeToString(sum[:])
	}
	raw, _ := json.Marshal(struct {
		Config     SecureLocalConfig
		KubeSHA    string
		Controller SecureProfile
	}{r.binding.config, r.binding.kubeSHA, SecureLocalProfile()})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func (r KubernetesRunner) namespace(tenant, record string) string {
	if r.secure != nil {
		ns, _ := SecureNamespace(tenant, "token-bound-project", record)
		return ns
	}
	return Namespace(tenant, record)
}
func (r KubernetesRunner) validateSecure(ctx context.Context) error {
	if r.secure == nil {
		return errors.New("secure binding required")
	}
	raw, e := os.ReadFile(r.Kubeconfig)
	if e != nil {
		return errors.New("secure kubeconfig unavailable")
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != r.secure.kubeSHA {
		return errors.New("secure kubeconfig changed")
	}
	if r.secure.remote != nil {
		return r.validateSecureEKS(ctx)
	}
	raw, e = r.run(ctx, "config", "view", "--minify", "-o", "jsonpath={.clusters[0].cluster.server}")
	if e != nil {
		return e
	}
	u, e := url.Parse(strings.TrimSpace(string(raw)))
	if e != nil || u.Scheme != "https" || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost" && u.Hostname() != "::1") {
		return errors.New("secure local runner requires loopback Kubernetes")
	}
	name := r.secure.config.ClusterName
	raw, e = r.run(ctx, "get", "node", name+"-control-plane", "-o", "jsonpath={.spec.providerID}")
	if e != nil || strings.TrimSpace(string(raw)) != "kind://docker/"+name+"/"+name+"-control-plane" {
		return errors.New("disposable secure cluster identity mismatch")
	}
	return nil
}
func (r SecureRunner) owned(ctx context.Context, tenant, record string) error {
	if r.binding == nil {
		return errors.New("secure binding required")
	}
	if r.binding.remote != nil && (tenant != r.binding.remote.config.TenantID || record != r.binding.remote.config.NetworkID) {
		return errSecureEKS
	}
	b := r.base()
	if e := b.validateSecure(ctx); e != nil {
		return e
	}
	raw, e := b.run(ctx, "get", "namespace", b.namespace(tenant, record), "-o", "json")
	if e != nil {
		return e
	}
	var obj struct {
		Metadata struct{ Labels map[string]string }
	}
	if json.Unmarshal(raw, &obj) != nil {
		return errors.New("invalid secure namespace")
	}
	return ValidateSecureNamespace(obj.Metadata.Labels, tenant, "token-bound-project", record)
}
func (r SecureRunner) Run(ctx context.Context, q Request, emit func(string, string) error) (Outcome, error) {
	if e := r.Validate(q); e != nil {
		return Outcome{}, e
	}
	ctx, cancel := context.WithTimeout(ctx, 28*time.Minute)
	defer cancel()
	b := r.base()
	if e := b.validateSecure(ctx); e != nil {
		return Outcome{}, e
	}
	if e := emit("validate", "Authorized tenant/project and immutable secure runtime binding"); e != nil {
		return Outcome{}, e
	}
	name, e := jobName(q.OperationID)
	if e != nil {
		return Outcome{}, e
	}
	ns := b.namespace(q.TenantID, q.NetworkID)
	raw, e := b.run(ctx, "get", "namespace", ns, "--ignore-not-found", "-o", "json")
	if e != nil {
		return Outcome{}, e
	}
	type M = map[string]any
	labels := M{"fabric.sathonate.io/tenant": q.TenantID, "fabric.sathonate.io/project": "token-bound-project", "fabric.sathonate.io/network": q.NetworkID, "fabric.sathonate.io/profile": domain.SecureLocalProfile}
	labels["pod-security.kubernetes.io/enforce"] = "baseline"
	labels["pod-security.kubernetes.io/enforce-version"] = "v1.34"
	if len(bytes.TrimSpace(raw)) == 0 {
		// Create, never overwrite a concurrently introduced namespace's ownership.
		doc, _ := json.Marshal(M{"apiVersion": "v1", "kind": "Namespace", "metadata": M{"name": ns, "labels": labels}})
		cmd := b.command(ctx, "create", "-f", "-")
		cmd.Stdin = bytes.NewReader(doc)
		if cmd.Run() != nil {
			return Outcome{}, errors.New("secure namespace creation failed; retry ownership validation")
		}
	}
	if e = r.owned(ctx, q.TenantID, q.NetworkID); e != nil {
		return Outcome{}, e
	}
	if e = r.installIsolation(ctx, ns); e != nil {
		return Outcome{}, e
	}
	current, e := b.run(ctx, "-n", ns, "get", "job", name, "--ignore-not-found", "-o", "json")
	if e != nil {
		return Outcome{}, e
	}
	if len(bytes.TrimSpace(current)) == 0 {
		if q.LifecycleAction == "restart-peer" {
			if e := r.restartPeer(ctx, q, ns); e != nil {
				return Outcome{}, e
			}
		}
		manifest, e := r.manifest(q, name, ns)
		if e != nil {
			return Outcome{}, e
		}
		if e = b.apply(ctx, manifest); e != nil {
			return Outcome{}, e
		}
	} else {
		var job struct {
			Metadata struct{ Labels map[string]string }
			Spec     struct {
				Template struct {
					Spec struct{ Containers []struct{ Image string } }
				}
			}
		}
		if json.Unmarshal(current, &job) != nil || job.Metadata.Labels["fabric.sathonate.io/network"] != q.NetworkID || job.Metadata.Labels["fabric.sathonate.io/tenant"] != q.TenantID || len(job.Spec.Template.Spec.Containers) != 1 || job.Spec.Template.Spec.Containers[0].Image != r.binding.config.WorkerImage {
			return Outcome{}, errors.New("unowned or drifted secure Job")
		}
	}
	return b.watch(ctx, q, name, ns, emit)
}
func (r SecureRunner) manifest(q Request, name, ns string) (map[string]any, error) {
	if r.binding.remote != nil {
		// Reuse the exact qualified manifest builder and portable image transform.
		local := SecureRunner{binding: &secureBinding{config: r.binding.config}}
		workloads, e := local.manifest(q, name, ns)
		if e != nil {
			return nil, e
		}
		return portableQualificationWorkloads(workloads, r.binding.remote.config.ControllerImage)
	}

	type M = map[string]any
	network, e := json.Marshal(q.Network)
	if e != nil {
		return nil, e
	}
	runtime, _ := json.Marshal(r.binding.config.Runtime)
	lifecycle, _ := json.Marshal(q.Lifecycle)
	previous, _ := json.Marshal(q.PreviousNetwork)
	labels := M{"fabric.sathonate.io/tenant": q.TenantID, "fabric.sathonate.io/network": q.NetworkID, "fabric.sathonate.io/operation": name, "fabric.sathonate.io/profile": domain.SecureLocalProfile}
	meta := func(n string) M { return M{"name": n, "namespace": ns, "labels": labels} }
	role := func(name string, rules []any) []any {
		return []any{M{"apiVersion": "v1", "kind": "ServiceAccount", "metadata": meta(name), "automountServiceAccountToken": true}, M{"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "Role", "metadata": meta(name), "rules": rules}, M{"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "RoleBinding", "metadata": meta(name), "roleRef": M{"apiGroup": "rbac.authorization.k8s.io", "kind": "Role", "name": name}, "subjects": []any{M{"kind": "ServiceAccount", "name": name, "namespace": ns}}}}
	}
	rule := func(group string, resources, verbs []string) M {
		return M{"apiGroups": []string{group}, "resources": resources, "verbs": verbs}
	}
	all := []string{"get", "list", "watch", "create", "update", "patch", "delete"}
	items := []any{}
	controllerRules := []any{rule("", []string{"events"}, []string{"create", "patch"}), rule("", []string{"secrets", "configmaps", "services", "persistentvolumeclaims"}, all), rule("", []string{"pods"}, []string{"get", "list", "watch", "delete"}), rule("apps", []string{"deployments", "replicasets"}, all), rule("coordination.k8s.io", []string{"leases"}, all)}
	fabric := []string{}
	for _, resource := range []string{"fabriccas", "fabricpeers", "fabricorderernodes", "fabricchaincodes"} {
		fabric = append(fabric, resource, resource+"/status", resource+"/finalizers")
	}
	controllerRules = append(controllerRules, rule("hlf.kungfusoftware.es", fabric, all))
	items = append(items, role("secure-controller", controllerRules)...)
	issuerRules := []any{rule("", []string{"secrets"}, []string{"get", "create", "patch", "update"}), rule("", []string{"pods", "services"}, []string{"get", "list", "watch"}), rule("apps", []string{"deployments"}, []string{"get", "list", "watch"}), rule("hlf.kungfusoftware.es", []string{"fabriccas", "fabricpeers", "fabricorderernodes", "fabricchaincodes"}, []string{"get", "list", "watch", "create", "patch", "update"})}
	items = append(items, role("secure-issuer", issuerRules)...)
	security := M{"allowPrivilegeEscalation": false, "runAsNonRoot": true, "capabilities": M{"drop": []string{"ALL"}}, "seccompProfile": M{"type": "RuntimeDefault"}}
	image := SecureLocalProfile().ControllerImage
	controller := M{"name": "controller", "image": image, "imagePullPolicy": "Never", "env": []any{M{"name": "SMP_NAMESPACE", "value": ns}, M{"name": "SMP_CONFIG_INIT_IMAGE", "value": image}}, "securityContext": security, "resources": M{"requests": M{"memory": "128Mi", "cpu": "100m"}, "limits": M{"memory": "512Mi", "cpu": "2"}}}
	items = append(items, M{"apiVersion": "apps/v1", "kind": "Deployment", "metadata": meta("secure-controller"), "spec": M{"replicas": 1, "selector": M{"matchLabels": M{"app": "secure-controller"}}, "template": M{"metadata": M{"labels": M{"app": "secure-controller", "sathonate.io/api-client": "true"}}, "spec": M{"automountServiceAccountToken": true, "serviceAccountName": "secure-controller", "containers": []any{controller}}}}})
	items = append(items, M{"apiVersion": "v1", "kind": "ConfigMap", "metadata": meta(name), "immutable": true, "data": M{"network.json": string(network), "runtime.json": string(runtime), "lifecycle.json": string(lifecycle), "previous.json": string(previous)}})
	container := M{"name": "worker", "image": r.binding.config.WorkerImage, "imagePullPolicy": "Never", "workingDir": "/workspace", "args": []string{"--secure-local", "--namespace", ns, "--runtime", "/inputs/runtime.json"}, "securityContext": M{"allowPrivilegeEscalation": false, "readOnlyRootFilesystem": true, "capabilities": M{"drop": []string{"ALL"}}}, "resources": M{"requests": M{"cpu": "100m", "memory": "256Mi"}, "limits": M{"cpu": "2", "memory": "1Gi"}}, "env": []any{M{"name": "HOME", "value": "/workspace"}, M{"name": "TMPDIR", "value": "/workspace"}}, "volumeMounts": []any{M{"name": "input", "mountPath": "/inputs", "readOnly": true}, M{"name": "workspace", "mountPath": "/workspace"}, M{"name": "tmp", "mountPath": "/tmp"}}}
	if q.Lifecycle != nil {
		container["args"] = append(container["args"].([]string), "--lifecycle", "/inputs/lifecycle.json")
	}
	if q.PreviousNetwork != nil {
		container["args"] = append(container["args"].([]string), "--previous", "/inputs/previous.json")
	}
	labels["sathonate.io/api-client"] = "true"
	pod := M{"automountServiceAccountToken": true, "serviceAccountName": "secure-issuer", "restartPolicy": "Never", "securityContext": M{"runAsNonRoot": true, "runAsUser": 10001, "runAsGroup": 10001, "fsGroup": 10001, "seccompProfile": M{"type": "RuntimeDefault"}}, "containers": []any{container}, "volumes": []any{M{"name": "input", "configMap": M{"name": name}}, M{"name": "workspace", "emptyDir": M{"medium": "Memory", "sizeLimit": "128Mi"}}, M{"name": "tmp", "emptyDir": M{"medium": "Memory", "sizeLimit": "64Mi"}}}}
	items = append(items, M{"apiVersion": "batch/v1", "kind": "Job", "metadata": meta(name), "spec": M{"backoffLimit": 0, "activeDeadlineSeconds": 1500, "template": M{"metadata": M{"labels": labels}, "spec": pod}}})
	return M{"apiVersion": "v1", "kind": "List", "items": items}, nil
}
func (r SecureRunner) Health(ctx context.Context, tenant, record string, previous *localfabric.Health) (localfabric.Health, error) {
	if e := r.owned(ctx, tenant, record); e != nil {
		return localfabric.Health{}, e
	}
	return r.base().Health(ctx, tenant, record, previous)
}
func (r SecureRunner) Logs(ctx context.Context, tenant, record, node string) (string, error) {
	if e := r.owned(ctx, tenant, record); e != nil {
		return "", e
	}
	return r.base().Logs(ctx, tenant, record, node)
}
func (r SecureRunner) Cleanup(context.Context, string, string) error {
	return errors.New("secure local resources require trusted disposable-cluster teardown; persistent material is never silently deleted")
}
func (r SecureRunner) CancelDeployment(ctx context.Context, q Request) error {
	if e := r.owned(ctx, q.TenantID, q.NetworkID); e != nil {
		return e
	}
	name, e := jobName(q.OperationID)
	if e != nil {
		return e
	}
	b := r.base()
	ns := b.namespace(q.TenantID, q.NetworkID)
	raw, e := b.run(ctx, "-n", ns, "get", "job", name, "--ignore-not-found", "-o", "json")
	if e != nil || len(bytes.TrimSpace(raw)) == 0 {
		return e
	}
	var item struct {
		Metadata struct {
			UID    string
			Labels map[string]string
		}
	}
	if json.Unmarshal(raw, &item) != nil || item.Metadata.Labels["fabric.sathonate.io/tenant"] != q.TenantID || item.Metadata.Labels["fabric.sathonate.io/network"] != q.NetworkID {
		return errors.New("unowned secure worker Job")
	}
	return b.deleteCleanupRef(ctx, CleanupRef{Resource: "jobs", Namespace: ns, Name: name, UID: item.Metadata.UID})
}
