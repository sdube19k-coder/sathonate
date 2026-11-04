package httpapi

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sathonate.local/fabric-control-plane/internal/deployment"
	"sathonate.local/fabric-control-plane/internal/drafts"
	"sathonate.local/fabric-control-plane/internal/identity"
	"testing"
	"time"
)

func TestDeploymentRequestsAreRevisionBoundAndRejectCredentials(t *testing.T) {
	store, err := drafts.Open(filepath.Join(t.TempDir(), "drafts.json"))
	if err != nil {
		t.Fatal(err)
	}
	manager, err := deployment.Open(t.TempDir(), deployment.MockRunner{Delay: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	h := NewConfigured(func(*http.Request) Repository { return fileRepository{store} }, []string{"localhost:8080"}, nil, Options{Auth: identity.Config{Mode: "local-development"}, Deployments: manager})
	raw, err := os.ReadFile("../../examples/local-kubernetes-network.json")
	if err != nil {
		t.Fatal(err)
	}
	created := request(h, "POST", "/api/v1/networks", raw, nil)
	var d drafts.Draft
	if created.Code != 201 || json.Unmarshal(created.Body.Bytes(), &d) != nil {
		t.Fatal(created.Body.String())
	}
	route := "/api/v1/networks/" + d.ID + "/deploy"
	for _, tc := range []struct {
		body, revision, key string
		status              int
	}{
		{`{}`, "", "qualified-key", 428}, {`{}`, `"99"`, "qualified-key", 409},
		{`{}`, `"1"`, "", 400}, {`{"awsSecretAccessKey":"secret"}`, `"1"`, "qualified-key", 400},
		{`{} {}`, `"1"`, "qualified-key", 400}, {`{"failureStage":"not-a-stage"}`, `"1"`, "qualified-key", 409},
	} {
		w := request(h, "POST", route, []byte(tc.body), map[string]string{"If-Match": tc.revision, "Idempotency-Key": tc.key})
		if w.Code != tc.status {
			t.Fatalf("expected %d got %d: %s", tc.status, w.Code, w.Body.String())
		}
	}
	headers := map[string]string{"If-Match": `"1"`, "Idempotency-Key": "qualified-key"}
	first := request(h, "POST", route, []byte(`{}`), headers)
	again := request(h, "POST", route, []byte(`{}`), headers)
	if first.Code != 202 || again.Code != 202 || first.Header().Get("Location") != again.Header().Get("Location") {
		t.Fatal("deployment idempotency failed", first.Body.String(), again.Body.String())
	}
}
