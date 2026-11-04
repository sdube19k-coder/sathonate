package httpapi

import (
	"bytes"
	"sathonate.local/fabric-control-plane/internal/identity"
	"testing"
)

func TestReadinessIsTruthfulWhenCloudIsAbsent(t *testing.T) {
	h := handler(t)
	for _, tc := range []struct{ method, path string }{{"GET", "/api/v1/readiness"}, {"POST", "/api/v1/readiness/probe"}} {
		w := request(h, tc.method, tc.path, []byte(`{}`), nil)
		if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte(`"ready":false`)) || !bytes.Contains(w.Body.Bytes(), []byte(`"executionEnabled":false`)) || !bytes.Contains(w.Body.Bytes(), []byte(`"status":"missing"`)) {
			t.Fatal("missing configuration reported ready", w.Code, w.Body.String())
		}
	}
}
func TestPrelaunchSetupNotExposedToEnterpriseTenants(t *testing.T) {
	h := NewConfigured(nil, []string{"localhost:8080"}, nil, Options{Auth: identity.Config{Mode: "okta"}})
	for _, method := range []string{"GET", "POST"} {
		path := "/api/v1/readiness"
		if method == "POST" {
			path += "/probe"
		}
		w := request(h, method, path, []byte(`{}`), nil)
		if w.Code != 404 {
			t.Fatal("local administrator setup exposed", w.Code)
		}
	}
}

func TestReadinessRejectsInlineAccessMaterial(t *testing.T) {
	h := handler(t)
	for _, body := range []string{`{"awsSecretAccessKey":"do-not-collect"}`, `{"kubeconfig":"token"}`, `{} {}`, `null`} {
		w := request(h, "POST", "/api/v1/readiness/probe", []byte(body), nil)
		if w.Code != 400 {
			t.Fatalf("unexpected readiness payload accepted: %d", w.Code)
		}
	}
}
