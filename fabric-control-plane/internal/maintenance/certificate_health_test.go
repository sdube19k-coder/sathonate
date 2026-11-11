package maintenance

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"testing"
	"time"
)

func TestServerCertificateHealth(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	rootPublic, rootKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	root := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: now.Add(-time.Hour),
		NotAfter: now.Add(48 * time.Hour), IsCA: true, BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign}
	issue := func(c, parent *x509.Certificate, public ed25519.PublicKey, signer ed25519.PrivateKey) []byte {
		t.Helper()
		der, err := x509.CreateCertificate(rand.Reader, c, parent, public, signer)
		if err != nil {
			t.Fatal(err)
		}
		return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	}
	roots := issue(root, root, rootPublic, rootKey)
	parsedRoot, err := publicCertificates(roots)
	if err != nil {
		t.Fatal(err)
	}
	leafPublic, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), NotBefore: now.Add(-time.Hour),
		NotAfter: now.Add(72 * time.Hour), DNSNames: []string{"peer.test"},
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	chain := issue(leaf, parsedRoot[0], leafPublic, rootKey)
	health, err := VerifyServerCertificate(chain, roots, "peer.test", now, 49*time.Hour)
	if err != nil || health.Status != "renewal-due" || !health.ExpiresAt.Equal(root.NotAfter) || len(health.SHA256) != 64 {
		t.Fatalf("unexpected health %+v %v", health, err)
	}
	health, err = VerifyServerCertificate(chain, roots, "peer.test", now, time.Hour)
	if err != nil || health.Status != "healthy" {
		t.Fatalf("unexpected health %+v %v", health, err)
	}
	private := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("fixture")})
	otherPublic, otherKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	foreignRoots := issue(root, root, otherPublic, otherKey)
	leaf.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	client := issue(leaf, parsedRoot[0], leafPublic, rootKey)
	for _, tc := range []struct {
		chain, roots []byte
		endpoint     string
		at           time.Time
	}{
		{chain, roots, "wrong.test", now}, {chain, roots, "peer.test", now.Add(49 * time.Hour)},
		{chain, foreignRoots, "peer.test", now}, {append(private, chain...), roots, "peer.test", now},
		{client, roots, "peer.test", now}, {chain, nil, "peer.test", now},
	} {
		if _, err := VerifyServerCertificate(tc.chain, tc.roots, tc.endpoint, tc.at, time.Hour); err == nil {
			t.Fatal("untrusted certificate accepted")
		}
	}
}
