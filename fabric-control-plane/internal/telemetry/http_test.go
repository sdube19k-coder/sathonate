package telemetry

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetricsAndReadiness(t *testing.T) {
	var m Metrics
	api := m.Observe(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			w.WriteHeader(503)
			w.WriteHeader(200)
			return
		}
		if err := http.NewResponseController(w).Flush(); err != nil {
			t.Fatal(err)
		}
	}))
	for _, method := range []string{"GET", "POST"} {
		api.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(method, "/api/tenant-private", nil))
	}
	ops := m.Handler(func(ctx context.Context) error {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("readiness deadline missing")
		}
		return errors.New("private-db")
	})
	readiness := httptest.NewRecorder()
	ops.ServeHTTP(readiness, httptest.NewRequest("GET", "/readyz", nil))
	if readiness.Code != 503 || strings.Contains(readiness.Body.String(), "private-db") {
		t.Fatal("unexpected readiness response")
	}
	scrape := httptest.NewRecorder()
	ops.ServeHTTP(scrape, httptest.NewRequest("GET", "/metrics", nil))
	body := scrape.Body.String()
	for _, want := range []string{`status_class="2xx"} 1`, `status_class="5xx"} 1`, `le="+Inf"} 2`, "sathonate_http_request_duration_seconds_count 2"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %s", want)
		}
	}
	if strings.Contains(body, "tenant-private") {
		t.Fatal("metrics disclose request path")
	}
}
