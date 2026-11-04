package kubernetesfabric

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"sathonate.local/fabric-control-plane/internal/domain"
)

// Build the policy from organization membership, never peer slice positions.
// Adding a peer must not replace another organization's required endorsement.
func allPeerOrganizationsPolicy(orgs map[string]domain.Organization, peers []domain.Node) (string, error) {
	seen := map[string]bool{}
	for _, peer := range peers {
		org, ok := orgs[peer.OrganizationRef]
		if !ok || !regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`).MatchString(org.MSPID) {
			return "", fmt.Errorf("invalid peer organization for endorsement policy")
		}
		seen[org.MSPID] = true
	}
	if len(seen) < 2 {
		return "", fmt.Errorf("endorsement requires at least two distinct peer organizations")
	}
	terms := make([]string, 0, len(seen))
	for msp := range seen {
		terms = append(terms, "'"+msp+".member'")
	}
	sort.Strings(terms)
	return "AND(" + strings.Join(terms, ",") + ")", nil
}
