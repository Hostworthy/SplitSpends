package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
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
