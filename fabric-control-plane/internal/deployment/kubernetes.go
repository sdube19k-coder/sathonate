package deployment

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"sathonate.local/fabric-control-plane/internal/cloud"
	"sathonate.local/fabric-control-plane/internal/kubernetesfabric"
	"sathonate.local/fabric-control-plane/internal/localfabric"
	"sathonate.local/fabric-control-plane/internal/redact"
)

// KubernetesRunner executes only against a dedicated local kind context.
// It has no AWS commands, cloud credentials or cloud provisioning path.
type KubernetesRunner struct {
	secure     *secureBinding
	eks        *eksBinding
	Kubeconfig string
	Image      string
}

func (r KubernetesRunner) Health(ctx context.Context, tenant, record string, previous *localfabric.Health) (localfabric.Health, error) {
	if previous == nil {
		return localfabric.Health{}, errors.New("no verified Fabric evidence is available")
	}
	raw, _ := json.Marshal(previous)
	var health localfabric.Health
	_ = json.Unmarshal(raw, &health)
	namespace := r.namespace(tenant, record)
	raw, err := r.run(ctx, "-n", namespace, "get", "fabriccas,fabricorderernodes,fabricpeers", "-o", "json")
	if err != nil {
		return health, err
	}
	var list struct {
		Items []struct {
			Kind     string
			Metadata struct{ Name string }
			Status   struct{ Status string }
		}
	}
	if err = json.Unmarshal(raw, &list); err != nil {
		return health, err
	}
	ready := map[string]bool{}
	for _, item := range list.Items {
		ready[item.Metadata.Name] = strings.EqualFold(item.Status.Status, "running")
	}
	health.Ready = true
	health.Alerts = []string{}
	for i := range health.Peers {
		health.Peers[i].Available = ready[health.Peers[i].Peer]
		if !health.Peers[i].Available {
			health.Ready = false
		}
	}
	for i := range health.CAs {
		health.CAs[i].Available = ready[health.CAs[i].Name]
		if !health.CAs[i].Available {
			health.Ready = false
		}
	}
	for i := range health.Orderers {
		if !ready[health.Orderers[i].Name] {
			health.Orderers[i].Status = "unavailable"
			health.Ready = false
		}
	}
	// A controller's last Running status can outlive a crashed pod. Check actual
	// workload readiness too, without pretending cached ledger data is a live query.
	podsRaw, err := r.run(ctx, "-n", namespace, "get", "pods", "-o", "json")
	if err != nil {
		return health, err
	}
	var pods struct {
		Items []struct {
			Metadata struct{ Labels map[string]string }
			Status   struct {
				Phase             string
				ContainerStatuses []struct{ Ready bool }
			}
		}
	}
	if err = json.Unmarshal(podsRaw, &pods); err != nil {
		return health, err
	}
	workloads := map[string]bool{}
	for _, pod := range pods.Items {
		if pod.Metadata.Labels["job-name"] != "" {
			continue
		}
		if pod.Status.Phase != "Running" {
			health.Ready = false
		}
		podReady := pod.Status.Phase == "Running" && len(pod.Status.ContainerStatuses) > 0
		for _, container := range pod.Status.ContainerStatuses {
			if !container.Ready {
				health.Ready = false
				podReady = false
			}
		}
		if podReady {
			workloads[pod.Metadata.Labels["release"]] = true
			if chaincode := pod.Metadata.Labels["chaincode"]; chaincode != "" {
				workloads[chaincode] = true
			}
		}
	}
	for i := range health.Peers {
		health.Peers[i].Available = health.Peers[i].Available && workloads[health.Peers[i].Peer]
		if !health.Peers[i].Available {
			health.Ready = false
		}
	}
	for i := range health.CAs {
		health.CAs[i].Available = health.CAs[i].Available && workloads[health.CAs[i].Name]
		if !health.CAs[i].Available {
			health.Ready = false
		}
	}
	for i := range health.Orderers {
		if !workloads[health.Orderers[i].Name] {
			health.Orderers[i].Status = "unavailable"
			health.Ready = false
		}
	}
	service := "smp-smoke"
	if !workloads[service] {
		service = kubernetesfabric.DynamicService(health.Channel, health.Chaincode.Name, health.Chaincode.Sequence)
	}
	health.Chaincode.Available = workloads[service]
	if !health.Chaincode.Available {
		health.Ready = false
	}
	if !health.Ready {
		health.Alerts = append(health.Alerts, "One or more Kubernetes Fabric workloads are unavailable")
	}
	health.Alerts = append(health.Alerts, "Ledger heights and transaction evidence are from the last completed verification. Run Transaction Check to verify fresh Fabric behavior.")
	return health, nil
}
func (r KubernetesRunner) Logs(ctx context.Context, tenant, record, node string) (string, error) {
	namespace := r.namespace(tenant, record)
	raw, err := r.run(ctx, "-n", namespace, "get", "pods", "-l", "release="+node, "-o", "json")
	if err != nil {
		return "", err
	}
	var pods struct {
		Items []struct{ Metadata struct{ Name string } }
	}
	if json.Unmarshal(raw, &pods) != nil || len(pods.Items) == 0 {
		return "", errors.New("node pod is unavailable")
	}
	logs, err := r.run(ctx, "-n", namespace, "logs", pods.Items[0].Metadata.Name, "--all-containers=true", "--tail=100", "--limit-bytes=32768")
	if err != nil {
		return "", err
	}
	return redact.Text(string(logs)), nil
}
func (r KubernetesRunner) Cleanup(ctx context.Context, tenant, record string) error {
	if r.eks != nil {
		return errors.New("EKS cleanup requires an administrator-approved current resource preview")
	}
	if err := r.validateAccess(ctx); err != nil {
		return err
	}
	namespace := r.namespace(tenant, record)
	raw, err := r.run(ctx, "get", "namespace", namespace, "--ignore-not-found", "-o", "json")
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	var current struct {
		Metadata struct{ Labels map[string]string }
	}
	if json.Unmarshal(raw, &current) != nil || current.Metadata.Labels["fabric.sathonate.io/network"] != record {
		return errors.New("refusing to delete an unowned namespace")
	}
	if r.eks != nil && (current.Metadata.Labels["fabric.sathonate.io/tenant"] != tenant || current.Metadata.Labels["fabric.sathonate.io/target"] != r.eks.target.ID) {
		return errors.New("refusing to clean up a foreign tenant target")
	}
	if _, err = r.run(ctx, "delete", "fabricmainchannels,fabricfollowerchannels", "-l", "sathonate.io/namespace="+namespace, "--ignore-not-found", "--wait=true", "--timeout=60s"); err != nil {
		return err
	}
	if _, err = r.run(ctx, "delete", "namespace", namespace, "--wait=true", "--timeout=90s"); err != nil {
		return err
	}
	_, err = r.run(ctx, "delete", "clusterrolebinding", namespace+"-worker", "--ignore-not-found")
	return err
}

func (r KubernetesRunner) Mode() string {
	if r.eks != nil {
		return "aws-eks"
	}
	return "local-kubernetes"
}
func (r KubernetesRunner) command(ctx context.Context, args ...string) *exec.Cmd {
	contextName := "kind-smp-qualification"
	if r.secure != nil {
		contextName = "kind-" + r.secure.config.ClusterName
		if r.secure.remote != nil {
			contextName = secureEKSRun
		}
	}
	if r.eks != nil {
		contextName = r.eks.target.KubeContext
	}
	return exec.CommandContext(ctx, "kubectl", append([]string{"--kubeconfig", r.Kubeconfig, "--context", contextName, "--request-timeout=20s"}, args...)...)
}
func (r KubernetesRunner) run(ctx context.Context, args ...string) ([]byte, error) {
	if r.secure != nil && r.secure.remote != nil && r.secure.remote.commands != nil {
		out, e := r.secure.remote.commands.Run(ctx, "kubectl", append([]string{"--kubeconfig", r.Kubeconfig, "--context", secureEKSRun, "--request-timeout=20s"}, args...))
		if e != nil {
			return nil, errSecureEKS
		}
		return out, nil
	}
	cmd := r.command(ctx, args...)
	var out, diagnostics bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &diagnostics
	if err := cmd.Run(); err != nil {
		if r.secure != nil {
			return nil, errors.New("secure local Kubernetes request failed; diagnostics withheld")
		}
		if r.eks != nil {
			return nil, errors.New("EKS Kubernetes request failed; verify scoped permissions and resource readiness")
		}
		return nil, fmt.Errorf("local Kubernetes command failed: %s", redact.Text(diagnostics.String()))
	}
	return out.Bytes(), nil
}
func (r KubernetesRunner) apply(ctx context.Context, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	cmd := r.command(ctx, "apply", "--server-side", "--field-manager=fabric-smp-local", "-f", "-")
	cmd.Stdin = bytes.NewReader(raw)
	var diagnostics bytes.Buffer
	cmd.Stderr = &diagnostics
	if err = cmd.Run(); err != nil {
		if r.secure != nil {
			return errors.New("secure local Kubernetes apply failed; diagnostics withheld")
		}
		if r.eks != nil {
			return errors.New("EKS apply failed; verify approved resources and scoped permissions")
		}
		return fmt.Errorf("local Kubernetes apply failed: %s", redact.Text(diagnostics.String()))
	}
	return nil
}
func Namespace(tenant, network string) string {
	sum := sha256.Sum256([]byte(tenant + "/" + network))
	return "smp-n-" + hex.EncodeToString(sum[:])[:16]
}
func jobName(id string) (string, error) {
	if !regexp.MustCompile(`^op_[a-f0-9]{32}$`).MatchString(id) {
		return "", errors.New("invalid operation identifier")
	}
	return "smp-" + strings.TrimPrefix(id, "op_"), nil
}
func (r KubernetesRunner) Cancel(ctx context.Context, tenant, id string) error {
	if err := r.validateAccess(ctx); err != nil {
		return err
	}
	name, err := jobName(id)
	if err != nil {
		return err
	}
	if r.eks != nil {
		_, err = r.run(ctx, "-n", r.eks.target.RunnerNamespace, "delete", "job", name, "--ignore-not-found", "--wait=true", "--timeout=45s")
	} else {
		_, err = r.run(ctx, "delete", "jobs", "-A", "-l", "fabric.sathonate.io/operation="+name, "--ignore-not-found", "--wait=true", "--timeout=45s")
	}
	return err
}
func (r KubernetesRunner) Run(ctx context.Context, request Request, emit func(string, string) error) (Outcome, error) {
	if err := rejectUnintegratedSecureProfile(request.Network); err != nil {
		return Outcome{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 28*time.Minute)
	defer cancel()
	name, err := jobName(request.OperationID)
	if err != nil {
		return Outcome{}, err
	}
	namespace := Namespace(request.TenantID, request.NetworkID)
	if r.Kubeconfig == "" || r.Image == "" {
		return Outcome{}, errors.New("local worker kubeconfig/image is not configured")
	}
	if request.FailureStage != "" {
		return Outcome{}, errors.New("failure injection cannot be used for real deployment")
	}
	if err = emit("validate", "Checking approved Kubernetes target and saved deployment input"); err != nil {
		return Outcome{}, err
	}
	if r.eks == nil {
		current, contextErr := r.run(ctx, "config", "current-context")
		if contextErr != nil || strings.TrimSpace(string(current)) != "kind-smp-qualification" {
			return Outcome{}, errors.New("local runner requires kind-smp-qualification context")
		}
	}
	if err = r.validateLocal(ctx); err != nil {
		return Outcome{}, err
	}
	if existing, getErr := r.run(ctx, "get", "namespace", namespace, "--ignore-not-found", "-o", "json"); getErr != nil {
		return Outcome{}, getErr
	} else if len(bytes.TrimSpace(existing)) > 0 {
		var obj map[string]any
		if json.Unmarshal(existing, &obj) != nil {
			return Outcome{}, errors.New("invalid namespace metadata")
		}
		metadata, _ := obj["metadata"].(map[string]any)
		labels, _ := metadata["labels"].(map[string]any)
		if labels["fabric.sathonate.io/network"] != request.NetworkID {
			return Outcome{}, errors.New("refusing to adopt a namespace owned by another network")
		}
	}
	networkJSON, err := json.Marshal(request.Network)
	if err != nil {
		return Outcome{}, err
	}
	type M = map[string]any
	labels := M{"fabric.sathonate.io/network": request.NetworkID, "fabric.sathonate.io/operation": name, "fabric.sathonate.io/managed-by": "fabric-smp-local"}
	pod := M{"serviceAccountName": "smp-worker", "restartPolicy": "Never", "securityContext": M{"runAsUser": 10001, "runAsGroup": 10001, "fsGroup": 10001}}
	container := M{"name": "worker", "image": r.Image, "imagePullPolicy": "Never", "workingDir": "/workspace", "args": []string{"--namespace", namespace}}
	container["securityContext"] = M{"allowPrivilegeEscalation": false, "capabilities": M{"drop": []string{"ALL"}}}
	container["resources"] = M{"requests": M{"cpu": "100m", "memory": "256Mi"}, "limits": M{"cpu": "2", "memory": "1Gi"}}
	container["volumeMounts"] = []any{M{"name": "input", "mountPath": "/inputs", "readOnly": true}, M{"name": "workspace", "mountPath": "/workspace"}}
	pod["containers"] = []any{container}
	pod["volumes"] = []any{M{"name": "input", "configMap": M{"name": name}}, M{"name": "workspace", "emptyDir": M{"medium": "Memory", "sizeLimit": "128Mi"}}}
	job := M{"apiVersion": "batch/v1", "kind": "Job", "metadata": M{"name": name, "namespace": namespace, "labels": labels}}
	job["spec"] = M{"backoffLimit": 0, "activeDeadlineSeconds": 1800, "template": M{"metadata": M{"labels": labels}, "spec": pod}}
	manifest := M{"apiVersion": "v1", "kind": "List", "items": []any{
		M{"apiVersion": "v1", "kind": "Namespace", "metadata": M{"name": namespace, "labels": labels}},
		M{"apiVersion": "v1", "kind": "ServiceAccount", "metadata": M{"name": "smp-worker", "namespace": namespace}},
		M{"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "ClusterRoleBinding", "metadata": M{"name": namespace + "-worker", "labels": labels}, "roleRef": M{"apiGroup": "rbac.authorization.k8s.io", "kind": "ClusterRole", "name": "cluster-admin"}, "subjects": []any{M{"kind": "ServiceAccount", "name": "smp-worker", "namespace": namespace}}},
		M{"apiVersion": "v1", "kind": "ConfigMap", "metadata": M{"name": name, "namespace": namespace}, "immutable": true, "data": M{"network.json": string(networkJSON)}}, job,
	}}
	if r.eks != nil {
		manifest, err = r.eksManifest(request, name)
		if err != nil {
			return Outcome{}, err
		}
	}
	// A restarted controller adopts the same immutable Job, never reapplies its
	// pod template. Retry uses a new operation Job after cancelling its predecessor.
	existingJob, readErr := r.run(ctx, "-n", namespace, "get", "job", name, "--ignore-not-found", "-o", "json")
	if readErr != nil {
		return Outcome{}, readErr
	}
	if len(bytes.TrimSpace(existingJob)) > 0 {
		var previous struct {
			Metadata struct{ Labels map[string]string }
		}
		if json.Unmarshal(existingJob, &previous) != nil || previous.Metadata.Labels["fabric.sathonate.io/network"] != request.NetworkID {
			return Outcome{}, errors.New("refusing an unowned worker Job")
		}
	} else if err = r.apply(ctx, manifest); err != nil {
		return Outcome{}, err
	}
	return r.watch(ctx, request, name, namespace, emit)
}

func (r KubernetesRunner) watch(ctx context.Context, request Request, name, namespace string, emit func(string, string) error) (Outcome, error) {
	var err error
	// Poll actual Job state and consume monotonically ordered JSON worker events.
	cursor := 0
	lastStage := "validate"
	outcome := Outcome{Resources: []Resource{}}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		raw, getErr := r.run(ctx, "-n", namespace, "get", "job", name, "-o", "json")
		if getErr != nil {
			return Outcome{}, getErr
		}
		var job struct {
			Status struct{ Succeeded, Failed int }
		}
		if err = json.Unmarshal(raw, &job); err != nil {
			return Outcome{}, err
		}
		logs, logErr := r.run(ctx, "-n", namespace, "logs", "job/"+name)
		if logErr == nil {
			scanner := bufio.NewScanner(bytes.NewReader(logs))
			scanner.Buffer(make([]byte, 4096), 1<<20)
			line := 0
			for scanner.Scan() {
				line++
				if line <= cursor {
					continue
				}
				var event kubernetesfabric.Event
				if json.Unmarshal(scanner.Bytes(), &event) != nil {
					continue
				}
				if event.Channel != nil {
					if r.eks == nil {
						return Outcome{}, errors.New("channel broker request is unavailable on this profile")
					}
					if err = r.broker(ctx, request, event.Channel); err != nil {
						return Outcome{}, err
					}
				}
				stage := event.Stage
				if stage == "" {
					stage = lastStage
				}
				if err = emit(stage, event.Message); err != nil {
					return Outcome{}, err
				}
				lastStage = stage
				if event.Health != nil {
					outcome.Health = event.Health
					outcome.TransactionID = event.TransactionID
				}
				if event.Security != nil {
					outcome.Security = event.Security
				}
				if event.Verification != nil {
					outcome.Verifications = append(outcome.Verifications, *event.Verification)
				}
			}
			if err = scanner.Err(); err != nil {
				return Outcome{}, err
			}
			cursor = line
		}
		if job.Status.Failed > 0 {
			return Outcome{}, errors.New("Bevel worker failed; see recorded redacted events and scoped workload events")
		}
		if job.Status.Succeeded > 0 {
			if outcome.Health == nil || !outcome.Health.Ready || outcome.TransactionID == "" {
				return Outcome{}, errors.New("worker completed without Fabric verification evidence")
			}
			effective := request.Network
			if request.Lifecycle != nil {
				effective = request.Lifecycle.Effective(effective)
			}
			for _, n := range effective.Spec.Nodes {
				outcome.Resources = append(outcome.Resources, Resource{Name: n.ID, Kind: n.Role, Organization: n.OrganizationRef, Status: "running"})
			}
			return outcome, nil
		}
		select {
		case <-ctx.Done():
			if (r.eks != nil || r.secure != nil) && errors.Is(ctx.Err(), context.Canceled) {
				return Outcome{}, ctx.Err()
			}
			cleanup, cancelCleanup := context.WithTimeout(context.Background(), 50*time.Second)
			if r.secure != nil {
				_ = (SecureRunner{binding: r.secure}).CancelDeployment(cleanup, request)
			} else {
				_ = r.Cancel(cleanup, request.TenantID, request.OperationID)
			}
			cancelCleanup()
			return Outcome{}, ctx.Err()
		case <-ticker.C:
		}
	}
}

// A context name alone is insufficient: reject renamed remote clusters before mutations.
func (r KubernetesRunner) validateAccess(ctx context.Context) error {
	if r.secure != nil {
		return r.validateSecure(ctx)
	}
	if r.eks != nil {
		return cloud.OperatorAccess(ctx, r.eks.target, r.eks.commands)
	}
	return r.validateLocal(ctx)
}

func (r KubernetesRunner) validateLocal(ctx context.Context) error {
	if r.eks != nil {
		return cloud.OperatorPreflight(ctx, r.eks.target, r.eks.commands)
	}
	raw, err := r.run(ctx, "config", "view", "--minify", "-o", "jsonpath={.clusters[0].cluster.server}")
	if err != nil {
		return err
	}
	u, err := url.Parse(strings.TrimSpace(string(raw)))
	if err != nil || u.Scheme != "https" || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost" && u.Hostname() != "::1") {
		return errors.New("local runner requires a loopback Kubernetes API")
	}
	raw, err = r.run(ctx, "get", "node", "smp-qualification-control-plane", "-o", "jsonpath={.spec.providerID}")
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(raw)) != "kind://docker/smp-qualification/smp-qualification-control-plane" {
		return errors.New("local runner requires the dedicated Docker kind node")
	}
	_, err = r.run(ctx, "get", "--raw=/readyz")
	return err
}
