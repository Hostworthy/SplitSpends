package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/auth"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/Hostworthy/SplitSpends/packages/platform"
	"github.com/google/uuid"
)

func quiet() *slog.Logger { return slog.New(slog.DiscardHandler) }

func envelope(t *testing.T, rec *httptest.ResponseRecorder) platform.Error {
	t.Helper()
	var e platform.Error
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatalf("not an error envelope: %v: %s", err, rec.Body)
	}
	return e
}

func TestWriteError(t *testing.T) {
	for _, tc := range []struct {
		kind   errs.Kind
		status int
	}{
		{errs.KindValidation, http.StatusUnprocessableEntity},
		{errs.KindNotFound, http.StatusNotFound},
		{errs.KindUnauthorized, http.StatusUnauthorized},
		{errs.KindForbidden, http.StatusForbidden},
		{errs.KindConflict, http.StatusConflict},
		{errs.KindRateLimited, http.StatusTooManyRequests},
		{errs.KindAIDisabled, http.StatusServiceUnavailable},
		{errs.KindNoRates, http.StatusServiceUnavailable},
		{errs.KindPending, http.StatusForbidden},
		{errs.KindDeclined, http.StatusForbidden},
		// A kind added to errs without a status here must not break the response.
		{errs.Kind("SOMETHING_NEW"), http.StatusInternalServerError},
	} {
		t.Run(string(tc.kind), func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			err := fmt.Errorf("wrapped: %w", &errs.Error{Kind: tc.kind, Message: "Readable message.", Field: "name"})
			WriteError(rec, req, quiet(), err)
			if rec.Code != tc.status {
				t.Fatalf("want %d, got %d", tc.status, rec.Code)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
				t.Errorf("Content-Type %q", ct)
			}
			e := envelope(t, rec)
			if e.Code != string(tc.kind) || e.Error != "Readable message." || e.Field != "name" {
				t.Errorf("unexpected envelope %+v", e)
			}
		})
	}
}

func TestWriteErrorHidesInternalErrors(t *testing.T) {
	var logs bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logs, nil))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	WriteError(rec, req, log, errors.New("database.query.failed: password=hunter2"))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d", rec.Code)
	}
	e := envelope(t, rec)
	if e.Code != "INTERNAL" || strings.Contains(rec.Body.String(), "hunter2") {
		t.Fatalf("internal details reached the client: %s", rec.Body)
	}
	if !strings.Contains(logs.String(), "middleware.error.internal") || !strings.Contains(logs.String(), "database.query.failed") {
		t.Fatalf("the cause must be logged: %s", logs.String())
	}
}

func TestObserve(t *testing.T) {
	t.Run("assigns a request id", func(t *testing.T) {
		var seen string
		h := Observe(quiet(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { seen = RequestID(r.Context()) }))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		if seen == "" || rec.Header().Get("X-Request-Id") != seen {
			t.Fatalf("request id %q, header %q", seen, rec.Header().Get("X-Request-Id"))
		}
	})
	t.Run("keeps a well-formed caller id and replaces others", func(t *testing.T) {
		h := Observe(quiet(), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		for id, keep := range map[string]bool{
			"abc-123":               true,
			"A.b_C-9":               true,
			strings.Repeat("x", 64): true,
			strings.Repeat("x", 65): false,
			"two words":             false,
			"caf\u00e9":             false,
			"id\x00":                false,
			"a/b":                   false,
		} {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("X-Request-Id", id)
			h.ServeHTTP(rec, req)
			if got := rec.Header().Get("X-Request-Id"); (got == id) != keep || got == "" {
				t.Errorf("caller id %q: got %q", id, got)
			}
		}
	})
	t.Run("recovers a panic", func(t *testing.T) {
		var logs bytes.Buffer
		h := Observe(slog.New(slog.NewJSONHandler(&logs, nil)), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			panic("boom")
		}))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/me", nil))
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("want 500, got %d", rec.Code)
		}
		e := envelope(t, rec)
		if e.Code != "INTERNAL" || e.RequestID == "" || e.RequestID != rec.Header().Get("X-Request-Id") {
			t.Fatalf("unexpected envelope %+v", e)
		}
		var record map[string]any
		for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
			var r map[string]any
			if err := json.Unmarshal([]byte(line), &r); err == nil && r["msg"] == "middleware.panic.recovered" {
				record = r
			}
		}
		if record == nil {
			t.Fatalf("the panic must be logged: %s", logs.String())
		}
		if stack, _ := record["stack"].(string); !strings.Contains(stack, "goroutine") {
			t.Fatalf("the panic log must carry a stack trace: %v", record)
		}
	})
	t.Run("lets a deliberate abort through", func(t *testing.T) {
		var logs bytes.Buffer
		h := Observe(slog.New(slog.NewJSONHandler(&logs, nil)), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			panic(http.ErrAbortHandler)
		}))
		rec := httptest.NewRecorder()
		defer func() {
			if err, _ := recover().(error); !errors.Is(err, http.ErrAbortHandler) {
				t.Fatalf("want http.ErrAbortHandler to propagate, got %v", err)
			}
			if rec.Body.Len() != 0 {
				t.Fatalf("an aborted response must get no error body: %s", rec.Body)
			}
			if strings.Contains(logs.String(), "middleware.panic.recovered") {
				t.Fatalf("an abort is not a panic to report: %s", logs.String())
			}
			if !strings.Contains(logs.String(), "http.request.completed") {
				t.Fatalf("an aborted request is still logged: %s", logs.String())
			}
		}()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/me", nil))
	})
}

func TestObserveRedactsInviteTokens(t *testing.T) {
	const secret = "abc123secret"
	for _, tc := range []struct {
		path  string
		fails bool
	}{
		{"/api/v1/invites/" + secret, false},
		{"/api/v1/invites/" + secret + "/accept", false},
		{"/join/" + secret, false},
		{"/api/v1/invites/" + secret, true},
	} {
		var logs bytes.Buffer
		log := slog.New(slog.NewJSONHandler(&logs, nil))
		h := Observe(log, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if tc.fails {
				WriteError(w, r, log, errors.New("database down"))
			}
		}))
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, tc.path, nil))
		if strings.Contains(logs.String(), secret) {
			t.Errorf("%s: the log holds the token: %s", tc.path, logs.String())
		}
		if !strings.Contains(logs.String(), "{token}") {
			t.Errorf("%s: the log must still show the route: %s", tc.path, logs.String())
		}
	}
	for path, want := range map[string]string{
		"/api/v1/groups/g1/invites/i1": "/api/v1/groups/g1/invites/i1",
		"/api/v1/invites/":             "/api/v1/invites/",
		"/join/":                       "/join/",
		"/api/v1/me":                   "/api/v1/me",
	} {
		if got := redact(path); got != want {
			t.Errorf("redact(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestSecurity(t *testing.T) {
	rec := httptest.NewRecorder()
	Security(false, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	for header, want := range map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"Referrer-Policy":         "strict-origin-when-cross-origin",
		"X-Frame-Options":         "DENY",
		"Content-Security-Policy": "default-src 'none'; frame-ancestors 'none'",
		"Permissions-Policy":      "camera=(self), microphone=(), geolocation=()",
	} {
		if got := rec.Header().Get(header); got != want {
			t.Errorf("%s: want %q, got %q", header, want, got)
		}
	}
}

func TestCORS(t *testing.T) {
	const listed = "https://app.example.com"
	for _, tc := range []struct {
		name      string
		origins   []string
		method    string
		origin    string
		preflight bool // whether Access-Control-Request-Method is sent
		allowed   bool
		vary      bool
		reached   bool // whether the request reaches the next handler
	}{
		{"listed origin", []string{listed + "/"}, http.MethodGet, listed, false, true, true, true},
		{"other origin", []string{listed}, http.MethodGet, "https://evil.example.com", false, false, true, true},
		{"any origin", []string{"*"}, http.MethodGet, "https://anywhere.example.com", false, true, true, true},
		{"no origin", []string{"*"}, http.MethodGet, "", false, false, true, true},
		{"no origins configured", nil, http.MethodGet, listed, false, false, false, true},
		{"preflight", []string{listed}, http.MethodOptions, listed, true, true, true, false},
		{"preflight from any origin", []string{"*"}, http.MethodOptions, "https://anywhere.example.com", true, true, true, false},
		{"preflight from other origin", []string{listed}, http.MethodOptions, "https://evil.example.com", true, false, true, true},
		{"options without request method", []string{listed}, http.MethodOptions, listed, false, true, true, true},
		{"options without origin", []string{"*"}, http.MethodOptions, "", true, false, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reached := false
			h := CORS(tc.origins, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				reached = true
				w.WriteHeader(http.StatusMethodNotAllowed)
			}))
			req := httptest.NewRequest(tc.method, "/api/v1/me", nil)
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			if tc.preflight {
				req.Header.Set("Access-Control-Request-Method", http.MethodPost)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			got := rec.Header().Get("Access-Control-Allow-Origin")
			if tc.allowed && got != tc.origin {
				t.Errorf("want origin %q allowed, got %q", tc.origin, got)
			}
			if !tc.allowed && got != "" {
				t.Errorf("origin must not be allowed, got %q", got)
			}
			if vary := slices.Contains(rec.Header().Values("Vary"), "Origin"); vary != tc.vary {
				t.Errorf("Vary: Origin present: %v, want %v", vary, tc.vary)
			}
			if rec.Header().Get("Access-Control-Allow-Credentials") != "" {
				t.Error("credentials must never be allowed")
			}
			if reached != tc.reached {
				t.Errorf("reached next handler: %v, want %v", reached, tc.reached)
			}
			if !tc.reached && (rec.Code != http.StatusNoContent || rec.Header().Get("Access-Control-Allow-Methods") == "") {
				t.Errorf("preflight: want 204 with Allow-Methods, got %d %v", rec.Code, rec.Header())
			}
		})
	}
	t.Run("keeps other Vary values", func(t *testing.T) {
		h := CORS([]string{listed}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Add("Vary", "Accept-Encoding")
		}))
		req := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
		req.Header.Set("Origin", listed)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if got := rec.Header().Values("Vary"); !slices.Contains(got, "Origin") || !slices.Contains(got, "Accept-Encoding") {
			t.Fatalf("want both Vary values, got %v", got)
		}
	})
}

func TestBearerToken(t *testing.T) {
	for header, want := range map[string]string{
		"":                  "",
		"Bearer abc":        "abc",
		"bearer abc":        "abc",
		"BEARER  abc ":      "abc",
		"Bearer ":           "",
		"Basic YWJjOmRlZg=": "",
		"Bearerabc":         "",
		"abc":               "",
	} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		if got := BearerToken(req); got != want {
			t.Errorf("%q: want %q, got %q", header, want, got)
		}
	}
}

// tokens accepts only the token "good".
type tokens struct{ principal *auth.Principal }

func (a tokens) Authenticate(_ context.Context, token string) (*auth.Principal, error) {
	if token != "good" {
		return nil, errs.Unauthorized("Sign in to continue.")
	}
	return a.principal, nil
}

func TestRequireAuth(t *testing.T) {
	p := &auth.Principal{UserID: uuid.New(), SessionID: uuid.New()}
	var seen *auth.Principal
	h := RequireAuth(tokens{principal: p}, quiet(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = Principal(r.Context())
	}))
	for header, want := range map[string]int{"": http.StatusUnauthorized, "Bearer bad": http.StatusUnauthorized, "Bearer good": http.StatusOK} {
		seen = nil
		req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Errorf("%q: want %d, got %d", header, want, rec.Code)
		}
		if want == http.StatusOK && seen != p {
			t.Errorf("%q: the handler must see the principal", header)
		}
		if want != http.StatusOK {
			if seen != nil {
				t.Errorf("%q: the handler must not run", header)
			}
			if e := envelope(t, rec); e.Code != "UNAUTHORIZED" {
				t.Errorf("%q: unexpected envelope %+v", header, e)
			}
		}
	}
}

func TestClientIP(t *testing.T) {
	for _, tc := range []struct {
		name      string
		hops      int
		remote    string
		forwarded []string
		real      string
		want      string
	}{
		{"peer without a proxy", 0, "203.0.113.7:4321", nil, "", "203.0.113.7"},
		{"IPv6 peer", 0, "[2001:db8::1]:4321", nil, "", "2001:db8::1"},
		{"peer without a port", 0, "203.0.113.7", nil, "", "203.0.113.7"},
		{"headers ignored without a proxy", 0, "10.0.0.1:4321", []string{"198.51.100.1"}, "198.51.100.2", "10.0.0.1"},
		{"one hop takes the entry the proxy appended", 1, "10.0.0.1:4321", []string{"6.6.6.6, 203.0.113.9"}, "", "203.0.113.9"},
		{"one hop with only the appended entry", 1, "10.0.0.1:4321", []string{"203.0.113.9"}, "", "203.0.113.9"},
		{"one hop ignores X-Real-Ip", 1, "10.0.0.1:4321", nil, "6.6.6.6", "10.0.0.1"},
		{"one hop with a spoofed X-Real-Ip", 1, "10.0.0.1:4321", []string{"203.0.113.9"}, "6.6.6.6", "203.0.113.9"},
		{"two hops skip the inner proxy", 2, "127.0.0.1:4321", []string{"6.6.6.6, 203.0.113.9, 130.176.1.1"}, "", "203.0.113.9"},
		{"header lines are joined in order", 2, "127.0.0.1:4321", []string{"6.6.6.6", "203.0.113.9, 130.176.1.1"}, "", "203.0.113.9"},
		{"fewer entries than hops falls back to the peer", 2, "127.0.0.1:4321", []string{"203.0.113.9"}, "", "127.0.0.1"},
		{"missing header falls back to the peer", 1, "10.0.0.1:4321", nil, "", "10.0.0.1"},
		{"garbage falls back to the peer", 1, "10.0.0.1:4321", []string{"6.6.6.6, unknown"}, "", "10.0.0.1"},
		{"entry with a port", 1, "10.0.0.1:4321", []string{"203.0.113.9:5555"}, "", "203.0.113.9"},
		{"bracketed IPv6 entry", 1, "10.0.0.1:4321", []string{"[2001:db8::9]"}, "", "2001:db8::9"},
		{"bracketed IPv6 entry with a port", 1, "10.0.0.1:4321", []string{"[2001:db8::9]:5555"}, "", "2001:db8::9"},
		{"IPv4-mapped entry", 1, "10.0.0.1:4321", []string{"::ffff:203.0.113.9"}, "", "203.0.113.9"},
	} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = tc.remote
		for _, v := range tc.forwarded {
			req.Header.Add("X-Forwarded-For", v)
		}
		if tc.real != "" {
			req.Header.Set("X-Real-Ip", tc.real)
		}
		if got := ClientIP(req, tc.hops); got != tc.want {
			t.Errorf("%s: want %q, got %q", tc.name, tc.want, got)
		}
	}
}

// TestLimitBehindProxy keys requests by the address the trusted proxy
// appended, so changing the client-written part of the header does not buy
// a fresh budget.
func TestLimitBehindProxy(t *testing.T) {
	h := Limit(NewLimiter(20, time.Minute), quiet(), ByAddress(1), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	for i := range 25 {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/otp/verify", nil)
		req.RemoteAddr = "10.0.0.1:4321"
		req.Header.Set("X-Forwarded-For", fmt.Sprintf("198.51.100.%d, 203.0.113.9", i))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		want := http.StatusOK
		if i >= 20 {
			want = http.StatusTooManyRequests
		}
		if rec.Code != want {
			t.Fatalf("request %d: want %d, got %d", i+1, want, rec.Code)
		}
	}
}

func TestLimit(t *testing.T) {
	calls := 0
	h := Limit(NewLimiter(2, time.Hour), quiet(), ByAddress(0), http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	status := func(remote string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/otp/request", nil)
		req.RemoteAddr = remote
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	for i, want := range []int{http.StatusOK, http.StatusOK, http.StatusTooManyRequests, http.StatusTooManyRequests} {
		if got := status("203.0.113.7:1000"); got != want {
			t.Fatalf("request %d: want %d, got %d", i+1, want, got)
		}
	}
	if got := status("203.0.113.8:1000"); got != http.StatusOK {
		t.Fatalf("another client has its own budget: got %d", got)
	}
	if calls != 3 {
		t.Fatalf("limited requests must not reach the handler: %d calls", calls)
	}
}

// clock returns a limiter whose time moves only when the test says so.
func clock(limit int, window time.Duration) (*Limiter, func(time.Duration)) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	l := NewLimiter(limit, window)
	l.now = func() time.Time { return now }
	return l, func(d time.Duration) { now = now.Add(d) }
}

func TestLimiterSlidingWindow(t *testing.T) {
	l, advance := clock(2, time.Minute)
	for i, step := range []struct {
		after time.Duration
		want  bool
	}{
		{0, true},
		{0, true},
		{0, false},
		{59 * time.Second, false},
		// A fixed window would allow two more here; the previous window
		// still almost fully overlaps.
		{2 * time.Second, true},
		{0, false},
		// Half of the previous window overlaps: 2/2 + 1 counted.
		{29 * time.Second, false},
		// Next window: the previous one held 1 request.
		{30 * time.Second, true},
		{0, false},
		// Two idle windows clear the budget.
		{2 * time.Minute, true},
		{0, true},
		{0, false},
	} {
		advance(step.after)
		if got := l.Allow("203.0.113.7"); got != step.want {
			t.Fatalf("step %d: want %v, got %v", i+1, step.want, got)
		}
	}
	if !l.Allow("203.0.113.8") {
		t.Fatal("another key has its own budget")
	}
}

func TestLimiterSweepsIdleKeys(t *testing.T) {
	l, advance := clock(1, time.Minute)
	for i := range 3 {
		l.Allow(fmt.Sprintf("203.0.113.%d", i))
	}
	advance(time.Minute)
	l.Allow("203.0.113.9")
	if len(l.keys) != 4 {
		t.Fatalf("keys idle for one window still count: %d keys", len(l.keys))
	}
	advance(time.Minute)
	l.Allow("203.0.113.10")
	if len(l.keys) != 2 || l.recent.Len() != 2 {
		t.Fatalf("keys idle for two windows must be dropped: %d keys, %d in order", len(l.keys), l.recent.Len())
	}
}

func TestLimiterBoundedMemory(t *testing.T) {
	l, _ := clock(1, time.Hour)
	for i := range 1_000_000 {
		l.Allow(fmt.Sprint(i))
	}
	if len(l.keys) > maxKeys || l.recent.Len() != len(l.keys) {
		t.Fatalf("want at most %d keys, got %d (%d in order)", maxKeys, len(l.keys), l.recent.Len())
	}
	if l.Allow("999999") {
		t.Fatal("a recent key must keep its count")
	}
}

func TestLimiterIPv6Prefix(t *testing.T) {
	h := Limit(NewLimiter(1, time.Hour), quiet(), ByAddress(0), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	status := func(remote string) int {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = remote
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	for _, step := range []struct {
		remote string
		want   int
	}{
		{"[2001:db8:1:2::1]:4321", http.StatusOK},
		{"[2001:db8:1:2:ffff:ffff:ffff:9]:4321", http.StatusTooManyRequests},
		{"[2001:db8:1:3::1]:4321", http.StatusOK},
		{"203.0.113.7:4321", http.StatusOK},
		{"[::ffff:203.0.113.7]:4321", http.StatusTooManyRequests},
	} {
		if got := status(step.remote); got != step.want {
			t.Errorf("%s: want %d, got %d", step.remote, step.want, got)
		}
	}
}

func TestByUser(t *testing.T) {
	key := ByUser(0)
	req := func(p *auth.Principal) *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = "203.0.113.7:4321"
		if p != nil {
			r = r.WithContext(WithPrincipal(r.Context(), p))
		}
		return r
	}
	ada, bob := uuid.New(), uuid.New()
	if key(req(&auth.Principal{UserID: ada})) == key(req(&auth.Principal{UserID: bob})) {
		t.Fatal("two people at one address must have their own budgets")
	}
	if a, b := key(req(&auth.Principal{UserID: ada})), key(req(&auth.Principal{UserID: ada, SessionID: uuid.New()})); a != b {
		t.Fatalf("one person's sessions share a budget: %q, %q", a, b)
	}
	if got := key(req(nil)); got != "203.0.113.7" {
		t.Fatalf("without a principal the address is the key, got %q", got)
	}
}

// BenchmarkLimiterManyKeys measures Allow with the limiter full of live keys,
// half the calls for new keys and half for known ones.
func BenchmarkLimiterManyKeys(b *testing.B) {
	l, _ := clock(20, time.Minute)
	keys := make([]string, 2*maxKeys)
	for i := range keys {
		keys[i] = fmt.Sprint(i)
	}
	for _, k := range keys[:maxKeys] {
		l.Allow(k)
	}
	for i := 0; b.Loop(); i++ {
		l.Allow(keys[i%len(keys)])
	}
}

// TestSecurityHeaders checks the Content-Security-Policy of API responses
// and web app pages, and that HSTS is sent only by servers reached over https.
func TestSecurityHeaders(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	for _, tc := range []struct {
		name    string
		handler http.Handler
		policy  []string
		hsts    bool
	}{
		{"api over http", Security(false, ok), []string{"default-src 'none'", "frame-ancestors 'none'"}, false},
		{"api over https", Security(true, ok), []string{"default-src 'none'"}, true},
		{"web app over https", Security(true, Web(true, ok)), []string{
			"default-src 'self'", "script-src 'self' 'wasm-unsafe-eval';", "style-src 'self' 'unsafe-inline'",
			"https://*.gravatar.com", "connect-src 'self' https:;", "object-src 'none'", "frame-ancestors 'none'",
		}, true},
		{"web app over http", Security(false, Web(false, ok)), []string{"connect-src 'self' https: http:;"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			tc.handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
			h := rec.Header()
			policy := h.Get("Content-Security-Policy")
			for _, want := range tc.policy {
				if !strings.Contains(policy, want) {
					t.Errorf("Content-Security-Policy %q lacks %q", policy, want)
				}
			}
			if got := h.Get("Strict-Transport-Security") != ""; got != tc.hsts {
				t.Errorf("Strict-Transport-Security %q, want present %v", h.Get("Strict-Transport-Security"), tc.hsts)
			}
		})
	}
}
