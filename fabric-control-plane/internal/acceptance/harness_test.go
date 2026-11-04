package acceptance

import (
	"context"
	"encoding/json"
	"os"
	"sathonate.local/fabric-control-plane/internal/cloud"
	"sathonate.local/fabric-control-plane/internal/deployment"
	"sathonate.local/fabric-control-plane/internal/domain"
	"sathonate.local/fabric-control-plane/internal/maintenance"
	"strings"
	"testing"
	"time"
)

type denyCommands struct{ calls int }

func (d *denyCommands) Run(context.Context, string, []string) ([]byte, error) {
	d.calls++
	return nil, cloudError{}
}

type cloudError struct{}

func (cloudError) Error() string { return "secret-canary" }
func fixture(t *testing.T) Config {
	t.Helper()
	f, e := os.Open("../../examples/eks-qualification-network.json")
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	s, e := domain.Decode(f)
	if e != nil {
		t.Fatal(e)
	}
	registry, e := cloud.LoadRegistry("../../examples/eks-operator-targets.template.json")
	if e != nil {
		t.Fatal(e)
	}
	target := registry.Targets[0]
	target.RunnerNamespace = deployment.Namespace(target.TenantID, "record")
	raw, _ := json.Marshal(s)
	target.Operator.NetworkSHA256 = maintenance.HashBytes(raw)
	registry.Targets[0] = target
	return Config{Registry: registry, Target: target, Network: s, TenantID: target.TenantID, NetworkID: "record", Action: "plan"}
}
func TestAcceptancePlanNeverContactsCloud(t *testing.T) {
	c := fixture(t)
	commands := &denyCommands{}
	c.Commander = commands
	e, err := Run(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(e)
	if commands.calls != 0 || e.EKSQualified || e.ProductionExecutionEnabled || strings.Contains(string(raw), "secret-canary") {
		t.Fatal("plan executed or leaked")
	}
}
func TestAcceptanceMutationsRequireExactApproval(t *testing.T) {
	for _, action := range []string{"deploy", "verify", "retry", "cancel", "disrupt", "cleanup"} {
		c := fixture(t)
		c.Action = action
		c.AllowCloudRead = true
		commands := &denyCommands{}
		c.Commander = commands
		if _, err := Run(context.Background(), c); err == nil || commands.calls != 0 {
			t.Fatal("ungated mutation", action)
		}
		c.Execute = true
		c.BudgetRef = "approved-budget"
		now := time.Now()
		c.Approvals = []Approval{{action, cloud.TargetDigest(c.Target), c.Target.Operator.NetworkSHA256, "approved-change", now.Add(time.Minute)}}
		if _, err := Validate(c, now); err != nil {
			t.Fatal("valid explicit approval rejected", err)
		}
		c.Approvals[0].NetworkSHA256 = strings.Repeat("f", 64)
		if _, err := Validate(c, now); err == nil {
			t.Fatal("wrong network approval accepted")
		}
	}
}
func TestAcceptanceReadOptInAndLoopbackBoundary(t *testing.T) {
	c := fixture(t)
	c.Action = "observe"
	if _, err := Validate(c, time.Now()); err == nil {
		t.Fatal("implicit cloud read")
	}
	for _, base := range []string{"https://customer.example", "http://user:password@127.0.0.1:8082", "http://127.0.0.1:8082/path"} {
		if _, _, err := client(base); err == nil {
			t.Fatal("unsafe API origin")
		}
	}
	if _, _, err := client("http://127.0.0.1:8082"); err != nil {
		t.Fatal(err)
	}
}
