package kubernetesfabric

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"sathonate.local/fabric-control-plane/internal/domain"
)

func dynamicFixture(t *testing.T, label string) (domain.NetworkSpec, Runtime) {
	t.Helper()
	raw, e := os.ReadFile("../../examples/dynamic/" + label + ".json")
	if e != nil {
		t.Fatal(e)
	}
	s, e := domain.Decode(strings.NewReader(string(raw)))
	if e != nil {
		t.Fatal(e)
	}
	r := Runtime{DynamicEnabled: true, StorageClass: "standard", Capacity: "1Gi", SmokeImage: "example/worker@sha256:" + strings.Repeat("c", 64), BrokerChannels: true, Images: map[string]string{}, NodeSelectors: map[string]map[string]string{"inventory-1": {"kubernetes.io/hostname": "disposable-node"}}, CodeArtifacts: map[string]CodeArtifact{s.Spec.Configuration.Chaincodes[0].RuntimeImage: {SHA256: strings.Repeat("a", 64), CodeVersion: "1.0"}}}
	for _, kind := range []string{"FabricCA", "FabricPeer", "FabricOrdererNode"} {
		r.Images[kind] = "example/fabric:qualified@sha256:" + strings.Repeat("d", 64)
	}
	return s, r
}

func cloneDynamic(t *testing.T, s domain.NetworkSpec) domain.NetworkSpec {
	t.Helper()
	raw, _ := json.Marshal(s)
	var copy domain.NetworkSpec
	if json.Unmarshal(raw, &copy) != nil {
		t.Fatal("clone")
	}
	return copy
}

func TestDynamicIndependentCustomerDesignsAndTrustedBinding(t *testing.T) {
	for _, label := range []string{"a", "b", "c"} {
		t.Run(label, func(t *testing.T) {
			s, r := dynamicFixture(t, label)
			if e := r.Validate(s); e != nil {
				t.Fatal(e)
			}
			resolved, e := DynamicRuntime(s, r)
			if e != nil {
				t.Fatal(e)
			}
			if len(r.NodeSelectors) != 1 || len(resolved.NodeSelectors) != 1+len(s.Spec.Nodes)+len(s.Spec.Configuration.Authorities) {
				t.Fatal("trusted inventory mutated or configured members missing")
			}
			n := s.Spec.Nodes[0]
			resolved.NodeSelectors[n.ID]["kubernetes.io/hostname"] = "mutated"
			if r.NodeSelectors["inventory-1"]["kubernetes.io/hostname"] != "disposable-node" || s.Spec.Configuration.Placements[n.PlacementRef].NodeSelector["kubernetes.io/hostname"] != "disposable-node" {
				t.Fatal("resolved runtime shares mutable selector maps")
			}
			w := Worker{Spec: s}
			for _, o := range s.Spec.Organizations {
				if w.caName(w.authority(o.ID, "ca")) != o.IdentityProviderRef || w.peerMSP(o.ID) != o.MSPID || w.firstPeer(o.ID) == "" {
					t.Fatal("hardcoded identity/member resolution")
				}
			}
		})
	}
}

func TestDynamicFailClosedBeforeExternalExecution(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*domain.NetworkSpec, *Runtime)
	}{
		{"disabled", func(s *domain.NetworkSpec, r *Runtime) { r.DynamicEnabled = false }},
		{"channel quorum too small", func(s *domain.NetworkSpec, r *Runtime) {
			s.Spec.Channels[0].OrdererRefs = s.Spec.Channels[0].OrdererRefs[:1]
		}},
		{"channel quorum even", func(s *domain.NetworkSpec, r *Runtime) {
			s.Spec.Channels[0].OrdererRefs = s.Spec.Channels[0].OrdererRefs[:2]
		}},
		{"production", func(s *domain.NetworkSpec, r *Runtime) { s.Spec.Environment = "production" }},
		{"aws", func(s *domain.NetworkSpec, r *Runtime) { s.Spec.Deployment.Provider = "aws" }},
		{"foreign placement", func(s *domain.NetworkSpec, r *Runtime) {
			s.Spec.Configuration.Placements["approved-local"] = domain.Placement{NodeSelector: map[string]string{"kubernetes.io/hostname": "retained"}}
		}},
		{"unapproved artifact", func(s *domain.NetworkSpec, r *Runtime) { r.CodeArtifacts = nil }},
		{"artifact mismatch", func(s *domain.NetworkSpec, r *Runtime) {
			s.Spec.Configuration.Chaincodes[0].PackageSHA256 = strings.Repeat("e", 64)
		}},
		{"foreign authority", func(s *domain.NetworkSpec, r *Runtime) {
			s.Spec.Organizations[0].IdentityProviderRef = s.Spec.Organizations[1].IdentityProviderRef
		}},
		{"unknown anchors", func(s *domain.NetworkSpec, r *Runtime) {
			s.Spec.Configuration.Anchors[s.Spec.Channels[0].ID] = []string{"foreign"}
		}},
		{"oversized quota", func(s *domain.NetworkSpec, r *Runtime) {
			r.Capacity = "100Gi"
			for id, n := range s.Spec.Configuration.Nodes {
				n.Storage.CapacityGiB = 100
				s.Spec.Configuration.Nodes[id] = n
			}
		}},
		{"malformed denylist", func(s *domain.NetworkSpec, r *Runtime) { r.DeniedPublicKeySHA256 = []string{"invalid"} }},
		{"missing configuration", func(s *domain.NetworkSpec, r *Runtime) { s.Spec.Configuration = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, r := dynamicFixture(t, "a")
			tc.mutate(&s, &r)
			if r.Validate(s) == nil {
				t.Fatal("unsafe execution accepted")
			}
			w := Worker{Spec: s, Runtime: r, Secure: true, Namespace: "smp-s-" + strings.Repeat("1", 32)}
			if w.Run(context.Background()) == nil {
				t.Fatal("unsafe worker executed")
			}
		})
	}
	s, r := dynamicFixture(t, "a")
	if ValidateProfile(s) == nil {
		t.Fatal("legacy profile fallback")
	}
	w := Worker{Spec: s, Runtime: r, Secure: false}
	if w.Run(context.Background()) == nil {
		t.Fatal("unprotected dynamic worker accepted")
	}
}

func TestDynamicTransitionRejectsReplacementAndSkippedCode(t *testing.T) {
	s, _ := dynamicFixture(t, "a")
	if e := ValidateDynamicTransition(s, cloneDynamic(t, s)); e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*domain.NetworkSpec)
	}{
		{"node removal", func(s *domain.NetworkSpec) { s.Spec.Nodes = s.Spec.Nodes[1:] }},
		{"identity substitution", func(s *domain.NetworkSpec) { s.Spec.Organizations[0].MSPID = "ForeignMSP" }},
		{"placement replacement", func(s *domain.NetworkSpec) {
			s.Spec.Configuration.Placements["approved-local"] = domain.Placement{NodeSelector: map[string]string{"host": "other"}}
		}},
		{"storage replacement", func(s *domain.NetworkSpec) {
			n := s.Spec.Configuration.Nodes[s.Spec.Nodes[0].ID]
			n.Storage.CapacityGiB++
			s.Spec.Configuration.Nodes[s.Spec.Nodes[0].ID] = n
		}},
		{"channel removal", func(s *domain.NetworkSpec) { s.Spec.Channels = nil }},
		{"consenter removal", func(s *domain.NetworkSpec) { s.Spec.Channels[0].OrdererRefs = s.Spec.Channels[0].OrdererRefs[1:] }},
		{"runtime replacement", func(s *domain.NetworkSpec) { s.Spec.FabricVersion = "3.1.5" }},
		{"code sequence skip", func(s *domain.NetworkSpec) { s.Spec.Configuration.Chaincodes[0].Sequence = 3 }},
		{"silent code change", func(s *domain.NetworkSpec) {
			s.Spec.Configuration.Chaincodes[0].RuntimeImage = "example/other@sha256:" + strings.Repeat("e", 64)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			next := cloneDynamic(t, s)
			tc.mutate(&next)
			if ValidateDynamicTransition(s, next) == nil {
				t.Fatal("unsafe transition accepted")
			}
		})
	}
	next := cloneDynamic(t, s)
	next.Spec.Configuration.Chaincodes[0].Sequence = 2
	next.Spec.Configuration.Chaincodes[0].Version = "2.0"
	next.Spec.Configuration.Chaincodes[0].RuntimeImage = "example/next@sha256:" + strings.Repeat("e", 64)
	if e := ValidateDynamicTransition(s, next); e != nil {
		t.Fatal("single code upgrade rejected", e)
	}
}

func TestDynamicSplitAuthoritiesAndPerMemberResources(t *testing.T) {
	s, r := dynamicFixture(t, "a")
	a := s.Spec.Configuration.Authorities[0]
	s.Spec.Configuration.Authorities[0].Roles = []string{"signing"}
	a.ID = "independent-tls"
	a.Roles = []string{"tls"}
	s.Spec.Configuration.Authorities = append(s.Spec.Configuration.Authorities, a)
	if e := r.Validate(s); e != nil {
		t.Fatal(e)
	}
	w := Worker{Spec: s, Runtime: r}
	if w.authority(a.OrganizationRef, "tlsca") != a.ID || w.authority(a.OrganizationRef, "ca") == a.ID {
		t.Fatal("split authorities conflated")
	}
	n := s.Spec.Nodes[0]
	v := Object{"kind": "FabricPeer", "metadata": Object{"name": n.ID}, "spec": Object{"storage": Object{"peer": Object{}, "chaincode": Object{}}}}
	if e := w.adapt(v); e != nil {
		t.Fatal(e)
	}
	spec := object(v["spec"])
	if text(object(object(spec["storage"])["peer"])["size"]) != "1Gi" || text(object(object(object(spec["resources"])["peer"])["requests"])["cpu"]) != "100m" || object(spec["resources"])["requests"] != nil {
		t.Fatal("configured resources not rendered")
	}
}

func TestVersionedServiceIssuerSurvivesMemberOrderingChanges(t *testing.T) {
	a, b := authorizedFixture(t), authorizedFixture(t)
	roots := map[string]string{"original": a["root.pem"], "another": b["root.pem"]}
	id, e := dynamicServiceIssuer(a["root.pem"], roots)
	if e != nil || id != "original" {
		t.Fatal("original issuer lost", e)
	}
	roots["aaa-new-member"] = authorizedFixture(t)["root.pem"]
	id, e = dynamicServiceIssuer(a["root.pem"], roots)
	if e != nil || id != "original" {
		t.Fatal("new member replaced immutable service issuer", e)
	}
	if _, e = dynamicServiceIssuer(authorizedFixture(t)["root.pem"], roots); e == nil {
		t.Fatal("foreign service root adopted")
	}
	roots["ambiguous"] = a["root.pem"]
	if _, e = dynamicServiceIssuer(a["root.pem"], roots); e == nil {
		t.Fatal("ambiguous issuer adopted")
	}
}
