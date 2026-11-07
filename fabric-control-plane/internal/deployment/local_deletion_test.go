package deployment

import (
	"strings"
	"testing"
)

func TestRetainedDeletionInventoryIncludesHistoricalDynamicCodeOnly(t *testing.T) {
	s := dynamicDesign(t)
	s.Spec.Configuration.Chaincodes[0].Sequence = 2
	releases := localDeletionReleases(s)
	if !releases[s.Spec.Nodes[0].ID] || !releases[s.Spec.Configuration.Authorities[0].ID] {
		t.Fatal("configured component omitted")
	}
	q := Request{TenantID: "tenant", NetworkID: "network"}
	var meta resourceMetadata
	meta.Metadata.Name = s.Spec.Nodes[0].ID
	meta.Metadata.Labels = map[string]string{"release": meta.Metadata.Name}
	if !localDeletionWorkloadOwned(q, releases, meta) {
		t.Fatal("owned component rejected")
	}
	meta.Metadata.Name = "foreign"
	meta.Metadata.Labels["release"] = "foreign"
	if localDeletionWorkloadOwned(q, releases, meta) {
		t.Fatal("foreign release accepted")
	}
	meta.Metadata.Name = "secure-controller"
	meta.Metadata.Labels = map[string]string{"fabric.sathonate.io/tenant": "foreign", "fabric.sathonate.io/network": "network"}
	if localDeletionWorkloadOwned(q, releases, meta) {
		t.Fatal("foreign controller accepted")
	}
}
func TestDeletionDigestBindsRetainedUIDsAndExactActions(t *testing.T) {
	p := CleanupPreview{Schema: "sathonate.local-retained-deletion/v1", NamespaceUID: "namespace", TargetSHA256: strings.Repeat("a", 64), Delete: []CleanupRef{{Resource: "deployments", Name: "peer", UID: "uid"}}, RetainedObjects: []RetainedRef{{Resource: "persistentvolumeclaims", Name: "data", UID: "original"}}}
	before := cleanupDigest(p)
	p.RetainedObjects[0].UID = "replacement"
	if before == cleanupDigest(p) {
		t.Fatal("replacement storage approval unchanged")
	}
	p.RetainedObjects[0].UID = "original"
	p.Delete[0].UID = "replacement"
	if before == cleanupDigest(p) {
		t.Fatal("replacement workload approval unchanged")
	}
	if strings.Contains(p.Notice, "erased") {
		t.Fatal("retention confused with erasure")
	}
}
