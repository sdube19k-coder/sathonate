package httpapi

import (
	"sathonate.local/fabric-control-plane/internal/deployment"
	"sathonate.local/fabric-control-plane/internal/identity"
	"testing"
)

func TestCancellationDoesNotAcceptCredentialsOrArbitraryOptions(t *testing.T) {
	manager, err := deployment.Open(t.TempDir(), deployment.MockRunner{})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	h := NewConfigured(nil, []string{"localhost:8080"}, nil, Options{Auth: identity.Config{Mode: "local-development"}, Deployments: manager})
	for _, body := range []string{`null`, `{} {}`, `{"awsSecretAccessKey":"do-not-collect"}`, `{"command":"execute"}`} {
		response := request(h, "POST", "/api/v1/deployments/op_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/cancel", []byte(body), nil)
		if response.Code != 400 {
			t.Fatal("unexpected cancellation payload accepted", response.Code)
		}
	}
}
