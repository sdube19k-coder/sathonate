package cloud

import (
	"bytes"
	"encoding/json"
	"testing"

	"sathonate.local/fabric-control-plane/internal/domain"
)

func TestPackageBindingAndDigest(t *testing.T) {
	s, target := candidate()
	p, err := Prepare(s, 3, target.TenantID, Registry{Targets: []Target{target}})
	if err != nil || p.Bundle == nil || p.Executable || p.Revision != 3 {
		t.Fatalf("invalid handoff: %+v %v", p, err)
	}
	s.Metadata.Name = "changed"
	changed, err := Prepare(s, 4, target.TenantID, Registry{Targets: []Target{target}})
	if err != nil || changed.Digest == p.Digest {
		t.Fatal("changed JSON retained the old digest")
	}
	other, err := Prepare(s, 4, "another-tenant", Registry{Targets: []Target{target}})
	if err != nil || other.Bundle != nil || len(other.Checks) != 1 {
		t.Fatal("cross-tenant package compiled")
	}
	s.Spec.Deployment.Cloud = &domain.CloudIntent{AccountID: "999999999999", ClusterMode: "existing", VPCMode: "existing", MinNodes: 1, DesiredNodes: 2, MaxNodes: 3}
	mismatch, err := Prepare(s, 4, target.TenantID, Registry{Targets: []Target{target}})
	if err != nil || mismatch.Bundle != nil {
		t.Fatal("UI account overrode the registered account")
	}
}

func TestCloudIntentRejectsSecretsAndInvalidRanges(t *testing.T) {
	s, _ := candidate()
	raw, _ := json.Marshal(s)
	var data map[string]any
	_ = json.Unmarshal(raw, &data)
	deployment := data["spec"].(map[string]any)["deployment"].(map[string]any)
	deployment["cloud"] = map[string]any{"secretAccessKey": "must-not-be-accepted"}
	raw, _ = json.Marshal(data)
	if _, err := domain.Decode(bytes.NewReader(raw)); err == nil {
		t.Fatal("inline cloud credential accepted")
	}
	s.Spec.Deployment.Cloud = &domain.CloudIntent{ClusterMode: "new", VPCMode: "new", MinNodes: 1, DesiredNodes: 2, MaxNodes: 3, VPCCIDR: "10.0.0.0/16", ServiceCIDR: "10.0.1.0/24"}
	if !domain.HasErrors(domain.Validate(s)) {
		t.Fatal("overlapping cloud ranges accepted")
	}
}
