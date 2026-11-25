package main

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/bootstrap"
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

func TestLoopback(t *testing.T) {
	tests := []struct{ addr, want string }{
		{":8080", "http://127.0.0.1:8080"},
		{"0.0.0.0:80", "http://127.0.0.1:80"},
		{"[::]:8080", "http://127.0.0.1:8080"},
		{"127.0.0.1:9000", "http://127.0.0.1:9000"},
		{"[::1]:9000", "http://[::1]:9000"},
		{"localhost:3000", "http://localhost:3000"},
		{"8080", "http://127.0.0.1:8080"},
		{"", "http://127.0.0.1:8080"},
	}
	for _, tt := range tests {
		if got := loopback(tt.addr); got != tt.want {
			t.Errorf("loopback(%q) = %q, want %q", tt.addr, got, tt.want)
		}
	}
}

// TestMCPLimit checks that /mcp counts requests per client address before
// any token check: the request after mcpLimit in a minute is refused, while
// another address is still served.
func TestMCPLimit(t *testing.T) {
	h := mcpHandler(&bootstrap.Config{Addr: ":0"}, slog.New(slog.DiscardHandler))
	send := func(addr string) int {
		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader("{}"))
		req.RemoteAddr = addr + ":1234"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	for i := range mcpLimit {
		if code := send("203.0.113.1"); code != http.StatusUnauthorized {
			t.Fatalf("request %d answered %d, want 401", i+1, code)
		}
	}
	if code := send("203.0.113.1"); code != http.StatusTooManyRequests {
		t.Fatalf("request over the limit answered %d, want 429", code)
	}
	if code := send("203.0.113.2"); code != http.StatusUnauthorized {
		t.Fatalf("another address answered %d, want 401", code)
	}
}
