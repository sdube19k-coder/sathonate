package httpapi

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sathonate.local/fabric-control-plane/internal/deployment"
	"sathonate.local/fabric-control-plane/internal/domain"
	"sathonate.local/fabric-control-plane/internal/drafts"
	"sathonate.local/fabric-control-plane/internal/identity"
	"strings"
	"testing"
)

func TestSecureDesignValidationIsNotDeploymentAcceptance(t *testing.T) {
	store, err := drafts.Open(filepath.Join(t.TempDir(), "drafts.json"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := deployment.Open(t.TempDir(), deployment.MockRunner{})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	h := NewConfigured(func(*http.Request) Repository { return fileRepository{store} }, []string{"localhost:8080"}, nil, Options{Auth: identity.Config{Mode: "local-development"}, Deployments: m})
	raw, err := os.ReadFile("../../examples/secure-local-network.json")
	if err != nil {
		t.Fatal(err)
	}
	w := request(h, "POST", "/api/v1/validate", raw, nil)
	var report struct {
		Valid             bool
		DeploymentProfile deployment.SecureProfile
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &report) != nil || !report.Valid || report.DeploymentProfile.ExecutionEnabled || len(report.DeploymentProfile.Blockers) == 0 {
		t.Fatal("validation misreported secure execution", w.Body.String())
	}
	w = request(h, "POST", "/api/v1/networks", raw, nil)
	var d drafts.Draft
	if w.Code != 201 || json.Unmarshal(w.Body.Bytes(), &d) != nil {
		t.Fatal(w.Body.String())
	}
	w = request(h, "POST", "/api/v1/networks/"+d.ID+"/deploy", []byte(`{}`), map[string]string{"If-Match": `"1"`, "Idempotency-Key": "secure-blocked-request"})
	if w.Code != 403 || !strings.Contains(w.Body.String(), "secure_auth_required") || len(m.Operations().List("local-development")) != 0 {
		t.Fatal("secure request dispatched to legacy path", w.Body.String())
	}
}

func TestSecureEKSAPIRejectsImplicitDeveloperBypass(t *testing.T) {
	store, e := drafts.Open(filepath.Join(t.TempDir(), "drafts.json"))
	if e != nil {
		t.Fatal(e)
	}
	manager, e := deployment.Open(t.TempDir(), deployment.MockRunner{})
	if e != nil {
		t.Fatal(e)
	}
	defer manager.Close()
	handler := NewConfigured(func(*http.Request) Repository { return fileRepository{store} }, []string{"localhost:8080"}, nil, Options{Auth: identity.Config{Mode: "local-development"}, Deployments: manager})
	raw, e := os.ReadFile("../../examples/secure-local-network.json")
	if e != nil {
		t.Fatal(e)
	}
	var spec domain.NetworkSpec
	if json.Unmarshal(raw, &spec) != nil {
		t.Fatal("fixture")
	}
	spec.Spec.Deployment.TargetRef = domain.SecureEKSProfile
	spec.Spec.Deployment.Provider = "aws"
	raw, _ = json.Marshal(spec)
	response := request(handler, "POST", "/api/v1/networks", raw, nil)
	var draft drafts.Draft
	if response.Code != 201 || json.Unmarshal(response.Body.Bytes(), &draft) != nil {
		t.Fatal("save failed", response.Code)
	}
	response = request(handler, "POST", "/api/v1/networks/"+draft.ID+"/deploy", []byte(`{}`), map[string]string{"If-Match": `"1"`, "Idempotency-Key": "secure-eks-no-auth-bypass"})
	if response.Code != 403 || !strings.Contains(response.Body.String(), "secure_auth_required") || len(manager.Operations().List("local-development")) != 0 {
		t.Fatal("implicit identity reached secure EKS execution")
	}
}
