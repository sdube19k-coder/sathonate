package deployment

import (
	"errors"
	"regexp"
	"sathonate.local/fabric-control-plane/internal/domain"
	"strconv"
)

// Every configured contract on every configured channel must have independent
// committed-transaction evidence; the last worker event cannot stand for all.
func verifyDynamicEvidence(s domain.NetworkSpec, out Outcome) error {
	fail := errors.New("dynamic worker evidence is incomplete or differs from desired membership")
	if s.Spec.Configuration == nil || len(out.Verifications) != len(s.Spec.Configuration.Chaincodes) || len(out.Verifications) == 0 || out.Health == nil {
		return fail
	}
	orgs := map[string]string{}
	nodes := map[string]domain.Node{}
	channels := map[string]domain.Channel{}
	cas := map[string]bool{}
	for _, o := range s.Spec.Organizations {
		orgs[o.ID] = o.MSPID
	}
	for _, n := range s.Spec.Nodes {
		nodes[n.ID] = n
	}
	for _, c := range s.Spec.Channels {
		channels[c.ID] = c
	}
	for _, a := range s.Spec.Configuration.Authorities {
		cas[a.ID] = true
	}
	seen := map[string]bool{}
	for _, v := range out.Verifications {
		h := v.Health
		key := h.Channel + "/" + h.Chaincode.Name
		if seen[key] || !h.Ready || !h.Chaincode.Available || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(v.TransactionID) || v.CodeVersion == "" {
			return fail
		}
		seen[key] = true
		found := false
		for _, cc := range s.Spec.Configuration.Chaincodes {
			if cc.ChannelRef == h.Channel && cc.Name == h.Chaincode.Name {
				found = true
				if cc.Sequence != h.Chaincode.Sequence || cc.Version != h.Chaincode.Version || cc.PackageSHA256 != v.ArtifactSHA256 || !regexp.MustCompile(`^`+regexp.QuoteMeta(cc.Name)+`_`+strconv.Itoa(cc.Sequence)+`:[a-f0-9]{64}$`).MatchString(v.PackageID) {
					return fail
				}
			}
		}
		if !found {
			return fail
		}
		c, ok := channels[h.Channel]
		if !ok || len(h.Peers) != len(c.PeerRefs) || len(h.Orderers) != len(c.OrdererRefs) || len(h.CAs) != len(cas) || len(h.Peers) == 0 {
			return fail
		}
		peers := map[string]bool{}
		orders := map[string]bool{}
		authorities := map[string]bool{}
		for _, p := range c.PeerRefs {
			peers[p] = true
		}
		for _, o := range c.OrdererRefs {
			orders[o] = true
		}
		height, hash := h.Peers[0].Height, h.Peers[0].Hash
		if height < 2 || hash == "" {
			return fail
		}
		for _, p := range h.Peers {
			n := nodes[p.Peer]
			if !peers[p.Peer] || n.Role != "peer" || p.MSPID != orgs[n.OrganizationRef] || !p.Available || p.Height != height || p.Hash != hash {
				return fail
			}
			delete(peers, p.Peer)
		}
		for _, o := range h.Orderers {
			if !orders[o.Name] || o.Status != "active" || o.Relation != "consenter" || o.Height < height {
				return fail
			}
			delete(orders, o.Name)
		}
		for _, a := range h.CAs {
			if !cas[a.Name] || authorities[a.Name] || !a.Available || !a.TLS {
				return fail
			}
			authorities[a.Name] = true
		}
		if len(peers)+len(orders) != 0 {
			return fail
		}
	}
	last := out.Verifications[len(out.Verifications)-1]
	if out.TransactionID != last.TransactionID || out.Health.Channel != last.Health.Channel || out.Health.Chaincode != last.Health.Chaincode {
		return fail
	}
	return nil
}
