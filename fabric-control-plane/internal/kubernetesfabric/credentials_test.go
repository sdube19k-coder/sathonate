package kubernetesfabric

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestEnrollmentArgumentsUsePrivateStdin(t *testing.T) {
	password := strings.Repeat("unique-secret-", 5)
	logical := []string{"ca", "register", "--secret", password, "--enroll-secret", password}
	argv, stdin, err := secretCLICommand(logical)
	if err != nil || strings.Contains(strings.Join(argv, " "), password) || len(argv) != 1 || argv[0] != "--smp-stdin-arguments" {
		t.Fatal("password reached process argv")
	}
	var decoded []string
	if json.Unmarshal(stdin, &decoded) != nil || decoded[3] != password {
		t.Fatal("private transport lost input")
	}
	if _, _, err = secretCLICommand([]string{strings.Repeat("x", (1<<20)+1)}); err == nil {
		t.Fatal("oversized input accepted")
	}
}
func TestRenderedEnrollmentExceptionIsNarrow(t *testing.T) {
	password := strings.Repeat("a", 64)
	w := Worker{secrets: []string{password}}
	peer := Object{"kind": "FabricPeer", "spec": Object{"secret": Object{"enrollment": Object{"component": Object{"enrollsecret": password}, "tls": Object{"enrollsecret": password}}}}}
	if err := w.validateRenderedCredentials(peer); err != nil {
		t.Fatal("required pinned operator field rejected", err)
	}
	peer["metadata"] = Object{"annotations": Object{"password": password}}
	if err := w.validateRenderedCredentials(peer); err == nil || strings.Contains(err.Error(), password) {
		t.Fatal("unexpected rendered credential accepted or exposed")
	}
	if err := w.validateRenderedCredentials(Object{"kind": "ConfigMap", "data": Object{"x": password}}); err == nil {
		t.Fatal("credential configuration artifact accepted")
	}
}
func TestEnrollmentEventsRedactPasswordAndKey(t *testing.T) {
	password := strings.Repeat("b", 64)
	encoded := base64.StdEncoding.EncodeToString([]byte(password))
	var output bytes.Buffer
	w := Worker{secrets: []string{password, encoded}, Output: func(e Event) error { return json.NewEncoder(&output).Encode(e) }}
	if err := w.event("ca", password+" "+encoded+" -----BEGIN PRIVATE KEY-----\nmaterial\n-----END PRIVATE KEY-----"); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{password, encoded, "material"} {
		if strings.Contains(output.String(), value) {
			t.Fatal("event leaked credential")
		}
	}
	if identityCredentialKey("org1", "peer1") == identityCredentialKey("org1", "peer2") || identityCredentialKey("org1", "peer1") == identityCredentialKey("org2", "peer1") {
		t.Fatal("identity credential names collide")
	}
}

func TestPinnedCATLSCredentialField(t *testing.T) {
	password := strings.Repeat("c", 64)
	identity := Object{"pass": password}
	v := Object{"kind": "FabricCA", "spec": Object{"tlsCA": Object{"registry": Object{"identities": []any{identity}}}}}
	w := Worker{secrets: []string{password}}
	if err := w.validateRenderedCredentials(v); err != nil {
		t.Fatal("pinned tlsCA path rejected")
	}
	restrictRegistrar(v)
	if object(identity["attrs"])["hf.Revoker"] != false {
		t.Fatal("TLS CA registrar privileges not restricted")
	}
}
