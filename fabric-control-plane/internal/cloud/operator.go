package cloud

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"time"

	"sathonate.local/fabric-control-plane/internal/domain"
	"sathonate.local/fabric-control-plane/internal/kubernetesfabric"
)

// OperatorTarget is an administrator-owned binding. AWS profile must itself
// assume RoleARN; customer documents cannot supply a profile or kubeconfig.
type OperatorTarget struct {
	NetworkSHA256      string                   `json:"networkSHA256"`
	Profile            string                   `json:"profile"`
	RoleARN            string                   `json:"roleArn"`
	Kubeconfig         string                   `json:"kubeconfig"`
	KubernetesMinor    string                   `json:"kubernetesMinor"`
	OperatorNamespace  string                   `json:"operatorNamespace"`
	OperatorDeployment string                   `json:"operatorDeployment"`
	OperatorImage      string                   `json:"operatorImage"`
	Runtime            kubernetesfabric.Runtime `json:"runtime"`
}
type Approval struct {
	TargetDigest     string    `json:"targetSHA256"`
	AuthorizationRef string    `json:"authorizationRef"`
	ExpiresAt        time.Time `json:"expiresAt"`
	Purpose          string    `json:"purpose"`
}
type OperatorPlan struct {
	BevelCommit   string                   `json:"bevelCommit"`
	Namespace     string                   `json:"namespace"`
	NetworkSHA256 string                   `json:"networkSHA256"`
	Runtime       kubernetesfabric.Runtime `json:"runtime"`
	Network       domain.NetworkSpec       `json:"network"`
}

func TargetDigest(t Target) string {
	raw, _ := json.Marshal(t)
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}
func (a Approval) Check(t Target, now time.Time) error {
	if a.Purpose != "eks-qualification" || a.TargetDigest != TargetDigest(t) || !name.MatchString(a.AuthorizationRef) || !a.ExpiresAt.After(now) || a.ExpiresAt.After(now.Add(7*24*time.Hour)) {
		return fmt.Errorf("a current administrator approval bound to this exact target is required")
	}
	return nil
}
func CompileOperator(s domain.NetworkSpec, t Target, namespace string) (OperatorPlan, error) {
	fail := func() (OperatorPlan, error) {
		return OperatorPlan{}, fmt.Errorf("network does not match the approved EKS operator target")
	}
	if err := kubernetesfabric.ValidateProfile(s); err != nil {
		return OperatorPlan{}, err
	}
	o := t.Operator
	if o == nil || t.TenantID == "" || !name.MatchString(t.ID) || !regexp.MustCompile(`^smp-n-[a-f0-9]{16}$`).MatchString(namespace) || t.RunnerNamespace != namespace || t.BevelCommit != kubernetesfabric.BevelCommit || t.FabricVersion != s.Spec.FabricVersion || s.Spec.Deployment.Engine != "bevel" || s.Spec.Deployment.Provider != "aws" || s.Spec.Deployment.Runtime != "kubernetes" || s.Spec.Deployment.TargetRef != t.ID {
		return fail()
	}
	if !regexp.MustCompile(`^[0-9]{12}$`).MatchString(t.AccountID) || !regexp.MustCompile(`^[a-z]{2}-[a-z]+-[0-9]+$`).MatchString(t.Region) || !name.MatchString(t.ClusterName) || !name.MatchString(t.KubeContext) || !name.MatchString(o.Profile) || o.Kubeconfig == "" || o.KubernetesMinor != "1.34" || !name.MatchString(o.OperatorNamespace) || !name.MatchString(o.OperatorDeployment) || !digestImage.MatchString(o.OperatorImage) || !digestImage.MatchString(t.RunnerImage) {
		return fail()
	}
	if !regexp.MustCompile(`^arn:aws:iam::` + t.AccountID + `:role/[A-Za-z0-9+=,.@_/-]+$`).MatchString(o.RoleARN) {
		return fail()
	}
	if t.StorageClass != o.Runtime.StorageClass {
		return fail()
	}
	if err := o.Runtime.Validate(s); err != nil {
		return OperatorPlan{}, err
	}
	if c := s.Spec.Deployment.Cloud; c != nil {
		if c.ClusterMode != "existing" || c.AccountID != t.AccountID || c.Region != t.Region || c.ClusterName != t.ClusterName || (c.RoleARN != "" && c.RoleARN != o.RoleARN) {
			return fail()
		}
	}
	zones := map[string]bool{}
	for _, n := range s.Spec.Nodes {
		p, ok := t.Placements[n.PlacementRef]
		selectors := o.Runtime.NodeSelectors[n.ID]
		if !ok || p.Zone == "" || n.FailureDomain != p.Zone || selectors["topology.kubernetes.io/zone"] != p.Zone || !reflect.DeepEqual(selectors, p.NodeSelector) {
			return fail()
		}
		if n.Role == "orderer" {
			zones[p.Zone] = true
		}
	}
	if len(zones) != 3 {
		return OperatorPlan{}, fmt.Errorf("three Raft orderers require three approved availability zones")
	}
	raw, _ := json.Marshal(s)
	sum := sha256.Sum256(raw)
	if o.NetworkSHA256 != hex.EncodeToString(sum[:]) {
		return OperatorPlan{}, fmt.Errorf("saved network does not match the approved target network digest")
	}
	return OperatorPlan{BevelCommit: t.BevelCommit, Namespace: namespace, NetworkSHA256: hex.EncodeToString(sum[:]), Runtime: o.Runtime, Network: s}, nil
}

// OperatorPreflight is read-only. No caller diagnostics or credential output is
// returned. The exact Kubernetes minor is deliberately bounded to the local test
// matrix, not a claim of compatibility with every EKS release.
func operatorAccess(ctx context.Context, t Target, c Commander) (func(...string) ([]byte, error), error) {
	if t.Operator == nil {
		return nil, fmt.Errorf("operator target is missing")
	}
	o := t.Operator
	run := func(binary string, args ...string) ([]byte, error) {
		b, e := c.Run(ctx, binary, args)
		if e != nil {
			return nil, fmt.Errorf("preflight command failed; check scoped identity, connectivity and permissions")
		}
		return b, nil
	}
	aws := func(args ...string) ([]byte, error) {
		return run("aws", append([]string{"--profile", o.Profile, "--region", t.Region, "--no-cli-pager"}, args...)...)
	}
	var who struct{ Account, Arn string }
	b, e := aws("sts", "get-caller-identity")
	if e != nil || json.Unmarshal(b, &who) != nil {
		return nil, fmt.Errorf("cannot verify approved assumed-role identity")
	}
	role := o.RoleARN[strings.LastIndex(o.RoleARN, "/")+1:]
	if who.Account != t.AccountID || !strings.HasPrefix(who.Arn, "arn:aws:sts::"+t.AccountID+":assumed-role/"+role+"/") {
		return nil, fmt.Errorf("AWS identity is not the approved assumed role")
	}
	var eks struct {
		Cluster struct {
			Name, Arn, Endpoint, Status, Version string
			CertificateAuthority                 struct{ Data string }
		}
	}
	b, e = aws("eks", "describe-cluster", "--name", t.ClusterName)
	if e != nil || json.Unmarshal(b, &eks) != nil || eks.Cluster.Name != t.ClusterName || eks.Cluster.Arn != "arn:aws:eks:"+t.Region+":"+t.AccountID+":cluster/"+t.ClusterName || eks.Cluster.Status != "ACTIVE" || !httpsURL(eks.Cluster.Endpoint) || eks.Cluster.CertificateAuthority.Data == "" {
		return nil, fmt.Errorf("active EKS cluster, CA and Kubernetes version could not be verified")
	}
	kub := func(args ...string) ([]byte, error) {
		return run("kubectl", append([]string{"--kubeconfig", o.Kubeconfig, "--context", t.KubeContext, "--request-timeout=20s"}, args...)...)
	}
	// Verify exec configuration before invoking any authenticated Kubernetes call.
	b, e = kub("config", "view", "--minify", "--raw", "-o", "json")
	if e != nil {
		return nil, e
	}
	var config struct {
		Clusters []struct {
			Cluster struct {
				Server   string
				CA       string `json:"certificate-authority-data"`
				Insecure bool   `json:"insecure-skip-tls-verify"`
			}
		}
		Users []struct{ User map[string]any }
	}
	if json.Unmarshal(b, &config) != nil || len(config.Clusters) != 1 || len(config.Users) != 1 || config.Clusters[0].Cluster.Server != eks.Cluster.Endpoint || config.Clusters[0].Cluster.CA != eks.Cluster.CertificateAuthority.Data || config.Clusters[0].Cluster.Insecure {
		return nil, fmt.Errorf("kubeconfig endpoint/CA is not bound to approved EKS")
	}
	expected := map[string]any{"exec": map[string]any{"apiVersion": "client.authentication.k8s.io/v1beta1", "command": "aws", "args": []any{"--profile", o.Profile, "--region", t.Region, "eks", "get-token", "--cluster-name", t.ClusterName}, "interactiveMode": "Never", "provideClusterInfo": false}}
	if configExec, ok := config.Users[0].User["exec"].(map[string]any); ok {
		if env, exists := configExec["env"]; exists {
			if env == nil {
				delete(configExec, "env")
			} else if values, ok := env.([]any); ok && len(values) == 0 {
				delete(configExec, "env")
			}
		}
	}
	if !reflect.DeepEqual(config.Users[0].User, expected) {
		return nil, fmt.Errorf("kubeconfig must use the exact approved AWS exec identity; static credentials and other commands are forbidden")
	}
	return kub, nil
}

// OperatorAccess verifies identity and cluster binding without requiring healthy
// storage/operator/nodes. Cancellation and diagnosis must work during failures.
func OperatorAccess(ctx context.Context, t Target, c Commander) error {
	_, err := operatorAccess(ctx, t, c)
	return err
}
func OperatorPreflight(ctx context.Context, t Target, c Commander) error {
	kub, e := operatorAccess(ctx, t, c)
	if e != nil {
		return e
	}
	o := t.Operator
	var b []byte
	b, e = kub("version", "-o", "json")
	var version struct{ ServerVersion struct{ Major, Minor string } }
	if e != nil || json.Unmarshal(b, &version) != nil || version.ServerVersion.Major+"."+strings.TrimSuffix(version.ServerVersion.Minor, "+") != o.KubernetesMinor {
		return fmt.Errorf("Kubernetes server is outside the tested minor")
	}
	if _, e = kub("get", "--raw=/readyz"); e != nil {
		return e
	}
	b, e = kub("get", "namespace", t.RunnerNamespace, "-o", "json")
	var ns struct {
		Metadata struct{ Labels map[string]string }
	}
	if e != nil || json.Unmarshal(b, &ns) != nil || ns.Metadata.Labels["fabric.sathonate.io/tenant"] != t.TenantID || ns.Metadata.Labels["fabric.sathonate.io/target"] != t.ID {
		return fmt.Errorf("namespace is not administrator-approved for this tenant and target")
	}
	b, e = kub("get", "storageclass", t.StorageClass, "-o", "json")
	var storage struct {
		Provisioner, VolumeBindingMode string
		Parameters                     map[string]string
	}
	if e != nil || json.Unmarshal(b, &storage) != nil || storage.Provisioner != "ebs.csi.aws.com" || storage.VolumeBindingMode != "WaitForFirstConsumer" || storage.Parameters["encrypted"] != "true" {
		return fmt.Errorf("encrypted EBS CSI delayed-binding storage is required")
	}
	if _, e = kub("get", "csidriver", "ebs.csi.aws.com"); e != nil {
		return fmt.Errorf("EBS CSI driver is not registered")
	}
	for _, r := range []string{"fabriccas", "fabricpeers", "fabricorderernodes", "fabricmainchannels", "fabricfollowerchannels", "fabricchaincodes"} {
		if _, e = kub("get", "crd", r+".hlf.kungfusoftware.es"); e != nil {
			return fmt.Errorf("pinned Bevel CRDs are not installed")
		}
	}
	b, e = kub("-n", o.OperatorNamespace, "get", "deployment", o.OperatorDeployment, "-o", "json")
	var operator struct {
		Metadata struct{ Annotations map[string]string }
		Spec     struct {
			Template struct {
				Spec struct{ Containers []struct{ Image string } }
			}
		}
		Status struct{ AvailableReplicas int }
	}
	if e != nil || json.Unmarshal(b, &operator) != nil || operator.Metadata.Annotations["fabric.sathonate.io/bevel-commit"] != kubernetesfabric.BevelCommit || operator.Status.AvailableReplicas < 1 || len(operator.Spec.Template.Spec.Containers) != 1 || operator.Spec.Template.Spec.Containers[0].Image != o.OperatorImage {
		return fmt.Errorf("running digest-pinned Bevel operator revision is not verified")
	}
	checks := [][]string{{"auth", "can-i", "get", "pods/log", "-n", t.RunnerNamespace}}
	for _, resource := range []string{"fabriccas.hlf.kungfusoftware.es", "fabricpeers.hlf.kungfusoftware.es", "fabricorderernodes.hlf.kungfusoftware.es", "fabricchaincodes.hlf.kungfusoftware.es", "secrets", "services", "pods", "deployments.apps", "serviceaccounts", "configmaps", "jobs.batch", "roles.rbac.authorization.k8s.io", "rolebindings.rbac.authorization.k8s.io"} {
		verbs := []string{"get", "list", "watch"}
		if resource != "services" && resource != "pods" && resource != "deployments.apps" {
			verbs = append(verbs, "create", "patch", "update", "delete")
		}
		for _, verb := range verbs {
			checks = append(checks, []string{"auth", "can-i", verb, resource, "-n", t.RunnerNamespace})
		}
	}
	for _, resource := range []string{"fabricmainchannels.hlf.kungfusoftware.es", "fabricfollowerchannels.hlf.kungfusoftware.es", "clusterroles.rbac.authorization.k8s.io", "clusterrolebindings.rbac.authorization.k8s.io"} {
		for _, verb := range []string{"get", "list", "create", "patch", "watch", "delete"} {
			checks = append(checks, []string{"auth", "can-i", verb, resource})
		}
	}
	// Bound subprocess concurrency so a complete permission check does not require
	// a serial AWS token process for every permission.
	for offset := 0; offset < len(checks); offset += 4 {
		end := offset + 4
		if end > len(checks) {
			end = len(checks)
		}
		results := make(chan error, end-offset)
		for _, args := range checks[offset:end] {
			go func(args []string) {
				raw, err := kub(args...)
				if err == nil && strings.TrimSpace(string(raw)) != "yes" {
					err = fmt.Errorf("controller lacks required resource/RBAC permissions")
				}
				results <- err
			}(args)
		}
		failed := false
		for i := offset; i < end; i++ {
			if err := <-results; err != nil {
				failed = true
			}
		}
		if failed {
			return fmt.Errorf("controller lacks required resource/RBAC permissions")
		}
	}
	b, e = kub("get", "nodes", "-o", "json")
	var nodes struct {
		Items []struct {
			Metadata struct{ Labels map[string]string }
			Spec     struct{ Unschedulable bool }
			Status   struct {
				Conditions []struct{ Type, Status string }
			}
		}
	}
	if e != nil || json.Unmarshal(b, &nodes) != nil {
		return fmt.Errorf("cannot verify approved scheduling zones")
	}
	for _, selector := range o.Runtime.NodeSelectors {
		found := false
		for _, node := range nodes.Items {
			match := !node.Spec.Unschedulable
			for k, v := range selector {
				match = match && node.Metadata.Labels[k] == v
			}
			ready := false
			for _, condition := range node.Status.Conditions {
				if condition.Type == "Ready" && condition.Status == "True" {
					ready = true
				}
			}
			found = found || (match && ready)
		}
		if !found {
			return fmt.Errorf("approved placement has no ready schedulable node")
		}
	}
	return nil
}
