package kubernetesfabric

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"testing"
	"time"
)

func authorizedFixture(t *testing.T) map[string]string {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	now := time.Now()
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "deployment-ca"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, err := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leafKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "peer-tls", OrganizationalUnit: []string{"peer"}}, NotBefore: ca.NotBefore, NotAfter: ca.NotAfter, DNSNames: []string{"peer.customer"}, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}}
	cert, err := x509.CreateCertificate(rand.Reader, leaf, ca, &leafKey.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	pk, _ := x509.MarshalPKCS8PrivateKey(leafKey)
	return map[string]string{"root.pem": string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})), "cert.pem": string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert})), "key.pem": string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pk}))}
}
func TestDeploymentCAIdentityAuthorization(t *testing.T) {
	data := authorizedFixture(t)
	other := authorizedFixture(t)
	validate := func(d map[string]string, root, cn, role, dns string, deny []string) error {
		return validateAuthorizedMaterial("customer", "peer-tls", dns, cn, role, root, d, deny, nil)
	}
	if validate(data, data["root.pem"], "peer-tls", "peer", "peer.customer", nil) != nil {
		t.Fatal("valid identity rejected")
	}
	for _, tc := range []struct{ name, root, cn, role, dns string }{{"foreign CA", other["root.pem"], "peer-tls", "peer", "peer.customer"}, {"identity", data["root.pem"], "other", "peer", "peer.customer"}, {"role", data["root.pem"], "peer-tls", "admin", "peer.customer"}, {"SAN", data["root.pem"], "peer-tls", "peer", "foreign.customer"}, {"missing root", "", "peer-tls", "peer", "peer.customer"}} {
		t.Run(tc.name, func(t *testing.T) {
			if validate(data, tc.root, tc.cn, tc.role, tc.dns, nil) == nil {
				t.Fatal("unauthorized material accepted")
			}
		})
	}
	for _, field := range []string{"cert.pem", "root.pem", "key.pem"} {
		t.Run("malformed "+field, func(t *testing.T) {
			copy := map[string]string{}
			for k, v := range data {
				copy[k] = v
			}
			copy[field] = "malformed"
			if validate(copy, data["root.pem"], "peer-tls", "peer", "peer.customer", nil) == nil {
				t.Fatal("malformed material accepted")
			}
		})
	}
	for _, field := range []string{"cert.pem", "root.pem"} {
		block, _ := pem.Decode([]byte(data[field]))
		sum := sha256.Sum256(block.Bytes)
		if validate(data, data["root.pem"], "peer-tls", "peer", "peer.customer", []string{hex.EncodeToString(sum[:])}) == nil {
			t.Fatal("exposed public fingerprint reused")
		}
	}
	block, _ := pem.Decode([]byte(data["cert.pem"]))
	leaf, _ := x509.ParseCertificate(block.Bytes)
	sum := sha256.Sum256(leaf.RawSubjectPublicKeyInfo)
	if validateAuthorizedMaterial("customer", "peer-tls", "peer.customer", "peer-tls", "peer", data["root.pem"], data, nil, []string{hex.EncodeToString(sum[:])}) == nil {
		t.Fatal("exposed public key accepted under certificate-independent denylist")
	}
	data["key.pem"] = other["key.pem"]
	if validate(data, data["root.pem"], "peer-tls", "peer", "peer.customer", nil) == nil {
		t.Fatal("mismatched key accepted")
	}
}
