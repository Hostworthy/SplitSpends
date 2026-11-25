package routes

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/bootstrap/bootstraptest"
	"github.com/Hostworthy/SplitSpends/packages/platform"
	"github.com/google/uuid"
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

func TestDecimal(t *testing.T) {
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
		if got := decimal(c.value); got == nil || *got != c.want {
			t.Errorf("decimal(%q) = %v, want %q", c.value, got, c.want)
		}
	}
	if got := decimal(""); got != nil {
		t.Errorf("decimal(\"\") = %q, want nil", *got)
	}
}

// TestShareValuesAreStrings checks that split values go out as decimal
// strings, as they come in, so clients never read them as floats.
func TestShareValuesAreStrings(t *testing.T) {
	s := newServer(t, bootstraptest.Options{})
	ada := s.signIn(t, "ada@example.com", "Ada")
	ben := s.signIn(t, "ben@example.com", "Ben")
	g := s.group(t, ada.Token, "Flat", "ben@example.com")
	third, rest := json.Number("33.33"), json.Number("66.67")
	in := platform.ExpenseInput{GroupID: &g.ID, Description: "Rent", Currency: "EUR", Total: 9000, SplitMethod: "percent",
		Payers: []platform.Payer{{UserID: ada.User.ID, Amount: 9000}},
		Splits: []platform.Split{{UserID: ada.User.ID, Value: &third}, {UserID: ben.User.ID, Value: &rest}}}
	res := s.call(t, http.MethodPost, platform.Prefix+"/expenses", ada.Token, in).want(t, http.StatusCreated)
	for _, want := range []string{`"value":"33.33"`, `"value":"66.67"`} {
		if !strings.Contains(string(res.body), want) {
			t.Errorf("want %s in %s", want, res.body)
		}
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

// TestExportIsReproducible checks that former members get their columns in
// a fixed order, after the members and by name, so two exports of the same
// group are the same file.
func TestExportIsReproducible(t *testing.T) {
	s := newServer(t, bootstraptest.Options{})
	ada := s.signIn(t, "ada@example.com", "Ada")
	former := []platform.LoginResult{
		s.signIn(t, "zed@example.com", "Zed"),
		s.signIn(t, "cy@example.com", "Cy"),
		s.signIn(t, "max@example.com", "Max"),
	}
	g := s.group(t, ada.Token, "Flat", "zed@example.com", "cy@example.com", "max@example.com")
	s.expense(t, ada.Token, g.ID, 400, ada.User.ID, ada.User.ID)
	for _, p := range former {
		s.expense(t, p.Token, g.ID, 100, p.User.ID, p.User.ID)
		s.call(t, http.MethodDelete, platform.Prefix+"/groups/"+g.ID+"/members/"+p.User.ID, p.Token, nil).want(t, http.StatusNoContent)
	}

	path := platform.Prefix + "/groups/" + g.ID + "/export"
	first := s.call(t, http.MethodGet, path, ada.Token, nil).want(t, http.StatusOK).body
	rows, err := csv.NewReader(bytes.NewReader(first)).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(rows[0], ","), "Date,Description,Category,Kind,Currency,Cost,Ada,Cy,Max,Zed"; got != want {
		t.Fatalf("header %q, want %q", got, want)
	}
	for range 5 {
		again := s.call(t, http.MethodGet, path, ada.Token, nil).want(t, http.StatusOK).body
		if !bytes.Equal(again, first) {
			t.Fatalf("a second export differs:\n%s\nfirst:\n%s", again, first)
		}
	}
}

// TestMalformedIDsNameTheirField checks that an id that does not parse is
// refused on the exact field that holds it, wherever it is sent.
func TestMalformedIDsNameTheirField(t *testing.T) {
	s := newServer(t, bootstraptest.Options{})
	p := platform.Prefix
	ada := s.signIn(t, "ada@example.com", "Ada")
	me := ada.User.ID
	expense := func(edit func(*platform.ExpenseInput)) platform.ExpenseInput {
		in := platform.ExpenseInput{Description: "Groceries", Currency: "EUR", Total: 3000, SplitMethod: "equal",
			Payers: []platform.Payer{{UserID: me, Amount: 3000}}, Splits: []platform.Split{{UserID: me}, {UserID: me}}}
		edit(&in)
		return in
	}
	local := uuid.NewString()
	for _, tc := range []struct {
		name, path string
		body       any
		field      string
	}{
		{"a split", p + "/expenses", expense(func(in *platform.ExpenseInput) { in.Splits[1].UserID = "x" }), "splits[1].userId"},
		{"a payer", p + "/expenses", expense(func(in *platform.ExpenseInput) { in.Payers[0].UserID = "x" }), "payers[0].userId"},
		{"an item", p + "/expenses", expense(func(in *platform.ExpenseInput) {
			in.SplitMethod, in.Splits = "itemized", nil
			in.Items = []platform.Item{{Name: "Bread", Amount: 3000, UserIDs: []string{me, "x"}}}
		}), "items[0].userIds[1]"},
		{"a reminder", p + "/reminders", platform.ReminderInput{UserID: "x"}, "userId"},
		{"an imported person", p + "/import", platform.ImportRequest{MeID: local,
			People: []platform.ImportPerson{{ID: local, Name: "Ada"}, {ID: "x", Name: "Ben"}}}, "people[1].id"},
		{"an imported expense", p + "/import", platform.ImportRequest{MeID: local,
			People:   []platform.ImportPerson{{ID: local, Name: "Ada"}},
			Expenses: []platform.ImportExpense{{ExpenseInput: expense(func(in *platform.ExpenseInput) { in.ID = &local; in.Splits[0].UserID = "x" })}}},
			"expenses[0].splits[0].userId"},
	} {
		f := s.call(t, http.MethodPost, tc.path, ada.Token, tc.body).want(t, http.StatusUnprocessableEntity).failure(t)
		if f.Field != tc.field || f.Error != "Invalid id." {
			t.Errorf("%s: want %q on %s, got %+v", tc.name, "Invalid id.", tc.field, f)
		}
	}
}

// TestImportRefusesUnknownKind checks that an imported entry that is neither
// an expense nor a payment is refused instead of saved as an expense.
func TestImportRefusesUnknownKind(t *testing.T) {
	s := newServer(t, bootstraptest.Options{})
	ada := s.signIn(t, "ada@example.com", "Ada")
	local, id := uuid.NewString(), uuid.NewString()
	bundle := platform.ImportRequest{MeID: local, People: []platform.ImportPerson{{ID: local, Name: "Ada"}},
		Expenses: []platform.ImportExpense{{Kind: "paymnet", ExpenseInput: platform.ExpenseInput{ID: &id, Description: "Rent",
			Currency: "EUR", Total: 1000, SplitMethod: "equal", Payers: []platform.Payer{{UserID: local, Amount: 1000}},
			Splits: []platform.Split{{UserID: local}}}}}}
	f := s.call(t, http.MethodPost, platform.Prefix+"/import", ada.Token, bundle).want(t, http.StatusUnprocessableEntity).failure(t)
	if f.Field != "expenses[0].kind" {
		t.Fatalf("want an error on expenses[0].kind, got %+v", f)
	}
}
