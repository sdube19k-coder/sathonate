package kubernetesfabric

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"sathonate.local/fabric-control-plane/internal/domain"
)

func TestRuntimeAdaptsExactBevelImageFields(t *testing.T) {
	pin := "@sha256:" + strings.Repeat("a", 64)
	for _, kind := range []string{"FabricCA", "FabricPeer", "FabricOrdererNode"} {
		w := Worker{Runtime: Runtime{StorageClass: "gp3", Images: map[string]string{kind: "registry.example/fabric:2.5.15" + pin}}}
		v := Object{"kind": kind, "spec": Object{}}
		if e := w.adapt(v); e != nil {
			t.Fatal(e)
		}
		spec := object(v["spec"])
		field := "tag"
		if kind == "FabricCA" {
			field = "version"
		}
		if spec[field] != "2.5.15"+pin || spec["image"] != "registry.example/fabric" {
			t.Fatal("incorrect operator image translation", v)
		}
	}
}

func TestPlacementUsesRequiredAffinityInsteadOfBrokenSelectorField(t *testing.T) {
	w := Worker{Runtime: Runtime{StorageClass: "gp3", NodeSelectors: map[string]map[string]string{"ord1": {"topology.kubernetes.io/zone": "ap-south-1a"}}}}
	value := Object{"kind": "FabricOrdererNode", "metadata": Object{"name": "ord1"}, "spec": Object{}}
	if err := w.adapt(value); err != nil {
		t.Fatal(err)
	}
	spec := object(value["spec"])
	if spec["nodeSelector"] != nil {
		t.Fatal("unsupported selector field retained")
	}
	raw, _ := json.Marshal(spec["affinity"])
	if !strings.Contains(string(raw), "requiredDuringSchedulingIgnoredDuringExecution") || !strings.Contains(string(raw), "ap-south-1a") {
		t.Fatal("zone placement is not required", string(raw))
	}
}
func TestChannelBrokerRejectsForeignReferencesAndPrivateKeys(t *testing.T) {
	f, e := os.Open("../../examples/local-kubernetes-network.json")
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	s, e := domain.Decode(f)
	if e != nil {
		t.Fatal(e)
	}
	ns := "smp-n-0123456789abcdef"
	base := Object{"apiVersion": "hlf.kungfusoftware.es/v1alpha1", "kind": "FabricFollowerChannel", "metadata": Object{"name": ns + "-org1-sathonate", "labels": Object{"sathonate.io/namespace": ns}}, "spec": Object{"anchorPeers": []any{Object{"host": "peer1." + ns, "port": 7051}}, "hlfIdentity": Object{"secretName": "smp-wallet", "secretNamespace": ns, "secretKey": "org1-ca.yaml"}, "mspId": "Org1MSP", "name": "sathonate", "externalPeersToJoin": []any{Object{"url": "grpcs://peer1." + ns + ":7051", "tlsCACert": "PUBLIC CERT"}}, "orderers": []any{Object{"url": "grpcs://orderer1." + ns + ":7050", "certificate": "PUBLIC CERT"}}, "peersToJoin": []any{}}}
	if e = ValidateChannelRequest(base, ns, s); e != nil {
		t.Fatal(e)
	}
	for _, mutate := range []func(Object){
		func(v Object) { object(object(v["spec"])["hlfIdentity"])["secretNamespace"] = "another-tenant" },
		func(v Object) {
			object(v["spec"])["externalPeersToJoin"] = []any{Object{"url": "grpcs://attacker:7051"}}
		},
		func(v Object) { object(v["spec"])["orderers"] = []any{Object{"certificate": "BEGIN PRIVATE KEY"}} },
		func(v Object) { object(v["spec"])["unknownResource"] = true },
		func(v Object) { delete(object(v["spec"]), "hlfIdentity") },
	} {
		raw, _ := json.Marshal(base)
		var copy Object
		_ = json.Unmarshal(raw, &copy)
		mutate(copy)
		if ValidateChannelRequest(copy, ns, s) == nil {
			t.Fatal("unsafe broker resource accepted")
		}
	}
}
