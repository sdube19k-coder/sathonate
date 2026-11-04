package deployment

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"sathonate.local/fabric-control-plane/internal/cloud"
	"sathonate.local/fabric-control-plane/internal/kubernetesfabric"
)

// This is REAL Fabric on kind using the EKS manifest, scoped worker RBAC,
// channel broker and image/runtime translation. It deliberately does not invoke
// EKS preflight or claim AWS IAM/EBS/AZ qualification.
func TestEKSWorkerLifecycleOnKind(t *testing.T) {
	if os.Getenv("SMP_EKS_LOCAL_E2E") != "1" {
		t.Skip("opt in to real local Fabric execution with SMP_EKS_LOCAL_E2E=1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	kube := os.Getenv("SMP_TEST_KUBECONFIG")
	if kube == "" {
		t.Fatal("explicit SMP_TEST_KUBECONFIG is required")
	}
	local := KubernetesRunner{Kubeconfig: kube, Image: "sathonate/fabric-worker:eks-scoped"}
	if err := local.validateLocal(ctx); err != nil {
		t.Fatal(err)
	}
	id := make([]byte, 16)
	_, _ = rand.Read(id)
	request := Request{TenantID: "eks-local-test", NetworkID: "eks-test-" + hex.EncodeToString(id[:8]), OperationID: "op_" + hex.EncodeToString(id), Network: fixture(t)}
	ns := Namespace(request.TenantID, request.NetworkID)
	runtime := kubernetesfabric.Runtime{StorageClass: "standard", Capacity: "1Gi", BrokerChannels: true, NodeSelectors: map[string]map[string]string{}, Images: map[string]string{
		"FabricCA":          "hyperledger/fabric-ca:1.5.15@sha256:1b64dc702bd400e14c8f452dedc0359b7dc06679b751b08937eb2bf2865fed40",
		"FabricPeer":        "hyperledger/fabric-peer:2.5.15@sha256:c48323777139fcabe77a98cf4eeaf4466827dec225f754064f52f22395a8bd89",
		"FabricOrdererNode": "hyperledger/fabric-orderer:2.5.15@sha256:72697d099f45b8e57f86d9a69fbedb683cd5888bc7b28ec01695773d0433b791",
	}}
	// Resolve the preloaded smoke manifest digest, not the image config digest.
	out, e := exec.CommandContext(ctx, "docker", "exec", "smp-qualification-control-plane", "ctr", "-n", "k8s.io", "images", "list").Output()
	if e != nil {
		t.Fatal(e)
	}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 2 && fields[0] == "docker.io/sathonate/smp-smoke:local" {
			runtime.SmokeImage = "sathonate/smp-smoke@" + fields[2]
			_, e = exec.CommandContext(ctx, "docker", "exec", "smp-qualification-control-plane", "ctr", "-n", "k8s.io", "images", "tag", "--force", fields[0], "docker.io/"+runtime.SmokeImage).CombinedOutput()
			if e != nil {
				t.Fatal("cannot register preloaded digest alias", e)
			}
		}
	}
	for _, n := range request.Network.Spec.Nodes {
		runtime.NodeSelectors[n.ID] = map[string]string{"kubernetes.io/hostname": "smp-qualification-control-plane"}
	}
	if e = runtime.Validate(request.Network); e != nil {
		t.Fatal(e)
	}
	target := cloud.Target{ID: "eks-local-test", TenantID: request.TenantID, KubeContext: "kind-smp-qualification", RunnerNamespace: ns, RunnerImage: local.Image, Operator: &cloud.OperatorTarget{Runtime: runtime}}
	runner := KubernetesRunner{Kubeconfig: kube, Image: local.Image, eks: &eksBinding{target: target}}
	// Only the dedicated kind loopback target was verified above. No cloud call.
	type M = map[string]any
	if e = local.apply(ctx, M{"apiVersion": "v1", "kind": "Namespace", "metadata": M{"name": ns, "labels": M{"fabric.sathonate.io/network": request.NetworkID, "fabric.sathonate.io/tenant": request.TenantID, "fabric.sathonate.io/target": target.ID}}}); e != nil {
		t.Fatal(e)
	}
	defer func() {
		cleanup, c := context.WithTimeout(context.Background(), 2*time.Minute)
		defer c()
		_, err := local.run(cleanup, "delete", "fabricmainchannels,fabricfollowerchannels", "-l", "sathonate.io/namespace="+ns, "--ignore-not-found", "--timeout=60s")
		if err != nil {
			t.Error(err)
		}
		_, err = local.run(cleanup, "delete", "namespace", ns, "--ignore-not-found", "--timeout=60s")
		if err != nil {
			t.Error(err)
		}
		_, _ = local.run(cleanup, "delete", "clusterrole,clusterrolebinding", ns+"-channels", "--ignore-not-found")
	}()
	name, _ := jobName(request.OperationID)
	manifest, e := runner.eksManifest(request, name)
	if e != nil {
		t.Fatal(e)
	}
	if e = runner.apply(ctx, manifest); e != nil {
		t.Fatal(e)
	}
	for _, check := range [][]string{{"create", "fabricmainchannels.hlf.kungfusoftware.es"}, {"list", "fabricmainchannels.hlf.kungfusoftware.es"}, {"get", "secrets", "-n", "default"}} {
		args := append([]string{"auth", "can-i", "--as=system:serviceaccount:" + ns + ":smp-worker"}, check...)
		b, _ := local.command(ctx, args...).Output()
		if strings.TrimSpace(string(b)) != "no" {
			t.Fatal("scoped worker gained forbidden permission", string(b))
		}
	}
	// Exercise actual authorization, rather than relying only on can-i discovery.
	for _, check := range [][]string{{"-n", "default", "get", "secret", "smp-isolation-nonexistent"}, {"get", "fabricmainchannel", "smp-isolation-nonexistent"}} {
		args := append([]string{"--as=system:serviceaccount:" + ns + ":smp-worker"}, check...)
		output, denied := local.command(ctx, args...).CombinedOutput()
		if denied == nil || !strings.Contains(strings.ToLower(string(output)), "forbidden") {
			t.Fatal("effective cross-namespace or channel read denial not verified")
		}
	}
	t.Log("Actual worker cross-namespace Secret and cluster channel GET requests forbidden")
	stages := []string{}
	before, e := runner.run(ctx, "-n", ns, "get", "job", name, "-o", "jsonpath={.metadata.uid}")
	if e != nil {
		t.Fatal(e)
	}
	disconnected := errors.New("test controller detached")
	_, e = runner.watch(ctx, request, name, ns, func(stage, message string) error {
		if stage == "ca" {
			return disconnected
		}
		return nil
	})
	if !errors.Is(e, disconnected) {
		t.Fatal("did not exercise controller detachment", e)
	}
	outcome, e := runner.watch(ctx, request, name, ns, func(stage, message string) error {
		if len(stages) == 0 || stages[len(stages)-1] != stage {
			stages = append(stages, stage)
			t.Log(stage, message)
		}
		return nil
	})
	if e != nil {
		b, _ := local.run(ctx, "-n", ns, "logs", "job/"+name)
		t.Log(string(b))
		t.Fatal(e)
	}
	if e = verifyEvidence(request.Network, outcome); e != nil {
		t.Fatal(e)
	}
	verifyEnrollmentSecretIsolation(t, ctx, local, ns, name)
	verifyPreciseCleanupProbe(t, ctx, runner, request)
	health, e := runner.Health(ctx, request.TenantID, request.NetworkID, outcome.Health)
	if e != nil || !health.Ready {
		t.Fatal("live workload readiness did not corroborate transaction evidence", e)
	}
	for _, node := range request.Network.Spec.Nodes {
		raw, err := runner.run(ctx, "-n", ns, "get", "deployment", node.ID, "-o", "json")
		if err != nil {
			t.Fatal(err)
		}
		var actual struct {
			Spec struct {
				Template struct {
					Spec struct {
						Containers []struct{ Image string }
						Affinity   map[string]any
					}
				}
			}
		}
		if json.Unmarshal(raw, &actual) != nil {
			t.Fatal("invalid workload observation")
		}
		kind := "FabricPeer"
		if node.Role == "orderer" {
			kind = "FabricOrdererNode"
		}
		pinned := false
		for _, container := range actual.Spec.Template.Spec.Containers {
			pinned = pinned || container.Image == runtime.Images[kind]
		}
		if !pinned || actual.Spec.Template.Spec.Affinity["nodeAffinity"] == nil {
			t.Fatal("operator did not enforce pinned image and placement", node.ID)
		}
	}
	after, e := runner.run(ctx, "-n", ns, "get", "job", name, "-o", "jsonpath={.metadata.uid}")
	if e != nil || string(after) != string(before) {
		t.Fatal("controller recovery replaced the worker Job", e)
	}
	// Reattach to a finished Job, consuming the same real evidence without creating
	// a second Job or a second Fabric topology.
	replay, e := runner.watch(ctx, request, name, ns, func(string, string) error { return nil })
	if e != nil || replay.TransactionID != outcome.TransactionID {
		t.Fatal("job evidence adoption failed", e)
	}
	raw, _ := json.Marshal(map[string]any{"operationId": request.OperationID, "namespace": ns, "transactionId": outcome.TransactionID, "height": outcome.Health.Peers[0].Height, "awsVerified": false})
	fmt.Println(string(raw))
}

func TestLocalWorkerRegressionOnKind(t *testing.T) {
	if os.Getenv("SMP_EKS_LOCAL_E2E") != "1" {
		t.Skip("opt in with SMP_EKS_LOCAL_E2E=1")
	}
	kube := os.Getenv("SMP_TEST_KUBECONFIG")
	if kube == "" {
		t.Fatal("explicit local kubeconfig required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	runner := KubernetesRunner{Kubeconfig: kube, Image: "sathonate/fabric-worker:eks-scoped"}
	id := make([]byte, 16)
	_, _ = rand.Read(id)
	request := Request{TenantID: "local-regression", NetworkID: "regression-" + hex.EncodeToString(id[:8]), OperationID: "op_" + hex.EncodeToString(id), Network: fixture(t)}
	defer func() {
		cleanup, c := context.WithTimeout(context.Background(), 2*time.Minute)
		defer c()
		if e := runner.Cleanup(cleanup, request.TenantID, request.NetworkID); e != nil {
			t.Error(e)
		}
	}()
	outcome, e := runner.Run(ctx, request, func(stage, message string) error { t.Log(stage, message); return nil })
	if e != nil {
		t.Fatal(e)
	}
	if e = verifyEvidence(request.Network, outcome); e != nil {
		t.Fatal(e)
	}
	name, _ := jobName(request.OperationID)
	verifyEnrollmentSecretIsolation(t, ctx, runner, Namespace(request.TenantID, request.NetworkID), name)
	fmt.Printf("local regression transaction=%s height=%d\n", outcome.TransactionID, outcome.Health.Peers[0].Height)
}

// Secret payloads are inspected in memory only and are never printed on failure.
func verifyEnrollmentSecretIsolation(t *testing.T, ctx context.Context, runner KubernetesRunner, ns, job string) {
	t.Helper()
	raw, err := runner.run(ctx, "-n", ns, "get", "secrets", "-o", "json")
	if err != nil {
		t.Fatal("cannot inspect enrollment secret isolation")
	}
	var values struct {
		Items []struct {
			Metadata struct{ Name string }
			Data     map[string]string
		}
	}
	if json.Unmarshal(raw, &values) != nil {
		t.Fatal("invalid secret inventory")
	}
	seen := map[string]bool{}
	credentials := []string{}
	for _, item := range values.Items {
		if !strings.HasPrefix(item.Metadata.Name, "smp-enrollment-") {
			continue
		}
		value := item.Data["password"]
		if value == "" || seen[value] {
			t.Fatal("enrollment credentials missing or reused")
		}
		seen[value] = true
		decoded, e := base64.StdEncoding.DecodeString(value)
		if e != nil || len(decoded) < 32 {
			t.Fatal("invalid enrollment credential")
		}
		credentials = append(credentials, string(decoded), value)
	}
	if len(seen) != 10 {
		t.Fatal("expected distinct registrar, node, administrator and contract credentials")
	}
	logs, err := runner.run(ctx, "-n", ns, "logs", "job/"+job)
	if err != nil {
		t.Fatal("cannot verify worker log confidentiality")
	}
	manifest, err := runner.run(ctx, "-n", ns, "get", "job", job, "-o", "json")
	if err != nil {
		t.Fatal("cannot verify worker job confidentiality")
	}
	for _, credential := range credentials {
		if bytes.Contains(logs, []byte(credential)) || bytes.Contains(manifest, []byte(credential)) {
			t.Fatal("enrollment credential leaked to worker log or job")
		}
	}
	t.Log("Verified ten distinct enrollment credentials; worker logs and Job manifest contain none")
}

// Exercise the conditional-delete transport only on newly created test ConfigMaps.
func verifyPreciseCleanupProbe(t *testing.T, ctx context.Context, runner KubernetesRunner, request Request) {
	t.Helper()
	ns := Namespace(request.TenantID, request.NetworkID)
	for _, entry := range []struct{ name, network string }{{"smp-cleanup-probe", request.NetworkID}, {"unrelated-cleanup-probe", "unrelated"}} {
		value := map[string]any{"apiVersion": "v1", "kind": "ConfigMap", "metadata": map[string]any{"name": entry.name, "namespace": ns, "labels": map[string]string{"fabric.sathonate.io/network": entry.network, "fabric.sathonate.io/tenant": request.TenantID}}, "data": map[string]string{"test": "non-secret"}}
		if runner.apply(ctx, value) != nil {
			t.Fatal("cannot create disposable cleanup probe")
		}
	}
	plan, err := runner.previewCleanup(ctx, request)
	if err != nil {
		t.Fatal("real local cleanup inventory rejected", err)
	}
	var probe CleanupRef
	for _, ref := range plan.Delete {
		if ref.Name == "unrelated-cleanup-probe" {
			t.Fatal("cleanup included unrelated resource")
		}
		if ref.Name == "smp-cleanup-probe" {
			probe = ref
		}
	}
	if probe.UID == "" {
		t.Fatal("cleanup preview omitted owned probe")
	}
	wrong := probe
	wrong.UID = "00000000-0000-0000-0000-000000000000"
	if runner.deleteCleanupRef(ctx, wrong) == nil {
		t.Fatal("conditional delete ignored UID precondition")
	}
	if _, err = runner.run(ctx, "-n", ns, "get", "configmap", "smp-cleanup-probe", "-o", "name"); err != nil {
		t.Fatal("failed UID check removed probe")
	}
	if err = runner.deleteCleanupRef(ctx, probe); err != nil {
		t.Fatal("exact cleanup failed", err)
	}
	if _, err = runner.run(ctx, "-n", ns, "get", "configmap", "unrelated-cleanup-probe", "-o", "name"); err != nil {
		t.Fatal("unrelated cleanup probe was removed")
	}
	t.Log("Verified exact cleanup inventory, real UID precondition denial and preservation of unrelated probe")
}
