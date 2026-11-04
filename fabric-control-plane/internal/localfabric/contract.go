package localfabric

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func archive(files map[string][]byte) ([]byte, error) {
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	for _, name := range []string{"metadata.json", "code.tar.gz", "connection.json"} {
		data, ok := files[name]
		if !ok {
			continue
		}
		if e := tw.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: int64(len(data))}); e != nil {
			return nil, e
		}
		if _, e := tw.Write(data); e != nil {
			return nil, e
		}
	}
	if e := tw.Close(); e != nil {
		return nil, e
	}
	if e := gz.Close(); e != nil {
		return nil, e
	}
	return b.Bytes(), nil
}
func (m *Manager) deployContract(ctx context.Context, passwords map[string]string) error {
	m.event("Packaging Sathonate's Go shim contract as a TLS chaincode service")
	if e := copyFile(filepath.Join("bin", "local-chaincode"), filepath.Join(m.root, "contract-bin", "local-chaincode")); e != nil {
		return errors.New("build bin/local-chaincode with scripts/build-contract.ps1 before starting")
	}
	root, e := os.ReadFile(filepath.Join(m.root, "organizations", "org1", "contract", "tls", "ca.crt"))
	if e != nil {
		return e
	}
	connection, e := json.Marshal(object{"address": "contract:9999", "domain_name": "contract", "dial_timeout": "10s", "tls_required": true, "client_auth_required": false, "root_cert": string(root)})
	if e != nil {
		return e
	}
	code, e := archive(map[string][]byte{"connection.json": connection})
	if e != nil {
		return e
	}
	metadata := []byte(`{"type":"ccaas","label":"sathonate_smoke_1"}`)
	pkg, e := archive(map[string][]byte{"metadata.json": metadata, "code.tar.gz": code})
	if e != nil {
		return e
	}
	sum := sha256.Sum256(pkg)
	id := "sathonate_smoke_1:" + hex.EncodeToString(sum[:])
	if e = os.WriteFile(filepath.Join(m.root, "smoke.tgz"), pkg, 0600); e != nil {
		return e
	}
	if e = os.WriteFile(filepath.Join(m.root, "contract-id.txt"), []byte(id), 0600); e != nil {
		return e
	}
	if e = m.write("compose.json", compose(m.root, passwords)); e != nil {
		return e
	}
	if _, e = m.compose(ctx, "up", "-d", "contract"); e != nil {
		return errors.New("could not start the Go chaincode service")
	}
	orderer := []string{"-o", "orderer1:7050", "--tls", "--cafile", "/work/organizations/orderers/orderer1/tls/ca.crt"}
	peers := []string{"--peerAddresses", "peer1:7051", "--tlsRootCertFiles", "/work/organizations/org1/peer1/tls/ca.crt", "--peerAddresses", "peer2:7051", "--tlsRootCertFiles", "/work/organizations/org2/peer2/tls/ca.crt"}
	for i := 1; i <= 2; i++ {
		installed, _ := m.peer(ctx, i, "lifecycle", "chaincode", "queryinstalled", "--output", "json")
		if !strings.Contains(string(installed), id) {
			m.event(fmt.Sprintf("Installing the CAAS package on peer%d", i))
			if b, e := m.peer(ctx, i, "lifecycle", "chaincode", "install", "/work/smoke.tgz"); e != nil {
				return fmt.Errorf("peer%d contract install failed: %.1600s", i, b)
			}
		}
	}
	if _, e = m.peer(ctx, 1, "lifecycle", "chaincode", "querycommitted", "--channelID", "sathonate", "--name", "sathonate_smoke"); e != nil {
		for i := 1; i <= 2; i++ {
			m.event(fmt.Sprintf("Approving the contract definition as Org%dMSP", i))
			args := []string{"lifecycle", "chaincode", "approveformyorg", "--channelID", "sathonate", "--name", "sathonate_smoke", "--version", "1.0", "--package-id", id, "--sequence", "1", "--waitForEvent", "--waitForEventTimeout", "30s"}
			args = append(args, orderer...)
			if b, e := m.peer(ctx, i, args...); e != nil {
				return fmt.Errorf("organization %d approval failed: %.1800s", i, b)
			}
		}
		m.event("Committing the contract definition with both organizations")
		args := []string{"lifecycle", "chaincode", "commit", "--channelID", "sathonate", "--name", "sathonate_smoke", "--version", "1.0", "--sequence", "1", "--waitForEvent", "--waitForEventTimeout", "30s"}
		args = append(args, orderer...)
		args = append(args, peers...)
		if b, e := m.peer(ctx, 1, args...); e != nil {
			return fmt.Errorf("contract commit failed: %.1800s", b)
		}
	}
	if e := m.stage("verify"); e != nil {
		return e
	}
	return m.smoke(ctx)
}
func (m *Manager) smoke(ctx context.Context) error {
	orderer := []string{"-o", "orderer1:7050", "--tls", "--cafile", "/work/organizations/orderers/orderer1/tls/ca.crt"}
	peers := []string{"--peerAddresses", "peer1:7051", "--tlsRootCertFiles", "/work/organizations/org1/peer1/tls/ca.crt", "--peerAddresses", "peer2:7051", "--tlsRootCertFiles", "/work/organizations/org2/peer2/tls/ca.crt"}
	m.event("Submitting a real endorsed write and waiting for commit on both peers")
	args := []string{"chaincode", "invoke", "-C", "sathonate", "-n", "sathonate_smoke", "-c", `{"Args":["Put","sathonate-ready","ready"]}`, "--waitForEvent", "--waitForEventTimeout", "30s"}
	args = append(args, orderer...)
	args = append(args, peers...)
	b, e := m.peer(ctx, 1, args...)
	if e != nil {
		return fmt.Errorf("smoke transaction failed: %.1800s", b)
	}
	tx := regexp.MustCompile(`[a-f0-9]{64}`).FindString(string(b))
	if tx == "" {
		return errors.New("committed transaction did not return a transaction ID")
	}
	for i := 1; i <= 2; i++ {
		if e := retry(ctx, 10, func() error {
			b, e := m.peer(ctx, i, "chaincode", "query", "-C", "sathonate", "-n", "sathonate_smoke", "-c", `{"Args":["Get","sathonate-ready"]}`)
			if e != nil || strings.TrimSpace(string(b)) != "ready" {
				return fmt.Errorf("query failed: %.1000s", b)
			}
			return nil
		}); e != nil {
			return fmt.Errorf("peer%d did not return the committed smoke value: %w", i, e)
		}
	}
	m.event("Application check passed: the committed value 'ready' was returned by both peers over TLS")
	m.mu.Lock()
	m.state.TransactionID = tx
	m.state.VerifiedAt = time.Now().UTC()
	m.mu.Unlock()
	return nil
}
