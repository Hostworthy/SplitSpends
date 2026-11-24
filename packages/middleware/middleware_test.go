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
		{errs.KindUnavailable, http.StatusServiceUnavailable},
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
	t.Run("keeps a short caller id and replaces a long one", func(t *testing.T) {
		h := Observe(quiet(), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		for id, keep := range map[string]bool{"abc-123": true, strings.Repeat("x", 65): false} {
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
		if !strings.Contains(logs.String(), "middleware.panic.recovered") {
			t.Fatalf("the panic must be logged: %s", logs.String())
		}
	})
}

func TestSecurity(t *testing.T) {
	rec := httptest.NewRecorder()
	Security(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	for header, want := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
		"X-Frame-Options":        "DENY",
	} {
		if got := rec.Header().Get(header); got != want {
			t.Errorf("%s: want %q, got %q", header, want, got)
		}
	}
}

func TestCORS(t *testing.T) {
	for _, tc := range []struct {
		name    string
		origins []string
		method  string
		origin  string
		allowed bool
		reached bool // whether the request reaches the next handler
	}{
		{"listed origin", []string{"https://app.example.com/"}, http.MethodGet, "https://app.example.com", true, true},
		{"other origin", []string{"https://app.example.com"}, http.MethodGet, "https://evil.example.com", false, true},
		{"any origin", []string{"*"}, http.MethodGet, "https://anywhere.example.com", true, true},
		{"no origin", []string{"*"}, http.MethodGet, "", false, true},
		{"no origins configured", nil, http.MethodGet, "https://app.example.com", false, true},
		{"preflight", []string{"https://app.example.com"}, http.MethodOptions, "https://app.example.com", true, false},
		{"preflight from other origin", []string{"https://app.example.com"}, http.MethodOptions, "https://evil.example.com", false, false},
		{"options without origin", []string{"*"}, http.MethodOptions, "", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reached := false
			h := CORS(tc.origins, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true }))
			req := httptest.NewRequest(tc.method, "/api/v1/me", nil)
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			got := rec.Header().Get("Access-Control-Allow-Origin")
			if tc.allowed && (got != tc.origin || rec.Header().Get("Vary") != "Origin") {
				t.Errorf("want origin %q allowed with Vary, got %q", tc.origin, got)
			}
			if !tc.allowed && got != "" {
				t.Errorf("origin must not be allowed, got %q", got)
			}
			if reached != tc.reached {
				t.Errorf("reached next handler: %v, want %v", reached, tc.reached)
			}
			if !tc.reached && rec.Code != http.StatusNoContent {
				t.Errorf("preflight: want 204, got %d", rec.Code)
			}
		})
	}
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

func TestClientIPWithoutProxy(t *testing.T) {
	for _, tc := range []struct{ remote, forwarded, real, want string }{
		{"203.0.113.7:4321", "", "", "203.0.113.7"},
		{"[2001:db8::1]:4321", "", "", "2001:db8::1"},
		{"203.0.113.7", "", "", "203.0.113.7"},
		{"203.0.113.7:4321", "198.51.100.1", "198.51.100.2", "203.0.113.7"},
	} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = tc.remote
		if tc.forwarded != "" {
			req.Header.Set("X-Forwarded-For", tc.forwarded)
		}
		if tc.real != "" {
			req.Header.Set("X-Real-Ip", tc.real)
		}
		if got := ClientIP(req, false); got != tc.want {
			t.Errorf("%+v: want %q, got %q", tc, tc.want, got)
		}
	}
}

func TestLimit(t *testing.T) {
	calls := 0
	h := Limit(NewLimiter(2, time.Hour), quiet(), false, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
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
