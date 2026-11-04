package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
)

// All AWS/cluster responses here are simulated. This is not EKS evidence.
type readinessCommands struct {
	mu      sync.Mutex
	base    *operatorCommands
	failure string
	calls   []string
}

func (c *readinessCommands) Run(ctx context.Context, binary string, args []string) ([]byte, error) {
	line := binary + " " + strings.Join(args, " ")
	c.mu.Lock()
	c.calls = append(c.calls, line)
	c.mu.Unlock()
	if strings.Contains(line, "auth can-i") {
		return c.base.Run(ctx, binary, args)
	}
	encode := func(v any) ([]byte, error) { return json.Marshal(v) }
	switch {
	case strings.Contains(line, "get namespace"):
		labels := map[string]string{"fabric.sathonate.io/tenant": c.base.target.TenantID, "fabric.sathonate.io/target": c.base.target.ID, "fabric.sathonate.io/network": "record", "pod-security.kubernetes.io/enforce": "restricted", "pod-security.kubernetes.io/enforce-version": "v1.34"}
		if c.failure == "labels" {
			labels["fabric.sathonate.io/network"] = "foreign"
		}
		if c.failure == "admission" {
			delete(labels, "pod-security.kubernetes.io/enforce")
		}
		return encode(map[string]any{"metadata": map[string]any{"labels": labels}})
	case strings.Contains(line, "get pods,deployments"):
		if c.failure == "malformed" {
			return []byte(`{}`), nil
		}
		if c.failure == "inventory" {
			return []byte(`{"items":[{}]}`), nil
		}
		return []byte(`{"items":[]}`), nil
	case strings.Contains(line, "get resourcequotas"):
		if c.failure == "quotas" {
			return []byte(`{"items":[]}`), nil
		}
		return []byte(`{"items":[{"spec":{"hard":{"requests.cpu":"10","requests.memory":"10Gi","limits.cpu":"20","limits.memory":"20Gi","pods":"20","persistentvolumeclaims":"10","requests.storage":"100Gi"}}}]}`), nil
	case strings.Contains(line, "get networkpolicies"):
		if c.failure == "policy" {
			return []byte(`{"items":[]}`), nil
		}
		if c.failure == "secret" {
			return nil, errors.New("token=super-secret postgres://user:password@host/db -----BEGIN PRIVATE KEY-----")
		}
		return []byte(`{"items":[{"spec":{"podSelector":{},"policyTypes":["Ingress","Egress"]}}]}`), nil
	case strings.Contains(line, "get nodes"):
		raw, err := c.base.Run(ctx, binary, args)
		var v map[string]any
		_ = json.Unmarshal(raw, &v)
		for _, item := range v["items"].([]any) {
			n := item.(map[string]any)
			labels := n["metadata"].(map[string]any)["labels"].(map[string]any)
			labels["kubernetes.io/os"] = "linux"
			labels["kubernetes.io/arch"] = "amd64"
			if c.failure == "taint" {
				n["spec"] = map[string]any{"taints": []any{map[string]any{"effect": "NoSchedule"}}}
			}
		}
		if err != nil {
			return nil, err
		}
		return encode(v)
	}
	return c.base.Run(ctx, binary, args)
}
func TestReadinessSimulatedPrerequisites(t *testing.T) {
	for _, tc := range []struct{ failure, id, status string }{{"", "placement", "PASS"}, {"labels", "namespace-labels", "BLOCKED"}, {"quotas", "quotas", "BLOCKED"}, {"policy", "default-deny", "BLOCKED"}, {"admission", "pod-admission", "BLOCKED"}, {"inventory", "fresh-namespace", "BLOCKED"}, {"malformed", "fresh-namespace", "BLOCKED"}, {"taint", "placement", "BLOCKED"}, {"secret", "default-deny", "BLOCKED"}} {
		t.Run(tc.failure, func(t *testing.T) {
			s, target := operatorFixture(t)
			target.Operator.Runtime.Images["token=super-secret"] = "super-secret"
			c := &readinessCommands{base: &operatorCommands{target: target}, failure: tc.failure}
			e := VerifyReadiness(context.Background(), s, target, Registry{Targets: []Target{target}}, target.RunnerNamespace, "record", strings.Repeat("b", 40), true, c)
			found := false
			for _, check := range e.Checks {
				if check.ID == tc.id && check.Status == tc.status {
					found = true
				}
			}
			if !found {
				t.Fatalf("missing expected check: %+v", e.Checks)
			}
			if e.AWSReadinessVerified || e.EKSQualified || e.ProductionExecutionEnabled || e.Status == "PASS" {
				t.Fatal("simulated/static evidence claimed readiness")
			}
			raw, _ := json.Marshal(e)
			for _, secret := range []string{"super-secret", "postgres://", "PRIVATE KEY", "approved-config", "smp-role"} {
				if strings.Contains(string(raw), secret) {
					t.Fatal("sensitive output", secret)
				}
			}
			if len(e.ImageDigests) != 6 {
				t.Fatal("image digest inventory incomplete")
			}
			for _, call := range c.calls {
				for _, verb := range []string{" apply ", " delete ", " patch ", " install ", " get secret "} {
					if strings.Contains(call, verb) && !strings.Contains(call, "auth can-i") {
						t.Fatal("unsafe command", call)
					}
				}
			}
		})
	}
}
func TestReadinessRejectsBeforeCloudCalls(t *testing.T) {
	s, target := operatorFixture(t)
	c := &readinessCommands{base: &operatorCommands{target: target}}
	e := VerifyReadiness(context.Background(), s, target, Registry{Targets: []Target{target, target}}, target.RunnerNamespace, "record", "secret-token", false, c)
	if e.Status != "BLOCKED" || len(c.calls) != 0 || e.SourceRevision != "" {
		t.Fatal("duplicate binding or source metadata accepted")
	}
	target.RunnerImage = "mutable:latest"
	e = VerifyReadiness(context.Background(), s, target, Registry{Targets: []Target{target}}, target.RunnerNamespace, "record", "", false, c)
	if e.Status != "BLOCKED" || len(c.calls) != 0 {
		t.Fatal("mutable image accepted")
	}
}

func TestReadinessIdentityFailureIsRedacted(t *testing.T) {
	s, target := operatorFixture(t)
	c := &readinessCommands{base: &operatorCommands{target: target, wrong: "account"}}
	e := VerifyReadiness(context.Background(), s, target, Registry{Targets: []Target{target}}, target.RunnerNamespace, "record", "", false, c)
	if e.Status != "BLOCKED" {
		t.Fatal("identity failure accepted")
	}
	for _, call := range c.calls {
		if strings.HasPrefix(call, "kubectl ") {
			t.Fatal("foreign identity reached cluster")
		}
	}
	for _, check := range e.Checks {
		if check.ID == "namespace-labels" && check.Status != "NOT VERIFIED" {
			t.Fatal("unperformed check misreported")
		}
	}
}

func TestReadinessEvidenceExcludesEnrollmentCredentials(t *testing.T) {
	spec, target := operatorFixture(t)
	canary := "enrollment-canary-do-not-export"
	target.Name = canary
	target.Operator.Kubeconfig = canary
	c := &readinessCommands{base: &operatorCommands{target: target}}
	e := VerifyReadiness(context.Background(), spec, target, Registry{Targets: []Target{target}}, target.RunnerNamespace, "record", "", false, c)
	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), canary) {
		t.Fatal("evidence exported enrollment-bearing administrator input")
	}
}
