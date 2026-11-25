package routes

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/Hostworthy/SplitSpends/packages/bootstrap/bootstraptest"
	"github.com/Hostworthy/SplitSpends/packages/platform"
	"github.com/google/uuid"
)

// anonymous lists the routes that answer without a session, with the wrapper
// each must use: sign-in and invite previews are rate limited per client.
// Everything else in router.go must answer 401 without a valid token.
var anonymous = map[string]string{
	"GET /health":                                   "public",
	"GET " + platform.Prefix + "/config":            "public",
	"POST /internal/jobs":                           "public", // checks JOBS_SECRET itself
	"POST " + platform.Prefix + "/auth/otp/request": "limited",
	"POST " + platform.Prefix + "/auth/otp/verify":  "limited",
	"GET " + platform.Prefix + "/invites/{token}":   "previews",
	platform.Prefix + "/":                           "public", // unknown endpoints answer 404
}

// route is one mux.Handle call in router.go.
type route struct {
	pattern string // as registered, for example "GET /api/v1/groups/{groupId}"
	wrapper string // authed, public, limited or previews
}

// registered reads the routes from router.go, so a route added there is
// covered here without editing this test.
func registered(t *testing.T) []route {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "router.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var routes []route
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) != 2 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Handle" {
			return true
		}
		wrap, ok := call.Args[1].(*ast.CallExpr)
		if !ok {
			t.Fatalf("router.go: route without a wrapper at offset %d", call.Pos())
		}
		name, ok := wrap.Fun.(*ast.Ident)
		if !ok {
			t.Fatalf("router.go: unexpected wrapper at offset %d", call.Pos())
		}
		routes = append(routes, route{pattern: pattern(t, call.Args[0]), wrapper: name.Name})
		return true
	})
	if len(routes) < 50 {
		t.Fatalf("found only %d routes in router.go; has the file changed shape?", len(routes))
	}
	return routes
}

// pattern evaluates a route pattern built from string literals and p.
func pattern(t *testing.T, e ast.Expr) string {
	t.Helper()
	switch v := e.(type) {
	case *ast.BasicLit:
		s, err := strconv.Unquote(v.Value)
		if err != nil {
			t.Fatal(err)
		}
		return s
	case *ast.Ident:
		if v.Name == "p" {
			return platform.Prefix
		}
	case *ast.BinaryExpr:
		return pattern(t, v.X) + pattern(t, v.Y)
	}
	t.Fatalf("router.go: cannot read route pattern %T", e)
	return ""
}

var wildcard = regexp.MustCompile(`\{[^}]+\}`)

// TestRoutesRequireAuth sends every route that is not anonymous a request
// with no token and one with a bad token, and expects 401 for both.
func TestRoutesRequireAuth(t *testing.T) {
	s := newServer(t, bootstraptest.Options{})
	seen := map[string]bool{}
	for _, r := range registered(t) {
		seen[r.pattern] = true
		if want, ok := anonymous[r.pattern]; ok {
			if r.wrapper != want {
				t.Errorf("%s: want wrapper %s, router.go uses %s", r.pattern, want, r.wrapper)
			}
			continue
		}
		method, path, ok := strings.Cut(r.pattern, " ")
		if !ok {
			t.Errorf("%s has no method; only the fallback may match every method", r.pattern)
			continue
		}
		path = wildcard.ReplaceAllString(path, uuid.NewString())
		for _, token := range []string{"", "not-a-session"} {
			res := s.call(t, method, path, token, nil)
			if res.status != http.StatusUnauthorized {
				t.Errorf("%s with token %q: want 401, got %d (wrapper %s)", r.pattern, token, res.status, r.wrapper)
				continue
			}
			if e := res.failure(t); e.Code != "UNAUTHORIZED" || e.Error == "" || e.RequestID == "" {
				t.Errorf("%s: unexpected error envelope %+v", r.pattern, e)
			}
		}
	}
	for p := range anonymous {
		if !seen[p] {
			t.Errorf("anonymous route %s is not registered in router.go", p)
		}
	}
}
