package mcp

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Hostworthy/SplitSpends/packages/platform"
)

// routes answers the fake API's GET requests by path; anything else is
// written as created with an empty expense.
func routes(bodies map[string]string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, platform.Prefix)
		if path == "/config" {
			config(w, r)
			return
		}
		if body, ok := bodies[path]; ok && r.Method == http.MethodGet {
			writeJSON(w, body)
			return
		}
		if r.Method == http.MethodGet {
			writeJSON(w, `{"items":[],"total":0}`)
			return
		}
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, `{"data":{"id":"e1"}}`)
	}
}

func resultText(res *sdk.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*sdk.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

func TestAddExpenseChecksSplitValues(t *testing.T) {
	me := `{"data":{"id":"u1","name":"Ada","defaultCurrency":"USD"}}`
	for _, value := range []string{"1/2", "0x10", "1e2", "-5", "1_000", ""} {
		a, c := serve(t, routes(map[string]string{"/me": me}))
		res, _, _ := (&tools{c: c}).addExpense(context.Background(), nil, addArgs{Description: "Rent", Amount: "100", SplitMethod: "percent",
			SplitValues: []splitValue{{UserID: "u1", Value: value}, {UserID: "u2", Value: "50"}}})
		if !res.IsError || !strings.Contains(resultText(res), "not a decimal number") {
			t.Errorf("%q: want a tool error, got %q", value, resultText(res))
		}
		if slices.Contains(a.requests(), "POST "+platform.Prefix+"/expenses") {
			t.Errorf("%q: the expense must not be sent", value)
		}
	}
}
