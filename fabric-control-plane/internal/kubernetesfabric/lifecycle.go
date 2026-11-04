package kubernetesfabric

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"google.golang.org/protobuf/proto"
	"sathonate.local/fabric-control-plane/internal/domain"
	"strings"
)

// LifecycleState is a bounded extension of the qualified local profile. It is
// separate from saved drafts and never enables arbitrary cloud/native execution.
type LifecycleState struct {
	PeerAdded bool `json:"peerAdded"`
	Sequence  int  `json:"sequence"`
}

func committedTarget(raw []byte, sequence int, version string) (bool, error) {
	var definitions []struct {
		Name, Version string
		Sequence      int
	}
	if len(strings.TrimSpace(string(raw))) != 0 && json.Unmarshal(raw, &definitions) != nil {
		return false, fmt.Errorf("invalid committed definitions response")
	}
	for _, d := range definitions {
		if d.Name == "smp_smoke" {
			if d.Sequence == sequence && d.Version == version {
				return true, nil
			}
			if sequence == 2 && d.Sequence == 1 && d.Version == "1.0" {
				return false, nil
			}
			return false, fmt.Errorf("committed definition conflicts with approved transition")
		}
	}
	if sequence != 1 {
		return false, fmt.Errorf("upgrade requires an existing sequence 1 definition")
	}
	return false, nil
}

func verifyCommittedDefinition(raw []byte, sequence int, version string) error {
	var d struct {
		Sequence            int
		Version             string
		ValidationParameter string `json:"validation_parameter"`
	}
	if json.Unmarshal(raw, &d) != nil || d.Sequence != sequence || d.Version != version {
		return secureDenied
	}
	encoded, e := base64.StdEncoding.DecodeString(d.ValidationParameter)
	if e != nil {
		return secureDenied
	}
	var app peer.ApplicationPolicy
	if proto.Unmarshal(encoded, &app) != nil {
		return secureDenied
	}
	policy := app.GetSignaturePolicy()
	if policy == nil || len(policy.Identities) != 2 {
		return secureDenied
	}
	rule := policy.Rule.GetNOutOf()
	if rule == nil || rule.N != 2 || len(rule.Rules) != 2 {
		return secureDenied
	}
	seen := map[string]bool{}
	indexes := map[int32]bool{}
	for _, r := range rule.Rules {
		if r == nil {
			return secureDenied
		}
		signed, ok := r.Type.(*common.SignaturePolicy_SignedBy)
		if !ok {
			return secureDenied
		}
		index := signed.SignedBy
		if r.GetNOutOf() != nil || index < 0 || index >= 2 || indexes[index] {
			return secureDenied
		}
		indexes[index] = true
		identity := policy.Identities[index]
		var role msp.MSPRole
		if identity.PrincipalClassification != msp.MSPPrincipal_ROLE || proto.Unmarshal(identity.Principal, &role) != nil || role.Role != msp.MSPRole_MEMBER || (role.MspIdentifier != "Org1MSP" && role.MspIdentifier != "Org2MSP") {
			return secureDenied
		}
		seen[role.MspIdentifier] = true
	}
	if len(seen) != 2 {
		return secureDenied
	}
	return nil
}

func (s LifecycleState) Validate() error {
	if s.Sequence != 1 && s.Sequence != 2 {
		return fmt.Errorf("only qualified chaincode sequences 1 and 2 are supported")
	}
	return nil
}
func (s LifecycleState) Version() string {
	if s.Sequence == 2 {
		return "2.0"
	}
	return "1.0"
}
func (s LifecycleState) Effective(base domain.NetworkSpec) domain.NetworkSpec {
	base.Spec.Nodes = append([]domain.Node(nil), base.Spec.Nodes...)
	base.Spec.Channels = append([]domain.Channel(nil), base.Spec.Channels...)
	if s.PeerAdded {
		for _, n := range base.Spec.Nodes {
			if n.ID == "org1-peer-1" {
				n.ID = "org1-peer-2"
				base.Spec.Nodes = append(base.Spec.Nodes, n)
				break
			}
		}
		base.Spec.Channels[0].PeerRefs = append(append([]string(nil), base.Spec.Channels[0].PeerRefs...), "org1-peer-2")
	}
	return base
}
