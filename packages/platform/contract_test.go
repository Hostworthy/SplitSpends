package platform

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// TestNullableFieldsOmitZero checks that every Nullable field in the
// contract has the omitzero tag option. Without it an unset field is sent
// as null, which turns "leave unchanged" into "clear".
func TestNullableFieldsOmitZero(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	seen := 0
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			st, ok := n.(*ast.StructType)
			if !ok {
				return true
			}
			for _, field := range st.Fields.List {
				if !nullable(field.Type) {
					continue
				}
				seen++
				tag := ""
				if field.Tag != nil {
					raw, _ := strconv.Unquote(field.Tag.Value)
					tag = reflect.StructTag(raw).Get("json")
				}
				if !slices.Contains(strings.Split(tag, ",")[1:], "omitzero") {
					t.Errorf("%s: a Nullable field needs the omitzero option, has json:%q", fset.Position(field.Pos()), tag)
				}
			}
			return true
		})
	}
	if seen == 0 {
		t.Fatal("no Nullable fields found; the test no longer looks at the contract")
	}
}

func nullable(e ast.Expr) bool {
	if ix, ok := e.(*ast.IndexExpr); ok {
		e = ix.X
	}
	id, ok := e.(*ast.Ident)
	return ok && id.Name == "Nullable"
}
