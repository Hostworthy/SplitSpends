// Package middleware holds HTTP cross-cutting concerns: request ids, access
// logs, panic recovery, CORS, authentication, rate limits and the mapping of
// domain errors to HTTP responses.
package middleware

import (
	"container/list"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
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

// maxKeys bounds the memory a Limiter uses. When it is full the least
// recently seen key is dropped, which only resets that key's budget.
const maxKeys = 100_000

// Limiter allows a number of requests per sliding window per key. It
// counts the current and the previous window and weights the previous one
// by how much of it still overlaps, so a burst across a window boundary
// cannot double the limit. Its state is in memory, so every server
// instance keeps its own counts.
type Limiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	now    func() time.Time
	keys   map[string]*list.Element
	// recent orders buckets by last use, most recent first, so idle and
	// overflowing buckets are dropped from the back in constant time.
	recent *list.List
}

type bucket struct {
	key      string
	start    time.Time
	previous int
	current  int
	seen     time.Time
}

// NewLimiter allows limit requests per window per key.
func NewLimiter(limit int, per time.Duration) *Limiter {
	return &Limiter{limit: limit, window: per, now: time.Now,
		keys: map[string]*list.Element{}, recent: list.New()}
}

// Allow records a request for key and reports whether it is within the
// limit. Refused requests are not counted.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.sweep(now)
	e, ok := l.keys[key]
	if !ok {
		if len(l.keys) >= maxKeys {
			l.drop(l.recent.Back())
		}
		e = l.recent.PushFront(&bucket{key: key, start: now})
		l.keys[key] = e
	}
	l.recent.MoveToFront(e)
	b := e.Value.(*bucket)
	b.seen = now
	elapsed := now.Sub(b.start)
	switch {
	case elapsed >= 2*l.window:
		b.start, b.previous, b.current, elapsed = now, 0, 0, 0
	case elapsed >= l.window:
		b.start, b.previous, b.current = b.start.Add(l.window), b.current, 0
		elapsed -= l.window
	}
	// The integer division rounds the overlapping share of the previous
	// window down, never refusing a request the limit allows.
	used := int64(b.previous)*int64(l.window-elapsed)/int64(l.window) + int64(b.current)
	if used >= int64(l.limit) {
		return false
	}
	b.current++
	return true
}

// sweep drops buckets idle for two windows; they no longer count anything.
// Each bucket is dropped at most once, so the cost per call is constant on
// average.
func (l *Limiter) sweep(now time.Time) {
	for e := l.recent.Back(); e != nil && now.Sub(e.Value.(*bucket).seen) >= 2*l.window; e = l.recent.Back() {
		l.drop(e)
	}
}

func (l *Limiter) drop(e *list.Element) {
	delete(l.keys, e.Value.(*bucket).key)
	l.recent.Remove(e)
}

// Key picks the budget a request counts against.
type Key func(r *http.Request) string

// ByAddress counts requests per client address, with hops trusted proxies
// in front of the server (see ClientIP). An IPv6 client usually holds a
// whole /64, so its addresses share one budget.
func ByAddress(hops int) Key {
	return func(r *http.Request) string {
		address := ClientIP(r, hops)
		ip, err := netip.ParseAddr(address)
		if err != nil {
			return address
		}
		if ip = ip.Unmap(); ip.Is4() {
			return ip.String()
		}
		return netip.PrefixFrom(ip.WithZone(""), 64).Masked().String()
	}
}

// ByUser counts requests per signed-in user, and per client address (see
// ByAddress) when nobody is signed in. Use it in a Limit placed inside
// RequireAuth, which sets the user.
func ByUser(hops int) Key {
	address := ByAddress(hops)
	return func(r *http.Request) string {
		if p := Principal(r.Context()); p != nil {
			return "user:" + p.UserID.String()
		}
		return address(r)
	}
}

// Limit applies l to next, counting each request against key.
func Limit(l *Limiter, log *slog.Logger, key Key, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !l.Allow(key(r)) {
			WriteError(w, r, log, errs.RateLimited("Too many requests. Slow down and try again shortly."))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ClientIP returns the caller's address. hops is the number of trusted
// proxies in front of the server, each of which appends the address it was
// connected from to X-Forwarded-For. Entries further left were written by
// the client and prove nothing, so the address is the hops-th entry from the
// right. When the header is shorter than that, or the entry is not an
// address, the request did not come through the proxies as configured and
// the peer address is used instead.
func ClientIP(r *http.Request, hops int) string {
	if hops > 0 {
		var entries []string
		for _, v := range r.Header.Values("X-Forwarded-For") {
			entries = append(entries, strings.Split(v, ",")...)
		}
		if len(entries) >= hops {
			if ip := address(entries[len(entries)-hops]); ip != "" {
				return ip
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// address parses one X-Forwarded-For entry, which some proxies write with a
// port or in brackets, and returns "" when it is not an IP address.
func address(entry string) string {
	entry = strings.TrimSpace(entry)
	if host, _, err := net.SplitHostPort(entry); err == nil {
		entry = host
	}
	ip, err := netip.ParseAddr(strings.Trim(entry, "[]"))
	if err != nil {
		return ""
	}
	return ip.Unmap().String()
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
	errs.KindPending:      http.StatusForbidden,
	errs.KindDeclined:     http.StatusForbidden,
}

// WriteError maps err to the error envelope. Unknown errors become 500s and
// are logged without leaking details to the client.
func WriteError(w http.ResponseWriter, r *http.Request, log *slog.Logger, err error) {
	body := platform.Error{RequestID: RequestID(r.Context())}
	status := http.StatusInternalServerError
	if de, ok := errs.As(err); ok {
		body.Error, body.Code, body.Field = de.Message, string(de.Kind), de.Field
		if mapped, known := statusByKind[de.Kind]; known {
			status = mapped
		} else {
			// A kind added to errs without a status still answers, as a 500.
			log.Error("middleware.error.unmapped", "requestId", body.RequestID, "path", r.URL.Path, "kind", de.Kind)
		}
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
