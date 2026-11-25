package archtest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// deployFiles set variables for the server, each found by its pattern.
var deployFiles = map[string]*regexp.Regexp{
	"../../deploy/aws/main.tf":               regexp.MustCompile(`(?m)^\s*([A-Z][A-Z0-9_]+)\s*=`),
	"../../deploy/gcp/main.tf":               regexp.MustCompile(`(?m)^\s*([A-Z][A-Z0-9_]+)\s*=`),
	"../../docker-compose.yml":               regexp.MustCompile(`(?m)^\s+([A-Z][A-Z0-9_]+):`),
	"../../deploy/docker-compose.yml":        regexp.MustCompile(`(?m)^\s+([A-Z][A-Z0-9_]+):`),
	"../../deploy/docker/docker-compose.yml": regexp.MustCompile(`(?m)^\s+([A-Z][A-Z0-9_]+):`),
	"../../render.yaml":                      regexp.MustCompile(`key:\s*([A-Z][A-Z0-9_]+)`),
	"../../apps/api/.env.example":            regexp.MustCompile(`(?m)^#?\s*([A-Z][A-Z0-9_]+)=`),
	"../../deploy/.env.example":              regexp.MustCompile(`(?m)^#?\s*([A-Z][A-Z0-9_]+)=`),
}

// images are variables that the images and tools beside the server read,
// documented with their stacks rather than in docs/ENVIRONMENT.md.
var images = []string{
	"AWS_LAMBDA_EXEC_WRAPPER", "AWS_LWA_PORT", "AWS_LWA_READINESS_CHECK_PATH", // Lambda Web Adapter
	"POSTGRES_DB", "POSTGRES_USER", "POSTGRES_PASSWORD", "PGPASSWORD", // PostgreSQL image and pg_dump
	"DOMAIN", // Caddy
}

// TestEnvDocumented checks that docs/ENVIRONMENT.md, the variables the
// server reads and the ones the deployment files set agree, so a misspelt
// name in a template or a new setting without documentation is caught.
func TestEnvDocumented(t *testing.T) {
	doc, err := os.ReadFile("../../docs/ENVIRONMENT.md")
	if err != nil {
		t.Fatal(err)
	}
	mentioned := map[string]bool{}
	for _, m := range regexp.MustCompile("`([A-Z][A-Z0-9_]+)").FindAllStringSubmatch(string(doc), -1) {
		mentioned[m[1]] = true
	}
	read := serverReads(t)
	set := map[string]bool{}
	for path, pattern := range deployFiles {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range pattern.FindAllStringSubmatch(string(data), -1) {
			name := m[1]
			set[name] = true
			if !mentioned[name] && !slices.Contains(images, name) {
				t.Errorf("%s sets %s, which docs/ENVIRONMENT.md does not describe", path, name)
			}
		}
	}
	for name := range read {
		if !mentioned[name] {
			t.Errorf("the server reads %s, which docs/ENVIRONMENT.md does not describe", name)
		}
	}
	rows := 0
	for _, line := range strings.Split(string(doc), "\n") {
		if !strings.HasPrefix(line, "| `") {
			continue
		}
		cell := strings.SplitN(line, "|", 3)[1]
		for _, m := range regexp.MustCompile("`([A-Z][A-Z0-9_]+)`").FindAllStringSubmatch(cell, -1) {
			rows++
			if !read[m[1]] && !set[m[1]] {
				t.Errorf("docs/ENVIRONMENT.md describes %s, which neither the server nor a deployment file uses", m[1])
			}
		}
	}
	if len(read) < 30 || len(set) < 30 || rows < 30 {
		t.Fatalf("found %d variables read, %d set and %d documented; the scan is not reading the files", len(read), len(set), rows)
	}
}

// serverReads lists the variables packages/bootstrap reads through its env
// and flag helpers.
func serverReads(t *testing.T) map[string]bool {
	t.Helper()
	read := map[string]bool{}
	paths, err := filepath.Glob("../../packages/bootstrap/*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			if fn, ok := call.Fun.(*ast.Ident); !ok || (fn.Name != "env" && fn.Name != "flag") {
				return true
			}
			if lit, ok := call.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
				if name, err := strconv.Unquote(lit.Value); err == nil {
					read[name] = true
				}
			}
			return true
		})
	}
	return read
}
