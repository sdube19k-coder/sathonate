// Package rollout prepares one immutable, UID-bound Fabric image change.
// A caller must supply observations from its protected target adapter.
package rollout

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"time"

	"sathonate.local/fabric-control-plane/internal/domain"
	"sathonate.local/fabric-control-plane/internal/identity"
	"sathonate.local/fabric-control-plane/internal/maintenance"
)

var imageDigest = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:/-]*@sha256:[a-f0-9]{64}$`)
var digest = regexp.MustCompile(`^[a-f0-9]{64}$`)
var componentID = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

type Component struct {
	ID, Kind, Namespace, Deployment, UID, ResourceVersion, Image, StorageIdentitySHA256 string
	Container                                                                           string
	ContainerIndex                                                                      int
	Ready                                                                               bool
	Rolling                                                                             bool
}

// ChannelState is supplied by the trusted Fabric observation adapter.
type ChannelState struct {
	ID           string
	ActiveVoters []string
	LedgerSHA256 string
}

type Snapshot struct {
	Channels                    []ChannelState
	BindingSHA256, NamespaceUID string
	ObservedAt                  time.Time
	Components                  []Component
	LedgerVerified              bool
	BackupVerified              bool
	EnrollmentPaused            bool
}

type Profile struct {
	// Trusted, release-reviewed edges keyed by kind, old image, then new image.
	// An approved edge is required even for a same-version image replacement.
	Edges map[string]map[string][]string
}

type Plan struct {
	InventorySHA256 string
	NamespaceUID    string
	Binding         maintenance.Binding
	ActorSHA256     string
	Component       Component
	TargetImage     string
	Channels        []string
	Digest          string
}

func topologyDigest(spec domain.NetworkSpec) string {
	raw, _ := json.Marshal(spec)
	return maintenance.HashBytes(raw)
}

func Preview(p identity.Principal, binding maintenance.Binding, spec domain.NetworkSpec, observation Snapshot, targetID, targetImage string, approved Profile, now time.Time) (Plan, error) {
	fail := errors.New("upgrade requires an approved image transition and current exact-target readiness, storage and quorum evidence")
	if !p.CanAdminister() || p.ProjectID == "" || p.StorageTenant() != binding.TenantID || binding.NetworkID == "" || !digest.MatchString(binding.TargetSHA256) || binding.NetworkSHA256 != topologyDigest(spec) || !imageDigest.MatchString(targetImage) || observation.BindingSHA256 != binding.Digest() || observation.NamespaceUID == "" || !observation.LedgerVerified || !observation.BackupVerified || observation.ObservedAt.After(now) || now.Sub(observation.ObservedAt) > time.Minute {
		return Plan{}, fail
	}
	if spec.Spec.Ordering.Consensus != "raft" || spec.Spec.Configuration == nil {
		return Plan{}, fail
	}
	expected := map[string]string{}
	for _, n := range spec.Spec.Nodes {
		if !componentID.MatchString(n.ID) || expected[n.ID] != "" || (n.Role != "peer" && n.Role != "orderer") {
			return Plan{}, fail
		}
		expected[n.ID] = n.Role
	}
	for _, ca := range spec.Spec.Configuration.Authorities {
		if !componentID.MatchString(ca.ID) || expected[ca.ID] != "" {
			return Plan{}, fail
		}
		expected[ca.ID] = "ca"
	}
	seen := map[string]Component{}
	namespace := ""
	deployments, uids := map[string]bool{}, map[string]bool{}
	for _, c := range observation.Components {
		if c.Kind == "" || c.Kind != expected[c.ID] || seen[c.ID].ID != "" || !componentID.MatchString(c.Deployment) || !componentID.MatchString(c.Namespace) || !componentID.MatchString(c.Container) || c.UID == "" || c.ResourceVersion == "" || !imageDigest.MatchString(c.Image) || !digest.MatchString(c.StorageIdentitySHA256) || c.Rolling || !c.Ready || c.ContainerIndex < 0 {
			return Plan{}, fail
		}
		if namespace == "" {
			namespace = c.Namespace
		}
		if c.Namespace != namespace || deployments[c.Deployment] || uids[c.UID] {
			return Plan{}, fail
		}
		seen[c.ID] = c
		deployments[c.Deployment], uids[c.UID] = true, true
	}
	if len(seen) != len(expected) {
		return Plan{}, fail
	}
	component, ok := seen[targetID]
	if !ok || targetImage == component.Image || (component.Kind == "ca" && !observation.EnrollmentPaused) {
		return Plan{}, fail
	}
	allowed := false
	for _, image := range approved.Edges[component.Kind][component.Image] {
		if image == targetImage {
			allowed = true
		}
	}
	if !allowed {
		return Plan{}, fail
	}
	channels := []string{}
	observedChannels := map[string]ChannelState{}
	for _, channel := range observation.Channels {
		if !componentID.MatchString(channel.ID) || observedChannels[channel.ID].ID != "" || !digest.MatchString(channel.LedgerSHA256) {
			return Plan{}, fail
		}
		observedChannels[channel.ID] = channel
	}
	if len(observedChannels) != len(spec.Spec.Channels) {
		return Plan{}, fail
	}
	channelIDs := map[string]bool{}
	for _, channel := range spec.Spec.Channels {
		if channelIDs[channel.ID] || observedChannels[channel.ID].ID == "" {
			return Plan{}, fail
		}
		channelIDs[channel.ID] = true
		voters := map[string]bool{}
		remaining := 0
		for _, id := range channel.OrdererRefs {
			if voters[id] || expected[id] != "orderer" {
				return Plan{}, fail
			}
			voters[id] = true
		}
		live := map[string]bool{}
		for _, id := range observedChannels[channel.ID].ActiveVoters {
			if !voters[id] || live[id] {
				return Plan{}, fail
			}
			live[id] = true
			if id != targetID {
				remaining++
			}
		}
		if len(voters) == 0 || len(live) != len(voters) || remaining < len(voters)/2+1 {
			return Plan{}, fail
		}
		affected := voters[targetID]
		for _, id := range channel.PeerRefs {
			if expected[id] != "peer" {
				return Plan{}, fail
			}
			affected = affected || id == targetID
		}
		if affected {
			channels = append(channels, channel.ID)
		}
	}
	if len(spec.Spec.Channels) == 0 || (component.Kind != "ca" && len(channels) == 0) {
		return Plan{}, fail
	}
	sort.Strings(channels)
	inventory := make([][4]string, 0, len(seen))
	for id, c := range seen {
		inventory = append(inventory, [4]string{id, c.UID, c.Image, c.StorageIdentitySHA256})
	}
	sort.Slice(inventory, func(i, j int) bool { return inventory[i][0] < inventory[j][0] })
	inventoryJSON, _ := json.Marshal(inventory)
	plan := Plan{InventorySHA256: maintenance.HashBytes(inventoryJSON), NamespaceUID: observation.NamespaceUID, Binding: binding, ActorSHA256: maintenance.HashBytes([]byte(p.Subject)), Component: component, TargetImage: targetImage, Channels: channels}
	raw, _ := json.Marshal(plan)
	plan.Digest = maintenance.HashBytes(raw)
	return plan, nil
}

// Driver supplies an authoritative fresh observation and submits one CAS image
// change. Production wiring requires a reconciler that owns the desired image.
type Driver interface {
	// Lock must fence all mutations to this bound network across replicas.
	Lock(context.Context, maintenance.Binding) (release func(), err error)
	Observe(context.Context, maintenance.Binding) (Snapshot, error)
	ChangeImage(context.Context, Plan) error
}

// Submit expects an approval fetched from a trusted server-owned approval record.
// It checks the reviewed digest against a fresh snapshot. An ambiguous write
// must be reconciled by the owner before another approval is issued.
func Submit(ctx context.Context, p identity.Principal, binding maintenance.Binding, spec domain.NetworkSpec, profile Profile, target, image, approval string, expires, now time.Time, driver Driver) error {
	if driver == nil || !p.CanAdminister() || p.ProjectID == "" || p.StorageTenant() != binding.TenantID || !digest.MatchString(approval) || !expires.After(now) || expires.After(now.Add(time.Hour)) {
		return errors.New("current bound upgrade approval required")
	}
	release, err := driver.Lock(ctx, binding)
	if err != nil || release == nil {
		return errors.New("exclusive upgrade mutation fence unavailable")
	}
	defer release()
	observed, err := driver.Observe(ctx, binding)
	if err != nil {
		return errors.New("upgrade observation unavailable")
	}
	fresh, err := Preview(p, binding, spec, observed, target, image, profile, now)
	if err != nil || fresh.Digest != approval {
		return errors.New("upgrade approval is stale or refused")
	}
	if err := driver.ChangeImage(ctx, fresh); err != nil {
		return errors.New("upgrade write outcome unconfirmed; inspect the existing operation before retry")
	}
	return nil
}
