package customer

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

func DecodeCommand(body io.Reader) (Command, error) {
	var command Command
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	var extra any
	if decoder.Decode(&command) != nil || decoder.Decode(&extra) != io.EOF {
		return Command{}, ErrInvalid
	}
	return command, nil
}

func Status(err error) int {
	switch {
	case err == nil:
		return http.StatusNoContent
	case errors.Is(err, ErrDenied):
		return http.StatusForbidden
	case errors.Is(err, ErrInvalid):
		return http.StatusBadRequest
	case errors.Is(err, ErrConflict):
		return http.StatusConflict
	default:
		return http.StatusServiceUnavailable
	}
}

// Webhook builds a standalone ingress handler for an explicitly supplied
// verifier. The main API does not register a payment ingress route.
func (s Service) Webhook(verifier PaymentVerifier) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 16384))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		signatures := r.Header.Values("Payment-Signature")
		if len(signatures) != 1 {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.WriteHeader(Status(s.ApplyPayment(r.Context(), verifier, signatures[0], body)))
	})
}
