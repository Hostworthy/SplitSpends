package user

import (
	"github.com/Hostworthy/SplitSpends/packages/core/ports"
	"github.com/google/uuid"
)

// Kind names something a user can be notified about. The values double as
// the preference keys in the API.
type Kind string

const (
	ExpenseAdded    Kind = "expenseAdded"
	ExpenseUpdated  Kind = "expenseUpdated"
	PaymentReceived Kind = "paymentReceived"
	AddedToGroup    Kind = "addedToGroup"
	Reminder        Kind = "reminders"
)

// Switches turn one delivery channel on or off per kind.
type Switches struct {
	ExpenseAdded    bool `json:"expenseAdded"`
	ExpenseUpdated  bool `json:"expenseUpdated"`
	PaymentReceived bool `json:"paymentReceived"`
	AddedToGroup    bool `json:"addedToGroup"`
	// Reminders are settle-up nudges from people you owe.
	Reminders bool `json:"reminders"`
}

// On reports whether k is switched on. Unknown kinds are on, so a new kind
// reaches people until they turn it off.
func (s Switches) On(k Kind) bool {
	switch k {
	case ExpenseAdded:
		return s.ExpenseAdded
	case ExpenseUpdated:
		return s.ExpenseUpdated
	case PaymentReceived:
		return s.PaymentReceived
	case AddedToGroup:
		return s.AddedToGroup
	case Reminder:
		return s.Reminders
	}
	return true
}

// Notifications are per-user preferences. The embedded switches are the
// email channel (their JSON keys stay at the top level, as they were before
// push existed); Push holds the same switches for push notifications.
type Notifications struct {
	Switches
	Push Switches `json:"push"`
}

// DefaultNotifications enables every kind on every channel.
func DefaultNotifications() Notifications {
	all := Switches{ExpenseAdded: true, ExpenseUpdated: true, PaymentReceived: true, AddedToGroup: true, Reminders: true}
	return Notifications{Switches: all, Push: all}
}

// SwitchesPatch changes some switches; nil fields keep their value.
type SwitchesPatch struct {
	ExpenseAdded    *bool
	ExpenseUpdated  *bool
	PaymentReceived *bool
	AddedToGroup    *bool
	Reminders       *bool
}

// NotificationsPatch changes some preferences. The embedded switches are the
// email channel.
type NotificationsPatch struct {
	SwitchesPatch
	Push *SwitchesPatch
}

func (s *Switches) apply(p SwitchesPatch) {
	set := func(dst, v *bool) {
		if v != nil {
			*dst = *v
		}
	}
	set(&s.ExpenseAdded, p.ExpenseAdded)
	set(&s.ExpenseUpdated, p.ExpenseUpdated)
	set(&s.PaymentReceived, p.PaymentReceived)
	set(&s.AddedToGroup, p.AddedToGroup)
	set(&s.Reminders, p.Reminders)
}

// Apply merges p into n.
func (n *Notifications) Apply(p NotificationsPatch) {
	n.Switches.apply(p.SwitchesPatch)
	if p.Push != nil {
		n.Push.apply(*p.Push)
	}
}

// Notice addresses a notification about k to u on the channels u wants:
// email when the email switch is on, push when the push switch is on. It
// reports false when u gets nothing, either because they never signed in or
// because they turned k off on every channel.
func (u *User) Notice(k Kind, subject string, lines []string, link string) (ports.Notification, bool) {
	n := ports.Notification{Kind: string(k), Name: u.Name, Subject: subject, Lines: lines, Link: link}
	if u.Status != StatusActive {
		return n, false
	}
	if u.Notifications.On(k) && u.Email != "" {
		n.To = u.Email
	}
	if u.Notifications.Push.On(k) {
		n.UserID = u.ID
	}
	return n, n.To != "" || n.UserID != uuid.Nil
}
