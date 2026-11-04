package httpapi

import (
	"bytes"
	"encoding/json"
	"testing"

	"sathonate.local/fabric-control-plane/internal/domain"
	"sathonate.local/fabric-control-plane/internal/drafts"
)

func TestUnconfiguredCloudIsSafeAndRevisionBound(t *testing.T) {
	h := handler(t)
	created := request(h, "POST", "/api/v1/networks", input(t), nil)
	var d drafts.Draft
	if created.Code != 201 || json.Unmarshal(created.Body.Bytes(), &d) != nil {
		t.Fatal(created.Body.String())
	}
	route := "/api/v1/networks/" + d.ID
	targets := request(h, "GET", "/api/v1/cloud/targets", nil, nil)
	if targets.Code != 200 || !bytes.Contains(targets.Body.Bytes(), []byte(`"items":[]`)) {
		t.Fatal(targets.Body.String())
	}
	for _, suffix := range []string{"/cloud-preflight", "/cloud-bundle"} {
		if w := request(h, "POST", route+suffix, nil, nil); w.Code != 428 {
			t.Fatal("missing revision accepted", w.Code)
		}
		if w := request(h, "POST", route+suffix, nil, map[string]string{"If-Match": `"9"`}); w.Code != 409 {
			t.Fatal("stale revision accepted", w.Code)
		}
	}
	headers := map[string]string{"If-Match": `"1"`}
	preflight := request(h, "POST", route+"/cloud-preflight", nil, headers)
	if preflight.Code != 200 || !bytes.Contains(preflight.Body.Bytes(), []byte(`"ready":false`)) || !bytes.Contains(preflight.Body.Bytes(), []byte("registered-target")) {
		t.Fatal(preflight.Body.String())
	}
	bundle := request(h, "POST", route+"/cloud-bundle", nil, headers)
	if bundle.Code != 422 {
		t.Fatal("unregistered target compiled", bundle.Code)
	}
	unchanged := request(h, "GET", route, nil, nil)
	var after drafts.Draft
	if json.Unmarshal(unchanged.Body.Bytes(), &after) != nil || after.Revision != d.Revision || after.Status != "draft" {
		t.Fatal("cloud checks mutated draft")
	}
}

func TestDeploymentPackagePreservesUIIntentAndRevision(t *testing.T) {
	h := handler(t)
	s, err := domain.Decode(bytes.NewReader(input(t)))
	if err != nil {
		t.Fatal(err)
	}
	s.Spec.Deployment.Engine = "bevel"
	s.Spec.Deployment.Provider = "aws"
	s.Spec.Deployment.Runtime = "kubernetes"
	s.Spec.Deployment.TargetRef = "company-eks"
	s.Spec.Deployment.Cloud = &domain.CloudIntent{AccountID: "123456789012", ClusterName: "company", Region: "ap-south-1", ClusterMode: "existing", VPCMode: "existing", MinNodes: 1, DesiredNodes: 2, MaxNodes: 3}
	raw, _ := json.Marshal(s)
	created := request(h, "POST", "/api/v1/networks", raw, nil)
	var d drafts.Draft
	if created.Code != 201 || json.Unmarshal(created.Body.Bytes(), &d) != nil {
		t.Fatal(created.Body.String())
	}
	route := "/api/v1/networks/" + d.ID + "/deployment-package"
	if w := request(h, "POST", route, nil, nil); w.Code != 428 {
		t.Fatal("revision missing")
	}
	if w := request(h, "POST", route, nil, map[string]string{"If-Match": `"2"`}); w.Code != 409 {
		t.Fatal("stale revision accepted")
	}
	w := request(h, "POST", route, nil, map[string]string{"If-Match": `"1"`})
	if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte(`"clusterName":"company"`)) || !bytes.Contains(w.Body.Bytes(), []byte(`"executable":false`)) {
		t.Fatal(w.Body.String())
	}
}
