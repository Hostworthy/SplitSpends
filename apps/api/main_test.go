package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProbe(t *testing.T) {
	tests := []struct {
		name   string
		status int
		ok     bool
	}{
		{"healthy", http.StatusOK, true},
		{"database down", http.StatusServiceUnavailable, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/health" {
					http.NotFound(w, r)
					return
				}
				w.WriteHeader(tt.status)
			}))
			defer srv.Close()
			if err := probe(srv.URL); (err == nil) != tt.ok {
				t.Fatalf("probe() = %v, want ok %v", err, tt.ok)
			}
		})
	}
}

func TestProbeUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	if err := probe(url); err == nil {
		t.Fatal("probe() = nil for a server that is not running")
	}
}

// TestServerLimits checks that a request cannot hold a connection forever,
// and that the write deadline leaves room for the longest handler: a run of
// POST /internal/jobs, which may take 5 minutes.
func TestServerLimits(t *testing.T) {
	srv := newServer(":0", http.NotFoundHandler())
	if srv.ReadHeaderTimeout == 0 || srv.ReadTimeout == 0 || srv.IdleTimeout == 0 {
		t.Errorf("every read needs a deadline: header %v, request %v, idle %v", srv.ReadHeaderTimeout, srv.ReadTimeout, srv.IdleTimeout)
	}
	if srv.MaxHeaderBytes == 0 || srv.MaxHeaderBytes > 1<<20 {
		t.Errorf("MaxHeaderBytes %d, want an explicit limit below the default", srv.MaxHeaderBytes)
	}
	if srv.WriteTimeout != 0 && srv.WriteTimeout <= 5*time.Minute {
		t.Errorf("WriteTimeout %v would cut off long handlers", srv.WriteTimeout)
	}
}
