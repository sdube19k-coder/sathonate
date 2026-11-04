package topology

import (
	"encoding/json"
	"fmt"
	"sathonate.local/fabric-control-plane/internal/domain"
	"strings"
	"testing"
)

func fixture(distribution []int, orderers int) Contract {
	c := Contract{Network: domain.NetworkSpec{APIVersion: domain.APIVersion, Kind: "FabricNetwork", Metadata: domain.Metadata{ID: "custom-design", Name: "Custom configurable topology"}, Spec: domain.Topology{Environment: "development", FabricVersion: "2.5.15", Ordering: domain.Ordering{Consensus: "raft", ChannelCapability: "V2_0"}, Deployment: domain.Deployment{Engine: "native", Runtime: "linux-containers", TargetRef: "design-inventory"}}}, Settings: Settings{Placements: map[string]Placement{"local-node": {NodeSelector: map[string]string{"kubernetes.io/hostname": "disposable-node"}}}, Nodes: map[string]NodeSettings{}, Anchors: map[string][]string{}}}
	resources := Resources{100, 1000, 128, 1024}
	storage := Storage{"standard", 1}
	channel := domain.Channel{ID: "customer-channel"}
	for i, count := range distribution {
		org := fmt.Sprintf("org%d", i+1)
		msp := fmt.Sprintf("Org%dMSP", i+1)
		c.Network.Spec.Organizations = append(c.Network.Spec.Organizations, domain.Organization{ID: org, MSPID: msp, IdentityProviderRef: org + "-ca"})
		channel.OrganizationRefs = append(channel.OrganizationRefs, org)
		c.Settings.Authorities = append(c.Settings.Authorities, Authority{ID: org + "-ca", OrganizationRef: org, Roles: []string{"signing", "tls"}, PlacementRef: "local-node", Resources: resources, Storage: storage})
		for n := 1; n <= count; n++ {
			name := fmt.Sprintf("%s-peer-%d", org, n)
			c.Network.Spec.Nodes = append(c.Network.Spec.Nodes, domain.Node{ID: name, OrganizationRef: org, Role: "peer", PlacementRef: "local-node", FailureDomain: "local-host"})
			c.Settings.Nodes[name] = NodeSettings{resources, storage}
			channel.PeerRefs = append(channel.PeerRefs, name)
			if n == 1 {
				c.Settings.Anchors[channel.ID] = append(c.Settings.Anchors[channel.ID], name)
			}
		}
	}
	for n := 1; n <= orderers; n++ {
		name := fmt.Sprintf("orderer-%d", n)
		c.Network.Spec.Nodes = append(c.Network.Spec.Nodes, domain.Node{ID: name, OrganizationRef: "org1", Role: "orderer", PlacementRef: "local-node", FailureDomain: "local-host"})
		c.Settings.Nodes[name] = NodeSettings{resources, storage}
		channel.OrdererRefs = append(channel.OrdererRefs, name)
	}
	c.Network.Spec.Channels = []domain.Channel{channel}
	c.Settings.Chaincodes = []Chaincode{{Name: "customer-contract", ChannelRef: channel.ID, Version: "1.0", Sequence: 1, PackageSHA256: strings.Repeat("a", 64), RuntimeImage: "example/approved@sha256:" + strings.Repeat("b", 64), Endorsement: Endorsement{MSPIDs: []string{"Org1MSP", "Org2MSP"}, Minimum: 2}}}
	return c
}
func TestRequiredTopologyDesignMatrixIsNeverExecutionQualification(t *testing.T) {
	for _, tc := range []struct {
		name     string
		peers    []int
		orderers int
	}{{"A", []int{1, 1}, 3}, {"B", []int{2, 1, 2}, 3}, {"C", []int{2, 1}, 5}} {
		t.Run(tc.name, func(t *testing.T) {
			assessment := Validate(fixture(tc.peers, tc.orderers))
			if !assessment.Valid {
				t.Fatal(assessment.Issues)
			}
			if assessment.ExecutionEnabled || assessment.Qualified || len(assessment.ExecutionBlockers) == 0 {
				t.Fatal("valid design misreported as execution qualification")
			}
		})
	}
}
func TestVersionAwareBFTDesignOnly(t *testing.T) {
	c := fixture([]int{1, 1}, 4)
	c.Network.Spec.Ordering.Consensus = "bft"
	if Validate(c).Valid {
		t.Fatal("Fabric 2 BFT accepted")
	}
	c.Network.Spec.FabricVersion = "3.1.5"
	c.Network.Spec.Ordering.ChannelCapability = "V3_0"
	r := Validate(c)
	if !r.Valid || r.ExecutionEnabled || r.Qualified {
		t.Fatal(r)
	}
	c.Network.Spec.Ordering.ChannelCapability = "V2_0"
	if Validate(c).Valid {
		t.Fatal("BFT without V3 capability accepted")
	}
}
func TestUnsafeConfigurationRejected(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*Contract)
	}{
		{"even raft", func(c *Contract) {
			c.Network.Spec.Ordering.Consensus = "raft"
			c.Network.Spec.Nodes = append(c.Network.Spec.Nodes, domain.Node{ID: "orderer-4", Role: "orderer", OrganizationRef: "org1", PlacementRef: "local-node", FailureDomain: "local-host"})
		}},
		{"missing resource binding", func(c *Contract) { delete(c.Settings.Nodes, "org1-peer-1") }},
		{"overcommit", func(c *Contract) {
			n := c.Settings.Nodes["org1-peer-1"]
			n.Resources.CPURequestMilli = n.Resources.CPULimitMilli + 1
			c.Settings.Nodes["org1-peer-1"] = n
		}},
		{"unknown placement", func(c *Contract) { c.Network.Spec.Nodes[0].PlacementRef = "foreign" }},
		{"malformed selector", func(c *Contract) {
			c.Settings.Placements["local-node"] = Placement{NodeSelector: map[string]string{"evil..domain/key": "valid"}}
		}},
		{"foreign anchor", func(c *Contract) { c.Settings.Anchors["customer-channel"] = []string{"foreign-peer"} }},
		{"missing TLS authority", func(c *Contract) { c.Settings.Authorities[0].Roles = []string{"signing"} }},
		{"duplicate authority role", func(c *Contract) { c.Settings.Authorities[0].Roles = []string{"tls", "tls"} }},
		{"mutable code image", func(c *Contract) { c.Settings.Chaincodes[0].RuntimeImage = "example/code:latest" }},
		{"invalid endorsement", func(c *Contract) {
			c.Settings.Chaincodes[0].Endorsement = Endorsement{MSPIDs: []string{"ForeignMSP"}, Minimum: 1}
		}},
		{"initial sequence skipped", func(c *Contract) { c.Settings.Chaincodes[0].Sequence = 2 }},
		{"unsafe collection", func(c *Contract) {
			c.Settings.Chaincodes[0].Collections = []Collection{{Name: "private-data", Members: Endorsement{MSPIDs: []string{"Org1MSP"}, Minimum: 1}, RequiredPeerCount: 2, MaximumPeerCount: 1}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := fixture([]int{1, 1}, 3)
			tc.mutate(&c)
			if Validate(c).Valid {
				t.Fatal("unsafe design accepted")
			}
		})
	}
}
func TestIndependentAuthorityRoleDesign(t *testing.T) {
	c := fixture([]int{1, 1}, 3)
	tls := c.Settings.Authorities[0]
	tls.ID = "org1-tls-ca"
	tls.Roles = []string{"tls"}
	c.Settings.Authorities[0].Roles = []string{"signing"}
	c.Settings.Authorities = append(c.Settings.Authorities, tls)
	if r := Validate(c); !r.Valid || r.Qualified {
		t.Fatal(r)
	}
}
func TestStrictContractDecodingRejectsCredentialsAndTrailingDocuments(t *testing.T) {
	c := fixture([]int{1, 1}, 3)
	raw, _ := json.Marshal(c)
	if _, err := Decode(strings.NewReader(string(raw))); err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{`{"password":"credential"}`, string(raw) + ` {}`, `{"network":{},"settings":{"privateKey":"no"}}`} {
		if _, err := Decode(strings.NewReader(input)); err == nil {
			t.Fatal("unknown/private input accepted")
		}
	}
}
