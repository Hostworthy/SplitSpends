// Package middleware holds HTTP cross-cutting concerns: request ids, access
// logs, panic recovery, CORS, authentication, rate limits and the mapping of
// domain errors to HTTP responses.
package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/auth"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/Hostworthy/SplitSpends/packages/platform"
)

type ctxKey int

const (
	requestIDKey ctxKey = iota
	principalKey
)

// RequestID returns the id assigned to the request.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// Principal returns the authenticated caller, or nil.
func Principal(ctx context.Context) *auth.Principal {
	p, _ := ctx.Value(principalKey).(*auth.Principal)
	return p
}

// WithPrincipal stores p in ctx. Exposed for tests.
func WithPrincipal(ctx context.Context, p *auth.Principal) context.Context {
	return context.WithValue(ctx, principalKey, p)
}

type recorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *recorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *recorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

func (r *recorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (r *recorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// Observe assigns a request id, recovers panics and writes an access log.
func Observe(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-Id")
		if id == "" || len(id) > 64 {
			buf := make([]byte, 8)
			_, _ = rand.Read(buf)
			id = hex.EncodeToString(buf)
		}
		w.Header().Set("X-Request-Id", id)
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		rec := &recorder{ResponseWriter: w}
		start := time.Now()
		defer func() {
			if v := recover(); v != nil {
				log.Error("middleware.panic.recovered", "requestId", id, "panic", v, "path", r.URL.Path)
				if rec.status == 0 {
					WriteError(rec, r.WithContext(ctx), log, errors.New("middleware.panic"))
				}
			}
			if r.URL.Path == "/health" {
				return
			}
			log.Info("http.request.completed", "requestId", id, "method", r.Method, "path", r.URL.Path,
				"status", rec.status, "bytes", rec.bytes, "durationMs", time.Since(start).Milliseconds())
		}()
		next.ServeHTTP(rec, r.WithContext(ctx))
	})
}

// Security sets conservative response headers.
func Security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

// CORS allows the listed origins ("*" allows any). Mobile apps do not send an
// Origin header, so this only matters for browsers on other origins.
func CORS(origins []string, next http.Handler) http.Handler {
	allowAll := false
	allowed := map[string]bool{}
	for _, o := range origins {
		o = strings.TrimSpace(o)
		if o == "*" {
			allowAll = true
		} else if o != "" {
			allowed[strings.TrimRight(o, "/")] = true
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && (allowAll || allowed[origin]) {
			h := w.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Vary", "Origin")
			h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-Id, Mcp-Session-Id, Mcp-Protocol-Version")
			h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			h.Set("Access-Control-Expose-Headers", "X-Request-Id, Mcp-Session-Id")
			h.Set("Access-Control-Max-Age", "600")
		}
		if r.Method == http.MethodOptions && origin != "" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Authenticator resolves bearer tokens.
type Authenticator interface {
	Authenticate(ctx context.Context, token string) (*auth.Principal, error)
}

// BearerToken extracts the token from the Authorization header.
func BearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

// RequireAuth rejects requests without a valid bearer token.
func RequireAuth(a Authenticator, log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, err := a.Authenticate(r.Context(), BearerToken(r))
		if err != nil {
			WriteError(w, r, log, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), p)))
	})
}

// Limiter is a fixed-window rate limiter keyed by client IP.
type Limiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	hits   map[string]*window
}

type window struct {
	start time.Time
	count int
}

// NewLimiter allows limit requests per window per client.
func NewLimiter(limit int, per time.Duration) *Limiter {
	return &Limiter{limit: limit, window: per, hits: map[string]*window{}}
}

// Allow records a hit for key and reports whether it is within the limit.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if len(l.hits) > 10000 {
		for k, v := range l.hits {
			if now.Sub(v.start) > l.window {
				delete(l.hits, k)
			}
		}
	}
	w, ok := l.hits[key]
	if !ok || now.Sub(w.start) > l.window {
		l.hits[key] = &window{start: now, count: 1}
		return true
	}
	w.count++
	return w.count <= l.limit
}

// Limit applies l to next.
func Limit(l *Limiter, log *slog.Logger, trustProxy bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !l.Allow(ClientIP(r, trustProxy)) {
			WriteError(w, r, log, errs.RateLimited("Too many requests. Slow down and try again shortly."))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ClientIP returns the caller IP, honouring X-Forwarded-For when trusted.
func ClientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			return strings.TrimSpace(strings.Split(fwd, ",")[0])
		}
		if real := r.Header.Get("X-Real-Ip"); real != "" {
			return real
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

var statusByKind = map[errs.Kind]int{
	errs.KindValidation:   http.StatusUnprocessableEntity,
	errs.KindNotFound:     http.StatusNotFound,
	errs.KindUnauthorized: http.StatusUnauthorized,
	errs.KindForbidden:    http.StatusForbidden,
	errs.KindConflict:     http.StatusConflict,
	errs.KindRateLimited:  http.StatusTooManyRequests,
	errs.KindUnavailable:  http.StatusServiceUnavailable,
	errs.KindNoRates:      http.StatusServiceUnavailable,
}

// WriteError maps err to the error envelope. Unknown errors become 500s and
// are logged without leaking details to the client.
func WriteError(w http.ResponseWriter, r *http.Request, log *slog.Logger, err error) {
	body := platform.Error{RequestID: RequestID(r.Context())}
	status := http.StatusInternalServerError
	if de, ok := errs.As(err); ok {
		status = statusByKind[de.Kind]
		body.Error, body.Code, body.Field = de.Message, string(de.Kind), de.Field
	} else {
		body.Error, body.Code = "Something went wrong. Please try again.", "INTERNAL"
		log.Error("middleware.error.internal", "requestId", body.RequestID, "path", r.URL.Path, "error", err)
	}
	WriteJSON(w, status, body)
}

// WriteJSON writes v as JSON with status.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
