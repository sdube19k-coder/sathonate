package readiness

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

type command struct {
	binary string
	args   []string
}
type fake struct {
	calls    []command
	account  string
	endpoint string
	storage  bool
}

func (f *fake) Run(_ context.Context, b string, a []string) ([]byte, error) {
	f.calls = append(f.calls, command{b, a})
	s := strings.Join(a, " ")
	switch {
	case strings.Contains(s, "get-caller-identity"):
		return []byte(`{"Account":"` + f.account + `"}`), nil
	case strings.Contains(s, "describe-cluster"):
		return []byte(`{"cluster":{"arn":"arn:aws:eks:ap-south-1:123456789012:cluster/test","name":"test","status":"ACTIVE","endpoint":"https://eks.example.test"}}`), nil
	case strings.Contains(s, "config view"):
		return []byte(f.endpoint), nil
	case strings.Contains(s, "readyz"):
		return []byte("ok"), nil
	case strings.Contains(s, "storageclass") && f.storage:
		return []byte(`{"provisioner":"ebs.csi.aws.com","volumeBindingMode":"WaitForFirstConsumer","parameters":{"encrypted":"true"}}`), nil
	}
	return nil, fmt.Errorf("unavailable token=must-not-appear")
}
func config() Config {
	return Config{AccountID: "123456789012", Profile: "sathonate-sso", Region: "ap-south-1", Cluster: "test", Context: "sathonate-eks", StorageClass: "gp3", ConsoleURL: "https://fabric.example.test", APIURL: "https://api.example.test", Issuer: "https://example.okta.com/oauth2/default", ClientID: "spa123", Audience: "api://fabric-smp", TenantClaim: "sathonate_tenant"}
}
func TestMissingConfigurationNeverUsesAmbientCredentialsOrContext(t *testing.T) {
	f := &fake{}
	r := (Config{}).Probe(context.Background(), f)
	if len(f.calls) != 0 || r.Ready || r.ExecutionEnabled {
		t.Fatal("missing configuration invoked cloud commands or enabled execution")
	}
}
func TestReadOnlyProbeBindsProfileAccountRegionAndEndpoint(t *testing.T) {
	f := &fake{account: "123456789012", endpoint: "https://eks.example.test", storage: true}
	r := config().Probe(context.Background(), f)
	if r.Ready || r.ExecutionEnabled {
		t.Fatal("access checks bypassed production qualification")
	}
	passed := 0
	for _, c := range r.Checks {
		if c.Status == "passed" {
			passed++
		}
		if strings.Contains(c.Detail, "must-not-appear") {
			t.Fatal("raw command diagnostics leaked")
		}
	}
	if passed != 4 {
		t.Fatal("expected identity/cluster/API/storage evidence", passed)
	}
	for _, call := range f.calls {
		args := strings.Join(call.args, " ")
		if call.binary == "aws" && !strings.HasPrefix(args, "--profile sathonate-sso --region ap-south-1") {
			t.Fatal("ambient AWS context used")
		}
		if call.binary == "kubectl" && !strings.HasPrefix(args, "--context sathonate-eks") {
			t.Fatal("ambient kube context used")
		}
		if strings.Contains(args, "create ") || strings.Contains(args, "apply ") || strings.Contains(args, "delete ") {
			t.Fatal("probe contains mutation")
		}
	}
}
func TestWrongAccountStopsBeforeClusterAccess(t *testing.T) {
	f := &fake{account: "999999999999"}
	config().Probe(context.Background(), f)
	if len(f.calls) != 1 {
		t.Fatal("wrong account reached cluster commands")
	}
}
func TestWrongContextCannotProbeAnotherCluster(t *testing.T) {
	f := &fake{account: "123456789012", endpoint: "https://other.example.test"}
	config().Probe(context.Background(), f)
	if len(f.calls) != 3 {
		t.Fatal("mismatched context reached workload commands")
	}
}
func TestUnsafeConfigDoesNotBecomeConfigured(t *testing.T) {
	c := config()
	c.Profile = "--profile evil"
	c.Context = "kind-smp-qualification"
	c.ConsoleURL = "https://user:secret@example.test"
	c.Issuer = "https://example.test/oauth2/default?token=secret"
	r := c.Report()
	for _, check := range r.Checks {
		if (check.ID == "aws-profile" || check.ID == "kube-context" || check.ID == "console-domain" || check.ID == "okta-issuer") && check.Status != "missing" {
			t.Fatal("unsafe configuration admitted", check.ID)
		}
	}
}
