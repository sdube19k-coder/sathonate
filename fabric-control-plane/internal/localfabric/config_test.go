package localfabric

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsolatedCAOnlyTopology(t *testing.T) {
	cfg := compose(t.TempDir(), map[string]string{"orderers": "secret", "org1": "secret", "org2": "secret"})
	services := cfg["services"].(object)
	if len(services) != 10 {
		t.Fatal("expected 3 CAs, 3 orderers, 2 peers, tools, contract")
	}
	for n, s := range services {
		v := s.(object)
		for _, port := range toStrings(v["ports"]) {
			if !strings.HasPrefix(port, "127.0.0.1:") {
				t.Fatal("externally published port", n, port)
			}
		}
		for _, mount := range toStrings(v["volumes"]) {
			if strings.Contains(mount, "docker.sock") {
				t.Fatal("Docker socket mounted into node")
			}
		}
	}
	for _, n := range []string{"ca-orderers", "ca-org1", "ca-org2"} {
		if services[n].(object)["image"] != "hyperledger/fabric-ca:"+CAVersion {
			t.Fatal("CA missing")
		}
	}
	for _, n := range []string{"orderer1", "orderer2", "orderer3"} {
		env := services[n].(object)["environment"].(object)
		if env["ORDERER_GENERAL_TLS_ENABLED"] != "true" || env["ORDERER_CHANNELPARTICIPATION_ENABLED"] != "true" {
			t.Fatal("unsafe orderer profile")
		}
	}
	b, _ := json.Marshal(configtx())
	if !strings.Contains(string(b), "etcdraft") || strings.Contains(string(b), "solo") {
		t.Fatal("wrong ordering consensus")
	}
}
func toStrings(v any) []string {
	if v == nil {
		return nil
	}
	return v.([]string)
}
func TestInterruptedOperationCannotRemainReady(t *testing.T) {
	root := t.TempDir()
	b, _ := json.Marshal(State{Phase: "starting", Verified: true, Channel: "sathonate", Version: Version})
	if e := os.WriteFile(filepath.Join(root, "state.json"), b, 0600); e != nil {
		t.Fatal(e)
	}
	m, e := New(root)
	if e != nil {
		t.Fatal(e)
	}
	if m.state.Phase != "interrupted" || m.state.Verified {
		t.Fatal("interrupted job marked ready")
	}
}
func TestArchiveIsDeterministic(t *testing.T) {
	a, e := archive(map[string][]byte{"metadata.json": []byte("metadata"), "code.tar.gz": []byte("code")})
	if e != nil {
		t.Fatal(e)
	}
	b, _ := archive(map[string][]byte{"metadata.json": []byte("metadata"), "code.tar.gz": []byte("code")})
	if string(a) != string(b) {
		t.Fatal("package ID changes without input changes")
	}
}
