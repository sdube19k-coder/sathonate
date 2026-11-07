package httpapi

import (
	"encoding/json"
	"fmt"
	"os"
	"sathonate.local/fabric-control-plane/internal/domain"
	"testing"
)

// This generates customer test inputs only. Production reconciles the supplied
// placements unchanged and validates them against server-owned inventory.
func qualificationWorkerPlacements(before *domain.NetworkSpec, next domain.NetworkSpec, workers []string) domain.NetworkSpec {
	raw, _ := json.Marshal(next)
	var isolated domain.NetworkSpec
	json.Unmarshal(raw, &isolated)
	next = isolated
	oldNodes, oldAuthorities := map[string]domain.Node{}, map[string]domain.Authority{}
	counts := map[string]int{}
	if before != nil {
		for _, n := range before.Spec.Nodes {
			oldNodes[n.ID] = n
			counts[n.Role]++
		}
		for _, a := range before.Spec.Configuration.Authorities {
			oldAuthorities[a.ID] = a
		}
	}
	if before == nil {
		for ref := range next.Spec.Configuration.Placements {
			next.Spec.Configuration.Placements[ref] = domain.Placement{NodeSelector: map[string]string{"kubernetes.io/hostname": workers[0]}}
		}
	}
	for i, worker := range workers {
		next.Spec.Configuration.Placements[fmt.Sprintf("qualification-worker-%d", i)] = domain.Placement{NodeSelector: map[string]string{"kubernetes.io/hostname": worker}}
	}
	for i := range next.Spec.Nodes {
		n := &next.Spec.Nodes[i]
		if previous, ok := oldNodes[n.ID]; ok {
			n.PlacementRef, n.FailureDomain = previous.PlacementRef, previous.FailureDomain
			continue
		}
		slot := counts[n.Role] % len(workers)
		counts[n.Role]++
		n.PlacementRef = fmt.Sprintf("qualification-worker-%d", slot)
	}
	count := len(oldAuthorities)
	for i := range next.Spec.Configuration.Authorities {
		a := &next.Spec.Configuration.Authorities[i]
		if previous, ok := oldAuthorities[a.ID]; ok {
			a.PlacementRef = previous.PlacementRef
			continue
		}
		a.PlacementRef = fmt.Sprintf("qualification-worker-%d", count%len(workers))
		count++
	}
	return next
}
func TestMultiWorkerFixturePreservesExistingPlacementAcrossExpansion(t *testing.T) {
	raw, e := os.ReadFile("../../examples/dynamic/a.json")
	if e != nil {
		t.Fatal(e)
	}
	var initial domain.NetworkSpec
	json.Unmarshal(raw, &initial)
	workers := []string{"owned-worker-a", "owned-worker-b", "owned-worker-c"}
	original, _ := json.Marshal(initial)
	input := initial
	initial = qualificationWorkerPlacements(nil, initial, workers)
	untouched, _ := json.Marshal(input)
	if string(original) != string(untouched) {
		t.Fatal("placement fixture mutated caller input")
	}
	seen := map[string]bool{}
	for _, n := range initial.Spec.Nodes {
		if n.Role == "orderer" {
			seen[initial.Spec.Configuration.Placements[n.PlacementRef].NodeSelector["kubernetes.io/hostname"]] = true
			if n.FailureDomain != "local-host" {
				t.Fatal("kind workers incorrectly advertised as independent physical hosts")
			}
		}
	}
	if len(seen) != 3 {
		t.Fatal("initial Raft quorum not distributed")
	}
	previous := initial
	for _, next := range customerExpansionStages(initial, nil) {
		next = qualificationWorkerPlacements(&previous, next, workers)
		old := map[string]domain.Node{}
		for _, n := range previous.Spec.Nodes {
			old[n.ID] = n
		}
		for _, n := range next.Spec.Nodes {
			if before, ok := old[n.ID]; ok && (before.PlacementRef != n.PlacementRef || before.FailureDomain != n.FailureDomain) {
				t.Fatal("existing node moved during expansion")
			}
		}
		// Code images are fixture-unapproved in this test; validate membership's
		// immutable placement property independently of artifact authorization.
		if len(next.Spec.Configuration.Authorities) < len(previous.Spec.Configuration.Authorities) {
			t.Fatal("authority lost")
		}
		previous = next
	}
}
