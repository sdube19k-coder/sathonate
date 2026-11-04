package httpapi

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEnrollmentCredentialsRedactedFromAPIError(t *testing.T) {
	response := httptest.NewRecorder()
	problem(response, 409, "deployment", `{"enrollsecret":"private-enrollment","pass":"private-bootstrap"} enrollSecret=private-assignment`, nil)
	for _, value := range []string{"private-enrollment", "private-bootstrap", "private-assignment"} {
		if strings.Contains(response.Body.String(), value) {
			t.Fatal("API error leaked credential")
		}
	}
}
