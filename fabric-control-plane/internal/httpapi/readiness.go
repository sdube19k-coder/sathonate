package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sathonate.local/fabric-control-plane/internal/cloud"
	"time"
)

func (a *API) launchReadiness(w http.ResponseWriter, r *http.Request) {
	if a.options.Auth.Mode != "local-development" {
		problem(w, 404, "not_found", "Prelaunch setup is available only to the local platform administrator", nil)
		return
	}
	if r.Method == http.MethodGet {
		respond(w, 200, a.options.Readiness.Report())
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var input *struct{}
	if err := decoder.Decode(&input); err != nil || input == nil {
		problem(w, 400, "request", "Use an empty options object; configure access on the server, not in request JSON", nil)
		return
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		problem(w, 400, "request", "Use one empty options object", nil)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 75*time.Second)
	defer cancel()
	commands := a.options.Commander
	if commands == nil {
		commands = cloud.OSCommander{}
	}
	respond(w, 200, a.options.Readiness.Probe(ctx, commands))
}
