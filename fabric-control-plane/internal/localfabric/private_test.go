package localfabric

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrivateCredentialDirectoryCanBeProtected(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := protectPrivateDirectory(dir); err != nil {
		t.Fatal("cannot enforce private credential directory", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "probe"), []byte("non-secret"), 0600); err != nil {
		t.Fatal("private directory unusable")
	}
}
