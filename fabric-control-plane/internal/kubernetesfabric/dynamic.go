package kubernetesfabric

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"google.golang.org/protobuf/proto"
	"sathonate.local/fabric-control-plane/internal/domain"
	"sathonate.local/fabric-control-plane/internal/localfabric"
)

// DynamicVerification is emitted only after exact readback and equal block
// hashes from every configured channel peer. Each channel/contract is distinct.
type DynamicVerification struct {
	Health         localfabric.Health `json:"health"`
	TransactionID  string             `json:"transactionId"`
	PackageID      string             `json:"packageId"`
	ArtifactSHA256 string             `json:"artifactSHA256"`
	CodeVersion    string             `json:"codeVersion"`
}

func (w *Worker) runDynamic(ctx context.Context) error {
	w.ctx = ctx
	if !w.Secure || w.Lifecycle != nil || !regexp.MustCompile(`^smp-s-[a-f0-9]{32}$`).MatchString(w.Namespace) {
		return secureDenied
	}
	runtime, e := DynamicRuntime(w.Spec, w.Runtime)
	if e != nil {
		return e
	}
	if w.Previous != nil && ValidateDynamicTransition(*w.Previous, w.Spec) != nil {
		return secureDenied
	}
	if w.Previous != nil {
		if w.Runtime.Validate(*w.Previous) != nil {
			return secureDenied
		}
		for _, c := range w.Previous.Spec.Channels {
			data, e := w.loadMaterial("channel-" + c.ID + "-genesis")
			if e != nil || data == nil {
				return secureDenied
			}
			block, e := base64.StdEncoding.DecodeString(data["block"])
			if e != nil || validateGenesis(block, c.ID) != nil {
				return secureDenied
			}
		}
		// A transition must adopt the exact existing network, never recreate a
		// missing member or silently replace its credential journal.
		for _, n := range w.Previous.Spec.Nodes {
			resource := "fabricpeers"
			if n.Role == "orderer" {
				resource = "fabricorderernodes"
			}
			v, e := w.secureGet(resource, n.ID)
			if e != nil || v == nil || text(object(object(v["metadata"])["labels"])["sathonate.io/namespace"]) != w.Namespace {
				return secureDenied
			}
			for _, role := range []string{"sign", "tls", "aux"} {
				material, e := w.loadMaterial(n.ID + "-" + role + "-identity")
				if e != nil || material == nil {
					return secureDenied
				}
			}
		}
		for _, a := range w.Previous.Spec.Configuration.Authorities {
			v, e := w.secureGet("fabriccas", a.ID)
			if e != nil || v == nil || text(object(object(v["metadata"])["labels"])["sathonate.io/namespace"]) != w.Namespace {
				return secureDenied
			}
			for _, name := range []string{a.ID + "-registry", a.ID + "-registrar-ca-identity", a.ID + "-registrar-tlsca-identity"} {
				data, e := w.loadMaterial(name)
				if e != nil || data == nil {
					return secureDenied
				}
			}
		}
	} else {
		for _, cc := range w.Spec.Spec.Configuration.Chaincodes {
			if cc.Sequence != 1 {
				return secureDenied
			}
		}
	}
	w.Runtime = runtime
	if capability, e := w.run("kubectl-hlf", "--smp-secret-input-version"); e != nil || strings.TrimSpace(string(capability)) != "smp-secret-input-v1" {
		return secureDenied
	}
	defer func() {
		entries, _ := os.ReadDir("/workspace")
		for _, f := range entries {
			_ = os.RemoveAll(filepath.Join("/workspace", f.Name()))
		}
	}()
	for _, stage := range []string{"validate", "identity", "kubernetes"} {
		if e = w.event(stage, "Validated explicit customer-defined disposable local configuration"); e != nil {
			return e
		}
	}
	if e = w.event("ca", "Reconciling protected configured authorities"); e != nil {
		return e
	}
	for _, a := range w.Spec.Spec.Configuration.Authorities {
		if e = w.secureCA(a.ID); e != nil {
			return e
		}
	}
	for _, o := range w.Spec.Spec.Organizations {
		for _, role := range []string{"channel-admin", "chaincode-admin", "client", "channel-tls"} {
			kind, caname := "admin", "ca"
			if role == "client" {
				kind = "client"
			}
			if role == "channel-tls" {
				caname = "tlsca"
			}
			if _, e = w.issueIdentity(o.ID, o.ID+"-"+role, caname, kind, ""); e != nil {
				return e
			}
		}
		ca, e := w.get("fabriccas", w.caName(w.authority(o.ID, "tlsca")))
		if e != nil {
			return e
		}
		if e = privateFile("/workspace/"+o.ID+"-root-tls.pem", []byte(text(object(ca["status"])["tlsca_cert"]))); e != nil {
			return e
		}
	}
	orgs := map[string]domain.Organization{}
	for _, o := range w.Spec.Spec.Organizations {
		orgs[o.ID] = o
	}
	for _, role := range []string{"orderer", "peer"} {
		stage := "orderers"
		if role == "peer" {
			stage = "peers"
		}
		if e = w.event(stage, "Reconciling configured members with validated immutable material"); e != nil {
			return e
		}
		for _, n := range w.Spec.Spec.Nodes {
			if n.Role == role {
				if e = w.secureNode(n, orgs[n.OrganizationRef]); e != nil {
					return e
				}
			}
		}
	}
	if e = w.event("channels", "Reconciling configured channels and canonical peer joins"); e != nil {
		return e
	}
	for _, c := range w.Spec.Spec.Channels {
		if e = w.dynamicChannel(c); e != nil {
			return e
		}
	}
	if e = w.event("verify", "Verifying configured immutable code artifacts and committed transactions"); e != nil {
		return e
	}
	for _, cc := range w.Spec.Spec.Configuration.Chaincodes {
		var channel domain.Channel
		for _, c := range w.Spec.Spec.Channels {
			if c.ID == cc.ChannelRef {
				channel = c
			}
		}
		v, e := w.dynamicContract(channel, cc)
		if e != nil {
			return e
		}
		if e = w.Output(Event{Message: "Configured contract transaction committed and read back on all channel peers", Health: &v.Health, TransactionID: v.TransactionID, Verification: &v}); e != nil {
			return e
		}
	}
	audit, e := w.argumentEvidence()
	if e != nil {
		return e
	}
	return w.Output(Event{Message: "Protected process argument inspection completed", Security: audit})
}

func (w *Worker) channelNodes(c domain.Channel, role string) []domain.Node {
	refs := c.PeerRefs
	if role == "orderer" {
		refs = c.OrdererRefs
	}
	set := map[string]bool{}
	for _, r := range refs {
		set[r] = true
	}
	var nodes []domain.Node
	for _, n := range w.Spec.Spec.Nodes {
		if n.Role == role && set[n.ID] {
			nodes = append(nodes, n)
		}
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	return nodes
}

func (w *Worker) participation(n domain.Node) (*http.Client, error) {
	a, e := w.loadMaterial(n.OrganizationRef + "-channel-tls-identity")
	if e != nil || a == nil {
		return nil, secureDenied
	}
	pair, e := tls.X509KeyPair([]byte(a["cert.pem"]), []byte(a["key.pem"]))
	if e != nil {
		return nil, secureDenied
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM([]byte(a["root.pem"])) {
		return nil, secureDenied
	}
	return &http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, Certificates: []tls.Certificate{pair}}}}, nil
}

func (w *Worker) dynamicChannel(c domain.Channel) error {
	if w.Previous != nil {
		for _, before := range w.Previous.Spec.Channels {
			if before.ID == c.ID {
				return w.reconcileDynamicChannel(before, c)
			}
		}
	}
	orders, peers := w.channelNodes(c, "orderer"), w.channelNodes(c, "peer")
	if len(orders) < 3 || len(peers) == 0 {
		return secureDenied
	}
	result, e := w.dynamicConfig(c, nil)
	if e != nil {
		return e
	}
	if validateGenesis(result.Genesis, c.ID) != nil {
		return secureDenied
	}
	// Persist the generated genesis once, before any orderer join. Retry must use
	// identical bytes, never a freshly generated nonce or block header.
	journal := "channel-" + c.ID + "-genesis"
	stored, e := w.loadMaterial(journal)
	if e != nil {
		return e
	}
	block := result.Genesis
	if stored != nil {
		block, e = base64.StdEncoding.DecodeString(stored["block"])
		if e != nil || validateGenesis(block, c.ID) != nil {
			return secureDenied
		}
	} else if e = w.materialSecret(journal, map[string]string{"block": base64.StdEncoding.EncodeToString(block)}, "issuer", true); e != nil {
		return e
	}
	for _, n := range orders {
		client, e := w.participation(n)
		if e != nil {
			return e
		}
		endpoint := "https://" + n.ID + "." + w.Namespace + ":7053/participation/v1/channels"
		req, _ := http.NewRequestWithContext(w.ctx, "GET", endpoint+"/"+c.ID, nil)
		res, e := client.Do(req)
		if e != nil {
			return secureDenied
		}
		status := res.StatusCode
		res.Body.Close()
		if status == 200 {
			continue
		}
		if status != 404 {
			return secureDenied
		}
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, e := writer.CreateFormFile("config-block", "genesis.block")
		if e != nil {
			return secureDenied
		}
		if _, e = part.Write(block); e != nil {
			return secureDenied
		}
		if writer.Close() != nil {
			return secureDenied
		}
		req, _ = http.NewRequestWithContext(w.ctx, "POST", endpoint, &body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		res, e = client.Do(req)
		if e != nil {
			return secureDenied
		}
		res.Body.Close()
		if res.StatusCode != 201 {
			return secureDenied
		}
	}
	path := "/workspace/" + c.ID + "-canonical.block"
	p := peers[0]
	o := orders[0]
	var fetchErr error
	for attempt := 0; attempt < 45; attempt++ {
		_, fetchErr = w.directPeerAt(p.OrganizationRef, "channel-admin", p.ID, "channel", "fetch", "0", path, "-c", c.ID, "-o", o.ID+"."+w.Namespace+":7050", "--tls", "--cafile", "/workspace/"+o.OrganizationRef+"-root-tls.pem")
		if fetchErr == nil {
			break
		}
		select {
		case <-w.ctx.Done():
			return w.ctx.Err()
		case <-time.After(time.Second):
		}
	}
	if fetchErr != nil {
		return fetchErr
	}
	canonical, e := os.ReadFile(path)
	var expectedBlock, canonicalBlock common.Block
	if e != nil || validateGenesis(canonical, c.ID) != nil || proto.Unmarshal(block, &expectedBlock) != nil || proto.Unmarshal(canonical, &canonicalBlock) != nil || !proto.Equal(expectedBlock.Header, canonicalBlock.Header) || !proto.Equal(expectedBlock.Data, canonicalBlock.Data) {
		return fmt.Errorf("canonical channel genesis differs from owned immutable journal")
	}
	for _, p := range peers {
		raw, e := w.directPeerAt(p.OrganizationRef, "channel-admin", p.ID, "channel", "list")
		if e != nil {
			return e
		}
		joined := false
		for _, line := range strings.Split(string(raw), "\n") {
			if strings.TrimSpace(line) == c.ID {
				joined = true
			}
		}
		if !joined {
			if _, e = w.directPeerAt(p.OrganizationRef, "channel-admin", p.ID, "channel", "join", "-b", path); e != nil {
				return e
			}
		}
	}
	return nil
}

func dynamicPolicy(p domain.Endorsement) (string, error) {
	if p.Minimum < 1 || p.Minimum > len(p.MSPIDs) {
		return "", secureDenied
	}
	ids := append([]string(nil), p.MSPIDs...)
	sort.Strings(ids)
	parts := []string{}
	for i, id := range ids {
		if !regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]{0,63}$`).MatchString(id) || i > 0 && ids[i-1] == id {
			return "", secureDenied
		}
		parts = append(parts, "'"+id+".peer'")
	}
	return fmt.Sprintf("OutOf(%d, %s)", p.Minimum, strings.Join(parts, ", ")), nil
}

func (w *Worker) dynamicContract(c domain.Channel, cc domain.Chaincode) (DynamicVerification, error) {
	fail := DynamicVerification{}
	peers := w.channelNodes(c, "peer")
	orders := w.channelNodes(c, "orderer")
	if len(peers) == 0 || len(orders) == 0 {
		return fail, secureDenied
	}
	first := peers[0]
	order := orders[0]
	policy, e := dynamicPolicy(cc.Endorsement)
	if e != nil {
		return fail, e
	}
	sequence := strconv.Itoa(cc.Sequence)
	// Service names include the channel and sequence, so upgrades never replace
	// the previous valid executable or its immutable TLS material midway.
	service := DynamicService(c.ID, cc.Name, cc.Sequence)
	previousTLS, e := w.loadMaterial(service + "-tls-identity")
	if e != nil {
		return fail, e
	}
	existingRoot := ""
	if previousTLS != nil {
		existingRoot = previousTLS["root.pem"]
	}
	issuerRoots := map[string]string{}
	for _, id := range c.OrganizationRefs {
		ca, e := w.get("fabriccas", w.caName(w.authority(id, "tlsca")))
		if e != nil {
			return fail, e
		}
		issuerRoots[id] = text(object(ca["status"])["tlsca_cert"])
	}
	issuer, e := dynamicServiceIssuer(existingRoot, issuerRoots)
	if e != nil {
		return fail, e
	}
	material, e := w.issueIdentity(issuer, service+"-tls", "tlsca", "client", service+"."+w.Namespace)
	if e != nil {
		return fail, e
	}
	secret := service + "-runtime-tls"
	if e = w.materialSecret(secret, chaincodeTLSMaterial(material), "issuer", true); e != nil {
		return fail, e
	}
	label := cc.Name + "_" + sequence
	connection, _ := json.Marshal(Object{"address": service + "." + w.Namespace + ":7052", "domain_name": service + "." + w.Namespace, "dial_timeout": "10s", "tls_required": true, "client_auth_required": false, "root_cert": material["root.pem"]})
	code, e := tarGzip(Object{"connection.json": connection})
	if e != nil {
		return fail, e
	}
	metadata, _ := json.Marshal(Object{"type": "ccaas", "label": label})
	pkg, e := tarGzip(Object{"metadata.json": metadata, "code.tar.gz": code})
	if e != nil {
		return fail, e
	}
	hash := sha256.Sum256(pkg)
	packageID := label + ":" + hex.EncodeToString(hash[:])
	path := "/workspace/" + service + ".tgz"
	if e = privateFile(path, pkg); e != nil {
		return fail, e
	}
	existing, e := w.secureGet("fabricchaincodes", service)
	if e != nil {
		return fail, e
	}
	if existing == nil {
		caName := w.caName(w.authority(issuer, "tlsca"))
		ca, e := w.get("fabriccas", caName)
		if e != nil {
			return fail, e
		}
		v := Object{"apiVersion": "hlf.kungfusoftware.es/v1alpha1", "kind": "FabricChaincode", "metadata": Object{"name": service, "namespace": w.Namespace}, "spec": Object{"image": cc.RuntimeImage, "imagePullPolicy": "IfNotPresent", "packageId": packageID, "chaincodeServerPort": 7052, "replicas": 1, "credentials": Object{"enrollid": "unused", "enrollsecret": "", "cahost": caName + "." + w.Namespace, "caport": 7054, "caname": "tlsca", "catls": Object{"cacert": base64.StdEncoding.EncodeToString([]byte(text(object(ca["status"])["tls_cert"])))}, "csr": Object{"hosts": []string{service + "." + w.Namespace}}, "external": Object{"secretName": secret, "secretNamespace": w.Namespace, "certificateKey": "tls.crt", "privateKeyKey": "tls.key", "rootCertificateKey": "tlsroot.crt"}}}}
		if e = w.apply(v); e != nil {
			return fail, e
		}
	} else if text(object(object(existing["metadata"])["labels"])["sathonate.io/namespace"]) != w.Namespace || text(object(existing["spec"])["packageId"]) != packageID || text(object(existing["spec"])["image"]) != cc.RuntimeImage {
		return fail, secureDenied
	}
	if e = w.wait("fabricchaincodes", service); e != nil {
		return fail, e
	}
	if _, e = w.kub("-n", w.Namespace, "rollout", "status", "deployment/"+service, "--timeout=180s"); e != nil {
		return fail, e
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM([]byte(material["root.pem"])) {
		return fail, secureDenied
	}
	dialer := tls.Dialer{NetDialer: &net.Dialer{Timeout: 5 * time.Second}, Config: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, ServerName: service + "." + w.Namespace}}
	ready, cancel := context.WithTimeout(w.ctx, 90*time.Second)
	defer cancel()
	for {
		conn, e := dialer.DialContext(ready, "tcp", service+"."+w.Namespace+":7052")
		if e == nil {
			conn.Close()
			break
		}
		select {
		case <-ready.Done():
			return fail, fmt.Errorf("configured chaincode TLS service unavailable")
		case <-time.After(time.Second):
		}
	}
	for _, p := range peers {
		raw, e := w.directPeerAt(p.OrganizationRef, "chaincode-admin", p.ID, "lifecycle", "chaincode", "queryinstalled", "--output", "json")
		if e != nil {
			return fail, e
		}
		var installed struct {
			Packages []struct {
				ID string `json:"package_id"`
			} `json:"installed_chaincodes"`
		}
		if json.Unmarshal(raw, &installed) != nil {
			return fail, secureDenied
		}
		found := false
		for _, item := range installed.Packages {
			if item.ID == packageID {
				found = true
			}
		}
		if !found {
			if _, e = w.directPeerAt(p.OrganizationRef, "chaincode-admin", p.ID, "lifecycle", "chaincode", "install", path); e != nil {
				return fail, e
			}
		}
	}
	definition := []string{"--channelID", c.ID, "--name", cc.Name, "--version", cc.Version, "--sequence", sequence, "--signature-policy", policy}
	ordering := []string{"-o", order.ID + "." + w.Namespace + ":7050", "--tls", "--cafile", "/workspace/" + order.OrganizationRef + "-root-tls.pem"}
	// Querying all definitions gives an empty, successful set on a new channel;
	// errors are never reinterpreted as a missing definition.
	raw, e := w.directPeerAt(first.OrganizationRef, "chaincode-admin", first.ID, "lifecycle", "chaincode", "querycommitted", "--channelID", c.ID, "--output", "json")
	if e != nil {
		return fail, e
	}
	var defs struct {
		Definitions []struct {
			Name, Version string
			Sequence      int
		} `json:"chaincode_definitions"`
	}
	if json.Unmarshal(raw, &defs) != nil {
		return fail, secureDenied
	}
	committed := false
	for _, d := range defs.Definitions {
		if d.Name == cc.Name {
			if d.Sequence > cc.Sequence || d.Sequence < cc.Sequence-1 {
				return fail, secureDenied
			}
			if d.Sequence == cc.Sequence {
				if d.Version != cc.Version {
					return fail, secureDenied
				}
				committed = true
			}
		}
	}
	approved := map[string]bool{}
	for _, p := range peers {
		if approved[p.OrganizationRef] {
			continue
		}
		approved[p.OrganizationRef] = true
		if w.Previous != nil {
			for _, oldChannel := range w.Previous.Spec.Channels {
				if oldChannel.ID == c.ID {
					for _, oldPeer := range w.channelNodes(oldChannel, "peer") {
						if oldPeer.OrganizationRef == p.OrganizationRef {
							p = oldPeer
							break
						}
					}
				}
			}
		}
		currentApproval, queryErr := w.directPeerAt(p.OrganizationRef, "chaincode-admin", p.ID, "lifecycle", "chaincode", "queryapproved", "--channelID", c.ID, "--name", cc.Name, "--sequence", sequence, "--output", "json")
		if queryErr == nil {
			currentPackage, e := approvedDynamicPackage(currentApproval)
			if e != nil || currentPackage != packageID || verifyDynamicDefinition(currentApproval, cc) != nil {
				return fail, fmt.Errorf("existing organization approval differs from the configured definition/package")
			}
			continue
		}
		if !strings.Contains(queryErr.Error(), "(approval-absent)") {
			return fail, queryErr
		}
		args := append([]string{"lifecycle", "chaincode", "approveformyorg"}, definition...)
		args = append(args, "--package-id", packageID)
		args = append(args, ordering...)
		if _, e = w.directPeerAt(p.OrganizationRef, "chaincode-admin", p.ID, args...); e != nil {
			return fail, e
		}
	}
	if !committed {
		args := append([]string{"lifecycle", "chaincode", "checkcommitreadiness"}, definition...)
		args = append(args, "--output", "json")
		raw, e = w.directPeerAt(first.OrganizationRef, "chaincode-admin", first.ID, args...)
		if e != nil {
			return fail, e
		}
		var ready struct{ Approvals map[string]bool }
		if json.Unmarshal(raw, &ready) != nil {
			return fail, secureDenied
		}
		for _, org := range c.OrganizationRefs {
			if !ready.Approvals[w.peerMSP(org)] {
				return fail, fmt.Errorf("configured channel organization approval missing")
			}
		}
		args = append([]string{"lifecycle", "chaincode", "commit"}, definition...)
		args = append(args, ordering...)
		args = append(args, "--waitForEvent", "--waitForEventTimeout", "45s")
		args = w.dynamicEndorsers(args, peers)
		if _, e = w.directPeerAt(first.OrganizationRef, "chaincode-admin", first.ID, args...); e != nil {
			return fail, e
		}
	}
	for _, p := range peers {
		raw, e := w.directPeerAt(p.OrganizationRef, "chaincode-admin", p.ID, "lifecycle", "chaincode", "querycommitted", "--channelID", c.ID, "--name", cc.Name, "--output", "json")
		if e != nil {
			return fail, e
		}
		if verifyDynamicDefinition(raw, cc) != nil {
			return fail, fmt.Errorf("committed contract definition or endorsement policy differs from configured intent")
		}
	}
	key := "verify-" + time.Now().UTC().Format("20060102150405.000000000")
	value := "verified-" + service
	invoke, _ := json.Marshal(Object{"Args": []string{"Put", key, value}})
	args := []string{"chaincode", "invoke", "-C", c.ID, "-n", cc.Name}
	args = append(args, ordering...)
	args = append(args, "--waitForEvent", "--waitForEventTimeout", "45s")
	args = w.dynamicEndorsers(args, peers)
	args = append(args, "-c", string(invoke))
	tx, e := w.directPeerAt(first.OrganizationRef, "client", first.ID, args...)
	if e != nil {
		return fail, e
	}
	txID := strings.TrimSpace(string(tx))
	if !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(txID) {
		return fail, secureDenied
	}
	artifact := w.Runtime.CodeArtifacts[cc.RuntimeImage]
	health := localfabric.Health{Ready: true, CheckedAt: time.Now().UTC(), Channel: c.ID, Peers: []localfabric.Ledger{}, Orderers: []localfabric.Consenter{}, CAs: []localfabric.CAHealth{}, Chaincode: localfabric.ContractHealth{Name: cc.Name, Available: true, Version: cc.Version, Sequence: cc.Sequence}, Alerts: []string{}}
	for _, p := range peers {
		read, cancel := context.WithTimeout(w.ctx, 45*time.Second)
		valueRaw, e := awaitLedgerReadback(read, time.Second, func() ([]byte, error) { return w.secureQueryAt(p.OrganizationRef, p.ID, c.ID, cc.Name, "Get", key) })
		cancel()
		if e != nil {
			return fail, e
		}
		if string(bytes.TrimSpace(valueRaw)) != value {
			return fail, secureDenied
		}
		version, e := w.secureQueryAt(p.OrganizationRef, p.ID, c.ID, cc.Name, "CodeVersion")
		if e != nil {
			return fail, e
		}
		if string(bytes.TrimSpace(version)) != artifact.CodeVersion {
			return fail, fmt.Errorf("live chaincode executable version differs from approved artifact")
		}
		info, e := w.secureQueryAt(p.OrganizationRef, p.ID, c.ID, "qscc", "GetChainInfo", c.ID)
		if e != nil {
			return fail, e
		}
		var ledger common.BlockchainInfo
		if proto.Unmarshal(info, &ledger) != nil || ledger.Height < 2 {
			return fail, secureDenied
		}
		health.Peers = append(health.Peers, localfabric.Ledger{Peer: p.ID, MSPID: w.peerMSP(p.OrganizationRef), Available: true, Height: ledger.Height, Hash: base64.StdEncoding.EncodeToString(ledger.CurrentBlockHash)})
	}
	for _, p := range health.Peers {
		if p.Height != health.Peers[0].Height || p.Hash != health.Peers[0].Hash {
			return fail, fmt.Errorf("configured peer ledgers have not converged")
		}
	}
	for _, o := range orders {
		client, e := w.participation(o)
		if e != nil {
			return fail, e
		}
		req, _ := http.NewRequestWithContext(w.ctx, "GET", "https://"+o.ID+"."+w.Namespace+":7053/participation/v1/channels/"+c.ID, nil)
		res, e := client.Do(req)
		if e != nil {
			return fail, secureDenied
		}
		var status struct {
			Status   string `json:"status"`
			Relation string `json:"consensusRelation"`
			Height   uint64 `json:"height"`
		}
		err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&status)
		res.Body.Close()
		if err != nil || res.StatusCode != 200 || status.Status != "active" || status.Relation != "consenter" || status.Height < health.Peers[0].Height {
			return fail, fmt.Errorf("configured orderer has not caught up")
		}
		health.Orderers = append(health.Orderers, localfabric.Consenter{Name: o.ID, Status: status.Status, Relation: status.Relation, Height: status.Height})
	}
	for _, a := range w.Spec.Spec.Configuration.Authorities {
		ca, e := w.get("fabriccas", a.ID)
		if e != nil {
			return fail, e
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(text(object(ca["status"])["tls_cert"]))) {
			return fail, secureDenied
		}
		client := http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: pool}}}
		req, _ := http.NewRequestWithContext(w.ctx, "GET", "https://"+a.ID+"."+w.Namespace+":7054/cainfo", nil)
		res, e := client.Do(req)
		if e != nil {
			return fail, secureDenied
		}
		res.Body.Close()
		if res.StatusCode != 200 {
			return fail, secureDenied
		}
		health.CAs = append(health.CAs, localfabric.CAHealth{Name: a.ID, Available: true, TLS: true})
	}
	return DynamicVerification{Health: health, TransactionID: txID, PackageID: packageID, ArtifactSHA256: cc.PackageSHA256, CodeVersion: artifact.CodeVersion}, nil
}

func (w *Worker) dynamicEndorsers(args []string, peers []domain.Node) []string {
	for _, p := range peers {
		args = append(args, "--peerAddresses", p.ID+"."+w.Namespace+":7051", "--tlsRootCertFiles", "/workspace/"+p.OrganizationRef+"-root-tls.pem")
	}
	return args
}
