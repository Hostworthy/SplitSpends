// Package reminder lets people nudge someone who owes them money to settle
// up. The message is built once here so every channel (email today, push
// later) sends the same words.
package reminder

import (
	"context"
	"strings"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/user"
	"github.com/Hostworthy/SplitSpends/packages/core/ports"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/money"
	"github.com/google/uuid"
)

// Window is how long a reminder blocks the next one from the same sender to
// the same person in the same scope (one group, or everything).
const Window = 24 * time.Hour

// MaxPerPerson is how many reminders one sender may send the same person per
// Window across all scopes, so sharing many groups does not multiply them.
const MaxPerPerson = 3

// Reminder is one nudge from FromID to ToID, optionally about one group.
type Reminder struct {
	ID      uuid.UUID
	FromID  uuid.UUID
	ToID    uuid.UUID
	GroupID *uuid.UUID
	// Owed is what ToID owes FromID per currency, all amounts positive.
	Owed money.Balances
	// Emailed is false when the recipient turned reminder emails off.
	Emailed bool
	SentAt  time.Time
}

// Repository persists reminders.
type Repository interface {
	// Claim records r unless a reminder with the same sender, recipient and
	// scope was sent after since. It reports whether r was recorded. The
	// check and the write are one atomic statement, so concurrent requests
	// cannot both pass.
	Claim(ctx context.Context, r *Reminder, since time.Time) (bool, error)
}

// Owed keeps the currencies in which the other person owes the caller. b is
// from the caller's point of view: positive means the caller is owed.
func Owed(b money.Balances) money.Balances {
	out := money.Balances{}
	for c, a := range b {
		if a > 0 {
			out[c] = a
		}
	}
	return out
}

// Message is the reminder text shared by every delivery channel.
type Message struct {
	Subject string
	Lines   []string
	Link    string
}

// Compose writes the reminder from sender to the recipient. group is the
// group name, empty for an overall reminder. link opens the app.
func Compose(sender, group string, owed money.Balances, link string) Message {
	in := ""
	if group != "" {
		in = " in " + group
	}
	return Message{
		Subject: sender + " sent you a reminder to settle up",
		Lines: []string{
			sender + " sent you a friendly reminder to settle up" + in + ".",
			"You owe " + sender + " " + Amounts(owed) + ".",
			"Once you have paid them back, record the payment so your balances stay right.",
		},
		Link: link,
	}
}

// Notification addresses m to the recipient on the channels they want
// (email, push or both). People who were invited but never signed in have
// no devices yet, so they get the email only. ok is false when nothing
// would be sent.
func (m Message) Notification(to *user.User) (n ports.Notification, ok bool) {
	if n, ok := to.Notice(user.Reminder, m.Subject, m.Lines, m.Link); ok {
		return n, true
	}
	if to.Status == user.StatusInvited && to.Email != "" && to.Notifications.On(user.Reminder) {
		return ports.Notification{Kind: string(user.Reminder), To: to.Email, Name: to.Name, Subject: m.Subject, Lines: m.Lines, Link: m.Link}, true
	}
	return ports.Notification{}, false
}

// Amounts renders balances as "€12.50 and $5.00", sorted by currency.
func Amounts(b money.Balances) string {
	parts := []string{}
	for _, x := range b.List() {
		parts = append(parts, money.Format(x.Amount, x.Currency))
	}
	switch len(parts) {
	case 0:
		return "nothing"
	case 1:
		return parts[0]
	}
	return strings.Join(parts[:len(parts)-1], ", ") + " and " + parts[len(parts)-1]
}
