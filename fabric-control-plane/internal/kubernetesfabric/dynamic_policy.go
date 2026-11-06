package kubernetesfabric

import (
	"bytes"
	"encoding/json"
	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"github.com/hyperledger/fabric-protos-go-apiv2/msp"
	"github.com/hyperledger/fabric-protos-go-apiv2/peer"
	"google.golang.org/protobuf/proto"
	"sathonate.local/fabric-control-plane/internal/domain"
)

func verifyDynamicDefinition(raw []byte, cc domain.Chaincode) error {
	var d struct {
		Version             string
		Sequence            int
		ValidationParameter []byte          `json:"validation_parameter"`
		EndorsementPlugin   string          `json:"endorsement_plugin"`
		ValidationPlugin    string          `json:"validation_plugin"`
		InitRequired        bool            `json:"init_required"`
		Collections         json.RawMessage `json:"collections"`
	}
	if json.Unmarshal(raw, &d) != nil || d.Version != cc.Version || d.Sequence != cc.Sequence || d.EndorsementPlugin != "escc" || d.ValidationPlugin != "vscc" || d.InitRequired {
		return secureDenied
	}
	if len(d.Collections) > 0 && !bytes.Equal(bytes.TrimSpace(d.Collections), []byte("null")) {
		var c struct {
			Config []json.RawMessage `json:"config"`
		}
		decoder := json.NewDecoder(bytes.NewReader(d.Collections))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&c) != nil || len(c.Config) > 0 {
			return secureDenied
		}
	}
	var app peer.ApplicationPolicy
	if proto.Unmarshal(d.ValidationParameter, &app) != nil {
		return secureDenied
	}
	p := app.GetSignaturePolicy()
	if p == nil || p.Version != 0 || p.Rule == nil || len(p.Identities) != len(cc.Endorsement.MSPIDs) {
		return secureDenied
	}
	rule := p.Rule.GetNOutOf()
	if rule == nil || int(rule.N) != cc.Endorsement.Minimum || len(rule.Rules) != len(cc.Endorsement.MSPIDs) {
		return secureDenied
	}
	want := map[string]bool{}
	for _, id := range cc.Endorsement.MSPIDs {
		if want[id] {
			return secureDenied
		}
		want[id] = true
	}
	used := map[int32]bool{}
	for _, r := range rule.Rules {
		if r == nil {
			return secureDenied
		}
		by, ok := r.Type.(*common.SignaturePolicy_SignedBy)
		if !ok || by.SignedBy < 0 || int(by.SignedBy) >= len(p.Identities) || used[by.SignedBy] {
			return secureDenied
		}
		used[by.SignedBy] = true
		i := p.Identities[by.SignedBy]
		var role msp.MSPRole
		if i == nil || i.PrincipalClassification != msp.MSPPrincipal_ROLE || proto.Unmarshal(i.Principal, &role) != nil || role.Role != msp.MSPRole_PEER || !want[role.MspIdentifier] {
			return secureDenied
		}
		delete(want, role.MspIdentifier)
	}
	if len(want) != 0 {
		return secureDenied
	}
	return nil
}

func approvedDynamicPackage(raw []byte) (string, error) {
	var response struct {
		Source map[string]json.RawMessage `json:"source"`
	}
	if json.Unmarshal(raw, &response) != nil {
		return "", secureDenied
	}
	var local json.RawMessage
	if len(response.Source) == 1 {
		if source, ok := response.Source["Type"]; ok {
			var typed map[string]json.RawMessage
			if json.Unmarshal(source, &typed) != nil || len(typed) != 1 {
				return "", secureDenied
			}
			local = typed["LocalPackage"]
		} else {
			local = response.Source["local_package"]
			if local == nil {
				local = response.Source["localPackage"]
			}
		}
	}
	var pkg struct {
		ID      string `json:"package_id"`
		CamelID string `json:"packageId"`
	}
	if len(local) == 0 || json.Unmarshal(local, &pkg) != nil {
		return "", secureDenied
	}
	if pkg.ID != "" && pkg.CamelID != "" {
		return "", secureDenied
	}
	if pkg.ID == "" {
		pkg.ID = pkg.CamelID
	}
	if pkg.ID == "" {
		return "", secureDenied
	}
	return pkg.ID, nil
}
