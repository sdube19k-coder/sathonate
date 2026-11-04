package deployment

// Qualification-only adapter. Infrastructure, IAM and cluster-wide dependencies
// are administrator-owned; this adapter creates only an authenticated network's
// protected issuer/controller workloads. It never provisions an AWS resource.
import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"sathonate.local/fabric-control-plane/internal/cloud"
	"sathonate.local/fabric-control-plane/internal/domain"
	"sathonate.local/fabric-control-plane/internal/kubernetesfabric"
	"sathonate.local/fabric-control-plane/internal/localfabric"
)

const secureEKSRun = "sathonate-s3-20261027"
const secureEKSRegistry = "429488690613.dkr.ecr.ap-south-1.amazonaws.com/" + secureEKSRun

var errSecureEKS = errors.New("secure EKS qualification binding rejected; no fallback permitted")

type SecureEKSConfig struct {
	Kubeconfig       string                   `json:"kubeconfig"`
	Profile          string                   `json:"profile"`
	RoleARN          string                   `json:"roleArn"`
	TenantID         string                   `json:"tenantId"`
	NetworkID        string                   `json:"networkId"`
	NetworkSHA256    string                   `json:"networkSHA256"`
	WorkerImage      string                   `json:"workerImage"`
	ControllerImage  string                   `json:"controllerImage"`
	Runtime          kubernetesfabric.Runtime `json:"runtime"`
	AuthorizationRef string                   `json:"authorizationRef"`
	ExpiresAt        time.Time                `json:"expiresAt"`
}

type secureEKSBinding struct {
	config   SecureEKSConfig
	commands cloud.Commander // injected only by package tests; never a config field
}

type SecureEKSRunner struct{ secure SecureRunner }

func LoadSecureEKSRunner(path string) (SecureEKSRunner, error) {
	raw, e := os.ReadFile(path)
	if e != nil || len(raw) > 1<<20 {
		return SecureEKSRunner{}, errSecureEKS
	}
	var c SecureEKSConfig
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&c) != nil {
		return SecureEKSRunner{}, errSecureEKS
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return SecureEKSRunner{}, errSecureEKS
	}
	return NewSecureEKSRunner(c)
}

func NewSecureEKSRunner(c SecureEKSConfig) (SecureEKSRunner, error) {
	digest := regexp.MustCompile(`^[a-f0-9]{64}$`)
	image := regexp.MustCompile(`^` + regexp.QuoteMeta(secureEKSRegistry) + `(:[A-Za-z0-9._-]+)?@sha256:[a-f0-9]{64}$`)
	role := regexp.MustCompile(`^arn:aws:iam::429488690613:role/aws-reserved/sso\.amazonaws\.com/([a-z]{2}(-[a-z]+)+-[0-9]+/)?AWSReservedSSO_Sprint3-permissions_[a-fA-F0-9]{16}$`)
	if c.Profile != "sathonate-sprint3" || !role.MatchString(c.RoleARN) ||
		!regexp.MustCompile(`^lp-[a-f0-9]{48}$`).MatchString(c.TenantID) || !regexp.MustCompile(`^[a-f0-9]{32}$`).MatchString(c.NetworkID) ||
		!digest.MatchString(c.NetworkSHA256) || !image.MatchString(c.WorkerImage) || !image.MatchString(c.ControllerImage) ||
		!strings.HasSuffix(c.ControllerImage, strings.Split(SecureLocalProfile().ControllerImage, "@")[1]) ||
		!regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`).MatchString(c.AuthorizationRef) ||
		c.ExpiresAt.IsZero() || c.ExpiresAt.After(time.Now().Add(4*time.Hour)) {
		return SecureEKSRunner{}, errSecureEKS
	}
	for _, img := range c.Runtime.Images {
		if !image.MatchString(img) {
			return SecureEKSRunner{}, errSecureEKS
		}
	}
	if !image.MatchString(c.Runtime.SmokeImage) || c.Runtime.Capacity != "10Gi" || c.Runtime.StorageClass != "smp-gp3-encrypted" {
		return SecureEKSRunner{}, errSecureEKS
	}
	raw, e := os.ReadFile(c.Kubeconfig)
	if e != nil || len(raw) > 1<<20 || !validSecureEKSKubeconfig(raw, c) {
		return SecureEKSRunner{}, errSecureEKS
	}
	// Deep copy: administrator config and runtime maps cannot drift after binding.
	saved, _ := json.Marshal(c)
	var copy SecureEKSConfig
	_ = json.Unmarshal(saved, &copy)
	sum := sha256.Sum256(raw)
	b := &secureBinding{config: SecureLocalConfig{Kubeconfig: c.Kubeconfig, ClusterName: secureEKSRun, WorkerImage: c.WorkerImage, Runtime: copy.Runtime}, kubeSHA: hex.EncodeToString(sum[:]), remote: &secureEKSBinding{config: copy}}
	return SecureEKSRunner{secure: SecureRunner{binding: b}}, nil
}

func validSecureEKSKubeconfig(raw []byte, c SecureEKSConfig) bool {
	// Only the JSON written by write-eks-kubeconfig.ps1 is accepted. No plugins,
	// embedded credentials, proxy, client certificate or insecure TLS override.
	var config struct {
		Current  string `json:"current-context"`
		Clusters []struct {
			Name    string
			Cluster map[string]any
		}
		Contexts []struct {
			Name    string
			Context map[string]any
		}
		Users []struct {
			Name string
			User map[string]any
		}
	}
	if json.Unmarshal(raw, &config) != nil || len(config.Clusters) != 1 || len(config.Contexts) != 1 || len(config.Users) != 1 || config.Current != secureEKSRun || config.Contexts[0].Name != secureEKSRun {
		return false
	}
	cluster := config.Clusters[0].Cluster
	if len(cluster) != 2 {
		return false
	}
	endpoint, ok := cluster["server"].(string)
	ca, caok := cluster["certificate-authority-data"].(string)
	if !ok || !caok || ca == "" || !regexp.MustCompile(`^https://[A-Za-z0-9.-]+\.ap-south-1\.eks\.amazonaws\.com$`).MatchString(endpoint) {
		return false
	}
	context := config.Contexts[0].Context
	if len(context) != 2 || context["cluster"] != config.Clusters[0].Name || context["user"] != config.Users[0].Name {
		return false
	}
	expected := map[string]any{"exec": map[string]any{"apiVersion": "client.authentication.k8s.io/v1beta1", "command": "aws", "args": []any{"--profile", c.Profile, "--region", "ap-south-1", "eks", "get-token", "--cluster-name", secureEKSRun}, "interactiveMode": "Never", "provideClusterInfo": false}}
	got, _ := json.Marshal(config.Users[0].User)
	want, _ := json.Marshal(expected)
	return bytes.Equal(got, want)
}

func (r SecureEKSRunner) Mode() string                   { return "secure-eks" }
func (r SecureEKSRunner) BindingDigest(q Request) string { return r.secure.BindingDigest(q) }

func (r SecureEKSRunner) execution(q Request, requireCurrent bool) (Request, error) {
	if r.secure.binding == nil || r.secure.binding.remote == nil {
		return q, errSecureEKS
	}
	c := r.secure.binding.remote.config
	if q.TenantID != c.TenantID || q.NetworkID != c.NetworkID || q.FailureStage != "" || (requireCurrent && !c.ExpiresAt.After(time.Now())) || domain.HasErrors(domain.Validate(q.Network)) {
		return q, errSecureEKS
	}
	raw, _ := json.Marshal(q.Network)
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != c.NetworkSHA256 {
		return q, errSecureEKS
	}
	d := q.Network.Spec.Deployment
	if d.TargetRef != domain.SecureEKSProfile || d.Provider != "aws" || d.Cloud == nil || d.Cloud.AccountID != "429488690613" || d.Cloud.Region != "ap-south-1" || d.Cloud.ClusterName != secureEKSRun || d.Cloud.ClusterMode != "existing" || d.Cloud.VPCMode != "existing" || d.Cloud.RoleARN != c.RoleARN || q.Network.Spec.Environment != "development" {
		return q, errSecureEKS
	}
	// Translate only execution input. The durable desired spec/hash remains AWS.
	raw, _ = json.Marshal(q.Network)
	var fixture domain.NetworkSpec
	_ = json.Unmarshal(raw, &fixture)
	fixture.Spec.Deployment = domain.Deployment{Engine: "bevel", Runtime: "kubernetes", Provider: "local", TargetRef: domain.SecureLocalProfile}
	zones := map[string]bool{}
	for i, n := range fixture.Spec.Nodes {
		selectors := c.Runtime.NodeSelectors[n.ID]
		if len(selectors) != 1 || selectors["topology.kubernetes.io/zone"] != n.FailureDomain || n.PlacementRef != n.FailureDomain || !regexp.MustCompile(`^ap-south-1[abc]$`).MatchString(n.FailureDomain) {
			return q, errSecureEKS
		}
		if n.Role == "orderer" {
			zones[n.FailureDomain] = true
		}
		fixture.Spec.Nodes[i].PlacementRef = "kind-node"
		fixture.Spec.Nodes[i].FailureDomain = "local-host"
	}
	if len(zones) != 3 || domain.HasErrors(domain.Validate(fixture)) || c.Runtime.Validate(fixture) != nil {
		return q, errSecureEKS
	}
	q.Network = fixture
	if q.TargetSHA256 != "" && q.TargetSHA256 != r.BindingDigest(q) {
		return q, errSecureEKS
	}
	return q, nil
}

func (r SecureEKSRunner) Validate(q Request) error { _, err := r.execution(q, true); return err }
func (r SecureEKSRunner) Run(ctx context.Context, q Request, emit func(string, string) error) (Outcome, error) {
	input, err := r.execution(q, true)
	if err != nil {
		return Outcome{}, err
	}
	ctx, cancel := context.WithDeadline(ctx, r.secure.binding.remote.config.ExpiresAt)
	defer cancel()
	if err = r.preflight(ctx); err != nil {
		return Outcome{}, err
	}
	outcome, err := r.secure.Run(ctx, input, emit)
	if err != nil {
		return Outcome{}, err
	}
	return r.finish(ctx, q, outcome)
}

// A completed adopted Job is historical transaction proof, not proof that its
// peers/orderers/storage are currently healthy after a restart.
func (r SecureEKSRunner) finish(ctx context.Context, q Request, outcome Outcome) (Outcome, error) {
	health, err := r.secure.Health(ctx, q.TenantID, q.NetworkID, outcome.Health)
	if err != nil {
		return Outcome{}, err
	}
	if !health.Ready {
		return Outcome{}, errSecureEKS
	}
	outcome.Health = &health
	return outcome, nil
}
func (r SecureEKSRunner) Health(ctx context.Context, tenant, record string, previous *localfabric.Health) (localfabric.Health, error) {
	return r.secure.Health(ctx, tenant, record, previous)
}
func (r SecureEKSRunner) Logs(ctx context.Context, tenant, record, node string) (string, error) {
	return r.secure.Logs(ctx, tenant, record, node)
}
func (r SecureEKSRunner) CancelDeployment(ctx context.Context, q Request) error {
	input, e := r.execution(q, false)
	if e != nil {
		return e
	}
	return r.secure.CancelDeployment(ctx, input)
}
func (r SecureEKSRunner) Cleanup(context.Context, string, string) error {
	return errors.New("secure EKS cleanup requires a current UID-bound administrator preview; infrastructure and persistent data need separate teardown")
}

func (r KubernetesRunner) validateSecureEKS(ctx context.Context) error {
	c := r.secure.remote.config
	if c.TenantID == "" {
		return errSecureEKS
	}
	t := cloud.Target{AccountID: "429488690613", Region: "ap-south-1", ClusterName: secureEKSRun, KubeContext: secureEKSRun,
		Operator: &cloud.OperatorTarget{Profile: c.Profile, RoleARN: c.RoleARN, Kubeconfig: c.Kubeconfig}}
	commands := r.secure.remote.commands
	if commands == nil {
		commands = cloud.OSCommander{}
	}
	if cloud.OperatorAccess(ctx, t, commands) != nil {
		return errSecureEKS
	}
	return nil
}

func (r SecureEKSRunner) preflight(ctx context.Context) error {
	b := r.secure.base()
	if e := b.validateSecure(ctx); e != nil {
		return e
	}
	var version struct{ ServerVersion struct{ Major, Minor string } }
	raw, e := b.run(ctx, "version", "-o", "json")
	if e != nil || json.Unmarshal(raw, &version) != nil || version.ServerVersion.Major != "1" || strings.TrimSuffix(version.ServerVersion.Minor, "+") != "34" {
		return errSecureEKS
	}
	var nodes struct {
		Items []struct {
			Metadata struct{ Labels map[string]string }
			Status   struct {
				Conditions []struct{ Type, Status string }
			}
		}
	}
	raw, e = b.run(ctx, "get", "nodes", "-o", "json")
	if e != nil || json.Unmarshal(raw, &nodes) != nil {
		return errSecureEKS
	}
	zones := map[string]bool{}
	for _, node := range nodes.Items {
		for _, condition := range node.Status.Conditions {
			if condition.Type == "Ready" && condition.Status == "True" {
				zones[node.Metadata.Labels["topology.kubernetes.io/zone"]] = true
			}
		}
	}
	for _, zone := range []string{"ap-south-1a", "ap-south-1b", "ap-south-1c"} {
		if !zones[zone] {
			return errSecureEKS
		}
	}
	var storage struct {
		Provisioner, VolumeBindingMode string
		Parameters                     map[string]string
	}
	raw, e = b.run(ctx, "get", "storageclass", "smp-gp3-encrypted", "-o", "json")
	if e != nil || json.Unmarshal(raw, &storage) != nil || storage.Provisioner != "ebs.csi.aws.com" || storage.VolumeBindingMode != "WaitForFirstConsumer" || storage.Parameters["encrypted"] != "true" || storage.Parameters["type"] != "gp3" {
		return errSecureEKS
	}
	if _, e = b.run(ctx, "get", "csidriver", "ebs.csi.aws.com", "-o", "name"); e != nil {
		return errSecureEKS
	}
	var cni struct {
		Status struct{ DesiredNumberScheduled, NumberReady int }
		Spec   struct {
			Template struct {
				Spec struct {
					Containers []struct {
						Name string
						Args []string
						Env  []struct{ Name, Value string }
					}
				}
			}
		}
	}
	raw, e = b.run(ctx, "-n", "kube-system", "get", "daemonset", "aws-node", "-o", "json")
	if e != nil || json.Unmarshal(raw, &cni) != nil || cni.Status.DesiredNumberScheduled < 3 || cni.Status.NumberReady != cni.Status.DesiredNumberScheduled {
		return errSecureEKS
	}
	strict, agent := false, false
	for _, container := range cni.Spec.Template.Spec.Containers {
		for _, env := range container.Env {
			if env.Name == "NETWORK_POLICY_ENFORCING_MODE" && env.Value == "strict" {
				strict = true
			}
		}
		if container.Name == "aws-eks-nodeagent" {
			for _, arg := range container.Args {
				if arg == "--enable-network-policy=true" {
					agent = true
				}
			}
		}
	}
	if !strict || !agent {
		return errSecureEKS
	}
	for _, plural := range []string{"fabriccas", "fabricpeers", "fabricorderernodes", "fabricchaincodes"} {
		if _, e = b.run(ctx, "get", "crd", plural+".hlf.kungfusoftware.es", "-o", "name"); e != nil {
			return errSecureEKS
		}
	}
	return nil
}
