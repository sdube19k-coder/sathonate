package deployment

import (
	"errors"
	"regexp"
	"sathonate.local/fabric-control-plane/internal/domain"
	"sathonate.local/fabric-control-plane/internal/kubernetesfabric"
)

// A successful process is insufficient: require evidence for every desired participant.
func verifyEvidence(spec domain.NetworkSpec, outcome Outcome) error {
	return verifyLifecycleEvidence(spec, nil, outcome)
}
func verifyLifecycleEvidence(spec domain.NetworkSpec, state *kubernetesfabric.LifecycleState, outcome Outcome) error {
	sequence, version := 1, "1.0"
	if state != nil {
		if state.Validate() != nil {
			return errors.New("invalid lifecycle evidence target")
		}
		spec = state.Effective(spec)
		sequence, version = state.Sequence, state.Version()
	}
	fail := errors.New("worker did not provide complete verified Fabric transaction evidence")
	h := outcome.Health
	if h == nil || !h.Ready || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(outcome.TransactionID) || len(spec.Spec.Channels) != 1 || h.Channel != spec.Spec.Channels[0].ID || !h.Chaincode.Available || h.Chaincode.Name != "smp_smoke" || h.Chaincode.Sequence != sequence || h.Chaincode.Version != version {
		return fail
	}
	peers := map[string]string{}
	orderers := map[string]bool{}
	cas := map[string]bool{}
	orgs := map[string]string{}
	for _, org := range spec.Spec.Organizations {
		orgs[org.ID] = org.MSPID
		cas[org.ID+"-ca"] = true
	}
	for _, node := range spec.Spec.Nodes {
		if node.Role == "peer" {
			peers[node.ID] = orgs[node.OrganizationRef]
		} else {
			orderers[node.ID] = true
		}
	}
	if len(h.Peers) != len(peers) || len(h.Orderers) != len(orderers) || len(h.CAs) != len(cas) || len(h.Peers) == 0 {
		return fail
	}
	height := h.Peers[0].Height
	hash := h.Peers[0].Hash
	if height < 2 || hash == "" {
		return fail
	}
	for _, p := range h.Peers {
		if peers[p.Peer] == "" || peers[p.Peer] != p.MSPID || !p.Available || p.Height != height || p.Hash != hash {
			return fail
		}
		delete(peers, p.Peer)
	}
	for _, o := range h.Orderers {
		if !orderers[o.Name] || o.Status != "active" || o.Relation != "consenter" || o.Height < height {
			return fail
		}
		delete(orderers, o.Name)
	}
	for _, ca := range h.CAs {
		if !cas[ca.Name] || !ca.Available || !ca.TLS {
			return fail
		}
		delete(cas, ca.Name)
	}
	if len(peers)+len(orderers)+len(cas) != 0 {
		return fail
	}
	return nil
}
