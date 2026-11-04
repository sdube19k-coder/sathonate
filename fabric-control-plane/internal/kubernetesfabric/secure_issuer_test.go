package kubernetesfabric

import (
	"bytes"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRestoredRegistrarCertificateMatchesFabricCAClientContract(t *testing.T) {
	source, e := os.ReadFile("../../.." + "/dependencies/bevel-operator-fabric/" + "internal/github.com/hyperledger/fabric-ca/lib/client.go")
	if e != nil {
		t.Skip("pinned Fabric CA dependency source not present")
	}
	// Verify against the pinned dependency, rather than only mirroring our code.
	if !bytes.Contains(source, []byte(`c.certFile = path.Join(certDir, "cert.pem")`)) {
		t.Fatal("review pinned CA client certificate lookup contract")
	}
	if filepath.Join("signcerts", restoredSigningCertificate) != filepath.Join("signcerts", "cert.pem") {
		t.Fatal("restored registrar cannot be loaded by Fabric CA client")
	}
}

func TestSecureLedgerFramingPreservesBinaryWhitespaceAndRejectsMalformedOutput(t *testing.T) {
	data := []byte{0x0a, 0x20, 0xff, 0x0a}
	decoded, e := decodeSecureLedgerHex([]byte(hex.EncodeToString(data) + "\n"))
	if e != nil || !bytes.Equal(decoded, data) {
		t.Fatal("ledger transport altered binary payload")
	}
	for _, invalid := range []string{"", "[10 32 255 10]\n", "abc", "invalid"} {
		if _, e = decodeSecureLedgerHex([]byte(invalid)); e == nil {
			t.Fatal("malformed ledger output accepted")
		}
	}
}

func TestSecurePeerErrorsExportOnlyFixedCategories(t *testing.T) {
	for _, diagnostic := range []string{"access denied: synthetic-sensitive-detail", "connection refused: synthetic-sensitive-detail", "unknown synthetic-sensitive-detail"} {
		err := securePeerFailure([]byte(diagnostic))
		if strings.Contains(err.Error(), "synthetic-sensitive-detail") || !strings.Contains(err.Error(), "private diagnostics withheld") {
			t.Fatal("private CLI diagnostics escaped their boundary")
		}
	}
}

func TestSecureChaincodeMaterialMatchesQualifiedMountedFilenames(t *testing.T) {
	identity := map[string]string{"cert.pem": "synthetic-certificate", "key.pem": "synthetic-key", "root.pem": "synthetic-root"}
	mounted := chaincodeTLSMaterial(identity)
	// This is the external contract enforced by the unchanged qualified
	// chaincode controller and consumed by its container's TLS file paths.
	for filename, expected := range map[string]string{"tls.crt": identity["cert.pem"], "tls.key": identity["key.pem"], "tlsroot.crt": identity["root.pem"]} {
		if mounted[filename] != expected {
			t.Fatalf("qualified chaincode mount %s is missing or mismatched", filename)
		}
	}
	if len(mounted) != 3 || identity["key.pem"] != "synthetic-key" {
		t.Fatal("unexpected material or mutation of immutable issuer input")
	}
}
