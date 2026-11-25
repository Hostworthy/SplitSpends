// Package mcp exposes SplitSpends to AI assistants over the Model Context
// Protocol. It is a client of the public HTTP API, so every call goes through
// the same validation and authorization as the apps. It never touches the
// database or core services directly.
package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/platform"
)

// Client calls the SplitSpends HTTP API with a bearer token.
type Client struct {
	base     string
	token    string
	http     *http.Client
	settings *settings
}

// configTTL is how long the instance configuration is reused, so
// currencies added on the server appear without restarting the MCP server.
const configTTL = time.Hour

// settings caches the instance configuration. Only successful fetches are
// kept: a failed one is retried by the next caller, with its own context.
type settings struct {
	mu      sync.Mutex
	config  *platform.Config
	fetched time.Time
	now     func() time.Time
}

// NewClient builds a client. base is the server origin, such as
// https://split.example.com. hc may be nil; a client passed in is copied
// so that, like the default one, it never follows redirects.
func NewClient(base, token string, hc *http.Client) *Client {
	// No tool uploads anything, so an API call that takes longer than this
	// is stuck rather than busy.
	client := http.Client{Timeout: 30 * time.Second}
	if hc != nil {
		client = *hc
	}
	// Following a 301, 302 or 303 turns a POST or DELETE into a GET, whose
	// answer would then read as the write succeeding.
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{base: strings.TrimRight(base, "/"), token: token, http: &client, settings: &settings{now: time.Now}}
}

// Origin checks the server address the stdio binary is given and returns it
// without trailing slashes. It must be an http or https URL with a host and
// no user, query or fragment; a path prefix is kept for servers behind a
// reverse proxy, but it must not already end in the API prefix. insecure
// reports plain http to another machine, where the token travels
// unencrypted.
func Origin(raw string) (origin string, insecure bool, err error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Opaque != "" {
		return "", false, fmt.Errorf("%q is not a server address such as https://split.example.com", raw)
	}
	if u.User != nil || strings.ContainsAny(raw, "?#") {
		return "", false, fmt.Errorf("%q must not contain a user name, password, query or fragment", raw)
	}
	origin = strings.TrimRight(raw, "/")
	if strings.HasSuffix(strings.TrimRight(u.Path, "/"), platform.Prefix) {
		return "", false, fmt.Errorf("%q ends in %s; give the server address without it", raw, platform.Prefix)
	}
	host := u.Hostname()
	local := strings.EqualFold(host, "localhost")
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		local = true
	}
	return origin, u.Scheme == "http" && !local, nil
}

// with returns a client that calls the same API with token, sharing the
// HTTP client and the cached configuration.
func (c *Client) with(token string) *Client {
	copied := *c
	copied.token = token
	return &copied
}

// APIError is a failed API call.
type APIError struct {
	Status int
	Body   platform.Error
}

func (e *APIError) Error() string {
	msg := fmt.Sprintf("request failed with status %d", e.Status)
	if e.Body.Error != "" {
		msg = e.Body.Error
	}
	if e.Body.Field != "" {
		msg += " (field " + e.Body.Field + ")"
	}
	if e.Body.RequestID != "" {
		msg += " (request id " + e.Body.RequestID + ")"
	}
	return msg
}

// failure is a call that got no usable answer from the API. Its message is
// fixed and safe to show; the detail, which can name internal addresses,
// is only logged.
type failure struct {
	message string
	method  string
	path    string
	status  int
	err     error
}

func (f *failure) Error() string { return f.message }

func (f *failure) Unwrap() error { return f.err }

// Messages of a failure.
const (
	unreachable = "the Split Spends server could not be reached; try again in a moment"
	unexpected  = "the Split Spends server sent an answer that could not be read"
	unprepared  = "the request could not be prepared; the server logged the details"
	oversized   = "the Split Spends server sent a response too large to read; ask for fewer results"
)

// maxResponse is the largest answer read from the API.
const maxResponse = 8 << 20

// do sends one request and returns the body of a 2xx answer.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, body any) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return nil, &failure{message: unprepared, method: method, path: path, err: err}
		}
		reader = bytes.NewReader(buf)
	}
	u := c.base + platform.Prefix + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, reader)
	if err != nil {
		return nil, &failure{message: unprepared, method: method, path: path, err: err}
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("User-Agent", "splitspends-mcp")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, &failure{message: unreachable, method: method, path: path, err: err}
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse+1))
	if err != nil {
		return nil, &failure{message: unexpected, method: method, path: path, status: resp.StatusCode, err: err}
	}
	if len(raw) > maxResponse {
		return nil, &failure{message: oversized, method: method, path: path, status: resp.StatusCode, err: fmt.Errorf("more than %d bytes", maxResponse)}
	}
	if resp.StatusCode >= http.StatusMultipleChoices && resp.StatusCode < http.StatusBadRequest {
		target := resp.Header.Get("Location")
		if loc, err := resp.Location(); err == nil {
			target = loc.String()
		}
		return nil, fmt.Errorf("mcp.api.redirected: the server redirected this request to %s and nothing was changed; set SPLITSPENDS_URL to the address the server uses", target)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		apiErr := &APIError{Status: resp.StatusCode}
		_ = json.Unmarshal(raw, &apiErr.Body)
		return nil, apiErr
	}
	return raw, nil
}

// member decodes the key member of a success envelope from path into out.
// An answer without it came from something other than the API endpoint
// and must not read as success.
func member(path string, raw []byte, key string, out any) error {
	var env map[string]json.RawMessage
	if err := json.Unmarshal(raw, &env); err != nil {
		return &failure{message: unexpected, path: path, err: err}
	}
	value, ok := env[key]
	if !ok {
		return fmt.Errorf("the answer has no %q; check that SPLITSPENDS_URL points at a Split Spends server", key)
	}
	if err := json.Unmarshal(value, out); err != nil {
		return &failure{message: unexpected, path: path, err: err}
	}
	return nil
}

func get[T any](ctx context.Context, c *Client, path string, query url.Values) (T, error) {
	var data T
	raw, err := c.do(ctx, http.MethodGet, path, query, nil)
	if err == nil {
		err = member(path, raw, "data", &data)
	}
	return data, err
}

func getList[T any](ctx context.Context, c *Client, path string, query url.Values) (platform.List[T], error) {
	var list platform.List[T]
	raw, err := c.do(ctx, http.MethodGet, path, query, nil)
	if err == nil {
		err = member(path, raw, "items", &list.Items)
	}
	if err == nil {
		err = member(path, raw, "total", &list.Total)
	}
	return list, err
}

func send[T any](ctx context.Context, c *Client, method, path string, body any) (T, error) {
	var data T
	raw, err := c.do(ctx, method, path, nil, body)
	if err == nil {
		err = member(path, raw, "data", &data)
	}
	return data, err
}

// Config returns the instance configuration, fetched at most once per
// configTTL. When a refresh fails the previous configuration is kept.
func (c *Client) Config(ctx context.Context) (*platform.Config, error) {
	s := c.settings
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.config != nil && s.now().Sub(s.fetched) < configTTL {
		return s.config, nil
	}
	cfg, err := get[platform.Config](ctx, c, "/config", nil)
	if err != nil {
		if s.config != nil {
			return s.config, nil
		}
		return nil, err
	}
	s.config, s.fetched = &cfg, s.now()
	return s.config, nil
}

// exponent returns the minor-unit exponent of a currency.
func (c *Client) exponent(ctx context.Context, code string) (int, error) {
	cfg, err := c.Config(ctx)
	if err != nil {
		return 0, err
	}
	for _, cur := range cfg.Currencies {
		if strings.EqualFold(cur.Code, code) {
			return cur.Exponent, nil
		}
	}
	return 0, fmt.Errorf("unsupported currency %q", code)
}

// decimal is the API's plain decimal syntax for split values (money.Decimal
// in core: no exponents, fractions, base prefixes or digit separators),
// without a sign, since nothing an assistant sends here is negative.
var decimal = regexp.MustCompile(`^(\d+(\.\d*)?|\.\d+)$`)

// decimalLength is the API's length limit for a split value.
const decimalLength = 20

// plain reports whether s, already trimmed, is a decimal the API accepts.
func plain(s string) bool {
	return len(s) <= decimalLength && decimal.MatchString(s)
}

// minor converts a decimal string such as "12.50" into minor units.
func (c *Client) minor(ctx context.Context, amount, currency string) (int64, error) {
	cur, err := c.unit(ctx, currency)
	if err != nil {
		return 0, err
	}
	return cur.minor(amount)
}

// unit returns a currency with its minor-unit exponent.
func (c *Client) unit(ctx context.Context, code string) (unit, error) {
	exp, err := c.exponent(ctx, code)
	return unit{code: code, exponent: exp}, err
}

// unit is a currency and its minor-unit exponent.
type unit struct {
	code     string
	exponent int
}

// minor converts a decimal string such as "12.50" into minor units.
func (u unit) minor(amount string) (int64, error) {
	s := strings.TrimSpace(amount)
	if !plain(s) {
		return 0, fmt.Errorf("amount %q is not a decimal number such as 12.50", amount)
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return 0, fmt.Errorf("amount %q is not a decimal number such as 12.50", amount)
	}
	r.Mul(r, new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(u.exponent)), nil)))
	if !r.IsInt() {
		return 0, fmt.Errorf("amount %q has more decimals than %s allows", amount, u.code)
	}
	if !r.Num().IsInt64() {
		return 0, fmt.Errorf("amount %q is too large", amount)
	}
	return r.Num().Int64(), nil
}

// format renders minor units as a decimal string.
func (c *Client) format(ctx context.Context, amount int64, currency string) string {
	exp, err := c.exponent(ctx, currency)
	if err != nil {
		return fmt.Sprintf("%d %s (minor units)", amount, currency)
	}
	r := new(big.Rat).SetFrac(big.NewInt(amount), new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(exp)), nil))
	return r.FloatString(exp) + " " + strings.ToUpper(currency)
}
