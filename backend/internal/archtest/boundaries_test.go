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

const module = "github.com/atb00ker/splitspends/backend/"

type rule struct {
	name   string
	from   string   // package path prefix, relative to the module
	forbid []string // import path prefixes (module-relative when starting with "internal/" or "cmd/")
	allow  []string // exceptions to forbid
}

var rules = []rule{
	{name: "core is pure", from: "internal/core", forbid: []string{
		"internal/", "net/http", "database/sql", "github.com/jackc", "github.com/pressly", "github.com/wneessen",
		"github.com/anthropics", "github.com/modelcontextprotocol",
	}, allow: []string{"internal/core"}},
	{name: "platform is stdlib only", from: "internal/platform", forbid: []string{"internal/", "github.com/"}, allow: []string{"internal/platform"}},
	{name: "database serves core only", from: "internal/database", forbid: []string{"internal/api", "internal/mcp", "internal/bootstrap", "internal/middleware", "internal/platform", "cmd/"}},
	{name: "mcp talks to the API, not the domain", from: "internal/mcp", forbid: []string{"internal/core", "internal/database", "internal/bootstrap", "internal/api", "github.com/jackc"}},
	{name: "api does not touch persistence", from: "internal/api", forbid: []string{"internal/database", "github.com/jackc", "internal/notifications", "internal/ai", "internal/storage"}},
	{name: "middleware does not touch persistence", from: "internal/middleware", forbid: []string{"internal/database", "internal/bootstrap", "internal/api", "github.com/jackc"}},
	{name: "adapters do not reach transports", from: "internal/notifications", forbid: []string{"internal/database", "internal/api", "internal/bootstrap", "internal/mcp"}},
	{name: "ai adapter stays an adapter", from: "internal/ai", forbid: []string{"internal/database", "internal/api", "internal/bootstrap", "internal/mcp"}},
	{name: "storage stays an adapter", from: "internal/storage", forbid: []string{"internal/"}, allow: []string{"internal/storage"}},
	{name: "bootstrap does not depend on transports", from: "internal/bootstrap", forbid: []string{"internal/api", "internal/mcp", "internal/middleware", "cmd/"}},
}

type pkg struct {
	ImportPath string
	Imports    []string
}

func load(t *testing.T) []pkg {
	t.Helper()
	cmd := exec.Command("go", "list", "-json", "../../...")
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
	if strings.HasPrefix(prefix, "internal/") || strings.HasPrefix(prefix, "cmd/") {
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
