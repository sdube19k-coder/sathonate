package topology

import (
	"errors"
	"sort"

	"sathonate.local/fabric-control-plane/internal/domain"
)

// StorageCapability is a customer-administered inventory entry for a bound
// Kubernetes target. Encryption must be established by the target operator.
type StorageCapability struct {
	Provisioner          string
	Encrypted            bool
	WaitForFirstConsumer bool
	MaxCapacityGiB       int
}

type StorageRequirement struct {
	ComponentID string
	Class       string
	CapacityGiB int
}

// CheckCustomerStorage maps every stateful component to an approved class.
// It performs no provisioning and makes no claim about cross-host mobility.
func CheckCustomerStorage(spec domain.NetworkSpec, inventory map[string]StorageCapability) ([]StorageRequirement, error) {
	invalid := errors.New("stateful component requires unique identity and a capacity-compatible encrypted customer storage class")
	configuration := spec.Spec.Configuration
	if configuration == nil || len(spec.Spec.Nodes) == 0 || len(configuration.Authorities) == 0 {
		return nil, invalid
	}
	plan := make([]StorageRequirement, 0, len(spec.Spec.Nodes)+len(configuration.Authorities))
	seen := map[string]bool{}
	add := func(component string, storage domain.Storage) bool {
		class, approved := inventory[storage.Class]
		if !id.MatchString(component) || seen[component] || !id.MatchString(storage.Class) ||
			!approved || class.Provisioner == "" || !class.Encrypted || !class.WaitForFirstConsumer ||
			storage.CapacityGiB <= 0 || storage.CapacityGiB > class.MaxCapacityGiB {
			return false
		}
		seen[component] = true
		plan = append(plan, StorageRequirement{ComponentID: component, Class: storage.Class, CapacityGiB: storage.CapacityGiB})
		return true
	}
	for _, node := range spec.Spec.Nodes {
		settings, exists := configuration.Nodes[node.ID]
		if !exists || (node.Role != "peer" && node.Role != "orderer") || !add(node.ID, settings.Storage) {
			return nil, invalid
		}
	}
	if len(configuration.Nodes) != len(spec.Spec.Nodes) {
		return nil, invalid
	}
	for _, authority := range configuration.Authorities {
		if !add(authority.ID, authority.Storage) {
			return nil, invalid
		}
	}
	sort.Slice(plan, func(i, j int) bool { return plan[i].ComponentID < plan[j].ComponentID })
	return plan, nil
}
