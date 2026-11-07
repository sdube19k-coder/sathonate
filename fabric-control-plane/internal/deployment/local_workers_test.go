package deployment

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sathonate.local/fabric-control-plane/internal/kubernetesfabric"
	"strings"
	"testing"
)

func TestTrustedLocalWorkerInventoryAndSingleNodeCompatibility(t *testing.T) {
	name := "smp-sprint1-a1b2c3d4"
	nodes := []string{name + "-worker", name + "-worker2", name + "-worker3"}
	config := func() SecureLocalConfig {
		selectors := map[string]map[string]string{}
		for _, n := range nodes {
			selectors[n] = map[string]string{"kubernetes.io/hostname": n}
		}
		return SecureLocalConfig{ClusterName: name, WorkerNodes: append([]string(nil), nodes...), Runtime: kubernetesfabric.Runtime{NodeSelectors: selectors}}
	}
	if validateLocalWorkerInventory(config()) != nil {
		t.Fatal("valid distinct worker inventory rejected")
	}
	for _, which := range []string{"duplicate", "foreign", "control-plane", "partial", "foreign-selector", "extra-selector", "missing-placement"} {
		c := config()
		switch which {
		case "duplicate":
			c.WorkerNodes[1] = c.WorkerNodes[0]
		case "foreign":
			c.WorkerNodes[0] = "smp-qualification-control-plane"
		case "control-plane":
			c.WorkerNodes[0] = name + "-control-plane"
		case "partial":
			c.WorkerNodes = c.WorkerNodes[:2]
		case "foreign-selector":
			c.Runtime.NodeSelectors[nodes[0]]["kubernetes.io/hostname"] = "foreign"
		case "extra-selector":
			c.Runtime.NodeSelectors[nodes[0]]["zone"] = "unapproved"
		case "missing-placement":
			delete(c.Runtime.NodeSelectors, nodes[2])
		}
		if validateLocalWorkerInventory(c) == nil {
			t.Fatal("untrusted inventory accepted", which)
		}
	}
	c := config()
	c.WorkerNodes = nil
	raw, _ := json.Marshal(c)
	if strings.Contains(string(raw), "workerNodes") || validateLocalWorkerInventory(c) != nil {
		t.Fatal("legacy target serialization or behavior changed")
	}
	c = config()
	c.Kubeconfig = filepath.Join(t.TempDir(), "config")
	os.WriteFile(c.Kubeconfig, []byte("public-test-placeholder"), 0600)
	c.WorkerImage = "example/worker@sha256:" + strings.Repeat("a", 64)
	runner, e := NewSecureRunner(c)
	if e != nil {
		t.Fatal(e)
	}
	if runner.Validate(Request{Network: secureDesign(t)}) == nil {
		t.Fatal("multi-node target enabled the fixed regression profile")
	}
	c.WorkerNodes[0] = "foreign"
	c.Runtime.NodeSelectors[nodes[0]]["kubernetes.io/hostname"] = "foreign"
	if runner.binding.config.WorkerNodes[0] != nodes[0] || runner.binding.config.Runtime.NodeSelectors[nodes[0]]["kubernetes.io/hostname"] != nodes[0] {
		t.Fatal("mutable trusted inventory shared")
	}
}
func TestWorkerProviderCannotAdoptAnotherCluster(t *testing.T) {
	cluster, node := "smp-sprint1-a1b2c3d4", "smp-sprint1-a1b2c3d4-worker"
	if !localWorkerProviderMatches(cluster, node, "kind://docker/"+cluster+"/"+node) {
		t.Fatal("owned worker rejected")
	}
	for _, id := range []string{"", "aws:///node", "kind://docker/smp-qualification/" + node, "kind://docker/" + cluster + "/foreign"} {
		if localWorkerProviderMatches(cluster, node, id) {
			t.Fatal("foreign provider accepted")
		}
	}
}
