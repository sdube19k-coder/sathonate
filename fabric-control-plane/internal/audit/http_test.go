package audit

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type failedSink struct{}

func (failedSink) Write([]byte) (int, error) { return 0, errors.New("unavailable") }
func TestAuditDenialsAndSensitiveDataExcluded(t *testing.T) {
	var log bytes.Buffer
	h := New(&log).Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		Bind(r.Context(), "private-subject", "tenant-a", "project-a")
		w.WriteHeader(403)
	}))
	r := httptest.NewRequest("POST", "http://localhost/api/private-id?token=private-query", strings.NewReader("private-body"))
	r.Header.Set("Authorization", "Bearer private-token")
	h.ServeHTTP(httptest.NewRecorder(), r)
	for _, value := range []string{"private-subject", "tenant-a", "project-a", "private-id", "private-query", "private-body", "private-token", "Authorization"} {
		if strings.Contains(log.String(), value) {
			t.Fatal("sensitive request data logged")
		}
	}
	if !strings.Contains(log.String(), `"status":403`) || !strings.Contains(log.String(), `"phase":"attempt"`) || !strings.Contains(log.String(), `"actorSHA256"`) {
		t.Fatal("audit outcome missing")
	}
}
func TestFailedAttemptPreventsMutation(t *testing.T) {
	called := false
	h := New(failedSink{}).Wrap(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("DELETE", "/networks/n", nil))
	if called || w.Code != 503 {
		t.Fatal("dispatch occurred without audit attempt")
	}
}
func TestAuditPreservesResponseController(t *testing.T) {
	var log bytes.Buffer
	w := httptest.NewRecorder()
	New(&log).Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if http.NewResponseController(w).Flush() != nil {
			t.Error("response control lost")
		}
		w.WriteHeader(204)
	})).ServeHTTP(w, httptest.NewRequest("GET", "/events", nil))
	if !w.Flushed {
		t.Fatal("stream not flushed")
	}
}
