package main

import (
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestConsoleRoutes(t *testing.T) {
	handler := console(fstest.MapFS{"index.html": {Data: []byte("console")}, "assets/main.js": {Data: []byte("asset")}})
	for _, tc := range []struct {
		path   string
		status int
		body   string
	}{
		{"/", 200, "console"}, {"/login/callback?code=fixture", 200, "console"},
		{"/networks", 200, "console"}, {"/assets/main.js", 200, "asset"},
		{"/api", 404, ""}, {"/api/not-found", 404, ""}, {"/assets/missing.js", 404, ""},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", tc.path, nil))
		if response.Code != tc.status || (tc.body != "" && response.Body.String() != tc.body) {
			t.Fatalf("%s returned %d %q", tc.path, response.Code, response.Body.String())
		}
	}
}
