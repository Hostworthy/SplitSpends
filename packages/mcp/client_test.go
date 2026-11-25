package mcp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/platform"
)

// api is a fake SplitSpends API that records the requests it sees.
type api struct {
	mu   sync.Mutex
	seen []string
}

func (a *api) record(r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.seen = append(a.seen, r.Method+" "+r.URL.Path)
}

func (a *api) requests() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.seen...)
}

// serve starts a fake API whose handler h answers every request.
func serve(t *testing.T, h http.HandlerFunc) (*api, *Client) {
	t.Helper()
	a := &api{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.record(r)
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	return a, NewClient(srv.URL, "ss_test", nil)
}

func TestDoRefusesRedirect(t *testing.T) {
	for _, status := range []int{http.StatusMovedPermanently, http.StatusFound, http.StatusTemporaryRedirect} {
		for _, call := range []struct {
			method, path string
			body         any
		}{
			{http.MethodPost, "/expenses", platform.ExpenseInput{Description: "Rent"}},
			{http.MethodDelete, "/expenses/abc", nil},
		} {
			a, c := serve(t, func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/moved"+platform.Prefix+call.path {
					writeJSON(w, `{"data":{"id":"abc"}}`)
					return
				}
				http.Redirect(w, r, "/moved"+r.URL.Path, status)
			})
			_, err := c.do(context.Background(), call.method, call.path, nil, call.body)
			if err == nil || !strings.Contains(err.Error(), "/moved"+platform.Prefix+call.path) {
				t.Errorf("%d %s %s: want an error naming the redirect target, got %v", status, call.method, call.path, err)
			}
			if got := a.requests(); len(got) != 1 {
				t.Errorf("%d %s %s: the redirect must not be followed, saw %v", status, call.method, call.path, got)
			}
		}
	}
}

func TestCallerClientDoesNotFollowRedirects(t *testing.T) {
	a := &api{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.record(r)
		http.Redirect(w, r, "/elsewhere", http.StatusFound)
	}))
	t.Cleanup(srv.Close)
	hc := &http.Client{}
	c := NewClient(srv.URL, "ss_test", hc)
	if _, err := c.do(context.Background(), http.MethodDelete, "/expenses/abc", nil, nil); err == nil {
		t.Fatal("a redirected delete must not read as success")
	}
	if got := a.requests(); len(got) != 1 {
		t.Fatalf("the redirect must not be followed, saw %v", got)
	}
	if hc.CheckRedirect != nil {
		t.Fatal("the caller's client must not be changed")
	}
}

func TestSendRequiresDataEnvelope(t *testing.T) {
	_, c := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{"items":[],"total":0}`)
	})
	if _, err := send[platform.Expense](context.Background(), c, http.MethodPost, "/expenses", platform.ExpenseInput{}); err == nil {
		t.Fatal("a list envelope must not read as a created expense")
	}
	if _, err := get[platform.Expense](context.Background(), c, "/expenses/abc", nil); err == nil {
		t.Fatal("a list envelope must not read as an expense")
	}
}

func TestListRequiresItemsEnvelope(t *testing.T) {
	_, c := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, `{"data":{"id":"abc"}}`)
	})
	if _, err := getList[platform.Expense](context.Background(), c, "/expenses", nil); err == nil {
		t.Fatal("a details envelope must not read as an empty list")
	}
}

func writeJSON(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(body))
}

// config answers /config with USD (two decimals), JPY (none) and KWD
// (three).
func config(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, `{"data":{"currencies":[{"code":"USD","exponent":2},{"code":"JPY","exponent":0},{"code":"KWD","exponent":3}]}}`)
}

func TestMinor(t *testing.T) {
	_, c := serve(t, config)
	cases := []struct {
		amount, currency string
		want             int64
		ok               bool
	}{
		{"12.50", "USD", 1250, true},
		{" 12.5 ", "usd", 1250, true},
		{"0.01", "USD", 1, true},
		{".5", "USD", 50, true},
		{"100", "JPY", 100, true},
		{"12.505", "USD", 0, false},
		{"1.5", "JPY", 0, false},
		{"184467440737095516.26", "USD", 0, false},
		{"92233720368547758.08", "USD", 0, false},
		{"99999999999999999999", "JPY", 0, false},
		{"1/2", "USD", 0, false},
		{"0x1p4", "USD", 0, false},
		{"0x10", "USD", 0, false},
		{"010/2", "USD", 0, false},
		{"1e2", "USD", 0, false},
		{"1_000", "USD", 0, false},
		{"-5", "USD", 0, false},
		{"+5", "USD", 0, false},
		{"", "USD", 0, false},
		{".", "USD", 0, false},
		{"5", "XXX", 0, false},
	}
	for _, tc := range cases {
		got, err := c.minor(context.Background(), tc.amount, tc.currency)
		if tc.ok && (err != nil || got != tc.want) {
			t.Errorf("minor(%q, %s) = %d, %v; want %d", tc.amount, tc.currency, got, err, tc.want)
		}
		if !tc.ok && err == nil {
			t.Errorf("minor(%q, %s) = %d; want an error", tc.amount, tc.currency, got)
		}
	}
}

// configCalls counts the /config requests a fake API saw.
func configCalls(a *api) int {
	n := 0
	for _, r := range a.requests() {
		if r == "GET "+platform.Prefix+"/config" {
			n++
		}
	}
	return n
}

func TestConfigRetriesAfterFailure(t *testing.T) {
	var failing atomic.Bool
	failing.Store(true)
	a, c := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if failing.Load() {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		config(w, r)
	})
	if _, err := c.minor(context.Background(), "12.50", "USD"); err == nil {
		t.Fatal("the first call should fail while the server is down")
	}
	failing.Store(false)
	if got, err := c.minor(context.Background(), "12.50", "USD"); err != nil || got != 1250 {
		t.Fatalf("after recovery: %d, %v", got, err)
	}
	if n := configCalls(a); n != 2 {
		t.Fatalf("want two /config requests, got %d", n)
	}
}

func TestConfigIgnoresCancelledCaller(t *testing.T) {
	a, c := serve(t, config)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.minor(cancelled, "1", "USD"); err == nil {
		t.Fatal("a cancelled call should fail")
	}
	if got, err := c.minor(context.Background(), "1", "USD"); err != nil || got != 100 {
		t.Fatalf("a cancelled first caller must not poison the cache: %d, %v", got, err)
	}
	if n := configCalls(a); n != 1 {
		t.Fatalf("want one /config request, got %d", n)
	}
}

func TestConfigCached(t *testing.T) {
	a, c := serve(t, config)
	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	c.settings.now = func() time.Time { return now }
	for range 2 {
		if _, err := c.minor(context.Background(), "1", "USD"); err != nil {
			t.Fatal(err)
		}
	}
	if n := configCalls(a); n != 1 {
		t.Fatalf("want one /config request, got %d", n)
	}
	now = now.Add(configTTL)
	if _, err := c.minor(context.Background(), "1", "USD"); err != nil {
		t.Fatal(err)
	}
	if n := configCalls(a); n != 2 {
		t.Fatalf("the configuration should be fetched again after %v, got %d requests", configTTL, n)
	}
}

func TestConfigKeptWhenRefreshFails(t *testing.T) {
	var failing atomic.Bool
	_, c := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if failing.Load() {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		config(w, r)
	})
	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	c.settings.now = func() time.Time { return now }
	if _, err := c.minor(context.Background(), "1", "USD"); err != nil {
		t.Fatal(err)
	}
	failing.Store(true)
	now = now.Add(configTTL)
	if got, err := c.minor(context.Background(), "1", "USD"); err != nil || got != 100 {
		t.Fatalf("a failed refresh should keep the known configuration: %d, %v", got, err)
	}
}

func TestFormat(t *testing.T) {
	_, c := serve(t, config)
	cases := []struct {
		amount   int64
		currency string
		want     string
	}{
		{1250, "USD", "12.50 USD"},
		{-1250, "USD", "-12.50 USD"},
		{-5, "usd", "-0.05 USD"},
		{0, "USD", "0.00 USD"},
		{100, "JPY", "100 JPY"},
		{-100, "JPY", "-100 JPY"},
		{1234, "KWD", "1.234 KWD"},
		{5, "XXX", "5 XXX (minor units)"},
	}
	for _, tc := range cases {
		if got := c.format(context.Background(), tc.amount, tc.currency); got != tc.want {
			t.Errorf("format(%d, %s) = %q, want %q", tc.amount, tc.currency, got, tc.want)
		}
	}
}

func TestDoErrors(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"validation with field", http.StatusUnprocessableEntity, `{"error":"Enter a total above zero.","code":"VALIDATION","field":"total"}`, "Enter a total above zero. (field total)"},
		{"not found", http.StatusNotFound, `{"error":"Expense not found.","code":"NOT_FOUND"}`, "Expense not found."},
		{"not JSON", http.StatusBadGateway, `<html>bad gateway</html>`, "request failed with status 502"},
		{"empty", http.StatusInternalServerError, ``, "request failed with status 500"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, c := serve(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})
			_, err := c.do(context.Background(), http.MethodGet, "/expenses/e1", nil, nil)
			var apiErr *APIError
			if !errors.As(err, &apiErr) || apiErr.Status != tc.status || err.Error() != tc.want {
				t.Fatalf("got %v, want %d %q", err, tc.status, tc.want)
			}
		})
	}
}

func TestDoSendsToken(t *testing.T) {
	var got http.Header
	_, c := serve(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		writeJSON(w, `{"data":{}}`)
	})
	if _, err := c.do(context.Background(), http.MethodPost, "/payments", nil, platform.PaymentInput{}); err != nil {
		t.Fatal(err)
	}
	if got.Get("Authorization") != "Bearer ss_test" || got.Get("Content-Type") != "application/json" {
		t.Fatalf("headers %v", got)
	}
	_, anonymous := serve(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		writeJSON(w, `{"data":{}}`)
	})
	if _, err := anonymous.with("").do(context.Background(), http.MethodGet, "/me", nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["Authorization"]; ok {
		t.Fatalf("a client without a token must send no Authorization header, sent %q", got.Get("Authorization"))
	}
}

func TestOrigin(t *testing.T) {
	cases := []struct {
		raw, want string
		insecure  bool
		ok        bool
	}{
		{"https://split.example.com", "https://split.example.com", false, true},
		{"https://split.example.com/", "https://split.example.com", false, true},
		{" https://split.example.com// ", "https://split.example.com", false, true},
		{"https://example.com/split", "https://example.com/split", false, true},
		{"http://localhost:8080", "http://localhost:8080", false, true},
		{"http://127.0.0.1:8080", "http://127.0.0.1:8080", false, true},
		{"http://[::1]:8080", "http://[::1]:8080", false, true},
		{"http://split.example.com", "http://split.example.com", true, true},
		{"http://192.168.1.20:8080", "http://192.168.1.20:8080", true, true},
		{"split.example.com", "", false, false},
		{"split.example.com:8080", "", false, false},
		{"ftp://split.example.com", "", false, false},
		{"https://", "", false, false},
		{"https://split.example.com/api/v1", "", false, false},
		{"https://example.com/split/api/v1/", "", false, false},
		{"https://user:pw@split.example.com", "", false, false},
		{"https://split.example.com?x=1", "", false, false},
		{"https://split.example.com/#top", "", false, false},
		{"", "", false, false},
	}
	for _, tc := range cases {
		got, insecure, err := Origin(tc.raw)
		if (err == nil) != tc.ok || got != tc.want || insecure != tc.insecure {
			t.Errorf("Origin(%q) = %q, %v, %v; want %q, %v, ok %v", tc.raw, got, insecure, err, tc.want, tc.insecure, tc.ok)
		}
	}
}
