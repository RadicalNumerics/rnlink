package golink

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"
)

// serveWithHealth exposes only readiness, never the application, on an optional
// host-network listener. Container deployments publish this port on loopback.
func (s *Server) serveWithHealth(ctx context.Context, address string, servers ...listenerServer) error {
	if address != "" {
		ln, err := net.Listen("tcp", address)
		if err != nil {
			for _, server := range servers {
				server.ln.Close()
			}
			return fmt.Errorf("health listener: %w", err)
		}
		mux := http.NewServeMux()
		mux.HandleFunc("GET /healthz", s.serveHealth)
		servers = append(servers, listenerServer{ln: ln, srv: &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}})
	}
	return serveHTTP(ctx, servers...)
}

func (s *Server) serveHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	var count int
	if err := s.db.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM Links").Scan(&count); err != nil {
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}
	if !s.dev {
		if s.lc == nil {
			http.Error(w, "tailnet unavailable", http.StatusServiceUnavailable)
			return
		}
		status, err := s.lc.Status(ctx)
		if err != nil || status == nil || status.BackendState != "Running" || status.Self == nil || !status.Self.Online {
			http.Error(w, "tailnet unavailable", http.StatusServiceUnavailable)
			return
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintln(w, "ok")
}
