package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Hostworthy/SplitSpends/packages/bootstrap/bootstraptest"
	"github.com/Hostworthy/SplitSpends/packages/middleware"
	"github.com/Hostworthy/SplitSpends/packages/platform"
)

// server is the API wired on the test database and served over HTTP behind
// the same middleware chain as apps/api/main.go.
type server struct {
	*bootstraptest.Env
	url     string
	clients atomic.Uint32 // client addresses handed out by signIn
	logs    *logs
}

// logs keeps what the API logged, for tests that check a failure was
// reported.
type logs struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *logs) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

// has reports whether a line with message key was logged.
func (l *logs) has(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Contains(l.buf.String(), `"msg":"`+key+`"`)
}

// jobsSecret is the bearer token POST /internal/jobs accepts in tests.
const jobsSecret = "test-jobs-secret"

// newServer starts the API on a fresh schema. It skips the test when
// TEST_DATABASE_URL is not set. Options work as in bootstraptest.Open.
//
// The server trusts one proxy hop, as behind a reverse proxy, so signIn can
// give every person a client address of their own and a test can sign in
// as many people as it needs without reaching the sign-in rate limit.
// Requests without X-Forwarded-For all count against the test's own address.
func newServer(t *testing.T, o bootstraptest.Options) *server {
	t.Helper()
	env := bootstraptest.Open(t, o)
	if env.Config.JobsSecret == "" {
		env.Config.JobsSecret = jobsSecret
	}
	env.Config.ProxyHops = 1
	logged := &logs{}
	log := slog.New(slog.NewJSONHandler(logged, nil))
	api := New(Deps{Services: env.Services, Settings: env.Config.Settings(), Log: log,
		Ping: func(ctx context.Context) error { return env.DB.Pool.Ping(ctx) },
		Jobs: func(context.Context) error { return nil }}).Routes()
	mux := http.NewServeMux()
	mux.Handle("/api/", api)
	mux.Handle("/health", api)
	mux.Handle("/internal/", api)
	srv := httptest.NewServer(middleware.Observe(log, middleware.Security(false, middleware.CORS(env.Config.CORSOrigins, mux))))
	t.Cleanup(srv.Close)
	return &server{Env: env, url: srv.URL, logs: logged}
}

// signIn requests a login code over HTTP, verifies it and returns the
// session. New emails become new accounts. Each call comes from a new client
// address, so sign-ins do not use up each other's rate limit.
func (s *server) signIn(t *testing.T, email, name string) platform.LoginResult {
	t.Helper()
	n := s.clients.Add(1)
	from := netip.AddrFrom4([4]byte{10, byte(n >> 16), byte(n >> 8), byte(n)}).String()
	post := func(path string, body any) *reply {
		req := s.request(t, http.MethodPost, platform.Prefix+path, body)
		req.Header.Set("X-Forwarded-For", from)
		return s.send(t, req, "")
	}
	s.Notifier.Forget(email)
	post("/auth/otp/request", platform.OTPRequest{Email: email}).want(t, http.StatusAccepted)
	code := s.Notifier.Code(email)
	if code == "" {
		t.Fatalf("no code was sent to %s", email)
	}
	res := post("/auth/otp/verify", platform.OTPVerify{Email: email, Code: code, Name: name})
	return data[platform.LoginResult](t, res.want(t, http.StatusOK))
}

// group creates a group in EUR owned by token's user with the people at
// emails as members, and checks they all joined.
func (s *server) group(t *testing.T, token, name string, emails ...string) platform.Group {
	t.Helper()
	currency := "EUR"
	in := platform.GroupCreate{GroupPatch: platform.GroupPatch{Name: &name, DefaultCurrency: &currency}}
	for _, email := range emails {
		in.Members = append(in.Members, platform.Invitee{Email: email})
	}
	g := data[platform.Group](t, s.call(t, http.MethodPost, platform.Prefix+"/groups", token, in).want(t, http.StatusCreated))
	if len(g.Members) != len(emails)+1 {
		t.Fatalf("want %d members, got %+v", len(emails)+1, g.Members)
	}
	return g
}

// expense books total EUR minor units in the group, paid by payer and split
// equally between the people in split (user ids).
func (s *server) expense(t *testing.T, token, groupID string, total int64, payer string, split ...string) platform.Expense {
	t.Helper()
	in := platform.ExpenseInput{GroupID: &groupID, Description: "Groceries", Currency: "EUR", Total: total,
		SplitMethod: "equal", Payers: []platform.Payer{{UserID: payer, Amount: total}}}
	for _, id := range split {
		in.Splits = append(in.Splits, platform.Split{UserID: id})
	}
	return data[platform.Expense](t, s.call(t, http.MethodPost, platform.Prefix+"/expenses", token, in).want(t, http.StatusCreated))
}

// request is one row of a table of API calls. path starts after
// platform.Prefix; body is sent as JSON unless nil.
type request struct {
	method, path string
	body         any
}

// refused sends every request with token and checks that each answers
// status with an error envelope of code. Use it for who-may-not tables:
// outsiders get 404 NOT_FOUND, so they cannot tell what exists.
func (s *server) refused(t *testing.T, token string, status int, code string, requests []request) {
	t.Helper()
	for _, r := range requests {
		res := s.call(t, r.method, platform.Prefix+r.path, token, r.body)
		if res.status != status {
			t.Errorf("%s %s: want %d, got %d: %s", r.method, r.path, status, res.status, strings.TrimSpace(string(res.body)))
			continue
		}
		if e := res.failure(t); e.Code != code || e.Error == "" || e.RequestID == "" {
			t.Errorf("%s %s: want code %s, got envelope %+v", r.method, r.path, code, e)
		}
	}
}

// reply is a finished response with its body read.
type reply struct {
	method, path string
	status       int
	header       http.Header
	body         []byte
}

// call sends body as JSON (nil sends none) with token as the bearer token
// ("" sends none). path starts at the server root, so API paths begin with
// platform.Prefix.
func (s *server) call(t *testing.T, method, path, token string, body any) *reply {
	t.Helper()
	return s.send(t, s.request(t, method, path, body), token)
}

// request builds a call without sending it, for tests that set headers.
func (s *server) request(t *testing.T, method, path string, body any) *http.Request {
	t.Helper()
	var payload io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		payload = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, s.url+path, payload)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

// upload posts data as the multipart form field "file".
func (s *server) upload(t *testing.T, path, token string, data []byte) *reply {
	t.Helper()
	var buf bytes.Buffer
	form := multipart.NewWriter(&buf)
	part, err := form.CreateFormFile("file", "receipt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := form.Close(); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, s.url+path, &buf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", form.FormDataContentType())
	return s.send(t, req, token)
}

func (s *server) send(t *testing.T, req *http.Request, token string) *reply {
	t.Helper()
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return &reply{method: req.Method, path: req.URL.Path, status: res.StatusCode, header: res.Header, body: raw}
}

// want fails the test unless the response has status.
func (r *reply) want(t *testing.T, status int) *reply {
	t.Helper()
	if r.status != status {
		t.Fatalf("%s %s: want %d, got %d: %s", r.method, r.path, status, r.status, strings.TrimSpace(string(r.body)))
	}
	return r
}

// failure decodes the error envelope.
func (r *reply) failure(t *testing.T) platform.Error {
	t.Helper()
	var e platform.Error
	if err := json.Unmarshal(r.body, &e); err != nil {
		t.Fatalf("%s %s: not an error envelope: %v: %s", r.method, r.path, err, r.body)
	}
	return e
}

// data decodes a {data} envelope.
func data[T any](t *testing.T, r *reply) T {
	t.Helper()
	var d platform.Details[T]
	if err := json.Unmarshal(r.body, &d); err != nil {
		t.Fatalf("%s %s: not a data envelope: %v: %s", r.method, r.path, err, r.body)
	}
	return d.Data
}

// items decodes an {items,total} envelope.
func items[T any](t *testing.T, r *reply) platform.List[T] {
	t.Helper()
	var l platform.List[T]
	if err := json.Unmarshal(r.body, &l); err != nil {
		t.Fatalf("%s %s: not a list envelope: %v: %s", r.method, r.path, err, r.body)
	}
	return l
}
