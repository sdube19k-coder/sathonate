// Package topology validates configurable initial-deployment intent. It performs
// no execution and never upgrades a valid design into qualified infrastructure.
package topology

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"

	"sathonate.local/fabric-control-plane/internal/domain"
)

type Contract struct {
	Network  domain.NetworkSpec `json:"network"`
	Settings Settings           `json:"settings"`
}
type Settings = domain.Configuration
type Placement = domain.Placement
type Resources = domain.Resources
type Storage = domain.Storage
type NodeSettings = domain.NodeSettings
type Authority = domain.Authority
type Endorsement = domain.Endorsement
type Collection = domain.Collection
type Chaincode = domain.Chaincode

type Assessment struct {
	Valid             bool           `json:"valid"`
	ExecutionEnabled  bool           `json:"executionEnabled"`
	Qualified         bool           `json:"qualified"`
	Issues            []domain.Issue `json:"issues"`
	ExecutionBlockers []string       `json:"executionBlockers"`
}

func Decode(r io.Reader) (Contract, error) {
	var c Contract
	d := json.NewDecoder(io.LimitReader(r, 1<<20))
	d.DisallowUnknownFields()
	if d.Decode(&c) != nil {
		return c, fmt.Errorf("invalid configurable topology contract")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return c, fmt.Errorf("expected one topology contract")
	}
	c.Network = domain.WithDefaults(c.Network)
	return c, nil
}

var id = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
var labelName = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9_.-]{0,61}[A-Za-z0-9])?$`)
var dnsLabel = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

func selectorKey(key string) bool {
	parts := strings.Split(key, "/")
	if len(parts) > 2 || !labelName.MatchString(parts[len(parts)-1]) {
		return false
	}
	if len(parts) == 2 {
		if len(parts[0]) > 253 {
			return false
		}
		for _, part := range strings.Split(parts[0], ".") {
			if !dnsLabel.MatchString(part) {
				return false
			}
		}
	}
	return true
}

var digest = regexp.MustCompile(`^[a-f0-9]{64}$`)
var image = regexp.MustCompile(`^[a-zA-Z0-9./:_-]+@sha256:[a-f0-9]{64}$`)

func Validate(c Contract) Assessment {
	report := Assessment{Issues: domain.Validate(c.Network), ExecutionBlockers: []string{
		"Configurable initial deployment has no qualified protected execution adapter; existing fixed profile remains separate",
		"Namespace-scoped dynamic genesis/channel configuration and identity/Membership transitions require runtime acceptance",
		"Configurations A/B/C, actual code upgrades, storage/recovery and browser workflows require disposable Fabric transaction qualification",
	}}
	add := func(path, message string) {
		report.Issues = append(report.Issues, domain.Issue{Severity: "error", Path: path, Message: message})
	}
	resources := func(path string, r Resources) {
		if r.CPURequestMilli < 1 || r.CPULimitMilli < r.CPURequestMilli || r.CPULimitMilli > 8000 || r.MemoryRequestMiB < 64 || r.MemoryLimitMiB < r.MemoryRequestMiB || r.MemoryLimitMiB > 32768 {
			add(path, "use positive requests no greater than limits; maximum 8000m CPU and 32768Mi memory")
		}
	}
	storage := func(path string, s Storage) {
		if !id.MatchString(s.Class) || s.CapacityGiB < 1 || s.CapacityGiB > 1024 {
			add(path, "storage requires a named class and 1–1024Gi capacity; class availability/encryption require runtime checks")
		}
	}
	placement := func(path, ref string) {
		p, ok := c.Settings.Placements[ref]
		if !ok || len(p.NodeSelector) == 0 || len(p.NodeSelector) > 8 {
			add(path, "placement must resolve to 1–8 server-approved Kubernetes selectors")
		}
	}
	for ref, p := range c.Settings.Placements {
		if !id.MatchString(ref) {
			add("settings.placements", "invalid placement identifier")
		}
		for k, v := range p.NodeSelector {
			if !selectorKey(k) || !labelName.MatchString(v) {
				add("settings.placements."+ref, "invalid Kubernetes selector")
			}
		}
	}
	if len(c.Network.Spec.Organizations) < 1 || len(c.Network.Spec.Organizations) > 8 {
		add("network.spec.organizations", "support 1–8 organizations per configurable design")
	}
	orgs := map[string]string{}
	peers := map[string]int{}
	nodes := map[string]domain.Node{}
	orderers := 0
	for _, o := range c.Network.Spec.Organizations {
		orgs[o.ID] = o.MSPID
	}
	for _, n := range c.Network.Spec.Nodes {
		nodes[n.ID] = n
		if n.Role == "peer" {
			peers[n.OrganizationRef]++
		} else if n.Role == "orderer" {
			orderers++
		}
		p := "settings.nodes." + n.ID
		s, ok := c.Settings.Nodes[n.ID]
		if !ok {
			add(p, "explicit per-node resource and storage settings required")
		}
		resources(p+".resources", s.Resources)
		storage(p+".storage", s.Storage)
		placement(p+".placement", n.PlacementRef)
	}
	for name := range c.Settings.Nodes {
		if _, ok := nodes[name]; !ok {
			add("settings.nodes."+name, "settings refer to an unknown node")
		}
	}
	for org := range orgs {
		if peers[org] < 1 || peers[org] > 8 {
			add("network.spec.nodes", "each organization requires 1–8 peers")
		}
	}
	if orderers > 10 {
		add("network.spec.nodes", "configurable designs are bounded to ten orderers")
	}
	switch c.Network.Spec.Ordering.Consensus {
	case "raft":
		if orderers < 3 || orderers%2 == 0 {
			add("network.spec.ordering", "configurable Raft requires an odd orderer count of at least three")
		}
	case "bft":
		report.ExecutionBlockers = append(report.ExecutionBlockers, "BFT requires Fabric 3.x, V3_0, explicit consenter identity mapping and independent real transaction acceptance; it is not qualified")
	}
	authorityIDs := map[string]bool{}
	roles := map[string]map[string]bool{}
	counts := map[string]int{}
	for i, a := range c.Settings.Authorities {
		p := fmt.Sprintf("settings.authorities[%d]", i)
		if !id.MatchString(a.ID) || authorityIDs[a.ID] || nodes[a.ID].ID != "" {
			add(p+".id", "authority ID must be unique and not conflict with a node")
		}
		authorityIDs[a.ID] = true
		if orgs[a.OrganizationRef] == "" {
			add(p+".organizationRef", "unknown organization")
		}
		counts[a.OrganizationRef]++
		if roles[a.OrganizationRef] == nil {
			roles[a.OrganizationRef] = map[string]bool{}
		}
		if len(a.Roles) < 1 || len(a.Roles) > 2 {
			add(p+".roles", "use signing and/or tls authority roles")
		}
		for _, role := range a.Roles {
			if (role != "signing" && role != "tls") || roles[a.OrganizationRef][role] {
				add(p+".roles", "authority roles must be supported and unique within the organization")
			}
			roles[a.OrganizationRef][role] = true
		}
		resources(p+".resources", a.Resources)
		storage(p+".storage", a.Storage)
		placement(p+".placementRef", a.PlacementRef)
	}
	for org := range orgs {
		if counts[org] < 1 || counts[org] > 2 || !roles[org]["signing"] || !roles[org]["tls"] {
			add("settings.authorities", "each organization requires exactly one signing and one TLS role, in one or two CA services")
		}
	}
	channels := map[string]domain.Channel{}
	for _, channel := range c.Network.Spec.Channels {
		channels[channel.ID] = channel
		seen := map[string]bool{}
		member := map[string]bool{}
		for _, o := range channel.OrganizationRefs {
			member[o] = true
		}
		for _, peer := range c.Settings.Anchors[channel.ID] {
			n := nodes[peer]
			if seen[peer] || n.Role != "peer" || !member[n.OrganizationRef] {
				add("settings.anchors."+channel.ID, "anchors must be distinct channel-member peers")
			}
			seen[peer] = true
		}
	}
	for channel := range c.Settings.Anchors {
		if _, ok := channels[channel]; !ok {
			add("settings.anchors."+channel, "unknown channel")
		}
	}
	endorsement := func(path string, p Endorsement, members map[string]bool) {
		seen := map[string]bool{}
		if p.Minimum < 1 || p.Minimum > len(p.MSPIDs) {
			add(path, "endorsement threshold must be 1..number of distinct MSPs")
		}
		for _, msp := range p.MSPIDs {
			if seen[msp] || !members[msp] {
				add(path, "policy MSPs must be distinct channel members")
			}
			seen[msp] = true
		}
	}
	chaincodes := map[string]bool{}
	for i, cc := range c.Settings.Chaincodes {
		p := fmt.Sprintf("settings.chaincodes[%d]", i)
		channel, ok := channels[cc.ChannelRef]
		if !ok {
			add(p+".channelRef", "unknown channel")
		}
		members := map[string]bool{}
		for _, org := range channel.OrganizationRefs {
			members[orgs[org]] = true
		}
		key := cc.ChannelRef + "/" + cc.Name
		if chaincodes[key] || !id.MatchString(cc.Name) {
			add(p+".name", "chaincode name must be valid and unique per channel")
		}
		chaincodes[key] = true
		if cc.Sequence != 1 {
			add(p+".sequence", "initial deployment requires sequence 1; upgrades are separate approved operations")
		}
		if cc.Version == "" || len(cc.Version) > 64 || strings.ContainsAny(cc.Version, "\r\n\x00") || !digest.MatchString(cc.PackageSHA256) || !image.MatchString(cc.RuntimeImage) {
			add(p, "version, package SHA256 and immutable approved runtime image are required")
		}
		endorsement(p+".endorsement", cc.Endorsement, members)
		collections := map[string]bool{}
		for j, col := range cc.Collections {
			cp := fmt.Sprintf("%s.collections[%d]", p, j)
			if !id.MatchString(col.Name) || collections[col.Name] {
				add(cp+".name", "collection names must be valid and unique")
			}
			collections[col.Name] = true
			endorsement(cp+".members", col.Members, members)
			if col.RequiredPeerCount < 0 || col.MaximumPeerCount < col.RequiredPeerCount || col.MaximumPeerCount > 64 {
				add(cp, "collection dissemination requires 0 <= required <= maximum <= 64")
			}
		}
	}
	report.Valid = !domain.HasErrors(report.Issues)
	return report
}
