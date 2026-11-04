package deployment

import (
	"encoding/json"
	"strings"
	"testing"

	"sathonate.local/fabric-control-plane/internal/kubernetesfabric"
)

func exportFixture(t *testing.T) QualificationExport {
	s := secureDesign(t)
	r := kubernetesfabric.Runtime{StorageClass: "smp-gp3-encrypted", Capacity: "10Gi", BrokerChannels: true, SmokeImage: "example.test/smoke@sha256:" + strings.Repeat("a", 64), Images: map[string]string{}, NodeSelectors: map[string]map[string]string{}}
	for _, kind := range []string{"FabricCA", "FabricPeer", "FabricOrdererNode"} {
		r.Images[kind] = "example.test/fabric:verified@sha256:" + strings.Repeat("b", 64)
	}
	for i, n := range s.Spec.Nodes {
		r.NodeSelectors[n.ID] = map[string]string{"topology.kubernetes.io/zone": []string{"ap-south-1a", "ap-south-1b", "ap-south-1c"}[i%3]}
	}
	return QualificationExport{TenantID: "lp-" + strings.Repeat("a", 48), NetworkID: strings.Repeat("b", 32), OperationID: "op_" + strings.Repeat("c", 32), WorkerImage: "example.test/worker@sha256:" + strings.Repeat("d", 64), ControllerImage: "example.test/controller@" + strings.Split(SecureLocalProfile().ControllerImage, "@")[1], APIIPs: []string{"10.100.0.1", "10.55.0.10"}, Network: s, Runtime: r}
}

func TestOfflineExportPreservesProtectedBoundaries(t *testing.T) {
	v, err := ExportQualification(exportFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(v)
	s := string(raw)
	for _, bad := range []string{"ClusterRole", "cluster-admin", "PRIVATE KEY", "0.0.0.0/0", `"imagePullPolicy":"Never"`} {
		if strings.Contains(s, bad) {
			t.Fatal("unsafe export", bad)
		}
	}
	for _, required := range []string{"--secure-local", "secure-issuer", "secure-controller", "default-deny", "80Gi", `"awsExecuted":false`, `"productionEnabled":false`, `"automountServiceAccountToken":false`} {
		if !strings.Contains(s, required) {
			t.Fatal("missing protected boundary", required)
		}
	}
	if SecureLocalProfile().AWSEnabled {
		t.Fatal("offline export enabled application AWS execution")
	}
	// A cloud cluster name is still rejected by the existing local constructor.
	if _, err := NewSecureRunner(SecureLocalConfig{ClusterName: "sathonate-s3-test"}); err == nil {
		t.Fatal("kind-only guard weakened")
	}
}

func TestOfflineExportRejectsUnqualifiedMaterialAndPlacement(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*QualificationExport)
	}{
		{"original-controller", func(v *QualificationExport) {
			v.ControllerImage = "upstream/controller@sha256:" + strings.Repeat("a", 64)
		}},
		{"mutable-worker", func(v *QualificationExport) { v.WorkerImage = "example.test/worker:latest" }},
		{"namespace-injection", func(v *QualificationExport) { v.NetworkID = "../../retained" }},
		{"global-api-egress", func(v *QualificationExport) { v.APIIPs = []string{"0.0.0.0"} }},
		{"loopback-api", func(v *QualificationExport) { v.APIIPs = []string{"127.0.0.1"} }},
		{"single-zone", func(v *QualificationExport) {
			for _, m := range v.Runtime.NodeSelectors {
				m["topology.kubernetes.io/zone"] = "ap-south-1a"
			}
		}},
		{"legacy-profile", func(v *QualificationExport) { v.Network.Spec.Deployment.TargetRef = "company-eks" }},
		{"unbounded-storage", func(v *QualificationExport) { v.Runtime.Capacity = "1000Gi" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := exportFixture(t)
			tc.change(&v)
			if _, err := ExportQualification(v); err == nil {
				t.Fatal("unsafe input accepted")
			}
		})
	}
}
