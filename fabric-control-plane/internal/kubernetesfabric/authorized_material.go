package kubernetesfabric

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"sathonate.local/fabric-control-plane/internal/certificateprototype"
	"time"
)

// The deployment CA public status is the trust anchor. A bundle cannot introduce
// its own unrelated root, even if its certificate and private key agree.
func validateAuthorizedMaterial(namespace, key, dns, commonName, role, expectedRoot string, data map[string]string, denied, deniedKeys []string) error {
	if _, err := certificateprototype.Build(namespace, key, certificateprototype.Material{Certificate: []byte(data["cert.pem"]), PrivateKey: []byte(data["key.pem"]), Root: []byte(data["root.pem"])}, time.Now()); err != nil {
		return secureDenied
	}
	root, rest := pem.Decode([]byte(expectedRoot))
	if root == nil || root.Type != "CERTIFICATE" || len(bytes.TrimSpace(rest)) != 0 {
		return secureDenied
	}
	supplied, rest := pem.Decode([]byte(data["root.pem"]))
	if supplied == nil || len(bytes.TrimSpace(rest)) != 0 || !bytes.Equal(root.Bytes, supplied.Bytes) {
		return secureDenied
	}
	ca, err := x509.ParseCertificate(root.Bytes)
	if err != nil || !ca.IsCA || ca.KeyUsage&x509.KeyUsageCertSign == 0 {
		return secureDenied
	}
	pair, err := tls.X509KeyPair([]byte(data["cert.pem"]), []byte(data["key.pem"]))
	if err != nil {
		return secureDenied
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil || leaf.IsCA || leaf.Subject.CommonName != commonName || leaf.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
		return secureDenied
	}
	allowedRole := false
	roleCount := 0
	for _, ou := range leaf.Subject.OrganizationalUnit {
		if ou == "peer" || ou == "orderer" || ou == "admin" || ou == "client" {
			roleCount++
		}
		if ou == role {
			allowedRole = true
		}
	}
	if !allowedRole || roleCount != 1 {
		return secureDenied
	}
	for _, cert := range []*x509.Certificate{leaf, ca} {
		keySum := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
		for _, fingerprint := range deniedKeys {
			if hex.EncodeToString(keySum[:]) == fingerprint {
				return secureDenied
			}
		}
		sum := sha256.Sum256(cert.Raw)
		for _, fingerprint := range denied {
			if hex.EncodeToString(sum[:]) == fingerprint {
				return secureDenied
			}
		}
	}
	if dns != "" {
		roots := x509.NewCertPool()
		roots.AddCert(ca)
		if _, err := leaf.Verify(x509.VerifyOptions{Roots: roots, DNSName: dns, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}); err != nil {
			return secureDenied
		}
	}
	return nil
}
