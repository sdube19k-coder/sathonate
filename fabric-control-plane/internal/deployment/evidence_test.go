package deployment

import (
	"sathonate.local/fabric-control-plane/internal/localfabric"
	"strings"
	"testing"
)

func TestReadyFlagCannotSubstituteForFabricEvidence(t *testing.T) {
	s := fixture(t)
	valid := func() Outcome {
		return Outcome{TransactionID: strings.Repeat("a", 64), Health: &localfabric.Health{Ready: true, Channel: "sathonate", Chaincode: localfabric.ContractHealth{Name: "smp_smoke", Available: true, Version: "1.0", Sequence: 1}, Peers: []localfabric.Ledger{{Peer: "peer1", MSPID: "Org1MSP", Available: true, Height: 9, Hash: "hash"}, {Peer: "peer2", MSPID: "Org2MSP", Available: true, Height: 9, Hash: "hash"}}, Orderers: []localfabric.Consenter{{Name: "orderer1", Status: "active", Relation: "consenter", Height: 9}, {Name: "orderer2", Status: "active", Relation: "consenter", Height: 9}, {Name: "orderer3", Status: "active", Relation: "consenter", Height: 9}}, CAs: []localfabric.CAHealth{{Name: "org1-ca", Available: true, TLS: true}, {Name: "org2-ca", Available: true, TLS: true}}}}
	}
	if err := verifyEvidence(s, valid()); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Outcome){func(o *Outcome) { o.Health = nil }, func(o *Outcome) { o.TransactionID = "mock-tx" }, func(o *Outcome) { o.Health.Peers[1].Hash = "different" }, func(o *Outcome) { o.Health.Peers[1] = o.Health.Peers[0] }, func(o *Outcome) { o.Health.CAs[0].TLS = false }, func(o *Outcome) { o.Health.Orderers[0].Height = 8 }, func(o *Outcome) { o.Health.Chaincode.Available = false }} {
		o := valid()
		mutate(&o)
		if verifyEvidence(s, o) == nil {
			t.Fatal("incomplete or inconsistent Fabric evidence accepted")
		}
	}
}
