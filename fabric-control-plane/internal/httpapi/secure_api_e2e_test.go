package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"sathonate.local/fabric-control-plane/internal/deployment"
	"sathonate.local/fabric-control-plane/internal/drafts"
	"sathonate.local/fabric-control-plane/internal/identity"
	"sathonate.local/fabric-control-plane/internal/operations"
	"sathonate.local/fabric-control-plane/internal/persistence"
	"strings"
	"testing"
	"time"
)

// Opt-in real API/issuer/controller/Fabric test. No runner or Fabric API is mocked.
func TestSecureApplicationAPIOnDisposableKind(t *testing.T) {
	if os.Getenv("SMP_SECURE_API_E2E") != "1" {
		t.Skip("requires explicit disposable secure API qualification")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 24*time.Minute)
	defer cancel()
	connection := os.Getenv("SATHONATE_TEST_DATABASE_URL")
	if !strings.Contains(connection, "/fabric_smp_tests?") {
		t.Fatal("dedicated fabric_smp_tests URL required")
	}
	db, e := persistence.Open(ctx, connection, false)
	if e != nil {
		t.Fatal("qualification database unavailable")
	}
	defer db.Close()
	if db.Migrate(ctx) != nil {
		t.Fatal("qualification database schema failed")
	}
	configPath := os.Getenv("SMP_SECURE_CONFIG")
	runner, e := deployment.LoadSecureRunner(configPath)
	if e != nil {
		t.Fatal(e)
	}
	var config deployment.SecureLocalConfig
	raw, _ := os.ReadFile(configPath)
	json.Unmarshal(raw, &config)
	makeGrant := func(tenant, project string, permissions []string) (string, identity.LocalGrant) {
		token, digest, e := identity.GenerateLocalToken()
		if e != nil {
			t.Fatal(e)
		}
		return token, identity.LocalGrant{TokenSHA256: digest, Developer: "qualification-developer", Tenant: tenant, Project: project, Permissions: permissions, ExpiresAt: time.Now().Add(time.Hour)}
	}
	permissions := []string{"read", "write", "deploy", "retry", "cancel", "cleanup"}
	unique := strings.TrimPrefix(config.ClusterName, "smp-sprint1-")
	token, grant := makeGrant("sprint1-"+unique, "project-a", permissions)
	foreignTenant, otherTenant := makeGrant("foreign-"+unique, "project-a", permissions)
	foreignProject, otherProject := makeGrant(grant.Tenant, "project-b", permissions)
	viewer, viewerGrant := makeGrant(grant.Tenant, grant.Project, []string{"read"})
	auth, e := identity.NewLocalTokens("local-development", []identity.LocalGrant{grant, otherTenant, otherProject, viewerGrant})
	if e != nil {
		t.Fatal(e)
	}
	var authentication interface {
		Config() identity.Config
		Middleware(http.Handler) http.Handler
	} = auth
	if os.Getenv("SMP_SECURE_OIDC_FIXTURE") == "1" {
		fixture, tokens := secureOIDCFixture(t, []identity.LocalGrant{grant, otherTenant, otherProject, viewerGrant})
		authentication = fixture
		token, foreignTenant, foreignProject, viewer = tokens[0], tokens[1], tokens[2], tokens[3]
	}
	manager, e := deployment.OpenDatabase(ctx, db.Pool(), runner)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { manager.Close() }()
	var server *httptest.Server
	openServer := func() {
		handler := NewConfigured(func(r *http.Request) Repository {
			return db.ForTenant(r.Context(), identity.FromContext(r.Context()).StorageTenant())
		}, []string{}, nil, Options{Auth: authentication.Config(), Deployments: manager, Storage: "PostgreSQL local project scopes"})
		server = httptest.NewServer(authentication.Middleware(handler))
	}
	// The API validates Host independently. Bind the test listener's exact host.
	openServer = func() {
		listener := httptest.NewUnstartedServer(nil)
		host := listener.Listener.Addr().String()
		handler := NewConfigured(func(r *http.Request) Repository {
			return db.ForTenant(r.Context(), identity.FromContext(r.Context()).StorageTenant())
		}, []string{host}, nil, Options{Auth: authentication.Config(), Deployments: manager, Storage: "PostgreSQL local project scopes"})
		listener.Config.Handler = authentication.Middleware(handler)
		listener.Start()
		server = listener
	}
	openServer()
	defer func() { server.Close() }()
	call := func(method, path, credential string, body []byte, headers map[string]string) (int, []byte) {
		t.Helper()
		req, e := http.NewRequestWithContext(ctx, method, server.URL+path, bytes.NewReader(body))
		if e != nil {
			t.Fatal(e)
		}
		req.Header.Set("Content-Type", "application/json")
		if credential != "" {
			req.Header.Set("Authorization", "Bearer "+credential)
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		res, e := http.DefaultClient.Do(req)
		if e != nil {
			t.Fatal("API request failed")
		}
		defer res.Body.Close()
		var out bytes.Buffer
		out.ReadFrom(res.Body)
		data := out.Bytes()
		for _, secret := range []string{token, foreignTenant, foreignProject, viewer} {
			if bytes.Contains(data, []byte(secret)) {
				t.Fatal("API exposed credential")
			}
		}
		return res.StatusCode, data
	}
	if status, _ := call("GET", "/api/v1/projects", "", nil, nil); status != 401 {
		t.Fatal("unauthenticated project access")
	}
	if status, _ := call("GET", "/api/v1/projects", token, nil, nil); status != 200 {
		t.Fatal("authenticated project unavailable")
	}
	design, e := os.ReadFile("../../examples/secure-local-network.json")
	if e != nil {
		t.Fatal(e)
	}
	status, body := call("POST", "/api/v1/networks", token, design, nil)
	var draft drafts.Draft
	if status != 201 || json.Unmarshal(body, &draft) != nil {
		t.Fatal("API design creation failed", status, string(body))
	}
	headers := map[string]string{"If-Match": `"1"`, "Idempotency-Key": "secure-application-" + unique}
	if status, _ = call("POST", "/api/v1/networks/"+draft.ID+"/deploy", viewer, []byte(`{}`), headers); status != 403 {
		t.Fatal("viewer escalated to deployment")
	}
	status, body = call("POST", "/api/v1/networks/"+draft.ID+"/deploy", token, []byte(`{}`), headers)
	var first operations.Operation
	if status != 202 || json.Unmarshal(body, &first) != nil {
		t.Fatal("API secure start failed", status, string(body))
	}
	status, body = call("POST", "/api/v1/networks/"+draft.ID+"/deploy", token, []byte(`{}`), headers)
	var duplicate operations.Operation
	if status != 202 || json.Unmarshal(body, &duplicate) != nil || duplicate.ID != first.ID {
		t.Fatal("deployment idempotency failed")
	}
	for _, credential := range []string{foreignTenant, foreignProject} {
		for _, path := range []string{"/api/v1/networks/" + draft.ID, "/api/v1/operations/" + first.ID, "/api/v1/deployments/" + first.ID} {
			if status, _ = call("GET", path, credential, nil, nil); status != 404 {
				t.Fatal("cross-scope object visible", path, status)
			}
		}
		if status, _ = call("POST", "/api/v1/operations/"+first.ID+"/retry", credential, []byte(`{}`), map[string]string{"Idempotency-Key": "cross-scope-retry"}); status < 400 {
			t.Fatal("cross-scope retry accepted")
		}
	}
	wait := func(id string, condition func(operations.Operation) bool) operations.Operation {
		t.Helper()
		for ctx.Err() == nil {
			status, data := call("GET", "/api/v1/operations/"+id, token, nil, nil)
			var op operations.Operation
			if status != 200 || json.Unmarshal(data, &op) != nil {
				t.Fatal("persisted operation unavailable")
			}
			if condition(op) {
				return op
			}
			if op.Status != "running" {
				for _, event := range op.Events {
					// These are the same redacted public operation events already
					// returned by the API, never private CLI diagnostics.
					t.Log(event.Stage, event.Message)
				}
				t.Fatal("unexpected terminal operation", op.Status, op.Error)
			}
			time.Sleep(time.Second)
		}
		t.Fatal("operation timed out")
		return operations.Operation{}
	}
	wait(first.ID, func(op operations.Operation) bool { return op.CurrentStage == "ca" })
	principal := identity.Principal{TenantID: grant.Tenant, ProjectID: grant.Project}
	namespace, _ := deployment.SecureNamespace(principal.StorageTenant(), "token-bound-project", draft.ID)
	kub := func(args ...string) []byte {
		t.Helper()
		cmd := exec.CommandContext(ctx, "kubectl", append([]string{"--kubeconfig", config.Kubeconfig, "--context", "kind-" + config.ClusterName, "--request-timeout=20s", "-n", namespace}, args...)...)
		out, e := cmd.Output()
		if e != nil {
			t.Fatal("scoped qualification Kubernetes observation failed")
		}
		return out
	}
	job := "smp-" + strings.TrimPrefix(first.ID, "op_")
	uid := string(kub("get", "job", job, "-o", "jsonpath={.metadata.uid}"))
	// Restart the real application manager while its existing in-cluster Job runs.
	server.Close()
	manager.Close()
	manager, e = deployment.OpenDatabase(ctx, db.Pool(), runner)
	if e != nil {
		t.Fatal("durable executor restart failed", e)
	}
	openServer()
	if string(kub("get", "job", job, "-o", "jsonpath={.metadata.uid}")) != uid {
		t.Fatal("application restart replaced worker Job")
	}
	// Real scoped fault: terminate only this disposable worker pod, then retry via API.
	kub("delete", "pod", "-l", "job-name="+job, "--wait=true", "--timeout=60s")
	wait(first.ID, func(op operations.Operation) bool { return op.Status == "failed" })
	status, body = call("POST", "/api/v1/operations/"+first.ID+"/retry", token, []byte(`{}`), map[string]string{"Idempotency-Key": "secure-retry-" + unique})
	var retry operations.Operation
	if status != 202 || json.Unmarshal(body, &retry) != nil || retry.RetryOf != first.ID {
		t.Fatal("API authorized retry failed", status, string(body))
	}
	completed := wait(retry.ID, func(op operations.Operation) bool { return op.Status == "succeeded" })
	if status, data := call("GET", "/api/v1/operations/"+retry.ID+"/diagnostics", viewer, nil, nil); status != 200 || !bytes.Contains(data, []byte(`"liveHealthChecked":false`)) {
		t.Fatal("read-only operational diagnostics unavailable")
	}
	for _, credential := range []string{foreignTenant, foreignProject} {
		if status, _ := call("GET", "/api/v1/operations/"+retry.ID+"/diagnostics", credential, nil, nil); status != 404 {
			t.Fatal("cross-scope diagnostics visible")
		}
	}
	status, body = call("GET", "/api/v1/deployments/"+retry.ID, token, nil, nil)
	var result deployment.Result
	if status != 200 || json.Unmarshal(body, &result) != nil || !result.Ready || !result.Verified || result.Simulated || result.Security == nil || !result.Security.CredentialsAbsent || result.Health == nil || len(result.Health.Peers) != 2 {
		t.Fatal("complete API transaction evidence missing", string(body))
	}
	firstTransaction := result.TransactionID
	firstHeights := map[string]uint64{}
	for _, peer := range result.Health.Peers {
		firstHeights[peer.Peer] = peer.Height
	}
	pvcUIDs := string(kub("get", "pvc", "-o", "jsonpath={range .items[*]}{.metadata.name}:{.metadata.uid}{'\\n'}{end}"))
	if pvcUIDs == "" {
		t.Fatal("persistent storage inventory missing")
	}
	// Only this disposable namespace is changed. Preserve the peer PVC, then
	// reconcile through the API and commit a second new transaction.
	beforePod := string(kub("get", "pod", "-l", "release=org1-peer-1", "-o", "jsonpath={.items[0].metadata.uid}"))
	kub("delete", "pod", "-l", "release=org1-peer-1", "--wait=true", "--timeout=60s")
	kub("rollout", "status", "deployment/org1-peer-1", "--timeout=120s")
	if string(kub("get", "pod", "-l", "release=org1-peer-1", "-o", "jsonpath={.items[0].metadata.uid}")) == beforePod {
		t.Fatal("peer pod was not replaced")
	}
	status, body = call("POST", "/api/v1/networks/"+draft.ID+"/deploy", token, []byte(`{}`), map[string]string{"If-Match": `"1"`, "Idempotency-Key": "storage-recovery-" + unique})
	var recovery operations.Operation
	if status != 202 || json.Unmarshal(body, &recovery) != nil {
		t.Fatal("recovery reconciliation rejected", status)
	}
	completed = wait(recovery.ID, func(op operations.Operation) bool { return op.Status == "succeeded" })
	status, body = call("GET", "/api/v1/deployments/"+recovery.ID, token, nil, nil)
	if status != 200 || json.Unmarshal(body, &result) != nil || !result.Verified || result.TransactionID == firstTransaction || result.Health == nil || len(result.Health.Peers) != 2 {
		t.Fatal("new post-restart committed transaction missing")
	}
	if string(kub("get", "pvc", "-o", "jsonpath={range .items[*]}{.metadata.name}:{.metadata.uid}{'\\n'}{end}")) != pvcUIDs {
		t.Fatal("recovery replaced persistent claims")
	}
	for _, peer := range result.Health.Peers {
		if !peer.Available || firstHeights[peer.Peer] == 0 || peer.Height <= firstHeights[peer.Peer] {
			t.Fatal("ledger did not advance after restart")
		}
	}
	// Both persistence repositories reject a different project even with the same tenant.
	postRestartHealth, _ := json.Marshal(result.Health)
	lifecycleEvidence := []any{}
	if os.Getenv("SMP_LIFECYCLE_E2E") == "1" {
		for _, action := range []string{"add-peer", "upgrade-chaincode", "restart-peer", "verify-network"} {
			parent := completed.ID
			route := "/api/v1/deployments/" + parent + "/lifecycle/" + action
			for _, foreign := range []string{foreignTenant, foreignProject} {
				if s, _ := call("GET", route+"/preview", foreign, nil, nil); s != 404 {
					t.Fatal("foreign lifecycle preview accepted")
				}
			}
			status, body = call("GET", route+"/preview", token, nil, nil)
			var preview deployment.ChangePreview
			if status != 200 || json.Unmarshal(body, &preview) != nil || !preview.LiveHealthChecked {
				t.Fatal("lifecycle preview unavailable", status, string(body))
			}
			approved, _ := json.Marshal(map[string]string{"approval": preview.Digest})
			if s, _ := call("POST", route+"/deploy", viewer, approved, map[string]string{"Idempotency-Key": "denied-" + action + unique}); s != 403 {
				t.Fatal("viewer lifecycle escalation")
			}
			if s, _ := call("POST", route+"/deploy", token, []byte(`{"approval":"stale"}`), map[string]string{"Idempotency-Key": "stale-" + action + unique}); s != 409 {
				t.Fatal("stale preview accepted")
			}
			key := map[string]string{"Idempotency-Key": "lifecycle-" + action + unique}
			status, body = call("POST", route+"/deploy", token, approved, key)
			var change operations.Operation
			if status != 202 || json.Unmarshal(body, &change) != nil {
				t.Fatal("lifecycle apply failed", status, string(body))
			}
			status, body = call("POST", route+"/deploy", token, approved, key)
			var again operations.Operation
			if status != 202 || json.Unmarshal(body, &again) != nil || again.ID != change.ID {
				t.Fatal("lifecycle acknowledgement not idempotent")
			}
			// Adopt the same worker after an API restart during a topology change.
			server.Close()
			manager.Close()
			manager, e = deployment.OpenDatabase(ctx, db.Pool(), runner)
			if e != nil {
				t.Fatal(e)
			}
			openServer()
			completed = wait(change.ID, func(op operations.Operation) bool { return op.Status == "succeeded" })
			status, body = call("GET", "/api/v1/deployments/"+completed.ID, token, nil, nil)
			if status != 200 || json.Unmarshal(body, &result) != nil || !result.Verified || result.Health == nil || len(result.Health.Peers) != 3 {
				t.Fatal("three-peer lifecycle evidence missing", string(body))
			}
			if action == "upgrade-chaincode" && (result.Health.Chaincode.Sequence != 2 || result.Health.Chaincode.Version != "2.0") {
				t.Fatal("chaincode upgrade not verified")
			}
			checkpointHealth, _ := json.Marshal(result.Health)
			lifecycleEvidence = append(lifecycleEvidence, map[string]any{"action": action, "operationId": completed.ID, "transactionId": result.TransactionID, "health": json.RawMessage(checkpointHealth), "approvedPreview": preview, "apiRestartAdopted": true})
		}
	}
	if _, e = db.ForTenant(ctx, identity.Principal{TenantID: grant.Tenant, ProjectID: "project-b"}.StorageTenant()).Get(draft.ID); e == nil {
		t.Fatal("project isolation missing in persistence")
	}
	evidence := map[string]any{"schema": "sathonate.sprint1-api/v1", "apiToFabric": "PASS", "authenticationMode": authentication.Config().Mode, "authenticatedTenantProject": true, "duplicateOperationReused": true, "crossTenantDenied": true, "crossProjectDenied": true, "viewerEscalationDenied": true, "workerJobAdoptedAfterApplicationRestart": true, "failedWorkerRetriedThroughAPI": true, "operationId": completed.ID, "initialOperationId": first.ID, "namespace": namespace, "transactionId": result.TransactionID, "committedAndReadOnBothPeers": true, "processArguments": result.Security, "mode": result.Mode, "checkedAt": time.Now().UTC(), "productionEnabled": false, "awsEnabled": false}
	evidence["initialTransactionId"] = firstTransaction
	evidence["persistentClaimsUnchangedAfterPeerRestart"] = true
	evidence["postRestartLedgerHealth"] = json.RawMessage(postRestartHealth)
	evidence["lifecycleChanges"] = lifecycleEvidence
	raw, _ = json.MarshalIndent(evidence, "", "  ")
	if os.WriteFile(os.Getenv("SMP_SECURE_API_EVIDENCE"), raw, 0600) != nil {
		t.Fatal("redacted API evidence write failed")
	}
}
