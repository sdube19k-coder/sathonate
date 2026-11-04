package domain

import (
	"os"
	"strings"
	"testing"
)

func example(t *testing.T) NetworkSpec {
	t.Helper()
	f, e := os.Open("../../examples/native-network.json")
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	s, e := Decode(f)
	if e != nil {
		t.Fatal(e)
	}
	return s
}

func TestValidateExamples(t *testing.T) {
	for _, name := range []string{"native-network.json", "cloud-network.json"} {
		t.Run(name, func(t *testing.T) {
			f, e := os.Open("../../examples/" + name)
			if e != nil {
				t.Fatal(e)
			}
			defer f.Close()
			s, e := Decode(f)
			if e != nil {
				t.Fatal(e)
			}
			if issues := Validate(s); HasErrors(issues) {
				t.Fatalf("invalid example: %+v", issues)
			}
		})
	}
}

func TestUnsafeTopologiesRejected(t *testing.T) {
	checks := []struct {
		name   string
		mutate func(*NetworkSpec)
	}{
		{"unknown engine", func(s *NetworkSpec) { s.Spec.Deployment.Engine = "shell" }},
		{"native cannot silently use kubernetes", func(s *NetworkSpec) { s.Spec.Deployment.Runtime = "kubernetes" }},
		{"unsupported major", func(s *NetworkSpec) { s.Spec.FabricVersion = "4.0.0" }},
		{"unknown peer", func(s *NetworkSpec) { s.Spec.Channels[0].PeerRefs[0] = "missing" }},
		{"wrong node role", func(s *NetworkSpec) { s.Spec.Channels[0].PeerRefs[0] = "orderer-carrier" }},
		{"nonmember organization", func(s *NetworkSpec) { s.Spec.Channels[0].OrganizationRefs = s.Spec.Channels[0].OrganizationRefs[:2] }},
		{"two orderers", func(s *NetworkSpec) { s.Spec.Channels[0].OrdererRefs = s.Spec.Channels[0].OrdererRefs[:2] }},
		{"concentrated quorum", func(s *NetworkSpec) { s.Spec.Nodes[1].FailureDomain = s.Spec.Nodes[0].FailureDomain }},
		{"duplicate orderer", func(s *NetworkSpec) { s.Spec.Channels[0].OrdererRefs[1] = s.Spec.Channels[0].OrdererRefs[0] }},
		{"duplicate MSP", func(s *NetworkSpec) { s.Spec.Organizations[1].MSPID = s.Spec.Organizations[0].MSPID }},
		{"duplicate node", func(s *NetworkSpec) { s.Spec.Nodes[1].ID = s.Spec.Nodes[0].ID }},
		{"placement cannot invent failure domains", func(s *NetworkSpec) { s.Spec.Nodes[1].PlacementRef = s.Spec.Nodes[0].PlacementRef }},
		{"empty topology", func(s *NetworkSpec) { s.Spec.Organizations = nil; s.Spec.Nodes = nil; s.Spec.Channels = nil }},
	}
	for _, c := range checks {
		t.Run(c.name, func(t *testing.T) {
			s := example(t)
			c.mutate(&s)
			if !HasErrors(Validate(s)) {
				t.Fatal("unsafe topology accepted")
			}
		})
	}
}

func TestDevelopmentCanPlanWithoutQuorumRedundancy(t *testing.T) {
	s := example(t)
	s.Spec.Environment = "development"
	s.Spec.Channels[0].OrdererRefs = s.Spec.Channels[0].OrdererRefs[:1]
	issues := Validate(s)
	if HasErrors(issues) {
		t.Fatalf("development topology rejected: %+v", issues)
	}
	found := false
	for _, i := range issues {
		if strings.Contains(i.Message, "lose quorum") {
			found = true
		}
	}
	if !found {
		t.Fatal("missing quorum warning")
	}
}

func TestDecodeRejectsUnknownFieldsAndTrailingDocuments(t *testing.T) {
	for _, input := range []string{`{"password":"inline-secret"}`, `{} {}`, `{"spec": {"deployment": {"command":"arbitrary shell"}}}`} {
		if _, e := Decode(strings.NewReader(input)); e == nil {
			t.Fatal("unexpected JSON accepted")
		}
	}
}
