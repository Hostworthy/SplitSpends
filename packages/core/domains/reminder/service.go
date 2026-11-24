package reminder

import (
	"context"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/activity"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/balance"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/group"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/user"
	"github.com/Hostworthy/SplitSpends/packages/core/ports"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/clock"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/money"
	"github.com/google/uuid"
)

// Groups resolves a group the caller belongs to.
type Groups interface {
	Member(ctx context.Context, actor, id uuid.UUID) (*group.Group, error)
}

// Connections answers whether two people are friends or share a group.
type Connections interface {
	Connected(ctx context.Context, a, b uuid.UUID) (bool, error)
}

// Deps groups Service dependencies.
type Deps struct {
	Repo        Repository
	Groups      Groups
	Connections Connections
	Users       *user.Service
	Balances    *balance.Service
	Activity    *activity.Service
	Notifier    ports.Notifier
	Tx          ports.Tx
	Clock       clock.Clock
	BaseURL     string
}

// Service sends settle-up reminders.
type Service struct {
	repo        Repository
	groups      Groups
	connections Connections
	users       *user.Service
	balances    *balance.Service
	activity    *activity.Service
	notifier    ports.Notifier
	tx          ports.Tx
	clock       clock.Clock
	baseURL     string
}

// NewService builds a Service.
func NewService(d Deps) *Service {
	return &Service{repo: d.Repo, groups: d.Groups, connections: d.Connections, users: d.Users, balances: d.Balances,
		activity: d.Activity, notifier: d.Notifier, tx: d.Tx, clock: d.Clock, baseURL: d.BaseURL}
}

// Send reminds to that they owe actor money, in one group when groupID is
// set or across everything otherwise. The recipient must owe the actor
// something in that scope and have an email address; one reminder per
// sender, recipient and scope is allowed every Window.
func (s *Service) Send(ctx context.Context, actor, to uuid.UUID, groupID *uuid.UUID) (*Reminder, error) {
	if to == actor {
		return nil, errs.Validation("userId", "You cannot remind yourself.")
	}
	var g *group.Group
	var connected bool
	if groupID != nil {
		var err error
		if g, err = s.groups.Member(ctx, actor, *groupID); err != nil {
			if errs.Is(err, errs.KindNotFound) {
				return nil, errs.Validation("groupId", "Group not found.")
			}
			return nil, err
		}
		connected = g.HasMember(to)
	} else {
		var err error
		if connected, err = s.connections.Connected(ctx, actor, to); err != nil {
			return nil, err
		}
	}
	sheet, err := s.balances.For(ctx, actor)
	if err != nil {
		return nil, err
	}
	owed := money.Balances{}
	if view := sheet.Friends[to]; view != nil {
		if g != nil {
			owed = Owed(view.ByGroup[g.ID])
		} else {
			owed = Owed(view.Total)
		}
	}
	if !connected && owed.Settled() {
		// Do not reveal anything about people the caller has no link with.
		return nil, errs.NotFound("Person")
	}
	target, err := s.users.Get(ctx, to)
	if err != nil {
		return nil, err
	}
	sender, err := s.users.Get(ctx, actor)
	if err != nil {
		return nil, err
	}
	in, groupName := "", ""
	if g != nil {
		groupName = g.Name
		in = " in " + g.Name
	}
	if owed.Settled() {
		return nil, errs.Conflict(target.Name + " does not owe you anything" + in + " right now.")
	}
	if target.Email == "" {
		return nil, errs.Conflict(target.Name + " has no email address yet, so they cannot receive reminders. Add their email first.")
	}

	now := s.clock.Now()
	r := &Reminder{ID: uuid.New(), FromID: actor, ToID: to, GroupID: groupID, Owed: owed, SentAt: now,
		Emailed: target.Notifications.Reminders && !ports.IsQuiet(ctx)}
	err = s.tx.Do(ctx, func(ctx context.Context) error {
		ok, err := s.repo.Claim(ctx, r, now.Add(-Window))
		if err != nil {
			return err
		}
		if !ok {
			return errs.RateLimited("You already reminded " + target.Name + in + " in the last 24 hours. Try again tomorrow.")
		}
		data := map[string]any{"targetName": target.Name, "targetId": target.ID.String(), "balances": toData(owed)}
		if groupName != "" {
			data["groupName"] = groupName
		}
		return s.activity.Record(ctx, activity.Activity{ActorID: actor, GroupID: groupID, Kind: activity.ReminderSent,
			Involved: []uuid.UUID{to}, Data: data})
	})
	if err != nil {
		return nil, err
	}
	if r.Emailed {
		s.notifier.Notify(ctx, Compose(sender.Name, groupName, owed, s.link(actor, groupID)).Notification(target))
	}
	return r, nil
}

// link opens the group, or the sender's page in the recipient's friend list.
func (s *Service) link(actor uuid.UUID, groupID *uuid.UUID) string {
	if groupID != nil {
		return s.baseURL + "/groups/" + groupID.String()
	}
	return s.baseURL + "/friends/" + actor.String()
}

func toData(b money.Balances) []map[string]any {
	out := []map[string]any{}
	for _, x := range b.List() {
		out = append(out, map[string]any{"currency": x.Currency, "amount": x.Amount})
	}
	return out
}
