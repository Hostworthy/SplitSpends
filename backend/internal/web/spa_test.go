package web

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHandler(t *testing.T) {
	dir := t.TempDir()
	font := filepath.Join(dir, "fonts", "fallback", "notocoloremoji", "v39", "a.woff2")
	if err := os.MkdirAll(filepath.Dir(font), 0o755); err != nil {
		t.Fatal(err)
	}
	for path, body := range map[string]string{font: "font", filepath.Join(dir, "index.html"): "<html>"} {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h := Handler(dir)

	tests := []struct {
		path   string
		status int
		cache  string
		body   string
	}{
		{"/fonts/fallback/notocoloremoji/v39/a.woff2", http.StatusOK, "public, max-age=604800", "font"},
		{"/fonts/fallback/notocoloremoji/v39/missing.woff2", http.StatusNotFound, "", ""},
		{"/groups/123", http.StatusOK, "no-cache", "<html>"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if rec.Code != tt.status {
				t.Fatalf("status = %d, want %d", rec.Code, tt.status)
			}
			if got := rec.Header().Get("Cache-Control"); got != tt.cache {
				t.Errorf("Cache-Control = %q, want %q", got, tt.cache)
			}
			if tt.body != "" && rec.Body.String() != tt.body {
				t.Errorf("body = %q, want %q", rec.Body.String(), tt.body)
			}
		})
	}
}
