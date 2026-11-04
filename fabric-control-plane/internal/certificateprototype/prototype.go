// Package certificateprototype constructs isolated pre-enrolled certificate
// objects using fields present in the pinned Bevel API. It is not a deployment
// adapter and never applies resources or changes existing identities.
package certificateprototype

import (
	"crypto/ecdsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"regexp"
	"time"
)

type Material struct{ Certificate, PrivateKey, Root []byte }
type Objects struct {
	Secret   map[string]any `json:"-"`
	External map[string]any `json:"external"`
}

// Default formatting/export is redacted. Secret is available only for explicit
// protected Kubernetes delivery, never ordinary evidence serialization.
func (Objects) String() string     { return "pre-enrolled certificate objects (private Secret withheld)" }
func (o Objects) GoString() string { return o.String() }

func Build(namespace, name string, m Material, now time.Time) (Objects, error) {
	denied := errors.New("pre-enrolled certificate material, trust or namespace validation failed")
	valid := regexp.MustCompile(`^[a-z]([a-z0-9-]{0,61}[a-z0-9])?$`)
	if !valid.MatchString(namespace) || !valid.MatchString(name) {
		return Objects{}, denied
	}
	pair, e := tls.X509KeyPair(m.Certificate, m.PrivateKey)
	if e != nil || len(pair.Certificate) != 1 {
		return Objects{}, denied
	}
	// Pinned controllers parse ECDSA private keys, not arbitrary TLS key types.
	if _, ok := pair.PrivateKey.(*ecdsa.PrivateKey); !ok {
		return Objects{}, denied
	}
	cert, e := x509.ParseCertificate(pair.Certificate[0])
	if e != nil {
		return Objects{}, denied
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(m.Root) {
		return Objects{}, denied
	}
	if _, e = cert.Verify(x509.VerifyOptions{Roots: roots, CurrentTime: now, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny}}); e != nil {
		return Objects{}, denied
	}
	block, _ := pem.Decode(m.Root)
	if block == nil || block.Type != "CERTIFICATE" {
		return Objects{}, denied
	}
	return Objects{Secret: map[string]any{"apiVersion": "v1", "kind": "Secret", "metadata": map[string]any{"name": name, "namespace": namespace}, "type": "Opaque", "immutable": true, "stringData": map[string]string{"cert.pem": string(m.Certificate), "key.pem": string(m.PrivateKey), "root.pem": string(m.Root)}}, External: map[string]any{"secretName": name, "secretNamespace": namespace, "rootCertificateKey": "root.pem", "certificateKey": "cert.pem", "privateKeyKey": "key.pem"}}, nil
}
