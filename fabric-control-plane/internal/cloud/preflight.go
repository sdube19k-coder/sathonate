package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"sathonate.local/fabric-control-plane/internal/domain"
)

type Check struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Message string `json:"message"`
}
type Report struct {
	TargetID  string    `json:"targetId"`
	CheckedAt time.Time `json:"checkedAt"`
	Ready     bool      `json:"ready"`
	Checks    []Check   `json:"checks"`
}
type Commander interface {
	Run(context.Context, string, []string) ([]byte, error)
}
type OSCommander struct{}
type limitedBuffer struct{ bytes.Buffer }

func (b *limitedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	remaining := (2 << 20) - b.Len()
	if remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		_, _ = b.Buffer.Write(p)
	}
	return n, nil
}
func (OSCommander) Run(ctx context.Context, binary string, args []string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Env = append(os.Environ(), "AWS_PAGER=", "KUBECTL_EXTERNAL_DIFF=")
	var output limitedBuffer
	cmd.Stdout = &output
	cmd.Stderr = &limitedBuffer{}
	e := cmd.Run()
	return output.Bytes(), e
}

func StaticChecks(s domain.NetworkSpec, t Target) []Check {
	checks := []Check{}
	add := func(id string, ok bool, good, bad string) {
		status, message := "passed", good
		if !ok {
			status, message = "blocked", bad
		}
		checks = append(checks, Check{id, status, message})
	}
	s = domain.WithDefaults(s)
	if c := s.Spec.Deployment.Cloud; c != nil {
		add("cluster-mode", c.ClusterMode == "existing", "Existing cluster selected", "Cluster provisioning is not implemented; register an existing EKS cluster")
		add("intent-binding", (c.AccountID == "" || c.AccountID == t.AccountID) && (c.Region == "" || c.Region == t.Region) && (c.ClusterName == "" || c.ClusterName == t.ClusterName) && (c.Domain == "" || c.Domain == t.DNSDomain), "UI cloud settings match the registered target", "UI account, region, cluster or DNS differs from the registered target; the target cannot be overridden")
	}
	add("target-binding", s.Spec.Deployment.TargetRef == t.ID && t.TenantID != "", "Desired target and registered organization binding match", "Desired target reference does not match this registered target")
	add("cloud-engine", s.Spec.Deployment.Engine == "bevel" && s.Spec.Deployment.Provider == "aws" && s.Spec.Deployment.Runtime == "kubernetes", "AWS EKS / Bevel selected", "This integration accepts AWS EKS Bevel designs only")
	add("consensus", s.Spec.Ordering.Consensus == "raft", "Raft selected", "This cloud integration does not execute BFT or Solo")
	add("aws-target", regexp.MustCompile(`^[0-9]{12}$`).MatchString(t.AccountID) && t.Region != "" && t.ClusterName != "" && t.KubeContext != "", "AWS target identifiers registered", "Register account ID, region, cluster name and kube context")
	add("fabric-profile", s.Spec.FabricVersion == t.FabricVersion && strings.HasPrefix(t.FabricVersion, "2.5."), "Fabric version matches the target profile", "Selected Fabric version does not match the registered Bevel profile")
	add("runner-image", digestImage.MatchString(t.RunnerImage), "Runner image is pinned by digest", "Configure an isolated Linux runner image by sha256 digest")
	add("source-pin", commitID.MatchString(t.BevelCommit), "Bevel source commit is pinned", "Pin an exact 40-character Bevel commit")
	add("qualification", t.CertificationRef != "", "Qualification evidence reference registered; still subject to platform release gates", "No EKS/Bevel qualification evidence is registered")
	add("gitops", httpsURL(t.GitOpsURL) && name.MatchString(t.GitOpsSecret), "GitOps repository and credential reference registered", "Register an approved HTTPS GitOps repository and Kubernetes credential Secret name")
	add("dns", regexp.MustCompile(`^[a-z0-9][a-z0-9.-]+\.[a-z]{2,}$`).MatchString(t.DNSDomain), "DNS suffix registered", "Register an owned DNS suffix for TLS endpoints")
	add("runner-namespace", name.MatchString(t.RunnerNamespace), "Runner namespace configured", "Configure a dedicated execution namespace")
	for _, org := range s.Spec.Organizations {
		binding, ok := t.Organizations[org.ID]
		add("org-"+org.ID, ok && shortName.MatchString(org.ID) && org.MSPID == org.ID+"MSP" && regexp.MustCompile(`^[A-Z]{2}$`).MatchString(binding.Country) && httpsURL(binding.VaultURL) && binding.VaultMount != "" && name.MatchString(binding.VaultTokenSecret), "Organization identity and Vault references match the fork contract", "Organization "+org.ID+" requires a <=12-character Bevel name, MSP ID "+org.ID+"MSP, country, TLS Vault URL/mount and token Secret reference; existing MSPs are never silently renamed")
	}
	for _, node := range s.Spec.Nodes {
		placement, ok := t.Placements[node.PlacementRef]
		add("placement-"+node.ID, ok && placement.Zone == node.FailureDomain && len(placement.NodeSelector) > 0, "Node placement is bound to target inventory", "Resolve "+node.PlacementRef+" to a node selector and zone matching "+node.FailureDomain)
	}
	for _, c := range s.Spec.Channels {
		add("channel-"+c.ID, shortName.MatchString(c.ID), "Channel name matches the fork schema", "Bevel channel names must be <=12 characters")
	}
	return checks
}
func Preflight(ctx context.Context, s domain.NetworkSpec, t Target, commands Commander) Report {
	report := Report{TargetID: t.ID, CheckedAt: time.Now().UTC(), Checks: StaticChecks(s, t)}
	add := func(id, status, message string) { report.Checks = append(report.Checks, Check{id, status, message}) }
	if t.BevelCheckout != "" {
		b, e := os.ReadFile(filepath.Join(t.BevelCheckout, "platforms", "network-schema.json"))
		if e != nil {
			add("source-schema", "blocked", "Cannot inspect the registered Bevel checkout")
		} else if !bytes.Contains(b, []byte(`"`+s.Spec.FabricVersion+`"`)) {
			add("source-schema", "blocked", "The checked-out Bevel schema does not admit this Fabric patch; an audited compatibility update is required")
		} else {
			add("source-schema", "passed", "Selected Fabric version appears in the inspected source schema")
		}
		b, _ = os.ReadFile(filepath.Join(t.BevelCheckout, "platforms", "hyperledger-fabric", "configuration", "roles", "helm_component", "templates", "orderernode.tpl"))
		if bytes.Contains(b, []byte("helm.fluxcd.io/v1")) {
			add("gitops-api", "blocked", "This checkout generates legacy Flux v1 HelmRelease resources; modern EKS compatibility is not qualified")
		}
	} else {
		add("source-inspection", "blocked", "Register the pinned Bevel checkout for compatibility inspection")
	}
	if t.KubeContext == "" {
		add("kubernetes-access", "blocked", "No registered Kubernetes context")
		return report
	}
	kubectl := func(args ...string) ([]byte, error) {
		return commands.Run(ctx, "kubectl", append([]string{"--context", t.KubeContext, "--request-timeout=15s"}, args...))
	}
	if out, e := commands.Run(ctx, "aws", []string{"sts", "get-caller-identity", "--output", "json", "--region", t.Region}); e != nil {
		add("aws-identity", "blocked", "AWS identity unavailable; configure a scoped local profile or workload identity")
	} else {
		var identity struct{ Account string }
		if json.Unmarshal(out, &identity) != nil || identity.Account != t.AccountID {
			add("aws-identity", "blocked", "AWS caller account does not match this registered target")
		} else {
			add("aws-identity", "passed", "AWS caller account matches the registered target")
		}
	}
	var clusterEndpoint string
	if out, e := commands.Run(ctx, "aws", []string{"eks", "describe-cluster", "--name", t.ClusterName, "--region", t.Region, "--output", "json"}); e != nil {
		add("eks-access", "blocked", "Cannot describe the registered EKS cluster")
	} else {
		var response struct {
			Cluster struct {
				ARN      string `json:"arn"`
				Endpoint string `json:"endpoint"`
				Status   string `json:"status"`
			}
		}
		if json.Unmarshal(out, &response) != nil || response.Cluster.Status != "ACTIVE" || response.Cluster.ARN != "arn:aws:eks:"+t.Region+":"+t.AccountID+":cluster/"+t.ClusterName || !httpsURL(response.Cluster.Endpoint) {
			add("eks-access", "blocked", "EKS response does not match the registered active account/region/cluster")
		} else {
			clusterEndpoint = response.Cluster.Endpoint
			add("eks-access", "passed", "Registered EKS cluster is active and its ARN matches")
		}
	}
	endpoint, e := kubectl("config", "view", "--minify", "-o", "jsonpath={.clusters[0].cluster.server}")
	if e != nil || clusterEndpoint == "" || strings.TrimSpace(string(endpoint)) != clusterEndpoint {
		add("cluster-binding", "blocked", "The Kubernetes context is not verified against the registered EKS endpoint")
	} else {
		add("cluster-binding", "passed", "Kubernetes context endpoint matches AWS EKS")
	}
	insecure, e := kubectl("config", "view", "--minify", "-o", "jsonpath={.clusters[0].cluster.insecure-skip-tls-verify}")
	if e != nil || strings.TrimSpace(string(insecure)) == "true" {
		add("kubernetes-tls", "blocked", "Kubernetes TLS certificate verification must be enabled")
	} else {
		add("kubernetes-tls", "passed", "Kubeconfig does not disable TLS verification")
	}
	if _, e := kubectl("get", "--raw=/readyz"); e != nil {
		add("kubernetes-access", "blocked", "Kubernetes API is unreachable or authentication failed")
		return report
	}
	add("kubernetes-access", "passed", "Kubernetes API readiness endpoint is reachable")
	for _, resource := range []struct {
		id   string
		args []string
	}{{"runner-namespace", []string{"get", "namespace", t.RunnerNamespace, "-o", "name"}}, {"storage", []string{"get", "storageclass", t.StorageClass, "-o", "name"}}, {"helmrelease-api", []string{"get", "crd", "helmreleases.helm.fluxcd.io", "-o", "name"}}, {"runner-service-account", []string{"get", "serviceaccount", "sathonate-runner", "-n", t.RunnerNamespace, "-o", "name"}}} {
		if _, e := kubectl(resource.args...); e != nil {
			add(resource.id, "blocked", "Required Kubernetes resource is missing or inaccessible")
		} else {
			add(resource.id, "passed", "Required Kubernetes resource exists")
		}
	}
	// Never fetch Secret data into the control API. Verify metadata only.
	for _, secret := range secretNames(t) {
		if _, e := kubectl("get", "secret", secret, "-n", t.RunnerNamespace, "-o", "name"); e != nil {
			add("secret-"+secret, "blocked", "Required execution Secret reference is missing or inaccessible")
		} else {
			add("secret-"+secret, "passed", "Execution Secret reference exists; contents were not read")
		}
	}
	for _, org := range s.Spec.Organizations {
		out, e := kubectl("get", "namespace", org.ID+"-net", "--ignore-not-found", "-o", "name")
		if e != nil {
			add("namespace-"+org.ID, "blocked", "Could not inspect organization namespace ownership")
		} else if len(bytes.TrimSpace(out)) > 0 {
			add("namespace-"+org.ID, "blocked", "Organization namespace already exists; initial creation must not adopt or overwrite it")
		} else {
			add("namespace-"+org.ID, "passed", "Organization namespace is unused")
		}
	}
	for _, node := range s.Spec.Nodes {
		placement, ok := t.Placements[node.PlacementRef]
		if !ok {
			continue
		}
		selectors := []string{}
		for k, v := range placement.NodeSelector {
			selectors = append(selectors, k+"="+v)
		}
		sort.Strings(selectors)
		out, e := kubectl("get", "nodes", "-l", strings.Join(selectors, ","), "-o", "json")
		var nodes struct {
			Items []struct {
				Metadata struct{ Labels map[string]string }
				Spec     struct{ Unschedulable bool }
				Status   struct {
					Conditions []struct {
						Type   string
						Status string
					}
				}
			}
		}
		valid := false
		if e == nil && json.Unmarshal(out, &nodes) == nil && len(nodes.Items) > 0 {
			valid = true
			for _, n := range nodes.Items {
				ready := false
				for _, c := range n.Status.Conditions {
					if c.Type == "Ready" && c.Status == "True" {
						ready = true
					}
				}
				if !ready || n.Spec.Unschedulable || n.Metadata.Labels["topology.kubernetes.io/zone"] != placement.Zone {
					valid = false
				}
			}
		}
		if valid {
			add("inventory-"+node.ID, "passed", "Selected nodes are ready in the declared zone")
		} else {
			add("inventory-"+node.ID, "blocked", fmt.Sprintf("Placement %s is not verified ready in zone %s", node.PlacementRef, placement.Zone))
		}
	}
	// These gates are deliberately not inferred from a successful kubectl response.
	add("production-release", "blocked", "Execution remains disabled until Okta/PostgreSQL isolation, runner provenance, durable scheduling and real EKS acceptance tests are qualified")
	return report
}
func secretNames(t Target) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range []string{t.GitOpsSecret, "eks-access"} {
		if value != "" && !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	for _, org := range t.Organizations {
		if org.VaultTokenSecret != "" && !seen[org.VaultTokenSecret] {
			seen[org.VaultTokenSecret] = true
			out = append(out, org.VaultTokenSecret)
		}
	}
	sort.Strings(out)
	return out
}
