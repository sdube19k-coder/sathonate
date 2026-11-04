package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sathonate.local/fabric-control-plane/internal/acceptance"
	"sathonate.local/fabric-control-plane/internal/cloud"
	"sathonate.local/fabric-control-plane/internal/domain"
	"sathonate.local/fabric-control-plane/internal/maintenance"
	"time"
)

func main() {
	action := flag.String("action", "plan", "plan, preflight, deploy, observe, verify, storage, isolation, disrupt, adopt, cancel, retry, cleanup-preview, cleanup")
	registryFile := flag.String("registry", "", "administrator target registry")
	networkFile := flag.String("network", "", "exact saved specification")
	tenant := flag.String("tenant", "", "authenticated qualification tenant")
	record := flag.String("network-id", "", "saved network record")
	targetID := flag.String("target", "", "approved target ID")
	api := flag.String("api", "http://127.0.0.1:8082", "loopback EKS qualification API")
	operation := flag.String("operation", "", "bound operation ID")
	key := flag.String("idempotency-key", "", "non-secret stable request identifier")
	node := flag.String("node", "", "one approved node for disruption")
	expected := flag.String("expected-job-uid", "", "previously recorded worker Job UID")
	cleanup := flag.String("cleanup-digest", "", "reviewed cleanup preview digest")
	approvalFile := flag.String("action-approvals", "", "private per-action approval JSON array")
	budget := flag.String("budget-authorization", "", "explicit workload/storage budget change reference")
	read := flag.Bool("allow-cloud-read", false, "explicit existing-cluster read authorization")
	execute := flag.Bool("execute", false, "explicit action execution; also requires current exact approval")
	output := flag.String("output", "", "new redacted evidence file")
	flag.Parse()
	fail := func() { fmt.Fprintln(os.Stderr, acceptance.FailureMessage(*action)); os.Exit(2) }
	registry, err := cloud.LoadRegistry(*registryFile)
	if err != nil {
		fail()
	}
	target, ok := registry.Find(*tenant, *targetID)
	if !ok {
		fail()
	}
	file, err := os.Open(*networkFile)
	if err != nil {
		fail()
	}
	network, err := domain.Decode(file)
	file.Close()
	if err != nil {
		fail()
	}
	approvals := []acceptance.Approval{}
	if *approvalFile != "" {
		f, e := os.Open(*approvalFile)
		if e != nil {
			fail()
		}
		d := json.NewDecoder(io.LimitReader(f, 65537))
		d.DisallowUnknownFields()
		var extra any
		if d.Decode(&approvals) != nil || d.Decode(&extra) != io.EOF {
			f.Close()
			fail()
		}
		f.Close()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	evidence, runErr := acceptance.Run(ctx, acceptance.Config{Registry: registry, Target: target, Network: network, TenantID: *tenant, NetworkID: *record, Action: *action, API: *api, OperationID: *operation, IdempotencyKey: *key, NodeID: *node, ExpectedJobUID: *expected, CleanupDigest: *cleanup, AllowCloudRead: *read, Execute: *execute, BudgetRef: *budget, Approvals: approvals})
	raw, _ := json.MarshalIndent(evidence, "", "  ")
	if *output != "" {
		if maintenance.CreatePrivate(*output, raw) != nil {
			fail()
		}
	}
	fmt.Println(string(raw))
	if runErr != nil {
		fail()
	}
}
