package assist

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/expense"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/group"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/rates"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/user"
	"github.com/Hostworthy/SplitSpends/packages/core/ports"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/google/uuid"
)

var (
	ada   = uuid.MustParse("00000000-0000-0000-0000-00000000000a")
	ben   = uuid.MustParse("00000000-0000-0000-0000-00000000000b")
	cleo  = uuid.MustParse("00000000-0000-0000-0000-00000000000c")
	trip  = uuid.MustParse("00000000-0000-0000-0000-0000000000f1")
	other = uuid.MustParse("00000000-0000-0000-0000-0000000000ff")
)

func noRates() *rates.Table { return nil }

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func world() *known {
	return &known{
		me:      &user.User{ID: ada, Name: "Ada", DefaultCurrency: "USD"},
		people:  map[uuid.UUID]*user.User{ben: {ID: ben, Name: "Ben"}, cleo: {ID: cleo, Name: "Cleo"}},
		groups:  map[uuid.UUID]*group.Group{trip: {ID: trip, Name: "Lisbon trip", DefaultCurrency: "EUR", Members: []uuid.UUID{ada, ben}}},
		friends: map[uuid.UUID]bool{ben: true, cleo: true},
	}
}

func TestItemizedDraftInGroupTheModelChose(t *testing.T) {
	d, err := build(world(), Request{}, ports.DraftExtract{
		Description: "Dinner", Amount: "56.40", GroupID: trip.String(), PayerID: ben.String(),
		ParticipantIDs: []string{ada.String(), ben.String(), cleo.String()},
		Items: []ports.DraftItem{
			{Name: "Pasta", Amount: "24.00", ParticipantIDs: []string{ben.String()}},
			{Name: "Wine", Amount: "26.00"},
		},
		Tax: "6.40", Category: "food.dining",
	}, "2026-10-06", "USD", noRates)
	must(t, err)

	if d.GroupID == nil || *d.GroupID != trip || d.Currency != "EUR" || d.Method != expense.MethodItemized {
		t.Fatalf("group, currency or method wrong: %+v", d)
	}
	if d.Total != 5640 || d.Tax != 640 || d.Payers[0].UserID != ben || d.Payers[0].Amount != 5640 {
		t.Fatalf("totals or payer wrong: %+v", d)
	}
	if len(d.Items[1].UserIDs) != 2 {
		t.Fatalf("an unassigned item is shared by every participant: %+v", d.Items[1])
	}
	if !strings.Contains(strings.Join(d.Warnings, " "), "Cleo is not in Lisbon trip") {
		t.Fatalf("a person outside the group must be dropped with a warning: %v", d.Warnings)
	}
	// The draft must be a valid expense as is.
	in := expense.Input{GroupID: d.GroupID, Description: d.Description, Currency: d.Currency, Total: d.Total, Method: d.Method,
		Date: d.Date, Payers: d.Payers, Items: d.Items, Tax: d.Tax, Tip: d.Tip}
	if _, err := expense.Normalize(in, time.Now()); err != nil {
		t.Fatalf("draft does not validate: %v", err)
	}
}

func TestInventedIDsAreIgnored(t *testing.T) {
	d, err := build(world(), Request{}, ports.DraftExtract{
		Amount: "30", GroupID: other.String(), PayerID: "not-a-uuid",
		ParticipantIDs: []string{uuid.NewString(), ben.String()}, Unmatched: []string{"Zoe"},
	}, "2026-10-06", "USD", noRates)
	must(t, err)
	if d.GroupID != nil {
		t.Fatal("a group the user is not in must be ignored")
	}
	if d.Payers[0].UserID != ada || len(d.Splits) != 1 || d.Splits[0].UserID != ben || d.Total != 3000 {
		t.Fatalf("payer defaults to me and only known friends stay: %+v", d)
	}
	if !strings.Contains(strings.Join(d.Warnings, " "), "Zoe") {
		t.Fatalf("unmatched names are reported: %v", d.Warnings)
	}
}

func TestReceiptTotalReconciles(t *testing.T) {
	d, err := build(world(), Request{GroupID: &trip}, ports.DraftExtract{
		Amount: "45.00",
		Items:  []ports.DraftItem{{Name: "A", Amount: "30.00"}, {Name: "B", Amount: "20.00"}},
	}, "2026-10-06", "USD", noRates)
	must(t, err)
	var items int64
	for _, it := range d.Items {
		items += it.Amount
	}
	if d.Total != 4500 || items+d.Tax+d.Tip != d.Total || d.Tax != 0 {
		t.Fatalf("a receipt discount must be spread over items: total %d items %d tax %d", d.Total, items, d.Tax)
	}
}

func eurTable() *rates.Table {
	t, _ := rates.New("USD", map[string]string{"EUR": "0.8", "JPY": "150"})
	t.Published = time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)
	return &t
}

func TestReceiptInAnotherCurrencyIsConverted(t *testing.T) {
	// A Tokyo receipt added to the euro group: amounts arrive in yen.
	d, err := build(world(), Request{GroupID: &trip}, ports.DraftExtract{
		Amount: "3300", Currency: "JPY",
		Items: []ports.DraftItem{{Name: "Ramen", Amount: "1800"}, {Name: "Gyoza", Amount: "1200"}},
		Tax:   "300",
	}, "2026-10-06", "USD", eurTable)
	must(t, err)
	// 1 JPY = 0.8/150 EUR: 1800 → 9.60, 1200 → 6.40, 300 → 1.60, total 17.60.
	if d.Currency != "EUR" || d.Total != 1760 || d.Items[0].Amount != 960 || d.Items[1].Amount != 640 || d.Tax != 160 {
		t.Fatalf("not converted to the group currency: %+v", d)
	}
	note := strings.Join(d.Warnings, " ")
	if !strings.Contains(note, "Converted from JPY to EUR at 1 JPY = 0.0053 EUR (rates of 24 Nov 2026)") || !strings.Contains(note, "3,300") {
		t.Fatalf("conversion must be explained: %v", d.Warnings)
	}
	in := expense.Input{GroupID: d.GroupID, Description: d.Description, Currency: d.Currency, Total: d.Total, Method: d.Method,
		Date: d.Date, Payers: d.Payers, Items: d.Items, Tax: d.Tax, Tip: d.Tip}
	if _, err := expense.Normalize(in, time.Now()); err != nil {
		t.Fatalf("converted draft does not validate: %v", err)
	}
}

func TestOwnCurrencyOutsideGroups(t *testing.T) {
	d, err := build(world(), Request{}, ports.DraftExtract{Amount: "10", Currency: "EUR", ParticipantIDs: []string{ben.String()}},
		"2026-10-06", "USD", eurTable)
	must(t, err)
	if d.Currency != "USD" || d.Total != 1250 {
		t.Fatalf("expenses outside groups use the user's currency: %+v", d)
	}
}

func TestNoRatesNoGuessing(t *testing.T) {
	_, err := build(world(), Request{GroupID: &trip}, ports.DraftExtract{Amount: "10", Currency: "USD"}, "2026-10-06", "USD", noRates)
	if !errs.Is(err, errs.KindNoRates) {
		t.Fatalf("without rates the draft must not silently keep another currency: %v", err)
	}
}

// TestInputCarriesNoEmails checks that the people sent to the AI provider are
// ids and names only.
func TestInputCarriesNoEmails(t *testing.T) {
	k := world()
	k.me.Email = "ada@example.com"
	k.people[ben].Email = "ben@example.com"
	k.people[cleo].Email = "cleo@example.com"
	in := k.input("dinner with Ben", "2026-10-06", "USD")
	if len(in.People) != 2 || in.Me.Name != "Ada" {
		t.Fatalf("people missing: %+v", in)
	}
	if sent := fmt.Sprintf("%+v", in); strings.Contains(sent, "@") {
		t.Fatalf("the draft input must not carry email addresses: %s", sent)
	}
}
