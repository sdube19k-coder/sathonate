package domain

// SecureLocalProfile is a reserved, versioned target. Acceptance of its design
// does not enable execution. Legacy runners must never interpret it as their
// existing smp-qualification target.
const SecureLocalProfile = "smp-secure-local-v1"

// SecureEKSProfile is reserved for the explicitly bound protected issuer adapter.
// Generic/legacy EKS runners must reject it rather than use the original controller.
const SecureEKSProfile = "smp-secure-eks-v1"

// ValidateSecureLocal narrows the design to the exact candidate channel builder
// and identity layout qualified at 1c7fca6. It performs no infrastructure calls.
func ValidateSecureLocal(s NetworkSpec) []Issue {
	issues := []Issue{}
	deny := func(path, message string) { issues = append(issues, Issue{"error", path, message}) }
	d := s.Spec.Deployment
	if d.TargetRef != SecureLocalProfile || d.Engine != "bevel" || d.Runtime != "kubernetes" || d.Provider != "local" || d.Cloud != nil || s.Spec.Environment != "development" {
		deny("spec.deployment", "secure-local-v1 requires the explicit local development target; cloud and production execution are disabled")
	}
	if s.Spec.FabricVersion != "2.5.15" || s.Spec.Ordering.Consensus != "raft" || s.Spec.Ordering.ChannelCapability != "V2_0" {
		deny("spec.ordering", "secure-local-v1 qualifies Fabric 2.5.15, Raft and V2_0 only")
	}
	orgs := map[string]string{"org1": "Org1MSP", "org2": "Org2MSP"}
	if len(s.Spec.Organizations) != len(orgs) {
		deny("spec.organizations", "secure-local-v1 requires exactly org1 and org2")
	}
	for _, org := range s.Spec.Organizations {
		if orgs[org.ID] == "" || orgs[org.ID] != org.MSPID || org.IdentityProviderRef != org.ID+"-ca" {
			deny("spec.organizations", "organization MSP and CA must match the qualified profile")
		}
		delete(orgs, org.ID)
	}
	if len(orgs) != 0 {
		deny("spec.organizations", "qualified organization is missing")
	}
	wanted := map[string][2]string{"org1-peer-1": {"org1", "peer"}, "org2-peer-1": {"org2", "peer"}, "orderer-1": {"org1", "orderer"}, "orderer-2": {"org1", "orderer"}, "orderer-3": {"org1", "orderer"}}
	if len(s.Spec.Nodes) != len(wanted) {
		deny("spec.nodes", "secure-local-v1 requires two peers and three orderers")
	}
	for _, node := range s.Spec.Nodes {
		pair, ok := wanted[node.ID]
		if !ok || pair != [2]string{node.OrganizationRef, node.Role} || node.PlacementRef != "kind-node" || node.FailureDomain != "local-host" {
			deny("spec.nodes", "node names, roles, organization and local placement must match the qualified profile")
		}
		delete(wanted, node.ID)
	}
	if len(wanted) != 0 {
		deny("spec.nodes", "qualified node is missing")
	}
	if len(s.Spec.Channels) != 1 || s.Spec.Channels[0].ID != "smpchannel" {
		deny("spec.channels", "secure-local-v1 requires the qualified smpchannel channel")
	} else {
		channel := s.Spec.Channels[0]
		for _, set := range []struct{ actual, expected []string }{
			{channel.OrganizationRefs, []string{"org1", "org2"}},
			{channel.PeerRefs, []string{"org1-peer-1", "org2-peer-1"}},
			{channel.OrdererRefs, []string{"orderer-1", "orderer-2", "orderer-3"}},
		} {
			refs := map[string]bool{}
			for _, ref := range set.expected {
				refs[ref] = true
			}
			valid := len(set.actual) == len(set.expected)
			for _, ref := range set.actual {
				valid = valid && refs[ref]
				delete(refs, ref)
			}
			if !valid || len(refs) != 0 {
				deny("spec.channels", "channel must contain every qualified organization, peer and orderer exactly once")
			}
		}
	}
	return issues
}
