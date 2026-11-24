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
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/platform"
)

// Client calls the SplitSpends HTTP API with a bearer token.
type Client struct {
	base  string
	token string
	http  *http.Client

	once   sync.Once
	config *platform.Config
	cfgErr error
}

// NewClient builds a client. base is the server origin, such as
// https://split.example.com.
func NewClient(base, token string, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: 3 * time.Minute}
	}
	return &Client{base: strings.TrimRight(base, "/"), token: token, http: hc}
}

// APIError is a failed API call.
type APIError struct {
	Status int
	Body   platform.Error
}

func (e *APIError) Error() string {
	if e.Body.Field != "" {
		return fmt.Sprintf("%s (field %s)", e.Body.Error, e.Body.Field)
	}
	if e.Body.Error != "" {
		return e.Body.Error
	}
	return fmt.Sprintf("request failed with status %d", e.Status)
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(buf)
	}
	u := c.base + platform.Prefix + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, reader)
	if err != nil {
		return err
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
		return fmt.Errorf("mcp.api.unreachable: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= http.StatusMultipleChoices {
		apiErr := &APIError{Status: resp.StatusCode}
		_ = json.Unmarshal(raw, &apiErr.Body)
		return apiErr
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func get[T any](ctx context.Context, c *Client, path string, query url.Values) (T, error) {
	var env platform.Details[T]
	err := c.do(ctx, http.MethodGet, path, query, nil, &env)
	return env.Data, err
}

func getList[T any](ctx context.Context, c *Client, path string, query url.Values) (platform.List[T], error) {
	var env platform.List[T]
	err := c.do(ctx, http.MethodGet, path, query, nil, &env)
	return env, err
}

func send[T any](ctx context.Context, c *Client, method, path string, body any) (T, error) {
	var env platform.Details[T]
	err := c.do(ctx, method, path, nil, body, &env)
	return env.Data, err
}

// Config returns the cached instance configuration.
func (c *Client) Config(ctx context.Context) (*platform.Config, error) {
	c.once.Do(func() {
		cfg, err := get[platform.Config](ctx, c, "/config", nil)
		c.config, c.cfgErr = &cfg, err
	})
	return c.config, c.cfgErr
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

// minor converts a decimal string such as "12.50" into minor units.
func (c *Client) minor(ctx context.Context, amount, currency string) (int64, error) {
	exp, err := c.exponent(ctx, currency)
	if err != nil {
		return 0, err
	}
	r, ok := new(big.Rat).SetString(strings.TrimSpace(amount))
	if !ok {
		return 0, fmt.Errorf("amount %q is not a decimal number", amount)
	}
	r.Mul(r, new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(exp)), nil)))
	if !r.IsInt() {
		return 0, fmt.Errorf("amount %q has more decimals than %s allows", amount, currency)
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
