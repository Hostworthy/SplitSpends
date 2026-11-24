// Package exchange fetches exchange rates over HTTP. It understands the
// open ExchangeRate-API format (open.er-api.com, the default) and the
// Frankfurter format (frankfurter.dev, which can also be self-hosted).
package exchange

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/rates"
)

// DefaultURL is free, needs no key and covers about 160 currencies.
const DefaultURL = "https://open.er-api.com/v6/latest/USD"

// Client implements rates.Source.
type Client struct {
	URL  string
	HTTP *http.Client
}

// New builds a Client for url.
func New(url string) *Client {
	return &Client{URL: url, HTTP: &http.Client{Timeout: 15 * time.Second}}
}

type response struct {
	Result   string                 `json:"result"`
	Provider string                 `json:"provider"`
	BaseCode string                 `json:"base_code"`
	Base     string                 `json:"base"`
	Date     string                 `json:"date"`
	Updated  int64                  `json:"time_last_update_unix"`
	Rates    map[string]json.Number `json:"rates"`
}

// Fetch downloads and validates the current rates.
func (c *Client) Fetch(ctx context.Context) (rates.Table, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL, nil)
	if err != nil {
		return rates.Table{}, fmt.Errorf("exchange.request.invalid: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return rates.Table{}, fmt.Errorf("exchange.fetch.failed: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return rates.Table{}, fmt.Errorf("exchange.fetch.status: %d", res.StatusCode)
	}
	return parse(io.LimitReader(res.Body, 1<<20), c.URL)
}

func parse(r io.Reader, source string) (rates.Table, error) {
	dec := json.NewDecoder(r)
	dec.UseNumber()
	var body response
	if err := dec.Decode(&body); err != nil {
		return rates.Table{}, fmt.Errorf("exchange.response.invalid: %w", err)
	}
	if body.Result != "" && body.Result != "success" {
		return rates.Table{}, fmt.Errorf("exchange.response.error: %s", body.Result)
	}
	base := body.BaseCode
	if base == "" {
		base = body.Base
	}
	raw := make(map[string]string, len(body.Rates))
	for code, n := range body.Rates {
		raw[code] = n.String()
	}
	t, ok := rates.New(base, raw)
	if !ok {
		return rates.Table{}, fmt.Errorf("exchange.response.empty: base %q", base)
	}
	switch {
	case body.Updated > 0:
		t.Published = time.Unix(body.Updated, 0).UTC()
	case body.Date != "":
		if d, err := time.Parse(time.DateOnly, body.Date); err == nil {
			t.Published = d
		}
	}
	t.Source = body.Provider
	if t.Source == "" {
		if u, err := url.Parse(source); err == nil {
			t.Source = u.Scheme + "://" + u.Host
		}
	}
	return t, nil
}
