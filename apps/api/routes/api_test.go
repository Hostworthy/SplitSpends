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
	"strings"
	"testing"

	"github.com/Hostworthy/SplitSpends/packages/bootstrap/bootstraptest"
	"github.com/Hostworthy/SplitSpends/packages/middleware"
	"github.com/Hostworthy/SplitSpends/packages/platform"
)

// server is the API wired on the test database and served over HTTP behind
// the same middleware chain as apps/api/main.go.
type server struct {
	*bootstraptest.Env
	url string
}

// jobsSecret is the bearer token POST /internal/jobs accepts in tests.
const jobsSecret = "test-jobs-secret"

// newServer starts the API on a fresh schema. It skips the test when
// TEST_DATABASE_URL is not set. Options work as in bootstraptest.Open.
func newServer(t *testing.T, o bootstraptest.Options) *server {
	t.Helper()
	env := bootstraptest.Open(t, o)
	if env.Config.JobsSecret == "" {
		env.Config.JobsSecret = jobsSecret
	}
	log := slog.New(slog.DiscardHandler)
	api := New(Deps{Services: env.Services, Config: env.Config, Log: log,
		Ping: func(ctx context.Context) error { return env.DB.Pool.Ping(ctx) },
		Jobs: func(context.Context) error { return nil }}).Routes()
	mux := http.NewServeMux()
	mux.Handle("/api/", api)
	mux.Handle("/health", api)
	mux.Handle("/internal/", api)
	srv := httptest.NewServer(middleware.Observe(log, middleware.Security(middleware.CORS(env.Config.CORSOrigins, mux))))
	t.Cleanup(srv.Close)
	return &server{Env: env, url: srv.URL}
}

// signIn requests a login code over HTTP, verifies it and returns the
// session. New emails become new accounts.
func (s *server) signIn(t *testing.T, email, name string) platform.LoginResult {
	t.Helper()
	s.Notifier.Forget(email)
	s.call(t, http.MethodPost, platform.Prefix+"/auth/otp/request", "", platform.OTPRequest{Email: email}).want(t, http.StatusAccepted)
	code := s.Notifier.Code(email)
	if code == "" {
		t.Fatalf("no code was sent to %s", email)
	}
	res := s.call(t, http.MethodPost, platform.Prefix+"/auth/otp/verify", "", platform.OTPVerify{Email: email, Code: code, Name: name})
	return data[platform.LoginResult](t, res.want(t, http.StatusOK))
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
	return s.send(t, req, token)
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
