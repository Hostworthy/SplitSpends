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

// Next returns the next occurrence after date.
func (r Recurrence) Next(date time.Time) time.Time {
	switch r {
	case RecurWeekly:
		return date.AddDate(0, 0, 7)
	case RecurBiweekly:
		return date.AddDate(0, 0, 14)
	case RecurMonthly:
		return date.AddDate(0, 1, 0)
	case RecurYearly:
		return date.AddDate(1, 0, 0)
	}
	return date
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
