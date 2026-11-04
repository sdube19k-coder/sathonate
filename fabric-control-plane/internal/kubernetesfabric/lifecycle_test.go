package kubernetesfabric

import (
	"encoding/base64"
	"encoding/json"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"google.golang.org/protobuf/proto"
	"os"
	"sathonate.local/fabric-control-plane/internal/domain"
	"testing"
)

func TestLifecycleDoesNotRelaxBaseProfile(t *testing.T) {
	raw, e := os.ReadFile("../../examples/secure-local-network.json")
	if e != nil {
		t.Fatal(e)
	}
	var base domain.NetworkSpec
	if json.Unmarshal(raw, &base) != nil {
		t.Fatal("fixture")
	}
	state := LifecycleState{PeerAdded: true, Sequence: 2}
	effective := state.Effective(base)
	if len(effective.Spec.Nodes) != 6 || len(base.Spec.Nodes) != 5 || len(effective.Spec.Channels[0].PeerRefs) != 3 || len(base.Spec.Channels[0].PeerRefs) != 2 {
		t.Fatal("lifecycle mutated saved design")
	}
	if !domain.HasErrors(domain.Validate(effective)) {
		t.Fatal("arbitrary extended draft accepted by base profile")
	}
	if (LifecycleState{Sequence: 3}).Validate() == nil {
		t.Fatal("unqualified sequence allowed")
	}
}
func TestCommittedTransitionRejectsRollbackAndSkippedSequences(t *testing.T) {
	for _, tc := range []struct {
		raw         string
		sequence    int
		version     string
		match, fail bool
	}{
		{`[]`, 1, "1.0", false, false}, {`[]`, 2, "2.0", false, true},
		{`[{"Name":"smp_smoke","Version":"1.0","Sequence":1}]`, 2, "2.0", false, false},
		{`[{"Name":"smp_smoke","Version":"2.0","Sequence":2}]`, 2, "2.0", true, false},
		{`[{"Name":"smp_smoke","Version":"2.0","Sequence":2}]`, 1, "1.0", false, true},
		{`{}`, 1, "1.0", false, true},
	} {
		match, e := committedTarget([]byte(tc.raw), tc.sequence, tc.version)
		if (e != nil) != tc.fail || match != tc.match {
			t.Fatal("unsafe committed definition transition", tc)
		}
	}
}
func TestCommittedPolicyRequiresBothOrganizationMembers(t *testing.T) {
	for _, weak := range []bool{false, true} {
		envelope := &common.SignaturePolicyEnvelope{Rule: &common.SignaturePolicy{Type: &common.SignaturePolicy_NOutOf_{NOutOf: &common.SignaturePolicy_NOutOf{N: 2, Rules: []*common.SignaturePolicy{{Type: &common.SignaturePolicy_SignedBy{SignedBy: 0}}, {Type: &common.SignaturePolicy_SignedBy{SignedBy: 1}}}}}}}
		for _, org := range []string{"Org1MSP", "Org2MSP"} {
			role, _ := proto.Marshal(&msp.MSPRole{MspIdentifier: org, Role: msp.MSPRole_MEMBER})
			envelope.Identities = append(envelope.Identities, &msp.MSPPrincipal{PrincipalClassification: msp.MSPPrincipal_ROLE, Principal: role})
		}
		if weak {
			envelope.Rule.GetNOutOf().N = 1
		}
		policy, _ := proto.Marshal(&peer.ApplicationPolicy{Type: &peer.ApplicationPolicy_SignaturePolicy{SignaturePolicy: envelope}})
		raw, _ := json.Marshal(map[string]any{"sequence": 2, "version": "2.0", "validation_parameter": base64.StdEncoding.EncodeToString(policy)})
		if (verifyCommittedDefinition(raw, 2, "2.0") != nil) != weak {
			t.Fatal("weak endorsement policy accepted")
		}
	}
}
