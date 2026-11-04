package kubernetesfabric

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"sathonate.local/fabric-control-plane/internal/domain"
)

const BevelCommit = "cd236ccc61d1d159b3dadb4625d08bb5682a8451"

// Runtime is supplied by the trusted adapter, never by a tenant network document.
// Empty Runtime retains the existing local profile.
type Runtime struct {
	StorageClass            string                       `json:"storageClass"`
	Capacity                string                       `json:"capacity"`
	SmokeImage              string                       `json:"smokeImage"`
	Images                  map[string]string            `json:"images"`
	NodeSelectors           map[string]map[string]string `json:"nodeSelectors"`
	DeniedPublicKeySHA256   []string                     `json:"deniedPublicKeySHA256,omitempty"`
	DeniedCertificateSHA256 []string                     `json:"deniedCertificateSHA256,omitempty"`
	BrokerChannels          bool                         `json:"brokerChannels"`
}

var pinnedImage = regexp.MustCompile(`^[a-zA-Z0-9./:_-]+@sha256:[a-f0-9]{64}$`)
var qualifiedImage = regexp.MustCompile(`^[a-zA-Z0-9./_-]+:[a-zA-Z0-9._-]+@sha256:[a-f0-9]{64}$`)

func ValidateProfile(s domain.NetworkSpec) error {
	if s.Spec.Deployment.TargetRef == domain.SecureLocalProfile || s.Spec.Deployment.TargetRef == domain.SecureEKSProfile {
		return fmt.Errorf("legacy Fabric worker cannot execute the secure-local-v1 profile; protected application issuer integration is required")
	}
	if domain.HasErrors(domain.Validate(s)) || s.Spec.FabricVersion != "2.5.15" || s.Spec.Ordering.Consensus != "raft" || s.Spec.Ordering.ChannelCapability != "V2_0" || len(s.Spec.Organizations) != 2 || len(s.Spec.Channels) != 1 {
		return fmt.Errorf("supported worker profile: Fabric 2.5.15, V2_0, two organizations, one channel, Raft")
	}
	peers, orderers := map[string]int{}, 0
	wantedPeers, wantedOrderers, wantedOrgs := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, org := range s.Spec.Organizations {
		if org.IdentityProviderRef != org.ID+"-ca" {
			return fmt.Errorf("worker requires the organization CA identity reference")
		}
		wantedOrgs[org.ID] = true
	}
	orderingOrg := ""
	for _, n := range s.Spec.Nodes {
		if len(n.ID) > 40 {
			return fmt.Errorf("node identifier exceeds the worker Helm resource-name limit")
		}
		if n.Role == "peer" {
			peers[n.OrganizationRef]++
			wantedPeers[n.ID] = true
		} else {
			wantedOrderers[n.ID] = true
			orderers++
			if orderingOrg == "" {
				orderingOrg = n.OrganizationRef
			}
			if orderingOrg != n.OrganizationRef {
				return fmt.Errorf("worker requires one ordering organization")
			}
		}
	}
	channel := s.Spec.Channels[0]
	for _, set := range []struct {
		refs   []string
		wanted map[string]bool
	}{{channel.OrganizationRefs, wantedOrgs}, {channel.PeerRefs, wantedPeers}, {channel.OrdererRefs, wantedOrderers}} {
		if len(set.refs) != len(set.wanted) {
			return fmt.Errorf("worker channel must include every organization, peer and orderer")
		}
		for _, ref := range set.refs {
			if !set.wanted[ref] {
				return fmt.Errorf("worker channel membership differs from the supported profile")
			}
			delete(set.wanted, ref)
		}
	}
	if len(peers) != 2 || orderers != 3 {
		return fmt.Errorf("worker requires one peer per organization and three orderers")
	}
	for _, count := range peers {
		if count != 1 {
			return fmt.Errorf("worker requires one peer per organization")
		}
	}
	// Channel CR names must fit Kubernetes DNS label limits.
	for _, o := range s.Spec.Organizations {
		if len(o.ID)+len(s.Spec.Channels[0].ID) > 37 {
			return fmt.Errorf("organization/channel identifiers exceed worker resource-name limit")
		}
	}
	return nil
}

func (r Runtime) Validate(s domain.NetworkSpec) error {
	if len(r.DeniedCertificateSHA256)+len(r.DeniedPublicKeySHA256) > 10000 {
		return fmt.Errorf("certificate denylist too large")
	}
	for _, fingerprint := range append(append([]string{}, r.DeniedCertificateSHA256...), r.DeniedPublicKeySHA256...) {
		if !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(fingerprint) {
			return fmt.Errorf("invalid public certificate fingerprint")
		}
	}

	if !regexp.MustCompile(`^[a-z][a-z0-9.-]{0,62}$`).MatchString(r.StorageClass) || !regexp.MustCompile(`^[1-9][0-9]{0,3}Gi$`).MatchString(r.Capacity) || !pinnedImage.MatchString(r.SmokeImage) || !r.BrokerChannels {
		return fmt.Errorf("invalid approved worker runtime")
	}
	for _, kind := range []string{"FabricCA", "FabricPeer", "FabricOrdererNode"} {
		if !qualifiedImage.MatchString(r.Images[kind]) {
			return fmt.Errorf("runtime requires digest-pinned Fabric images with explicit tags")
		}
	}
	for _, n := range s.Spec.Nodes {
		if len(r.NodeSelectors[n.ID]) == 0 {
			return fmt.Errorf("missing approved node placement")
		}
	}
	return nil
}

func (w *Worker) storageClass() string {
	if w.Runtime.StorageClass != "" {
		return w.Runtime.StorageClass
	}
	return "standard"
}
func (w *Worker) capacity() string {
	if w.Runtime.Capacity != "" {
		return w.Runtime.Capacity
	}
	return "1Gi"
}
func (w *Worker) smokeImage() string {
	if w.Runtime.SmokeImage != "" {
		return w.Runtime.SmokeImage
	}
	return "sathonate/smp-smoke:local"
}
func (w *Worker) smokePolicy() string {
	if w.Runtime.SmokeImage != "" {
		return "IfNotPresent"
	}
	return "Never"
}

func (w *Worker) adapt(v Object) error {
	if w.Runtime.StorageClass == "" {
		return nil
	}
	spec := object(v["spec"])
	kind := text(v["kind"])
	if image := w.Runtime.Images[kind]; image != "" {
		idx := strings.LastIndex(strings.Split(image, "@")[0], ":")
		spec["image"] = image[:idx]
		if kind == "FabricCA" {
			spec["version"] = image[idx+1:]
		} else {
			spec["tag"] = image[idx+1:]
		}
	}
	if selectors := w.Runtime.NodeSelectors[text(object(v["metadata"])["name"])]; len(selectors) > 0 {
		keys := make([]string, 0, len(selectors))
		for key := range selectors {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		expressions := []any{}
		for _, key := range keys {
			expressions = append(expressions, Object{"key": key, "operator": "In", "values": []string{selectors[key]}})
		}
		// v1.14.0's nodeSelector CR field is a corev1.NodeSelector but its
		// Helm chart expects a flat PodSpec map. Use supported node affinity
		// instead, avoiding that upstream schema/chart mismatch.
		spec["nodeSelector"] = nil
		affinity := object(spec["affinity"])
		affinity["nodeAffinity"] = Object{"requiredDuringSchedulingIgnoredDuringExecution": Object{"nodeSelectorTerms": []any{Object{"matchExpressions": expressions}}}}
		spec["affinity"] = affinity
	}
	v["spec"] = spec
	return nil
}

// ValidateChannelRequest checks every namespace, endpoint and identity reference
// before the trusted controller writes a cluster-scoped Bevel channel object.
// Only the shared worker's exact schema is accepted; unknown fields fail closed.
func ValidateChannelRequest(v Object, namespace string, s domain.NetworkSpec) error {
	if err := ValidateProfile(s); err != nil {
		return err
	}
	metadata := object(v["metadata"])
	spec := object(v["spec"])
	kind := text(v["kind"])
	if v["apiVersion"] != "hlf.kungfusoftware.es/v1alpha1" || len(v) != 4 || len(metadata) != 2 || object(metadata["labels"])["sathonate.io/namespace"] != namespace {
		return fmt.Errorf("invalid broker channel metadata")
	}
	channel := s.Spec.Channels[0].ID
	if spec["name"] != channel {
		return fmt.Errorf("channel does not match saved design")
	}
	allowed := map[string]bool{}
	if kind == "FabricMainChannel" {
		if metadata["name"] != namespace+"-"+channel {
			return fmt.Errorf("unapproved main channel name")
		}
		for _, key := range []string{"name", "adminOrdererOrganizations", "adminPeerOrganizations", "channelConfig", "externalOrdererOrganizations", "externalPeerOrganizations", "peerOrganizations", "identities", "ordererOrganizations", "orderers"} {
			allowed[key] = true
		}
	} else if kind == "FabricFollowerChannel" {
		found := false
		for _, o := range s.Spec.Organizations {
			if metadata["name"] == namespace+"-"+o.ID+"-"+channel && spec["mspId"] == o.MSPID {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("unapproved follower channel")
		}
		for _, key := range []string{"anchorPeers", "hlfIdentity", "mspId", "name", "externalPeersToJoin", "orderers", "peersToJoin"} {
			allowed[key] = true
		}
	} else {
		return fmt.Errorf("unapproved broker kind")
	}
	for key := range spec {
		if !allowed[key] {
			return fmt.Errorf("unapproved broker field")
		}
	}
	if len(spec) != len(allowed) {
		return fmt.Errorf("incomplete broker channel schema")
	}
	// Recursively reject foreign references. Certificates are public PEM only;
	// private material belongs exclusively to namespace-local Secrets.
	hosts := map[string]bool{}
	cas := map[string]bool{}
	msps := map[string]bool{}
	for _, n := range s.Spec.Nodes {
		for _, port := range []string{"7050", "7051", "7053"} {
			hosts[n.ID+"."+namespace+":"+port] = true
		}
		hosts[n.ID+"."+namespace] = true
	}
	for _, o := range s.Spec.Organizations {
		cas[o.ID+"-ca"] = true
		msps[o.MSPID] = true
	}
	var walk func(any) error
	walk = func(value any) error {
		switch x := value.(type) {
		case map[string]any:
			for key, val := range x {
				str, _ := val.(string)
				switch key {
				case "secretNamespace", "caNamespace", "namespace":
					if str != namespace {
						return fmt.Errorf("foreign channel namespace")
					}
				case "secretName":
					if str != "smp-wallet" {
						return fmt.Errorf("unapproved channel secret")
					}
				case "secretKey":
					found := false
					for _, o := range s.Spec.Organizations {
						if str == o.ID+"-ca.yaml" || str == o.ID+"-tlsca.yaml" {
							found = true
						}
					}
					if !found {
						return fmt.Errorf("unapproved wallet key")
					}
				case "caName":
					if !cas[str] {
						return fmt.Errorf("foreign channel CA")
					}
				case "mspID", "mspId":
					if !msps[str] {
						return fmt.Errorf("foreign channel MSP")
					}
				case "host":
					if !hosts[str] {
						return fmt.Errorf("foreign channel host")
					}
				case "url":
					if !strings.HasPrefix(str, "grpcs://") || !hosts[strings.TrimPrefix(str, "grpcs://")] {
						return fmt.Errorf("foreign channel URL")
					}
				case "ordererEndpoints":
					values, ok := val.([]any)
					if !ok {
						return fmt.Errorf("invalid endpoints")
					}
					for _, v := range values {
						if !hosts[text(v)] {
							return fmt.Errorf("foreign orderer endpoint")
						}
					}
				case "externalOrdererOrganizations", "externalPeerOrganizations", "orderersToJoin", "peersToJoin":
					list, ok := val.([]any)
					if !ok || len(list) != 0 {
						return fmt.Errorf("unsupported channel references")
					}
				}
				if err := walk(val); err != nil {
					return err
				}
			}
		case []any:
			for _, v := range x {
				if err := walk(v); err != nil {
					return err
				}
			}
		case string:
			if strings.Contains(x, "PRIVATE KEY") {
				return fmt.Errorf("private material in channel event")
			}
		}
		return nil
	}
	raw, _ := json.Marshal(spec)
	var normalized any
	_ = json.Unmarshal(raw, &normalized)
	return walk(normalized)
}
