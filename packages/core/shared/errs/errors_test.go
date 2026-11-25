package errs

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"testing"
)

// TestKindsListsEveryKind checks that Kinds names every Kind constant
// declared in this package, so a new kind cannot be left out of it.
func TestKindsListsEveryKind(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "errors.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var declared []Kind
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok {
			return true
		}
		if typ, ok := spec.Type.(*ast.Ident); !ok || typ.Name != "Kind" {
			return true
		}
		for _, v := range spec.Values {
			if lit, ok := v.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				declared = append(declared, Kind(lit.Value[1:len(lit.Value)-1]))
			}
		}
		return true
	})
	listed := Kinds()
	slices.Sort(declared)
	slices.Sort(listed)
	if len(declared) == 0 || !slices.Equal(declared, listed) {
		t.Fatalf("Kinds() = %v, declared %v", listed, declared)
	}
}
