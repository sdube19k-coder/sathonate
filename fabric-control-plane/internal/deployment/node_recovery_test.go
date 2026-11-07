package deployment

import (
	"encoding/json"
	"sathonate.local/fabric-control-plane/internal/operations"
	"strings"
	"testing"
)

func TestRecoveryTemplateBindsAllFieldsAndIgnoresOnlyItsMarker(t *testing.T) {
	raw := []byte(`{"metadata":{"labels":{"release":"peer"}},"spec":{"containers":[{"image":"approved","env":[{"name":"PUBLIC","value":"before"}]}]}}`)
	a, e := recoveryTemplateHash(raw)
	if e != nil {
		t.Fatal(e)
	}
	var obj map[string]any
	json.Unmarshal(raw, &obj)
	obj["metadata"].(map[string]any)["annotations"] = map[string]any{recoveryAnnotation: "digest"}
	same, _ := json.Marshal(obj)
	b, _ := recoveryTemplateHash(same)
	if a != b {
		t.Fatal("retry changed restart checkpoint hash")
	}
	changed := []byte(strings.Replace(string(same), "before", "after", 1))
	c, _ := recoveryTemplateHash(changed)
	if c == a {
		t.Fatal("unmodeled environment drift escaped template binding")
	}
}
func TestRecoveryRetryStillRejectsResourceAndMaterialReplacement(t *testing.T) {
	a := NodeRecovery{Node: "peer", Kind: "peer", DeploymentUID: "owned", ResourceUID: "cr", TemplateHash: "hash", Replicas: 0, PodUIDs: []string{}, Claims: map[string]string{"data": "pvc"}, Materials: map[string]materialVersion{"tls": {UID: "secret", Version: "1"}}}
	b := a
	b.Replicas = 1
	b.PodUIDs = []string{"replacement"}
	if sameRecoveryResource(a, b, false) || !sameRecoveryResource(a, b, true) {
		t.Fatal("ambiguous acknowledgement did not adopt exactly the approved restart")
	}
	b.DeploymentUID = "foreign"
	if sameRecoveryResource(a, b, true) {
		t.Fatal("foreign deployment adopted")
	}
	b = a
	b.Claims = map[string]string{"data": "replacement"}
	if sameRecoveryResource(a, b, true) {
		t.Fatal("replacement storage adopted")
	}
	b = a
	b.Materials = map[string]materialVersion{"tls": {UID: "secret", Version: "2"}}
	if sameRecoveryResource(a, b, true) {
		t.Fatal("changed credential material adopted")
	}
}

func TestRecoveryDeploymentOwnershipUsesActualNamespaceAndHelmIdentity(t *testing.T) {
	labels := map[string]string{"release": "peer", "app.kubernetes.io/managed-by": "Helm"}
	annotations := map[string]string{"meta.helm.sh/release-name": "peer"}
	// Blank rendering annotation alone never authorizes a foreign namespace.
	if !recoveryDeploymentOwned("owned", "peer", "owned", labels, annotations) || recoveryDeploymentOwned("owned", "peer", "foreign", labels, annotations) {
		t.Fatal("actual namespace binding incorrect")
	}
	annotations["meta.helm.sh/release-namespace"] = "foreign"
	if recoveryDeploymentOwned("owned", "peer", "owned", labels, annotations) {
		t.Fatal("foreign Helm namespace accepted")
	}
	annotations["meta.helm.sh/release-namespace"] = "owned"
	labels["release"] = "foreign"
	if recoveryDeploymentOwned("owned", "peer", "owned", labels, annotations) {
		t.Fatal("foreign release accepted")
	}
}

func TestRecoveryCheckpointPreservesWorkerStageOrder(t *testing.T) {
	store, e := operations.Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	op, _, e := store.Begin("tenant", "actor", "network", "recovery", "recovery-checkpoint", "digest", "version", "", Stages)
	if e != nil {
		t.Fatal(e)
	}
	emit := func(stage, message string) error {
		if e := store.Advance(op.ID, stage); e != nil {
			return e
		}
		return store.Event(op.ID, message)
	}
	if emit("validate", "preflight") != nil || emitRecoveryCheckpoint(emit, "peer") != nil {
		t.Fatal("recovery checkpoint skipped a worker stage")
	}
	if emit("identity", "worker identity") != nil || emit("kubernetes", "worker preparation") != nil {
		t.Fatal("recovery checkpoint blocked normal worker progress")
	}
}

func TestEmptyClientTrustPlaceholderRequiresQualifiedTLSConfiguration(t *testing.T) {
	config := map[string]string{"ORDERER_GENERAL_TLS_ENABLED": "true", "ORDERER_GENERAL_TLS_CLIENTAUTHREQUIRED": "false", "ORDERER_GENERAL_CLUSTER_ROOTCAS": "/var/hyperledger/tls/server/cert/cacert.pem", "ORDERER_ADMIN_TLS_ENABLED": "true", "ORDERER_ADMIN_TLS_CLIENTROOTCAS": "[/var/hyperledger/admin/clientcacert.crt]"}
	if !optionalRecoveryMaterial("orderer", "raft", "raft-peer-tlsrootcert", config) {
		t.Fatal("unused chart placeholder rejected")
	}
	for _, name := range []string{"raft-tls", "raft-idkey", "raft-admin", "foreign-peer-tlsrootcert"} {
		if optionalRecoveryMaterial("orderer", "raft", name, config) {
			t.Fatal("required or foreign material waived")
		}
	}
	for key := range config {
		value := config[key]
		config[key] = "unexpected"
		if optionalRecoveryMaterial("orderer", "raft", "raft-peer-tlsrootcert", config) {
			t.Fatal("trust/configuration change waived", key)
		}
		config[key] = value
	}
	if optionalRecoveryMaterial("peer", "raft", "raft-peer-tlsrootcert", config) {
		t.Fatal("peer material waived")
	}
}
