package redact

import (
	"strings"
	"testing"
)

func TestCredentialForms(t *testing.T) {
	input := `https://admin:super-secret@ca:7054 {"access_token":"abc123","password":"top-secret"} Bearer eyJabc.def.ghi vault_token=hvs.hidden
-----BEGIN EC PRIVATE KEY-----
key-material
-----END EC PRIVATE KEY-----
transaction 1234abcd`
	out := Text(input)
	for _, secret := range []string{"super-secret", "abc123", "top-secret", "eyJabc", "hvs.hidden", "key-material"} {
		if strings.Contains(out, secret) {
			t.Fatal("credential survived redaction", secret)
		}
	}
	if !strings.Contains(out, "transaction 1234abcd") {
		t.Fatal("ordinary diagnostics removed")
	}
}
