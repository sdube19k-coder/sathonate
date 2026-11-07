package kubernetesfabric

import (
	"fmt"
	"sathonate.local/fabric-control-plane/internal/domain"
	"testing"
)

// Customer names/MSPs/channels stay unchanged. Only approved placement inputs
// vary; the same generic resolver must handle every organization and authority.
func TestDynamicCustomerDesignsResolveAcrossThreeWorkers(t *testing.T) {
	for _, label := range []string{"a", "b", "c"} {
		t.Run(label, func(t *testing.T) {
			s, r := dynamicFixture(t, label)
			r.NodeSelectors = map[string]map[string]string{}
			s.Spec.Configuration.Placements = map[string]domain.Placement{}
			for i := 0; i < 3; i++ {
				ref := fmt.Sprintf("customer-location-%d", i)
				selector := map[string]string{"kubernetes.io/hostname": fmt.Sprintf("owned-worker-%d", i)}
				r.NodeSelectors[ref] = selector
				s.Spec.Configuration.Placements[ref] = domain.Placement{NodeSelector: selector}
			}
			counts := map[string]int{}
			for i := range s.Spec.Nodes {
				n := &s.Spec.Nodes[i]
				n.PlacementRef = fmt.Sprintf("customer-location-%d", counts[n.Role]%3)
				counts[n.Role]++
			}
			for i := range s.Spec.Configuration.Authorities {
				s.Spec.Configuration.Authorities[i].PlacementRef = fmt.Sprintf("customer-location-%d", i%3)
			}
			if err := ValidateDynamic(s, r); err != nil {
				t.Fatal(err)
			}
			resolved, err := DynamicRuntime(s, r)
			if err != nil {
				t.Fatal(err)
			}
			for _, n := range s.Spec.Nodes {
				want := s.Spec.Configuration.Placements[n.PlacementRef].NodeSelector["kubernetes.io/hostname"]
				if resolved.NodeSelectors[n.ID]["kubernetes.io/hostname"] != want {
					t.Fatal("node placement lost", n.ID)
				}
			}
			for _, a := range s.Spec.Configuration.Authorities {
				want := s.Spec.Configuration.Placements[a.PlacementRef].NodeSelector["kubernetes.io/hostname"]
				if resolved.NodeSelectors[a.ID]["kubernetes.io/hostname"] != want {
					t.Fatal("authority placement lost", a.ID)
				}
			}
			s.Spec.Configuration.Placements[s.Spec.Nodes[0].PlacementRef] = domain.Placement{NodeSelector: map[string]string{"kubernetes.io/hostname": "retained-foreign-node"}}
			if ValidateDynamic(s, r) == nil {
				t.Fatal("customer escaped trusted worker inventory")
			}
		})
	}
}
