package kubernetesfabric

import (
	"testing"

	"sathonate.local/fabric-control-plane/internal/domain"
)

func TestPeerAdditionPreservesBothOrganizationEndorsements(t *testing.T) {
	orgs := map[string]domain.Organization{"a": {MSPID: "Org1MSP"}, "b": {MSPID: "Org2MSP"}}
	peers := []domain.Node{{OrganizationRef: "a"}, {OrganizationRef: "a"}, {OrganizationRef: "b"}}
	for _, nodes := range [][]domain.Node{peers, {peers[2], peers[0], peers[1]}, {peers[0], peers[2]}} {
		got, err := allPeerOrganizationsPolicy(orgs, nodes)
		if err != nil || got != "AND('Org1MSP.member','Org2MSP.member')" {
			t.Fatalf("peer count/order changed organization endorsements: %q %v", got, err)
		}
	}
	for _, nodes := range [][]domain.Node{{peers[0], peers[1]}, {{OrganizationRef: "unknown"}, peers[2]}} {
		if _, err := allPeerOrganizationsPolicy(orgs, nodes); err == nil {
			t.Fatal("invalid or single organization policy accepted")
		}
	}
	orgs["a"] = domain.Organization{MSPID: "Org1MSP.member')"}
	if _, err := allPeerOrganizationsPolicy(orgs, peers); err == nil {
		t.Fatal("policy expression injection accepted")
	}
}
