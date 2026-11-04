// Package domain defines the platform contract independently of either deployment engine.
package domain

import (
	"encoding/json"
	"fmt"
	"io"
)

const APIVersion = "fabric.sathonate.io/v1alpha1"

type NetworkSpec struct {
	APIVersion string   `json:"apiVersion"`
	Kind       string   `json:"kind"`
	Metadata   Metadata `json:"metadata"`
	Spec       Topology `json:"spec"`
}

type Metadata struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Topology struct {
	Environment   string         `json:"environment"`
	FabricVersion string         `json:"fabricVersion"`
	Deployment    Deployment     `json:"deployment"`
	Ordering      Ordering       `json:"ordering"`
	Organizations []Organization `json:"organizations"`
	Nodes         []Node         `json:"nodes"`
	Channels      []Channel      `json:"channels"`
}

type Ordering struct {
	Consensus         string `json:"consensus"`
	ChannelCapability string `json:"channelCapability"`
}

// WithDefaults preserves the implicit Raft behavior of pre-ordering drafts.
// Newly saved designs always persist explicit ordering configuration.
func WithDefaults(s NetworkSpec) NetworkSpec {
	if s.Spec.Ordering.Consensus == "" {
		s.Spec.Ordering.Consensus = "raft"
	}
	if s.Spec.Ordering.ChannelCapability == "" {
		s.Spec.Ordering.ChannelCapability = "V2_0"
		if len(s.Spec.FabricVersion) > 0 && s.Spec.FabricVersion[0] == '3' {
			s.Spec.Ordering.ChannelCapability = "V3_0"
		}
	}
	return s
}

type Deployment struct {
	Engine    string       `json:"engine"`
	TargetRef string       `json:"targetRef"`
	Runtime   string       `json:"runtime"`
	Provider  string       `json:"provider,omitempty"`
	Cloud     *CloudIntent `json:"cloud,omitempty"`
}

// CloudIntent contains user choices, never credentials or executable templates.
// Target registration independently controls the actual identity and cluster.
type CloudIntent struct {
	AccountID      string   `json:"accountId,omitempty"`
	RoleARN        string   `json:"roleArn,omitempty"`
	Region         string   `json:"region,omitempty"`
	TenantID       string   `json:"tenantId,omitempty"`
	SubscriptionID string   `json:"subscriptionId,omitempty"`
	ResourceGroup  string   `json:"resourceGroup,omitempty"`
	Domain         string   `json:"domain,omitempty"`
	ClusterMode    string   `json:"clusterMode"`
	ClusterName    string   `json:"clusterName,omitempty"`
	InstanceType   string   `json:"instanceType,omitempty"`
	KubeVersion    string   `json:"kubeVersion,omitempty"`
	MinNodes       int      `json:"minNodes"`
	DesiredNodes   int      `json:"desiredNodes"`
	MaxNodes       int      `json:"maxNodes"`
	VPCMode        string   `json:"vpcMode"`
	VPCCIDR        string   `json:"vpcCidr,omitempty"`
	ServiceCIDR    string   `json:"serviceCidr,omitempty"`
	PodCIDR        string   `json:"podCidr,omitempty"`
	Addons         []string `json:"addons"`
}

type Organization struct {
	ID                  string `json:"id"`
	MSPID               string `json:"mspId"`
	IdentityProviderRef string `json:"identityProviderRef"`
}

type Node struct {
	ID              string `json:"id"`
	OrganizationRef string `json:"organizationRef"`
	Role            string `json:"role"`
	PlacementRef    string `json:"placementRef"`
	FailureDomain   string `json:"failureDomain"`
}

type Channel struct {
	ID               string   `json:"id"`
	OrganizationRefs []string `json:"organizationRefs"`
	OrdererRefs      []string `json:"ordererRefs"`
	PeerRefs         []string `json:"peerRefs"`
}

// Decode rejects unknown fields and additional JSON documents. Never interpret input as code.
func Decode(r io.Reader) (NetworkSpec, error) {
	var s NetworkSpec
	d := json.NewDecoder(r)
	d.DisallowUnknownFields()
	if err := d.Decode(&s); err != nil {
		return s, fmt.Errorf("decode specification: %w", err)
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return s, fmt.Errorf("expected exactly one JSON document")
	}
	return WithDefaults(s), nil
}
