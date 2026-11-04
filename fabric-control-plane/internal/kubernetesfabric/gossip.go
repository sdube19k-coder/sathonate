package kubernetesfabric

import (
	"sathonate.local/fabric-control-plane/internal/domain"
	"sort"
	"strings"
)

func peerBootstrap(spec domain.NetworkSpec, node domain.Node, namespace string) string {
	peers := []string{}
	for _, peer := range spec.Spec.Nodes {
		if peer.Role == "peer" && peer.OrganizationRef == node.OrganizationRef && peer.ID != node.ID {
			peers = append(peers, peer.ID+"."+namespace+":7051")
		}
	}
	sort.Strings(peers)
	return strings.Join(peers, " ")
}
