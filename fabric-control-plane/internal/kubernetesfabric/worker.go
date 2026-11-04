// Package kubernetesfabric executes the pinned Bevel operator's real Fabric lifecycle.
package kubernetesfabric

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/hyperledger/fabric-protos-go-apiv2/common"
	"google.golang.org/protobuf/proto"
	"sathonate.local/fabric-control-plane/internal/domain"
	"sathonate.local/fabric-control-plane/internal/localfabric"
	"sathonate.local/fabric-control-plane/internal/redact"
	"sigs.k8s.io/yaml"
)

type Object = map[string]any
type Event struct {
	Security      *ArgumentEvidence   `json:"security,omitempty"`
	Stage         string              `json:"stage,omitempty"`
	Message       string              `json:"message,omitempty"`
	Health        *localfabric.Health `json:"health,omitempty"`
	TransactionID string              `json:"transactionId,omitempty"`
	Channel       Object              `json:"channelRequest,omitempty"`
}
type Worker struct {
	Lifecycle       *LifecycleState
	Secure          bool
	argumentSamples map[string]int
	Namespace       string
	Runtime         Runtime
	Spec            domain.NetworkSpec
	Output          func(Event) error
	ctx             context.Context
	secrets         []string
}

func (w *Worker) event(stage, message string) error {
	return w.Output(Event{Stage: stage, Message: w.redacted(message)})
}
func (w *Worker) redacted(message string) string {
	for _, secret := range w.secrets {
		message = strings.ReplaceAll(message, secret, "[REDACTED]")
	}
	return redact.Text(message)
}
func (w *Worker) run(name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(w.ctx, name, args...)
	var out, diagnostics bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &diagnostics
	if err := w.execute(cmd); err != nil {
		if w.Secure {
			return nil, fmt.Errorf("protected worker command failed; diagnostics withheld")
		}
		message := diagnostics.String() + out.String()
		for _, secret := range w.secrets {
			message = strings.ReplaceAll(message, secret, "[REDACTED]")
		}
		message = redact.Text(message)
		if len(message) > 1600 {
			message = message[:1600]
		}
		return nil, fmt.Errorf("%s failed: %s", name, message)
	}
	return out.Bytes(), nil
}
func (w *Worker) kub(args ...string) ([]byte, error) { return w.run("kubectl", args...) }
func (w *Worker) hlf(args ...string) ([]byte, error) {
	return w.protectedHLF(args)
}
func (w *Worker) apply(value any) error {
	if v, ok := value.(map[string]any); ok {
		switch text(v["kind"]) {
		case "FabricCA", "FabricPeer", "FabricOrdererNode", "FabricChaincode":
			metadata := object(v["metadata"])
			labels := object(metadata["labels"])
			labels["sathonate.io/namespace"] = w.Namespace
			metadata["labels"] = labels
			v["metadata"] = metadata
		}
	}

	if v, ok := value.(map[string]any); ok && w.Runtime.BrokerChannels && (v["kind"] == "FabricMainChannel" || v["kind"] == "FabricFollowerChannel") {
		if err := ValidateChannelRequest(v, w.Namespace, w.Spec); err != nil {
			return err
		}
		if err := w.Output(Event{Stage: "channels", Message: "Requesting scoped channel reconciliation", Channel: v}); err != nil {
			return err
		}
		resource := "fabricmainchannels"
		if v["kind"] == "FabricFollowerChannel" {
			resource = "fabricfollowerchannels"
		}
		timer := time.NewTicker(time.Second)
		defer timer.Stop()
		deadline := time.NewTimer(90 * time.Second)
		defer deadline.Stop()
		for {
			if _, err := w.get(resource, text(object(v["metadata"])["name"])); err == nil {
				return nil
			}
			select {
			case <-w.ctx.Done():
				return w.ctx.Err()
			case <-deadline.C:
				return fmt.Errorf("channel broker did not acknowledge resource; check controller events")
			case <-timer.C:
			}
		}
	}

	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(w.ctx, "kubectl", "apply", "--server-side", "--field-manager=fabric-smp-worker", "-f", "-")
	cmd.Stdin = bytes.NewReader(raw)
	var logs bytes.Buffer
	cmd.Stderr = &logs
	if err = cmd.Run(); err != nil {
		if w.Secure {
			return fmt.Errorf("protected Kubernetes apply failed; diagnostics withheld")
		}
		return fmt.Errorf("Kubernetes apply failed: %s", w.redacted(logs.String()))
	}
	return nil
}
func (w *Worker) get(resource, name string) (Object, error) {
	raw, err := w.kub("-n", w.Namespace, "get", resource, name, "-o", "json")
	if err != nil {
		return nil, err
	}
	var v Object
	err = json.Unmarshal(raw, &v)
	return v, err
}
func object(v any) Object {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return Object{}
}
func text(v any) string { s, _ := v.(string); return s }
func (w *Worker) wait(resource, name string) error {
	// Bevel can temporarily publish FAILED with an empty message while Helm is
	// starting a new workload. Wait for the actual Running condition rather than
	// turning that intermediate observation into a terminal deployment failure.
	_, err := w.kub("-n", w.Namespace, "wait", "--for=condition=Running", "--timeout=300s", resource+"/"+name)
	if err != nil {
		if value, readErr := w.get(resource, name); readErr == nil {
			message := w.redacted(text(object(value["status"])["message"]))
			if message != "" {
				return fmt.Errorf("Bevel resource %s did not become ready: %s: %w", name, message, err)
			}
		}
	}
	return err
}

func (w *Worker) render(args ...string) error {
	raw, err := w.hlf(append(args, "--output")...)
	if err != nil {
		return err
	}
	var v Object
	if err = yaml.Unmarshal(raw, &v); err != nil {
		return err
	}
	spec := object(v["spec"])
	if text(v["kind"]) == "FabricPeer" {
		name := text(object(v["metadata"])["name"])
		endpoint := name + "." + w.Namespace + ":7051"
		spec["externalEndpoint"] = endpoint
		gossip := object(spec["gossip"])
		gossip["externalEndpoint"] = endpoint
		gossip["endpoint"] = endpoint
		spec["gossip"] = gossip
	}
	if _, exists := spec["imagePullPolicy"]; exists {
		spec["imagePullPolicy"] = "IfNotPresent"
	}
	// All acceptance traffic stays on cluster DNS; no ingress or paid load balancer.
	if service := object(spec["service"]); len(service) > 0 {
		if _, ok := service["serviceType"]; ok {
			service["serviceType"] = "ClusterIP"
		}
		if _, ok := service["type"]; ok {
			service["type"] = "ClusterIP"
		}
	}
	if text(v["kind"]) == "FabricCA" {
		restrictRegistrar(v)
	}
	if err := w.adapt(v); err != nil {
		return err
	}
	if err := w.validateRenderedCredentials(v); err != nil {
		return err
	}
	return w.apply(v)
}
func (w *Worker) credential(org string) (string, error) {
	secretName := "smp-enrollment-" + org
	raw, err := w.kub("-n", w.Namespace, "get", "secret", secretName, "--ignore-not-found", "-o", "json")
	if err != nil {
		return "", err
	} // An API outage must never rotate existing credentials.
	if len(bytes.TrimSpace(raw)) > 0 {
		var value Object
		if json.Unmarshal(raw, &value) != nil {
			return "", fmt.Errorf("invalid saved enrollment secret")
		}
		if w.Secure && (text(object(object(value["metadata"])["labels"])["sathonate.io/namespace"]) != w.Namespace || text(object(object(value["metadata"])["labels"])["sathonate.io/owner"]) != "issuer" || value["immutable"] != true) {
			return "", secureDenied
		}
		encoded := text(object(value["data"])["password"])
		b, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil || len(b) < 32 {
			return "", fmt.Errorf("invalid saved enrollment secret")
		}
		w.secrets = append(w.secrets, string(b), base64.StdEncoding.EncodeToString(b))
		return string(b), nil
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	password := hex.EncodeToString(b)
	w.secrets = append(w.secrets, password, base64.StdEncoding.EncodeToString([]byte(password)))
	labels := Object{"sathonate.io/namespace": w.Namespace}
	if w.Secure {
		labels["sathonate.io/owner"] = "issuer"
	}
	err = w.apply(Object{"apiVersion": "v1", "kind": "Secret", "metadata": Object{"name": secretName, "namespace": w.Namespace, "labels": labels}, "type": "Opaque", "immutable": true, "stringData": Object{"password": password}})
	return password, err
}
func (w *Worker) register(ca, user, password, registrarPassword, kind, msp string) error {
	_, err := w.hlf("ca", "register", "--name", ca, "--namespace", w.Namespace, "--ca-url", "https://"+ca+"."+w.Namespace+":7054", "--user", user, "--secret", password, "--type", kind, "--enroll-id", "registrar", "--enroll-secret", registrarPassword, "--mspid", msp)
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "already registered") {
		return nil
	}
	return err
}
func (w *Worker) Run(ctx context.Context) (runErr error) {
	if w.Secure {
		return w.runSecure(ctx)
	}
	w.ctx = ctx
	defer func() {
		if runErr != nil {
			runErr = fmt.Errorf("%s", w.redacted(runErr.Error()))
		}
	}()

	if err := ValidateProfile(w.Spec); err != nil {
		return err
	}
	if w.Runtime.StorageClass != "" {
		if err := w.Runtime.Validate(w.Spec); err != nil {
			return err
		}
	}
	if domain.HasErrors(domain.Validate(w.Spec)) || len(w.Spec.Spec.Organizations) != 2 || len(w.Spec.Spec.Channels) != 1 || w.Spec.Spec.Ordering.Consensus != "raft" || w.Spec.Spec.FabricVersion != "2.5.15" {
		return fmt.Errorf("local acceptance profile requires Fabric 2.5.15, two organizations, one channel and Raft")
	}
	// Never fall back to a CLI that needs passwords in process arguments.
	capability, err := w.run("kubectl-hlf", "--smp-secret-input-version")
	if err != nil || strings.TrimSpace(string(capability)) != "smp-secret-input-v1" {
		return fmt.Errorf("worker requires the protected-input Bevel CLI build")
	}
	defer clearWorkspaceSecrets()
	peers := []domain.Node{}
	orderers := []domain.Node{}
	orgs := map[string]domain.Organization{}
	for _, org := range w.Spec.Spec.Organizations {
		orgs[org.ID] = org
	}
	for _, n := range w.Spec.Spec.Nodes {
		if n.Role == "peer" {
			peers = append(peers, n)
		} else {
			orderers = append(orderers, n)
		}
	}
	if len(peers) != 2 || len(orderers) != 3 || peers[0].OrganizationRef == peers[1].OrganizationRef {
		return fmt.Errorf("local acceptance profile requires one peer in each organization and three orderers")
	}
	for _, n := range orderers {
		if n.OrganizationRef != orderers[0].OrganizationRef {
			return fmt.Errorf("initial local ordering profile requires one ordering organization")
		}
	}
	if err := w.event("validate", "Validated pinned Bevel Fabric execution profile"); err != nil {
		return err
	}
	if err := w.event("identity", "Using the namespace-scoped Fabric worker service account"); err != nil {
		return err
	}
	if err := w.event("kubernetes", "Reconciling Bevel-managed Kubernetes resources"); err != nil {
		return err
	}
	nodePasswords := map[string]string{}
	if err := w.event("ca", "Starting Fabric CAs and registering node/admin identities"); err != nil {
		return err
	}
	for _, org := range w.Spec.Spec.Organizations {
		password, err := w.credential(org.ID)
		if err != nil {
			return err
		}
		ca := org.ID + "-ca"
		if _, err = w.get("fabriccas", ca); err != nil {
			if err = w.render("ca", "create", "--name", ca, "--namespace", w.Namespace, "--image", "hyperledger/fabric-ca", "--version", "1.5.15", "--storage-class", w.storageClass(), "--capacity", w.capacity(), "--enroll-id", "registrar", "--enroll-pw", password); err != nil {
				return err
			}
		}
		if err = w.wait("fabriccas", ca); err != nil {
			return err
		}
		users := []struct{ name, kind string }{{"smp-admin-v2", "admin"}}
		if org.ID == peers[0].OrganizationRef {
			users = append(users, struct{ name, kind string }{"smp-contract-v2", "client"})
		}
		for _, n := range w.Spec.Spec.Nodes {
			if n.OrganizationRef == org.ID {
				users = append(users, struct{ name, kind string }{"smp-" + n.ID + "-v2", n.Role})
			}
		}
		for _, user := range users {
			identityPassword, e := w.credential(identityCredentialKey(org.ID, user.name))
			if e != nil {
				return e
			}
			nodePasswords[org.ID+"/"+user.name] = identityPassword
			if err = w.register(ca, user.name, identityPassword, password, user.kind, org.MSPID); err != nil {
				return err
			}
		}
	}
	if err := w.event("orderers", "Enrolling and deploying three Raft orderers with Bevel Helm reconciliation"); err != nil {
		return err
	}
	for _, n := range orderers {
		org := orgs[n.OrganizationRef]
		if _, err := w.get("fabricorderernodes", n.ID); err != nil {
			if err = w.render("ordnode", "create", "--name", n.ID, "--namespace", w.Namespace, "--image", "hyperledger/fabric-orderer", "--version", "2.5.15", "--storage-class", w.storageClass(), "--capacity", w.capacity(), "--enroll-id", "smp-"+n.ID+"-v2", "--enroll-pw", nodePasswords[org.ID+"/smp-"+n.ID+"-v2"], "--mspid", org.MSPID, "--ca-name", org.ID+"-ca."+w.Namespace, "--ca-host", org.ID+"-ca."+w.Namespace, "--ca-port", "7054"); err != nil {
				return err
			}
		}
		if err := w.wait("fabricorderernodes", n.ID); err != nil {
			return err
		}
	}
	if err := w.event("peers", "Enrolling and deploying one peer in each organization"); err != nil {
		return err
	}
	for _, n := range peers {
		org := orgs[n.OrganizationRef]
		if _, err := w.get("fabricpeers", n.ID); err != nil {
			if err = w.render("peer", "create", "--name", n.ID, "--namespace", w.Namespace, "--image", "hyperledger/fabric-peer", "--version", "2.5.15", "--storage-class", w.storageClass(), "--capacity", w.capacity(), "--statedb", "leveldb", "--enroll-id", "smp-"+n.ID+"-v2", "--enroll-pw", nodePasswords[org.ID+"/smp-"+n.ID+"-v2"], "--mspid", org.MSPID, "--ca-name", org.ID+"-ca."+w.Namespace, "--ca-host", org.ID+"-ca."+w.Namespace, "--ca-port", "7054"); err != nil {
				return err
			}
		}
		if err := w.wait("fabricpeers", n.ID); err != nil {
			return err
		}
	}
	if err := w.event("channels", "Enrolling channel administrators and joining orderers/peers"); err != nil {
		return err
	}
	if err := w.channels(orgs, nodePasswords, peers, orderers); err != nil {
		return err
	}
	if err := w.event("verify", "Installing the Go smoke contract and checking a committed write on both peers"); err != nil {
		return err
	}
	return w.contract(orgs, nodePasswords, peers, orderers)
}

func (w *Worker) channels(orgs map[string]domain.Organization, passwords map[string]string, peers, orderers []domain.Node) error {
	identities := Object{}
	wallet := Object{}
	peerOrgs := []any{}
	adminPeers := []any{}
	for _, org := range w.Spec.Spec.Organizations {
		for _, kind := range []string{"ca", "tlsca"} {
			path := filepath.Join("/workspace", org.ID+"-"+kind+".yaml")
			if _, err := w.hlf("ca", "enroll", "--name", org.ID+"-ca", "--namespace", w.Namespace, "--ca-url", "https://"+org.ID+"-ca."+w.Namespace+":7054", "--user", "smp-admin-v2", "--secret", passwords[org.ID+"/smp-admin-v2"], "--mspid", org.MSPID, "--ca-name", kind, "--output", path); err != nil {
				return err
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			wallet[org.ID+"-"+kind+".yaml"] = string(raw)
		}
		sign := Object{"secretKey": org.ID + "-ca.yaml", "secretName": "smp-wallet", "secretNamespace": w.Namespace}
		tls := Object{"secretKey": org.ID + "-tlsca.yaml", "secretName": "smp-wallet", "secretNamespace": w.Namespace}
		identities[org.MSPID] = sign
		identities[org.MSPID+"-sign"] = sign
		identities[org.MSPID+"-tls"] = tls
		peerOrgs = append(peerOrgs, Object{"mspID": org.MSPID, "caName": org.ID + "-ca", "caNamespace": w.Namespace})
		adminPeers = append(adminPeers, Object{"mspID": org.MSPID})
	}
	if err := w.apply(Object{"apiVersion": "v1", "kind": "Secret", "metadata": Object{"name": "smp-wallet", "namespace": w.Namespace}, "type": "Opaque", "stringData": wallet}); err != nil {
		return err
	}
	orderingOrg := orgs[orderers[0].OrganizationRef]
	endpoints := []any{}
	nodes := []any{}
	joins := []any{}
	for _, n := range orderers {
		value, err := w.get("fabricorderernodes", n.ID)
		if err != nil {
			return err
		}
		tls := text(object(value["status"])["tlsCert"])
		if !strings.Contains(tls, "BEGIN CERTIFICATE") {
			return fmt.Errorf("missing orderer TLS certificate")
		}
		endpoints = append(endpoints, n.ID+"."+w.Namespace+":7050")
		nodes = append(nodes, Object{"host": n.ID + "." + w.Namespace, "port": 7050, "tlsCert": tls})
		joins = append(joins, Object{"host": n.ID + "." + w.Namespace, "port": 7053})
	}
	channel := w.Spec.Spec.Channels[0].ID
	main := Object{"apiVersion": "hlf.kungfusoftware.es/v1alpha1", "kind": "FabricMainChannel", "metadata": Object{"name": w.Namespace + "-" + channel, "labels": Object{"sathonate.io/namespace": w.Namespace}}, "spec": Object{"name": channel, "adminOrdererOrganizations": []any{Object{"mspID": orderingOrg.MSPID}}, "adminPeerOrganizations": adminPeers, "channelConfig": Object{"application": Object{"capabilities": []string{"V2_0"}}, "capabilities": []string{"V2_0"}, "orderer": Object{"batchSize": Object{"absoluteMaxBytes": 1048576, "maxMessageCount": 10, "preferredMaxBytes": 524288}, "batchTimeout": "2s", "capabilities": []string{"V2_0"}, "etcdRaft": Object{"options": Object{"electionTick": 10, "heartbeatTick": 1, "maxInflightBlocks": 5, "snapshotIntervalSize": 16777216, "tickInterval": "500ms"}}, "ordererType": "etcdraft", "state": "STATE_NORMAL"}}, "externalOrdererOrganizations": []any{}, "externalPeerOrganizations": []any{}, "peerOrganizations": peerOrgs, "identities": identities, "ordererOrganizations": []any{Object{"caName": orderingOrg.ID + "-ca", "caNamespace": w.Namespace, "externalOrderersToJoin": joins, "mspID": orderingOrg.MSPID, "ordererEndpoints": endpoints, "orderersToJoin": []any{}}}, "orderers": nodes}}
	if existing, err := w.get("fabricmainchannels", w.Namespace+"-"+channel); err != nil {
		if err := w.apply(main); err != nil {
			return err
		}
	} else if text(object(object(existing["metadata"])["labels"])["sathonate.io/namespace"]) != w.Namespace {
		return fmt.Errorf("channel resource belongs to another network")
	}
	if err := w.wait("fabricmainchannels", w.Namespace+"-"+channel); err != nil {
		return err
	}
	firstOrderer, _ := w.get("fabricorderernodes", orderers[0].ID)
	cert := text(object(firstOrderer["status"])["tlsCert"])
	for _, peer := range peers {
		org := orgs[peer.OrganizationRef]
		name := w.Namespace + "-" + org.ID + "-" + channel
		follower := Object{"apiVersion": "hlf.kungfusoftware.es/v1alpha1", "kind": "FabricFollowerChannel", "metadata": Object{"name": name, "labels": Object{"sathonate.io/namespace": w.Namespace}}, "spec": Object{"anchorPeers": []any{Object{"host": peer.ID + "." + w.Namespace, "port": 7051}}, "hlfIdentity": identities[org.MSPID], "mspId": org.MSPID, "name": channel, "externalPeersToJoin": []any{}, "orderers": []any{Object{"certificate": cert, "url": "grpcs://" + orderers[0].ID + "." + w.Namespace + ":7050"}}, "peersToJoin": []any{Object{"name": peer.ID, "namespace": w.Namespace}}}}
		peerResource, err := w.get("fabricpeers", peer.ID)
		if err != nil {
			return err
		}
		followerSpec := object(follower["spec"])
		followerSpec["peersToJoin"] = []any{}
		followerSpec["externalPeersToJoin"] = []any{Object{"url": "grpcs://" + peer.ID + "." + w.Namespace + ":7051", "tlsCACert": text(object(peerResource["status"])["tlsCaCert"])}}
		if existing, err := w.get("fabricfollowerchannels", name); err != nil {
			if err := w.apply(follower); err != nil {
				return err
			}
		} else if text(object(object(existing["metadata"])["labels"])["sathonate.io/namespace"]) != w.Namespace {
			return fmt.Errorf("follower resource belongs to another network")
		}
		if err := w.wait("fabricfollowerchannels", name); err != nil {
			return err
		}
	}
	return nil
}

func tarGzip(files Object) ([]byte, error) {
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	for _, name := range []string{"connection.json", "metadata.json", "code.tar.gz"} {
		v, ok := files[name]
		if !ok {
			continue
		}
		raw := v.([]byte)
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: int64(len(raw))}); err != nil {
			return nil, err
		}
		if _, err := tw.Write(raw); err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
func (w *Worker) contract(orgs map[string]domain.Organization, passwords map[string]string, peers, orderers []domain.Node) error {
	sequence, version := 1, "1.0"
	if w.Lifecycle != nil {
		sequence, version = w.Lifecycle.Sequence, w.Lifecycle.Version()
	}
	seq := fmt.Sprint(sequence)
	ca, err := w.get("fabriccas", peers[0].OrganizationRef+"-ca")
	if err != nil {
		return err
	}
	root := text(object(ca["status"])["tlsca_cert"])
	connection, _ := json.Marshal(Object{"address": "smp-smoke." + w.Namespace + ":7052", "domain_name": "smp-smoke." + w.Namespace, "dial_timeout": "10s", "tls_required": true, "client_auth_required": false, "root_cert": root})
	code, err := tarGzip(Object{"connection.json": connection})
	if err != nil {
		return err
	}
	pkg, err := tarGzip(Object{"metadata.json": []byte(`{"type":"ccaas","label":"smp_smoke_1"}`), "code.tar.gz": code})
	if err != nil {
		return err
	}
	hash := sha256.Sum256(pkg)
	packageID := "smp_smoke_1:" + hex.EncodeToString(hash[:])
	if err = os.WriteFile("/workspace/chaincode.tgz", pkg, 0600); err != nil {
		return err
	}
	profile := "/workspace/network.yaml"
	args := []string{"inspect", "--internal", "--output", profile, "--namespace", w.Namespace}
	for _, org := range w.Spec.Spec.Organizations {
		args = append(args, "-o", org.MSPID)
	}
	if w.Runtime.BrokerChannels {
		err = w.scopedProfile(profile)
	} else {
		_, err = w.hlf(args...)
	}
	if err != nil {
		return err
	}
	for _, org := range w.Spec.Spec.Organizations {
		if _, err = w.hlf("utils", "adduser", "--userPath", "/workspace/"+org.ID+"-ca.yaml", "--config", profile, "--username", "admin", "--mspid", org.MSPID); err != nil {
			return err
		}
	}
	profileBytes, err := os.ReadFile(profile)
	if err != nil {
		return err
	}
	var connectionProfile Object
	if err = yaml.Unmarshal(profileBytes, &connectionProfile); err != nil {
		return err
	}
	peerConnections := object(connectionProfile["peers"])
	for _, peer := range peers {
		peerConnections[peer.ID] = peerConnections[peer.ID+"."+w.Namespace]
	}
	profileBytes, err = yaml.Marshal(connectionProfile)
	if err != nil {
		return err
	}
	if err = os.WriteFile(profile, profileBytes, 0600); err != nil {
		return err
	}
	for _, peer := range peers {
		if err := w.event("verify", "Checking chaincode package on "+peer.ID); err != nil {
			return err
		}
		if w.Secure {
			installed, e := w.directPeerAt(peer.OrganizationRef, "chaincode-admin", peer.ID, "lifecycle", "chaincode", "queryinstalled", "--output", "json")
			if e != nil {
				return e
			}
			var packages struct {
				Installed []struct {
					ID string `json:"package_id"`
				} `json:"installed_chaincodes"`
			}
			if json.Unmarshal(installed, &packages) != nil {
				return fmt.Errorf("invalid installed package response")
			}
			found := false
			for _, pkg := range packages.Installed {
				if pkg.ID == packageID {
					found = true
				}
			}
			if !found {
				if _, e = w.directPeerAt(peer.OrganizationRef, "chaincode-admin", peer.ID, "lifecycle", "chaincode", "install", "/workspace/chaincode.tgz"); e != nil {
					return e
				}
			}
			continue
		}
		installed, queryErr := w.hlf("chaincode", "queryinstalled", "--config", profile, "--user", "admin", "--peer", peer.ID+"."+w.Namespace, "--output", "json")
		if queryErr != nil {
			return queryErr
		}
		if !bytes.Contains(installed, []byte(packageID)) {
			if _, err = w.hlf("chaincode", "install", "--path", "/workspace/chaincode.tgz", "--config", profile, "--language", "golang", "--label", "smp_smoke_1", "--user", "admin", "--peer", peer.ID+"."+w.Namespace); err != nil {
				return err
			}
		}
	}
	org := orgs[peers[0].OrganizationRef]
	if existing, getErr := w.get("fabricchaincodes", "smp-smoke"); getErr == nil {
		serviceSpec := object(existing["spec"])
		if text(serviceSpec["packageId"]) != packageID || text(serviceSpec["image"]) != w.smokeImage() {
			return fmt.Errorf("existing chaincode service does not match the expected package/image")
		}
	} else if strings.Contains(getErr.Error(), "(NotFound)") && !w.Secure {
		if _, err = w.hlf("externalchaincode", "sync", "--name", "smp-smoke", "--namespace", w.Namespace, "--image", w.smokeImage(), "--package-id", packageID, "--tls-required=true", "--ca-name", org.ID+"-ca", "--ca-namespace", w.Namespace, "--enroll-id", "smp-contract-v2", "--enroll-secret", passwords[org.ID+"/smp-contract-v2"], "--port", "7052", "--replicas", "1"); err != nil {
			return err
		}
	} else {
		return getErr
	}
	// Bevel's update command forces PullAlways. This image is deliberately local,
	// so pin its policy without replacing the running TLS service on reconciliation.
	if _, err = w.kub("-n", w.Namespace, "patch", "fabricchaincodes", "smp-smoke", "--type=merge", "-p", `{"spec":{"imagePullPolicy":"`+w.smokePolicy()+`"}}`); err != nil {
		return err
	}
	if _, err = w.kub("-n", w.Namespace, "label", "fabricchaincodes", "smp-smoke", "sathonate.io/namespace="+w.Namespace, "--overwrite"); err != nil {
		return err
	}
	if err = w.wait("fabricchaincodes", "smp-smoke"); err != nil {
		return err
	}
	if _, err = w.kub("-n", w.Namespace, "rollout", "status", "deployment/smp-smoke", "--timeout=180s"); err != nil {
		return err
	}
	// The operator's Running condition precedes service readiness. Verify a real
	// authenticated TLS handshake instead of assuming a reconciled CR is usable.
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM([]byte(root)) {
		return fmt.Errorf("missing chaincode TLS root")
	}
	service := "smp-smoke." + w.Namespace
	dialer := tls.Dialer{NetDialer: &net.Dialer{Timeout: 5 * time.Second}, Config: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, ServerName: service}}
	readyCtx, readyCancel := context.WithTimeout(w.ctx, 90*time.Second)
	defer readyCancel()
	for {
		connection, dialErr := dialer.DialContext(readyCtx, "tcp", service+":7052")
		if dialErr == nil {
			connection.Close()
			break
		}
		select {
		case <-readyCtx.Done():
			return fmt.Errorf("chaincode TLS service did not become ready")
		case <-time.After(time.Second):
		}
	}
	channel := w.Spec.Spec.Channels[0].ID
	policy, err := allPeerOrganizationsPolicy(orgs, peers)
	if err != nil {
		return err
	}
	var committed []byte
	if !w.Secure {
		committed, err = w.hlf("chaincode", "querycommitted", "--config", profile, "--user", "admin", "--peer", peers[0].ID+"."+w.Namespace, "--channel", channel, "--output", "json")
	}
	if w.Secure {
		committed, err = w.directPeer("org1", "chaincode-admin", "lifecycle", "chaincode", "querycommitted", "--channelID", channel, "--name", "smp_smoke", "--output", "json")
		if err != nil && strings.Contains(err.Error(), "definition-absent") {
			committed = []byte("[]")
			err = nil
		} else if err == nil {
			var definition struct {
				Sequence int
				Version  string
			}
			if json.Unmarshal(committed, &definition) != nil {
				return fmt.Errorf("invalid committed definition")
			}
			committed, _ = json.Marshal([]map[string]any{{"Name": "smp_smoke", "Sequence": definition.Sequence, "Version": definition.Version}})
		}
	}

	if err != nil {
		return err
	}
	alreadyCommitted, err := committedTarget(committed, sequence, version)
	if err != nil {
		return err
	}
	// These roots are also needed when a retry adopts an already committed definition.
	for _, peer := range peers {
		ca, queryErr := w.get("fabriccas", peer.OrganizationRef+"-ca")
		if queryErr != nil {
			return queryErr
		}
		if err = os.WriteFile("/workspace/"+peer.OrganizationRef+"-root-tls.pem", []byte(text(object(ca["status"])["tlsca_cert"])), 0600); err != nil {
			return err
		}
	}
	if !alreadyCommitted {
		approvedOrgs := map[string]bool{}
		for _, peer := range peers {
			if approvedOrgs[peer.OrganizationRef] {
				continue
			}
			approvedOrgs[peer.OrganizationRef] = true
			if w.Secure {
				_, e := w.directPeerAt(peer.OrganizationRef, "chaincode-admin", peer.ID, "lifecycle", "chaincode", "approveformyorg", "--channelID", channel, "--name", "smp_smoke", "--version", version, "--sequence", seq, "--package-id", packageID, "--signature-policy", policy, "-o", orderers[0].ID+"."+w.Namespace+":7050", "--tls", "--cafile", "/workspace/"+orderers[0].OrganizationRef+"-root-tls.pem")
				if e != nil {
					return e
				}
				continue
			}
			if err := w.event("verify", "Approving contract definition for "+orgs[peer.OrganizationRef].MSPID); err != nil {
				return err
			}
			approved, queryErr := w.hlf("chaincode", "queryapproved", "--config", profile, "--user", "admin", "--peer", peer.ID+"."+w.Namespace, "--channel", channel, "--chaincode", "smp_smoke", "--output", "json")
			if queryErr == nil {
				var current struct {
					PackageID string `json:"packageID"`
					Version   string `json:"version"`
					Sequence  int    `json:"sequence"`
				}
				if json.Unmarshal(approved, &current) != nil {
					return fmt.Errorf("invalid chaincode approval response")
				}
				if current.PackageID == packageID && current.Version == version && current.Sequence == sequence {
					continue
				}
				if sequence != 2 || current.Sequence != 1 || current.Version != "1.0" || current.PackageID != packageID {
					return fmt.Errorf("existing chaincode approval differs from the expected package")
				}
			}
			missing := ""
			if queryErr != nil {
				missing = strings.ToLower(queryErr.Error())
			}
			if queryErr != nil && !strings.Contains(missing, "not found") && !strings.Contains(missing, "not defined") && !strings.Contains(missing, "does not exist") && !strings.Contains(missing, "could not find") && !(strings.Contains(missing, "could not fetch approved chaincode definition") && strings.Contains(missing, "sequence: '0'")) {
				return queryErr
			}
			if _, err = w.hlf("chaincode", "approveformyorg", "--config", profile, "--user", "admin", "--peer", peer.ID+"."+w.Namespace, "--package-id", packageID, "--version", version, "--sequence", seq, "--name", "smp_smoke", "--policy", policy, "--channel", channel); err != nil {
				return err
			}
		}
		if w.Secure {
			raw, e := w.directPeer("org1", "chaincode-admin", "lifecycle", "chaincode", "checkcommitreadiness", "--channelID", channel, "--name", "smp_smoke", "--version", version, "--sequence", seq, "--signature-policy", policy, "--output", "json")
			var readiness struct{ Approvals map[string]bool }
			if e != nil || json.Unmarshal(raw, &readiness) != nil || !readiness.Approvals["Org1MSP"] || !readiness.Approvals["Org2MSP"] {
				return fmt.Errorf("both organization approvals are required before commit")
			}
		}
		if err = w.event("verify", "Committing the definition with explicit endorsements from both peer organizations"); err != nil {
			return err
		}
		cliArgs := []string{"lifecycle", "chaincode", "commit", "--channelID", channel, "--name", "smp_smoke", "--version", version, "--sequence", seq, "--signature-policy", policy, "--waitForEvent", "--waitForEventTimeout", "45s", "-o", orderers[0].ID + "." + w.Namespace + ":7050", "--tls", "--cafile", "/workspace/" + orderers[0].OrganizationRef + "-root-tls.pem"}
		for _, peer := range peers {
			ca, queryErr := w.get("fabriccas", peer.OrganizationRef+"-ca")
			if queryErr != nil {
				return queryErr
			}
			path := "/workspace/" + peer.OrganizationRef + "-root-tls.pem"
			if err = os.WriteFile(path, []byte(text(object(ca["status"])["tlsca_cert"])), 0600); err != nil {
				return err
			}
			cliArgs = append(cliArgs, "--peerAddresses", peer.ID+"."+w.Namespace+":7051", "--tlsRootCertFiles", path)
		}
		if _, err = w.fabricPeer(org, peers[0].ID, cliArgs...); err != nil {
			return err
		}
	}
	if w.Secure {
		for _, org := range []string{"org1", "org2"} {
			raw, e := w.directPeer(org, "chaincode-admin", "lifecycle", "chaincode", "querycommitted", "--channelID", channel, "--name", "smp_smoke", "--output", "json")
			if e != nil || verifyCommittedDefinition(raw, sequence, version) != nil {
				return fmt.Errorf("committed chaincode definition verification failed")
			}
		}
	}
	key := "smp-" + time.Now().UTC().Format("20060102150405.000000000")
	value := "ready-" + w.Namespace
	transactionUser := "admin"
	if w.Secure {
		transactionUser = "client"
		for _, organization := range w.Spec.Spec.Organizations {
			if _, err = w.hlf("utils", "adduser", "--userPath", "/workspace/"+organization.ID+"-client.yaml", "--config", profile, "--username", transactionUser, "--mspid", organization.MSPID); err != nil {
				return err
			}
		}
	}
	var tx []byte
	if w.Secure {
		tx, err = w.secureTransaction(channel, key, value)
	} else {
		tx, err = w.hlf("chaincode", "invoke", "--config", profile, "--user", transactionUser, "--peer", peers[0].ID+"."+w.Namespace, "--chaincode", "smp_smoke", "--channel", channel, "--fcn", "Put", "-a", key, "-a", value)
	}
	if err != nil {
		return err
	}
	txID := strings.TrimSpace(string(tx))
	if !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(txID) {
		return fmt.Errorf("no valid transaction ID returned")
	}
	health := localfabric.Health{CheckedAt: time.Now().UTC(), Channel: channel, Ready: true, Peers: []localfabric.Ledger{}, Orderers: []localfabric.Consenter{}, CAs: []localfabric.CAHealth{}, Chaincode: localfabric.ContractHealth{Name: "smp_smoke", Available: true, Version: version, Sequence: sequence}, Alerts: []string{}}
	for _, peer := range peers {
		var queried []byte
		if w.Secure {
			readCtx, cancelRead := context.WithTimeout(w.ctx, 45*time.Second)
			queried, err = awaitLedgerReadback(readCtx, time.Second, func() ([]byte, error) {
				return w.secureQueryAt(peer.OrganizationRef, peer.ID, channel, "smp_smoke", "Get", key)
			})
			cancelRead()
			queried = bytes.TrimSpace(queried)
		} else {
			queried, err = w.hlf("chaincode", "query", "--config", profile, "--user", transactionUser, "--peer", peer.ID+"."+w.Namespace, "--chaincode", "smp_smoke", "--channel", channel, "--fcn", "Get", "-a", key)
		}
		if err != nil {
			return err
		}
		if string(queried) != value {
			return fmt.Errorf("committed value not returned by %s", peer.ID)
		}
		var info []byte
		if w.Secure {
			info, err = w.secureQueryAt(peer.OrganizationRef, peer.ID, channel, "qscc", "GetChainInfo", channel)
		} else {
			info, err = w.hlf("chaincode", "query", "--config", profile, "--user", transactionUser, "--peer", peer.ID+"."+w.Namespace, "--chaincode", "qscc", "--channel", channel, "--fcn", "GetChainInfo", "-a", channel)
		}
		if err != nil {
			return err
		}
		var ledger common.BlockchainInfo
		if err = proto.Unmarshal(info, &ledger); err != nil {
			return err
		}
		health.Peers = append(health.Peers, localfabric.Ledger{Peer: peer.ID, MSPID: orgs[peer.OrganizationRef].MSPID, Available: true, Height: ledger.Height, Hash: base64.StdEncoding.EncodeToString(ledger.CurrentBlockHash)})
	}
	if health.Peers[0].Height != health.Peers[1].Height || health.Peers[0].Hash != health.Peers[1].Hash {
		return fmt.Errorf("peer ledgers have not converged")
	}
	for _, n := range orderers {
		org := orgs[n.OrganizationRef]
		raw, err := os.ReadFile("/workspace/" + org.ID + "-tlsca.yaml")
		if err != nil {
			return err
		}
		var identity Object
		if err = yaml.Unmarshal(raw, &identity); err != nil {
			return err
		}
		pair, err := tls.X509KeyPair([]byte(text(object(identity["cert"])["pem"])), []byte(text(object(identity["key"])["pem"])))
		if err != nil {
			return err
		}
		ca, err := w.get("fabriccas", org.ID+"-ca")
		if err != nil {
			return err
		}
		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM([]byte(text(object(ca["status"])["tlsca_cert"]))) {
			return fmt.Errorf("missing orderer admin TLS CA")
		}
		client := http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, Certificates: []tls.Certificate{pair}}}}
		request, err := http.NewRequestWithContext(w.ctx, "GET", "https://"+n.ID+"."+w.Namespace+":7053/participation/v1/channels/"+channel, nil)
		if err != nil {
			return err
		}
		response, err := client.Do(request)
		if err != nil {
			return fmt.Errorf("orderer administration check failed: %w", err)
		}
		var status struct {
			Status   string `json:"status"`
			Relation string `json:"consensusRelation"`
			Height   uint64 `json:"height"`
		}
		decodeErr := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&status)
		response.Body.Close()
		if response.StatusCode != 200 || decodeErr != nil || status.Status != "active" || status.Relation != "consenter" || status.Height < health.Peers[0].Height {
			return fmt.Errorf("orderer %s participation is not active and caught up", n.ID)
		}
		health.Orderers = append(health.Orderers, localfabric.Consenter{Name: n.ID, Status: status.Status, Relation: status.Relation, Height: status.Height})
	}
	for _, o := range w.Spec.Spec.Organizations {
		ca, err := w.get("fabriccas", o.ID+"-ca")
		if err != nil {
			return err
		}
		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM([]byte(text(object(ca["status"])["tls_cert"]))) {
			return fmt.Errorf("missing CA server TLS certificate")
		}
		client := http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}}}
		request, err := http.NewRequestWithContext(w.ctx, "GET", "https://"+o.ID+"-ca."+w.Namespace+":7054/cainfo", nil)
		if err != nil {
			return err
		}
		response, err := client.Do(request)
		if err != nil {
			return err
		}
		response.Body.Close()
		if response.StatusCode != 200 {
			return fmt.Errorf("CA health check failed")
		}
		health.CAs = append(health.CAs, localfabric.CAHealth{Name: o.ID + "-ca", Available: true, TLS: true})
	}
	return w.Output(Event{Message: "Real chaincode write committed and read from both peer organizations", Health: &health, TransactionID: txID})
}

func (w *Worker) fabricPeer(org domain.Organization, peer string, args ...string) ([]byte, error) {
	raw, err := os.ReadFile("/workspace/" + org.ID + "-ca.yaml")
	if err != nil {
		return nil, err
	}
	var identity Object
	if err = yaml.Unmarshal(raw, &identity); err != nil {
		return nil, err
	}
	ca, err := w.get("fabriccas", org.ID+"-ca")
	if err != nil {
		return nil, err
	}
	msp := filepath.Join("/workspace", org.ID+"-msp")
	for dir, contents := range map[string]string{"signcerts": text(object(identity["cert"])["pem"]), "keystore": text(object(identity["key"])["pem"]), "cacerts": text(object(ca["status"])["ca_cert"])} {
		if err = os.MkdirAll(filepath.Join(msp, dir), 0700); err != nil {
			return nil, err
		}
		if err = os.WriteFile(filepath.Join(msp, dir, "identity.pem"), []byte(contents), 0600); err != nil {
			return nil, err
		}
	}
	config := []byte("NodeOUs:\n  Enable: true\n  ClientOUIdentifier:\n    Certificate: cacerts/identity.pem\n    OrganizationalUnitIdentifier: client\n  PeerOUIdentifier:\n    Certificate: cacerts/identity.pem\n    OrganizationalUnitIdentifier: peer\n  AdminOUIdentifier:\n    Certificate: cacerts/identity.pem\n    OrganizationalUnitIdentifier: admin\n  OrdererOUIdentifier:\n    Certificate: cacerts/identity.pem\n    OrganizationalUnitIdentifier: orderer\n")
	if err = os.WriteFile(filepath.Join(msp, "config.yaml"), config, 0600); err != nil {
		return nil, err
	}
	binary := os.Getenv("FABRIC_PEER_BINARY")
	if binary == "" {
		binary = "/usr/local/bin/peer"
	}
	cmd := exec.CommandContext(w.ctx, binary, args...)
	cmd.Env = append(os.Environ(), "CORE_PEER_LOCALMSPID="+org.MSPID, "CORE_PEER_MSPCONFIGPATH="+msp, "CORE_PEER_ADDRESS="+peer+"."+w.Namespace+":7051", "CORE_PEER_TLS_ENABLED=true", "CORE_PEER_TLS_ROOTCERT_FILE=/workspace/"+org.ID+"-root-tls.pem")
	var out, diagnostics bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &diagnostics
	if err = w.execute(cmd); err != nil {
		if w.Secure {
			return nil, secureDenied
		}
		return nil, fmt.Errorf("official Fabric peer command failed: %s", w.redacted(diagnostics.String()))
	}
	return out.Bytes(), nil
}
