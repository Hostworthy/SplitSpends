package expense

import (
	"bytes"
	"fmt"
	"math/big"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/money"
	"github.com/google/uuid"
)

// MaxTotal caps a single expense (about 10 billion major units in 2-decimal
// currencies) so sums never approach int64 overflow.
const MaxTotal int64 = 1_000_000_000_000

// MaxPeople caps the payers, the splits and the people on one item, and
// MaxItems the items, so the work one expense costs stays small. Groups hold
// far fewer people (group.MaxMembers).
const (
	MaxPeople = 200
	MaxItems  = 200
)

// Payer says how much one user paid.
type Payer struct {
	UserID uuid.UUID
	Amount int64
}

// SplitInput is one participant's split input. Value is a decimal string whose
// meaning depends on the method; it is empty for equal splits.
type SplitInput struct {
	UserID uuid.UUID
	Value  string
}

// Input is everything needed to create or replace an expense.
type Input struct {
	// ID lets offline clients choose the id so retries are idempotent.
	ID          *uuid.UUID
	GroupID     *uuid.UUID
	Description string
	Currency    string
	Total       int64
	Date        string
	Category    string
	Notes       string
	Method      Method
	Payers      []Payer
	Splits      []SplitInput
	Items       []Item
	Tax         int64
	Tip         int64
	ReceiptID   *uuid.UUID
	Recurrence  Recurrence
	// UpdatedAt, on an update, is the version of the expense the change was
	// made to. The update fails when the expense has changed since; nil skips
	// the check.
	UpdatedAt *time.Time
}

// FirstDay is the earliest date an expense can have. Dates also stop a year
// after today, so a typo cannot start a series that runs for centuries.
var FirstDay = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

// Normalized is a validated Input with computed shares.
type Normalized struct {
	Input
	Date   time.Time
	Shares []Share
}

// Normalize validates in and computes every participant's owed amount. An
// empty date means today and an empty method means equal. The Dart ledger
// ports this function, so every rule on the input belongs here.
func Normalize(in Input, today time.Time) (*Normalized, error) {
	in.Description = strings.Join(strings.Fields(in.Description), " ")
	if in.Description == "" {
		return nil, errs.Validation("description", "Add a description.")
	}
	if utf8.RuneCountInString(in.Description) > 120 {
		return nil, errs.Validation("description", "Keep the description under 120 characters.")
	}
	in.Notes = strings.TrimSpace(in.Notes)
	if utf8.RuneCountInString(in.Notes) > 2000 {
		return nil, errs.Validation("notes", "Notes are limited to 2000 characters.")
	}
	cur, ok := money.Lookup(in.Currency)
	if !ok {
		return nil, errs.Validation("currency", "Unsupported currency.")
	}
	in.Currency = cur.Code
	if in.Total <= 0 {
		return nil, errs.Validation("total", "The amount must be greater than zero.")
	}
	if in.Total > MaxTotal {
		return nil, errs.Validation("total", "The amount is too large.")
	}
	if in.Category == "" {
		in.Category = DefaultCategory
	}
	if !KnownCategory(in.Category) && !CustomCategory(in.Category) {
		return nil, errs.Validation("category", "Unknown category.")
	}
	if in.Category == PaymentCategory {
		return nil, errs.Validation("category", "That category is only for payments. Choose another one.")
	}
	if in.Method == "" {
		in.Method = MethodEqual
	}
	if in.Recurrence == "" {
		in.Recurrence = RecurNone
	}
	if !in.Recurrence.Valid() {
		return nil, errs.Validation("recurrence", "Unknown recurrence.")
	}
	date := today
	if in.Date != "" {
		d, err := time.Parse(time.DateOnly, in.Date)
		if err != nil {
			return nil, errs.Validation("date", "Use a YYYY-MM-DD date.")
		}
		date = d
	}
	if date.Before(FirstDay) || date.After(today.AddDate(1, 0, 0)) {
		return nil, errs.Validation("date", "Choose a date from 2000 up to a year from today.")
	}

	paid, err := payers(in.Payers, in.Total)
	if err != nil {
		return nil, err
	}
	owed, values, err := split(in)
	if err != nil {
		return nil, err
	}
	if in.Method != MethodItemized {
		in.Items, in.Tax, in.Tip = nil, 0, 0
	}
	return &Normalized{Input: in, Date: date, Shares: merge(paid, owed, values)}, nil
}

func payers(list []Payer, total int64) (map[uuid.UUID]int64, error) {
	if len(list) == 0 {
		return nil, errs.Validation("payers", "Choose who paid.")
	}
	if len(list) > MaxPeople {
		return nil, errs.Validation("payers", fmt.Sprintf("At most %d people can pay for one expense.", MaxPeople))
	}
	out := make(map[uuid.UUID]int64, len(list))
	var sum int64
	for _, p := range list {
		if p.UserID == uuid.Nil {
			return nil, errs.Validation("payers", "Every payer needs a user.")
		}
		if _, dup := out[p.UserID]; dup {
			return nil, errs.Validation("payers", "A person is listed as payer twice.")
		}
		if p.Amount < 0 || p.Amount > MaxTotal {
			return nil, errs.Validation("payers", "Paid amounts must be positive.")
		}
		out[p.UserID] = p.Amount
		sum += p.Amount
	}
	if sum != total {
		return nil, errs.Validation("payers", "Paid amounts must add up to the total.")
	}
	return out, nil
}

func split(in Input) (owed map[uuid.UUID]int64, values map[uuid.UUID]string, err error) {
	if in.Method == MethodItemized {
		return itemized(in)
	}
	if len(in.Splits) == 0 {
		return nil, nil, errs.Validation("splits", "Choose who this expense is split between.")
	}
	if len(in.Splits) > MaxPeople {
		return nil, nil, errs.Validation("splits", fmt.Sprintf("An expense can be split between at most %d people.", MaxPeople))
	}
	ids := make([]uuid.UUID, len(in.Splits))
	// raw holds the trimmed values: what is stored is what was parsed.
	raw := make([]string, len(in.Splits))
	seen := make(map[uuid.UUID]bool, len(in.Splits))
	for i, s := range in.Splits {
		if s.UserID == uuid.Nil || seen[s.UserID] {
			return nil, nil, errs.Validation("splits", "Each person can appear in the split once.")
		}
		seen[s.UserID] = true
		ids[i] = s.UserID
		raw[i] = strings.TrimSpace(s.Value)
	}

	var parts []int64
	values = make(map[uuid.UUID]string, len(in.Splits))
	switch in.Method {
	case MethodEqual, "":
		parts = money.Even(in.Total, len(ids))
	case MethodExact:
		parts = make([]int64, len(ids))
		var sum int64
		for i, s := range in.Splits {
			v, err := integer(raw[i], "splits")
			if err != nil {
				return nil, nil, err
			}
			if v < 0 {
				return nil, nil, errs.Validation("splits", "Amounts cannot be negative.")
			}
			parts[i] = v
			sum += v
			values[s.UserID] = raw[i]
		}
		if sum != in.Total {
			return nil, nil, errs.Validation("splits", "Amounts must add up to the total.")
		}
	case MethodPercent, MethodShares:
		weights := make([]*big.Rat, len(ids))
		sum := new(big.Rat)
		for i, s := range in.Splits {
			w, ok := money.Decimal(raw[i])
			if !ok || w.Sign() < 0 {
				return nil, nil, errs.Validation("splits", "Use positive numbers.")
			}
			weights[i] = w
			sum.Add(sum, w)
			values[s.UserID] = raw[i]
		}
		if in.Method == MethodPercent && sum.Cmp(big.NewRat(100, 1)) != 0 {
			return nil, nil, errs.Validation("splits", "Percentages must add up to 100.")
		}
		if sum.Sign() == 0 {
			return nil, nil, errs.Validation("splits", "At least one share must be above zero.")
		}
		parts, _ = money.Allocate(in.Total, weights)
	case MethodAdjustment:
		adjust := make([]int64, len(ids))
		var sum int64
		for i, s := range in.Splits {
			v := int64(0)
			if raw[i] != "" {
				if v, err = integer(raw[i], "splits"); err != nil {
					return nil, nil, err
				}
			}
			adjust[i] = v
			sum += v
			values[s.UserID] = raw[i]
		}
		even := money.Even(in.Total-sum, len(ids))
		parts = make([]int64, len(ids))
		for i := range ids {
			parts[i] = adjust[i] + even[i]
			if parts[i] < 0 {
				return nil, nil, errs.Validation("splits", "Adjustments leave someone owing a negative amount.")
			}
		}
	default:
		return nil, nil, errs.Validation("splitMethod", "Unknown split method.")
	}

	owed = make(map[uuid.UUID]int64, len(ids))
	for i, id := range ids {
		owed[id] = parts[i]
	}
	return owed, values, nil
}

// itemized shares each item evenly among its users, then spreads tax and tip
// in proportion to each user's item subtotal.
func itemized(in Input) (map[uuid.UUID]int64, map[uuid.UUID]string, error) {
	if len(in.Items) == 0 {
		return nil, nil, errs.Validation("items", "Add at least one item.")
	}
	if len(in.Items) > MaxItems {
		return nil, nil, errs.Validation("items", fmt.Sprintf("An expense can have at most %d items.", MaxItems))
	}
	if in.Tax < 0 || in.Tip < 0 {
		return nil, nil, errs.Validation("tax", "Tax and tip cannot be negative.")
	}
	// Bounding each term keeps items plus tax and tip far from overflow.
	if in.Tax > MaxTotal || in.Tip > MaxTotal {
		return nil, nil, errs.Validation("tax", "Tax and tip are too large.")
	}
	subtotals := map[uuid.UUID]int64{}
	order := []uuid.UUID{}
	var itemsSum int64
	for i := range in.Items {
		item := &in.Items[i]
		item.Name = strings.Join(strings.Fields(item.Name), " ")
		if item.Name == "" {
			item.Name = "Item"
		}
		if utf8.RuneCountInString(item.Name) > 120 {
			return nil, nil, errs.Validation("items", "Item names are limited to 120 characters.")
		}
		if item.Amount <= 0 || item.Amount > MaxTotal {
			return nil, nil, errs.Validation("items", "Item amounts must be greater than zero.")
		}
		if len(item.UserIDs) == 0 {
			return nil, nil, errs.Validation("items", "Assign every item to at least one person.")
		}
		if len(item.UserIDs) > MaxPeople {
			return nil, nil, errs.Validation("items", fmt.Sprintf("An item can be shared by at most %d people.", MaxPeople))
		}
		seen := map[uuid.UUID]bool{}
		for _, id := range item.UserIDs {
			if id == uuid.Nil || seen[id] {
				return nil, nil, errs.Validation("items", "Each person can be assigned to an item once.")
			}
			seen[id] = true
		}
		parts := money.Even(item.Amount, len(item.UserIDs))
		for j, id := range item.UserIDs {
			if _, ok := subtotals[id]; !ok {
				order = append(order, id)
			}
			subtotals[id] += parts[j]
		}
		itemsSum += item.Amount
	}
	if itemsSum+in.Tax+in.Tip != in.Total {
		return nil, nil, errs.Validation("total", "Items plus tax and tip must equal the total.")
	}
	weights := make([]*big.Rat, len(order))
	for i, id := range order {
		weights[i] = big.NewRat(subtotals[id], 1)
	}
	extra, err := money.Allocate(in.Tax+in.Tip, weights)
	if err != nil {
		return nil, nil, errs.Validation("items", "Items must have a positive total.")
	}
	owed := make(map[uuid.UUID]int64, len(order))
	for i, id := range order {
		owed[id] = subtotals[id] + extra[i]
	}
	return owed, map[uuid.UUID]string{}, nil
}

func merge(paid, owed map[uuid.UUID]int64, values map[uuid.UUID]string) []Share {
	order := make([]uuid.UUID, 0, len(paid)+len(owed))
	seen := map[uuid.UUID]bool{}
	add := func(m map[uuid.UUID]int64) {
		for id := range m {
			if !seen[id] {
				seen[id] = true
				order = append(order, id)
			}
		}
	}
	add(paid)
	add(owed)
	// Byte order equals the order of the lowercase hex strings, which is how
	// the Dart ledger sorts.
	slices.SortFunc(order, func(a, b uuid.UUID) int { return bytes.Compare(a[:], b[:]) })
	shares := make([]Share, 0, len(order))
	for _, id := range order {
		shares = append(shares, Share{UserID: id, Paid: paid[id], Owed: owed[id], Value: values[id]})
	}
	return shares
}

func integer(s, field string) (int64, error) {
	r, ok := money.Decimal(s)
	if !ok || !r.IsInt() || !r.Num().IsInt64() {
		return 0, errs.Validation(field, "Amounts must be whole numbers of minor units.")
	}
	v := r.Num().Int64()
	if v > MaxTotal || v < -MaxTotal {
		return 0, errs.Validation(field, "Amount is too large.")
	}
	return v, nil
}

// decimalText is a plain decimal number: no exponent, fraction or base prefix.
var decimalText = regexp.MustCompile(`^[+-]?(\d+(\.\d*)?|\.\d+)$`)

// ParseDecimal converts a decimal string such as "12.5" or "1,234.50" to minor
// units for a currency exponent, rounding half away from zero. It reads AI
// answers, so it accepts only plain decimals ("1e3" is not a thousand) and
// rejects amounts beyond MaxTotal, which keeps sums of them far from
// overflowing.
func ParseDecimal(s string, exponent int) (int64, bool) {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", ""))
	if !decimalText.MatchString(s) {
		return 0, false
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return 0, false
	}
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(exponent)), nil)
	r.Mul(r, new(big.Rat).SetInt(scale))
	num, den := r.Num(), r.Denom()
	q, m := new(big.Int).QuoRem(num, den, new(big.Int))
	if new(big.Int).Mul(new(big.Int).Abs(m), big.NewInt(2)).Cmp(den) >= 0 {
		if num.Sign() < 0 {
			q.Sub(q, big.NewInt(1))
		} else {
			q.Add(q, big.NewInt(1))
		}
	}
	if !q.IsInt64() {
		return 0, false
	}
	v := q.Int64()
	if v > MaxTotal || v < -MaxTotal {
		return 0, false
	}
	return v, true
}
