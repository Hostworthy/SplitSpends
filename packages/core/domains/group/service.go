package group

import (
	"context"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/activity"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/balance"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/user"
	"github.com/Hostworthy/SplitSpends/packages/core/ports"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/clock"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/money"
	"github.com/google/uuid"
)

// Service implements group use cases.
type Service struct {
	repo     Repository
	users    *user.Service
	balances *balance.Service
	activity *activity.Service
	notifier ports.Notifier
	tx       ports.Tx
	clock    clock.Clock
	baseURL  string
}

// NewService builds a Service.
func NewService(repo Repository, users *user.Service, balances *balance.Service, act *activity.Service, notifier ports.Notifier, tx ports.Tx, c clock.Clock, baseURL string) *Service {
	return &Service{repo: repo, users: users, balances: balances, activity: act, notifier: notifier, tx: tx, clock: c, baseURL: baseURL}
}

// View is a group with members and the caller's balance.
type View struct {
	*Group
	People   []*user.User
	Balances money.Balances
}

// Invitee identifies someone to add by email.
type Invitee struct {
	Email string
	Name  string
}

// CreateInput creates a group.
type CreateInput struct {
	// ID lets offline clients choose the id so retries are idempotent.
	ID *uuid.UUID
	Patch
	Members []Invitee
	// People are existing users added directly, for example during an import
	// where the caller has just connected with them.
	People []uuid.UUID
}

// Member returns the group if actor is an active member.
func (s *Service) Member(ctx context.Context, actor, id uuid.UUID) (*Group, error) {
	g, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if !g.HasMember(actor) {
		return nil, errs.NotFound("Group")
	}
	return g, nil
}

// Create makes a group with the actor and invitees as members.
func (s *Service) Create(ctx context.Context, actor uuid.UUID, in CreateInput) (*View, error) {
	if in.ID != nil && *in.ID != uuid.Nil {
		g, err := s.repo.Get(ctx, *in.ID)
		if err == nil {
			if g.HasMember(actor) {
				return s.view(ctx, actor, g)
			}
			return nil, errs.Conflict("That group id is already in use.")
		}
		if !errs.Is(err, errs.KindNotFound) {
			return nil, err
		}
	}
	me, err := s.users.Get(ctx, actor)
	if err != nil {
		return nil, err
	}
	name := ""
	if in.Name != nil {
		name = *in.Name
	}
	now := s.clock.Now()
	g, err := New(name, actor, me.DefaultCurrency, now)
	if err != nil {
		return nil, err
	}
	if in.ID != nil && *in.ID != uuid.Nil {
		g.ID = *in.ID
	}
	in.Name = nil
	if err := g.Apply(in.Patch, now); err != nil {
		return nil, err
	}
	var added []*user.User
	err = s.tx.Do(ctx, func(ctx context.Context) error {
		if err := s.repo.Create(ctx, g); err != nil {
			return err
		}
		if err := s.repo.AddMember(ctx, g.ID, actor, now); err != nil {
			return err
		}
		g.Members = []uuid.UUID{actor}
		for _, inv := range in.Members {
			u, _, err := s.users.Ensure(ctx, inv.Email, inv.Name, me)
			if err != nil {
				return err
			}
			if g.HasMember(u.ID) {
				continue
			}
			if err := s.repo.AddMember(ctx, g.ID, u.ID, now); err != nil {
				return err
			}
			g.Members = append(g.Members, u.ID)
			added = append(added, u)
		}
		for _, id := range in.People {
			if g.HasMember(id) {
				continue
			}
			if err := s.repo.AddMember(ctx, g.ID, id, now); err != nil {
				return err
			}
			g.Members = append(g.Members, id)
		}
		return s.activity.Record(ctx, activity.Activity{ActorID: actor, GroupID: &g.ID, Kind: activity.GroupCreated,
			Involved: g.Members, Data: map[string]any{"groupName": g.Name}})
	})
	if err != nil {
		return nil, err
	}
	for _, u := range added {
		if !ports.IsQuiet(ctx) {
			s.notifyAdded(ctx, u, me, g)
		}
	}
	return s.View(ctx, actor, g.ID)
}

func (s *Service) notifyAdded(ctx context.Context, u, by *user.User, g *Group) {
	n, ok := u.Notice(user.AddedToGroup, by.Name+" added you to "+g.Name,
		[]string{by.Name + " added you to the group “" + g.Name + "”."}, s.baseURL)
	if ok {
		s.notifier.Notify(ctx, n)
	}
}

// Mine returns the actor's active groups with their members, without
// balances.
func (s *Service) Mine(ctx context.Context, actor uuid.UUID) ([]*Group, error) {
	return s.repo.ForUser(ctx, actor, false)
}

// View returns a group the actor belongs to.
func (s *Service) View(ctx context.Context, actor, id uuid.UUID) (*View, error) {
	g, err := s.Member(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	return s.view(ctx, actor, g)
}

func (s *Service) view(ctx context.Context, actor uuid.UUID, g *Group) (*View, error) {
	people, err := s.users.List(ctx, g.Members)
	if err != nil {
		return nil, err
	}
	sheet, err := s.balances.Group(ctx, g.ID, g.SimplifyDebts)
	if err != nil {
		return nil, err
	}
	mine := sheet.Members[actor]
	if mine == nil {
		mine = money.Balances{}
	}
	return &View{Group: g, People: people, Balances: mine}, nil
}

// List returns the actor's groups with their balances.
func (s *Service) List(ctx context.Context, actor uuid.UUID, archived bool) ([]*View, error) {
	groups, err := s.repo.ForUser(ctx, actor, archived)
	if err != nil {
		return nil, err
	}
	sheet, err := s.balances.For(ctx, actor)
	if err != nil {
		return nil, err
	}
	ids := []uuid.UUID{}
	for _, g := range groups {
		ids = append(ids, g.Members...)
	}
	people, err := s.users.Index(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make([]*View, 0, len(groups))
	for _, g := range groups {
		v := &View{Group: g, Balances: sheet.Groups[g.ID]}
		if v.Balances == nil {
			v.Balances = money.Balances{}
		}
		for _, id := range g.Members {
			if u := people[id]; u != nil {
				v.People = append(v.People, u)
			}
		}
		out = append(out, v)
	}
	return out, nil
}

// Update applies a patch.
func (s *Service) Update(ctx context.Context, actor, id uuid.UUID, p Patch) (*View, error) {
	g, err := s.Member(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	if err := g.Apply(p, s.clock.Now()); err != nil {
		return nil, err
	}
	err = s.tx.Do(ctx, func(ctx context.Context) error {
		if err := s.repo.Update(ctx, g); err != nil {
			return err
		}
		return s.activity.Record(ctx, activity.Activity{ActorID: actor, GroupID: &g.ID, Kind: activity.GroupUpdated,
			Involved: g.Members, Data: map[string]any{"groupName": g.Name}})
	})
	if err != nil {
		return nil, err
	}
	return s.view(ctx, actor, g)
}

// Delete removes a settled group and its expenses.
func (s *Service) Delete(ctx context.Context, actor, id uuid.UUID) error {
	g, err := s.Member(ctx, actor, id)
	if err != nil {
		return err
	}
	settled, err := s.balances.GroupSettled(ctx, g.ID)
	if err != nil {
		return err
	}
	if !settled {
		return errs.Conflict("Settle all balances before deleting the group.")
	}
	return s.repo.Delete(ctx, g.ID)
}

// AddMember adds someone by email, inviting them if they are new.
func (s *Service) AddMember(ctx context.Context, actor, id uuid.UUID, inv Invitee) (*View, error) {
	g, err := s.Member(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	me, err := s.users.Get(ctx, actor)
	if err != nil {
		return nil, err
	}
	var added *user.User
	err = s.tx.Do(ctx, func(ctx context.Context) error {
		u, _, err := s.users.Ensure(ctx, inv.Email, inv.Name, me)
		if err != nil {
			return err
		}
		if g.HasMember(u.ID) {
			return errs.Conflict(u.Name + " is already in this group.")
		}
		if err := s.repo.AddMember(ctx, g.ID, u.ID, s.clock.Now()); err != nil {
			return err
		}
		g.Members = append(g.Members, u.ID)
		added = u
		return s.activity.Record(ctx, activity.Activity{ActorID: actor, GroupID: &g.ID, Kind: activity.GroupMemberAdded,
			Involved: g.Members, Data: map[string]any{"groupName": g.Name, "targetName": u.Name, "targetId": u.ID.String()}})
	})
	if err != nil {
		return nil, err
	}
	s.notifyAdded(ctx, added, me, g)
	return s.view(ctx, actor, g)
}

// RemoveMember removes a settled member. Members may remove themselves.
func (s *Service) RemoveMember(ctx context.Context, actor, id, member uuid.UUID) error {
	g, err := s.Member(ctx, actor, id)
	if err != nil {
		return err
	}
	if !g.HasMember(member) {
		return errs.NotFound("Member")
	}
	settled, err := s.balances.Settled(ctx, g.ID, member)
	if err != nil {
		return err
	}
	if !settled {
		return errs.Conflict("That person still has an outstanding balance in this group.")
	}
	target, err := s.users.Get(ctx, member)
	if err != nil {
		return err
	}
	return s.tx.Do(ctx, func(ctx context.Context) error {
		if err := s.repo.RemoveMember(ctx, g.ID, member, s.clock.Now()); err != nil {
			return err
		}
		return s.activity.Record(ctx, activity.Activity{ActorID: actor, GroupID: &g.ID, Kind: activity.GroupMemberRemoved,
			Involved: g.Members, Data: map[string]any{"groupName": g.Name, "targetName": target.Name, "targetId": target.ID.String()}})
	})
}

// Sheet is a group's balance sheet with member details.
type Sheet struct {
	Group   *Group
	Members map[uuid.UUID]money.Balances
	Debts   []balance.Debt
}

// Balances returns every member's balance and the settle-up plan.
func (s *Service) Balances(ctx context.Context, actor, id uuid.UUID) (*Sheet, error) {
	g, err := s.Member(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	view, err := s.balances.Group(ctx, g.ID, g.SimplifyDebts)
	if err != nil {
		return nil, err
	}
	members := map[uuid.UUID]money.Balances{}
	for _, m := range g.Members {
		members[m] = money.Balances{}
	}
	for id, b := range view.Members {
		members[id] = b
	}
	return &Sheet{Group: g, Members: members, Debts: view.Debts}, nil
}
