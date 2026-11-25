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
