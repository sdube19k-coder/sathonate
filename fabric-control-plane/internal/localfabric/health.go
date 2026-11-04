package localfabric

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Ledger struct {
	Peer      string `json:"peer"`
	MSPID     string `json:"mspId"`
	Available bool   `json:"available"`
	Height    uint64 `json:"height"`
	Hash      string `json:"currentBlockHash"`
	Error     string `json:"error,omitempty"`
}
type Consenter struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Relation string `json:"consensusRelation"`
	Height   uint64 `json:"height"`
}
type CAHealth struct {
	Name      string `json:"name"`
	Available bool   `json:"available"`
	TLS       bool   `json:"tls"`
}
type ContractHealth struct {
	Name      string `json:"name"`
	Available bool   `json:"available"`
	Version   string `json:"version,omitempty"`
	Sequence  int    `json:"sequence,omitempty"`
}
type Health struct {
	CheckedAt time.Time      `json:"checkedAt"`
	Ready     bool           `json:"ready"`
	Channel   string         `json:"channel"`
	Peers     []Ledger       `json:"peers"`
	Orderers  []Consenter    `json:"orderers"`
	CAs       []CAHealth     `json:"certificateAuthorities"`
	Chaincode ContractHealth `json:"chaincode"`
	Alerts    []string       `json:"alerts"`
}

func decodeCLI(b []byte, v any) error {
	index := bytes.IndexByte(b, '{')
	if index < 0 {
		return fmt.Errorf("no JSON response")
	}
	return json.NewDecoder(bytes.NewReader(b[index:])).Decode(v)
}
func (m *Manager) Health(ctx context.Context) Health {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	h := Health{CheckedAt: time.Now().UTC(), Channel: "sathonate", Peers: make([]Ledger, 2), Orderers: make([]Consenter, 3), CAs: make([]CAHealth, 3), Alerts: []string{}, Chaincode: ContractHealth{Name: "sathonate_smoke"}}
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			p := Ledger{Peer: fmt.Sprintf("peer%d", index+1), MSPID: fmt.Sprintf("Org%dMSP", index+1)}
			b, e := m.peer(ctx, index+1, "channel", "getinfo", "-c", "sathonate")
			var result struct {
				Height uint64 `json:"height"`
				Hash   string `json:"currentBlockHash"`
			}
			if e == nil && decodeCLI(b, &result) == nil && result.Height > 0 && result.Hash != "" {
				p.Available = true
				p.Height = result.Height
				p.Hash = result.Hash
			} else {
				p.Error = "Channel ledger could not be queried over TLS"
			}
			h.Peers[index] = p
		}(i)
	}
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			n := fmt.Sprintf("orderer%d", index+1)
			o := Consenter{Name: n, Status: "unreachable"}
			b, e := m.tool(ctx, "osnadmin", "channel", "list", "--channelID", "sathonate", "-o", n+":7053", "--ca-file", "/work/organizations/orderers/"+n+"/tls/ca.crt", "--client-cert", "/work/organizations/orderers/admin/tls/server.crt", "--client-key", "/work/organizations/orderers/admin/tls/server.key")
			var result struct {
				Status   string `json:"status"`
				Relation string `json:"consensusRelation"`
				Height   uint64 `json:"height"`
			}
			if e == nil && decodeCLI(b, &result) == nil && result.Status != "" {
				o.Status = result.Status
				o.Relation = result.Relation
				o.Height = result.Height
			}
			h.Orderers[index] = o
		}(i)
	}
	for i, org := range []string{"orderers", "org1", "org2"} {
		wg.Add(1)
		go func(index int, org string) {
			defer wg.Done()
			ca := CAHealth{Name: "ca-" + org, TLS: true}
			pem, e := os.ReadFile(filepath.Join(m.root, "cas", org, "tls-cert.pem"))
			if e == nil {
				pool := x509.NewCertPool()
				if pool.AppendCertsFromPEM(pem) {
					transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}}
					defer transport.CloseIdleConnections()
					client := &http.Client{Transport: transport, Timeout: 4 * time.Second}
					req, _ := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("https://localhost:%d/cainfo", 17054+index*1000), nil)
					if response, e := client.Do(req); e == nil {
						defer response.Body.Close()
						var result struct {
							Success bool `json:"success"`
						}
						if response.StatusCode == 200 && json.NewDecoder(io.LimitReader(response.Body, 128<<10)).Decode(&result) == nil {
							ca.Available = result.Success
						}
					}
				}
			}
			h.CAs[index] = ca
		}(i, org)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		b, e := m.peer(ctx, 1, "lifecycle", "chaincode", "querycommitted", "--channelID", "sathonate", "--name", "sathonate_smoke", "--output", "json")
		var result struct {
			Version  string `json:"version"`
			Sequence int    `json:"sequence"`
		}
		if e == nil && decodeCLI(b, &result) == nil && result.Version != "" && result.Sequence > 0 {
			h.Chaincode.Available = true
			h.Chaincode.Version = result.Version
			h.Chaincode.Sequence = result.Sequence
		}
	}()
	wg.Wait()
	for _, p := range h.Peers {
		if !p.Available {
			h.Alerts = append(h.Alerts, p.Peer+": channel query failed")
		}
	}
	if h.Peers[0].Available && h.Peers[1].Available && (h.Peers[0].Height != h.Peers[1].Height || h.Peers[0].Hash != h.Peers[1].Hash) {
		h.Alerts = append(h.Alerts, "Peer ledgers are not at the same block; refresh after in-flight transactions settle")
	}
	for _, o := range h.Orderers {
		if o.Status != "active" || o.Relation != "consenter" {
			h.Alerts = append(h.Alerts, o.Name+": active Raft consenter was not confirmed")
		}
	}
	for _, ca := range h.CAs {
		if !ca.Available {
			h.Alerts = append(h.Alerts, ca.Name+": TLS CA information check failed")
		}
	}
	if !h.Chaincode.Available {
		h.Alerts = append(h.Alerts, "The committed contract definition could not be confirmed")
	}
	h.Ready = len(h.Alerts) == 0
	return h
}
