package topology

import (
	"testing"

	"sathonate.local/fabric-control-plane/internal/domain"
)

func TestCustomerStorageSelection(t *testing.T) {
	fast := domain.Storage{Class: "fast", CapacityGiB: 30}
	audit := domain.Storage{Class: "audit", CapacityGiB: 7}
	spec := domain.NetworkSpec{Spec: domain.Topology{
		Nodes: []domain.Node{{ID: "peer-west", Role: "peer"}, {ID: "orderer-east", Role: "orderer"}},
		Configuration: &domain.Configuration{
			Nodes:       map[string]domain.NodeSettings{"peer-west": {Storage: fast}, "orderer-east": {Storage: fast}},
			Authorities: []domain.Authority{{ID: "org-ca", Storage: audit}},
		},
	}}
	inventory := map[string]StorageCapability{
		"fast":  {Provisioner: "customer.test/fast", Encrypted: true, WaitForFirstConsumer: true, MaxCapacityGiB: 60},
		"audit": {Provisioner: "customer.test/audit", Encrypted: true, WaitForFirstConsumer: true, MaxCapacityGiB: 10},
	}
	plan, err := CheckCustomerStorage(spec, inventory)
	if err != nil || len(plan) != 3 || plan[1] != (StorageRequirement{ComponentID: "org-ca", Class: "audit", CapacityGiB: 7}) {
		t.Fatalf("unexpected storage plan %+v %v", plan, err)
	}
	for _, unsafe := range []StorageCapability{
		{Provisioner: "customer.test/audit", WaitForFirstConsumer: true, MaxCapacityGiB: 10},
		{Provisioner: "customer.test/audit", Encrypted: true, MaxCapacityGiB: 10},
		{Provisioner: "customer.test/audit", Encrypted: true, WaitForFirstConsumer: true, MaxCapacityGiB: 6},
	} {
		inventory["audit"] = unsafe
		if _, err := CheckCustomerStorage(spec, inventory); err == nil {
			t.Fatal("unsafe storage accepted")
		}
	}
	delete(inventory, "audit")
	if _, err := CheckCustomerStorage(spec, inventory); err == nil {
		t.Fatal("unknown storage class accepted")
	}
}
