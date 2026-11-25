package platform

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// TestSharedShapesDocumented checks that every field the server sends in
// the shared shapes appears in the Shared Shapes section of docs/API.md,
// which clients other than the app are written against.
func TestSharedShapesDocumented(t *testing.T) {
	shapes := sharedShapes(t)
	for name, v := range map[string]any{
		"User": User{}, "Me": Me{}, "PushSubscription": PushSubscription{}, "Balance": Balance{}, "Debt": Debt{},
		"Group": Group{}, "DefaultSplit": DefaultSplit{}, "Category": Category{}, "Expense": Expense{},
		"Activity": Activity{}, "Account": Account{},
	} {
		doc, ok := shapes[name]
		if !ok {
			t.Errorf("docs/API.md has no %s shape", name)
			continue
		}
		for _, field := range fields(reflect.TypeOf(v)) {
			if !strings.Contains(doc, strconv.Quote(field)) {
				t.Errorf("docs/API.md: the %s shape does not mention %q", name, field)
			}
		}
	}
}

// TestUserStatusesDocumented checks that every user.Status the server can
// send is listed in the User or Account shape of docs/API.md.
func TestUserStatusesDocumented(t *testing.T) {
	shapes := sharedShapes(t)
	doc := shapes["User"] + shapes["Account"]
	file, err := parser.ParseFile(token.NewFileSet(), "../core/domains/user/user.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok {
			return true
		}
		if id, ok := spec.Type.(*ast.Ident); !ok || id.Name != "Status" {
			return true
		}
		for _, value := range spec.Values {
			if lit, ok := value.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				seen++
				if !strings.Contains(doc, lit.Value) {
					t.Errorf("docs/API.md: no User or Account shape lists the status %s", lit.Value)
				}
			}
		}
		return true
	})
	if seen == 0 {
		t.Fatal("no user statuses found; the test no longer looks at user.Status")
	}
}

// sharedShapes splits the Shared Shapes block of docs/API.md into the text
// of each shape. A shape starts with a comment after a blank line, and the
// comment's first word is its name.
func sharedShapes(t *testing.T) map[string]string {
	t.Helper()
	raw, err := os.ReadFile("../../docs/API.md")
	if err != nil {
		t.Fatal(err)
	}
	_, block, ok := strings.Cut(string(raw), "## Shared Shapes\n\n```jsonc\n")
	if !ok {
		t.Fatal("docs/API.md has no Shared Shapes block")
	}
	block, _, _ = strings.Cut(block, "```")
	shapes := map[string]string{}
	name, starts := "", true
	for line := range strings.Lines(block) {
		if heading, ok := strings.CutPrefix(line, "// "); ok && starts {
			name = strings.Fields(heading)[0]
		}
		starts = strings.TrimSpace(line) == ""
		shapes[name] += line
	}
	return shapes
}

// fields lists the JSON names of t's fields. Embedded structs are left out:
// the docs write them as a spread (...User) and check them on their own.
func fields(t reflect.Type) []string {
	var names []string
	for f := range t.Fields() {
		if f.Anonymous {
			continue
		}
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name != "" && name != "-" {
			names = append(names, name)
		}
	}
	return names
}
