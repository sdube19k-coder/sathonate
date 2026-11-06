package domain

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type Issue struct {
	Severity string `json:"severity"`
	Path     string `json:"path"`
	Message  string `json:"message"`
}

var resourceID = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
var mspID = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]{0,63}$`)
var fabricVersion = regexp.MustCompile(`^(2\.5|3\.(0|[1-9][0-9]*))\.(0|[1-9][0-9]*)$`)

func HasErrors(issues []Issue) bool {
	for _, i := range issues {
		if i.Severity == "error" {
			return true
		}
	}
	return false
}

// Validate performs offline topology checks. It cannot verify credentials, hosts, or live Fabric state.
func Validate(s NetworkSpec) []Issue {
	s = WithDefaults(s)
	issues := []Issue{}
	add := func(severity, path, message string) { issues = append(issues, Issue{severity, path, message}) }
	// Specifications are public intent and later enter immutable ConfigMaps.
	// Reject recognizable private material even when disguised as a display name.
	publicIntent, _ := json.Marshal(s)
	if regexp.MustCompile(`-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----|eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{16,}`).Match(publicIntent) {
		add("error", "spec", "private keys and bearer tokens are not accepted in public network intent")
	}
	checkID := func(path, value string) {
		if !resourceID.MatchString(value) {
			add("error", path, "must be a lowercase resource identifier, 1-63 characters")
		}
	}
	if s.APIVersion != APIVersion {
		add("error", "apiVersion", "unsupported API version")
	}
	if s.Kind != "FabricNetwork" {
		add("error", "kind", "must be FabricNetwork")
	}
	checkID("metadata.id", s.Metadata.ID)
	if strings.TrimSpace(s.Metadata.Name) == "" {
		add("error", "metadata.name", "name is required")
	}
	t := s.Spec
	if t.Environment != "development" && t.Environment != "production" {
		add("error", "spec.environment", "must be development or production")
	}
	if !fabricVersion.MatchString(t.FabricVersion) {
		add("error", "spec.fabricVersion", "planning supports exact 2.5.x or 3.x versions; no version is deployment-certified yet")
	} else {
		add("warning", "spec.fabricVersion", "version format accepted for planning only; executable compatibility profile is not certified")
	}
	d := t.Deployment
	issues = append(issues, validateCloudIntent(d)...)
	if d.TargetRef == SecureLocalProfile {
		issues = append(issues, ValidateSecureLocal(s)...)
	}
	ordering := t.Ordering
	if ordering.Consensus != "raft" && ordering.Consensus != "bft" {
		add("error", "spec.ordering.consensus", "choose raft or bft; Solo is a retired test-only consensus and is not supported")
	}
	if ordering.ChannelCapability != "V2_0" && ordering.ChannelCapability != "V3_0" {
		add("error", "spec.ordering.channelCapability", "must be V2_0 or V3_0")
	}
	isV3 := strings.HasPrefix(t.FabricVersion, "3.")
	if ordering.ChannelCapability == "V3_0" && !isV3 {
		add("error", "spec.ordering.channelCapability", "V3_0 channel capability requires Fabric 3.x")
	}
	if ordering.Consensus == "bft" {
		if !isV3 || ordering.ChannelCapability != "V3_0" {
			add("error", "spec.ordering.consensus", "SmartBFT requires Fabric 3.x and V3_0 channel capability")
		}
		if d.Engine != "native" {
			add("error", "spec.ordering.consensus", "BFT planning is available only for the native engine; this Bevel fork has no certified BFT path")
		}
		add("warning", "spec.ordering", "BFT is a design-only profile; organization trust, identity mappings and Fabric 3.x configuration require execution certification")
	}
	checkID("spec.deployment.targetRef", d.TargetRef)
	switch d.Engine {
	case "bevel":
		if isV3 {
			add("error", "spec.fabricVersion", "this Bevel integration plans Fabric 2.5.x only; use native planning for Fabric 3.x")
		}
		if d.Runtime != "kubernetes" {
			add("error", "spec.deployment.runtime", "Bevel requires kubernetes")
		}
		if d.Provider != "aws" && d.Provider != "azure" && d.Provider != "local" {
			add("error", "spec.deployment.provider", "Bevel target must be aws, azure or local")
		}
		if d.Provider == "local" && (t.Environment != "development" || (d.TargetRef != "smp-qualification" && d.TargetRef != SecureLocalProfile && d.TargetRef != DynamicLocalProfile)) {
			add("error", "spec.deployment", "local Kubernetes execution requires the development environment and smp-qualification target")
		}
	case "native":
		if d.Runtime != "linux-containers" {
			add("error", "spec.deployment.runtime", "initial native target is linux-containers")
		}
		if d.Provider != "" {
			add("error", "spec.deployment.provider", "native server placement uses target inventory, not a cloud provider")
		}
	default:
		add("error", "spec.deployment.engine", "must be bevel or native")
	}
	if len(t.Organizations) == 0 {
		add("error", "spec.organizations", "at least one organization is required")
	}
	orgs := map[string]Organization{}
	msps := map[string]bool{}
	for i, o := range t.Organizations {
		p := fmt.Sprintf("spec.organizations[%d]", i)
		checkID(p+".id", o.ID)
		if _, exists := orgs[o.ID]; exists {
			add("error", p+".id", "duplicate organization")
		}
		orgs[o.ID] = o
		if !mspID.MatchString(o.MSPID) || msps[o.MSPID] {
			add("error", p+".mspId", "MSP identifier must be valid and unique")
		}
		msps[o.MSPID] = true
		checkID(p+".identityProviderRef", o.IdentityProviderRef)
	}
	nodes := map[string]Node{}
	placements := map[string]string{}
	for i, n := range t.Nodes {
		p := fmt.Sprintf("spec.nodes[%d]", i)
		checkID(p+".id", n.ID)
		checkID(p+".placementRef", n.PlacementRef)
		checkID(p+".failureDomain", n.FailureDomain)
		if fd, exists := placements[n.PlacementRef]; exists && fd != n.FailureDomain {
			add("error", p+".failureDomain", "one placement cannot claim conflicting failure domains")
		}
		placements[n.PlacementRef] = n.FailureDomain
		if _, exists := nodes[n.ID]; exists {
			add("error", p+".id", "duplicate node")
		}
		nodes[n.ID] = n
		if _, ok := orgs[n.OrganizationRef]; !ok {
			add("error", p+".organizationRef", "organization does not exist")
		}
		if n.Role != "peer" && n.Role != "orderer" {
			add("error", p+".role", "must be peer or orderer")
		}
	}
	if len(t.Channels) == 0 {
		add("error", "spec.channels", "at least one channel is required by the initial create planner")
	}
	channels := map[string]bool{}
	for i, c := range t.Channels {
		p := fmt.Sprintf("spec.channels[%d]", i)
		checkID(p+".id", c.ID)
		if channels[c.ID] {
			add("error", p+".id", "duplicate channel")
		}
		channels[c.ID] = true
		members := map[string]bool{}
		for _, ref := range c.OrganizationRefs {
			if _, ok := orgs[ref]; !ok {
				add("error", p+".organizationRefs", "unknown organization: "+ref)
			}
			if members[ref] {
				add("error", p+".organizationRefs", "duplicate organization: "+ref)
			}
			members[ref] = true
		}
		if len(members) == 0 {
			add("error", p+".organizationRefs", "at least one channel organization is required")
		}
		domains := map[string]int{}
		ordererOwners := map[string]int{}
		peerOrgs := map[string]int{}
		for _, group := range []struct {
			refs        []string
			role, field string
		}{{c.OrdererRefs, "orderer", "ordererRefs"}, {c.PeerRefs, "peer", "peerRefs"}} {
			seen := map[string]bool{}
			if len(group.refs) == 0 {
				add("error", p+"."+group.field, "at least one node reference is required")
			}
			for _, ref := range group.refs {
				if seen[ref] {
					add("error", p+"."+group.field, "duplicate node: "+ref)
					continue
				}
				seen[ref] = true
				n, ok := nodes[ref]
				if !ok || n.Role != group.role {
					add("error", p+"."+group.field, "missing node or incorrect role: "+ref)
					continue
				}
				if !members[n.OrganizationRef] {
					add("error", p+"."+group.field, "node organization is not a channel member: "+ref)
				}
				if group.role == "orderer" {
					domains[n.FailureDomain]++
					ordererOwners[n.OrganizationRef]++
				} else {
					peerOrgs[n.OrganizationRef]++
				}
			}
		}
		count := len(c.OrdererRefs)
		severity := "warning"
		if t.Environment == "production" {
			severity = "error"
		}
		if ordering.Consensus == "raft" && (count < 3 || count%2 == 0) {
			add(severity, p+".ordererRefs", "production Raft profile requires at least 3 orderers and an odd count")
		}
		if ordering.Consensus == "bft" && (count < 4 || (count-1)%3 != 0) {
			add("error", p+".ordererRefs", "BFT planning requires 3f+1 orderers with f >= 1: use 4, 7 or 10 nodes")
		}
		quorum := count/2 + 1
		if ordering.Consensus == "bft" {
			quorum = count - (count-1)/3
			faultBudget := (count - 1) / 3
			for _, org := range c.OrganizationRefs {
				if ordererOwners[org] > faultBudget {
					add("warning", p+".ordererRefs", fmt.Sprintf("organization %s owns %d orderers, exceeding the BFT fault budget of %d; a full compromise of that organization is outside this design's fault assumptions", org, ordererOwners[org], faultBudget))
				}
			}
		}
		domainNames := make([]string, 0, len(domains))
		for fd := range domains {
			domainNames = append(domainNames, fd)
		}
		sort.Strings(domainNames)
		for _, fd := range domainNames {
			lost := domains[fd]
			if count-lost < quorum {
				add(severity, p+".ordererRefs", "loss of failure domain "+fd+" would lose quorum")
			}
		}
		for _, ref := range c.OrganizationRefs {
			if peerOrgs[ref] == 1 {
				add("warning", p+".peerRefs", "organization "+ref+" has one participating peer; no peer redundancy")
			}
		}
		// Inventory evidence must confirm declared failure domains before execution.
	}
	return issues
}
