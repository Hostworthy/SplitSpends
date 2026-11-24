// Package archtest enforces the dependency rules from docs/adr/001: the
// domain sits at the centre and adapters depend on it, never the reverse.
package archtest

import (
	"encoding/json"
	"io"
	"os/exec"
	"strings"
	"testing"
)

const module = "github.com/Hostworthy/SplitSpends/"

type rule struct {
	name   string
	from   string   // package path prefix, relative to the module
	forbid []string // import path prefixes (module-relative when starting with "packages/" or "apps/")
	allow  []string // exceptions to forbid
}

var rules = []rule{
	{name: "core is pure", from: "packages/core", forbid: []string{
		"packages/", "apps/", "net/http", "database/sql", "github.com/jackc", "github.com/pressly", "github.com/wneessen",
		"github.com/anthropics", "github.com/modelcontextprotocol", "github.com/minio",
	}, allow: []string{"packages/core"}},
	{name: "platform is stdlib only", from: "packages/platform", forbid: []string{"packages/", "apps/", "github.com/"}, allow: []string{"packages/platform"}},
	{name: "database serves core only", from: "packages/database", forbid: []string{"apps/", "packages/mcp", "packages/bootstrap", "packages/middleware", "packages/platform"}},
	{name: "mcp talks to the API, not the domain", from: "packages/mcp", forbid: []string{"packages/core", "packages/database", "packages/bootstrap", "apps/", "github.com/jackc"}},
	{name: "api routes do not touch persistence", from: "apps/api/routes", forbid: []string{"packages/database", "github.com/jackc", "packages/notifications", "packages/ai", "packages/storage"}},
	{name: "middleware does not touch persistence", from: "packages/middleware", forbid: []string{"packages/database", "packages/bootstrap", "apps/", "github.com/jackc"}},
	{name: "notifications stay an adapter", from: "packages/notifications", forbid: []string{"packages/database", "packages/bootstrap", "packages/mcp", "apps/"}},
	{name: "webpush stays an adapter", from: "packages/webpush", forbid: []string{"packages/database", "packages/bootstrap", "packages/mcp", "packages/notifications", "apps/"}},
	{name: "ai stays an adapter", from: "packages/ai", forbid: []string{"packages/database", "packages/bootstrap", "packages/mcp", "apps/"}},
	{name: "storage stays an adapter", from: "packages/storage", forbid: []string{"packages/", "apps/"}, allow: []string{"packages/storage"}},
	{name: "bootstrap does not depend on transports", from: "packages/bootstrap", forbid: []string{"apps/", "packages/mcp", "packages/middleware"}},
	{name: "packages never import apps", from: "packages/", forbid: []string{"apps/"}},
	{name: "the mcp app only hosts the mcp package", from: "apps/mcp", forbid: []string{"packages/core", "packages/database", "packages/bootstrap", "apps/api"}},
}

type pkg struct {
	ImportPath string
	Imports    []string
}

func load(t *testing.T) []pkg {
	t.Helper()
	cmd := exec.Command("go", "list", "-json", "../../packages/...", "../../apps/...")
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	var pkgs []pkg
	dec := json.NewDecoder(out)
	for {
		var p pkg
		if err := dec.Decode(&p); err == io.EOF {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		pkgs = append(pkgs, p)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	return pkgs
}

func resolve(prefix string) string {
	if strings.HasPrefix(prefix, "packages/") || strings.HasPrefix(prefix, "apps/") {
		return module + prefix
	}
	return prefix
}

func matches(path string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(path, resolve(p)) {
			return true
		}
	}
	return false
}

func TestBoundaries(t *testing.T) {
	pkgs := load(t)
	if len(pkgs) < 10 {
		t.Fatalf("expected the whole module, got %d packages", len(pkgs))
	}
	for _, r := range rules {
		for _, p := range pkgs {
			if !strings.HasPrefix(p.ImportPath, module+r.from) {
				continue
			}
			for _, imp := range p.Imports {
				if matches(imp, r.forbid) && !matches(imp, r.allow) {
					t.Errorf("%s: %s imports %s", r.name, strings.TrimPrefix(p.ImportPath, module), strings.TrimPrefix(imp, module))
				}
			}
		}
	}
}
