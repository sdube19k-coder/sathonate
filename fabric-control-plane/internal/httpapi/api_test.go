package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"sathonate.local/fabric-control-plane/internal/drafts"
)

func handler(t *testing.T) http.Handler {
	t.Helper()
	store, e := drafts.Open(filepath.Join(t.TempDir(), "data.json"))
	if e != nil {
		t.Fatal(e)
	}
	return New(store, []string{"localhost:8080"}, []string{"http://localhost:5173"})
}
func input(t *testing.T) []byte {
	t.Helper()
	b, e := os.ReadFile("../../examples/native-network.json")
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func request(h http.Handler, method, path string, body []byte, headers map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://localhost:8080"+path, bytes.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func TestHTTPDraftFlow(t *testing.T) {
	h := handler(t)
	created := request(h, "POST", "/api/v1/networks", input(t), nil)
	if created.Code != 201 {
		t.Fatal(created.Code, created.Body.String())
	}
	var draft drafts.Draft
	if e := json.Unmarshal(created.Body.Bytes(), &draft); e != nil {
		t.Fatal(e)
	}
	if draft.Status != "draft" || draft.Plan != nil {
		t.Fatal("draft pretending to be deployed")
	}
	route := "/api/v1/networks/" + draft.ID
	reviewed := request(h, "POST", route+"/review", []byte("{}"), map[string]string{"If-Match": "\"1\""})
	if reviewed.Code != 200 {
		t.Fatal(reviewed.Code, reviewed.Body.String())
	}
	updated := request(h, "PUT", route, input(t), map[string]string{"If-Match": "\"1\""})
	if updated.Code != 200 {
		t.Fatal(updated.Code, updated.Body.String())
	}
	stale := request(h, "PUT", route, input(t), map[string]string{"If-Match": "\"1\""})
	if stale.Code != 409 {
		t.Fatal("stale update not rejected", stale.Code)
	}
	missing := request(h, "PUT", route, input(t), nil)
	if missing.Code != 428 {
		t.Fatal("missing revision not rejected")
	}
	archived := request(h, "DELETE", route, nil, map[string]string{"If-Match": "\"2\""})
	if archived.Code != 204 {
		t.Fatal("archive failed", archived.Code)
	}
	list := request(h, "GET", "/api/v1/networks", nil, nil)
	if list.Code != 200 || !bytes.Contains(list.Body.Bytes(), []byte(`"items":[]`)) {
		t.Fatal("list did not exclude archived draft")
	}
}
func TestHTTPInputAndBrowserBoundaries(t *testing.T) {
	h := handler(t)
	for _, tc := range []struct {
		name, method, path string
		body               []byte
		headers            map[string]string
		status             int
	}{
		{"inline secret rejected", "POST", "/api/v1/networks", []byte(`{"password":"secret"}`), nil, 400},
		{"unsafe topology rejected", "POST", "/api/v1/networks", []byte(`{}`), nil, 422},
		{"malicious origin rejected", "POST", "/api/v1/networks", input(t), map[string]string{"Origin": "https://evil.example"}, 403},
		{"form posts rejected", "POST", "/api/v1/networks", input(t), map[string]string{"Content-Type": "text/plain"}, 415},
		{"missing route", "GET", "/api/v1/no-such-route", nil, nil, 404},
		{"oversized body", "POST", "/api/v1/networks", bytes.Repeat([]byte(" "), (1<<20)+10), nil, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := request(h, tc.method, tc.path, tc.body, tc.headers)
			if w.Code != tc.status {
				t.Fatalf("got %d: %s", w.Code, w.Body.String())
			}
		})
	}
	r := httptest.NewRequest("GET", "http://attacker.example/api/v1/networks", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("untrusted host accepted")
	}
	meta := request(h, "GET", "/api/v1/meta", nil, nil)
	if !bytes.Contains(meta.Body.Bytes(), []byte(`"executionEnabled":false`)) {
		t.Fatal("API must state execution is disabled")
	}
}
