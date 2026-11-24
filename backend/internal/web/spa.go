// Package web serves the compiled Flutter web app with a single-page-app
// fallback, so deep links such as /groups/123 load index.html.
package web

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Handler serves dir. Unknown paths without a file extension get index.html.
func Handler(dir string) http.Handler {
	files := http.FileServer(http.Dir(dir))
	index := filepath.Join(dir, "index.html")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := path.Clean("/" + r.URL.Path)
		full := filepath.Join(dir, filepath.FromSlash(clean))
		if info, err := os.Stat(full); err == nil && !info.IsDir() {
			switch {
			case strings.HasSuffix(clean, ".html"), clean == "/flutter_service_worker.js", clean == "/version.json", clean == "/manifest.json":
				w.Header().Set("Cache-Control", "no-cache")
			case strings.HasPrefix(clean, "/assets/"), strings.HasPrefix(clean, "/canvaskit/"), strings.HasPrefix(clean, "/icons/"), strings.HasPrefix(clean, "/fonts/"):
				w.Header().Set("Cache-Control", "public, max-age=604800")
			}
			files.ServeHTTP(w, r)
			return
		}
		if filepath.Ext(clean) != "" && clean != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, index)
	})
}
