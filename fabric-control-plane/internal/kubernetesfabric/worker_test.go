package kubernetesfabric

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"os"
	"sathonate.local/fabric-control-plane/internal/domain"
	"strings"
	"testing"
)

func TestWorkerRejectsUnsupportedTopologyBeforeExecution(t *testing.T) {
	raw, err := os.Open("../../examples/local-kubernetes-network.json")
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	spec, err := domain.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	spec.Spec.FabricVersion = "3.0.0"
	w := Worker{Spec: spec, Output: func(Event) error { t.Fatal("unsupported input emitted provisioning progress"); return nil }}
	if err = w.Run(context.Background()); err == nil {
		t.Fatal("unsupported profile accepted")
	}
}
func TestEnrollmentSecretsAndPrivateKeysAreRedacted(t *testing.T) {
	secret := strings.Repeat("deadbeef", 8)
	w := Worker{secrets: []string{secret}}
	result := w.redacted("enrollment failed: " + secret + "\n-----BEGIN PRIVATE KEY-----\nprivate-material\n-----END PRIVATE KEY-----")
	if strings.Contains(result, secret) || strings.Contains(result, "private-material") {
		t.Fatal("worker diagnostic leaked secret")
	}
}
func TestChaincodePackageIsDeterministicAndPrivate(t *testing.T) {
	files := Object{"metadata.json": []byte(`{"type":"ccaas","label":"smp_smoke_1"}`), "code.tar.gz": []byte("code")}
	first, err := tarGzip(files)
	if err != nil {
		t.Fatal(err)
	}
	second, err := tarGzip(files)
	if err != nil || !bytes.Equal(first, second) {
		t.Fatal("package digest changes across retries")
	}
	reader, err := gzip.NewReader(bytes.NewReader(first))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	archive := tar.NewReader(reader)
	count := 0
	for {
		header, err := archive.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if header.Mode != 0600 {
			t.Fatal("package file permissions expose content")
		}
		count++
	}
	if count != 2 {
		t.Fatal("missing package files")
	}
}
