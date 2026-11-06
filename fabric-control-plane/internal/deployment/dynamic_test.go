package deployment

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"sathonate.local/fabric-control-plane/internal/domain"
	"sathonate.local/fabric-control-plane/internal/kubernetesfabric"
	"sathonate.local/fabric-control-plane/internal/localfabric"
)

func dynamicDesign(t *testing.T) domain.NetworkSpec {
	t.Helper()
	f, e := os.Open("../../examples/dynamic/b.json")
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	s, e := domain.Decode(f)
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func dynamicOutcome(s domain.NetworkSpec) Outcome {
	o := Outcome{}
	orgs := map[string]string{}
	nodes := map[string]domain.Node{}
	for _, org := range s.Spec.Organizations {
		orgs[org.ID] = org.MSPID
	}
	for _, n := range s.Spec.Nodes {
		nodes[n.ID] = n
	}
	for _, cc := range s.Spec.Configuration.Chaincodes {
		h := localfabric.Health{Ready: true, Channel: cc.ChannelRef, Chaincode: localfabric.ContractHealth{Name: cc.Name, Available: true, Version: cc.Version, Sequence: cc.Sequence}}
		for _, c := range s.Spec.Channels {
			if c.ID != cc.ChannelRef {
				continue
			}
			for _, id := range c.PeerRefs {
				h.Peers = append(h.Peers, localfabric.Ledger{Peer: id, MSPID: orgs[nodes[id].OrganizationRef], Available: true, Height: 7, Hash: "equal-hash"})
			}
			for _, id := range c.OrdererRefs {
				h.Orderers = append(h.Orderers, localfabric.Consenter{Name: id, Status: "active", Relation: "consenter", Height: 7})
			}
		}
		for _, a := range s.Spec.Configuration.Authorities {
			h.CAs = append(h.CAs, localfabric.CAHealth{Name: a.ID, Available: true, TLS: true})
		}
		v := kubernetesfabric.DynamicVerification{Health: h, TransactionID: strings.Repeat("c", 64), PackageID: cc.Name + "_1:" + strings.Repeat("d", 64), ArtifactSHA256: cc.PackageSHA256, CodeVersion: "1.0"}
		o.Verifications = append(o.Verifications, v)
		o.Health = &h
		o.TransactionID = v.TransactionID
	}
	return o
}

func TestDynamicEvidenceRequiresAllConfiguredChannelsAndContracts(t *testing.T) {
	s := dynamicDesign(t)
	base := dynamicOutcome(s)
	if e := verifyEvidence(s, base); e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*Outcome)
	}{
		{"one channel omitted", func(o *Outcome) { o.Verifications = o.Verifications[:1] }},
		{"duplicate channel", func(o *Outcome) { o.Verifications[1] = o.Verifications[0] }},
		{"peer omitted", func(o *Outcome) { o.Verifications[0].Health.Peers = o.Verifications[0].Health.Peers[1:] }},
		{"foreign peer", func(o *Outcome) { o.Verifications[0].Health.Peers[0].Peer = "foreign" }},
		{"wrong identity", func(o *Outcome) { o.Verifications[0].Health.Peers[0].MSPID = "ForeignMSP" }},
		{"unequal ledger", func(o *Outcome) { o.Verifications[0].Health.Peers[0].Hash = "divergent" }},
		{"wrong artifact", func(o *Outcome) { o.Verifications[0].ArtifactSHA256 = strings.Repeat("e", 64) }},
		{"old package sequence", func(o *Outcome) { o.Verifications[0].PackageID = "customer-ledger_2:" + strings.Repeat("d", 64) }},
		{"orderer behind", func(o *Outcome) { o.Verifications[0].Health.Orderers[0].Height = 6 }},
		{"duplicate CA", func(o *Outcome) { o.Verifications[0].Health.CAs[1] = o.Verifications[0].Health.CAs[0] }},
		{"uncommitted tx", func(o *Outcome) { o.Verifications[0].TransactionID = "pending" }},
		{"no live code version", func(o *Outcome) { o.Verifications[0].CodeVersion = "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, _ := json.Marshal(base)
			var out Outcome
			if json.Unmarshal(raw, &out) != nil {
				t.Fatal("clone")
			}
			tc.mutate(&out)
			if verifyEvidence(s, out) == nil {
				t.Fatal("incomplete evidence qualified")
			}
		})
	}
}

func TestDynamicCannotFallBackToLegacyOrMockDeployment(t *testing.T) {
	s := dynamicDesign(t)
	m, e := Open(t.TempDir(), MockRunner{})
	if e != nil {
		t.Fatal(e)
	}
	defer m.Close()
	if _, e = m.Start("tenant", "actor", "record", s, 1, "dynamic-initial-request", "", ""); !errors.Is(e, ErrSecureIntegrationBlocked) {
		t.Fatal("dynamic intent reached mock or legacy runner", e)
	}
	if len(m.Operations().List("tenant")) != 0 {
		t.Fatal("blocked intent created operation")
	}
	if (SecureRunner{}).Validate(Request{Network: s}) == nil {
		t.Fatal("unbound dynamic runner accepted")
	}
}
