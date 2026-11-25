// Package archtest enforces the dependency rules from docs/adr/001: the
// domain sits at the centre and adapters depend on it, never the reverse.
package archtest

import (
	"encoding/json"
	"io"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

const module = "github.com/Hostworthy/SplitSpends/"

// std stands for every standard library package in a rule's only list.
const std = "std"

// Paths in rules are module-relative when they start with "packages" or
// "apps". A path covers itself and the packages below it, never a sibling
// that merely shares its prefix: "packages/core" is not "packages/corex".
// Rules apply to test files too.
type rule struct {
	name   string
	from   string   // the packages the rule covers
	except []string // packages under from the rule leaves out
	// only, when set, lists everything the packages may import; std is the
	// standard library.
	only   []string
	forbid []string // imports the packages may not have
	allow  []string // exceptions to forbid
	tests  []string // further exceptions for test files only
}

var rules = []rule{
	{name: "core is pure", from: "packages/core", only: []string{std, "packages/core", "github.com/google/uuid"},
		forbid: []string{"net", "os/exec", "database", "plugin", "syscall"}, allow: []string{"net/mail", "net/url", "net/netip"}},
	{name: "platform is stdlib only", from: "packages/platform", only: []string{std, "packages/platform"}},
	{name: "logger is stdlib only", from: "packages/logger", only: []string{std, "packages/logger"}},
	{name: "spa is stdlib only", from: "packages/spa", only: []string{std, "packages/spa"}},
	{name: "database serves core only", from: "packages/database", forbid: []string{"apps", "packages/mcp", "packages/bootstrap", "packages/middleware",
		"packages/platform", "packages/notifications", "packages/webpush", "packages/ai", "packages/storage", "packages/exchange"},
		tests: []string{"packages/bootstrap/bootstraptest"}},
	{name: "mcp talks to the API, not the domain", from: "packages/mcp", forbid: []string{"packages/core", "packages/database", "packages/bootstrap", "apps", "github.com/jackc"}},
	{name: "the api app gets its adapters from bootstrap", from: "apps/api", forbid: []string{"packages/database", "github.com/jackc", "packages/notifications",
		"packages/ai", "packages/storage", "packages/exchange", "packages/webpush"}, tests: []string{"packages/webpush"}},
	{name: "middleware does not touch persistence", from: "packages/middleware", forbid: []string{"packages/database", "packages/bootstrap", "apps", "github.com/jackc"}},
	{name: "notifications stay an adapter", from: "packages/notifications", forbid: []string{"packages/database", "packages/bootstrap", "packages/mcp", "apps"}},
	{name: "webpush stays an adapter", from: "packages/webpush", forbid: []string{"packages/database", "packages/bootstrap", "packages/mcp", "packages/notifications", "apps"}},
	{name: "ai stays an adapter", from: "packages/ai", forbid: []string{"packages/database", "packages/bootstrap", "packages/mcp", "apps"}},
	{name: "exchange stays an adapter", from: "packages/exchange", forbid: []string{"packages/database", "packages/bootstrap", "packages/mcp", "packages/platform", "apps"}},
	{name: "storage stays an adapter", from: "packages/storage", forbid: []string{"packages", "apps"}, allow: []string{"packages/storage"}},
	{name: "bootstrap does not depend on transports", from: "packages/bootstrap", forbid: []string{"apps", "packages/mcp", "packages/middleware"}},
	{name: "only bootstrap wires repositories", from: "packages", except: []string{"packages/bootstrap", "packages/database"},
		forbid: []string{"packages/database/repositories"}},
	{name: "apps never wire repositories", from: "apps", forbid: []string{"packages/database/repositories"}},
	{name: "packages never import apps", from: "packages", forbid: []string{"apps"}},
	{name: "the mcp app only hosts the mcp package", from: "apps/mcp", forbid: []string{"packages/core", "packages/database", "packages/bootstrap", "apps/api"}},
}

type pkg struct {
	ImportPath   string
	Imports      []string
	TestImports  []string
	XTestImports []string
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

func resolve(path string) string {
	if path == "packages" || path == "apps" || strings.HasPrefix(path, "packages/") || strings.HasPrefix(path, "apps/") {
		return module + path
	}
	return path
}

// under reports whether path is prefix or a package below it.
func under(path, prefix string) bool {
	prefix = resolve(prefix)
	return path == prefix || strings.HasPrefix(path, prefix+"/")
}

// standard reports whether path is in the standard library, whose import
// paths have no dot in their first element.
func standard(path string) bool {
	first, _, _ := strings.Cut(path, "/")
	return !strings.Contains(first, ".")
}

func matches(path string, prefixes []string) bool {
	for _, p := range prefixes {
		if under(path, p) || (p == std && standard(path)) {
			return true
		}
	}
	return false
}

// broken returns why r refuses imp in a package's code, or "" when r
// allows it.
func (r rule) broken(imp string) string {
	if r.only != nil && !matches(imp, r.only) {
		return "outside its allowed imports"
	}
	if matches(imp, r.forbid) && !matches(imp, r.allow) {
		return "forbidden"
	}
	return ""
}

// covers reports whether r applies to the package at path.
func (r rule) covers(path string) bool {
	return under(path, r.from) && !matches(path, r.except)
}

func TestBoundaries(t *testing.T) {
	pkgs := load(t)
	if len(pkgs) < 10 {
		t.Fatalf("expected the whole module, got %d packages", len(pkgs))
	}
	for _, r := range rules {
		covered := 0
		for _, p := range pkgs {
			if !r.covers(p.ImportPath) {
				continue
			}
			covered++
			check := func(imp string, test bool) {
				if imp == p.ImportPath || (test && matches(imp, r.tests)) {
					return
				}
				if why := r.broken(imp); why != "" {
					t.Errorf("%s: %s imports %s (%s)", r.name, strings.TrimPrefix(p.ImportPath, module), strings.TrimPrefix(imp, module), why)
				}
			}
			for _, imp := range p.Imports {
				check(imp, false)
			}
			for _, imp := range slices.Concat(p.TestImports, p.XTestImports) {
				check(imp, true)
			}
		}
		if covered == 0 {
			t.Errorf("%s: rule matched no packages under %s", r.name, r.from)
		}
	}
}

// TestRuleMatching pins how paths are compared, so a rule can neither spill
// onto a sibling package nor miss the one it names.
func TestRuleMatching(t *testing.T) {
	for _, c := range []struct {
		path, prefix string
		want         bool
	}{
		{module + "packages/core", "packages/core", true},
		{module + "packages/core/domains/group", "packages/core", true},
		{module + "packages/corex", "packages/core", false},
		{"net/smtp", "net", true},
		{"netx", "net", false},
		{"github.com/jackc/pgx/v5", "github.com/jackc", true},
	} {
		if got := under(c.path, c.prefix); got != c.want {
			t.Errorf("under(%q, %q) = %v, want %v", c.path, c.prefix, got, c.want)
		}
	}
	core := rules[0]
	for imp, want := range map[string]bool{
		"strings":                               true,
		"net/mail":                              true,
		"net/url":                               true,
		"github.com/google/uuid":                true,
		module + "packages/core/ports":          true,
		"net":                                   false,
		"net/http":                              false,
		"net/http/httptest":                     false,
		"net/smtp":                              false,
		"os/exec":                               false,
		"database/sql":                          false,
		"github.com/SherClockHolmes/webpush-go": false,
		module + "packages/database":            false,
		module + "packages/corex":               false,
	} {
		if got := core.broken(imp) == ""; got != want {
			t.Errorf("core importing %s: allowed = %v, want %v", imp, got, want)
		}
	}
}
