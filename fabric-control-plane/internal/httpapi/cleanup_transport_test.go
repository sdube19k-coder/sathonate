package httpapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCleanupResponseSurvivesOrdinaryServerWriteDeadline(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := cleanupResponseDeadline(w, time.Now().Add(time.Second)); err != nil {
			t.Error(err)
			return
		}
		time.Sleep(100 * time.Millisecond)
		w.Write([]byte("acknowledged"))
	}))
	server.Config.WriteTimeout = 20 * time.Millisecond
	server.Start()
	defer server.Close()
	response, err := server.Client().Get(server.URL)
	if err != nil {
		t.Fatal("cleanup acknowledgement lost at ordinary deadline:", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || string(body) != "acknowledged" {
		t.Fatal("cleanup response incomplete")
	}
}
