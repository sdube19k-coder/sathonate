// Package audit records server-generated metadata, never tokens, URLs or bodies.
// The sink must be protected and retained by the deployment operator.
package audit

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"
)

type Event struct {
	Schema      string    `json:"schema"`
	RequestID   string    `json:"requestId"`
	Phase       string    `json:"phase"`
	At          time.Time `json:"at"`
	Method      string    `json:"method"`
	PathSHA256  string    `json:"pathSHA256"`
	ActorSHA256 string    `json:"actorSHA256,omitempty"`
	ScopeSHA256 string    `json:"scopeSHA256,omitempty"`
	Status      int       `json:"status,omitempty"`
}
type Journal struct {
	mu   sync.Mutex
	sink io.Writer
}
type metadata struct{ actor, scope string }
type key struct{}

func New(sink io.Writer) *Journal { return &Journal{sink: sink} }
func digest(v string) string      { b := sha256.Sum256([]byte(v)); return hex.EncodeToString(b[:]) }

// Bind accepts only the principal resolved by the server's authentication layer.
func Bind(ctx context.Context, subject, tenant, project string) {
	if m, ok := ctx.Value(key{}).(*metadata); ok && subject != "" {
		actor, _ := json.Marshal([]string{subject, tenant, project})
		scope, _ := json.Marshal([]string{tenant, project})
		m.actor = digest(string(actor))
		m.scope = digest(string(scope))
	}
}
func (j *Journal) append(e Event) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	return json.NewEncoder(j.sink).Encode(e)
}

type response struct {
	http.ResponseWriter
	status int
}

func (w *response) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *response) FlushError() error {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return http.NewResponseController(w.ResponseWriter).Flush()
}

func (w *response) WriteHeader(s int) {
	if s >= 100 && s < 200 {
		w.ResponseWriter.WriteHeader(s)
		return
	}
	if w.status != 0 {
		return
	}
	w.status = s
	w.ResponseWriter.WriteHeader(s)
}
func (w *response) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}
func (j *Journal) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var id [16]byte
		if _, err := rand.Read(id[:]); err != nil {
			http.Error(w, "audit unavailable", 503)
			return
		}
		method := r.Method
		switch method {
		case "GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS":
		default:
			method = "OTHER"
		}
		e := Event{Schema: "sathonate.audit/v1", RequestID: hex.EncodeToString(id[:]), Phase: "attempt", At: time.Now().UTC(), Method: method, PathSHA256: digest(r.URL.EscapedPath())}
		// Refuse dispatch if the attempt cannot be journalled. Completion failure
		// cannot roll back an already committed Fabric operation; retain the attempt.
		if j.append(e) != nil {
			http.Error(w, "audit unavailable", 503)
			return
		}
		m := &metadata{}
		r = r.WithContext(context.WithValue(r.Context(), key{}, m))
		rw := &response{ResponseWriter: w}
		completed := false
		defer func() {
			e.At = time.Now().UTC()
			e.Phase = "completion"
			e.ActorSHA256 = m.actor
			e.ScopeSHA256 = m.scope
			e.Status = rw.status
			if !completed {
				e.Phase = "interrupted"
				if e.Status == 0 {
					e.Status = 500
				}
			} else if e.Status == 0 {
				e.Status = 200
			}
			// Leave the attempt unmatched if the sink fails. Never retry a mutation or
			// fabricate completion. Sink durability/alerting is a production gate.
			_ = j.append(e)
		}()
		next.ServeHTTP(rw, r)
		completed = true
	})
}
