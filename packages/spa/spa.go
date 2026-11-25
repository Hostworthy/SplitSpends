// Package spa serves the compiled Flutter web app with a single-page-app
// fallback, so deep links such as /groups/123 load index.html.
package spa

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
			w.Header().Set("Cache-Control", cacheControl(clean))
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

// cacheControl keeps browsers from running an old build after an upgrade.
// Flutter names its output without a content hash, so every file the app
// loads with its code (the scripts, assets and canvaskit) is revalidated on
// each load. Only fonts and icons are cached for a week: they rarely change
// and a stale copy does not break the app. deploy/aws/main.tf applies the
// same rule to the files it uploads; change both together.
func cacheControl(clean string) string {
	if strings.HasPrefix(clean, "/fonts/") || strings.HasPrefix(clean, "/icons/") {
		return "public, max-age=604800"
	}
	return "no-cache"
}
