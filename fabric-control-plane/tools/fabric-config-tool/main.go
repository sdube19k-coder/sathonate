// Isolated protobuf-v1 config generator. It has no Kubernetes/network access or
// private-key inputs; the protected worker signs the resulting update in RAM.
package main

import (
	"bytes"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-config/configtx"
	"github.com/hyperledger/fabric-config/configtx/membership"
	"github.com/hyperledger/fabric-config/configtx/orderer"
	cb "github.com/hyperledger/fabric-protos-go/common"
	ab "github.com/hyperledger/fabric-protos-go/orderer"
	"github.com/hyperledger/fabric-protos-go/orderer/etcdraft"
	pb "github.com/hyperledger/fabric-protos-go/peer"
)

type Organization struct {
	MSP, Root, TLSRoot string
	Anchors            []string
}
type Node struct{ Name, MSP, TLS, Sign string }
type Input struct {
	Namespace       string         `json:"namespace"`
	Channel         string         `json:"channel"`
	Organizations   []Organization `json:"organizations"`
	ApplicationMSPs []string       `json:"applicationMSPs"`
	Orderers        []Node         `json:"orderers"`
	Current         []byte         `json:"current,omitempty"`
}
type Result struct {
	Genesis []byte `json:"genesis,omitempty"`
	Update  []byte `json:"update,omitempty"`
	Changed bool   `json:"changed"`
}

var denied = errors.New("configuration input, ownership or certificate validation failed")
var name = regexp.MustCompile(`^[a-z][a-z0-9-]{0,39}$`)
var ns = regexp.MustCompile(`^smp-s-[a-f0-9]{32}$`)
var mspName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]{0,63}$`)

func certificate(raw string, ca bool) (*x509.Certificate, error) {
	b, rest := pem.Decode([]byte(raw))
	if b == nil || b.Type != "CERTIFICATE" || len(bytes.TrimSpace(rest)) != 0 {
		return nil, denied
	}
	c, e := x509.ParseCertificate(b.Bytes)
	if e != nil || time.Now().Before(c.NotBefore) || time.Now().After(c.NotAfter) || c.IsCA != ca {
		return nil, denied
	}
	if ca && (!c.BasicConstraintsValid || c.KeyUsage&x509.KeyUsageCertSign == 0 || c.CheckSignatureFrom(c) != nil) {
		return nil, denied
	}
	return c, nil
}
func policies(admin bool) map[string]configtx.Policy {
	p := map[string]configtx.Policy{"Readers": {Type: "ImplicitMeta", Rule: "ANY Readers"}, "Writers": {Type: "ImplicitMeta", Rule: "ANY Writers"}, "Admins": {Type: "ImplicitMeta", Rule: "MAJORITY Admins"}}
	if admin {
		p["Endorsement"] = configtx.Policy{Type: "ImplicitMeta", Rule: "MAJORITY Endorsement"}
		p["LifecycleEndorsement"] = p["Endorsement"]
	}
	return p
}
func organization(o Organization, root, tlsRoot *x509.Certificate, endpoints []string, namespace string) (configtx.Organization, error) {
	anchors := []configtx.Address{}
	seen := map[string]bool{}
	for _, a := range o.Anchors {
		if !name.MatchString(a) || seen[a] {
			return configtx.Organization{}, denied
		}
		seen[a] = true
		anchors = append(anchors, configtx.Address{Host: a + "." + namespace, Port: 7051})
	}
	ous := func(role string) membership.OUIdentifier {
		return membership.OUIdentifier{Certificate: root, OrganizationalUnitIdentifier: role}
	}
	p := map[string]configtx.Policy{}
	for _, k := range []string{"Readers", "Writers", "Endorsement"} {
		p[k] = configtx.Policy{Type: "Signature", Rule: fmt.Sprintf("OR('%s.member')", o.MSP)}
	}
	p["Admins"] = configtx.Policy{Type: "Signature", Rule: fmt.Sprintf("OR('%s.admin')", o.MSP)}
	return configtx.Organization{Name: o.MSP, MSP: configtx.MSP{Name: o.MSP, RootCerts: []*x509.Certificate{root}, TLSRootCerts: []*x509.Certificate{tlsRoot}, NodeOUs: membership.NodeOUs{Enable: true, ClientOUIdentifier: ous("client"), PeerOUIdentifier: ous("peer"), AdminOUIdentifier: ous("admin"), OrdererOUIdentifier: ous("orderer")}}, Policies: p, AnchorPeers: anchors, OrdererEndpoints: endpoints}, nil
}
func generate(v Input) (*cb.Block, error) {
	if !ns.MatchString(v.Namespace) || !name.MatchString(v.Channel) || len(v.Organizations) < 1 || len(v.Organizations) > 8 || len(v.Orderers) < 3 || len(v.Orderers) > 10 {
		return nil, denied
	}
	roots := map[string]*x509.Certificate{}
	tlsRoots := map[string]*x509.Certificate{}
	orgs := map[string]Organization{}
	for _, o := range v.Organizations {
		if !mspName.MatchString(o.MSP) || orgs[o.MSP].MSP != "" {
			return nil, denied
		}
		r, e := certificate(o.Root, true)
		if e != nil {
			return nil, e
		}
		t, e := certificate(o.TLSRoot, true)
		if e != nil {
			return nil, e
		}
		roots[o.MSP] = r
		tlsRoots[o.MSP] = t
		orgs[o.MSP] = o
	}
	endpoints := map[string][]string{}
	consenters := []orderer.Consenter{}
	seen := map[string]bool{}
	for _, n := range v.Orderers {
		if !name.MatchString(n.Name) || seen[n.Name] || roots[n.MSP] == nil {
			return nil, denied
		}
		seen[n.Name] = true
		t, e := certificate(n.TLS, false)
		if e != nil {
			return nil, e
		}
		s, e := certificate(n.Sign, false)
		if e != nil {
			return nil, e
		}
		pool := x509.NewCertPool()
		pool.AddCert(tlsRoots[n.MSP])
		for _, usage := range []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth} {
			if _, e = t.Verify(x509.VerifyOptions{Roots: pool, DNSName: n.Name + "." + v.Namespace, KeyUsages: []x509.ExtKeyUsage{usage}}); e != nil {
				return nil, denied
			}
		}
		pool = x509.NewCertPool()
		pool.AddCert(roots[n.MSP])
		if _, e = s.Verify(x509.VerifyOptions{Roots: pool, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny}}); e != nil {
			return nil, denied
		}
		role := false
		for _, ou := range s.Subject.OrganizationalUnit {
			role = role || ou == "orderer"
		}
		if !role || s.Subject.CommonName != n.Name+"-sign" {
			return nil, denied
		}
		endpoints[n.MSP] = append(endpoints[n.MSP], n.Name+"."+v.Namespace+":7050")
		consenters = append(consenters, orderer.Consenter{Address: orderer.EtcdAddress{Host: n.Name + "." + v.Namespace, Port: 7050}, ClientTLSCert: t, ServerTLSCert: t})
	}
	application := []configtx.Organization{}
	ordering := []configtx.Organization{}
	members := map[string]bool{}
	for _, id := range v.ApplicationMSPs {
		if members[id] || orgs[id].MSP == "" {
			return nil, denied
		}
		members[id] = true
		o, e := organization(orgs[id], roots[id], tlsRoots[id], nil, v.Namespace)
		if e != nil {
			return nil, e
		}
		application = append(application, o)
	}
	if len(application) == 0 {
		return nil, denied
	}
	ids := []string{}
	for id := range endpoints {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		o, e := organization(orgs[id], roots[id], tlsRoots[id], endpoints[id], v.Namespace)
		if e != nil {
			return nil, e
		}
		ordering = append(ordering, o)
	}
	op := policies(false)
	op["BlockValidation"] = configtx.Policy{Type: "ImplicitMeta", Rule: "ANY Writers"}
	acls := map[string]string{}
	for _, function := range []string{"_lifecycle/CheckCommitReadiness", "_lifecycle/CommitChaincodeDefinition", "_lifecycle/QueryChaincodeDefinition", "_lifecycle/QueryChaincodeDefinitions", "peer/Propose", "peer/ChaincodeToChaincode"} {
		acls[function] = "/Channel/Application/Writers"
	}
	for _, function := range []string{"qscc/GetChainInfo", "qscc/GetBlockByNumber", "qscc/GetBlockByHash", "qscc/GetTransactionByID", "qscc/GetBlockByTxID", "cscc/GetConfigBlock", "cscc/GetChannelConfig", "event/Block", "event/FilteredBlock"} {
		acls[function] = "/Channel/Application/Readers"
	}
	c := configtx.Channel{Capabilities: []string{"V2_0"}, Policies: policies(false), Application: configtx.Application{Organizations: application, Capabilities: []string{"V2_0"}, Policies: policies(true), ACLs: acls}, Orderer: configtx.Orderer{OrdererType: orderer.ConsensusTypeEtcdRaft, Organizations: ordering, Policies: op, Capabilities: []string{"V2_0"}, BatchTimeout: time.Second, BatchSize: orderer.BatchSize{MaxMessageCount: 100, AbsoluteMaxBytes: 1048576, PreferredMaxBytes: 524288}, State: orderer.ConsensusStateNormal, EtcdRaft: orderer.EtcdRaft{Consenters: consenters, Options: orderer.EtcdRaftOptions{TickInterval: "500ms", ElectionTick: 10, HeartbeatTick: 1, MaxInflightBlocks: 5, SnapshotIntervalSize: 16777216}}}}
	block, e := configtx.NewApplicationChannelGenesisBlock(c, v.Channel)
	if e != nil {
		return nil, e
	}
	// The pinned fork omits Organization.AnchorPeers in newOrgConfigGroup.
	// Materialize configured anchors before finalizing this new genesis block.
	var env cb.Envelope
	var payload cb.Payload
	var ce cb.ConfigEnvelope
	if proto.Unmarshal(block.Data.Data[0], &env) != nil || proto.Unmarshal(env.Payload, &payload) != nil || proto.Unmarshal(payload.Data, &ce) != nil {
		return nil, denied
	}
	for _, o := range v.Organizations {
		g := ce.Config.ChannelGroup.Groups["Application"].Groups[o.MSP]
		if g == nil {
			continue
		}
		anchors := &pb.AnchorPeers{}
		for _, a := range o.Anchors {
			anchors.AnchorPeers = append(anchors.AnchorPeers, &pb.AnchorPeer{Host: a + "." + v.Namespace, Port: 7051})
		}
		value, _ := proto.Marshal(anchors)
		g.Values["AnchorPeers"] = &cb.ConfigValue{ModPolicy: "Admins", Value: value}
	}
	payload.Data, _ = proto.Marshal(&ce)
	env.Payload, _ = proto.Marshal(&payload)
	block.Data.Data[0], _ = proto.Marshal(&env)
	hash := sha256.Sum256(bytes.Join(block.Data.Data, nil))
	block.Header.DataHash = hash[:]
	return block, nil
}
func configFromBlock(block *cb.Block) (*cb.Config, error) {
	if block == nil || block.Header == nil || block.Data == nil || len(block.Data.Data) != 1 {
		return nil, denied
	}
	hash := sha256.Sum256(bytes.Join(block.Data.Data, nil))
	if !bytes.Equal(hash[:], block.Header.DataHash) {
		return nil, denied
	}
	var env cb.Envelope
	var payload cb.Payload
	var ce cb.ConfigEnvelope
	if proto.Unmarshal(block.Data.Data[0], &env) != nil || proto.Unmarshal(env.Payload, &payload) != nil || payload.Header == nil || proto.Unmarshal(payload.Data, &ce) != nil || ce.Config == nil || ce.Config.ChannelGroup == nil {
		return nil, denied
	}
	return ce.Config, nil
}
func reconcile(v Input) (Result, error) {
	block, e := generate(v)
	if e != nil {
		return Result{}, e
	}
	raw, e := proto.Marshal(block)
	if e != nil {
		return Result{}, denied
	}
	if len(v.Current) == 0 {
		if len(v.Orderers)%2 == 0 {
			return Result{}, denied
		}
		return Result{Genesis: raw, Changed: true}, nil
	}
	// Require the fetched CONFIG block, including its channel binding and hash.
	// A raw Config protobuf cannot distinguish another channel's membership.
	var currentBlock cb.Block
	if proto.Unmarshal(v.Current, &currentBlock) != nil {
		return Result{}, denied
	}
	previous, e := configFromBlock(&currentBlock)
	if e != nil {
		return Result{}, e
	}
	var currentEnv cb.Envelope
	var currentPayload cb.Payload
	var currentHeader cb.ChannelHeader
	if proto.Unmarshal(currentBlock.Data.Data[0], &currentEnv) != nil || proto.Unmarshal(currentEnv.Payload, &currentPayload) != nil || proto.Unmarshal(currentPayload.Header.ChannelHeader, &currentHeader) != nil || currentHeader.ChannelId != v.Channel || currentHeader.Type != int32(cb.HeaderType_CONFIG) {
		return Result{}, denied
	}
	old := *previous
	desired, e := configFromBlock(block)
	if e != nil {
		return Result{}, e
	}
	tx := configtx.New(&old)
	updated := tx.UpdatedConfig()
	for _, group := range []string{"Application", "Orderer"} {
		current := updated.ChannelGroup.Groups[group]
		target := desired.ChannelGroup.Groups[group]
		if current == nil || target == nil || current.Groups == nil {
			return Result{}, denied
		}
		for id, existing := range current.Groups {
			n := target.Groups[id]
			if n == nil || existing == nil || existing.Values["MSP"] == nil || n.Values["MSP"] == nil || !bytes.Equal(existing.Values["MSP"].Value, n.Values["MSP"].Value) {
				return Result{}, denied
			}
		}
		for id, n := range target.Groups {
			existing := current.Groups[id]
			if existing == nil {
				current.Groups[id] = proto.Clone(n).(*cb.ConfigGroup)
			} else if group == "Orderer" {
				if n.Values["Endpoints"] != nil {
					if existing.Values == nil {
						existing.Values = map[string]*cb.ConfigValue{}
					}
					if oldValue := existing.Values["Endpoints"]; oldValue != nil {
						oldValue.Value = n.Values["Endpoints"].Value
					} else {
						existing.Values["Endpoints"] = proto.Clone(n.Values["Endpoints"]).(*cb.ConfigValue)
					}
				}
			} else if group == "Application" {
				if a := n.Values["AnchorPeers"]; a != nil {
					if prev := existing.Values["AnchorPeers"]; prev != nil {
						prev.Value = a.Value
					} else {
						existing.Values["AnchorPeers"] = proto.Clone(a).(*cb.ConfigValue)
					}
				}
			}
		}
	}
	ov := updated.ChannelGroup.Groups["Orderer"].Values["ConsensusType"]
	nv := desired.ChannelGroup.Groups["Orderer"].Values["ConsensusType"]
	var oc, nc ab.ConsensusType
	var om, nm etcdraft.ConfigMetadata
	if ov == nil || nv == nil || proto.Unmarshal(ov.Value, &oc) != nil || proto.Unmarshal(nv.Value, &nc) != nil || oc.Type != "etcdraft" || nc.Type != "etcdraft" || proto.Unmarshal(oc.Metadata, &om) != nil || proto.Unmarshal(nc.Metadata, &nm) != nil {
		return Result{}, denied
	}
	if len(nm.Consenters) < len(om.Consenters) || len(nm.Consenters) > len(om.Consenters)+1 || !proto.Equal(om.Options, nm.Options) {
		return Result{}, denied
	}
	for _, o := range om.Consenters {
		found := false
		for _, n := range nm.Consenters {
			if proto.Equal(o, n) {
				found = true
			}
		}
		if !found {
			return Result{}, denied
		}
	}
	if len(nm.Consenters) > len(om.Consenters) {
		for _, n := range nm.Consenters {
			found := false
			for _, o := range om.Consenters {
				if proto.Equal(o, n) {
					found = true
				}
			}
			if !found {
				om.Consenters = append(om.Consenters, n)
			}
		}
		oc.Metadata, _ = proto.Marshal(&om)
		ov.Value, _ = proto.Marshal(&oc)
	}
	if proto.Equal(&old, updated) {
		return Result{Changed: false}, nil
	}
	update, e := tx.ComputeMarshaledUpdate(v.Channel)
	if e != nil {
		return Result{}, denied
	}
	ce := &cb.ConfigUpdateEnvelope{ConfigUpdate: update}
	data, _ := proto.Marshal(ce)
	ch, _ := proto.Marshal(&cb.ChannelHeader{Type: int32(cb.HeaderType_CONFIG_UPDATE), ChannelId: v.Channel})
	payload, _ := proto.Marshal(&cb.Payload{Header: &cb.Header{ChannelHeader: ch}, Data: data})
	env, _ := proto.Marshal(&cb.Envelope{Payload: payload})
	return Result{Update: env, Changed: true}, nil
}
func main() {
	d := json.NewDecoder(io.LimitReader(os.Stdin, 1<<20))
	d.DisallowUnknownFields()
	var v Input
	var extra any
	if d.Decode(&v) != nil || d.Decode(&extra) != io.EOF {
		fmt.Fprintln(os.Stderr, "invalid structured configuration input")
		os.Exit(1)
	}
	r, e := reconcile(v)
	if e != nil {
		fmt.Fprintln(os.Stderr, denied)
		os.Exit(1)
	}
	if json.NewEncoder(os.Stdout).Encode(r) != nil {
		os.Exit(1)
	}
}
