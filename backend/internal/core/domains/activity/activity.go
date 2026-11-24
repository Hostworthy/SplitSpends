// Package activity records an append-only feed of what happened, visible to
// every user listed in Involved.
package activity

import (
	"context"
	"time"

	"github.com/atb00ker/splitspends/backend/internal/core/shared/clock"
	"github.com/google/uuid"
)

// Kind names an event.
type Kind string

const (
	ExpenseCreated     Kind = "expense.created"
	ExpenseUpdated     Kind = "expense.updated"
	ExpenseDeleted     Kind = "expense.deleted"
	ExpenseRestored    Kind = "expense.restored"
	PaymentCreated     Kind = "payment.created"
	CommentCreated     Kind = "comment.created"
	GroupCreated       Kind = "group.created"
	GroupUpdated       Kind = "group.updated"
	GroupMemberAdded   Kind = "group.member-added"
	GroupMemberRemoved Kind = "group.member-removed"
	FriendAdded        Kind = "friend.added"
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
	UserID  uuid.UUID
	GroupID *uuid.UUID
	Limit   int
	Offset  int
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
	case PaymentCreated:
		return actor + " recorded a payment from " + str("fromName") + " to " + str("toName") + in
	case CommentCreated:
		return actor + " commented on “" + str("description") + "”"
	case GroupCreated:
		return actor + " created the group " + str("groupName")
	case GroupUpdated:
		return actor + " updated the group " + str("groupName")
	case GroupMemberAdded:
		return actor + " added " + str("targetName") + " to " + str("groupName")
	case GroupMemberRemoved:
		if str("targetName") == actor {
			return actor + " left " + str("groupName")
		}
		return actor + " removed " + str("targetName") + " from " + str("groupName")
	case FriendAdded:
		return actor + " added " + str("targetName") + " as a friend"
	}
	return actor + " did something"
}
