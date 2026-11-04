package localfabric

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"crypto/tls"
	"encoding/base64"
	"encoding/pem"
	"sathonate.local/fabric-control-plane/internal/certificateprototype"
	"strings"
	"testing"
	"time"
)

// Explicit opt-in: validates official Fabric CA config-file delivery in a new
// isolated disposable container, without touching the retained local network.
func TestProtectedCAConfigOnDocker(t *testing.T) {
	if os.Getenv("SMP_CREDENTIAL_DOCKER_E2E") != "1" {
		t.Skip("opt in to disposable real Fabric CA configuration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	dir := t.TempDir()
	password := secret()
	identityPassword := secret()
	write := func(name string, v any) {
		raw, e := json.Marshal(v)
		if e != nil {
			t.Fatal("cannot encode test config")
		}
		if os.WriteFile(filepath.Join(dir, name), raw, 0600) != nil {
			t.Fatal("cannot write private test config")
		}
	}
	write("server.json", caBootstrap(password))
	config, argv, err := protectedCAClientConfig([]string{"enroll", "-u", "https://registrar:" + password + "@localhost:7054"})
	if err != nil || len(argv) != 1 {
		t.Fatal("invalid protected client config")
	}
	write("client.json", config)
	write("register.json", object{"url": "https://localhost:7054", "id": object{"secret": identityPassword}})
	write("identity.json", object{"url": "https://test-peer:" + identityPassword + "@localhost:7054"})
	name := fmt.Sprintf("smp-credential-test-%d", time.Now().UnixNano())
	defer func() { _ = exec.Command("docker", "rm", "-f", name).Run() }()
	script := `mkdir -p /etc/hyperledger/fabric-ca-server; cp /private/server.json /etc/hyperledger/fabric-ca-server/smp-bootstrap.json; fabric-ca-server start --config /etc/hyperledger/fabric-ca-server/smp-bootstrap.json >/tmp/ca-private.log 2>&1 &
 mkdir -p /client; cp /private/client.json /client/client.json; cp /private/register.json /client/register.json; cp /private/identity.json /client/identity.json
 for i in $(seq 1 30); do
  fabric-ca-client enroll --config /client/client.json --mspdir /client/msp --tls.certfiles /etc/hyperledger/fabric-ca-server/tls-cert.pem >/tmp/client-private.log 2>&1 && {
    fabric-ca-client register --config /client/register.json --mspdir /client/msp --tls.certfiles /etc/hyperledger/fabric-ca-server/tls-cert.pem --id.name test-peer --id.type peer >/tmp/register-private.log 2>&1 || exit 1
    fabric-ca-client enroll --config /client/identity.json --mspdir /client/peer-msp --tls.certfiles /etc/hyperledger/fabric-ca-server/tls-cert.pem >/tmp/identity-private.log 2>&1 || exit 1
    before=$(base64 /client/peer-msp/signcerts/cert.pem | tr -d '\n')
    fabric-ca-client reenroll --config /client/identity.json --mspdir /client/peer-msp --tls.certfiles /etc/hyperledger/fabric-ca-server/tls-cert.pem >/tmp/reenroll-private.log 2>&1 || exit 1
    printf 'SMP_CERT_BUNDLE:%s:%s:%s:%s\n' "$before" "$(base64 /client/peer-msp/signcerts/cert.pem | tr -d '\n')" "$(cat /client/peer-msp/keystore/* | base64 | tr -d '\n')" "$(cat /client/peer-msp/cacerts/* | base64 | tr -d '\n')"
    exit 0
  }
  sleep 1
 done
 cat /tmp/ca-private.log /tmp/client-private.log
 exit 1`
	cmd := exec.CommandContext(ctx, "docker", "run", "--rm", "--name", name, "--network", "none", "--mount", "type=bind,source="+dir+",target=/private,readonly", "-e", "FABRIC_CA_SERVER_HOME=/ca", "-e", "FABRIC_CA_SERVER_TLS_ENABLED=true", "-e", "FABRIC_CA_SERVER_CSR_HOSTS=localhost", "-e", "FABRIC_CA_CLIENT_HOME=/client", "--entrypoint", "sh", "hyperledger/fabric-ca:"+CAVersion, "-c", script)
	// Never publish upstream diagnostics from a failed secret-bearing operation.
	output, runErr := cmd.CombinedOutput()
	if runErr != nil {
		t.Fatal("protected CA test failed; secret-bearing diagnostics withheld")
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var fields []string
	for _, line := range lines {
		if strings.HasPrefix(line, "SMP_CERT_BUNDLE:") {
			fields = strings.Split(line, ":")
		}
	}
	if len(fields) != 5 {
		t.Fatal("private certificate bundle unavailable")
	}
	decode := func(v string) []byte {
		raw, e := base64.StdEncoding.DecodeString(strings.TrimSpace(v))
		if e != nil {
			t.Fatal("private certificate decoding failed")
		}
		return raw
	}
	if fields[1] == fields[2] {
		t.Fatal("reenrollment did not replace certificate")
	}
	material := certificateprototype.Material{Certificate: decode(fields[2]), PrivateKey: decode(fields[3]), Root: decode(fields[4])}
	// Renewal may retain old keys. Select only the key matching the renewed cert.
	keys := material.PrivateKey
	material.PrivateKey = nil
	for len(keys) > 0 {
		block, rest := pem.Decode(keys)
		if block == nil {
			break
		}
		keys = rest
		candidate := pem.EncodeToMemory(block)
		if _, err := tls.X509KeyPair(material.Certificate, candidate); err == nil {
			material.PrivateKey = candidate
			break
		}
	}
	objects, e := certificateprototype.Build("smp-prototype", "preenrolled-peer", material, time.Now())
	if e != nil {
		t.Fatal("pre-enrolled certificate prototype rejected genuine CA material")
	}
	encoded, _ := json.Marshal(objects.External)
	if strings.Contains(string(encoded), password) || strings.Contains(string(encoded), identityPassword) || strings.Contains(string(encoded), "PRIVATE KEY") {
		t.Fatal("external reference leaked credential")
	}
	t.Log("Genuine CA reenrollment and password-free external certificate Secret prototype verified in memory; no Fabric CR was deployed")
	t.Log("Official Fabric CA TLS registrar enrollment, restricted registration and separate peer enrollment succeeded using protected config files; no password in container arguments")
}
