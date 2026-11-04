package kubernetesfabric

import (
	"sathonate.local/fabric-control-plane/internal/domain"
	"testing"
)

func TestBootstrapUsesOnlyOtherManagedPeersOfSameOrganization(t *testing.T) {
	spec := domain.NetworkSpec{Spec: domain.Topology{Nodes: []domain.Node{{ID: "p2", Role: "peer", OrganizationRef: "a"}, {ID: "p1", Role: "peer", OrganizationRef: "a"}, {ID: "foreign", Role: "peer", OrganizationRef: "b"}, {ID: "ord", Role: "orderer", OrganizationRef: "a"}}}}
	if got := peerBootstrap(spec, spec.Spec.Nodes[0], "owned-ns"); got != "p1.owned-ns:7051" {
		t.Fatal("unowned or self bootstrap", got)
	}
	if got := peerBootstrap(spec, spec.Spec.Nodes[2], "owned-ns"); got != "" {
		t.Fatal("cross-organization bootstrap", got)
	}
}
