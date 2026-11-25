package reminder

import (
	"context"
	"fmt"

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
	Quota       ports.Quota
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
	quota       ports.Quota
	tx          ports.Tx
	clock       clock.Clock
	baseURL     string
}

// NewService builds a Service.
func NewService(d Deps) *Service {
	return &Service{repo: d.Repo, groups: d.Groups, connections: d.Connections, users: d.Users, balances: d.Balances,
		activity: d.Activity, notifier: d.Notifier, quota: d.Quota, tx: d.Tx, clock: d.Clock, baseURL: d.BaseURL}
}

// Send reminds to that they owe actor money, in one group when groupID is
// set or across everything otherwise. The recipient must owe the actor
// something in that scope and have an email address; one reminder per
// sender, recipient and scope is allowed every Window, and at most
// MaxPerPerson per sender and recipient across scopes.
func (s *Service) Send(ctx context.Context, actor, to uuid.UUID, groupID *uuid.UUID) (*Reminder, error) {
	if to == actor {
		return nil, errs.Validation("userId", "You cannot remind yourself.")
	}
	g, connected, err := s.scope(ctx, actor, to, groupID)
	if err != nil {
		return nil, err
	}
	owed, err := s.owed(ctx, actor, to, g)
	if err != nil {
		return nil, err
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
	note, notify := Compose(sender.Name, groupName, owed, s.link(actor, groupID)).Notification(target)
	notify = notify && !ports.IsQuiet(ctx)
	r := &Reminder{ID: uuid.New(), FromID: actor, ToID: to, GroupID: groupID, Owed: owed, SentAt: now,
		Emailed: notify && note.To != ""}
	if err := s.claim(ctx, r, target, groupName); err != nil {
		return nil, err
	}
	if notify {
		s.notifier.Notify(ctx, note)
	}
	return r, nil
}

// scope is the group a reminder is about, when groupID is set, and whether
// to shares it with actor, or else is connected to them at all.
func (s *Service) scope(ctx context.Context, actor, to uuid.UUID, groupID *uuid.UUID) (*group.Group, bool, error) {
	if groupID == nil {
		connected, err := s.connections.Connected(ctx, actor, to)
		return nil, connected, err
	}
	g, err := s.groups.Member(ctx, actor, *groupID)
	if errs.Is(err, errs.KindNotFound) {
		return nil, false, errs.Validation("groupId", "Group not found.")
	}
	if err != nil {
		return nil, false, err
	}
	return g, g.HasMember(to), nil
}

// owed is what to owes actor, in g or, when g is nil, overall.
func (s *Service) owed(ctx context.Context, actor, to uuid.UUID, g *group.Group) (money.Balances, error) {
	sheet, err := s.balances.For(ctx, actor)
	if err != nil {
		return nil, err
	}
	view := sheet.Friends[to]
	switch {
	case view == nil:
		return money.Balances{}, nil
	case g != nil:
		return Owed(view.ByGroup[g.ID]), nil
	default:
		return Owed(view.Total), nil
	}
}

// claim saves r unless its sender already reminded target in the same
// scope within Window, or sent them MaxPerPerson reminders in any scope.
func (s *Service) claim(ctx context.Context, r *Reminder, target *user.User, groupName string) error {
	in := ""
	if groupName != "" {
		in = " in " + groupName
	}
	return s.tx.Do(ctx, func(ctx context.Context) error {
		ok, err := s.repo.Claim(ctx, r, r.SentAt.Add(-Window))
		if err != nil {
			return err
		}
		if !ok {
			return errs.RateLimited("You already reminded " + target.Name + in + " in the last 24 hours. Try again tomorrow.")
		}
		ok, err = s.quota.Take(ctx, "remind:"+r.FromID.String()+":"+r.ToID.String(), MaxPerPerson, Window, r.SentAt)
		if err != nil {
			return err
		}
		if !ok {
			return errs.RateLimited(fmt.Sprintf("You already sent %s %d reminders in the last 24 hours. Try again tomorrow.", target.Name, MaxPerPerson))
		}
		data := map[string]any{"targetName": target.Name, "targetId": target.ID.String(), "balances": toData(r.Owed)}
		if groupName != "" {
			data["groupName"] = groupName
		}
		return s.activity.Record(ctx, activity.Activity{ActorID: r.FromID, GroupID: r.GroupID, Kind: activity.ReminderSent,
			Involved: []uuid.UUID{r.ToID}, Data: data})
	})
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
