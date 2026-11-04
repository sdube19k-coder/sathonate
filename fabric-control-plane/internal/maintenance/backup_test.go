package maintenance

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBackupAuthenticationBindingAndPrivateRestore(t *testing.T) {
	_, binding := fixture()
	key := bytes.Repeat([]byte{7}, 32)
	secret := []byte("private-enrollment-and-wallet-payload")
	encoded, receipt, err := SealBackup(binding, key, secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, secret) {
		t.Fatal("plaintext backup")
	}
	out, _ := json.Marshal(receipt)
	if bytes.Contains(out, secret) {
		t.Fatal("receipt leaked secret")
	}
	restored, _, err := OpenBackup(binding, key, encoded)
	if err != nil || !bytes.Equal(restored, secret) {
		t.Fatal("roundtrip failed")
	}
	bad := binding
	bad.TenantID = "foreign"
	if _, _, err = OpenBackup(bad, key, encoded); err == nil {
		t.Fatal("foreign restore")
	}
	wrongKey := bytes.Repeat([]byte{8}, 32)
	if _, _, err = OpenBackup(binding, wrongKey, encoded); err == nil {
		t.Fatal("wrong key accepted")
	}
	var envelope backupEnvelope
	json.Unmarshal(encoded, &envelope)
	envelope.Ciphertext[0] ^= 1
	tampered, _ := json.Marshal(envelope)
	if _, _, err = OpenBackup(binding, key, tampered); err == nil {
		t.Fatal("tampered backup accepted")
	}
	envelope.Nonce = nil
	tampered, _ = json.Marshal(envelope)
	if _, _, err = OpenBackup(binding, key, tampered); err == nil {
		t.Fatal("bad nonce accepted")
	}
	path := filepath.Join(t.TempDir(), "restore-private")
	if CreatePrivate(path, restored) != nil {
		t.Fatal("cannot create private restore")
	}
	if CreatePrivate(path, []byte("overwrite")) == nil {
		t.Fatal("restore overwrite accepted")
	}
	actual, _ := os.ReadFile(path)
	if !bytes.Equal(actual, secret) {
		t.Fatal("restored bytes changed")
	}
	if _, err = InspectCertificate([]byte("-----BEGIN PRIVATE KEY-----\nsecret\n-----END PRIVATE KEY-----"), time.Now()); err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatal("private key accepted or leaked")
	}
}
