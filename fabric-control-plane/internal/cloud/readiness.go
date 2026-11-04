package cloud

import (
	"context"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"time"

	"sathonate.local/fabric-control-plane/internal/domain"
)

// Evidence intentionally contains no raw API responses, diagnostics, names or configuration.
// Only fixed check messages, hashes, digests and controlled source metadata are exported.
type ReadinessCheck struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	Detail    string    `json:"detail"`
	CheckedAt time.Time `json:"checkedAt"`
}
type ReadinessEvidence struct {
	Schema                     string            `json:"schema"`
	StartedAt                  time.Time         `json:"startedAt"`
	FinishedAt                 time.Time         `json:"finishedAt"`
	SourceRevision             string            `json:"sourceRevision"`
	SourceClean                bool              `json:"sourceClean"`
	TargetSHA256               string            `json:"targetSHA256"`
	NetworkSHA256              string            `json:"networkSHA256"`
	ImageDigests               map[string]string `json:"imageDigests"`
	Checks                     []ReadinessCheck  `json:"checks"`
	Status                     string            `json:"status"`
	AWSReadinessVerified       bool              `json:"awsReadinessVerified"`
	EKSQualified               bool              `json:"eksQualified"`
	ProductionExecutionEnabled bool              `json:"productionExecutionEnabled"`
}

// VerifyReadiness only reads the approved existing target. It cannot qualify a
// deployment: policy enforcement, registry pulls and volume attachment need live tests.
func VerifyReadiness(ctx context.Context, s domain.NetworkSpec, t Target, registry Registry, namespace, record, revision string, clean bool, c Commander) ReadinessEvidence {
	e := ReadinessEvidence{Schema: "sathonate.eks-readiness/v1", StartedAt: time.Now().UTC(), TargetSHA256: TargetDigest(t), ImageDigests: map[string]string{}, Checks: []ReadinessCheck{}, Status: "NOT VERIFIED"}
	add := func(id, status, detail string) {
		e.Checks = append(e.Checks, ReadinessCheck{id, status, detail, time.Now().UTC()})
	}
	finish := func() ReadinessEvidence {
		required := []string{"target", "registry-exclusivity", "image-pins", "eks-preflight", "namespace-labels", "fresh-namespace", "quotas", "quota-capacity", "default-deny", "isolation-enforcement", "pod-admission", "effective-admission", "admission-inventory", "placement", "scheduling-capacity", "registry-provenance", "storage-attachment", "namespace-access", "fabric-behavior", "postgresql", "database-operation-isolation", "storage-iam", "csi-controller-health", "image-pull", "certificate-lifecycle", "controller-containment"}
		for _, id := range required {
			found := false
			for _, check := range e.Checks {
				found = found || check.ID == id
			}
			if !found {
				add(id, "NOT VERIFIED", "Not assessed because an earlier prerequisite failed")
			}
		}
		sort.Slice(e.Checks, func(i, j int) bool { return e.Checks[i].ID < e.Checks[j].ID })
		e.FinishedAt = time.Now().UTC()
		for _, v := range e.Checks {
			if v.Status == "BLOCKED" {
				e.Status = "BLOCKED"
			}
		}
		return e
	}
	if commitID.MatchString(revision) {
		e.SourceRevision = revision
		e.SourceClean = clean
	}
	if e.SourceRevision == "" || !clean {
		add("source", "NOT VERIFIED", "A clean committed source revision was not established")
	} else {
		add("source", "PASS", "Clean source revision recorded")
	}
	plan, err := CompileOperator(s, t, namespace)
	if err != nil {
		add("target", "BLOCKED", "Validated network and approved target binding do not match")
		return finish()
	}
	e.NetworkSHA256 = plan.NetworkSHA256
	count := 0
	for _, other := range registry.Targets {
		if other.AccountID == t.AccountID && other.Region == t.Region && other.ClusterName == t.ClusterName && other.RunnerNamespace == namespace {
			count++
			if TargetDigest(other) != TargetDigest(t) {
				count += 2
			}
		}
	}
	if count != 1 {
		add("registry-exclusivity", "BLOCKED", "Namespace must have exactly one matching administrator registry binding")
		return finish()
	}
	add("target", "PASS", "Validated network matches administrator target")
	add("registry-exclusivity", "PASS", "Exactly one matching namespace binding exists in the supplied registry")
	images := map[string]string{"worker": t.RunnerImage, "operator": t.Operator.OperatorImage, "smoke": t.Operator.Runtime.SmokeImage}
	for _, k := range []string{"FabricCA", "FabricPeer", "FabricOrdererNode"} {
		images[k] = t.Operator.Runtime.Images[k]
	}
	for k, v := range images {
		parts := strings.Split(v, "@sha256:")
		if len(parts) == 2 && regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(parts[1]) {
			if parts[1] == strings.Repeat("0", 64) {
				add("image-pins", "BLOCKED", "Placeholder image digests are forbidden")
				return finish()
			}
			e.ImageDigests[k] = "sha256:" + parts[1]
		}
	}
	add("image-pins", "PASS", "Image digest references match the administrator target; this does not verify signatures or registry contents")
	if c == nil {
		c = OSCommander{}
	}
	if OperatorPreflight(ctx, t, c) != nil {
		add("eks-preflight", "BLOCKED", "Live approved identity, cluster, storage, operator or permission preflight failed; no diagnostics exported")
		return finish()
	}
	add("eks-preflight", "PASS", "Bound identity, cluster, operator, permissions and storage metadata checks passed")
	kub, err := operatorAccess(ctx, t, c)
	if err != nil {
		add("identity-recheck", "BLOCKED", "Approved cluster identity could not be revalidated")
		return finish()
	}
	read := func(out any, args ...string) bool {
		b, err := kub(args...)
		return err == nil && json.Unmarshal(b, out) == nil
	}
	var ns struct {
		Metadata struct{ Labels map[string]string }
	}
	if !read(&ns, "get", "namespace", namespace, "-o", "json") || ns.Metadata.Labels["fabric.sathonate.io/network"] != record || ns.Metadata.Labels["fabric.sathonate.io/tenant"] != t.TenantID || ns.Metadata.Labels["fabric.sathonate.io/target"] != t.ID {
		add("namespace-labels", "BLOCKED", "All tenant, target and network ownership labels must match")
	} else {
		add("namespace-labels", "PASS", "All namespace ownership labels match")
	}
	var inventory struct{ Items []json.RawMessage }
	if !read(&inventory, "-n", namespace, "get", "pods,deployments,statefulsets,daemonsets,jobs,cronjobs,persistentvolumeclaims,fabriccas,fabricpeers,fabricorderernodes,fabricchaincodes", "-o", "json") || inventory.Items == nil {
		add("fresh-namespace", "BLOCKED", "Cannot establish an empty fresh-network namespace")
	} else if len(inventory.Items) != 0 {
		add("fresh-namespace", "BLOCKED", "Fresh qualification requires no existing workloads, volumes or Fabric resources")
	} else {
		add("fresh-namespace", "PASS", "No workloads, PVCs or Fabric resources found; registry binding is exclusive")
	}
	var quotas struct {
		Items []struct {
			Spec struct {
				Hard          map[string]string
				Scopes        []string
				ScopeSelector json.RawMessage
			}
		}
	}
	quota := map[string]bool{}
	if read(&quotas, "-n", namespace, "get", "resourcequotas", "-o", "json") {
		for _, q := range quotas.Items {
			if len(q.Spec.Scopes) != 0 || len(q.Spec.ScopeSelector) != 0 {
				continue
			}
			for k, v := range q.Spec.Hard {
				if regexp.MustCompile(`^[0-9]+(\.[0-9]+)?([EPTGMK]i?|[mun])?$`).MatchString(v) && strings.ContainsAny(v, "123456789") {
					quota[k] = true
				}
			}
		}
	}
	good := true
	for _, k := range []string{"requests.cpu", "requests.memory", "limits.cpu", "limits.memory", "pods", "persistentvolumeclaims", "requests.storage"} {
		good = good && quota[k]
	}
	if !good {
		add("quotas", "BLOCKED", "Unscoped positive CPU, memory, pod, PVC and storage quotas are required")
	} else {
		add("quotas", "PASS", "Required unscoped quota limits exist")
	}
	add("quota-capacity", "NOT VERIFIED", "Quota headroom and effective Fabric resource requests need a reviewed capacity calculation")
	var policies struct {
		Items []struct {
			Spec struct {
				PodSelector struct {
					MatchLabels      map[string]string
					MatchExpressions []json.RawMessage
				}
				PolicyTypes []string
				Ingress     []json.RawMessage
				Egress      []json.RawMessage
			}
		}
	}
	ingress, egress := false, false
	if read(&policies, "-n", namespace, "get", "networkpolicies", "-o", "json") {
		for _, p := range policies.Items {
			if len(p.Spec.PodSelector.MatchLabels) != 0 || len(p.Spec.PodSelector.MatchExpressions) != 0 {
				continue
			}
			for _, typ := range p.Spec.PolicyTypes {
				ingress = ingress || (typ == "Ingress" && len(p.Spec.Ingress) == 0)
				egress = egress || (typ == "Egress" && len(p.Spec.Egress) == 0)
			}
		}
	}
	if !ingress || !egress {
		add("default-deny", "BLOCKED", "Namespace-wide explicit ingress and egress default-deny policies are required")
	} else {
		add("default-deny", "PASS", "Namespace-wide ingress and egress default-deny objects exist")
	}
	add("isolation-enforcement", "NOT VERIFIED", "Additive allow rules, CNI enforcement, DNS, Fabric connectivity and cross-tenant denial require review and live probes")
	if ns.Metadata.Labels["pod-security.kubernetes.io/enforce"] != "restricted" || ns.Metadata.Labels["pod-security.kubernetes.io/enforce-version"] != "v1.34" {
		add("pod-admission", "BLOCKED", "Version-pinned restricted Pod Security enforcement is required for this verifier baseline")
	} else {
		add("pod-admission", "PASS", "Namespace requests version-pinned restricted Pod Security enforcement")
	}
	var admissionInventory struct{ Items []json.RawMessage }
	if !read(&admissionInventory, "get", "validatingadmissionpolicies,validatingadmissionpolicybindings,validatingwebhookconfigurations,mutatingwebhookconfigurations", "-o", "json") || admissionInventory.Items == nil {
		add("admission-inventory", "NOT VERIFIED", "Cluster admission policy and webhook inventory could not be read completely")
	} else {
		add("admission-inventory", "PASS", "Cluster admission policy and webhook inventory was readable; applicability and behavior are not established")
	}
	add("effective-admission", "NOT VERIFIED", "Admission exemptions, webhooks and policy engines are not proven by namespace labels; verify Fabric template compatibility before deployment")
	var csi struct {
		Spec   struct{ Replicas int }
		Status struct{ AvailableReplicas int }
	}
	if read(&csi, "-n", "kube-system", "get", "deployment", "ebs-csi-controller", "-o", "json") && csi.Spec.Replicas > 0 && csi.Status.AvailableReplicas == csi.Spec.Replicas {
		add("csi-controller-health", "PASS", "Standard EBS CSI controller deployment reports its requested replicas available; this is not attachment evidence")
	} else {
		add("csi-controller-health", "NOT VERIFIED", "Standard EBS CSI controller availability could not be established; alternative installations require explicit review")
	}
	var nodes struct {
		Items []struct {
			Metadata struct{ Labels map[string]string }
			Spec     struct {
				Unschedulable bool
				Taints        []struct{ Effect string }
			}
			Status struct {
				Conditions []struct{ Type, Status string }
			}
		}
	}
	placement := read(&nodes, "get", "nodes", "-o", "json")
	for _, selector := range t.Operator.Runtime.NodeSelectors {
		found := false
		for _, n := range nodes.Items {
			ok := !n.Spec.Unschedulable
			for k, v := range selector {
				ok = ok && n.Metadata.Labels[k] == v
			}
			for _, taint := range n.Spec.Taints {
				if taint.Effect == "NoSchedule" || taint.Effect == "NoExecute" {
					ok = false
				}
			}
			ready := false
			for _, condition := range n.Status.Conditions {
				ready = ready || (condition.Type == "Ready" && condition.Status == "True")
			}
			found = found || (ok && ready && n.Metadata.Labels["kubernetes.io/os"] == "linux" && n.Metadata.Labels["kubernetes.io/arch"] == "amd64")
		}
		placement = placement && found
	}
	if !placement {
		add("placement", "BLOCKED", "Every approved placement needs a ready uncordoned Linux amd64 node without blocking taints")
	} else {
		add("placement", "PASS", "Approved three-AZ topology has eligible nodes for the amd64 worker build")
	}
	for _, v := range []struct{ id, detail string }{{"scheduling-capacity", "Actual capacity, admission and volume topology scheduling remain unverified"}, {"registry-provenance", "Registry accessibility, signatures, approved build provenance and image architecture remain unverified"}, {"storage-attachment", "Encrypted EBS configuration was checked; CSI IAM, KMS access, attachment, recovery and retention remain unverified"}, {"namespace-access", "RBAC subjects, admission privilege escalation and controller isolation need administrator security review"}, {"fabric-behavior", "No workloads or transactions were run; EKS qualification is not complete"}} {
		add(v.id, "NOT VERIFIED", v.detail)
	}
	for _, v := range []struct{ id, detail string }{
		{"postgresql", "No explicit protected database TLS probe was requested"},
		{"database-operation-isolation", "Operation/deployment records rely on controller query scoping; runtime role isolation and hostile-tenant database tests require separate qualification"},
		{"storage-iam", "EBS driver IAM/KMS permissions, node attachment and actual volume encryption are not proven by StorageClass metadata"},
		{"image-pull", "Worker/node registry identity, private pulls, manifest architecture and pull-secret references are not verified by digest syntax"},
		{"certificate-lifecycle", "Password-bearing controller resources and live renewal/rotation/retirement are not eliminated or qualified"},
		{"controller-containment", "Cluster-channel and RBAC bootstrap privileges are not contained by worker namespace permissions"},
	} {
		add(v.id, "NOT VERIFIED", v.detail)
	}
	sort.Slice(e.Checks, func(i, j int) bool { return e.Checks[i].ID < e.Checks[j].ID })
	return finish()
}
