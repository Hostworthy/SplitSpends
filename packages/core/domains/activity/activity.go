// Package activity records an append-only feed of what happened, visible to
// every user listed in Involved.
package activity

import (
	"context"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/shared/clock"
	"github.com/google/uuid"
)

// Kind names an event.
type Kind string

const (
	ExpenseCreated     Kind = "expense.created"
	ExpenseUpdated     Kind = "expense.updated"
	ExpenseDeleted     Kind = "expense.deleted"
	ExpenseRestored    Kind = "expense.restored"
	ExpenseStopped     Kind = "expense.stopped"
	PaymentCreated     Kind = "payment.created"
	CommentCreated     Kind = "comment.created"
	GroupCreated       Kind = "group.created"
	GroupUpdated       Kind = "group.updated"
	GroupDeleted       Kind = "group.deleted"
	GroupMemberAdded   Kind = "group.member-added"
	GroupMemberRemoved Kind = "group.member-removed"
	GroupMemberJoined  Kind = "group.member-joined"
	FriendAdded        Kind = "friend.added"
	ReminderSent       Kind = "reminder.sent"
)

// Activity is one feed event. Data holds a snapshot of display fields such as
// description, groupName, amount, currency and targetName.
type Activity struct {
	ID        uuid.UUID
	ActorID   uuid.UUID
	GroupID   *uuid.UUID
	ExpenseID *uuid.UUID
	Kind      Kind
	Data      map[string]any
	Involved  []uuid.UUID
	CreatedAt time.Time
}

// Filter narrows a feed query.
type Filter struct {
	UserID    uuid.UUID
	GroupID   *uuid.UUID
	ExpenseID *uuid.UUID
	Limit     int
	Offset    int
}

// Repository persists activity.
type Repository interface {
	Create(ctx context.Context, a *Activity) error
	List(ctx context.Context, f Filter) ([]*Activity, int, error)
}

// Service records and lists activity.
type Service struct {
	repo  Repository
	clock clock.Clock
}

// NewService builds a Service.
func NewService(repo Repository, c clock.Clock) *Service { return &Service{repo: repo, clock: c} }

// Record stores an event. Callers usually pass the transaction context.
func (s *Service) Record(ctx context.Context, a Activity) error {
	a.ID = uuid.New()
	a.CreatedAt = s.clock.Now()
	if a.Data == nil {
		a.Data = map[string]any{}
	}
	seen := map[uuid.UUID]bool{}
	involved := make([]uuid.UUID, 0, len(a.Involved)+1)
	for _, id := range append(a.Involved, a.ActorID) {
		if !seen[id] {
			seen[id] = true
			involved = append(involved, id)
		}
	}
	a.Involved = involved
	return s.repo.Create(ctx, &a)
}

// List returns the feed visible to f.UserID.
func (s *Service) List(ctx context.Context, f Filter) ([]*Activity, int, error) {
	return s.repo.List(ctx, f)
}

// Describe renders a one-line summary. actor is the actor's display name.
func Describe(a *Activity, actor string) string {
	str := func(k string) string {
		v, _ := a.Data[k].(string)
		return v
	}
	in := ""
	if g := str("groupName"); g != "" {
		in = " in " + g
	}
	switch a.Kind {
	case ExpenseCreated:
		return actor + " added “" + str("description") + "”" + in
	case ExpenseUpdated:
		return actor + " updated “" + str("description") + "”" + in
	case ExpenseDeleted:
		return actor + " deleted “" + str("description") + "”" + in
	case ExpenseRestored:
		return actor + " restored “" + str("description") + "”" + in
	case ExpenseStopped:
		if in == "" {
			return "“" + str("description") + "” stopped repeating because someone in it is no longer connected"
		}
		return "“" + str("description") + "” stopped repeating" + in + " because someone in it left the group"
	case PaymentCreated:
		return actor + " recorded a payment from " + str("fromName") + " to " + str("toName") + in
	case CommentCreated:
		return actor + " commented on “" + str("description") + "”"
	case GroupCreated:
		return actor + " created the group " + str("groupName")
	case GroupUpdated:
		return actor + " updated the group " + str("groupName")
	case GroupDeleted:
		return actor + " deleted the group " + str("groupName")
	case GroupMemberAdded:
		return actor + " added " + str("targetName") + " to " + str("groupName")
	case GroupMemberRemoved:
		if str("targetName") == actor {
			return actor + " left " + str("groupName")
		}
		return actor + " removed " + str("targetName") + " from " + str("groupName")
	case GroupMemberJoined:
		return actor + " joined " + str("groupName") + " with an invite link"
	case FriendAdded:
		return actor + " added " + str("targetName") + " as a friend"
	case ReminderSent:
		return actor + " reminded " + str("targetName") + " to settle up" + in
	}
	return actor + " did something"
}
