package deployment

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"testing"
)

func TestColdSnapshotExcludesOnlyTransientCheckpoint(t *testing.T) {
	raw := []byte(`{"items":[
		{"kind":"ConfigMap","metadata":{"name":"sathonate-cold-checkpoint"}},
		{"kind":"Secret","metadata":{"name":"sathonate-cold-checkpoint"}},
		{"kind":"ConfigMap","metadata":{"name":"channel-config"}}
	]}`)
	got, err := coldSnapshotResources(raw)
	var snapshot struct{ Items []json.RawMessage }
	if err != nil || json.Unmarshal(got, &snapshot) != nil || len(snapshot.Items) != 2 ||
		!bytes.Contains(snapshot.Items[0], []byte(`"Secret"`)) ||
		!bytes.Contains(snapshot.Items[1], []byte(`"channel-config"`)) {
		t.Fatal("snapshot did not preserve the original application inventory")
	}
}

func coldTestArchive(t *testing.T, headers ...*tar.Header) []byte {
	t.Helper()
	var b bytes.Buffer
	g := gzip.NewWriter(&b)
	w := tar.NewWriter(g)
	for _, h := range headers {
		if w.WriteHeader(h) != nil {
			t.Fatal("test tar header")
		}
		if h.Size > 0 {
			w.Write(bytes.Repeat([]byte("x"), int(h.Size)))
		}
	}
	w.Close()
	g.Close()
	return b.Bytes()
}
func TestColdArchiveRejectsTraversalLinksDuplicatesAndNonVolumeRoots(t *testing.T) {
	root := &tar.Header{Name: "data", Typeflag: tar.TypeDir, Mode: 0700}
	for _, header := range []*tar.Header{{Name: "../escape", Typeflag: tar.TypeReg}, {Name: "/absolute", Typeflag: tar.TypeReg}, {Name: "data/link", Typeflag: tar.TypeSymlink, Linkname: "/etc/shadow"}, {Name: "other", Typeflag: tar.TypeDir}, {Name: "data", Typeflag: tar.TypeDir}, {Name: `data\escape`, Typeflag: tar.TypeReg}} {
		if validateColdArchive(coldTestArchive(t, root, header)) == nil {
			t.Fatal("unsafe archive accepted", header.Name)
		}
	}
	if validateColdArchive(coldTestArchive(t, root, &tar.Header{Name: "data/ledger", Typeflag: tar.TypeReg, Size: 1, Mode: 0600})) != nil {
		t.Fatal("valid bounded archive refused")
	}
	if validateColdArchive([]byte("corrupt")) == nil {
		t.Fatal("corrupt archive accepted")
	}
}
func TestColdPlanBindsTargetIdentityStorageAndTemplates(t *testing.T) {
	p := coldPlan{NamespaceUID: "ns", TargetSHA256: "target", Deployments: []coldDeployment{{Name: "peer", UID: "deployment", TemplateHash: "template", Replicas: 1}}, Volumes: []coldVolume{{Name: "data", UID: "claim"}}}
	before := coldPlanDigest(p)
	p.Volumes[0].UID = "replacement"
	if coldPlanDigest(p) == before {
		t.Fatal("storage replacement not bound")
	}
	p.Volumes[0].UID = "claim"
	p.Deployments[0].TemplateHash = "changed"
	if coldPlanDigest(p) == before {
		t.Fatal("template drift not bound")
	}
	if !coldJSONEqual([]byte(`{"a":1,"b":2}`), []byte(`{"b":2,"a":1}`)) || coldJSONEqual([]byte(`{"a":1}`), []byte(`{"a":2}`)) {
		t.Fatal("snapshot comparison invalid")
	}
}

func TestColdDatabaseGuardRejectsRemoteAndProductionBeforeConnecting(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "::1", "localhost"} {
		if !ColdDatabaseAllowed(host, "fabric_smp_tests") {
			t.Fatal("local disposable DB refused")
		}
	}
	for _, test := range [][2]string{{"database.example.com", "fabric_smp_tests"}, {"127.0.0.1", "production"}, {"10.0.0.1", "fabric_smp_tests"}, {"localhost", "fabric_smp"}} {
		if ColdDatabaseAllowed(test[0], test[1]) {
			t.Fatal("unapproved DB accepted")
		}
	}
}
