package archtest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// key is a log message: lowercase dotted segments, kebab-case inside one.
var key = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*(\.[a-z0-9]+(-[a-z0-9]+)*)+$`)

// levels are the slog calls whose message is a key, with where it sits.
var levels = map[string]int{
	"Debug": 0, "Info": 0, "Warn": 0, "Error": 0,
	"DebugContext": 1, "InfoContext": 1, "WarnContext": 1, "ErrorContext": 1,
}

// TestLogKeys checks that every log message written as a literal is a
// dotted key, with details left to attributes. Calls are recognised by
// name, so a literal handed to another Info or Error method counts too.
func TestLogKeys(t *testing.T) {
	checked := 0
	for _, root := range []string{"../../packages", "../../apps"} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return err
			}
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				at, ok := levels[sel.Sel.Name]
				if !ok || len(call.Args) <= at {
					return true
				}
				lit, ok := call.Args[at].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				msg, err := strconv.Unquote(lit.Value)
				if err != nil {
					return true
				}
				checked++
				if !key.MatchString(msg) {
					t.Errorf("%s: log message %q is not a dotted key such as api.server.listening", fset.Position(lit.Pos()), msg)
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if checked < 20 {
		t.Fatalf("found only %d log calls; the scan is not reading the code", checked)
	}
}
