package httpapi

import (
	"encoding/json"
	"os"
	"sathonate.local/fabric-control-plane/internal/deployment"
	"sathonate.local/fabric-control-plane/internal/domain"
	"sathonate.local/fabric-control-plane/internal/kubernetesfabric"
	"sathonate.local/fabric-control-plane/internal/localfabric"
	"strings"
	"testing"
)

// Customer specifications used only by the disposable acceptance fixture. The
// executor has no knowledge of these names, counts or test layouts.
func expandedCustomerDesign(before domain.NetworkSpec, artifacts map[string]kubernetesfabric.CodeArtifact) domain.NetworkSpec {
	raw, _ := json.Marshal(before)
	var after domain.NetworkSpec
	_ = json.Unmarshal(raw, &after)
	first := after.Spec.Organizations[0]
	placement := after.Spec.Nodes[0].PlacementRef
	settings := after.Spec.Configuration.Nodes[after.Spec.Nodes[0].ID]
	newOrg := domain.Organization{ID: "inspection", MSPID: "InspectionMSP", IdentityProviderRef: "inspection-root"}
	after.Spec.Organizations = append(after.Spec.Organizations, newOrg)
	a := after.Spec.Configuration.Authorities[0]
	a.ID = newOrg.IdentityProviderRef
	a.OrganizationRef = newOrg.ID
	after.Spec.Configuration.Authorities = append(after.Spec.Configuration.Authorities, a)
	newPeers := []domain.Node{{ID: first.ID + "-expanded", OrganizationRef: first.ID, Role: "peer", PlacementRef: placement, FailureDomain: "local-host"}, {ID: "inspection-east", OrganizationRef: newOrg.ID, Role: "peer", PlacementRef: placement, FailureDomain: "local-host"}}
	newOrders := []domain.Node{{ID: "raft-expanded-east", OrganizationRef: first.ID, Role: "orderer", PlacementRef: placement, FailureDomain: "local-host"}, {ID: "raft-expanded-west", OrganizationRef: first.ID, Role: "orderer", PlacementRef: placement, FailureDomain: "local-host"}}
	for _, n := range append(newPeers, newOrders...) {
		after.Spec.Nodes = append(after.Spec.Nodes, n)
		after.Spec.Configuration.Nodes[n.ID] = settings
	}
	for i := range after.Spec.Channels {
		c := &after.Spec.Channels[i]
		c.OrganizationRefs = append(c.OrganizationRefs, newOrg.ID)
		for _, n := range newPeers {
			c.PeerRefs = append(c.PeerRefs, n.ID)
		}
		for _, n := range newOrders {
			c.OrdererRefs = append(c.OrdererRefs, n.ID)
		}
		anchors := []string{}
		for _, id := range after.Spec.Configuration.Anchors[c.ID] {
			if id != before.Spec.Nodes[0].ID {
				anchors = append(anchors, id)
			}
		}
		anchors = append(anchors, newPeers[0].ID, newPeers[1].ID)
		after.Spec.Configuration.Anchors[c.ID] = anchors
	}
	for i := range after.Spec.Configuration.Chaincodes {
		cc := &after.Spec.Configuration.Chaincodes[i]
		cc.Sequence++
		cc.Version = "2.0"
		cc.Endorsement.MSPIDs = append(cc.Endorsement.MSPIDs, newOrg.MSPID)
		cc.Endorsement.Minimum = len(cc.Endorsement.MSPIDs)
		for image, a := range artifacts {
			if a.CodeVersion == "2.0" {
				cc.RuntimeImage = image
				cc.PackageSHA256 = a.SHA256
			}
		}
	}
	c := after.Spec.Channels[0]
	c.ID = "customer-added-ledger"
	after.Spec.Channels = append(after.Spec.Channels, c)
	after.Spec.Configuration.Anchors[c.ID] = append([]string(nil), after.Spec.Configuration.Anchors[after.Spec.Channels[0].ID]...)
	cc := after.Spec.Configuration.Chaincodes[0]
	cc.ChannelRef = c.ID
	cc.Sequence = 1
	after.Spec.Configuration.Chaincodes = append(after.Spec.Configuration.Chaincodes, cc)
	return after
}

func deploymentSnapshot(v deployment.Result) (deployment.Result, error) {
	raw, e := json.Marshal(v)
	if e != nil {
		return deployment.Result{}, e
	}
	var copy deployment.Result
	e = json.Unmarshal(raw, &copy)
	return copy, e
}

func customerExpansionStages(before domain.NetworkSpec, artifacts map[string]kubernetesfabric.CodeArtifact) []domain.NetworkSpec {
	full := expandedCustomerDesign(before, artifacts)
	clone := func(s domain.NetworkSpec) domain.NetworkSpec {
		raw, _ := json.Marshal(s)
		var copy domain.NetworkSpec
		_ = json.Unmarshal(raw, &copy)
		return copy
	}
	peerOnly := clone(before)
	peer := full.Spec.Nodes[len(before.Spec.Nodes)]
	peerOnly.Spec.Nodes = append(peerOnly.Spec.Nodes, peer)
	peerOnly.Spec.Configuration.Nodes[peer.ID] = full.Spec.Configuration.Nodes[peer.ID]
	for i := range peerOnly.Spec.Channels {
		c := &peerOnly.Spec.Channels[i]
		c.PeerRefs = append(c.PeerRefs, peer.ID)
		anchors := []string{}
		for _, id := range full.Spec.Configuration.Anchors[c.ID] {
			if id != "inspection-east" {
				anchors = append(anchors, id)
			}
		}
		peerOnly.Spec.Configuration.Anchors[c.ID] = anchors
	}
	orgOnly := clone(peerOnly)
	orgOnly.Spec.Organizations = append(orgOnly.Spec.Organizations, full.Spec.Organizations[len(full.Spec.Organizations)-1])
	orgOnly.Spec.Configuration.Authorities = append(orgOnly.Spec.Configuration.Authorities, full.Spec.Configuration.Authorities[len(full.Spec.Configuration.Authorities)-1])
	peer = full.Spec.Nodes[len(before.Spec.Nodes)+1]
	orgOnly.Spec.Nodes = append(orgOnly.Spec.Nodes, peer)
	orgOnly.Spec.Configuration.Nodes[peer.ID] = full.Spec.Configuration.Nodes[peer.ID]
	for i := range orgOnly.Spec.Channels {
		c := &orgOnly.Spec.Channels[i]
		c.OrganizationRefs = append(c.OrganizationRefs, "inspection")
		c.PeerRefs = append(c.PeerRefs, peer.ID)
		orgOnly.Spec.Configuration.Anchors[c.ID] = append([]string(nil), full.Spec.Configuration.Anchors[c.ID]...)
	}
	return []domain.NetworkSpec{peerOnly, orgOnly, full}
}

func TestDeploymentComparisonSnapshotSurvivesJSONDecodeReuse(t *testing.T) {
	result := deployment.Result{Health: &localfabric.Health{Channel: "before"}, Verifications: []kubernetesfabric.DynamicVerification{{CodeVersion: "1.0"}}}
	before, e := deploymentSnapshot(result)
	if e != nil {
		t.Fatal(e)
	}
	changed := deployment.Result{Health: &localfabric.Health{Channel: "after"}, Verifications: []kubernetesfabric.DynamicVerification{{CodeVersion: "2.0"}, {CodeVersion: "2.0"}}}
	raw, _ := json.Marshal(changed)
	if json.Unmarshal(raw, &result) != nil {
		t.Fatal("decode")
	}
	if before.Health.Channel != "before" || before.Verifications[0].CodeVersion != "1.0" {
		t.Fatal("acceptance comparison snapshot was overwritten by later API decoding")
	}
}

func TestCustomerStagesExerciseIndependentMembershipAndRealCodeChange(t *testing.T) {
	raw, e := os.ReadFile("../../examples/dynamic/a.json")
	if e != nil {
		t.Fatal(e)
	}
	var before domain.NetworkSpec
	if json.Unmarshal(raw, &before) != nil {
		t.Fatal("fixture")
	}
	artifacts := map[string]kubernetesfabric.CodeArtifact{"example/v2@sha256:" + strings.Repeat("d", 64): {SHA256: strings.Repeat("e", 64), CodeVersion: "2.0"}}
	stages := customerExpansionStages(before, artifacts)
	for i, next := range stages {
		if e := kubernetesfabric.ValidateDynamicTransition(before, next); e != nil {
			t.Fatal(i, e)
		}
		if i < 2 && next.Spec.Configuration.Chaincodes[0].Sequence != 1 {
			t.Fatal("membership test implicitly upgraded code")
		}
		before = next
	}
	if len(stages[0].Spec.Organizations) != 2 || len(stages[0].Spec.Nodes) != 6 || len(stages[1].Spec.Organizations) != 3 || len(stages[1].Spec.Nodes) != 7 || len(stages[2].Spec.Nodes) != 9 || len(stages[2].Spec.Channels) != 2 {
		t.Fatal("independent customer expansion stages differ")
	}
}
