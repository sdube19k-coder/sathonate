// Package planner creates deterministic review artifacts. It never executes a deployment.
package planner

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"sathonate.local/fabric-control-plane/internal/domain"
)

const Version = "create-network/alpha2-ordering"

type Step struct {
	ID           string   `json:"id"`
	Action       string   `json:"action"`
	DependsOn    []string `json:"dependsOn"`
	Verification string   `json:"verification"`
}

type Plan struct {
	PlannerVersion           string         `json:"plannerVersion"`
	ID                       string         `json:"id"`
	Operation                string         `json:"operation"`
	NetworkID                string         `json:"networkId"`
	Engine                   string         `json:"engine"`
	Consensus                string         `json:"consensus"`
	TargetRef                string         `json:"targetRef"`
	ExpectedObservedRevision int            `json:"expectedObservedRevision"`
	Executable               bool           `json:"executable"`
	Steps                    []Step         `json:"steps"`
	Warnings                 []domain.Issue `json:"warnings"`
}

func Create(s domain.NetworkSpec) (Plan, error) {
	s = domain.WithDefaults(s)
	issues := domain.Validate(s)
	if domain.HasErrors(issues) {
		return Plan{}, fmt.Errorf("specification has validation errors")
	}
	p := Plan{PlannerVersion: Version, Operation: "create-network", NetworkID: s.Metadata.ID, Engine: s.Spec.Deployment.Engine, TargetRef: s.Spec.Deployment.TargetRef, Executable: false, Steps: []Step{}, Warnings: issues}
	p.Consensus = s.Spec.Ordering.Consensus
	appendStep := func(id, action, verification string) {
		deps := []string{}
		if len(p.Steps) > 0 {
			deps = append(deps, p.Steps[len(p.Steps)-1].ID)
		}
		p.Steps = append(p.Steps, Step{id, action, deps, verification})
	}
	appendStep("observe", "Resolve authorized target inventory and observe existing resources", "Target belongs to tenant; network does not already exist; observed revision is 0")
	appendStep("preflight", "Check compatibility, placement, capacity, DNS, time sync and identity providers", "Certified digest-pinned profile and reachable targets; declared failure domains verified")
	appendStep("artifacts", "Compile immutable engine artifacts and record their digests", "No unresolved references; no credentials in public artifacts")
	appendStep("ordering", fmt.Sprintf("Validate %s ordering configuration and per-channel consenters", p.Consensus), "Node ownership, quorum, failure domains and channel capability match the selected consensus; target evidence still required")
	if p.Engine == "bevel" {
		appendStep("infrastructure", "Bevel adapter: ensure AWS/Azure prerequisites through referenced playbooks", "Cluster, storage, routing and scoped secret access are usable")
		appendStep("bevel-network", "Bevel adapter: run isolated pinned network pipeline", "Verify each CA, peer, orderer and channel; Git push or callback alone is insufficient")
	} else {
		appendStep("identities", "Native engine: enroll organization and node signing/TLS identities", "Certificate chains, SANs, ownership and secret permissions verified")
		appendStep("nodes", "Native engine: deploy official Fabric peer/orderer containers on Linux hosts", "Readiness and mTLS administration endpoints verified")
		appendStep("channels", "Native engine: generate channel blocks, join orderers using participation API, then peers", "Orderer participation active and peer membership confirmed against expected genesis")
	}
	appendStep("verify", "Reconcile observed Fabric state with desired topology", "Membership, MSPs, channel configuration and ordering health match desired topology")
	appendStep("complete", "Persist observations, artifact references and terminal operation result", "All required steps verified; execution journal durable")
	// Digest is local plan identity only, not authorization or a substitute for the future signed execution bundle.
	material, err := json.Marshal(struct {
		Spec domain.NetworkSpec
		Plan Plan
	}{s, p})
	if err != nil {
		return Plan{}, err
	}
	digest := sha256.Sum256(material)
	p.ID = hex.EncodeToString(digest[:])
	return p, nil
}
