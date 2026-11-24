package exchange

import (
	"context"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const openFormat = `{"result":"success","provider":"https://www.exchangerate-api.com","time_last_update_unix":1795996801,"base_code":"USD","rates":{"USD":1,"EUR":0.912345,"JPY":159.07,"XAU":0.0004,"BTC":1e-5}}`

const frankfurterFormat = `{"amount":1.0,"base":"EUR","date":"2026-11-24","rates":{"USD":1.0963,"GBP":0.8371}}`

func TestFetchOpenFormat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(openFormat))
	}))
	defer srv.Close()
	table, err := New(srv.URL).Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if table.Base != "USD" || table.Source != "https://www.exchangerate-api.com" {
		t.Fatalf("table = %+v", table)
	}
	if want := time.Unix(1795996801, 0).UTC(); !table.Published.Equal(want) {
		t.Fatalf("published = %v", table.Published)
	}
	// Exact decimal text, never a float.
	if r, _ := table.Rate("EUR"); r.Cmp(big.NewRat(912345, 1000000)) != 0 {
		t.Fatalf("EUR = %v", r)
	}
	if _, ok := table.Rate("XAU"); ok {
		t.Fatal("unsupported codes should be dropped")
	}
}

func TestParseFrankfurterFormat(t *testing.T) {
	table, err := parse(strings.NewReader(frankfurterFormat), "https://rates.example.com/v1/latest?base=EUR")
	if err != nil {
		t.Fatal(err)
	}
	if table.Base != "EUR" || table.Source != "https://rates.example.com" || table.Published.Format(time.DateOnly) != "2026-11-24" {
		t.Fatalf("table = %+v", table)
	}
	if len(table.Rates) != 3 {
		t.Fatalf("rates = %v", table.Codes())
	}
}

func TestFetchErrors(t *testing.T) {
	for name, body := range map[string]string{
		"provider error": `{"result":"error","error-type":"unsupported-code"}`,
		"no rates":       `{"base":"USD","rates":{}}`,
		"not json":       `<html>`,
	} {
		if _, err := parse(strings.NewReader(body), "https://x"); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	if _, err := New(srv.URL).Fetch(context.Background()); err == nil || !strings.HasPrefix(err.Error(), "exchange.fetch.status") {
		t.Fatalf("err = %v", err)
	}
}
