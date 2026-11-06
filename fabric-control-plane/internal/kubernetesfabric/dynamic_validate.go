package kubernetesfabric

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"sathonate.local/fabric-control-plane/internal/domain"
	"sathonate.local/fabric-control-plane/internal/topology"
)

type CodeArtifact struct {
	SHA256      string `json:"sha256"`
	CodeVersion string `json:"codeVersion"`
}

// ValidateDynamic requires a server-enabled disposable target and immutable
// approved code artifacts. Customer placements must match trusted inventory.
func ValidateDynamic(s domain.NetworkSpec, r Runtime) error {
	deny := fmt.Errorf("dynamic local configuration is invalid or exceeds the approved target")
	if !r.DynamicEnabled || s.Spec.Configuration == nil || s.Spec.Deployment.TargetRef != domain.DynamicLocalProfile || s.Spec.Environment != "development" || s.Spec.Deployment.Provider != "local" || s.Spec.Deployment.Engine != "bevel" || s.Spec.FabricVersion != "2.5.15" || s.Spec.Ordering.Consensus != "raft" || s.Spec.Ordering.ChannelCapability != "V2_0" {
		return deny
	}
	copy := *s.Spec.Configuration
	copy.Chaincodes = append([]domain.Chaincode(nil), copy.Chaincodes...)
	for i, cc := range copy.Chaincodes {
		if cc.Sequence < 1 || cc.Sequence > 1024 || len(cc.Collections) > 0 {
			return deny
		}
		copy.Chaincodes[i].Sequence = 1
	}
	if !topology.Validate(topology.Contract{Network: s, Settings: copy}).Valid || len(s.Spec.Nodes) > 12 || len(s.Spec.Organizations) > 4 || len(s.Spec.Channels) > 4 || len(copy.Authorities) > 8 || len(copy.Chaincodes) < 1 || len(copy.Chaincodes) > 8 {
		return deny
	}
	if !r.BrokerChannels || !pinnedImage.MatchString(r.SmokeImage) || !regexp.MustCompile(`^[a-z][a-z0-9.-]{0,62}$`).MatchString(r.StorageClass) || !regexp.MustCompile(`^[1-9][0-9]{0,3}Gi$`).MatchString(r.Capacity) {
		return deny
	}
	for _, kind := range []string{"FabricCA", "FabricPeer", "FabricOrdererNode"} {
		if !qualifiedImage.MatchString(r.Images[kind]) {
			return deny
		}
	}
	maxGi, e := strconv.Atoi(strings.TrimSuffix(r.Capacity, "Gi"))
	if e != nil || maxGi < 1 {
		return deny
	}
	for _, p := range copy.Placements {
		approved := false
		for _, selector := range r.NodeSelectors {
			if reflect.DeepEqual(p.NodeSelector, selector) {
				approved = true
			}
		}
		if !approved {
			return deny
		}
	}
	budget := func(resources domain.Resources, storage domain.Storage) bool {
		return resources.CPULimitMilli <= 2000 && resources.MemoryLimitMiB <= 2048 && storage.Class == r.StorageClass && storage.CapacityGiB <= maxGi
	}
	for _, n := range s.Spec.Nodes {
		if len(n.ID) > 32 || !budget(copy.Nodes[n.ID].Resources, copy.Nodes[n.ID].Storage) {
			return deny
		}
	}
	for _, a := range copy.Authorities {
		if len(a.ID) > 32 || !budget(a.Resources, a.Storage) {
			return deny
		}
	}
	// Reserve capacity for the controller, issuer and versioned chaincode pods.
	// Reject oversized intent before creating any resource; namespace quota is
	// independently authoritative and is never raised to accommodate a design.
	cpu, memory, cpuLimit, memoryLimit, storage := 0, 0, 0, 0, 0
	account := func(n domain.NodeSettings) {
		cpu += n.Resources.CPURequestMilli
		memory += n.Resources.MemoryRequestMiB
		cpuLimit += n.Resources.CPULimitMilli
		memoryLimit += n.Resources.MemoryLimitMiB
		storage += n.Storage.CapacityGiB
	}
	for _, n := range copy.Nodes {
		account(n)
	}
	for _, a := range copy.Authorities {
		account(domain.NodeSettings{Resources: a.Resources, Storage: a.Storage})
	}
	if cpu > 14000 || memory > 14*1024 || cpuLimit > 34000 || memoryLimit > 34*1024 || storage > 30 || len(copy.Nodes)+len(copy.Authorities) > 18 || 8*len(copy.Nodes)+9*len(copy.Authorities)+8*len(s.Spec.Organizations)+3*len(copy.Chaincodes)+len(s.Spec.Channels) > 190 {
		return deny
	}
	for _, o := range s.Spec.Organizations {
		found := false
		for _, a := range copy.Authorities {
			if a.OrganizationRef == o.ID {
				for _, role := range a.Roles {
					if role == "signing" {
						found = a.ID == o.IdentityProviderRef
					}
				}
			}
		}
		if !found {
			return fmt.Errorf("organization identity reference must resolve to its configured signing authority")
		}
	}
	for _, ch := range s.Spec.Channels {
		if len(ch.ID) > 32 || len(ch.OrdererRefs) < 3 || len(ch.OrdererRefs)%2 == 0 {
			return deny
		}
		peers := map[string]domain.Node{}
		for _, n := range s.Spec.Nodes {
			peers[n.ID] = n
		}
		members := map[string]bool{}
		for _, p := range ch.PeerRefs {
			members[p] = true
		}
		anchors := map[string]bool{}
		for _, p := range copy.Anchors[ch.ID] {
			if !members[p] {
				return deny
			}
			anchors[peers[p].OrganizationRef] = true
		}
		for _, org := range ch.OrganizationRefs {
			if !anchors[org] {
				return fmt.Errorf("each channel organization requires a managed anchor peer")
			}
		}
		count := 0
		for _, cc := range copy.Chaincodes {
			if cc.ChannelRef == ch.ID {
				count++
			}
		}
		if count == 0 {
			return fmt.Errorf("each channel requires an approved chaincode and transaction verification")
		}
	}
	for _, cc := range copy.Chaincodes {
		a, ok := r.CodeArtifacts[cc.RuntimeImage]
		if len(cc.Name) > 24 || !ok || a.SHA256 != cc.PackageSHA256 || a.CodeVersion == "" || len(a.CodeVersion) > 64 || strings.ContainsAny(a.CodeVersion, "\x00\r\n") {
			return fmt.Errorf("chaincode must resolve to a server-approved immutable code artifact")
		}
	}
	return nil
}

// DynamicRuntime produces a copy. Customer input can select inventory entries,
// never mutate the trusted binding used for ownership checks or later retries.
func DynamicRuntime(s domain.NetworkSpec, r Runtime) (Runtime, error) {
	if e := r.Validate(s); e != nil {
		return Runtime{}, e
	}
	raw, _ := json.Marshal(r)
	var out Runtime
	json.Unmarshal(raw, &out)
	for _, n := range s.Spec.Nodes {
		out.NodeSelectors[n.ID] = cloneSelector(s.Spec.Configuration.Placements[n.PlacementRef].NodeSelector)
	}
	for _, a := range s.Spec.Configuration.Authorities {
		out.NodeSelectors[a.ID] = cloneSelector(s.Spec.Configuration.Placements[a.PlacementRef].NodeSelector)
	}
	return out, nil
}

func cloneSelector(v map[string]string) map[string]string {
	out := map[string]string{}
	for k, s := range v {
		out[k] = s
	}
	return out
}

func ValidateDynamicTransition(before, after domain.NetworkSpec) error {
	deny := fmt.Errorf("unsupported dynamic transition: removals, identity/storage replacement or skipped code sequences require a separate qualified procedure")
	if before.Spec.Configuration == nil || after.Spec.Configuration == nil {
		return deny
	}
	if before.Metadata.ID != after.Metadata.ID || before.Spec.Environment != after.Spec.Environment || before.Spec.FabricVersion != after.Spec.FabricVersion || !reflect.DeepEqual(before.Spec.Deployment, after.Spec.Deployment) || before.Spec.Ordering != after.Spec.Ordering {
		return deny
	}
	orgs := map[string]domain.Organization{}
	for _, o := range after.Spec.Organizations {
		orgs[o.ID] = o
	}
	for _, o := range before.Spec.Organizations {
		if orgs[o.ID] != o {
			return deny
		}
	}
	nodes := map[string]domain.Node{}
	for _, n := range after.Spec.Nodes {
		nodes[n.ID] = n
	}
	for _, n := range before.Spec.Nodes {
		if nodes[n.ID] != n || !reflect.DeepEqual(before.Spec.Configuration.Nodes[n.ID], after.Spec.Configuration.Nodes[n.ID]) || !reflect.DeepEqual(before.Spec.Configuration.Placements[n.PlacementRef], after.Spec.Configuration.Placements[n.PlacementRef]) {
			return deny
		}
	}
	cas := map[string]domain.Authority{}
	for _, a := range after.Spec.Configuration.Authorities {
		cas[a.ID] = a
	}
	for _, a := range before.Spec.Configuration.Authorities {
		if !reflect.DeepEqual(a, cas[a.ID]) || !reflect.DeepEqual(before.Spec.Configuration.Placements[a.PlacementRef], after.Spec.Configuration.Placements[a.PlacementRef]) {
			return deny
		}
	}
	channels := map[string]domain.Channel{}
	for _, c := range after.Spec.Channels {
		channels[c.ID] = c
	}
	subset := func(a, b []string) bool {
		set := map[string]bool{}
		for _, x := range b {
			set[x] = true
		}
		for _, x := range a {
			if !set[x] {
				return false
			}
		}
		return true
	}
	for _, c := range before.Spec.Channels {
		next, ok := channels[c.ID]
		if !ok || !subset(c.OrganizationRefs, next.OrganizationRefs) || !subset(c.PeerRefs, next.PeerRefs) || !subset(c.OrdererRefs, next.OrdererRefs) {
			return deny
		}
	}
	ccs := map[string]domain.Chaincode{}
	for _, c := range after.Spec.Configuration.Chaincodes {
		ccs[c.ChannelRef+"/"+c.Name] = c
	}
	for _, c := range before.Spec.Configuration.Chaincodes {
		next, ok := ccs[c.ChannelRef+"/"+c.Name]
		if !ok || next.Sequence < c.Sequence || next.Sequence > c.Sequence+1 {
			return deny
		}
		if next.Sequence == c.Sequence && !reflect.DeepEqual(c, next) {
			return deny
		}
	}
	old := map[string]bool{}
	for _, c := range before.Spec.Configuration.Chaincodes {
		old[c.ChannelRef+"/"+c.Name] = true
	}
	for _, c := range after.Spec.Configuration.Chaincodes {
		if !old[c.ChannelRef+"/"+c.Name] && c.Sequence != 1 {
			return deny
		}
	}
	return nil
}
