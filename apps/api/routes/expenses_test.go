package routes

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/bootstrap/bootstraptest"
	"github.com/Hostworthy/SplitSpends/packages/platform"
)

func TestUpdateVersion(t *testing.T) {
	s := newServer(t, bootstraptest.Options{})
	p := platform.Prefix
	ada := s.signIn(t, "ada@example.com", "Ada")
	ben := s.signIn(t, "ben@example.com", "Ben")
	g := s.group(t, ada.Token, "Flat", "ben@example.com")
	e := s.expense(t, ada.Token, g.ID, 3000, ada.User.ID, ada.User.ID, ben.User.ID)
	edit := func(total int64, at string) platform.ExpenseInput {
		return platform.ExpenseInput{Description: "Groceries", Currency: "EUR", Total: total, SplitMethod: "equal", UpdatedAt: &at,
			Payers: []platform.Payer{{UserID: ada.User.ID, Amount: total}},
			Splits: []platform.Split{{UserID: ada.User.ID}, {UserID: ben.User.ID}}}
	}
	path := p + "/expenses/" + e.ID

	s.Clock.Add(time.Minute)
	saved := data[platform.Expense](t, s.call(t, http.MethodPut, path, ben.Token, edit(4000, e.UpdatedAt)).want(t, http.StatusOK))
	if saved.UpdatedAt == e.UpdatedAt {
		t.Fatal("an edit must move updatedAt")
	}
	s.Clock.Add(time.Minute)
	if f := s.call(t, http.MethodPut, path, ada.Token, edit(5000, e.UpdatedAt)).want(t, http.StatusConflict).failure(t); f.Code != "CONFLICT" {
		t.Fatalf("want CONFLICT, got %+v", f)
	}
	if f := s.call(t, http.MethodPut, path, ada.Token, edit(5000, "yesterday")).want(t, http.StatusUnprocessableEntity).failure(t); f.Field != "updatedAt" {
		t.Fatalf("want an error on updatedAt, got %+v", f)
	}
	s.call(t, http.MethodPut, path, ada.Token, edit(5000, saved.UpdatedAt)).want(t, http.StatusOK)
}

func TestSafeCell(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"", ""},
		{"Ada", "Ada"},
		{"=1+1", "'=1+1"},
		{"+1", "'+1"},
		{"-1", "'-1"},
		{"@SUM(A1)", "'@SUM(A1)"},
		{"\tTab", "'\tTab"},
		{"\rReturn", "'\rReturn"},
		{"Ada = Ben", "Ada = Ben"},
	} {
		if got := safeCell(c.in); got != c.want {
			t.Errorf("safeCell(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestDecimalString(t *testing.T) {
	for _, c := range []struct {
		amount   int64
		exponent int
		want     string
	}{
		{0, 2, "0.00"},
		{5, 2, "0.05"},
		{-100, 2, "-1.00"},
		{123456, 2, "1234.56"},
		{-1500, 0, "-1500"},
		{1, 3, "0.001"},
	} {
		if got := decimalString(c.amount, c.exponent); got != c.want {
			t.Errorf("decimalString(%d, %d) = %q, want %q", c.amount, c.exponent, got, c.want)
		}
	}
}

func TestNumber(t *testing.T) {
	for _, c := range []struct {
		value string
		want  string
	}{
		{"60", "60"},
		{"33.3333", "33.3333"},
		{"2.50", "2.50"},
		{".5", "0.5"},
		{"1.", "1"},
		{"+2", "2"},
		{"05", "5"},
		{"-0", "0"},
	} {
		got := number(c.value)
		if got == nil || got.String() != c.want {
			t.Errorf("number(%q) = %v, want %q", c.value, got, c.want)
			continue
		}
		if _, err := json.Marshal(got); err != nil {
			t.Errorf("number(%q) is not valid JSON: %v", c.value, err)
		}
	}
	if got := number(""); got != nil {
		t.Errorf("number(\"\") = %v, want nil", *got)
	}
}

func TestExportNeutralisesNames(t *testing.T) {
	s := newServer(t, bootstraptest.Options{})
	ada := s.signIn(t, "ada@example.com", "Ada")
	ben := s.signIn(t, "ben@example.com", "=1+1")
	g := s.group(t, ada.Token, "Flat", "ben@example.com")
	s.expense(t, ada.Token, g.ID, 200, ada.User.ID, ada.User.ID, ben.User.ID)

	res := s.call(t, http.MethodGet, platform.Prefix+"/groups/"+g.ID+"/export", ada.Token, nil).want(t, http.StatusOK)
	rows, err := csv.NewReader(bytes.NewReader(res.body)).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("want a header and one row, got %q", rows)
	}
	column := slices.Index(rows[0], "'=1+1")
	if column < 0 {
		t.Fatalf("header %q does not carry the neutralised name", rows[0])
	}
	if got := rows[1][column]; got != "-1.00" {
		t.Errorf("Ben's net is %q, want -1.00", got)
	}
}
