package golink

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"tailscale.com/client/local"
	"tailscale.com/ipn/ipnstate"
	"testing"
	"time"
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

func TestHealthReportsCurrentNodeKeyExpiry(t *testing.T) {
	s := newTestServer(t)
	s.dev = false
	expiry := time.Now().UTC().Add(6 * 24 * time.Hour).Truncate(time.Second)
	status := ipnstate.Status{BackendState: "Running", Self: &ipnstate.PeerStatus{Online: true, KeyExpiry: &expiry}}
	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(status)
	}))
	defer daemon.Close()
	s.lc = &local.Client{OmitAuth: true, Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", daemon.Listener.Addr().String())
	}}
	check := func(want string) {
		t.Helper()
		w := httptest.NewRecorder()
		s.serveHealth(w, httptest.NewRequest("GET", "/healthz", nil))
		if w.Code != http.StatusOK || w.Header().Get("X-RN-Node-Key-Expiry") != want {
			t.Fatalf("status=%d expiry=%q want=%q", w.Code, w.Header().Get("X-RN-Node-Key-Expiry"), want)
		}
	}
	check(expiry.Format(time.RFC3339))
	expiry = expiry.Add(180 * 24 * time.Hour)
	check(expiry.Format(time.RFC3339))
	status.Self.KeyExpiry = nil
	check("disabled")
}
