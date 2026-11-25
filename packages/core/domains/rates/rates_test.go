package rates

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"math/big"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/shared/clock"
)

func TestSharedVectors(t *testing.T) {
	raw, err := os.ReadFile("../../../../spec/vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Conversions struct {
			Table struct {
				Base  string
				Rates map[string]string
			}
			Convert []struct {
				Name   string
				Amount Amount
				To     string
				Expect *int64
			}
			Totals []struct {
				Name    string
				Amounts []Amount
				To      string
				Expect  int64
				Missing []string
			}
		}
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	c := doc.Conversions
	table, ok := New(c.Table.Base, c.Table.Rates)
	if !ok {
		t.Fatal("table rejected")
	}
	if len(c.Convert) == 0 || len(c.Totals) == 0 {
		t.Fatal("no conversion vectors")
	}
	for _, v := range c.Convert {
		got, ok := table.Convert(v.Amount, v.To)
		switch {
		case v.Expect == nil && ok:
			t.Errorf("%s: got %d, want no conversion", v.Name, got)
		case v.Expect != nil && (!ok || got != *v.Expect):
			t.Errorf("%s: got %d (%v), want %d", v.Name, got, ok, *v.Expect)
		}
	}
	for _, v := range c.Totals {
		got, missing := table.Total(v.Amounts, v.To)
		if missing == nil {
			missing = []string{}
		}
		if got != v.Expect || !reflect.DeepEqual(missing, v.Missing) {
			t.Errorf("%s: got %d %v, want %d %v", v.Name, got, missing, v.Expect, v.Missing)
		}
	}
}

func TestNewDropsInvalidRates(t *testing.T) {
	table, ok := New("usd", map[string]string{"EUR": "0.9", "GBP": "-1", "JPY": "abc", "XXX": "2", "USD": "5", "INR": "1e2"})
	if !ok {
		t.Fatal("table rejected")
	}
	if got := table.Codes(); !reflect.DeepEqual(got, []string{"EUR", "INR", "USD"}) {
		t.Fatalf("codes = %v", got)
	}
	if r, _ := table.Rate("USD"); r.Cmp(big.NewRat(1, 1)) != 0 {
		t.Fatalf("base rate = %v", r)
	}
	if _, ok := New("USD", map[string]string{"GBP": "0"}); ok {
		t.Fatal("a table with only the base should be rejected")
	}
	if _, ok := New("XXX", map[string]string{"EUR": "1"}); ok {
		t.Fatal("unknown base should be rejected")
	}
}

func TestNewDropsImplausibleRates(t *testing.T) {
	table, ok := New("USD", map[string]string{"EUR": "1e30", "GBP": "1e-50", "JPY": "1/3", "INR": "83.2", "KWD": "1e-9", "VND": "1e9"})
	if !ok {
		t.Fatal("table rejected")
	}
	if got := table.Codes(); !reflect.DeepEqual(got, []string{"INR", "KWD", "USD", "VND"}) {
		t.Fatalf("codes = %v", got)
	}
}

func TestConvertOverflowReportsFalse(t *testing.T) {
	table, ok := New("USD", map[string]string{"VND": "1e9"})
	if !ok {
		t.Fatal("table rejected")
	}
	if v, ok := table.Convert(Amount{Currency: "USD", Minor: math.MaxInt64 / 2}, "VND"); ok {
		t.Fatalf("convert = %d, want no conversion", v)
	}
	big := []Amount{{Currency: "USD", Minor: math.MaxInt64}, {Currency: "USD", Minor: math.MaxInt64}}
	if total, missing := table.Total(big, "USD"); total != 0 || !reflect.DeepEqual(missing, []string{"USD"}) {
		t.Fatalf("total = %d %v, want 0 [USD]", total, missing)
	}
}

func TestDecimal(t *testing.T) {
	long := "0." + strings.Repeat("123456789", 5)
	for in, want := range map[string]string{"0.912": "0.912", "1e2": "100", "159.07": "159.07", "1.5e-3": "0.0015", long: long, "1e-45": "0." + strings.Repeat("0", 44) + "1"} {
		r, _ := new(big.Rat).SetString(in)
		if got := Decimal(r); got != want {
			t.Errorf("Decimal(%s) = %s, want %s", in, got, want)
		}
	}
}

type memRepo struct{ t *Table }

func (m *memRepo) Latest(context.Context) (*Table, error) { return m.t, nil }
func (m *memRepo) Save(_ context.Context, t Table) error  { m.t = &t; return nil }

type fakeSource struct {
	calls int
	err   error
}

func (f *fakeSource) Fetch(context.Context) (Table, error) {
	f.calls++
	if f.err != nil {
		return Table{}, f.err
	}
	t, _ := New("USD", map[string]string{"EUR": "0.9"})
	return t, nil
}

func TestServiceCachesAndFallsBack(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	c := &clock.Fixed{At: now}
	repo, src := &memRepo{}, &fakeSource{}
	var logs bytes.Buffer
	s := NewService(repo, src, c, slog.New(slog.NewJSONHandler(&logs, nil)))

	got, err := s.Latest(ctx)
	if err != nil || got == nil || src.calls != 1 {
		t.Fatalf("first Latest: %v %v calls=%d", got, err, src.calls)
	}
	if !got.Fetched.Equal(now) || !got.Published.Equal(now) {
		t.Fatalf("timestamps: %+v", got)
	}
	if _, _ = s.Latest(ctx); src.calls != 1 {
		t.Fatalf("fresh table should be served from the repository, calls=%d", src.calls)
	}

	c.At = now.Add(Fresh + time.Minute)
	src.err = errors.New("provider down")
	got, err = s.Latest(ctx)
	if err != nil || got == nil || src.calls != 2 {
		t.Fatalf("stale table should be served when the provider fails: %v %v calls=%d", got, err, src.calls)
	}
	if !strings.Contains(logs.String(), `"msg":"rates.refresh.failed"`) || !strings.Contains(logs.String(), "provider down") {
		t.Fatalf("a failed refresh should be logged, got %q", logs.String())
	}
	if err := s.Refresh(ctx); err == nil {
		t.Fatal("Refresh should report the provider error")
	}

	off := NewService(&memRepo{}, nil, c, slog.New(slog.DiscardHandler))
	if off.Enabled() {
		t.Fatal("nil source should disable rates")
	}
	if got, err := off.Latest(ctx); got != nil || err != nil {
		t.Fatalf("disabled Latest = %v %v", got, err)
	}
}
