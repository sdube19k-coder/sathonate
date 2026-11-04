// fabric-eks inspects administrator configuration or performs read-only
// preflight. It never provisions AWS resources or submits Kubernetes workloads.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"sathonate.local/fabric-control-plane/internal/cloud"
	"sathonate.local/fabric-control-plane/internal/deployment"
	"sathonate.local/fabric-control-plane/internal/domain"
)

func main() {
	action := flag.String("action", "inspect", "inspect, preflight or readiness (all read-only)")
	registryFile := flag.String("registry", "", "trusted target registry JSON")
	networkFile := flag.String("network", "", "validated saved network JSON")
	tenant := flag.String("tenant", "", "authenticated tenant ID")
	record := flag.String("network-id", "", "saved draft record ID (not display name)")
	targetID := flag.String("target", "", "registered target ID")
	evidenceFile := flag.String("evidence", "", "new evidence JSON file for readiness; refuses overwrite")
	databaseCheck := flag.Bool("check-postgres", false, "explicit read-only verified-TLS probe using DATABASE_URL from the local secret facility")
	flag.Parse()
	fail := func(message string) { fmt.Fprintln(os.Stderr, message); os.Exit(1) }
	if *tenant == "" || *record == "" || (*action != "inspect" && *action != "preflight" && *action != "readiness") {
		fail("explicit tenant/network-id and inspect, preflight or readiness action required")
	}
	registry, e := cloud.LoadRegistry(*registryFile)
	if e != nil {
		fail("invalid registry")
	}
	target, ok := registry.Find(*tenant, *targetID)
	if !ok {
		fail("target is not registered for this tenant")
	}
	f, e := os.Open(*networkFile)
	if e != nil {
		fail("network file unavailable")
	}
	spec, e := domain.Decode(f)
	f.Close()
	if e != nil || domain.HasErrors(domain.Validate(spec)) {
		fail("invalid network")
	}
	if *action == "readiness" {
		if *evidenceFile == "" {
			fail("readiness requires a new -evidence file")
		}
		// Fixed read-only git commands; customer input is never executable.
		revision, _ := exec.Command("git", "rev-parse", "HEAD").Output()
		status, statusErr := exec.Command("git", "status", "--porcelain", "--untracked-files=all").Output()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		evidence := cloud.VerifyReadiness(ctx, spec, target, registry, deployment.Namespace(*tenant, *record), *record, strings.TrimSpace(string(revision)), statusErr == nil && len(status) == 0, cloud.OSCommander{})
		if *databaseCheck {
			evidence = cloud.WithDatabaseEvidence(evidence, cloud.PostgreSQLReadiness(ctx, os.Getenv("DATABASE_URL")))
		}
		file, err := os.OpenFile(*evidenceFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			fail("cannot create new evidence file")
		}
		err = json.NewEncoder(file).Encode(evidence)
		closeErr := file.Close()
		if err != nil || closeErr != nil {
			fail("cannot write evidence")
		}
		if err = json.NewEncoder(os.Stdout).Encode(evidence); err != nil {
			fail("cannot write report")
		}
		// A read-only scan never exits successfully as EKS qualification.
		if evidence.Status == "BLOCKED" {
			os.Exit(2)
		}
		os.Exit(3) // incomplete runtime verification, not deployment readiness
	}
	raw, _ := json.Marshal(spec)
	hash := sha256.Sum256(raw)
	report := map[string]any{"namespace": deployment.Namespace(*tenant, *record), "networkSHA256": hex.EncodeToString(hash[:]), "targetSHA256": cloud.TargetDigest(target), "productionExecutionEnabled": false, "awsVerified": false}
	if *action == "preflight" {
		if _, e = cloud.CompileOperator(spec, target, deployment.Namespace(*tenant, *record)); e != nil {
			fail(e.Error())
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		if e = cloud.OperatorPreflight(ctx, target, cloud.OSCommander{}); e != nil {
			fail(e.Error())
		}
		report["awsVerified"] = true
		report["notice"] = "Read-only checks passed; this does not qualify production execution or authorize provisioning."
	}
	if e = json.NewEncoder(os.Stdout).Encode(report); e != nil {
		fail("cannot write report")
	}
}
