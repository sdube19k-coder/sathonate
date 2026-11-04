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
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sathonate.local/fabric-control-plane/internal/domain"
	"sigs.k8s.io/yaml"
	"sort"
	"strings"
	"time"
)

var secureDenied = errors.New("secure issuer ownership, trust or protected operation failed; private diagnostics withheld")

func securePeerFailure(raw []byte) error {
	message := strings.ToLower(string(raw))
	category := "unclassified"
	for _, candidate := range []struct{ marker, category string }{
		{"external builder", "chaincode-builder"},
		{"already successfully installed", "package-already-installed"},
		{"chaincode is already installed", "package-already-installed"},
		{"could not get chaincode package", "package-unavailable"},
		{"identity is not valid", "msp-identity"},
		{"signature is not valid", "msp-signature"},
		{"could not find chaincode", "definition-absent"},
		{"chaincode is not defined", "definition-absent"},
		{"namespace smp_smoke is not defined", "definition-absent"},
		{"certificate signed by unknown authority", "tls-trust"},
		{"certificate is valid for", "tls-hostname"},
		{"connection refused", "endpoint-unavailable"},
		{"deadline exceeded", "timeout"},
		{"access denied", "authorization"},
		{"authorization failure", "authorization"},
		{"chaincode registration failed", "chaincode-service"},
		{"key not found", "ledger-value-unavailable"},
		{"endorsement failure", "endorsement"},
	} {
		if strings.Contains(message, candidate.marker) {
			category = candidate.category
			break
		}
	}
	return fmt.Errorf("protected Fabric peer command failed (%s); private diagnostics withheld", category)
}

func yamlUnmarshal(raw []byte, v any) error { return yaml.Unmarshal(raw, v) }
func securePackageID(raw []byte) string {
	sum := sha256.Sum256(raw)
	return "smp_smoke_1:" + hex.EncodeToString(sum[:])
}
func (w *Worker) secureGet(resource, name string) (Object, error) {
	raw, e := w.kub("-n", w.Namespace, "get", resource, name, "--ignore-not-found", "-o", "json")
	if e != nil {
		return nil, secureDenied
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil
	}
	var v Object
	if json.Unmarshal(raw, &v) != nil {
		return nil, secureDenied
	}
	return v, nil
}
func (w *Worker) loadMaterial(name string) (map[string]string, error) {
	v, e := w.secureGet("secret", name)
	if e != nil || v == nil {
		return nil, e
	}
	if text(object(object(v["metadata"])["labels"])["sathonate.io/namespace"]) != w.Namespace || v["immutable"] != true || text(object(object(v["metadata"])["labels"])["sathonate.io/owner"]) != "issuer" {
		return nil, secureDenied
	}
	data := map[string]string{}
	for key, value := range object(v["data"]) {
		b, e := base64.StdEncoding.DecodeString(text(value))
		if e != nil {
			return nil, secureDenied
		}
		data[key] = string(b)
	}
	return data, nil
}
func (w *Worker) materialSecret(name string, data map[string]string, owner string, immutable bool) error {
	existing, e := w.secureGet("secret", name)
	if e != nil {
		return e
	}
	if existing != nil {
		labels := object(object(existing["metadata"])["labels"])
		if text(labels["sathonate.io/namespace"]) != w.Namespace || text(labels["sathonate.io/owner"]) != owner {
			return secureDenied
		}
		actual := map[string]string{}
		for key, value := range object(existing["data"]) {
			b, e := base64.StdEncoding.DecodeString(text(value))
			if e != nil {
				return secureDenied
			}
			actual[key] = string(b)
		}
		if !reflect.DeepEqual(actual, data) || immutable && existing["immutable"] != true {
			return secureDenied
		}
		return nil
	}
	return w.apply(Object{"apiVersion": "v1", "kind": "Secret", "metadata": Object{"name": name, "namespace": w.Namespace, "labels": Object{"sathonate.io/namespace": w.Namespace, "sathonate.io/owner": owner}}, "immutable": immutable, "stringData": data})
}
func privateFile(path string, raw []byte) error {
	if !strings.HasPrefix(path, "/workspace/") || strings.Contains(path, "..") {
		return secureDenied
	}
	if os.MkdirAll(filepath.Dir(path), 0700) != nil {
		return secureDenied
	}
	if os.WriteFile(path, raw, 0600) != nil {
		return secureDenied
	}
	return nil
}
func (w *Worker) caCall(key, org, command, registrar string, config Object) error {
	raw, e := json.Marshal(config)
	if e != nil {
		return secureDenied
	}
	path := "/workspace/" + key + ".json"
	if privateFile(path, raw) != nil {
		return secureDenied
	}
	defer os.Remove(path)
	msp := key
	if registrar != "" {
		msp = registrar
	}
	cmd := exec.CommandContext(w.ctx, "fabric-ca-client", command, "--config", path, "--mspdir", "/workspace/"+msp+"/msp", "--tls.certfiles", "/workspace/"+org+"-server.pem")
	// CA enrollment/registration diagnostics can contain usable credentials.
	cmd.Env = append(os.Environ(), "FABRIC_CA_CLIENT_HOME=/workspace")
	var diagnostic bytes.Buffer
	cmd.Stdout = &diagnostic
	cmd.Stderr = &diagnostic
	if w.execute(cmd) != nil {
		return fmt.Errorf("protected CA %s failed (%s); private diagnostics withheld", command, caFailureCategory(diagnostic.Bytes()))
	}
	return nil
}
func readMSP(key string) (map[string]string, error) {
	root := "/workspace/" + key + "/msp/"
	files, e := filepath.Glob(root + "keystore/*")
	if e != nil || len(files) != 1 {
		return nil, secureDenied
	}
	keys := map[string]string{"cert.pem": root + "signcerts/cert.pem", "key.pem": files[0]}
	rootDir := "cacerts"
	if _, e := os.Stat(root + "tlscacerts"); e == nil {
		rootDir = "tlscacerts"
	}
	roots, e := filepath.Glob(root + rootDir + "/*")
	if e != nil || len(roots) != 1 {
		return nil, secureDenied
	}
	keys["root.pem"] = roots[0]
	data := map[string]string{}
	for name, path := range keys {
		b, e := os.ReadFile(path)
		if e != nil {
			return nil, secureDenied
		}
		data[name] = string(b)
	}
	return data, nil
}
func (w *Worker) restoreMSP(key string, data map[string]string) error {
	for dir, name := range map[string]string{"signcerts": "cert.pem", "keystore": "key.pem", "cacerts": "root.pem"} {
		filename := "identity.pem"
		if dir == "signcerts" {
			filename = restoredSigningCertificate
		}
		if privateFile("/workspace/"+key+"/msp/"+dir+"/"+filename, []byte(data[name])) != nil {
			return secureDenied
		}
	}
	config := []byte("NodeOUs:\n  Enable: true\n  ClientOUIdentifier:\n    Certificate: cacerts/identity.pem\n    OrganizationalUnitIdentifier: client\n  PeerOUIdentifier:\n    Certificate: cacerts/identity.pem\n    OrganizationalUnitIdentifier: peer\n  AdminOUIdentifier:\n    Certificate: cacerts/identity.pem\n    OrganizationalUnitIdentifier: admin\n  OrdererOUIdentifier:\n    Certificate: cacerts/identity.pem\n    OrganizationalUnitIdentifier: orderer\n")
	return privateFile("/workspace/"+key+"/msp/config.yaml", config)
}

// Fabric CA Client.Init/LoadMyIdentity requires this exact basename, unlike
// peer SDK MSP loaders which accept any PEM filename in signcerts.
const restoredSigningCertificate = "cert.pem"

func (w *Worker) validateIdentity(key, dns string, data map[string]string, expectedRoot, commonName, role string) error {
	if err := validateAuthorizedMaterial(w.Namespace, key, dns, commonName, role, expectedRoot, data, w.Runtime.DeniedCertificateSHA256, w.Runtime.DeniedPublicKeySHA256); err != nil {
		return err
	}
	w.secrets = append(w.secrets, data["key.pem"], base64.StdEncoding.EncodeToString([]byte(data["key.pem"])))
	return w.restoreMSP(key, data)
}
func (w *Worker) issueIdentity(org, key, caname, kind, dns string) (map[string]string, error) {
	if kind != "peer" && kind != "orderer" && kind != "client" && kind != "admin" {
		return nil, secureDenied
	}
	ca, err := w.secureGet("fabriccas", org+"-ca")
	if err != nil || ca == nil || text(object(object(ca["metadata"])["labels"])["sathonate.io/namespace"]) != w.Namespace {
		return nil, secureDenied
	}
	rootField := "ca_cert"
	if caname == "tlsca" {
		rootField = "tlsca_cert"
	} else if caname != "ca" {
		return nil, secureDenied
	}
	expectedRoot := text(object(ca["status"])[rootField])
	data, e := w.loadMaterial(key + "-identity")
	if e != nil {
		return nil, e
	}
	if data != nil {
		return data, w.validateIdentity(key, dns, data, expectedRoot, key, kind)
	}
	// Persist random registration credentials before registration, so ambiguous CA
	// outcomes never silently change identity credentials on retry.
	credential, e := w.credential("secure-" + key)
	if e != nil {
		return nil, e
	}
	endpoint := "https://" + org + "-ca." + w.Namespace + ":7054"
	config := Object{"url": endpoint, "caname": caname, "id": Object{"name": key, "secret": credential, "type": kind}}
	// A repeated registration is inspected through fixed semantic output only.
	raw, _ := json.Marshal(config)
	path := "/workspace/" + key + ".json"
	if privateFile(path, raw) != nil {
		return nil, secureDenied
	}
	cmd := exec.CommandContext(w.ctx, "fabric-ca-client", "register", "--config", path, "--mspdir", "/workspace/"+org+"-registrar-"+caname+"/msp", "--tls.certfiles", "/workspace/"+org+"-server.pem")
	var diagnostic bytes.Buffer
	cmd.Env = append(os.Environ(), "FABRIC_CA_CLIENT_HOME=/workspace")
	cmd.Stdout = &diagnostic
	cmd.Stderr = &diagnostic
	e = w.execute(cmd)
	os.Remove(path)
	if e != nil && !strings.Contains(strings.ToLower(diagnostic.String()), "already registered") {
		return nil, fmt.Errorf("protected identity registration failed (%s); private diagnostics withheld", caFailureCategory(diagnostic.Bytes()))
	}
	config = Object{"url": "https://" + key + ":" + credential + "@" + org + "-ca." + w.Namespace + ":7054", "caname": caname}
	if caname == "tlsca" {
		hosts := []string{"localhost"}
		if dns != "" {
			hosts = []string{strings.TrimSuffix(dns, "."+w.Namespace), dns, "localhost"}
		}
		config["enrollment"] = Object{"profile": "tls"}
		config["csr"] = Object{"hosts": hosts}
	}
	if err := w.caCall(key, org, "enroll", "", config); err != nil {
		return nil, err
	}
	data, e = readMSP(key)
	if e != nil {
		return nil, fmt.Errorf("protected identity MSP read failed; private diagnostics withheld")
	}
	if w.validateIdentity(key, dns, data, expectedRoot, key, kind) != nil {
		return nil, fmt.Errorf("protected identity certificate authorization failed; private diagnostics withheld")
	}
	if w.materialSecret(key+"-identity", data, "issuer", true) != nil {
		return nil, secureDenied
	}
	return data, nil
}
func (w *Worker) reference(name string) Object {
	return Object{"secretName": name, "secretNamespace": w.Namespace, "certificateKey": "cert.pem", "privateKeyKey": "key.pem", "rootCertificateKey": "root.pem"}
}

// The qualified chaincode workload mounts these filenames directly. Node
// references use a different mapping and must never be reused for this chart.
func chaincodeTLSMaterial(data map[string]string) map[string]string {
	return map[string]string{"tls.crt": data["cert.pem"], "tls.key": data["key.pem"], "tlsroot.crt": data["root.pem"]}
}
func (w *Worker) publicRender(args ...string) (Object, error) {
	raw, e := w.hlf(append(args, "--output")...)
	if e != nil {
		return nil, e
	}
	var v Object
	if yamlUnmarshal(raw, &v) != nil {
		return nil, secureDenied
	}
	spec := object(v["spec"])
	if _, ok := spec["imagePullPolicy"]; ok {
		spec["imagePullPolicy"] = "IfNotPresent"
	} else if _, ok := spec["pullPolicy"]; ok {
		spec["pullPolicy"] = "IfNotPresent"
	}
	return v, nil
}
func (w *Worker) secureCA(org string) error {
	registry, e := w.loadMaterial(org + "-registry")
	if e != nil {
		return e
	}
	if registry == nil {
		attrs := Object{"hf.Registrar.Roles": "peer,orderer,admin,client", "hf.Registrar.DelegateRoles": "", "hf.Registrar.Attributes": "", "hf.Revoker": false, "hf.IntermediateCA": false, "hf.GenCRL": false, "hf.AffiliationMgr": false}
		bundle := Object{}
		for _, kind := range []string{"ca", "tlsCA"} {
			password, e := w.credential("secure-registrar-" + org + "-" + strings.ToLower(kind))
			if e != nil {
				return e
			}
			bundle[kind] = Object{"maxenrollments": -1, "identities": []any{Object{"name": "registrar", "pass": password, "type": "client", "affiliation": "", "attrs": attrs}}}
		}
		raw, _ := json.Marshal(bundle)
		registry = map[string]string{"registry.json": string(raw)}
		if w.materialSecret(org+"-registry", registry, "issuer", true) != nil {
			return secureDenied
		}
	}
	current, e := w.secureGet("fabriccas", org+"-ca")
	if e != nil {
		return e
	}
	if current == nil {
		v, e := w.publicRender("ca", "create", "--name", org+"-ca", "--namespace", w.Namespace, "--image", "hyperledger/fabric-ca", "--version", "1.5.15", "--storage-class", w.storageClass(), "--capacity", w.capacity(), "--enroll-id", "unused", "--enroll-pw", "unused")
		if e != nil {
			return e
		}
		object(v["metadata"])["annotations"] = Object{"sathonate.io/ca-registry-secret": org + "-registry"}
		spec := object(v["spec"])
		spec["env"] = []any{}
		object(spec["service"])["type"] = "ClusterIP"
		for _, kind := range []string{"ca", "tlsCA"} {
			object(object(spec[kind])["registry"])["identities"] = []any{}
		}
		if w.adapt(v) != nil || w.apply(v) != nil {
			return secureDenied
		}
	} else if text(object(object(current["metadata"])["labels"])["sathonate.io/namespace"]) != w.Namespace || text(object(object(current["metadata"])["annotations"])["sathonate.io/ca-registry-secret"]) != org+"-registry" {
		return secureDenied
	}
	if w.wait("fabriccas", org+"-ca") != nil {
		return secureDenied
	}
	ca, e := w.get("fabriccas", org+"-ca")
	if e != nil {
		return e
	}
	if privateFile("/workspace/"+org+"-server.pem", []byte(text(object(ca["status"])["tls_cert"]))) != nil {
		return secureDenied
	}
	var bundle map[string]struct {
		Identities []struct {
			Pass string `json:"pass"`
		} `json:"identities"`
	}
	if json.Unmarshal([]byte(registry["registry.json"]), &bundle) != nil {
		return secureDenied
	}
	for kind, registryKind := range map[string]string{"ca": "ca", "tlsca": "tlsCA"} {
		key := org + "-registrar-" + kind
		data, e := w.loadMaterial(key + "-identity")
		if e != nil {
			return e
		}
		if data == nil {
			if len(bundle[registryKind].Identities) != 1 {
				return secureDenied
			}
			password := bundle[registryKind].Identities[0].Pass
			w.secrets = append(w.secrets, password)
			if w.caCall(key, org, "enroll", "", Object{"url": "https://registrar:" + password + "@" + org + "-ca." + w.Namespace + ":7054", "caname": kind}) != nil {
				return secureDenied
			}
			data, e = readMSP(key)
			if e != nil {
				return e
			}
			if w.validateIdentity(key, "", data, text(object(ca["status"])[map[string]string{"ca": "ca_cert", "tlsca": "tlsca_cert"}[kind]]), "registrar", "client") != nil {
				return secureDenied
			}
			if w.materialSecret(key+"-identity", data, "issuer", true) != nil {
				return secureDenied
			}
		}
		if w.validateIdentity(key, "", data, text(object(ca["status"])[map[string]string{"ca": "ca_cert", "tlsca": "tlsca_cert"}[kind]]), "registrar", "client") != nil {
			return secureDenied
		}
	}
	for attempt := 0; attempt < 3; attempt++ {
		if e := w.caCall("audit-"+org, org, "getcainfo", org+"-registrar-ca", Object{"url": "https://" + org + "-ca." + w.Namespace + ":7054", "caname": "ca"}); e != nil {
			return e
		}
		if w.argumentSamples["fabric-ca-client"] > 0 {
			return nil
		}
	}
	return fmt.Errorf("CA command completed without live argument sample; retry required")
}
func (w *Worker) secureNode(n domain.Node, org domain.Organization) error {
	materials := map[string]map[string]string{}
	for _, role := range []string{"sign", "tls", "aux"} {
		ca, kind, dns := "ca", n.Role, ""
		if role != "sign" {
			ca = "tlsca"
			dns = n.ID + "." + w.Namespace
		}
		if role == "aux" {
			kind = "client"
		}
		data, e := w.issueIdentity(org.ID, n.ID+"-"+role, ca, kind, dns)
		if e != nil {
			return e
		}
		materials[role] = data
	}
	aux := materials["aux"]
	if n.Role == "peer" {
		if w.materialSecret(n.ID+"-tls-ops", map[string]string{"tls.crt": aux["cert.pem"], "tls.key": aux["key.pem"]}, n.ID, false) != nil || w.materialSecret(n.ID+"-tlsrootcert", map[string]string{"cacert.pem": aux["root.pem"]}, n.ID, false) != nil {
			return secureDenied
		}
	} else if w.materialSecret(n.ID+"-admin", map[string]string{"tls.crt": aux["cert.pem"], "tls.key": aux["key.pem"], "cacert.crt": aux["root.pem"], "clientcacert.crt": aux["root.pem"]}, n.ID, false) != nil {
		return secureDenied
	}
	resource, command, image := "fabricpeers", "peer", "hyperledger/fabric-peer"
	if n.Role == "orderer" {
		resource, command, image = "fabricorderernodes", "ordnode", "hyperledger/fabric-orderer"
	}
	existing, e := w.secureGet(resource, n.ID)
	if e != nil {
		return e
	}
	if existing == nil {
		args := []string{command, "create", "--name", n.ID, "--namespace", w.Namespace, "--image", image, "--version", "2.5.15", "--storage-class", w.storageClass(), "--capacity", w.capacity(), "--enroll-id", "unused", "--enroll-pw", "unused", "--mspid", org.MSPID, "--ca-name", org.ID + "-ca." + w.Namespace, "--ca-host", org.ID + "-ca." + w.Namespace, "--ca-port", "7054"}
		if n.Role == "peer" {
			args = append(args, "--statedb", "leveldb")
		}
		v, e := w.publicRender(args...)
		if e != nil {
			return e
		}
		spec := object(v["spec"])
		for component, suffix := range map[string]string{"component": "sign", "tls": "tls"} {
			part := object(object(object(spec["secret"])["enrollment"])[component])
			part["enrollsecret"] = ""
			part["external"] = w.reference(n.ID + "-" + suffix + "-identity")
		}
		service := object(spec["service"])
		if _, ok := service["serviceType"]; ok {
			service["serviceType"] = "ClusterIP"
		} else {
			service["type"] = "ClusterIP"
		}
		if n.Role == "peer" {
			object(spec["couchdb"])["password"] = ""
			endpoint := n.ID + "." + w.Namespace + ":7051"
			spec["externalEndpoint"] = endpoint
			gossip := object(spec["gossip"])
			gossip["externalEndpoint"] = endpoint
			gossip["endpoint"] = endpoint
			gossip["bootstrap"] = peerBootstrap(w.Spec, n, w.Namespace)
			spec["gossip"] = gossip
		}
		if w.adapt(v) != nil || w.apply(v) != nil {
			return secureDenied
		}
	} else if text(object(object(existing["metadata"])["labels"])["sathonate.io/namespace"]) != w.Namespace {
		return secureDenied
	}
	if w.wait(resource, n.ID) != nil {
		return secureDenied
	}
	_, e = w.kub("-n", w.Namespace, "rollout", "status", "deployment/"+n.ID, "--timeout=180s")
	return e
}
func (w *Worker) directPeer(org, identity string, args ...string) ([]byte, error) {
	return w.directPeerAt(org, identity, org+"-peer-1", args...)
}
func (w *Worker) directPeerAt(org, identity, peer string, args ...string) ([]byte, error) {
	ca, e := w.get("fabriccas", org+"-ca")
	if e != nil {
		return nil, e
	}
	root := "/workspace/" + org + "-root-tls.pem"
	if privateFile(root, []byte(text(object(ca["status"])["tlsca_cert"]))) != nil {
		return nil, secureDenied
	}
	cmd := exec.CommandContext(w.ctx, "peer", args...)
	msp := "Org1MSP"
	if org == "org2" {
		msp = "Org2MSP"
	}
	cmd.Env = append(os.Environ(), "CORE_PEER_LOCALMSPID="+msp, "CORE_PEER_ADDRESS="+peer+"."+w.Namespace+":7051", "CORE_PEER_TLS_ENABLED=true", "CORE_PEER_TLS_ROOTCERT_FILE="+root, "CORE_PEER_MSPCONFIGPATH=/workspace/"+org+"-"+identity+"/msp")
	var out, diagnostic bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &diagnostic
	if w.execute(cmd) != nil {
		return nil, fmt.Errorf("protected peer %s on %s: %w", peerOperation(args), peer, securePeerFailure(diagnostic.Bytes()))
	}
	if len(args) >= 2 && args[0] == "chaincode" && args[1] == "invoke" {
		// The sample Put response is its transaction ID. Export only that
		// fixed-format payload after the CLI has waited for commit events.
		match := regexp.MustCompile(`payload:"([a-f0-9]{64})"`).FindSubmatch(diagnostic.Bytes())
		if len(match) != 2 {
			return nil, secureDenied
		}
		return append([]byte(nil), match[1]...), nil
	}
	return out.Bytes(), nil
}

func (w *Worker) secureTransaction(channel, key, value string) ([]byte, error) {
	request, _ := json.Marshal(Object{"Args": []string{"Put", key, value}})
	args := []string{"chaincode", "invoke", "-C", channel, "-n", "smp_smoke", "-o", "orderer-1." + w.Namespace + ":7050", "--tls", "--cafile", "/workspace/org1-root-tls.pem", "--waitForEvent", "--waitForEventTimeout", "45s"}
	for _, org := range []string{"org1", "org2"} {
		args = append(args, "--peerAddresses", org+"-peer-1."+w.Namespace+":7051", "--tlsRootCertFiles", "/workspace/"+org+"-root-tls.pem")
	}
	return w.directPeer("org1", "client", append(args, "-c", string(request))...)
}

func (w *Worker) secureQuery(org, channel, chaincode string, values ...string) ([]byte, error) {
	return w.secureQueryAt(org, org+"-peer-1", channel, chaincode, values...)
}
func (w *Worker) secureQueryAt(org, peer, channel, chaincode string, values ...string) ([]byte, error) {
	request, _ := json.Marshal(Object{"Args": values})
	args := []string{"chaincode", "query", "-C", channel, "-n", chaincode, "-c", string(request)}
	if chaincode == "qscc" {
		args = append(args, "--hex")
	}
	raw, e := w.directPeerAt(org, "client", peer, args...)
	if e != nil || chaincode != "qscc" {
		return raw, e
	}
	return decodeSecureLedgerHex(raw)
}

// Fabric 2.5.15 --raw prints a decimal byte slice, while --hex has an
// unambiguous text framing. Strip text whitespace before decoding; never trim
// binary protobuf bytes, whose hashes may legitimately end in whitespace.
func decodeSecureLedgerHex(raw []byte) ([]byte, error) {
	decoded, e := hex.DecodeString(strings.TrimSpace(string(raw)))
	if e != nil || len(decoded) == 0 || len(decoded) > 1<<20 {
		return nil, secureDenied
	}
	return decoded, nil
}
func (w *Worker) secureChannels() error {
	input := Object{"namespace": w.Namespace, "organizations": []any{}, "orderers": []any{}}
	for _, org := range []string{"org1", "org2"} {
		ca, e := w.get("fabriccas", org+"-ca")
		if e != nil {
			return e
		}
		status := object(ca["status"])
		msp := "Org1MSP"
		if org == "org2" {
			msp = "Org2MSP"
		}
		input["organizations"] = append(input["organizations"].([]any), Object{"msp": msp, "root": status["ca_cert"], "tlsRoot": status["tlsca_cert"]})
	}
	for _, name := range []string{"orderer-1", "orderer-2", "orderer-3"} {
		v, e := w.get("fabricorderernodes", name)
		if e != nil {
			return e
		}
		status := object(v["status"])
		input["orderers"] = append(input["orderers"].([]any), Object{"name": name, "tls": status["tlsCert"], "sign": status["signCert"]})
	}
	raw, _ := json.Marshal(input)
	cmd := exec.CommandContext(w.ctx, "candidate-tools", "channel-genesis")
	cmd.Stdin = bytes.NewReader(raw)
	cmd.Env = append(os.Environ(), "SMP_NAMESPACE="+w.Namespace)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = io.Discard
	if w.execute(cmd) != nil {
		return secureDenied
	}
	block, e := base64.StdEncoding.DecodeString(out.String())
	if e != nil {
		return secureDenied
	}
	if privateFile("/workspace/genesis.block", block) != nil {
		return secureDenied
	}
	admin, e := w.loadMaterial("org1-channel-tls-identity")
	if e != nil {
		return e
	}
	pair, e := tls.X509KeyPair([]byte(admin["cert.pem"]), []byte(admin["key.pem"]))
	if e != nil {
		return secureDenied
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM([]byte(admin["root.pem"])) {
		return secureDenied
	}
	client := http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, Certificates: []tls.Certificate{pair}}}}
	for _, name := range []string{"orderer-1", "orderer-2", "orderer-3"} {
		endpoint := "https://" + name + "." + w.Namespace + ":7053/participation/v1/channels"
		req, _ := http.NewRequestWithContext(w.ctx, "GET", endpoint+"/smpchannel", nil)
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
		part, _ := writer.CreateFormFile("config-block", "genesis.block")
		part.Write(block)
		writer.Close()
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
	// Joining a running channel requires its canonical block zero. A newly
	// generated genesis can carry a different header/nonce and strand a new
	// peer on a fork which cannot accept the orderer's subsequent blocks.
	if e := w.fetchCanonicalGenesis("smpchannel"); e != nil {
		return e
	}
	for _, node := range w.Spec.Spec.Nodes {
		if node.Role != "peer" {
			continue
		}
		org := node.OrganizationRef
		raw, e := w.directPeerAt(org, "channel-admin", node.ID, "channel", "list")
		if e != nil {
			return e
		}
		if !strings.Contains(string(raw), "smpchannel") {
			if _, e = w.directPeerAt(org, "channel-admin", node.ID, "channel", "join", "-b", "/workspace/genesis.block"); e != nil {
				return e
			}
		}
	}
	return nil
}
func (w *Worker) runSecure(ctx context.Context) error {
	w.ctx = ctx
	if domain.HasErrors(domain.Validate(w.Spec)) || w.Spec.Spec.Deployment.TargetRef != domain.SecureLocalProfile || w.Runtime.Validate(w.Spec) != nil {
		return secureDenied
	}
	// The accepted design is a set of named members. SDK defaults and the
	// chaincode server must use the same first organization regardless of JSON order.
	w.Spec.Spec.Organizations = append([]domain.Organization(nil), w.Spec.Spec.Organizations...)
	w.Spec.Spec.Nodes = append([]domain.Node(nil), w.Spec.Spec.Nodes...)
	sort.Slice(w.Spec.Spec.Organizations, func(i, j int) bool { return w.Spec.Spec.Organizations[i].ID < w.Spec.Spec.Organizations[j].ID })
	sort.Slice(w.Spec.Spec.Nodes, func(i, j int) bool { return w.Spec.Spec.Nodes[i].ID < w.Spec.Spec.Nodes[j].ID })
	if w.Lifecycle != nil {
		if w.Lifecycle.Validate() != nil {
			return secureDenied
		}
		// Lifecycle changes may reconcile existing members but must never create a
		// replacement base network if its namespace or required resources disappeared.
		for _, node := range w.Spec.Spec.Nodes {
			resource := "fabricpeers"
			if node.Role == "orderer" {
				resource = "fabricorderernodes"
			}
			v, e := w.secureGet(resource, node.ID)
			if e != nil || v == nil || text(object(object(v["metadata"])["labels"])["sathonate.io/namespace"]) != w.Namespace {
				return secureDenied
			}
		}
		for _, org := range []string{"org1", "org2"} {
			v, e := w.secureGet("fabriccas", org+"-ca")
			if e != nil || v == nil {
				return secureDenied
			}
		}
		w.Spec = w.Lifecycle.Effective(w.Spec)
		if w.Lifecycle.PeerAdded {
			w.Runtime.NodeSelectors["org1-peer-2"] = w.Runtime.NodeSelectors["org1-peer-1"]
		}
	}
	if capability, e := w.run("kubectl-hlf", "--smp-secret-input-version"); e != nil || strings.TrimSpace(string(capability)) != "smp-secret-input-v1" {
		return secureDenied
	}
	// Workspace is a dedicated RAM emptyDir. Kubernetes Secrets are the durable,
	// namespace-confined issuer journal. Private configs never enter public CRs.
	defer func() {
		entries, _ := os.ReadDir("/workspace")
		for _, entry := range entries {
			_ = os.RemoveAll(filepath.Join("/workspace", entry.Name()))
		}
	}()
	for _, stage := range []string{"validate", "identity", "kubernetes"} {
		if w.event(stage, "Validated explicit secure local application profile") != nil {
			return secureDenied
		}
	}
	if w.event("ca", "Reconciling protected root/TLS registries and independent registrars") != nil {
		return secureDenied
	}
	for _, org := range []string{"org1", "org2"} {
		if e := w.secureCA(org); e != nil {
			return e
		}
		for _, role := range []string{"channel-admin", "chaincode-admin", "client", "channel-tls"} {
			kind, caname := "admin", "ca"
			if role == "client" {
				kind = "client"
			}
			if role == "channel-tls" {
				caname = "tlsca"
			}
			data, e := w.issueIdentity(org, org+"-"+role, caname, kind, "")
			if e != nil {
				return e
			}
			if role == "chaincode-admin" || role == "channel-tls" || role == "client" {
				suffix := "-ca.yaml"
				if role == "channel-tls" {
					suffix = "-tlsca.yaml"
				}
				if role == "client" {
					suffix = "-client.yaml"
				}
				raw, _ := json.Marshal(Object{"cert": Object{"pem": data["cert.pem"]}, "key": Object{"pem": data["key.pem"]}})
				if privateFile("/workspace/"+org+suffix, raw) != nil {
					return secureDenied
				}
			}
		}
	}
	peers, orderers := []domain.Node{}, []domain.Node{}
	orgs := map[string]domain.Organization{}
	for _, org := range w.Spec.Spec.Organizations {
		orgs[org.ID] = org
	}
	for _, role := range []string{"orderer", "peer"} {
		stage := "orderers"
		if role == "peer" {
			stage = "peers"
		}
		if w.event(stage, "Validating external materials before candidate reconciliation") != nil {
			return secureDenied
		}
		for _, n := range w.Spec.Spec.Nodes {
			if n.Role != role {
				continue
			}
			if e := w.secureNode(n, orgs[n.OrganizationRef]); e != nil {
				return e
			}
			if role == "peer" {
				peers = append(peers, n)
			} else {
				orderers = append(orderers, n)
			}
		}
	}
	if w.event("channels", "Reconciling direct channel participation with isolated administrator identities") != nil {
		return secureDenied
	}
	if e := w.secureChannels(); e != nil {
		return e
	}
	if w.event("verify", "Reconciling pinned chaincode and verifying committed transactions on both peers") != nil {
		return secureDenied
	}
	chaincodeTLS, e := w.issueIdentity("org1", "smp-smoke-tls", "tlsca", "client", "smp-smoke."+w.Namespace)
	if e != nil {
		return e
	}
	const chaincodeTLSSecret = "smp-smoke-runtime-tls"
	if e = w.materialSecret(chaincodeTLSSecret, chaincodeTLSMaterial(chaincodeTLS), "issuer", true); e != nil {
		return e
	}
	chaincodeReference := Object{"secretName": chaincodeTLSSecret, "secretNamespace": w.Namespace, "certificateKey": "tls.crt", "privateKeyKey": "tls.key", "rootCertificateKey": "tlsroot.crt"}
	// Package bytes are shared with the existing tested Go contract lifecycle.
	ca, e := w.get("fabriccas", "org1-ca")
	if e != nil {
		return e
	}
	root := text(object(ca["status"])["tlsca_cert"])
	connection, _ := json.Marshal(Object{"address": "smp-smoke." + w.Namespace + ":7052", "domain_name": "smp-smoke." + w.Namespace, "dial_timeout": "10s", "tls_required": true, "client_auth_required": false, "root_cert": root})
	code, e := tarGzip(Object{"connection.json": connection})
	if e != nil {
		return e
	}
	pkg, e := tarGzip(Object{"metadata.json": []byte(`{"type":"ccaas","label":"smp_smoke_1"}`), "code.tar.gz": code})
	if e != nil {
		return e
	}
	packageID := securePackageID(pkg)
	existing, e := w.secureGet("fabricchaincodes", "smp-smoke")
	if e != nil {
		return e
	}
	if existing == nil {
		serverCA := text(object(ca["status"])["tls_cert"])
		if w.apply(Object{"apiVersion": "hlf.kungfusoftware.es/v1alpha1", "kind": "FabricChaincode", "metadata": Object{"name": "smp-smoke", "namespace": w.Namespace}, "spec": Object{"image": w.smokeImage(), "imagePullPolicy": "IfNotPresent", "packageId": packageID, "chaincodeServerPort": 7052, "replicas": 1, "credentials": Object{"enrollid": "unused", "enrollsecret": "", "cahost": "org1-ca." + w.Namespace, "caport": 7054, "caname": "tlsca", "catls": Object{"cacert": base64.StdEncoding.EncodeToString([]byte(serverCA))}, "csr": Object{"hosts": []string{"smp-smoke." + w.Namespace}}, "external": chaincodeReference}}}) != nil {
			return secureDenied
		}
	} else if text(object(object(existing["metadata"])["labels"])["sathonate.io/namespace"]) != w.Namespace {
		return secureDenied
	}
	if e := w.contract(orgs, nil, peers, orderers); e != nil {
		return e
	}
	audit, e := w.argumentEvidence()
	if e != nil {
		return e
	}
	return w.Output(Event{Message: "Protected process argument inspection completed", Security: audit})
}
