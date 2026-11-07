package deployment

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

// This opt-in test uses a NEW owned namespace and empty PVC only. It performs
// no operation on any Fabric namespace, key, ledger or retained cluster.
func TestLiveColdHelperUID(t *testing.T) {
	if os.Getenv("SMP_COLD_UID_E2E") != "1" {
		t.Skip("requires explicit disposable helper UID qualification")
	}
	r, e := LoadSecureRunner(os.Getenv("SMP_COLD_UID_RUNTIME"))
	if e != nil {
		t.Fatal("disposable config unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	if r.base().validateSecure(ctx) != nil {
		t.Fatal("disposable provider identity unavailable")
	}
	ns := "cold-uid-" + strings.ToLower(time.Now().UTC().Format("150405000000"))
	b := r.base()
	create := func(v any) resourceMetadata {
		t.Helper()
		raw, _ := json.Marshal(v)
		cmd := b.command(ctx, "create", "-f", "-", "-o", "json")
		cmd.Stdin = bytes.NewReader(raw)
		var out bytes.Buffer
		cmd.Stdout = &out
		if cmd.Run() != nil {
			t.Fatal("guarded disposable create failed")
		}
		var meta resourceMetadata
		if json.Unmarshal(out.Bytes(), &meta) != nil || meta.Metadata.UID == "" {
			t.Fatal("create UID acknowledgement unavailable")
		}
		return meta
	}
	namespace := create(map[string]any{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]any{"name": ns, "labels": map[string]string{"pod-security.kubernetes.io/enforce": "baseline", "pod-security.kubernetes.io/enforce-version": "v1.34"}}})
	deleteRaw := func(path, uid string) error {
		options, _ := json.Marshal(map[string]any{"apiVersion": "v1", "kind": "DeleteOptions", "preconditions": map[string]string{"uid": uid}, "propagationPolicy": "Foreground", "gracePeriodSeconds": 1})
		cmd := b.command(ctx, "delete", "--raw", path, "-f", "-")
		cmd.Stdin = bytes.NewReader(options)
		return cmd.Run()
	}
	defer func() {
		if deleteRaw("/api/v1/namespaces/"+ns, namespace.Metadata.UID) != nil {
			t.Error("disposable namespace cleanup refused")
		}
		if _, e := b.run(context.Background(), "wait", "--for=delete", "namespace/"+ns, "--timeout=90s"); e != nil {
			t.Error("disposable namespace removal unverified")
		}
	}()
	claim := create(map[string]any{"apiVersion": "v1", "kind": "PersistentVolumeClaim", "metadata": map[string]any{"name": "empty-data", "namespace": ns}, "spec": map[string]any{"accessModes": []string{"ReadWriteOnce"}, "resources": map[string]any{"requests": map[string]string{"storage": "50Mi"}}}})
	name := "cold-volume-" + coldHashName("empty-data")
	canary := create(map[string]any{"apiVersion": "v1", "kind": "Pod", "metadata": map[string]any{"name": name, "namespace": ns}, "spec": map[string]any{"automountServiceAccountToken": false, "terminationGracePeriodSeconds": 1, "restartPolicy": "Never", "containers": []any{map[string]any{"name": "canary", "image": r.binding.config.WorkerImage, "imagePullPolicy": "Never", "command": []string{"sleep", "300"}, "volumeMounts": []any{map[string]any{"name": "data", "mountPath": "/volume"}}}}, "volumes": []any{map[string]any{"name": "data", "persistentVolumeClaim": map[string]any{"claimName": "empty-data"}}}}})
	if _, e = b.run(ctx, "-n", ns, "wait", "pod/"+name, "--for=condition=Ready", "--timeout=90s"); e != nil {
		t.Fatal("empty disposable volume did not bind")
	}
	if _, e = b.run(ctx, "-n", ns, "exec", name, "--", "python", "-c", "from pathlib import Path;Path('/volume/recovery-test.txt').write_bytes(b'cold-helper-round-trip')"); e != nil {
		t.Fatal("disposable canary write failed")
	}
	plan := coldPlan{Namespace: ns, NamespaceUID: namespace.Metadata.UID}
	volume := coldVolume{Name: "empty-data", UID: claim.Metadata.UID}
	if _, e = r.coldVolume(ctx, plan, volume, nil, "capture"); e == nil {
		t.Fatal("helper adopted a colliding foreign pod")
	}
	if e = b.deleteCleanupRef(ctx, CleanupRef{Resource: "pods", Namespace: ns, Name: name, UID: "foreign-uid"}); e == nil {
		t.Fatal("wrong UID deleted a foreign pod")
	}
	raw, e := b.run(ctx, "-n", ns, "get", "pod", name, "-o", "json")
	var observed resourceMetadata
	if e != nil || json.Unmarshal(raw, &observed) != nil || observed.Metadata.UID != canary.Metadata.UID {
		t.Fatal("foreign pod was altered")
	}
	if deleteRaw("/api/v1/namespaces/"+ns+"/pods/"+name, canary.Metadata.UID) != nil {
		t.Fatal("canary cleanup failed")
	}
	if _, e = b.run(ctx, "-n", ns, "wait", "--for=delete", "pod/"+name, "--timeout=60s"); e != nil {
		t.Fatal("canary removal unverified")
	}
	archive, e := r.coldVolume(ctx, plan, volume, nil, "capture")
	if e != nil || validateColdArchive(archive) != nil {
		t.Fatal("final-source UID-bound helper capture failed")
	}
	fault := create(map[string]any{"apiVersion": "v1", "kind": "Pod", "metadata": map[string]any{"name": "cold-empty-fault", "namespace": ns}, "spec": map[string]any{"automountServiceAccountToken": false, "terminationGracePeriodSeconds": 1, "restartPolicy": "Never", "containers": []any{map[string]any{"name": "fault", "image": r.binding.config.WorkerImage, "imagePullPolicy": "Never", "command": []string{"python", "-c", "import os;os.unlink('/volume/recovery-test.txt')"}, "volumeMounts": []any{map[string]any{"name": "data", "mountPath": "/volume"}}}}, "volumes": []any{map[string]any{"name": "data", "persistentVolumeClaim": map[string]any{"claimName": "empty-data"}}}}})
	if _, e = b.run(ctx, "-n", ns, "wait", "pod/cold-empty-fault", "--for=jsonpath={.status.phase}=Succeeded", "--timeout=90s"); e != nil {
		t.Fatal("disposable file-loss injection failed")
	}
	if deleteRaw("/api/v1/namespaces/"+ns+"/pods/cold-empty-fault", fault.Metadata.UID) != nil {
		t.Fatal("fault pod cleanup refused")
	}
	if _, e = b.run(ctx, "-n", ns, "wait", "--for=delete", "pod/cold-empty-fault", "--timeout=60s"); e != nil {
		t.Fatal("fault pod cleanup unverified")
	}
	if _, e = r.coldVolume(ctx, plan, volume, archive, "restore"); e != nil {
		t.Fatal("final-source helper file restoration failed")
	}
	verified, e := r.coldVolume(ctx, plan, volume, archive, "verify")
	if e != nil || strings.TrimSpace(string(verified)) != "VERIFIED" {
		t.Fatal("final-source restored bytes/ownership verification failed")
	}

	raw, e = b.run(ctx, "-n", ns, "get", "pod", name, "--ignore-not-found", "-o", "name")
	if e != nil || len(bytes.TrimSpace(raw)) != 0 {
		t.Fatal("owned helper was not removed")
	}
	raw, e = b.run(ctx, "-n", ns, "get", "pvc", "empty-data", "-o", "json")
	if e != nil || json.Unmarshal(raw, &observed) != nil || observed.Metadata.UID != claim.Metadata.UID {
		t.Fatal("helper replaced the empty PVC")
	}
}

func coldHashName(name string) string {
	h := sha256.Sum256([]byte(name))
	return hex.EncodeToString(h[:])[:16]
}
