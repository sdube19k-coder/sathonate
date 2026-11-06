package deployment

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// Supplementary schema/projection proof only. Every command is a GET against
// the retained fixture. This is not acceptance of the new HTTP/Kube binding.
func TestLiveReadOnlyWorkloadProjection(t *testing.T) {
	if os.Getenv("SMP_WORKLOAD_READ_ONLY_E2E") != "1" {
		t.Skip("requires explicit read-only retained inventory qualification")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	base := []string{"--kubeconfig", "E:/Sathonate/.cache/smp-qualification.kubeconfig", "--context", "kind-smp-qualification", "--request-timeout=15s"}
	get := func(args ...string) []byte {
		t.Helper()
		out, e := exec.CommandContext(ctx, "E:/Sathonate/tools/kubectl-1.34/kubectl.exe", append(base, args...)...).Output()
		if e != nil {
			t.Fatal("read-only inventory GET failed; private diagnostics withheld")
		}
		return out
	}
	if strings.TrimSpace(string(get("get", "node", "smp-qualification-control-plane", "-o", "jsonpath={.spec.providerID}"))) != "kind://docker/smp-qualification/smp-qualification-control-plane" {
		t.Fatal("retained read-only fixture identity mismatch")
	}
	namespace := "smp-n-968a212068be7d07"
	var deployments struct {
		Items []struct {
			Metadata struct{ Name string }
			Status   struct{ ReadyReplicas int }
		}
	}
	if json.Unmarshal(get("-n", namespace, "get", "deployments", "-o", "json"), &deployments) != nil || len(deployments.Items) != 8 {
		t.Fatal("retained inventory shape changed; do not mutate")
	}
	expected := map[string]string{}
	for _, d := range deployments.Items {
		if d.Status.ReadyReplicas != 1 {
			t.Fatal("retained workload not ready; do not mutate")
		}
		expected[d.Metadata.Name] = "retained-component"
	}
	snapshot, e := projectWorkloads(namespace, expected, get("-n", namespace, "get", "pods", "-o", "json"), get("-n", namespace, "get", "persistentvolumeclaims", "-o", "json"))
	if e != nil || !snapshot.WorkloadsReady || len(snapshot.Pods) != 8 || len(snapshot.Volumes) != 7 || snapshot.LedgerChecked {
		t.Fatal("read-only live projection did not match retained readiness", e)
	}
	if path := os.Getenv("SMP_WORKLOAD_READ_ONLY_EVIDENCE"); path != "" {
		raw, _ := json.MarshalIndent(map[string]any{"schema": "sathonate.sprint5-read-only-projection/v1", "status": "PASS_PROJECTION_ONLY", "apiBindingQualified": false, "commands": "GET_ONLY", "mutations": false, "observation": snapshot}, "", "  ")
		if os.WriteFile(path, raw, 0600) != nil {
			t.Fatal("public projection evidence could not be saved")
		}
	}
	t.Log("Read-only live projection: eight ready workloads and seven bound claims; ledger not queried")
}
