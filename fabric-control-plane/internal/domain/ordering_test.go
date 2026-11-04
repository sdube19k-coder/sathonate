package domain

import "testing"

func bftExample(t *testing.T) NetworkSpec {
	s := example(t)
	s.Spec.FabricVersion = "3.1.5"
	s.Spec.Ordering = Ordering{Consensus: "bft", ChannelCapability: "V3_0"}
	s.Spec.Nodes = append(s.Spec.Nodes, Node{ID: "orderer-four", OrganizationRef: s.Spec.Organizations[0].ID, Role: "orderer", PlacementRef: "host-d", FailureDomain: "rack-d"})
	s.Spec.Channels[0].OrdererRefs = append(s.Spec.Channels[0].OrdererRefs, "orderer-four")
	return s
}

func TestConsensusCompatibility(t *testing.T) {
	if issues := Validate(bftExample(t)); HasErrors(issues) {
		t.Fatalf("valid native BFT design rejected: %+v", issues)
	}
	cases := []struct {
		name   string
		change func(*NetworkSpec)
	}{
		{"BFT on Fabric 2.5", func(s *NetworkSpec) { s.Spec.FabricVersion = "2.5.16" }},
		{"BFT without channel capability", func(s *NetworkSpec) { s.Spec.Ordering.ChannelCapability = "V2_0" }},
		{"BFT using this Bevel fork", func(s *NetworkSpec) {
			s.Spec.Deployment = Deployment{Engine: "bevel", Runtime: "kubernetes", Provider: "aws", TargetRef: "cloud"}
		}},
		{"too few BFT consenters", func(s *NetworkSpec) { s.Spec.Channels[0].OrdererRefs = s.Spec.Channels[0].OrdererRefs[:3] }},
		{"concentrated BFT domain", func(s *NetworkSpec) { s.Spec.Nodes[len(s.Spec.Nodes)-1].FailureDomain = s.Spec.Nodes[0].FailureDomain }},
		{"retired Solo", func(s *NetworkSpec) { s.Spec.Ordering.Consensus = "solo" }},
		{"unknown consensus", func(s *NetworkSpec) { s.Spec.Ordering.Consensus = "kafka" }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := bftExample(t)
			c.change(&s)
			if !HasErrors(Validate(s)) {
				t.Fatal("incompatible ordering design accepted")
			}
		})
	}
}

func TestLegacyRaftAndNativeV3(t *testing.T) {
	legacy := example(t)
	legacy.Spec.Ordering = Ordering{}
	if s := WithDefaults(legacy); s.Spec.Ordering.Consensus != "raft" || s.Spec.Ordering.ChannelCapability != "V2_0" {
		t.Fatal("legacy ordering defaults changed")
	}
	s := example(t)
	s.Spec.FabricVersion = "3.1.5"
	s.Spec.Ordering = Ordering{Consensus: "raft", ChannelCapability: "V3_0"}
	if issues := Validate(s); HasErrors(issues) {
		t.Fatalf("native Raft 3.x rejected: %+v", issues)
	}
	s.Spec.Deployment = Deployment{Engine: "bevel", Runtime: "kubernetes", Provider: "aws", TargetRef: "cloud"}
	if !HasErrors(Validate(s)) {
		t.Fatal("Fabric 3.x silently accepted by legacy Bevel integration")
	}
}
