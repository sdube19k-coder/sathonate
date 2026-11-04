package domain

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func secureFixture(t *testing.T) NetworkSpec {
	t.Helper()
	f, err := os.Open("../../examples/secure-local-network.json")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	s, err := Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSecureLocalExactQualifiedTopology(t *testing.T) {
	s := secureFixture(t)
	if HasErrors(Validate(s)) {
		t.Fatal(Validate(s))
	}
	for name, mutate := range map[string]func(*NetworkSpec){
		"aws":           func(s *NetworkSpec) { s.Spec.Deployment.Provider = "aws" },
		"azure":         func(s *NetworkSpec) { s.Spec.Deployment.Provider = "azure" },
		"production":    func(s *NetworkSpec) { s.Spec.Environment = "production" },
		"cloud-options": func(s *NetworkSpec) { s.Spec.Deployment.Cloud = &CloudIntent{} },
		"native": func(s *NetworkSpec) {
			s.Spec.Deployment.Engine = "native"
			s.Spec.Deployment.Runtime = "linux-containers"
			s.Spec.Deployment.Provider = ""
		},
		"bft":                func(s *NetworkSpec) { s.Spec.Ordering.Consensus = "bft" },
		"solo":               func(s *NetworkSpec) { s.Spec.Ordering.Consensus = "solo" },
		"fabric-version":     func(s *NetworkSpec) { s.Spec.FabricVersion = "2.5.14" },
		"capability":         func(s *NetworkSpec) { s.Spec.Ordering.ChannelCapability = "V3_0" },
		"extra-org":          func(s *NetworkSpec) { s.Spec.Organizations = append(s.Spec.Organizations, s.Spec.Organizations[0]) },
		"duplicate-org":      func(s *NetworkSpec) { s.Spec.Organizations[1] = s.Spec.Organizations[0] },
		"foreign-msp":        func(s *NetworkSpec) { s.Spec.Organizations[0].MSPID = "ForeignMSP" },
		"foreign-ca":         func(s *NetworkSpec) { s.Spec.Organizations[0].IdentityProviderRef = "foreign-ca" },
		"extra-peer":         func(s *NetworkSpec) { s.Spec.Nodes = append(s.Spec.Nodes, s.Spec.Nodes[3]) },
		"wrong-role":         func(s *NetworkSpec) { s.Spec.Nodes[0].Role = "peer" },
		"wrong-ordering-org": func(s *NetworkSpec) { s.Spec.Nodes[0].OrganizationRef = "org2" },
		"foreign-placement":  func(s *NetworkSpec) { s.Spec.Nodes[0].PlacementRef = "remote-node" },
		"extra-channel":      func(s *NetworkSpec) { s.Spec.Channels = append(s.Spec.Channels, s.Spec.Channels[0]) },
		"wrong-channel":      func(s *NetworkSpec) { s.Spec.Channels[0].ID = "another" },
		"missing-member":     func(s *NetworkSpec) { s.Spec.Channels[0].PeerRefs = s.Spec.Channels[0].PeerRefs[:1] },
		"duplicate-member":   func(s *NetworkSpec) { s.Spec.Channels[0].PeerRefs[1] = s.Spec.Channels[0].PeerRefs[0] },
	} {
		t.Run(name, func(t *testing.T) {
			s := secureFixture(t)
			mutate(&s)
			if !HasErrors(Validate(s)) {
				t.Fatal("unsupported secure design accepted")
			}
		})
	}
}

func TestSecureLocalInputRejectsUnqualifiedStateDBAndSecrets(t *testing.T) {
	raw, _ := json.Marshal(secureFixture(t))
	for _, field := range []string{`"stateDatabase":"couchdb",`, `"enrollsecret":"never-export-canary",`, `"namespace":"retained",`, `"kubeContext":"retained",`} {
		input := strings.Replace(string(raw), `"engine":"bevel",`, field+`"engine":"bevel",`, 1)
		if _, err := Decode(strings.NewReader(input)); err == nil {
			t.Fatal("untrusted execution field accepted")
		}
	}
}
