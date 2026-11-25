package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
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

// rpc posts one JSON-RPC request to an MCP endpoint and returns the status
// and the decoded answer.
func rpc(t *testing.T, endpoint, token, method string, params any) (int, map[string]any) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	req, _ := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

// toolText returns the text and error flag of a tools/call answer.
func toolText(t *testing.T, answer map[string]any) (string, bool) {
	t.Helper()
	result, ok := answer["result"].(map[string]any)
	if !ok {
		t.Fatalf("no result in %v", answer)
	}
	var b strings.Builder
	content, _ := result["content"].([]any)
	for _, c := range content {
		if m, ok := c.(map[string]any); ok {
			text, _ := m["text"].(string)
			b.WriteString(text)
		}
	}
	isError, _ := result["isError"].(bool)
	return b.String(), isError
}

func TestToolPanicIsRecovered(t *testing.T) {
	s := sdk.NewServer(&sdk.Implementation{Name: "test", Version: "test"}, nil)
	ts := &toolset{server: s, client: func(*sdk.CallToolRequest) *Client { return nil }, log: slog.New(slog.DiscardHandler)}
	add(ts, "explode", "Panics.", nil, func(*tools, context.Context, *sdk.CallToolRequest, empty) (*sdk.CallToolResult, any, error) {
		var p *platform.Expense
		return text(p.ID), nil, nil
	})
	srv := httptest.NewServer(stream(func(*http.Request) *sdk.Server { return s }, nil))
	t.Cleanup(srv.Close)
	for range 2 {
		status, answer := rpc(t, srv.URL, "ss_test", "tools/call", map[string]any{"name": "explode", "arguments": map[string]any{}})
		if status != http.StatusOK {
			t.Fatalf("status %d", status)
		}
		if text, isError := toolText(t, answer); !isError || strings.Contains(text, "nil pointer") {
			t.Fatalf("want a plain tool error, got %q (isError %v)", text, isError)
		}
	}
}

func TestListActivityShortTimestamp(t *testing.T) {
	_, c := serve(t, routes(map[string]string{
		"/activity": `{"items":[{"id":"a1","summary":"Ada added Rent","createdAt":"2024"}],"total":1}`,
	}))
	srv := httptest.NewServer(HTTPHandler(c.base, "test", nil, slog.New(slog.DiscardHandler)))
	t.Cleanup(srv.Close)
	status, answer := rpc(t, srv.URL, "ss_test", "tools/call", map[string]any{"name": "list_activity", "arguments": map[string]any{}})
	if status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	if text, isError := toolText(t, answer); isError || !strings.Contains(text, "2024 Ada added Rent") {
		t.Fatalf("got %q (isError %v)", text, isError)
	}
}

func TestHTTPHandlerRequiresBearer(t *testing.T) {
	a, c := serve(t, routes(nil))
	srv := httptest.NewServer(HTTPHandler(c.base, "test", nil, slog.New(slog.DiscardHandler)))
	t.Cleanup(srv.Close)
	for _, header := range []string{"", "Bearer", "Bearer   ", "Basic c3M6dGVzdA==", "ss_test"} {
		req, _ := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var body platform.Error
		_ = json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized || resp.Header.Get("WWW-Authenticate") != "Bearer" || body.Code != "UNAUTHORIZED" || body.Error == "" {
			t.Errorf("%q: got %d %q %+v", header, resp.StatusCode, resp.Header.Get("WWW-Authenticate"), body)
		}
	}
	if got := a.requests(); len(got) != 0 {
		t.Fatalf("the API must see no request, saw %v", got)
	}
}

func TestHTTPHandlerUsesEachCallersToken(t *testing.T) {
	var mu sync.Mutex
	tokens := map[string]bool{}
	_, c := serve(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		tokens[r.Header.Get("Authorization")] = true
		mu.Unlock()
		routes(nil)(w, r)
	})
	srv := httptest.NewServer(HTTPHandler(c.base, "test", nil, slog.New(slog.DiscardHandler)))
	t.Cleanup(srv.Close)
	for _, token := range []string{"ss_ada", "ss_ben"} {
		if status, answer := rpc(t, srv.URL, token, "tools/call", map[string]any{"name": "list_groups", "arguments": map[string]any{}}); status != http.StatusOK {
			t.Fatalf("status %d: %v", status, answer)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if !tokens["Bearer ss_ada"] || !tokens["Bearer ss_ben"] || len(tokens) != 2 {
		t.Fatalf("each call should reach the API with its own token, saw %v", tokens)
	}
}

func BenchmarkHTTPHandlerToolsList(b *testing.B) {
	h := HTTPHandler("http://127.0.0.1:1", "test", nil, slog.New(slog.DiscardHandler))
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	b.ReportAllocs()
	for b.Loop() {
		req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		req.Header.Set("Authorization", "Bearer ss_test")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			b.Fatalf("status %d: %s", w.Code, w.Body)
		}
	}
}
