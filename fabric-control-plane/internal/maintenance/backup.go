package maintenance

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"os"
	"time"
)

const MaxBackupBytes = 16 << 20

type backupEnvelope struct {
	Schema        string    `json:"schema"`
	BindingSHA256 string    `json:"bindingSHA256"`
	CreatedAt     time.Time `json:"createdAt"`
	Nonce         []byte    `json:"nonce"`
	Ciphertext    []byte    `json:"ciphertext"`
}
type BackupReceipt struct {
	Schema           string    `json:"schema"`
	BindingSHA256    string    `json:"bindingSHA256"`
	CiphertextSHA256 string    `json:"ciphertextSHA256"`
	CreatedAt        time.Time `json:"createdAt"`
}

func backupCipher(key []byte) (cipher.AEAD, error) {
	if len(key) != 32 {
		return nil, errors.New("backup requires a protected 32-byte encryption key")
	}
	block, e := aes.NewCipher(key)
	if e != nil {
		return nil, errors.New("backup key unavailable")
	}
	return cipher.NewGCM(block)
}
func backupAAD(e backupEnvelope) []byte {
	raw, _ := json.Marshal([]string{e.Schema, e.BindingSHA256, e.CreatedAt.UTC().Format(time.RFC3339Nano)})
	return raw
}
func SealBackup(binding Binding, key, payload []byte, now time.Time) ([]byte, BackupReceipt, error) {
	if !binding.valid() || len(payload) == 0 || len(payload) > MaxBackupBytes {
		return nil, BackupReceipt{}, errors.New("invalid bound backup input")
	}
	aead, e := backupCipher(key)
	if e != nil {
		return nil, BackupReceipt{}, e
	}
	envelope := backupEnvelope{Schema: "sathonate.backup/v1", BindingSHA256: binding.Digest(), CreatedAt: now.UTC(), Nonce: make([]byte, aead.NonceSize())}
	if _, e = rand.Read(envelope.Nonce); e != nil {
		return nil, BackupReceipt{}, errors.New("backup randomness unavailable")
	}
	envelope.Ciphertext = aead.Seal(nil, envelope.Nonce, payload, backupAAD(envelope))
	raw, e := json.Marshal(envelope)
	if e != nil {
		return nil, BackupReceipt{}, errors.New("backup encoding failed")
	}
	receipt := BackupReceipt{envelope.Schema, envelope.BindingSHA256, HashBytes(raw), envelope.CreatedAt}
	return raw, receipt, nil
}
func OpenBackup(binding Binding, key, encoded []byte) ([]byte, BackupReceipt, error) {
	denied := errors.New("backup authentication or binding verification failed")
	if !binding.valid() || len(encoded) > (MaxBackupBytes*2) {
		return nil, BackupReceipt{}, denied
	}
	var envelope backupEnvelope
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	var extra any
	if decoder.Decode(&envelope) != nil || decoder.Decode(&extra) != io.EOF || envelope.Schema != "sathonate.backup/v1" || envelope.BindingSHA256 != binding.Digest() {
		return nil, BackupReceipt{}, denied
	}
	aead, e := backupCipher(key)
	if e != nil {
		return nil, BackupReceipt{}, e
	}
	if len(envelope.Nonce) != aead.NonceSize() {
		return nil, BackupReceipt{}, denied
	}
	plain, e := aead.Open(nil, envelope.Nonce, envelope.Ciphertext, backupAAD(envelope))
	if e != nil {
		return nil, BackupReceipt{}, denied
	}
	return plain, BackupReceipt{envelope.Schema, envelope.BindingSHA256, HashBytes(encoded), envelope.CreatedAt}, nil
}

// CreatePrivate refuses overwrite and restricts access before writing any bytes.
func CreatePrivate(path string, data []byte) error {
	file, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return errors.New("private output must be a new file in an approved directory")
	}
	defer file.Close()
	if e = protectPrivateFile(path); e != nil {
		return errors.New("cannot restrict private output access")
	}
	if _, e = file.Write(data); e != nil {
		return errors.New("private output write failed")
	}
	if file.Sync() != nil {
		return errors.New("private output sync failed")
	}
	return nil
}

type CertificateObservation struct {
	SHA256        string    `json:"sha256"`
	NotBefore     time.Time `json:"notBefore"`
	NotAfter      time.Time `json:"notAfter"`
	ValidNow      bool      `json:"validNow"`
	ChainVerified bool      `json:"chainVerified"`
}

func InspectCertificate(raw []byte, now time.Time) (CertificateObservation, error) {
	if len(raw) > 1<<20 || bytes.Contains(raw, []byte("PRIVATE KEY")) {
		return CertificateObservation{}, errors.New("input must contain only one public certificate")
	}
	block, rest := pem.Decode(raw)
	if block == nil || block.Type != "CERTIFICATE" || len(bytes.TrimSpace(rest)) != 0 {
		return CertificateObservation{}, errors.New("input must contain only one public certificate")
	}
	cert, e := x509.ParseCertificate(block.Bytes)
	if e != nil {
		return CertificateObservation{}, errors.New("invalid public certificate")
	}
	return CertificateObservation{SHA256: HashBytes(block.Bytes), NotBefore: cert.NotBefore, NotAfter: cert.NotAfter, ValidNow: !now.Before(cert.NotBefore) && now.Before(cert.NotAfter), ChainVerified: false}, nil
}
