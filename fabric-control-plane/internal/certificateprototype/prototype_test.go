package certificateprototype

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T) Material {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	root := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "prototype-root"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, e := x509.CreateCertificate(rand.Reader, root, root, &key.PublicKey, key)
	if e != nil {
		t.Fatal(e)
	}
	ca, _ := x509.ParseCertificate(der)
	leafKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "prototype-peer"}, NotBefore: root.NotBefore, NotAfter: root.NotAfter, KeyUsage: x509.KeyUsageDigitalSignature}
	cert, e := x509.CreateCertificate(rand.Reader, leaf, ca, &leafKey.PublicKey, key)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := x509.MarshalPKCS8PrivateKey(leafKey)
	return Material{pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: raw}), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})}
}
func TestExternalSecretPrototype(t *testing.T) {
	m := fixture(t)
	o, e := Build("smp-prototype", "peer-cert", m, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	if o.Secret["immutable"] != true {
		t.Fatal("mutable secret")
	}
	export, _ := json.Marshal(o)
	if strings.Contains(string(export), "PRIVATE KEY") || strings.Contains(string(export), "stringData") || strings.Contains(fmt.Sprintf("%+v %#v", o, o), "PRIVATE KEY") {
		t.Fatal("default object formatting exposed private Secret")
	}
	ref, _ := json.Marshal(o.External)
	if strings.Contains(string(ref), "PRIVATE KEY") || strings.Contains(string(ref), "enrollsecret") {
		t.Fatal("reference contains credentials")
	}
	if o.External["secretNamespace"] != "smp-prototype" {
		t.Fatal("namespace drift")
	}
}
func TestPrototypeRejectsUnsafeMaterialWithoutEcho(t *testing.T) {
	m := fixture(t)
	for _, tc := range []struct {
		name, namespace string
		m               Material
		now             time.Time
	}{
		{"namespace", "../foreign", m, time.Now()},
		{"expired", "smp-prototype", m, time.Now().Add(24 * time.Hour)},
		{"key", "smp-prototype", Material{m.Certificate, []byte("secret-canary"), m.Root}, time.Now()},
		{"trust", "smp-prototype", Material{m.Certificate, m.PrivateKey, fixture(t).Root}, time.Now()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, e := Build(tc.namespace, "peer-cert", tc.m, tc.now)
			if e == nil {
				t.Fatal("unsafe material accepted")
			}
			if strings.Contains(e.Error(), "secret-canary") || strings.Contains(e.Error(), "PRIVATE KEY") {
				t.Fatal("error leaked input")
			}
		})
	}
}
