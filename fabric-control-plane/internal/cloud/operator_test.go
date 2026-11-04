package cloud

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"sathonate.local/fabric-control-plane/internal/domain"
	"sathonate.local/fabric-control-plane/internal/kubernetesfabric"
)

func operatorFixture(t *testing.T) (domain.NetworkSpec, Target) {
	t.Helper()
	f, e := os.Open("../../examples/local-kubernetes-network.json")
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	s, e := domain.Decode(f)
	if e != nil {
		t.Fatal(e)
	}
	s.Spec.Deployment.Provider = "aws"
	s.Spec.Deployment.TargetRef = "eks"
	pin := "@sha256:" + strings.Repeat("a", 64)
	runtime := kubernetesfabric.Runtime{StorageClass: "gp3", Capacity: "10Gi", SmokeImage: "registry.example/smoke" + pin, BrokerChannels: true, Images: map[string]string{"FabricCA": "hyperledger/fabric-ca:1.5.15" + pin, "FabricPeer": "hyperledger/fabric-peer:2.5.15" + pin, "FabricOrdererNode": "hyperledger/fabric-orderer:2.5.15" + pin}, NodeSelectors: map[string]map[string]string{}}
	target := Target{ID: "eks", TenantID: "tenant-a", AccountID: "123456789012", Region: "ap-south-1", ClusterName: "smp", KubeContext: "smp-eks", RunnerNamespace: "smp-n-0123456789abcdef", StorageClass: "gp3", RunnerImage: "registry.example/worker" + pin, BevelCommit: kubernetesfabric.BevelCommit, FabricVersion: "2.5.15", Placements: map[string]Placement{}}
	for i := range s.Spec.Nodes {
		n := &s.Spec.Nodes[i]
		zone := fmt.Sprintf("ap-south-1%c", 'a'+i%3)
		n.PlacementRef = fmt.Sprintf("pool-%d", i)
		n.FailureDomain = zone
		selector := map[string]string{"topology.kubernetes.io/zone": zone}
		target.Placements[n.PlacementRef] = Placement{Zone: zone, NodeSelector: selector}
		runtime.NodeSelectors[n.ID] = selector
	}
	target.Operator = &OperatorTarget{Profile: "smp-role", RoleARN: "arn:aws:iam::123456789012:role/smp-controller", Kubeconfig: "approved-config", KubernetesMinor: "1.34", OperatorNamespace: "smp-system", OperatorDeployment: "smp-bevel", OperatorImage: "registry.example/operator" + pin, Runtime: runtime}
	raw, _ := json.Marshal(s)
	h := sha256.Sum256(raw)
	target.Operator.NetworkSHA256 = hex.EncodeToString(h[:])
	return s, target
}
func TestOperatorTranslationApprovalAndIsolation(t *testing.T) {
	s, target := operatorFixture(t)
	plan, e := CompileOperator(s, target, target.RunnerNamespace)
	if e != nil {
		t.Fatal(e)
	}
	if plan.BevelCommit != kubernetesfabric.BevelCommit || plan.Runtime.StorageClass != "gp3" || !reflect.DeepEqual(plan.Network, s) {
		t.Fatal("translation changed desired topology")
	}
	a := Approval{Purpose: "eks-qualification", TargetDigest: TargetDigest(target), AuthorizationRef: "change-123", ExpiresAt: time.Now().Add(time.Hour)}
	if e = a.Check(target, time.Now()); e != nil {
		t.Fatal(e)
	}
	changed := target
	changed.RunnerImage = "different"
	if a.Check(changed, time.Now()) == nil {
		t.Fatal("approval did not bind image")
	}
	if _, e = CompileOperator(s, target, "smp-n-ffffffffffffffff"); e == nil {
		t.Fatal("foreign namespace accepted")
	}
	s.Spec.Nodes[1].FailureDomain = "wrong"
	if _, e = CompileOperator(s, target, target.RunnerNamespace); e == nil {
		t.Fatal("placement mismatch accepted")
	}
	s, target = operatorFixture(t)
	target.BevelCommit = strings.Repeat("b", 40)
	if _, e = CompileOperator(s, target, target.RunnerNamespace); e == nil {
		t.Fatal("wrong Bevel revision accepted")
	}
	s, target = operatorFixture(t)
	s.Spec.FabricVersion = "2.5.0"
	if _, e = CompileOperator(s, target, target.RunnerNamespace); e == nil {
		t.Fatal("unsupported profile accepted")
	}
}

func TestOperatorPackageDoesNotUseLegacyAnsibleExporter(t *testing.T) {
	spec, target := operatorFixture(t)
	bundle, err := Prepare(spec, 1, target.TenantID, Registry{Targets: []Target{target}})
	if err != nil || bundle.OperatorPlan == nil || bundle.Bundle != nil || bundle.Executable {
		t.Fatal("operator target routed through legacy executable workflow", err)
	}
	if bundle.OperatorPlan.BevelCommit != kubernetesfabric.BevelCommit {
		t.Fatal("package lost pinned operator contract")
	}
}

// This simulates AWS/EKS APIs; it is not EKS deployment evidence.
type operatorCommands struct {
	mu     sync.Mutex
	target Target
	wrong  string
	calls  []string
}

func (c *operatorCommands) Run(_ context.Context, binary string, args []string) ([]byte, error) {
	line := binary + " " + strings.Join(args, " ")
	c.mu.Lock()
	c.calls = append(c.calls, line)
	c.mu.Unlock()
	t := c.target
	o := t.Operator
	encode := func(v any) ([]byte, error) { return json.Marshal(v) }
	switch {
	case strings.Contains(line, "auth can-i"):
		if c.wrong == "permission" {
			return []byte("no"), nil
		}
		return []byte("yes"), nil

	case strings.Contains(line, "get-caller-identity"):
		account := t.AccountID
		if c.wrong == "account" {
			account = "999999999999"
		}
		return encode(map[string]any{"Account": account, "Arn": "arn:aws:sts::" + account + ":assumed-role/smp-controller/session"})
	case strings.Contains(line, "describe-cluster"):
		return encode(map[string]any{"cluster": map[string]any{"name": t.ClusterName, "arn": "arn:aws:eks:" + t.Region + ":" + t.AccountID + ":cluster/" + t.ClusterName, "status": "ACTIVE", "endpoint": "https://eks.example.com", "version": "1.34", "certificateAuthority": map[string]any{"data": "public-ca"}}})
	case strings.Contains(line, "config view"):
		endpoint := "https://eks.example.com"
		if c.wrong == "endpoint" {
			endpoint = "https://foreign.example.com"
		}
		config, err := encode(map[string]any{"clusters": []any{map[string]any{"cluster": map[string]any{"server": endpoint, "certificate-authority-data": "public-ca"}}}, "users": []any{map[string]any{"user": map[string]any{"exec": map[string]any{"apiVersion": "client.authentication.k8s.io/v1beta1", "command": "aws", "args": []any{"--profile", o.Profile, "--region", t.Region, "eks", "get-token", "--cluster-name", t.ClusterName}, "interactiveMode": "Never", "provideClusterInfo": false}}}}})
		if c.wrong == "exec" {
			config = []byte(strings.Replace(string(config), `"command":"aws"`, `"command":"powershell"`, 1))
		}
		return config, err
	case strings.Contains(line, "version -o"):
		return []byte(`{"serverVersion":{"major":"1","minor":"34"}}`), nil
	case strings.Contains(line, "get namespace"):
		return encode(map[string]any{"metadata": map[string]any{"labels": map[string]string{"fabric.sathonate.io/tenant": t.TenantID, "fabric.sathonate.io/target": t.ID}}})
	case strings.Contains(line, "get storageclass"):
		return []byte(`{"provisioner":"ebs.csi.aws.com","volumeBindingMode":"WaitForFirstConsumer","parameters":{"encrypted":"true"}}`), nil
	case strings.Contains(line, "get deployment"):
		return encode(map[string]any{"metadata": map[string]any{"annotations": map[string]string{"fabric.sathonate.io/bevel-commit": kubernetesfabric.BevelCommit}}, "spec": map[string]any{"template": map[string]any{"spec": map[string]any{"containers": []any{map[string]any{"image": o.OperatorImage}}}}}, "status": map[string]any{"availableReplicas": 1}})
	case strings.Contains(line, "get nodes"):
		items := []any{}
		for _, selectors := range o.Runtime.NodeSelectors {
			items = append(items, map[string]any{"metadata": map[string]any{"labels": selectors}, "status": map[string]any{"conditions": []any{map[string]any{"type": "Ready", "status": "True"}}}})
		}
		return encode(map[string]any{"items": items})
	}
	return []byte(`{}`), nil
}
func TestOperatorPreflightUsesOnlyBoundReadOnlyCalls(t *testing.T) {
	_, target := operatorFixture(t)
	for _, wrong := range []string{"", "account", "endpoint", "exec", "permission"} {
		t.Run(wrong, func(t *testing.T) {
			commands := &operatorCommands{target: target, wrong: wrong}
			err := OperatorPreflight(context.Background(), target, commands)
			if (err == nil) != (wrong == "") {
				t.Fatal(err)
			}
			for _, call := range commands.calls {
				for _, bad := range []string{" create ", " apply ", " delete ", "assume-role", "get-secret-value"} {
					if strings.Contains(call, bad) && !strings.Contains(call, "auth can-i") {
						t.Fatal("mutating preflight", call)
					}
				}
			}
			if wrong == "account" && len(commands.calls) != 1 {
				t.Fatal("foreign identity was used")
			}
			if (wrong == "endpoint" || wrong == "exec") && len(commands.calls) != 3 {
				t.Fatal("foreign endpoint was contacted")
			}
		})
	}
}
