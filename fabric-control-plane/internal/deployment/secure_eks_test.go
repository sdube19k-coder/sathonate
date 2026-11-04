package deployment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sathonate.local/fabric-control-plane/internal/domain"
	"sathonate.local/fabric-control-plane/internal/kubernetesfabric"
	"sathonate.local/fabric-control-plane/internal/localfabric"
	"sathonate.local/fabric-control-plane/internal/operations"
)

func secureEKSFixture(t *testing.T) (SecureEKSConfig, Request) {
	t.Helper()
	f := exportFixture(t)
	role := "arn:aws:iam::429488690613:role/aws-reserved/sso.amazonaws.com/ap-south-1/AWSReservedSSO_Sprint3-permissions_0123456789abcdef"
	q := Request{TenantID: f.TenantID, NetworkID: f.NetworkID, OperationID: f.OperationID, Network: f.Network}
	q.Network.Spec.Deployment = domain.Deployment{Engine: "bevel", Runtime: "kubernetes", Provider: "aws", TargetRef: domain.SecureEKSProfile, Cloud: &domain.CloudIntent{AccountID: "429488690613", RoleARN: role, Region: "ap-south-1", ClusterName: secureEKSRun, ClusterMode: "existing", VPCMode: "existing", MinNodes: 3, DesiredNodes: 3, MaxNodes: 3}}
	for i, n := range q.Network.Spec.Nodes {
		zone := f.Runtime.NodeSelectors[n.ID]["topology.kubernetes.io/zone"]
		q.Network.Spec.Nodes[i].PlacementRef = zone
		q.Network.Spec.Nodes[i].FailureDomain = zone
	}
	raw, _ := json.Marshal(q.Network)
	sum := sha256.Sum256(raw)
	c := SecureEKSConfig{Profile: "sathonate-sprint3", RoleARN: role, TenantID: q.TenantID, NetworkID: q.NetworkID, NetworkSHA256: hex.EncodeToString(sum[:]), WorkerImage: secureEKSRegistry + ":worker@sha256:" + strings.Repeat("d", 64), ControllerImage: secureEKSRegistry + ":controller@" + strings.Split(f.ControllerImage, "@")[1], Runtime: f.Runtime, AuthorizationRef: "approved-sprint3-qualification", ExpiresAt: time.Now().Add(time.Hour)}
	c.Runtime.StorageClass = "smp-gp3-encrypted"
	c.Runtime.SmokeImage = secureEKSRegistry + ":smoke@sha256:" + strings.Repeat("a", 64)
	for kind := range c.Runtime.Images {
		c.Runtime.Images[kind] = secureEKSRegistry + ":fabric@sha256:" + strings.Repeat("b", 64)
	}
	c.Kubeconfig = filepath.Join(t.TempDir(), "eks.json")
	kube := map[string]any{"apiVersion": "v1", "kind": "Config", "current-context": secureEKSRun, "clusters": []any{map[string]any{"name": secureEKSRun, "cluster": map[string]any{"server": "https://fixture.ap-south-1.eks.amazonaws.com", "certificate-authority-data": "cHVibGljLWZpeHR1cmU="}}}, "contexts": []any{map[string]any{"name": secureEKSRun, "context": map[string]any{"cluster": secureEKSRun, "user": secureEKSRun}}}, "users": []any{map[string]any{"name": secureEKSRun, "user": map[string]any{"exec": map[string]any{"apiVersion": "client.authentication.k8s.io/v1beta1", "command": "aws", "args": []any{"--profile", c.Profile, "--region", "ap-south-1", "eks", "get-token", "--cluster-name", secureEKSRun}, "interactiveMode": "Never", "provideClusterInfo": false}}}}}
	raw, _ = json.Marshal(kube)
	if os.WriteFile(c.Kubeconfig, raw, 0600) != nil {
		t.Fatal("fixture config")
	}
	return c, q
}

func TestSecureEKSPostgresDoesNotAdoptForeignScope(t *testing.T) {
	connection := os.Getenv("SATHONATE_TEST_DATABASE_URL")
	if connection == "" {
		t.Skip("requires dedicated PostgreSQL test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, e := pgxpool.New(ctx, connection)
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	if _, e = pool.Exec(ctx, operations.SQLSchema); e != nil {
		t.Fatal(e)
	}
	c, q := secureEKSFixture(t)
	runner, e := NewSecureEKSRunner(c)
	if e != nil {
		t.Fatal(e)
	}
	fake := &secureEKSCommands{c: c, q: q}
	runner.secure.binding.remote.commands = fake
	store, e := operations.OpenRecoverableDatabase(ctx, pool)
	if e != nil {
		t.Fatal(e)
	}
	foreign := "lp-" + strings.Repeat("f", 48)
	key := strings.ReplaceAll(time.Now().Format("150405.000000000"), ".", "-")
	defer pool.Exec(context.Background(), "DELETE FROM sathonate_deployments WHERE tenant_id=$1", foreign)
	defer pool.Exec(context.Background(), "DELETE FROM sathonate_operations WHERE tenant_id=$1", foreign)
	op, _, e := store.BeginPrepared(foreign, "fixture", key, "secure-eks-deploy", key, "fixture-digest", "2.5.15", "", Stages, func(op operations.Operation) ([]byte, error) {
		return json.Marshal(Result{Mode: "secure-eks", Network: q.Network, OperationID: op.ID, TargetSHA256: runner.BindingDigest(q), Phase: "deploying"})
	})
	if e != nil {
		t.Fatal(e)
	}
	manager, e := OpenDatabase(ctx, pool, runner)
	if e != nil {
		t.Fatal(e)
	}
	defer manager.Close()
	observed, e := manager.Operations().Get(foreign, op.ID)
	if e != nil || observed.Status != "running" || len(observed.Events) != len(op.Events) || len(fake.calls) != 0 {
		t.Fatal("foreign durable operation was replayed/mutated by new binding", e)
	}
}

type secureEKSCommands struct {
	c     SecureEKSConfig
	q     Request
	fail  string
	calls []string
}

func (f *secureEKSCommands) Run(_ context.Context, binary string, args []string) ([]byte, error) {
	line := binary + " " + strings.Join(args, " ")
	f.calls = append(f.calls, line)
	encode := func(v any) ([]byte, error) { return json.Marshal(v) }
	if strings.Contains(line, "get-caller-identity") {
		role := "AWSReservedSSO_Sprint3-permissions_0123456789abcdef"
		arn := "arn:aws:sts::429488690613:assumed-role/" + role + "/fixture"
		if f.fail == "root" {
			arn = "arn:aws:iam::429488690613:root"
		}
		return encode(map[string]string{"Account": "429488690613", "Arn": arn})
	}
	if strings.Contains(line, "describe-cluster") {
		endpoint := "https://fixture.ap-south-1.eks.amazonaws.com"
		if f.fail == "endpoint" {
			endpoint = "https://foreign.ap-south-1.eks.amazonaws.com"
		}
		return encode(map[string]any{"cluster": map[string]any{"name": secureEKSRun, "arn": "arn:aws:eks:ap-south-1:429488690613:cluster/" + secureEKSRun, "status": "ACTIVE", "endpoint": endpoint, "certificateAuthority": map[string]string{"data": "cHVibGljLWZpeHR1cmU="}}})
	}
	if strings.Contains(line, "config view") {
		return os.ReadFile(f.c.Kubeconfig)
	}
	if strings.Contains(line, "version -o json") {
		return []byte(`{"serverVersion":{"major":"1","minor":"34"}}`), nil
	}
	if strings.Contains(line, "get nodes") {
		items := []any{}
		for _, zone := range []string{"ap-south-1a", "ap-south-1b", "ap-south-1c"} {
			if f.fail == "missing-zone" && zone == "ap-south-1c" {
				continue
			}
			items = append(items, map[string]any{"metadata": map[string]any{"labels": map[string]string{"topology.kubernetes.io/zone": zone}}, "status": map[string]any{"conditions": []any{map[string]string{"type": "Ready", "status": "True"}}}})
		}
		return encode(map[string]any{"items": items})
	}
	if strings.Contains(line, "get storageclass") {
		encrypted := "true"
		if f.fail == "storage" {
			encrypted = "false"
		}
		return encode(map[string]any{"provisioner": "ebs.csi.aws.com", "volumeBindingMode": "WaitForFirstConsumer", "parameters": map[string]string{"encrypted": encrypted, "type": "gp3"}})
	}
	if strings.Contains(line, "get daemonset aws-node") {
		mode := "strict"
		if f.fail == "cni" {
			mode = "standard"
		}
		return []byte(`{"status":{"desiredNumberScheduled":3,"numberReady":3},"spec":{"template":{"spec":{"containers":[{"name":"aws-node","env":[{"name":"NETWORK_POLICY_ENFORCING_MODE","value":"` + mode + `"}]},{"name":"aws-eks-nodeagent","args":["--enable-network-policy=true"]}]}}}}`), nil
	}
	if strings.Contains(line, "get crd") || strings.Contains(line, "get csidriver") {
		return []byte("fixture"), nil
	}
	if strings.Contains(line, "get fabriccas,fabricorderernodes,fabricpeers") || strings.Contains(line, "get pods -o json") {
		return []byte(`{"items":[]}`), nil
	}
	ns, _ := SecureNamespace(f.q.TenantID, "token-bound-project", f.q.NetworkID)
	if strings.Contains(line, "get namespace") {
		if strings.Contains(line, "jsonpath") {
			return []byte("namespace-uid"), nil
		}
		labels := map[string]string{"fabric.sathonate.io/tenant": f.q.TenantID, "fabric.sathonate.io/project": "token-bound-project", "fabric.sathonate.io/network": f.q.NetworkID, "fabric.sathonate.io/profile": domain.SecureLocalProfile}
		if f.fail == "foreign-namespace" {
			labels["fabric.sathonate.io/tenant"] = "foreign"
		}
		return encode(map[string]any{"metadata": map[string]any{"name": ns, "uid": "namespace-uid", "labels": labels}})
	}
	if strings.Contains(line, "--ignore-not-found") {
		return nil, nil
	}
	return nil, errors.New("unplanned fixture command")
}

func TestSecureEKSProtectedTranslationAndManifest(t *testing.T) {
	c, q := secureEKSFixture(t)
	r, e := NewSecureEKSRunner(c)
	if e != nil {
		t.Fatal(e)
	}
	before, _ := json.Marshal(q.Network)
	exec, e := r.execution(q, true)
	if e != nil {
		t.Fatal(e)
	}
	after, _ := json.Marshal(q.Network)
	if string(before) != string(after) || q.Network.Spec.Deployment.Provider != "aws" {
		t.Fatal("desired design mutated")
	}
	if exec.Network.Spec.Deployment.TargetRef != domain.SecureLocalProfile {
		t.Fatal("wrong protected worker")
	}
	name, _ := jobName(q.OperationID)
	ns, _ := SecureNamespace(q.TenantID, "token-bound-project", q.NetworkID)
	manifest, e := r.secure.manifest(exec, name, ns)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(manifest)
	for _, bad := range []string{`"imagePullPolicy":"Never"`, "ClusterRole", "PRIVATE KEY", "sathonate/controller-candidate@"} {
		if strings.Contains(string(raw), bad) {
			t.Fatal("unsafe manifest", bad)
		}
	}
	if !strings.Contains(string(raw), "--secure-local") || !strings.Contains(string(raw), c.ControllerImage) {
		t.Fatal("protected protocol missing")
	}
	c.Runtime.NodeSelectors["orderer-1"]["topology.kubernetes.io/zone"] = "foreign"
	if r.Validate(q) != nil {
		t.Fatal("external config mutation affected binding")
	}
	if kubernetesfabric.ValidateProfile(q.Network) == nil {
		t.Fatal("legacy worker accepted secure EKS")
	}
}

func TestSecureEKSConfigAndScopeDenials(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*SecureEKSConfig, *Request)
	}{
		{"root-role", func(c *SecureEKSConfig, q *Request) { c.RoleARN = "arn:aws:iam::429488690613:root" }},
		{"root-profile", func(c *SecureEKSConfig, q *Request) { c.Profile = "sathonate-preflight" }},
		{"mutable-worker", func(c *SecureEKSConfig, q *Request) { c.WorkerImage = secureEKSRegistry + ":latest" }},
		{"original-controller", func(c *SecureEKSConfig, q *Request) {
			c.ControllerImage = secureEKSRegistry + "@sha256:" + strings.Repeat("a", 64)
		}},
		{"expired-approval", func(c *SecureEKSConfig, q *Request) { c.ExpiresAt = time.Now().Add(-time.Minute) }},
		{"unbounded-window", func(c *SecureEKSConfig, q *Request) { c.ExpiresAt = time.Now().Add(5 * time.Hour) }},
		{"foreign-tenant", func(c *SecureEKSConfig, q *Request) { q.TenantID = "lp-" + strings.Repeat("f", 48) }},
		{"foreign-record", func(c *SecureEKSConfig, q *Request) { q.NetworkID = strings.Repeat("f", 32) }},
		{"design-drift", func(c *SecureEKSConfig, q *Request) { q.Network.Metadata.Name = "changed" }},
		{"target-drift", func(c *SecureEKSConfig, q *Request) { q.TargetSHA256 = strings.Repeat("f", 64) }},
		{"inline-failure", func(c *SecureEKSConfig, q *Request) { q.FailureStage = "ca" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			c, q := secureEKSFixture(t)
			test.change(&c, &q)
			r, e := NewSecureEKSRunner(c)
			if e == nil {
				e = r.Validate(q)
			}
			if e == nil {
				t.Fatal("unsafe request allowed")
			}
		})
	}
}

func TestSecureEKSPreflightDenialsBeforeMutation(t *testing.T) {
	for _, failure := range []string{"root", "endpoint", "storage", "cni", "missing-zone", "foreign-namespace"} {
		t.Run(failure, func(t *testing.T) {
			c, q := secureEKSFixture(t)
			r, e := NewSecureEKSRunner(c)
			if e != nil {
				t.Fatal(e)
			}
			fake := &secureEKSCommands{c: c, q: q, fail: failure}
			r.secure.binding.remote.commands = fake
			if failure == "foreign-namespace" {
				e = r.secure.owned(context.Background(), q.TenantID, q.NetworkID)
			} else {
				e = r.preflight(context.Background())
			}
			if e == nil {
				t.Fatal("preflight allowed unsafe binding")
			}
			for _, line := range fake.calls {
				for _, bad := range []string{" apply ", " create ", " delete ", "assume-role"} {
					if strings.Contains(line, bad) {
						t.Fatal("mutated during preflight")
					}
				}
			}
		})
	}
}

func TestSecureEKSPositivePreflightAndCleanupApproval(t *testing.T) {
	c, q := secureEKSFixture(t)
	r, e := NewSecureEKSRunner(c)
	if e != nil {
		t.Fatal(e)
	}
	fake := &secureEKSCommands{c: c, q: q}
	r.secure.binding.remote.commands = fake
	if e = r.preflight(context.Background()); e != nil {
		t.Fatal(e)
	}
	stopped := errors.New("durable event unavailable")
	if _, e = r.Run(context.Background(), q, func(string, string) error { return stopped }); !errors.Is(e, stopped) {
		t.Fatal("event failure did not stop deployment", e)
	}
	plan, e := r.PreviewCleanup(context.Background(), q)
	if e != nil {
		t.Fatal(e)
	}
	if e = r.CleanupApproved(context.Background(), q, strings.Repeat("f", 64)); e == nil {
		t.Fatal("stale cleanup accepted")
	}
	if e = r.CleanupApproved(context.Background(), q, plan.Digest); e != nil {
		t.Fatal(e)
	}
	if e = r.secure.owned(context.Background(), "foreign", q.NetworkID); e == nil {
		t.Fatal("foreign read allowed")
	}
	if e = r.Cleanup(context.Background(), q.TenantID, q.NetworkID); e == nil {
		t.Fatal("cleanup without approval allowed")
	}
	if _, e = r.finish(context.Background(), q, Outcome{Health: &localfabric.Health{Ready: true}}); !errors.Is(e, errSecureEKS) {
		t.Fatal("cached Job evidence incorrectly promoted absent current workloads", e)
	}
}

func TestSecureEKSKubeconfigRejectsCredentialsAndFileDrift(t *testing.T) {
	for _, bad := range []string{"static-token", "exec-command", "proxy"} {
		t.Run(bad, func(t *testing.T) {
			c, _ := secureEKSFixture(t)
			raw, _ := os.ReadFile(c.Kubeconfig)
			var doc map[string]any
			_ = json.Unmarshal(raw, &doc)
			user := doc["users"].([]any)[0].(map[string]any)["user"].(map[string]any)
			switch bad {
			case "static-token":
				user["token"] = "rejected-fixture"
			case "exec-command":
				user["exec"].(map[string]any)["command"] = "powershell"
			case "proxy":
				doc["clusters"].([]any)[0].(map[string]any)["cluster"].(map[string]any)["proxy-url"] = "http://proxy.invalid"
			}
			raw, _ = json.Marshal(doc)
			_ = os.WriteFile(c.Kubeconfig, raw, 0600)
			if _, e := NewSecureEKSRunner(c); e == nil {
				t.Fatal("unsafe kubeconfig accepted")
			}
		})
	}
	c, q := secureEKSFixture(t)
	r, e := NewSecureEKSRunner(c)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := os.ReadFile(c.Kubeconfig)
	_ = os.WriteFile(c.Kubeconfig, append(raw, ' '), 0600)
	if _, e = r.Run(context.Background(), q, func(string, string) error { return nil }); e == nil {
		t.Fatal("kubeconfig drift accepted")
	}
}
