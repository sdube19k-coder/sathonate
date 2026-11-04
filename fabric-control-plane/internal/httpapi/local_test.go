package httpapi

import (
	"bytes"
	"testing"

	"sathonate.local/fabric-control-plane/internal/identity"
	"sathonate.local/fabric-control-plane/internal/localfabric"
	"sathonate.local/fabric-control-plane/internal/operations"
)

func TestLocalRuntimeCannotBeExposedInProduction(t *testing.T) {
	local, e := localfabric.New(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	h := NewConfigured(nil, []string{"localhost:8080"}, nil, Options{Auth: identity.Config{Mode: "okta"}, Local: local})
	for _, tc := range []struct{ method, path string }{{"GET", "/api/v1/local-runtime"}, {"GET", "/api/v1/local-runtime/health"}, {"GET", "/api/v1/operations"}, {"GET", "/api/v1/operations/op_unknown"}, {"GET", "/api/v1/operations/op_unknown/events"}, {"POST", "/api/v1/operations/op_unknown/retry"}, {"POST", "/api/v1/local-runtime/start"}, {"POST", "/api/v1/local-runtime/stop"}, {"POST", "/api/v1/local-runtime/test"}, {"GET", "/api/v1/local-runtime/logs/peer1"}} {
		w := request(h, tc.method, tc.path, nil, nil)
		if w.Code != 404 {
			t.Fatal("production local execution enabled", tc.path, w.Code)
		}
	}
}

func TestOperationHTTPIsolationAndEventCursor(t *testing.T) {
	local, e := localfabric.New(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	jobs := local.Operations()
	stages := []operations.Stage{{Key: "validate", Label: "Validate"}}
	own, _, e := jobs.Begin("local-development", "local-developer", "local", "verify", "private-dedup-key", "digest", "version", "", stages)
	if e != nil {
		t.Fatal(e)
	}
	foreign, _, e := jobs.Begin("foreign-tenant", "operator", "remote", "verify", "foreign-key", "digest", "version", "", stages)
	if e != nil {
		t.Fatal(e)
	}
	if e = jobs.Advance(own.ID, "validate"); e != nil {
		t.Fatal(e)
	}
	if e = jobs.Event(own.ID, "Bearer secret-journal-token"); e != nil {
		t.Fatal(e)
	}
	h := NewConfigured(nil, []string{"localhost:8080"}, nil, Options{Auth: identity.Config{Mode: "local-development"}, Local: local})
	for _, suffix := range []string{"", "/events"} {
		w := request(h, "GET", "/api/v1/operations/"+foreign.ID+suffix, nil, nil)
		if w.Code != 404 {
			t.Fatal("cross-tenant operation exposed", w.Code)
		}
	}
	retry := request(h, "POST", "/api/v1/operations/"+foreign.ID+"/retry", nil, nil)
	if retry.Code != 404 {
		t.Fatal("cross-tenant retry accepted")
	}
	list := request(h, "GET", "/api/v1/operations", nil, nil)
	if list.Code != 200 || bytes.Contains(list.Body.Bytes(), []byte(foreign.ID)) || bytes.Contains(list.Body.Bytes(), []byte("private-dedup-key")) || bytes.Contains(list.Body.Bytes(), []byte("secret-journal-token")) {
		t.Fatal("operation response leaked private data", list.Body.String())
	}
	events := request(h, "GET", "/api/v1/operations/"+own.ID+"/events?after=1", nil, nil)
	if events.Code != 200 || bytes.Contains(events.Body.Bytes(), []byte(`"sequence":1,`)) {
		t.Fatal("event cursor ignored")
	}
	invalid := request(h, "GET", "/api/v1/operations/"+own.ID+"/events?after=-1", nil, nil)
	if invalid.Code != 400 {
		t.Fatal("invalid cursor accepted")
	}
}
