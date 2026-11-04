// Package readiness performs read-only prelaunch checks. It never provisions resources.
package readiness

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"regexp"
	"sathonate.local/fabric-control-plane/internal/cloud"
	"strings"
	"time"
)

type Config struct {
	AccountID    string
	Profile      string
	Region       string
	Cluster      string
	Context      string
	StorageClass string
	ConsoleURL   string
	APIURL       string
	Issuer       string
	ClientID     string
	Audience     string
	TenantClaim  string
}

func Environment() Config {
	return Config{AccountID: os.Getenv("CLOUD_AWS_ACCOUNT_ID"), Profile: os.Getenv("CLOUD_AWS_PROFILE"), Region: os.Getenv("CLOUD_AWS_REGION"), Cluster: os.Getenv("CLOUD_EKS_CLUSTER"), Context: os.Getenv("CLOUD_KUBE_CONTEXT"), StorageClass: os.Getenv("CLOUD_STORAGE_CLASS"), ConsoleURL: os.Getenv("CONSOLE_PUBLIC_URL"), APIURL: os.Getenv("API_PUBLIC_URL"), Issuer: os.Getenv("OKTA_ISSUER"), ClientID: os.Getenv("OKTA_CLIENT_ID"), Audience: os.Getenv("OKTA_AUDIENCE"), TenantClaim: os.Getenv("OKTA_TENANT_CLAIM")}
}

type Check struct {
	ID       string `json:"id"`
	Group    string `json:"group"`
	Title    string `json:"title"`
	Status   string `json:"status"`
	Detail   string `json:"detail"`
	Required string `json:"required,omitempty"`
}
type Report struct {
	CheckedAt        time.Time `json:"checkedAt"`
	Ready            bool      `json:"ready"`
	ExecutionEnabled bool      `json:"executionEnabled"`
	Probed           bool      `json:"probed"`
	Checks           []Check   `json:"checks"`
}

var identifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/@-]{0,255}$`)
var account = regexp.MustCompile(`^[0-9]{12}$`)
var region = regexp.MustCompile(`^[a-z]{2}(?:-[a-z]+)+-[0-9]$`)

func origin(s string) bool {
	u, e := url.Parse(s)
	return e == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && (u.Path == "" || u.Path == "/")
}
func (c Config) Report() Report {
	r := Report{CheckedAt: time.Now().UTC(), Checks: []Check{}}
	add := func(id, group, title string, configured bool, required string) {
		status := "missing"
		detail := "Configure this prerequisite on the server. No credentials are collected by the console."
		if configured {
			status = "configured"
			detail = "Configuration present; live qualification is still required."
		}
		r.Checks = append(r.Checks, Check{id, group, title, status, detail, required})
	}
	add("aws-account", "AWS", "AWS account", account.MatchString(c.AccountID), "CLOUD_AWS_ACCOUNT_ID: company-owned 12-digit account ID")
	add("aws-profile", "AWS", "Short-lived AWS identity", identifier.MatchString(c.Profile), "CLOUD_AWS_PROFILE: locally configured SSO profile; no access keys in JSON")
	add("aws-region", "AWS", "Deployment region", region.MatchString(c.Region), "CLOUD_AWS_REGION: explicitly chosen AWS region")
	add("eks-cluster", "Kubernetes", "EKS cluster", identifier.MatchString(c.Cluster), "CLOUD_EKS_CLUSTER: create an approved EKS cluster after reviewing its cost")
	add("kube-context", "Kubernetes", "Dedicated Kubernetes context", identifier.MatchString(c.Context) && c.Context != "kind-smp-qualification", "CLOUD_KUBE_CONTEXT: separate EKS context; never reuse the local kind context")
	add("storage", "Kubernetes", "Encrypted persistent storage", identifier.MatchString(c.StorageClass), "CLOUD_STORAGE_CLASS: EBS CSI storage class with encryption and WaitForFirstConsumer")
	add("console-domain", "Domains", "Console HTTPS origin", origin(c.ConsoleURL), "CONSOLE_PUBLIC_URL: company-controlled HTTPS origin")
	add("api-domain", "Domains", "API HTTPS origin", origin(c.APIURL), "API_PUBLIC_URL: company-controlled HTTPS origin")
	u, e := url.Parse(c.Issuer)
	issuerOK := e == nil && u.Scheme == "https" && u.Host != "" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && strings.Contains(u.Path, "/oauth2/")
	add("okta-issuer", "Okta", "Custom authorization server", issuerOK, "OKTA_ISSUER: HTTPS custom authorization-server issuer")
	add("okta-client", "Okta", "SPA application with PKCE", identifier.MatchString(c.ClientID), "OKTA_CLIENT_ID: SPA client ID; authorization code + PKCE, no client secret")
	add("okta-audience", "Okta", "API audience and tenant claim", c.Audience != "" && c.Audience != c.ClientID && identifier.MatchString(c.Audience) && identifier.MatchString(c.TenantClaim), "OKTA_AUDIENCE and OKTA_TENANT_CLAIM; access-token scopes fabric.read/fabric.write and tenant/role claims")
	r.Checks = append(r.Checks, Check{"production-qualification", "Release gate", "Production deployment qualification", "blocked", "The local Bevel workflow is verified. The EKS qualification adapter is implemented. Actual EKS execution, scoped workload RBAC, secret management, backup/restore, certificate rotation and live Okta login still require production qualification before public cloud execution can be enabled.", "An approved cloud environment and a successful real Fabric transaction acceptance run; no flag can bypass missing implementation or evidence."})
	return r
}
func (c Config) Probe(ctx context.Context, commands cloud.Commander) Report {
	r := c.Report()
	r.Probed = true
	add := func(id, group, title, status, detail string) {
		r.Checks = append(r.Checks, Check{ID: id, Group: group, Title: title, Status: status, Detail: detail})
	}
	// Do not invoke AWS's default credential chain or kubectl's current context.
	if !account.MatchString(c.AccountID) || !identifier.MatchString(c.Profile) || !region.MatchString(c.Region) || !identifier.MatchString(c.Cluster) || !identifier.MatchString(c.Context) || c.Context == "kind-smp-qualification" {
		add("live-access", "Live checks", "Cloud access verification", "blocked", "AWS profile, account, region, cluster and dedicated EKS context must all be explicitly configured. No cloud command was run.")
		return r
	}
	aws := func(args ...string) ([]byte, error) {
		return commands.Run(ctx, "aws", append([]string{"--profile", c.Profile, "--region", c.Region, "--no-cli-pager"}, args...))
	}
	raw, err := aws("sts", "get-caller-identity", "--output", "json")
	var caller struct{ Account string }
	if err != nil || json.Unmarshal(raw, &caller) != nil || caller.Account != c.AccountID {
		add("live-identity", "Live checks", "AWS identity binding", "blocked", "The selected profile is unavailable or its account differs from the configured account. Sign in locally and verify the binding; diagnostics and credentials are not returned.")
		return r
	}
	add("live-identity", "Live checks", "AWS identity binding", "passed", "Selected profile resolves to the explicitly configured account.")
	raw, err = aws("eks", "describe-cluster", "--name", c.Cluster, "--output", "json")
	var result struct {
		Cluster struct {
			ARN      string
			Endpoint string
			Status   string
			Name     string
		}
	}
	if err != nil || json.Unmarshal(raw, &result) != nil || result.Cluster.Status != "ACTIVE" || result.Cluster.Name != c.Cluster || !strings.HasPrefix(result.Cluster.ARN, "arn:aws:eks:"+c.Region+":"+c.AccountID+":cluster/") {
		add("live-eks", "Live checks", "Existing EKS cluster", "blocked", "The selected active cluster could not be verified in the configured account and region.")
		return r
	}
	kub := func(args ...string) ([]byte, error) {
		return commands.Run(ctx, "kubectl", append([]string{"--context", c.Context, "--request-timeout=15s"}, args...))
	}
	raw, err = kub("config", "view", "--minify", "-o", "jsonpath={.clusters[0].cluster.server}")
	if err != nil || strings.TrimSpace(string(raw)) != result.Cluster.Endpoint {
		add("live-context", "Live checks", "Kubernetes endpoint binding", "blocked", "The explicit Kubernetes context does not match the EKS endpoint; no cluster workload checks were run.")
		return r
	}
	add("live-eks", "Live checks", "Existing EKS cluster", "passed", "The EKS cluster is active and the explicit Kubernetes context matches its endpoint.")
	if _, err = kub("get", "--raw=/readyz"); err != nil {
		add("live-kube", "Live checks", "Kubernetes API readiness", "blocked", "The bound Kubernetes API is unavailable or access is denied.")
		return r
	}
	add("live-kube", "Live checks", "Kubernetes API readiness", "passed", "The bound Kubernetes API answered its readiness endpoint.")
	if identifier.MatchString(c.StorageClass) {
		raw, err = kub("get", "storageclass", c.StorageClass, "-o", "json")
		var sc struct {
			Provisioner       string
			VolumeBindingMode string
			Parameters        map[string]string
		}
		if err == nil && json.Unmarshal(raw, &sc) == nil && sc.Provisioner == "ebs.csi.aws.com" && sc.VolumeBindingMode == "WaitForFirstConsumer" && sc.Parameters["encrypted"] == "true" {
			add("live-storage", "Live checks", "EBS CSI storage configuration", "passed", "Configured EBS storage class uses encryption and delayed volume binding. A real PVC attachment test is still required.")
		} else {
			add("live-storage", "Live checks", "EBS CSI storage configuration", "blocked", "The configured encrypted EBS CSI storage class with delayed binding was not found.")
		}
	}
	add("live-fabric", "Live checks", "Fabric and enterprise qualification", "blocked", "Read-only access checks do not prove deployment readiness. Operator installation, workload permissions, Okta login, recovery and real chaincode acceptance remain required.")
	return r
}
