package drunix

import (
	"encoding/json"
	"testing"
)

func referenceSpec() NetworkSpec {
	return NetworkSpec{
		APIVersion: SpecVersion, Version: Version, Name: "discovery-net", Consensus: "raft",
		Orderer: Orderer{ID: "orderer", MSPID: "OrdererMSP"},
		Organizations: []Organization{
			{ID: "north", MSPID: "NorthMSP", LitePeer: "lite-north", CommittingPeer: "commit-north",
				ValidationService: "validation-north", StateDatabase: "yugabytedb", TransientStore: "keydb"},
			{ID: "south", MSPID: "SouthMSP", LitePeer: "lite-south", CommittingPeer: "commit-south",
				ValidationService: "validation-south", StateDatabase: "yugabytedb", TransientStore: "keydb"},
		},
	}
}

func TestDecodeReferenceSpec(t *testing.T) {
	raw, err := json.Marshal(referenceSpec())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(raw); err != nil {
		t.Fatal(err)
	}
	for _, input := range [][]byte{
		append(append([]byte{}, raw...), []byte("{}")...),
		[]byte(`{"credentials":"must-not-be-accepted"}`),
		[]byte(`null`),
		make([]byte, 65537),
	} {
		if _, err := Decode(input); err == nil {
			t.Fatal("invalid spec accepted")
		}
	}
}

func TestRejectUnsupportedConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*NetworkSpec)
	}{
		{"schema", func(s *NetworkSpec) { s.APIVersion = "fabric.sathonate.io/v1alpha1" }},
		{"version", func(s *NetworkSpec) { s.Version = "latest" }},
		{"consensus", func(s *NetworkSpec) { s.Consensus = "bft" }},
		{"organization count", func(s *NetworkSpec) { s.Organizations = s.Organizations[:1] }},
		{"duplicate organization", func(s *NetworkSpec) { s.Organizations[1].ID = s.Organizations[0].ID }},
		{"duplicate MSP", func(s *NetworkSpec) { s.Organizations[0].MSPID = s.Orderer.MSPID }},
		{"duplicate node", func(s *NetworkSpec) { s.Organizations[1].LitePeer = s.Organizations[0].CommittingPeer }},
		{"missing validation service", func(s *NetworkSpec) { s.Organizations[0].ValidationService = "" }},
		{"unsupported state", func(s *NetworkSpec) { s.Organizations[0].StateDatabase = "postgres" }},
		{"missing transient store", func(s *NetworkSpec) { s.Organizations[0].TransientStore = "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			spec := referenceSpec()
			tc.edit(&spec)
			if err := spec.Validate(); err == nil {
				t.Fatal("unsupported configuration accepted")
			}
		})
	}
}
