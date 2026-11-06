package kubernetesfabric

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"google.golang.org/protobuf/proto"
	"sathonate.local/fabric-control-plane/internal/domain"
)

func TestDynamicCommittedPolicyIsDecodedIndependently(t *testing.T) {
	cc := domain.Chaincode{Version: "2.0", Sequence: 2, Endorsement: domain.Endorsement{Minimum: 2, MSPIDs: []string{"WarehouseMSP", "CarrierMSP", "AuditorMSP"}}}
	makeDefinition := func(mutate func(*common.SignaturePolicyEnvelope)) []byte {
		p := &common.SignaturePolicyEnvelope{Rule: &common.SignaturePolicy{Type: &common.SignaturePolicy_NOutOf_{NOutOf: &common.SignaturePolicy_NOutOf{N: 2}}}}
		for i, id := range cc.Endorsement.MSPIDs {
			role, _ := proto.Marshal(&msp.MSPRole{MspIdentifier: id, Role: msp.MSPRole_PEER})
			p.Identities = append(p.Identities, &msp.MSPPrincipal{PrincipalClassification: msp.MSPPrincipal_ROLE, Principal: role})
			p.Rule.GetNOutOf().Rules = append(p.Rule.GetNOutOf().Rules, &common.SignaturePolicy{Type: &common.SignaturePolicy_SignedBy{SignedBy: int32(i)}})
		}
		if mutate != nil {
			mutate(p)
		}
		raw, _ := proto.Marshal(&peer.ApplicationPolicy{Type: &peer.ApplicationPolicy_SignaturePolicy{SignaturePolicy: p}})
		out, _ := json.Marshal(Object{"version": "2.0", "sequence": 2, "endorsement_plugin": "escc", "validation_plugin": "vscc", "validation_parameter": raw})
		return out
	}
	if verifyDynamicDefinition(makeDefinition(nil), cc) != nil {
		t.Fatal("configured policy rejected")
	}
	for _, tc := range []struct {
		name   string
		mutate func(*common.SignaturePolicyEnvelope)
	}{
		{"weaker threshold", func(p *common.SignaturePolicyEnvelope) { p.Rule.GetNOutOf().N = 1 }},
		{"duplicate signer", func(p *common.SignaturePolicyEnvelope) { p.Rule.GetNOutOf().Rules[2] = p.Rule.GetNOutOf().Rules[0] }},
		{"foreign MSP", func(p *common.SignaturePolicyEnvelope) {
			p.Identities[0].Principal, _ = proto.Marshal(&msp.MSPRole{MspIdentifier: "ForeignMSP", Role: msp.MSPRole_PEER})
		}},
		{"client allowed", func(p *common.SignaturePolicyEnvelope) {
			p.Identities[0].Principal, _ = proto.Marshal(&msp.MSPRole{MspIdentifier: cc.Endorsement.MSPIDs[0], Role: msp.MSPRole_MEMBER})
		}},
		{"out of bounds signer", func(p *common.SignaturePolicyEnvelope) {
			p.Rule.GetNOutOf().Rules[0].Type = &common.SignaturePolicy_SignedBy{SignedBy: 99}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if verifyDynamicDefinition(makeDefinition(tc.mutate), cc) == nil {
				t.Fatal("changed committed policy accepted")
			}
		})
	}
	cc.Version = "3.0"
	if verifyDynamicDefinition(makeDefinition(nil), cc) == nil {
		t.Fatal("wrong definition version accepted")
	}
}

func TestDynamicPolicyRejectsExpressionInjection(t *testing.T) {
	p := domain.Endorsement{Minimum: 2, MSPIDs: []string{"ZetaMSP", "AlphaMSP"}}
	raw, e := dynamicPolicy(p)
	if e != nil || raw != "OutOf(2, 'AlphaMSP.peer', 'ZetaMSP.peer')" {
		t.Fatal(raw, e)
	}
	for _, ids := range [][]string{{"GoodMSP", "GoodMSP"}, {"GoodMSP", "EvilMSP')"}, {"GoodMSP", strings.Repeat("a", 65)}} {
		p.MSPIDs = ids
		if _, e = dynamicPolicy(p); e == nil {
			t.Fatal("unsafe endorsement expression accepted")
		}
	}
}

func TestApprovedPackageSourceDoesNotUseSubstringMatching(t *testing.T) {
	for _, raw := range []string{`{"source":{"Type":{"LocalPackage":{"package_id":"client_1:hash"}}}}`, `{"source":{"local_package":{"package_id":"client_1:hash"}}}`, `{"source":{"localPackage":{"packageId":"client_1:hash"}}}`} {
		id, e := approvedDynamicPackage([]byte(raw))
		if e != nil || id != "client_1:hash" {
			t.Fatal("official approved package framing rejected", e)
		}
	}
	for _, raw := range []string{`{"source":{"Type":{"Unavailable":{}}},"unrelated":"client_1:hash"}`, `{"source":{"local_package":{"package_id":"right","packageId":"wrong"}}}`, `{"source":{"local_package":{"package_id":"right"},"other":{}}}`} {
		if _, e := approvedDynamicPackage([]byte(raw)); e == nil {
			t.Fatal("ambiguous/unavailable approval source accepted")
		}
	}
}

func TestApprovalAbsentIsNotAnAuthorizationOrTrustBypass(t *testing.T) {
	missing := "could not fetch approved chaincode definition (name: 'customer-ledger', sequence: '1') on channel 'shipments'"
	if !strings.Contains(securePeerFailure([]byte(missing)).Error(), "(approval-absent)") {
		t.Fatal("missing approved definition not identified")
	}
	for _, suffix := range []string{"access denied", "authorization failure", "certificate signed by unknown authority", "connection refused", "deadline exceeded"} {
		if strings.Contains(securePeerFailure([]byte(missing+": "+suffix)).Error(), "(approval-absent)") {
			t.Fatal("trust/authorization failure reinterpreted as approval absence")
		}
	}
}
