package deployment

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestCleanupExactScopeOwnershipAndUID(t *testing.T) {
	request := Request{TenantID: "tenant-a", NetworkID: "record", Network: fixture(t)}
	ns := Namespace(request.TenantID, request.NetworkID)
	refs := cleanupNames(request)
	if len(refs) != 11 {
		t.Fatalf("unexpected supported cleanup scope %d", len(refs))
	}
	for _, ref := range refs {
		if ref.Resource == "secrets" || ref.Resource == "persistentvolumeclaims" || ref.Resource == "namespaces" {
			t.Fatal("data destruction included")
		}
		ref.UID = "uid-one"
		path, e := cleanupPath(ref)
		if e != nil || strings.Contains(path, "--all") {
			t.Fatal("unsafe exact path")
		}
	}
	ref := CleanupRef{Resource: "fabricpeers", Namespace: ns, Name: "peer1", UID: "uid-one"}
	var meta resourceMetadata
	meta.Metadata.Name = ref.Name
	meta.Metadata.Namespace = ns
	meta.Metadata.UID = ref.UID
	meta.Metadata.Labels = map[string]string{"sathonate.io/namespace": ns}
	if cleanupOwnership(ref, meta, request) != nil {
		t.Fatal("valid owner refused")
	}
	meta.Metadata.Labels["sathonate.io/namespace"] = "foreign"
	if cleanupOwnership(ref, meta, request) == nil {
		t.Fatal("foreign owner accepted")
	}
	if _, e := cleanupPath(CleanupRef{Resource: "secrets", Name: "credential", UID: "uid"}); e == nil {
		t.Fatal("secret delete supported")
	}
	if _, e := cleanupPath(CleanupRef{Resource: "jobs", Namespace: ns, Name: "../foreign", UID: "uid"}); e == nil {
		t.Fatal("path traversal allowed")
	}
}
func TestCleanupApprovalTracksReplacementAndIgnoresTimestamp(t *testing.T) {
	p := CleanupPreview{TargetSHA256: strings.Repeat("a", 64), NetworkSHA256: strings.Repeat("b", 64), NamespaceUID: "ns-uid", Delete: []CleanupRef{{Resource: "jobs", Name: "job", UID: "one"}}}
	first := cleanupDigest(p)
	p.ObservedAt = time.Now()
	if cleanupDigest(p) != first {
		t.Fatal("clock invalidated preview")
	}
	p.Delete[0].UID = "two"
	if cleanupDigest(p) == first {
		t.Fatal("replacement object retained approval")
	}
	raw, _ := json.Marshal(p)
	if strings.Contains(string(raw), "password") {
		t.Fatal("unexpected secret field")
	}
}
