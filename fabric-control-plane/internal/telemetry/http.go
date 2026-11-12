// Package telemetry exports bounded, process-wide service metrics.
package telemetry

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"
)

var bounds = [...]float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 15}

type Metrics struct {
	entitlements [3]uint64
	mu           sync.Mutex
	classes      [6]uint64
	histogram    [len(bounds)]uint64
	count        uint64
	sum          float64
}

var entitlementLabels = [...]string{"allowed", "denied", "unavailable"}

func (m *Metrics) ObserveEntitlement(outcome string) {
	for i, label := range entitlementLabels {
		if outcome == label {
			m.mu.Lock()
			m.entitlements[i]++
			m.mu.Unlock()
			return
		}
	}
}

func (m *Metrics) Observe(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		response := &statusWriter{ResponseWriter: w}
		returned := false
		defer func() {
			status := response.code
			if !returned {
				status = 500
			} else if status == 0 {
				status = 200
			}
			class := status / 100
			if class < 1 || class > 5 {
				class = 0
			}
			elapsed := time.Since(start).Seconds()
			m.mu.Lock()
			defer m.mu.Unlock()
			m.classes[class]++
			m.count++
			m.sum += elapsed
			for i, bound := range bounds {
				if elapsed <= bound {
					m.histogram[i]++
				}
			}
		}()
		next.ServeHTTP(response, r)
		returned = true
	})
}

type statusWriter struct {
	http.ResponseWriter
	code int
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *statusWriter) WriteHeader(code int) {
	if code >= 100 && code < 200 {
		w.ResponseWriter.WriteHeader(code)
		return
	}
	if w.code == 0 {
		w.code = code
		w.ResponseWriter.WriteHeader(code)
	}
}
func (w *statusWriter) Write(p []byte) (int, error) {
	if w.code == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(p)
}
func (w *statusWriter) FlushError() error {
	if w.code == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return http.NewResponseController(w.ResponseWriter).Flush()
}

func (m *Metrics) Handler(ready func(context.Context) error) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if ready != nil && ready(ctx) != nil {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /metrics", m.scrape)
	return mux
}
func (m *Metrics) scrape(w http.ResponseWriter, _ *http.Request) {
	m.mu.Lock()
	classes, buckets, count, sum, entitlements := m.classes, m.histogram, m.count, m.sum, m.entitlements
	m.mu.Unlock()
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	fmt.Fprintln(w, "# HELP sathonate_http_requests_total Completed API responses by status class.\n# TYPE sathonate_http_requests_total counter")
	for class, value := range classes {
		if class == 0 {
			fmt.Fprintf(w, "sathonate_http_requests_total{status_class=\"other\"} %d\n", value)
			continue
		}
		fmt.Fprintf(w, "sathonate_http_requests_total{status_class=\"%dxx\"} %d\n", class, value)
	}
	fmt.Fprintln(w, "# HELP sathonate_customer_entitlement_checks_total Customer deployment entitlement decisions.\n# TYPE sathonate_customer_entitlement_checks_total counter")
	for i, label := range entitlementLabels {
		fmt.Fprintf(w, "sathonate_customer_entitlement_checks_total{result=%q} %d\n", label, entitlements[i])
	}
	fmt.Fprintln(w, "# HELP sathonate_http_request_duration_seconds API handler latency.\n# TYPE sathonate_http_request_duration_seconds histogram")
	for i, bound := range bounds {
		fmt.Fprintf(w, "sathonate_http_request_duration_seconds_bucket{le=%q} %d\n", fmt.Sprint(bound), buckets[i])
	}
	fmt.Fprintf(w, "sathonate_http_request_duration_seconds_bucket{le=\"+Inf\"} %d\nsathonate_http_request_duration_seconds_sum %g\nsathonate_http_request_duration_seconds_count %d\n", count, sum, count)
}
