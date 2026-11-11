package maintenance

import (
	"bytes"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"time"
)

type CertificateHealth struct {
	SHA256    string    `json:"sha256"`
	ExpiresAt time.Time `json:"expiresAt"`
	Status    string    `json:"status"`
}

func publicCertificates(raw []byte) ([]*x509.Certificate, error) {
	var certificates []*x509.Certificate
	for len(bytes.TrimSpace(raw)) != 0 {
		raw = bytes.TrimSpace(raw)
		block, rest := pem.Decode(raw)
		if block == nil || block.Type != "CERTIFICATE" || len(block.Headers) != 0 ||
			!bytes.HasPrefix(raw, []byte("-----BEGIN CERTIFICATE-----")) ||
			bytes.Count(raw[:len(raw)-len(rest)], []byte("-----BEGIN ")) != 1 {
			return nil, errors.New("expected public certificate PEM only")
		}
		certificate, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, errors.New("invalid certificate")
		}
		certificates = append(certificates, certificate)
		raw = rest
	}
	if len(certificates) == 0 {
		return nil, errors.New("missing public certificate")
	}
	return certificates, nil
}

// VerifyServerCertificate evaluates an explicit server trust chain and endpoint.
// It returns expiry for the longest-lived currently valid chain.
func VerifyServerCertificate(chain, roots []byte, endpoint string, now time.Time, warnBefore time.Duration) (CertificateHealth, error) {
	if endpoint == "" || now.IsZero() || warnBefore <= 0 {
		return CertificateHealth{}, errors.New("endpoint, observation time and positive renewal window required")
	}
	leafChain, err := publicCertificates(chain)
	if err != nil {
		return CertificateHealth{}, err
	}
	anchors, err := publicCertificates(roots)
	if err != nil {
		return CertificateHealth{}, err
	}
	rootPool := x509.NewCertPool()
	for _, ca := range anchors {
		if !ca.IsCA {
			return CertificateHealth{}, errors.New("trust anchor is not a CA")
		}
		rootPool.AddCert(ca)
	}
	intermediatePool := x509.NewCertPool()
	for _, ca := range leafChain[1:] {
		intermediatePool.AddCert(ca)
	}
	leaf := leafChain[0]
	if leaf.IsCA {
		return CertificateHealth{}, errors.New("server endpoint requires a leaf certificate")
	}
	verified, err := leaf.Verify(x509.VerifyOptions{DNSName: endpoint, Roots: rootPool, Intermediates: intermediatePool, CurrentTime: now, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}})
	if err != nil {
		return CertificateHealth{}, errors.New("server certificate trust verification failed")
	}
	var expiry time.Time
	for _, candidate := range verified {
		earliest := leaf.NotAfter
		for _, member := range candidate {
			if member.NotAfter.Before(earliest) {
				earliest = member.NotAfter
			}
		}
		if earliest.After(expiry) {
			expiry = earliest
		}
	}
	status := "healthy"
	if expiry.Sub(now) <= warnBefore {
		status = "renewal-due"
	}
	return CertificateHealth{SHA256: HashBytes(leaf.Raw), ExpiresAt: expiry.UTC(), Status: status}, nil
}
