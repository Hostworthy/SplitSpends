// Package expense owns expenses, settle-up payments, splitting rules and
// comments. Payments are expenses with KindPayment so balances come from a
// single ledger.
package expense

import (
	"time"

	"github.com/google/uuid"
)

// Kind distinguishes shared costs from settle-up payments.
type Kind string

const (
	KindExpense Kind = "expense"
	KindPayment Kind = "payment"
)

// Method is how the total is divided between participants.
type Method string

const (
	MethodEqual      Method = "equal"
	MethodExact      Method = "exact"
	MethodPercent    Method = "percent"
	MethodShares     Method = "shares"
	MethodAdjustment Method = "adjustment"
	MethodItemized   Method = "itemized"
)

// Recurrence schedules automatic copies of an expense.
type Recurrence string

const (
	RecurNone     Recurrence = "none"
	RecurWeekly   Recurrence = "weekly"
	RecurBiweekly Recurrence = "biweekly"
	RecurMonthly  Recurrence = "monthly"
	RecurYearly   Recurrence = "yearly"
)

// Valid reports whether r is a known recurrence.
func (r Recurrence) Valid() bool {
	switch r {
	case RecurNone, RecurWeekly, RecurBiweekly, RecurMonthly, RecurYearly:
		return true
	}
	return false
}

// Next returns the next occurrence after date. Monthly and yearly series
// fall on day, the day of the month the series started on, or on the last
// day of a shorter month: a series from 31 January books 28 February and
// then 31 March, never skipping a month or drifting into the next one.
func (r Recurrence) Next(date time.Time, day int) time.Time {
	switch r {
	case RecurWeekly:
		return date.AddDate(0, 0, 7)
	case RecurBiweekly:
		return date.AddDate(0, 0, 14)
	case RecurMonthly:
		return onDay(date, date.Year(), date.Month()+1, day)
	case RecurYearly:
		return onDay(date, date.Year()+1, date.Month(), day)
	}
	return date
}

// onDay returns day of the given month at the clock time of date, or the
// month's last day when it is shorter. month may be 13, as time.Date allows.
func onDay(date time.Time, year int, month time.Month, day int) time.Time {
	first := time.Date(year, month, 1, date.Hour(), date.Minute(), date.Second(), date.Nanosecond(), date.Location())
	last := first.AddDate(0, 1, -1).Day()
	return first.AddDate(0, 0, min(max(day, 1), last)-1)
}

// CatchUp is how many missed occurrences a series books at most when it has
// fallen behind. Older ones are skipped, so a series that starts long ago
// cannot flood a group with copies.
const CatchUp = 12

// Missed returns the occurrences from next up to and including today (the
// latest CatchUp of them, oldest first) and the first occurrence after
// today, where the series continues. day is the series' day of the month,
// as for Next.
func (r Recurrence) Missed(next, today time.Time, day int) ([]time.Time, time.Time) {
	if r == RecurNone || !r.Valid() {
		return nil, next
	}
	var dates []time.Time
	for ; !next.After(today); next = r.Next(next, day) {
		if len(dates) == CatchUp {
			dates = dates[1:]
		}
		dates = append(dates, next)
	}
	return dates, next
}

// Share is one participant's position in an expense. Value keeps the split
// input (percent, shares, exact or adjustment) so the expense can be edited.
type Share struct {
	UserID uuid.UUID
	Paid   int64
	Owed   int64
	Value  string
}

// Item is a line of an itemized expense, shared evenly by UserIDs.
type Item struct {
	Name    string      `json:"name"`
	Amount  int64       `json:"amount"`
	UserIDs []uuid.UUID `json:"userIds"`
}

// Expense is a shared cost or a payment between two people.
type Expense struct {
	ID          uuid.UUID
	GroupID     *uuid.UUID
	Kind        Kind
	Description string
	Category    string
	Currency    string
	Total       int64
	Date        time.Time
	Notes       string
	Method      Method
	Shares      []Share
	Items       []Item
	Tax         int64
	Tip         int64
	ReceiptID   *uuid.UUID
	Recurrence  Recurrence
	NextDate    *time.Time
	CreatedBy   uuid.UUID
	UpdatedBy   uuid.UUID
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   *time.Time

	// AnchorDay is the day of the month a recurring series falls on, kept
	// so it returns to the 31st after booking 28 February. Zero when the
	// expense does not recur.
	AnchorDay int

	// CommentCount is a read-model field filled by list queries.
	CommentCount int
}

// Participants returns every user with a share.
func (e *Expense) Participants() []uuid.UUID {
	out := make([]uuid.UUID, 0, len(e.Shares))
	for _, s := range e.Shares {
		out = append(out, s.UserID)
	}
	return out
}

// Involves reports whether id has a share.
func (e *Expense) Involves(id uuid.UUID) bool {
	for _, s := range e.Shares {
		if s.UserID == id {
			return true
		}
	}
	return false
}

// Net returns paid minus owed for id.
func (e *Expense) Net(id uuid.UUID) int64 {
	for _, s := range e.Shares {
		if s.UserID == id {
			return s.Paid - s.Owed
		}
	}
	return 0
}

// Payer returns the user who paid the most (the first on ties).
func (e *Expense) Payer() uuid.UUID {
	var best Share
	for _, s := range e.Shares {
		if s.Paid > best.Paid {
			best = s
		}
	}
	return best.UserID
}

// Recipient returns, for payments, the user who received the money.
func (e *Expense) Recipient() uuid.UUID {
	for _, s := range e.Shares {
		if s.Owed > 0 {
			return s.UserID
		}
	}
	return uuid.Nil
}
