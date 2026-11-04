package cloud

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"sathonate.local/fabric-control-plane/internal/domain"
)

func candidate() (domain.NetworkSpec, Target) {
	s := domain.NetworkSpec{APIVersion: domain.APIVersion, Kind: "FabricNetwork", Metadata: domain.Metadata{ID: "lab", Name: "Lab"}, Spec: domain.Topology{Environment: "production", FabricVersion: "2.5.0", Deployment: domain.Deployment{Engine: "bevel", Provider: "aws", Runtime: "kubernetes", TargetRef: "eks"}, Ordering: domain.Ordering{Consensus: "raft", ChannelCapability: "V2_0"}, Organizations: []domain.Organization{{ID: "orga", MSPID: "orgaMSP", IdentityProviderRef: "orga-ca"}, {ID: "orgb", MSPID: "orgbMSP", IdentityProviderRef: "orgb-ca"}}, Nodes: []domain.Node{{ID: "ord1", Role: "orderer", OrganizationRef: "orga", PlacementRef: "a", FailureDomain: "zone-a"}, {ID: "ord2", Role: "orderer", OrganizationRef: "orgb", PlacementRef: "b", FailureDomain: "zone-b"}, {ID: "ord3", Role: "orderer", OrganizationRef: "orga", PlacementRef: "c", FailureDomain: "zone-c"}, {ID: "peer1", Role: "peer", OrganizationRef: "orga", PlacementRef: "a", FailureDomain: "zone-a"}, {ID: "peer2", Role: "peer", OrganizationRef: "orgb", PlacementRef: "b", FailureDomain: "zone-b"}}, Channels: []domain.Channel{{ID: "main", OrganizationRefs: []string{"orga", "orgb"}, OrdererRefs: []string{"ord1", "ord2", "ord3"}, PeerRefs: []string{"peer1", "peer2"}}}}}
	t := Target{ID: "eks", TenantID: "business-a", Name: "Qualification lab", AccountID: "123456789012", Region: "ap-south-1", ClusterName: "test", KubeContext: "context", RunnerNamespace: "runners", StorageClass: "gp3", DNSDomain: "fabric.example.com", GitOpsURL: "https://git.example.com/company/bevel.git", GitOpsSecret: "gitops", RunnerImage: "registry.example.com/runner@sha256:" + strings.Repeat("a", 64), BevelCommit: strings.Repeat("b", 40), FabricVersion: "2.5.0", CertificationRef: "test-only-candidate-not-production", Organizations: map[string]OrganizationBinding{"orga": {Country: "IN", VaultURL: "https://vault-a.example.com", VaultMount: "secret", VaultTokenSecret: "vault-a"}, "orgb": {Country: "IN", VaultURL: "https://vault-b.example.com", VaultMount: "secret", VaultTokenSecret: "vault-b"}}, Placements: map[string]Placement{"a": {Zone: "zone-a", NodeSelector: map[string]string{"pool": "a"}}, "b": {Zone: "zone-b", NodeSelector: map[string]string{"pool": "b"}}, "c": {Zone: "zone-c", NodeSelector: map[string]string{"pool": "c"}}}}
	return s, t
}
func TestCompilerAndStagingManifest(t *testing.T) {
	s, target := candidate()
	bundle, e := Compile(s, target)
	if e != nil {
		t.Fatal(e)
	}
	b, e := json.Marshal(bundle)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(string(b), "ansible.builtin.file") || strings.Contains(string(b), "root-secret-value") {
		t.Fatal("secrets must remain file references")
	}
	manifest, e := Manifests(bundle, target)
	if e != nil {
		t.Fatal(e)
	}
	encoded, _ := json.Marshal(manifest)
	for _, required := range []string{"batch/v1", "sha256:", "runAsNonRoot", "automountServiceAccountToken", "backoffLimit", "gitops", "eks-access", `"suspend":true`} {
		if !strings.Contains(string(encoded), required) {
			t.Fatal("missing job constraint", required)
		}
	}
	s.Spec.Organizations[0].MSPID = "CustomMSP"
	if _, e = Compile(s, target); e == nil {
		t.Fatal("compiler silently renamed an existing MSP")
	}
}

type recordingCommands struct{ calls []string }

func (c *recordingCommands) Run(_ context.Context, binary string, args []string) ([]byte, error) {
	c.calls = append(c.calls, binary+" "+strings.Join(args, " "))
	if binary == "aws" && args[0] == "sts" {
		return []byte(`{"Account":"123456789012"}`), nil
	}
	return []byte(`{"items":[]}`), nil
}
func TestPreflightNeverClaimsProductionAndDoesNotReadSecretData(t *testing.T) {
	s, target := candidate()
	commands := &recordingCommands{}
	report := Preflight(context.Background(), s, target, commands)
	if report.Ready {
		t.Fatal("unqualified environment became production-ready")
	}
	for _, call := range commands.calls {
		if strings.Contains(call, "get secret") && strings.Contains(call, "-o json") {
			t.Fatal("preflight read secret contents")
		}
		if strings.Contains(call, "apply ") || strings.Contains(call, "create ") || strings.Contains(call, "delete ") {
			t.Fatal("preflight mutated infrastructure")
		}
	}
}
func TestTargetsAreTenantScoped(t *testing.T) {
	_, target := candidate()
	registry := Registry{Targets: []Target{target}}
	if _, ok := registry.Find("business-b", target.ID); ok {
		t.Fatal("cross-tenant target access")
	}
	if len(registry.Summaries("business-b")) != 0 {
		t.Fatal("target metadata leaked across tenants")
	}
}
