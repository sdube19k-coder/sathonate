package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	cb "github.com/hyperledger/fabric-protos-go/common"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric-protos-go/orderer/etcdraft"
)

func testInput(t *testing.T, orgs, nodes int) Input {
	t.Helper()
	v := Input{Namespace: "smp-s-12345678901234567890123456789012", Channel: "customer-ledger"}
	var firstKey *ecdsa.PrivateKey
	var firstRoot *x509.Certificate
	for i := 0; i < orgs; i++ {
		key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		tpl := &x509.Certificate{SerialNumber: big.NewInt(int64(i + 1)), Subject: pkix.Name{CommonName: fmt.Sprintf("Independent root %d", i)}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
		der, e := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
		if e != nil {
			t.Fatal(e)
		}
		root, _ := x509.ParseCertificate(der)
		if i == 0 {
			firstKey = key
			firstRoot = root
		}
		cert := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
		id := fmt.Sprintf("Department%dMSP", i)
		v.Organizations = append(v.Organizations, Organization{MSP: id, Root: cert, TLSRoot: cert, Anchors: []string{fmt.Sprintf("department-%d-peer", i)}})
		v.ApplicationMSPs = append(v.ApplicationMSPs, id)
	}
	for i := 0; i < nodes; i++ {
		name := fmt.Sprintf("customer-orderer-%d", i)
		leaf := func(tls bool) string {
			key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			cn := name + "-sign"
			if tls {
				cn = name + "-tls"
			}
			c := &x509.Certificate{SerialNumber: big.NewInt(int64(100 + i*2)), Subject: pkix.Name{CommonName: cn, OrganizationalUnit: []string{"orderer"}}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature}
			if tls {
				c.DNSNames = []string{name + "." + v.Namespace}
				c.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}
			}
			der, e := x509.CreateCertificate(rand.Reader, c, firstRoot, &key.PublicKey, firstKey)
			if e != nil {
				t.Fatal(e)
			}
			return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
		}
		v.Orderers = append(v.Orderers, Node{Name: name, MSP: v.Organizations[0].MSP, TLS: leaf(true), Sign: leaf(false)})
	}
	return v
}
func TestIndependentCustomerGenesisMatrix(t *testing.T) {
	for _, tc := range []struct{ orgs, nodes int }{{2, 3}, {3, 3}, {2, 5}} {
		t.Run(fmt.Sprintf("%d-organizations-%d-orderers", tc.orgs, tc.nodes), func(t *testing.T) {
			v := testInput(t, tc.orgs, tc.nodes)
			block, e := generate(v)
			if e != nil {
				t.Fatal(e)
			}
			c, e := configFromBlock(block)
			if e != nil || len(c.ChannelGroup.Groups["Application"].Groups) != tc.orgs {
				t.Fatal("application membership missing", e)
			}
			var cons ab.ConsensusType
			var raft etcdraft.ConfigMetadata
			if proto.Unmarshal(c.ChannelGroup.Groups["Orderer"].Values["ConsensusType"].Value, &cons) != nil || proto.Unmarshal(cons.Metadata, &raft) != nil || len(raft.Consenters) != tc.nodes {
				t.Fatal("Raft membership differs from customer configuration")
			}
			for _, org := range v.Organizations {
				if c.ChannelGroup.Groups["Application"].Groups[org.MSP].Values["AnchorPeers"] == nil {
					t.Fatal("anchor configuration absent")
				}
			}
		})
	}
}
func TestConfigurationUpdateIsIdempotentAndSingleConsenter(t *testing.T) {
	all := testInput(t, 3, 5)
	base := all
	base.Orderers = all.Orderers[:3]
	base.ApplicationMSPs = all.ApplicationMSPs[:2]
	block, e := generate(base)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := proto.Marshal(block)
	base.Current = raw
	r, e := reconcile(base)
	if e != nil || r.Changed {
		t.Fatal("same input was not a no-op", e)
	}
	grow := all
	grow.Orderers = all.Orderers[:4]
	grow.Current = raw
	r, e = reconcile(grow)
	if e != nil || !r.Changed || len(r.Update) == 0 {
		t.Fatal("additive organization and single orderer update missing", e)
	}
	var env cb.Envelope
	var payload cb.Payload
	var envelope cb.ConfigUpdateEnvelope
	var update cb.ConfigUpdate
	if proto.Unmarshal(r.Update, &env) != nil || proto.Unmarshal(env.Payload, &payload) != nil || proto.Unmarshal(payload.Data, &envelope) != nil || proto.Unmarshal(envelope.ConfigUpdate, &update) != nil || update.ChannelId != all.Channel || len(envelope.Signatures) != 0 {
		t.Fatal("update not unsigned and channel-bound")
	}
	application := update.WriteSet.Groups["Application"]
	if application == nil || application.Groups[all.ApplicationMSPs[2]] == nil || application.Groups[all.ApplicationMSPs[2]].Values["AnchorPeers"] == nil {
		t.Fatal("new application MSP/anchors absent from real update")
	}
	ordering := update.WriteSet.Groups["Orderer"]
	if ordering == nil || ordering.Values["ConsensusType"] == nil {
		t.Fatal("Raft config update absent")
	}
	var consensus ab.ConsensusType
	var membership etcdraft.ConfigMetadata
	if proto.Unmarshal(ordering.Values["ConsensusType"].Value, &consensus) != nil || proto.Unmarshal(consensus.Metadata, &membership) != nil || len(membership.Consenters) != 4 {
		t.Fatal("actual consenter update differs")
	}
	if ordering.Groups[all.Organizations[0].MSP] == nil || ordering.Groups[all.Organizations[0].MSP].Values["Endpoints"] == nil {
		t.Fatal("expanded ordering endpoints missing")
	}
	all.Current = raw
	if _, e = reconcile(all); e == nil {
		t.Fatal("simultaneous multi-consenter update accepted")
	}
	base.Organizations[0].Root = testInput(t, 1, 3).Organizations[0].Root
	if _, e = reconcile(base); e == nil {
		t.Fatal("uncoordinated root replacement accepted")
	}
}

func TestCurrentConfigurationMustBeAValidBoundChannelBlock(t *testing.T) {
	v := testInput(t, 2, 3)
	block, e := generate(v)
	if e != nil {
		t.Fatal(e)
	}
	current, _ := proto.Marshal(block)
	v.Current = current
	if _, e = reconcile(v); e != nil {
		t.Fatal(e)
	}
	v.Channel = "foreign-channel"
	if _, e = reconcile(v); e == nil {
		t.Fatal("foreign channel config accepted")
	}
	v.Channel = "customer-ledger"
	block.Header.DataHash[0] ^= 1
	v.Current, _ = proto.Marshal(block)
	if _, e = reconcile(v); e == nil {
		t.Fatal("corrupt block hash accepted")
	}
	block, _ = generate(v)
	c, _ := configFromBlock(block)
	v.Current, _ = proto.Marshal(c)
	if _, e = reconcile(v); e == nil {
		t.Fatal("unbound raw config accepted")
	}
	block, _ = generate(v)
	var env cb.Envelope
	var payload cb.Payload
	var ce cb.ConfigEnvelope
	_ = proto.Unmarshal(block.Data.Data[0], &env)
	_ = proto.Unmarshal(env.Payload, &payload)
	_ = proto.Unmarshal(payload.Data, &ce)
	ce.Config.ChannelGroup.Groups["Application"].Groups[v.ApplicationMSPs[0]] = nil
	payload.Data, _ = proto.Marshal(&ce)
	env.Payload, _ = proto.Marshal(&payload)
	block.Data.Data[0], _ = proto.Marshal(&env)
	hash := sha256.Sum256(block.Data.Data[0])
	block.Header.DataHash = hash[:]
	v.Current, _ = proto.Marshal(block)
	if _, e = reconcile(v); e == nil {
		t.Fatal("malformed current membership accepted")
	}
}

func TestNoInitialEvenRaftOrImplicitRemoval(t *testing.T) {
	v := testInput(t, 3, 5)
	v.Orderers = v.Orderers[:4]
	if _, e := reconcile(v); e == nil {
		t.Fatal("initial even Raft accepted")
	}
	v.Orderers = v.Orderers[:3]
	block, e := generate(v)
	if e != nil {
		t.Fatal(e)
	}
	v.Current, _ = proto.Marshal(block)
	v.ApplicationMSPs = v.ApplicationMSPs[:2]
	if _, e = reconcile(v); e == nil {
		t.Fatal("application member removal accepted")
	}
}
func TestForeignNamespaceAndInvalidOrdererTrustRejected(t *testing.T) {
	v := testInput(t, 2, 3)
	v.Namespace = "smp-s-99999999999999999999999999999999"
	if _, e := generate(v); e == nil {
		t.Fatal("foreign service certificate accepted")
	}
	v = testInput(t, 2, 3)
	v.Orderers[0].TLS = testInput(t, 2, 3).Orderers[0].TLS
	if _, e := generate(v); e == nil {
		t.Fatal("foreign issuer accepted")
	}
}
