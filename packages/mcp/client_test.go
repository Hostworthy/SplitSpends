package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

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

// config answers /config with USD (two decimals) and JPY (none).
func config(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, `{"data":{"currencies":[{"code":"USD","exponent":2},{"code":"JPY","exponent":0}]}}`)
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
