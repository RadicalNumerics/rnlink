package golink

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthReadiness(t *testing.T) {
	s := newTestServer(t)
	check := func(want int) {
		t.Helper()
		w := httptest.NewRecorder()
		s.serveHealth(w, httptest.NewRequest("GET", "/healthz", nil))
		if w.Code != want {
			t.Fatalf("status %d, want %d: %s", w.Code, want, w.Body.String())
		}
	}
	check(http.StatusOK)
	s.dev = false
	check(http.StatusServiceUnavailable)
	s.dev = true
	s.db.db.Close()
	check(http.StatusServiceUnavailable)
}
