package expense

import (
	"math"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/google/uuid"
)

var (
	ada  = uuid.MustParse("00000000-0000-0000-0000-00000000000a")
	ben  = uuid.MustParse("00000000-0000-0000-0000-00000000000b")
	cleo = uuid.MustParse("00000000-0000-0000-0000-00000000000c")
	day  = time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)
)

func base(method Method, splits ...SplitInput) Input {
	return Input{Description: "Dinner", Currency: "usd", Total: 10000, Method: method,
		Payers: []Payer{{ada, 10000}}, Splits: splits}
}

func owed(t *testing.T, n *Normalized) map[uuid.UUID]int64 {
	t.Helper()
	out := map[uuid.UUID]int64{}
	var paid, sum int64
	for _, s := range n.Shares {
		out[s.UserID] = s.Owed
		sum += s.Owed
		paid += s.Paid
	}
	if sum != n.Total || paid != n.Total {
		t.Fatalf("owed %d paid %d total %d", sum, paid, n.Total)
	}
	return out
}

func TestEqualSplit(t *testing.T) {
	n, err := Normalize(base(MethodEqual, SplitInput{UserID: ada}, SplitInput{UserID: ben}, SplitInput{UserID: cleo}), day)
	if err != nil {
		t.Fatal(err)
	}
	o := owed(t, n)
	if o[ada] != 3334 || o[ben] != 3333 || o[cleo] != 3333 {
		t.Fatalf("got %v", o)
	}
	if n.Currency != "USD" || n.Category != DefaultCategory || !n.Date.Equal(day) {
		t.Fatalf("normalization: %+v", n.Input)
	}
}

func TestPercentSplit(t *testing.T) {
	n, err := Normalize(base(MethodPercent, SplitInput{ada, "50"}, SplitInput{ben, "25.5"}, SplitInput{cleo, "24.5"}), day)
	if err != nil {
		t.Fatal(err)
	}
	o := owed(t, n)
	if o[ada] != 5000 || o[ben] != 2550 || o[cleo] != 2450 {
		t.Fatalf("got %v", o)
	}
	if _, err := Normalize(base(MethodPercent, SplitInput{ada, "50"}, SplitInput{ben, "40"}), day); err == nil {
		t.Fatal("percentages not summing to 100 must fail")
	}
}

func TestSharesSplit(t *testing.T) {
	n, err := Normalize(base(MethodShares, SplitInput{ada, "2"}, SplitInput{ben, "1"}, SplitInput{cleo, "1"}), day)
	if err != nil {
		t.Fatal(err)
	}
	o := owed(t, n)
	if o[ada] != 5000 || o[ben] != 2500 {
		t.Fatalf("got %v", o)
	}
}

func TestExactSplit(t *testing.T) {
	if _, err := Normalize(base(MethodExact, SplitInput{ada, "6000"}, SplitInput{ben, "3000"}), day); err == nil {
		t.Fatal("exact amounts below total must fail")
	}
	n, err := Normalize(base(MethodExact, SplitInput{ada, "7000"}, SplitInput{ben, "3000"}), day)
	if err != nil {
		t.Fatal(err)
	}
	if o := owed(t, n); o[ben] != 3000 {
		t.Fatalf("got %v", o)
	}
}

func TestAdjustmentSplit(t *testing.T) {
	n, err := Normalize(base(MethodAdjustment, SplitInput{ada, "1000"}, SplitInput{ben, ""}, SplitInput{cleo, "-500"}), day)
	if err != nil {
		t.Fatal(err)
	}
	o := owed(t, n)
	// rest = 10000 - 500 = 9500 split evenly = 3167, 3167, 3166
	if o[ada] != 4167 || o[ben] != 3167 || o[cleo] != 2666 {
		t.Fatalf("got %v", o)
	}
}

func TestItemizedSplit(t *testing.T) {
	in := Input{Description: "Lunch", Currency: "EUR", Total: 3300, Method: MethodItemized,
		Payers: []Payer{{ben, 3300}},
		Items: []Item{
			{Name: "Pizza", Amount: 1200, UserIDs: []uuid.UUID{ada}},
			{Name: "Pasta", Amount: 1000, UserIDs: []uuid.UUID{ben}},
			{Name: "Wine", Amount: 800, UserIDs: []uuid.UUID{ada, ben}},
		},
		Tax: 100, Tip: 200,
	}
	n, err := Normalize(in, day)
	if err != nil {
		t.Fatal(err)
	}
	o := owed(t, n)
	// ada items 1600, ben 1400; extra 300 split 160/140
	if o[ada] != 1760 || o[ben] != 1540 {
		t.Fatalf("got %v", o)
	}
	in.Tip = 0
	if _, err := Normalize(in, day); err == nil {
		t.Fatal("total mismatch must fail")
	}
}

func TestMultiplePayers(t *testing.T) {
	in := base(MethodEqual, SplitInput{UserID: ada}, SplitInput{UserID: ben})
	in.Payers = []Payer{{ada, 4000}, {cleo, 6000}}
	n, err := Normalize(in, day)
	if err != nil {
		t.Fatal(err)
	}
	if len(n.Shares) != 3 {
		t.Fatalf("want 3 participants, got %d", len(n.Shares))
	}
	in.Payers = []Payer{{ada, 4000}}
	if _, err := Normalize(in, day); err == nil {
		t.Fatal("payer total mismatch must fail")
	}
}

func TestDateRange(t *testing.T) {
	cases := []struct {
		date string
		ok   bool
	}{
		{"0001-01-01", false},
		{"1999-12-31", false},
		{"2000-01-01", true},
		{"2027-11-24", true},
		{"2027-11-25", false},
		{"9999-12-31", false},
	}
	for _, c := range cases {
		in := base(MethodEqual, SplitInput{UserID: ada})
		in.Date = c.date
		_, err := Normalize(in, day)
		if c.ok && err != nil {
			t.Errorf("%s: %v", c.date, err)
		}
		if !c.ok {
			if de, ok := errs.As(err); !ok || de.Field != "date" {
				t.Errorf("%s: want an error on date, got %v", c.date, err)
			}
		}
	}
}

func TestParseDecimal(t *testing.T) {
	cases := []struct {
		in   string
		exp  int
		want int64
	}{{"12.50", 2, 1250}, {"0.005", 2, 1}, {"1,234.5", 2, 123450}, {"1500", 0, 1500}, {"2.0004", 3, 2000}, {"-1.255", 2, -126}, {"10000000000.00", 2, MaxTotal}}
	for _, c := range cases {
		got, ok := ParseDecimal(c.in, c.exp)
		if !ok || got != c.want {
			t.Errorf("ParseDecimal(%q,%d) = %d,%v want %d", c.in, c.exp, got, ok, c.want)
		}
	}
	for _, in := range []string{"", "1e3", "1/3", "0x10", "abc", "1.2.3", "10000000000.01", "-10000000000.01"} {
		if got, ok := ParseDecimal(in, 2); ok {
			t.Errorf("ParseDecimal(%q,2) = %d, want rejected", in, got)
		}
	}
}

// people returns n distinct user ids.
func people(n int) []uuid.UUID {
	ids := make([]uuid.UUID, n)
	for i := range ids {
		ids[i] = uuid.UUID{15: byte(i), 14: byte(i >> 8), 0: 1}
	}
	return ids
}

func TestNormalizeRejectsTooManyParticipants(t *testing.T) {
	many := people(MaxPeople + 1)
	cases := []struct {
		name  string
		field string
		edit  func(in *Input)
	}{
		{"payers", "payers", func(in *Input) {
			in.Payers = []Payer{{ada, in.Total}}
			for _, id := range many[1:] {
				in.Payers = append(in.Payers, Payer{UserID: id})
			}
		}},
		{"splits", "splits", func(in *Input) {
			in.Splits = nil
			for _, id := range many {
				in.Splits = append(in.Splits, SplitInput{UserID: id})
			}
		}},
		{"items", "items", func(in *Input) {
			in.Method, in.Splits, in.Total = MethodItemized, nil, MaxItems+1
			in.Payers = []Payer{{ada, in.Total}}
			for range MaxItems + 1 {
				in.Items = append(in.Items, Item{Name: "Tea", Amount: 1, UserIDs: []uuid.UUID{ada}})
			}
		}},
		{"item users", "items", func(in *Input) {
			in.Method, in.Splits = MethodItemized, nil
			in.Items = []Item{{Name: "Cake", Amount: in.Total, UserIDs: many}}
		}},
	}
	for _, c := range cases {
		in := base(MethodEqual, SplitInput{UserID: ada})
		c.edit(&in)
		_, err := Normalize(in, day)
		if de, ok := errs.As(err); !ok || de.Field != c.field {
			t.Errorf("%s: want an error on %s, got %v", c.name, c.field, err)
		}
	}
	in := base(MethodEqual)
	for _, id := range many[:MaxPeople] {
		in.Splits = append(in.Splits, SplitInput{UserID: id})
	}
	n, err := Normalize(in, day)
	if err != nil {
		t.Fatalf("%d people must be allowed: %v", MaxPeople, err)
	}
	for i := 1; i < len(n.Shares); i++ {
		if n.Shares[i-1].UserID.String() >= n.Shares[i].UserID.String() {
			t.Fatalf("shares are not ordered by id at %d", i)
		}
	}
}

func BenchmarkMerge(b *testing.B) {
	ids := people(MaxPeople)
	paid := map[uuid.UUID]int64{ids[0]: 1}
	owed := make(map[uuid.UUID]int64, len(ids))
	for _, id := range ids {
		owed[id] = 1
	}
	for b.Loop() {
		merge(paid, owed, nil)
	}
}

func TestItemizedRejectsHugeTax(t *testing.T) {
	cases := []struct {
		name     string
		tax, tip int64
		item     int64
	}{
		{"tax above max", MaxTotal + 1, 0, 100},
		{"tip above max", 0, MaxTotal + 1, 100},
		{"sum wraps", math.MaxInt64, math.MaxInt64, 102},
	}
	for _, c := range cases {
		in := Input{Description: "Lunch", Currency: "USD", Total: 100, Method: MethodItemized,
			Payers: []Payer{{ada, 100}},
			Items:  []Item{{Name: "Soup", Amount: c.item, UserIDs: []uuid.UUID{ada, ben}}},
			Tax:    c.tax, Tip: c.tip,
		}
		_, err := Normalize(in, day)
		if de, ok := errs.As(err); !ok || de.Field != "tax" {
			t.Errorf("%s: want an error on tax, got %v", c.name, err)
		}
	}
}
