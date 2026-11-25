package spa

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandler(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "web")
	files := map[string]string{
		"../secret.txt": "secret",
		"index.html":    "<html>",
		"fonts/fallback/notocoloremoji/v39/a.woff2": "font",
		"icons/Icon-192.png":                        "icon",
		"flutter_bootstrap.js":                      "bootstrap",
		"flutter.js":                                "flutter",
		"main.dart.js":                              "main",
		"push_sw.js":                                "push",
		"assets/AssetManifest.bin.json":             "manifest",
		"canvaskit/canvaskit.wasm":                  "wasm",
	}
	for name, body := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
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
		{"/icons/Icon-192.png", http.StatusOK, "public, max-age=604800", "icon"},
		{"/groups/123", http.StatusOK, "no-cache", "<html>"},
		// Flutter names its build output without a content hash, so
		// everything the app loads with its code is revalidated.
		{"/flutter_bootstrap.js", http.StatusOK, "no-cache", "bootstrap"},
		{"/flutter.js", http.StatusOK, "no-cache", "flutter"},
		{"/main.dart.js", http.StatusOK, "no-cache", "main"},
		{"/push_sw.js", http.StatusOK, "no-cache", "push"},
		{"/assets/AssetManifest.bin.json", http.StatusOK, "no-cache", "manifest"},
		{"/canvaskit/canvaskit.wasm", http.StatusOK, "no-cache", "wasm"},
		{"/assets/", http.StatusOK, "no-cache", "<html>"},
		{"/index.html", http.StatusMovedPermanently, "no-cache", ""},
		{"/../secret.txt", http.StatusNotFound, "", ""},
		{"/../../etc/passwd", http.StatusBadRequest, "", ""},
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
			if strings.Contains(rec.Body.String(), "secret") {
				t.Errorf("served a file outside the web directory")
			}
		})
	}
}
