// Package localfabric runs an isolated, CA-enrolled Fabric development network.
// It does not implement remote hosting or the Bevel cloud execution contract.
package localfabric

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sathonate.local/fabric-control-plane/internal/operations"
	"sathonate.local/fabric-control-plane/internal/redact"
	"strings"
	"sync"
	"time"
)

const Version = "2.5.15"
const CAVersion = "1.5.15"
const project = "sathonate-local"

type Event struct {
	Time    time.Time `json:"time"`
	Message string    `json:"message"`
}
type Container struct {
	Name   string `json:"name"`
	State  string `json:"state"`
	Status string `json:"status"`
	Ports  string `json:"ports"`
}
type State struct {
	OperationID     string      `json:"operationId,omitempty"`
	TransactionID   string      `json:"transactionId,omitempty"`
	VerifiedAt      time.Time   `json:"verifiedAt,omitempty"`
	Phase           string      `json:"phase"`
	Error           string      `json:"error,omitempty"`
	Verified        bool        `json:"verified"`
	Ready           bool        `json:"ready"`
	Channel         string      `json:"channel"`
	Version         string      `json:"version"`
	Events          []Event     `json:"events"`
	Containers      []Container `json:"containers"`
	DockerAvailable bool        `json:"dockerAvailable"`
}
type Manager struct {
	root        string
	mu          sync.Mutex
	busy        bool
	state       State
	jobs        *operations.Store
	recordError error
}

func New(root string) (*Manager, error) {
	root, e := filepath.Abs(root)
	if e != nil {
		return nil, e
	}
	if e = os.MkdirAll(root, 0700); e != nil {
		return nil, e
	}
	m := &Manager{root: root, state: State{Phase: "not-created", Channel: "sathonate", Version: Version, Events: []Event{}, Containers: []Container{}}}
	if b, e := os.ReadFile(filepath.Join(root, "state.json")); e == nil {
		if e = json.Unmarshal(b, &m.state); e != nil {
			return nil, errors.New("invalid local runtime state")
		}
		if m.state.Phase == "starting" || m.state.Phase == "stopping" || m.state.Phase == "verifying" {
			m.state.Phase = "interrupted"
			m.state.Error = "The API stopped during an operation. Start again to reconcile existing resources."
			m.state.Verified = false
		}
	}
	m.jobs, e = operations.Open(filepath.Join(root, "operations"))
	if e != nil {
		return nil, e
	}
	return m, nil
}
func (m *Manager) persistLocked() error {
	b, e := json.MarshalIndent(m.state, "", "  ")
	if e != nil {
		return e
	}
	p := filepath.Join(m.root, "state.json")
	if e = os.WriteFile(p+".tmp", b, 0600); e != nil {
		return e
	}
	return os.Rename(p+".tmp", p)
}
func (m *Manager) event(message string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	message = redact.Text(message)
	m.state.Events = append(m.state.Events, Event{time.Now().UTC(), message})
	if m.state.OperationID != "" {
		if e := m.jobs.Event(m.state.OperationID, message); e != nil {
			m.recordError = errors.New("operation event could not be persisted")
		}
	}
	if len(m.state.Events) > 150 {
		m.state.Events = m.state.Events[len(m.state.Events)-150:]
	}
	_ = m.persistLocked()
}
func (m *Manager) Snapshot(ctx context.Context) State {
	m.mu.Lock()
	s := m.state
	s.Events = append([]Event{}, m.state.Events...)
	m.mu.Unlock()
	s.Containers = []Container{}
	s.Ready = false
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	b, e := docker(ctx, "ps", "-a", "--filter", "label=com.docker.compose.project="+project, "--format", "{{json .}}")
	if e != nil {
		s.DockerAvailable = false
		return s
	}
	s.DockerAvailable = true
	running := 0
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		var row struct{ Names, State, Status, Ports string }
		if json.Unmarshal([]byte(line), &row) != nil {
			continue
		}
		s.Containers = append(s.Containers, Container{row.Names, row.State, row.Status, row.Ports})
		if row.State == "running" {
			running++
		}
	}
	s.Ready = s.Verified && running == 10 && len(s.Containers) == 10 && s.Phase == "running"
	return s
}
func (m *Manager) Start() error                  { _, e := m.Run("start", "", "", "local-developer"); return e }
func (m *Manager) Stop() error                   { _, e := m.Run("stop", "", "", "local-developer"); return e }
func (m *Manager) Test() error                   { _, e := m.Run("verify", "", "", "local-developer"); return e }
func (m *Manager) Operations() *operations.Store { return m.jobs }
func (m *Manager) Run(kind, key, retryOf, actor string) (operations.Operation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	stages := []operations.Stage{}
	switch kind {
	case "start":
		for _, pair := range [][2]string{{"validate", "Validate local runtime"}, {"images", "Fetch official Fabric images"}, {"identities", "Enroll Fabric CA identities"}, {"nodes", "Start peers and Raft orderers"}, {"channel", "Create and join channel"}, {"chaincode", "Install and commit Go contract"}, {"verify", "Verify endorsed transaction"}} {
			stages = append(stages, operations.Stage{Key: pair[0], Label: pair[1]})
		}
	case "stop":
		stages = []operations.Stage{{Key: "stop", Label: "Stop containers and retain ledger data"}}
	case "verify":
		stages = []operations.Stage{{Key: "verify", Label: "Verify endorsed transaction"}}
	default:
		return operations.Operation{}, errors.New("unsupported local operation")
	}
	b, _ := json.Marshal(configtx())
	sum := sha256.Sum256(append(b, []byte(Version+"/"+CAVersion)...))
	digest := hex.EncodeToString(sum[:])
	// Check replays before mutable runtime prerequisites: retries of the same HTTP request must return the same operation.
	if key != "" {
		for _, old := range m.jobs.List("local-development") {
			if old.IdempotencyKey == key {
				if old.Kind != kind || old.ProfileDigest != digest || old.RetryOf != retryOf {
					return operations.Operation{}, operations.ErrIdempotency
				}
				return old, nil
			}
		}
	}
	if m.busy {
		return operations.Operation{}, operations.ErrConflict
	}
	if kind == "verify" && m.state.Phase != "running" && retryOf == "" {
		return operations.Operation{}, errors.New("start the network and wait for readiness first")
	}
	op, existing, e := m.jobs.Begin("local-development", actor, project, kind, key, digest, "fabric-"+Version+"/ca-"+CAVersion, retryOf, stages)
	if e != nil {
		return operations.Operation{}, e
	}
	if existing {
		return op, nil
	}
	previous := m.state
	m.busy = true
	m.recordError = nil
	m.state.OperationID = op.ID
	m.state.Error = ""
	m.state.Verified = false
	switch kind {
	case "start":
		m.state.Phase = "starting"
	case "stop":
		m.state.Phase = "stopping"
	case "verify":
		m.state.Phase = "verifying"
	}
	if e = m.persistLocked(); e != nil {
		m.busy = false
		m.state = previous
		_ = m.jobs.Finish(op.ID, errors.New("runtime state could not be persisted"))
		return operations.Operation{}, e
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
		defer cancel()
		var runError error
		switch kind {
		case "start":
			runError = m.up(ctx)
		case "stop":
			runError = m.stage("stop")
			if runError == nil {
				m.event("Stopping only the Fabric SMP local project; identities and ledgers are retained")
				_, runError = m.compose(ctx, "stop")
				if runError != nil {
					runError = errors.New("Docker stop failed; inspect node logs and retry")
				}
			}
		case "verify":
			runError = m.stage("verify")
			if runError == nil {
				runError = m.smoke(ctx)
			}
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		m.busy = false
		if runError == nil && m.recordError != nil {
			runError = m.recordError
		}
		if runError != nil {
			runError = errors.New(redact.Text(runError.Error()))
		}
		if e := m.jobs.Finish(op.ID, runError); e != nil {
			runError = errors.New("operation journal could not be finalized; resources may have changed")
		}
		if runError != nil {
			m.state.Phase = "failed"
			m.state.Error = runError.Error()
			m.state.Verified = false
		} else if kind == "stop" {
			m.state.Phase = "stopped"
		} else {
			m.state.Phase = "running"
			m.state.Verified = true
		}
		_ = m.persistLocked()
	}()
	return op, nil
}
func (m *Manager) stage(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.recordError != nil {
		return m.recordError
	}
	return m.jobs.Advance(m.state.OperationID, key)
}
func docker(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Env = append(os.Environ(), "MSYS_NO_PATHCONV=1")
	b, e := cmd.CombinedOutput()
	return b, e
}
func (m *Manager) compose(ctx context.Context, args ...string) ([]byte, error) {
	base := []string{"compose", "--project-name", project, "--file", filepath.Join(m.root, "compose.json")}
	return docker(ctx, append(base, args...)...)
}
func (m *Manager) write(name string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	return os.WriteFile(filepath.Join(m.root, name), b, 0600)
}
func secret() string {
	b := make([]byte, 24)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func (m *Manager) tool(ctx context.Context, args ...string) ([]byte, error) {
	return m.compose(ctx, append([]string{"exec", "-T", "tools"}, args...)...)
}
func (m *Manager) client(ctx context.Context, org string, args ...string) error {
	config, publicArgs, e := protectedCAClientConfig(args)
	if e != nil {
		return e
	}
	dir, e := os.MkdirTemp(m.root, "ca-input-")
	if e != nil {
		return errors.New("cannot prepare private CA input")
	}
	defer os.RemoveAll(dir)
	if e = protectPrivateDirectory(dir); e != nil {
		return errors.New("cannot protect CA input directory")
	}
	config["mspdir"] = "/work/organizations/" + org + "/registrar/msp"
	raw, e := json.Marshal(config)
	if e != nil {
		return errors.New("cannot encode private CA input")
	}
	if e = os.WriteFile(filepath.Join(dir, "client.json"), raw, 0600); e != nil {
		return errors.New("cannot write private CA input")
	}
	base := []string{"run", "--rm", "--network", project, "--mount", "type=bind,source=" + m.root + ",target=/work", "--mount", "type=bind,source=" + dir + ",target=/secret,readonly", "-e", "FABRIC_CA_CLIENT_HOME=/work/organizations/" + org + "/registrar", "--tmpfs", "/secret-run:rw,noexec,nosuid,size=1m", "--entrypoint", "sh", "hyperledger/fabric-ca:" + CAVersion, "-c", `umask 077; cp /secret/client.json /secret-run/client.json; exec fabric-ca-client "$@"`, "fabric-ca-client"}
	base = append(base, publicArgs...)
	base = append(base, "--config", "/secret-run/client.json", "--tls.certfiles", "/work/cas/"+org+"/tls-cert.pem")
	if _, e = docker(ctx, base...); e != nil {
		return errors.New("Fabric CA client failed; private diagnostics withheld")
	}
	return nil
}
func (m *Manager) Logs(ctx context.Context, node string) (string, error) {
	allowed := map[string]bool{"peer1": true, "peer2": true, "orderer1": true, "orderer2": true, "orderer3": true}
	if !allowed[node] {
		return "", errors.New("unknown peer/orderer")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	b, e := m.compose(ctx, "logs", "--no-color", "--tail", "80", node)
	return redact.Text(string(b)), e
}

func (m *Manager) up(ctx context.Context) error {
	if e := m.stage("validate"); e != nil {
		return e
	}
	m.event("Checking Linux Docker engine and Compose")
	if b, e := docker(ctx, "info", "--format", "{{.OSType}}"); e != nil || strings.TrimSpace(string(b)) != "linux" {
		return errors.New("start Docker Desktop with the Linux engine, then retry")
	}
	if e := protectPrivateDirectory(m.root); e != nil {
		return errors.New("cannot restrict private Fabric runtime directory")
	}
	passwords := map[string]string{}
	path := filepath.Join(m.root, "bootstrap.json")
	if b, e := os.ReadFile(path); e == nil {
		if json.Unmarshal(b, &passwords) != nil {
			return errors.New("invalid local CA bootstrap state")
		}
	} else {
		for _, o := range []string{"orderers", "org1", "org2"} {
			passwords[o] = secret()
		}
		if e = m.write("bootstrap.json", passwords); e != nil {
			return e
		}
	}
	for _, o := range []string{"orderers", "org1", "org2"} {
		if passwords[o] == "" {
			return errors.New("missing persisted CA bootstrap secret")
		}
		if e := os.MkdirAll(filepath.Join(m.root, "cas", o), 0700); e != nil {
			return e
		}
		if e := m.write(filepath.Join("cas", o, "smp-bootstrap.json"), caBootstrap(passwords[o])); e != nil {
			return e
		}
	}
	if e := m.write("compose.json", compose(m.root, passwords)); e != nil {
		return e
	}
	if e := m.write("configtx.yaml", configtx()); e != nil {
		return e
	}
	if e := m.stage("images"); e != nil {
		return e
	}
	m.event("Pulling official Fabric peer/orderer/tools 2.5.15 and Fabric CA 1.5.15 images")
	if _, e := m.compose(ctx, "pull"); e != nil {
		return errors.New("image pull failed; check Docker registry connectivity")
	}
	if e := m.stage("identities"); e != nil {
		return e
	}
	m.event("Starting the ordering CA and two organization CAs with TLS")
	if _, e := m.compose(ctx, "up", "-d", "ca-orderers", "ca-org1", "ca-org2"); e != nil {
		return errors.New("could not start Fabric CA containers")
	}
	for _, o := range []string{"orderers", "org1", "org2"} {
		m.event("Enrolling the " + o + " CA registrar")
		if e := retry(ctx, 25, func() error {
			return m.client(ctx, o, "enroll", "-u", "https://registrar:"+passwords[o]+"@ca-"+o+":7054")
		}); e != nil {
			return fmt.Errorf("%s CA registrar enrollment failed; check Docker CA logs locally", o)
		}
		ids := []struct{ name, kind string }{{"admin", "admin"}}
		if o == "orderers" {
			for i := 1; i <= 3; i++ {
				ids = append(ids, struct{ name, kind string }{fmt.Sprintf("orderer%d", i), "orderer"})
			}
		} else {
			n := "peer1"
			if o == "org2" {
				n = "peer2"
			}
			ids = append(ids, struct{ name, kind string }{n, "peer"})
			if o == "org1" {
				ids = append(ids, struct{ name, kind string }{"contract", "client"})
			}
		}
		for _, id := range ids {
			base := filepath.Join(m.root, "organizations", o, id.name)
			if _, e := os.Stat(filepath.Join(base, "tls", "signcerts", "cert.pem")); e == nil {
				continue
			}
			// Persist independently random identity credentials before registration.
			identityKey := o + "/" + id.name
			enrollment := passwords[identityKey]
			if enrollment == "" {
				enrollment = secret()
				passwords[identityKey] = enrollment
				if e := m.write("bootstrap.json", passwords); e != nil {
					return e
				}
			}
			reg := m.client(ctx, o, "register", "--id.name", id.name, "--id.secret", enrollment, "--id.type", id.kind)
			// A previous interrupted attempt may already have registered this identity; enrollment is authoritative.
			_ = reg
			m.event("Enrolling " + o + "/" + id.name + " MSP and TLS identities through Fabric CA")
			if e := m.client(ctx, o, "enroll", "-u", "https://"+id.name+":"+enrollment+"@ca-"+o+":7054", "-M", "/work/organizations/"+o+"/"+id.name+"/msp"); e != nil {
				return fmt.Errorf("MSP enrollment failed for %s/%s", o, id.name)
			}
			if e := m.client(ctx, o, "enroll", "-u", "https://"+id.name+":"+enrollment+"@ca-"+o+":7054", "--enrollment.profile", "tls", "--csr.hosts", id.name+",localhost", "-M", "/work/organizations/"+o+"/"+id.name+"/tls"); e != nil {
				return fmt.Errorf("TLS enrollment failed for %s/%s", o, id.name)
			}
		}
		if e := m.normalize(o, ids); e != nil {
			return e
		}
	}
	if e := m.stage("nodes"); e != nil {
		return e
	}
	m.event("Starting three Raft orderers, two peers and the Fabric CLI container")
	if _, e := m.compose(ctx, "up", "-d", "orderer1", "orderer2", "orderer3", "peer1", "peer2", "tools"); e != nil {
		return errors.New("Fabric node startup failed; check node logs")
	}
	if e := m.stage("channel"); e != nil {
		return e
	}
	m.event("Generating channel configuration with the official configtxgen binary")
	if _, e := os.Stat(filepath.Join(m.root, "sathonate.block")); os.IsNotExist(e) {
		if b, e := m.tool(ctx, "configtxgen", "-profile", "Sathonate", "-channelID", "sathonate", "-outputBlock", "/work/sathonate.block"); e != nil {
			return fmt.Errorf("configtxgen failed: %.1800s", b)
		}
	}
	for i := 1; i <= 3; i++ {
		node := fmt.Sprintf("orderer%d", i)
		m.event("Joining " + node + " to sathonate via the osnadmin participation API")
		base := []string{"osnadmin", "channel", "join", "--channelID", "sathonate", "--config-block", "/work/sathonate.block", "-o", node + ":7053", "--ca-file", "/work/organizations/orderers/" + node + "/tls/ca.crt", "--client-cert", "/work/organizations/orderers/admin/tls/server.crt", "--client-key", "/work/organizations/orderers/admin/tls/server.key"}
		if e := retry(ctx, 20, func() error {
			b, e := m.tool(ctx, base...)
			if e == nil && strings.Contains(string(b), "201") {
				return nil
			}
			b, e = m.tool(ctx, "osnadmin", "channel", "list", "--channelID", "sathonate", "-o", node+":7053", "--ca-file", "/work/organizations/orderers/"+node+"/tls/ca.crt", "--client-cert", "/work/organizations/orderers/admin/tls/server.crt", "--client-key", "/work/organizations/orderers/admin/tls/server.key")
			if e == nil && strings.Contains(string(b), `"name": "sathonate"`) {
				return nil
			}
			return errors.New("orderer not joined")
		}); e != nil {
			return fmt.Errorf("channel join failed for %s", node)
		}
	}
	for i := 1; i <= 2; i++ {
		m.event(fmt.Sprintf("Joining peer%d and verifying its channel ledger", i))
		if e := retry(ctx, 25, func() error {
			if _, e := m.peer(ctx, i, "channel", "getinfo", "-c", "sathonate"); e == nil {
				return nil
			}
			if _, e := m.peer(ctx, i, "channel", "join", "-b", "/work/sathonate.block"); e != nil {
				return e
			}
			_, e := m.peer(ctx, i, "channel", "getinfo", "-c", "sathonate")
			return e
		}); e != nil {
			return fmt.Errorf("peer%d channel verification failed", i)
		}
	}
	if e := m.stage("chaincode"); e != nil {
		return e
	}
	if e := m.deployContract(ctx, passwords); e != nil {
		return e
	}
	m.event("Verified: both peer ledgers, three Raft consenters, and a committed Go contract write queried on both peers. Local network is ready.")
	return nil
}
func retry(ctx context.Context, n int, f func() error) error {
	var e error
	for i := 0; i < n; i++ {
		if e = f(); e == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return e
}
func (m *Manager) peer(ctx context.Context, i int, args ...string) ([]byte, error) {
	org := fmt.Sprintf("org%d", i)
	node := fmt.Sprintf("peer%d", i)
	base := []string{"exec", "-T", "-e", "FABRIC_CFG_PATH=/etc/hyperledger/fabric", "-e", "CORE_PEER_LOCALMSPID=" + fmt.Sprintf("Org%dMSP", i), "-e", "CORE_PEER_MSPCONFIGPATH=/work/organizations/" + org + "/admin/msp", "-e", "CORE_PEER_ADDRESS=" + node + ":7051", "-e", "CORE_PEER_TLS_ENABLED=true", "-e", "CORE_PEER_TLS_ROOTCERT_FILE=/work/organizations/" + org + "/" + node + "/tls/ca.crt", "tools", "peer"}
	return m.compose(ctx, append(base, args...)...)
}
