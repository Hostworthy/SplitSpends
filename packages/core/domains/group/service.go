package group

import (
	"context"
	"fmt"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/activity"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/balance"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/user"
	"github.com/Hostworthy/SplitSpends/packages/core/ports"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/clock"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/money"
	"github.com/google/uuid"
)

// Users finds and invites the people groups hold.
type Users interface {
	Get(ctx context.Context, id uuid.UUID) (*user.User, error)
	List(ctx context.Context, ids []uuid.UUID) ([]*user.User, error)
	// Index is List keyed by id; ids that do not exist are missing.
	Index(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]*user.User, error)
	// Ensure returns the account for email, adding one invited by inviter
	// when there is none.
	Ensure(ctx context.Context, email, name string, inviter *user.User) (*user.User, bool, error)
	SendInvite(ctx context.Context, u, inviter *user.User)
}

// Balances answers who owes what, in a group and overall.
type Balances interface {
	For(ctx context.Context, me uuid.UUID) (*balance.Sheet, error)
	Group(ctx context.Context, groupID uuid.UUID, simplify bool) (*balance.GroupView, error)
	// Settled reports whether userID owes and is owed nothing in the group.
	Settled(ctx context.Context, groupID, userID uuid.UUID) (bool, error)
	// GroupSettled reports whether everyone in the group is settled.
	GroupSettled(ctx context.Context, groupID uuid.UUID) (bool, error)
}

// Recorder adds to the activity feed.
type Recorder interface {
	Record(ctx context.Context, a activity.Activity) error
}

// Service implements group use cases.
type Service struct {
	repo     Repository
	users    Users
	balances Balances
	activity Recorder
	notifier ports.Notifier
	quota    ports.Quota
	tx       ports.Tx
	clock    clock.Clock
	baseURL  string
}

// Someone added to groups by the same person is emailed about it at most
// MaxAddedEmails times per AddedWindow; later additions only reach their
// devices.
const (
	MaxAddedEmails = 3
	AddedWindow    = 24 * time.Hour
)

// Deps groups Service dependencies.
type Deps struct {
	Repo     Repository
	Users    Users
	Balances Balances
	Activity Recorder
	Notifier ports.Notifier
	Quota    ports.Quota
	Tx       ports.Tx
	Clock    clock.Clock
	BaseURL  string
}

// NewService builds a Service.
func NewService(d Deps) *Service {
	return &Service{repo: d.Repo, users: d.Users, balances: d.Balances, activity: d.Activity, notifier: d.Notifier,
		quota: d.Quota, tx: d.Tx, clock: d.Clock, baseURL: d.BaseURL}
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

// Lookup returns a group without asking who wants it, for work done on
// nobody's behalf, such as booking the next copy of a recurring expense.
// Anything a person asks for goes through Member.
func (s *Service) Lookup(ctx context.Context, id uuid.UUID) (*Group, error) {
	return s.repo.Get(ctx, id)
}

// Hold keeps the group's members as they are until the transaction in ctx
// ends, so a check made on them still holds when the work it allowed is
// saved. It returns errs.NotFound when the group is missing.
func (s *Service) Hold(ctx context.Context, id uuid.UUID) error {
	return s.repo.Hold(ctx, id)
}

// existing returns the group with a client-chosen id when it already exists
// and the actor belongs to it, so retried creates are idempotent. An id used
// for someone else's group is a conflict.
func (s *Service) existing(ctx context.Context, actor uuid.UUID, id *uuid.UUID) (*View, error) {
	if id == nil || *id == uuid.Nil {
		return nil, nil
	}
	g, err := s.repo.Get(ctx, *id)
	if errs.Is(err, errs.KindNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !g.HasMember(actor) {
		return nil, errs.Taken()
	}
	return s.view(ctx, actor, g)
}

// Create makes a group with the actor and invitees as members.
func (s *Service) Create(ctx context.Context, actor uuid.UUID, in CreateInput) (*View, error) {
	if v, err := s.existing(ctx, actor, in.ID); v != nil || err != nil {
		return v, err
	}
	if 1+len(in.Members)+len(in.People) > MaxMembers {
		return nil, full()
	}
	me, err := s.users.Get(ctx, actor)
	if err != nil {
		return nil, err
	}
	if err := s.room(ctx, actor, me); err != nil {
		return nil, err
	}
	now := s.clock.Now()
	g, split, err := start(in, me, now)
	if err != nil {
		return nil, err
	}
	var added, invited []*user.User
	err = s.tx.Do(ctx, func(ctx context.Context) error {
		if err := s.repo.Create(ctx, g); err != nil {
			return err
		}
		if err := s.repo.AddMember(ctx, g.ID, actor, now); err != nil {
			return err
		}
		g.Members = []uuid.UUID{actor}
		if added, invited, err = s.populate(ctx, g, me, in, now); err != nil {
			return err
		}
		if split != nil {
			if err := g.Apply(Patch{DefaultSplit: split}, now); err != nil {
				return err
			}
			if err := s.repo.Update(ctx, g); err != nil {
				return err
			}
		}
		return s.activity.Record(ctx, activity.Activity{ActorID: actor, GroupID: &g.ID, Kind: activity.GroupCreated,
			Involved: g.Members, Data: map[string]any{"groupName": g.Name}})
	})
	if errs.Is(err, errs.KindConflict) {
		// A request with the same id may have saved its group first.
		if v, err := s.existing(ctx, actor, in.ID); v != nil || err != nil {
			return v, err
		}
	}
	if err != nil {
		return nil, err
	}
	for _, u := range invited {
		s.users.SendInvite(ctx, u, me)
	}
	s.notifyAdded(ctx, me, g, added...)
	return s.View(ctx, actor, g.ID)
}

// start builds the group in asks for, created by me, without its default
// split: that is checked against the members, so it is returned to apply
// once they are known.
func start(in CreateInput, me *user.User, now time.Time) (*Group, *DefaultSplit, error) {
	name := ""
	if in.Name != nil {
		name = *in.Name
	}
	g, err := New(name, me.ID, me.DefaultCurrency, now)
	if err != nil {
		return nil, nil, err
	}
	if in.ID != nil && *in.ID != uuid.Nil {
		g.ID = *in.ID
	}
	split := in.DefaultSplit
	in.Name, in.DefaultSplit = nil, nil
	if err := g.Apply(in.Patch, now); err != nil {
		return nil, nil, err
	}
	return g, split, nil
}

// populate adds the invitees and people of a new group, returning the
// invitees who were added and those who got an account just now.
func (s *Service) populate(ctx context.Context, g *Group, me *user.User, in CreateInput, now time.Time) (added, invited []*user.User, err error) {
	for _, inv := range in.Members {
		u, created, err := s.users.Ensure(ctx, inv.Email, inv.Name, me)
		if err != nil {
			return nil, nil, err
		}
		if created {
			invited = append(invited, u)
		}
		if g.HasMember(u.ID) {
			continue
		}
		if err := s.enroll(ctx, me.ID, g, u, now); err != nil {
			return nil, nil, err
		}
		added = append(added, u)
	}
	for _, id := range in.People {
		if g.HasMember(id) {
			continue
		}
		u, err := s.users.Get(ctx, id)
		if err != nil {
			return nil, nil, err
		}
		if err := s.enroll(ctx, me.ID, g, u, now); err != nil {
			return nil, nil, err
		}
	}
	return added, invited, nil
}

// enroll adds u to g, unless they are in as many groups as one person may be.
func (s *Service) enroll(ctx context.Context, actor uuid.UUID, g *Group, u *user.User, now time.Time) error {
	if err := s.room(ctx, actor, u); err != nil {
		return err
	}
	if err := s.repo.AddMember(ctx, g.ID, u.ID, now); err != nil {
		return err
	}
	g.Members = append(g.Members, u.ID)
	return nil
}

// room refuses to add u to another group once they belong to MaxPerUser.
func (s *Service) room(ctx context.Context, actor uuid.UUID, u *user.User) error {
	n, err := s.repo.Joined(ctx, u.ID)
	if err != nil || n < MaxPerUser {
		return err
	}
	if u.ID == actor {
		return errs.Conflict(fmt.Sprintf("You are already in %d groups, the most one person can be in. Leave or delete a group you no longer use first.", MaxPerUser))
	}
	return errs.Conflict(fmt.Sprintf("%s is already in %d groups, the most one person can be in.", u.Name, MaxPerUser))
}

func full() error {
	return errs.Validation("members", fmt.Sprintf("A group can have at most %d members.", MaxMembers))
}

func (s *Service) notifyAdded(ctx context.Context, by *user.User, g *Group, people ...*user.User) {
	if ports.IsQuiet(ctx) {
		return
	}
	var notes []ports.Notification
	for _, u := range people {
		n, ok := u.Notice(user.AddedToGroup, by.Name+" added you to "+g.Name,
			[]string{by.Name + " added you to the group “" + g.Name + "”."}, s.baseURL)
		if !ok {
			continue
		}
		if n.To != "" {
			emailed, err := s.quota.Take(ctx, "added:"+by.ID.String()+":"+u.ID.String(), MaxAddedEmails, AddedWindow, s.clock.Now())
			if err != nil || !emailed {
				n.To = ""
			}
		}
		notes = append(notes, n)
	}
	s.notifier.Notify(ctx, notes...)
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

// List returns a page of the actor's groups with their balances, most
// recently changed first, and how many there are. A limit of 0 returns them
// all.
func (s *Service) List(ctx context.Context, actor uuid.UUID, archived bool, limit, offset int) ([]*View, int, error) {
	groups, err := s.repo.ForUser(ctx, actor, archived)
	if err != nil {
		return nil, 0, err
	}
	total := len(groups)
	groups = groups[min(offset, total):]
	if limit > 0 && limit < len(groups) {
		groups = groups[:limit]
	}
	sheet, err := s.balances.For(ctx, actor)
	if err != nil {
		return nil, 0, err
	}
	ids := []uuid.UUID{}
	for _, g := range groups {
		ids = append(ids, g.Members...)
	}
	people, err := s.users.Index(ctx, ids)
	if err != nil {
		return nil, 0, err
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
	return out, total, nil
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

// Delete removes a settled group with all its expenses and history. Only the
// person who created the group may, or any member once its creator has left,
// because anyone holding an invite link can become a member. The group stays
// locked from the check to the delete, so nothing booked meanwhile is lost,
// and the feed entry has no group, so it outlives the group.
func (s *Service) Delete(ctx context.Context, actor, id uuid.UUID) error {
	return s.tx.Do(ctx, func(ctx context.Context) error {
		g, err := s.repo.Lock(ctx, id)
		if err != nil {
			return err
		}
		if !g.HasMember(actor) {
			return errs.NotFound("Group")
		}
		if actor != g.CreatedBy && g.HasMember(g.CreatedBy) {
			return errs.Forbidden("Only the person who created this group can delete it.")
		}
		settled, err := s.balances.GroupSettled(ctx, g.ID)
		if err != nil {
			return err
		}
		if !settled {
			return errs.Conflict("Settle all balances before deleting the group.")
		}
		if err := s.activity.Record(ctx, activity.Activity{ActorID: actor, Kind: activity.GroupDeleted,
			Involved: g.Members, Data: map[string]any{"groupName": g.Name}}); err != nil {
			return err
		}
		return s.repo.Delete(ctx, g.ID)
	})
}

// AddMember adds someone by email, inviting them if they are new.
func (s *Service) AddMember(ctx context.Context, actor, id uuid.UUID, inv Invitee) (*View, error) {
	g, err := s.Member(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	if len(g.Members) >= MaxMembers {
		return nil, full()
	}
	me, err := s.users.Get(ctx, actor)
	if err != nil {
		return nil, err
	}
	var added *user.User
	var invited bool
	err = s.tx.Do(ctx, func(ctx context.Context) error {
		if g, err = s.locked(ctx, actor, id); err != nil {
			return err
		}
		if len(g.Members) >= MaxMembers {
			return full()
		}
		u, created, err := s.users.Ensure(ctx, inv.Email, inv.Name, me)
		if err != nil {
			return err
		}
		invited = created
		if g.HasMember(u.ID) {
			return errs.Conflict(u.Name + " is already in this group.")
		}
		if err := s.enroll(ctx, actor, g, u, s.clock.Now()); err != nil {
			return err
		}
		added = u
		return s.activity.Record(ctx, activity.Activity{ActorID: actor, GroupID: &g.ID, Kind: activity.GroupMemberAdded,
			Involved: g.Members, Data: map[string]any{"groupName": g.Name, "targetName": u.Name, "targetId": u.ID.String()}})
	})
	if err != nil {
		return nil, err
	}
	if invited {
		s.users.SendInvite(ctx, added, me)
	}
	s.notifyAdded(ctx, me, g, added)
	return s.view(ctx, actor, g)
}

// Join adds the actor to a group, for example through an invite link.
// Joining a group you already belong to changes nothing; an archived group
// takes no new members.
func (s *Service) Join(ctx context.Context, actor, id uuid.UUID) (*View, error) {
	g, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if g.HasMember(actor) {
		return s.view(ctx, actor, g)
	}
	me, err := s.users.Get(ctx, actor)
	if err != nil {
		return nil, err
	}
	err = s.tx.Do(ctx, func(ctx context.Context) error {
		// Locked and read again so two people joining at once cannot both
		// take the last place.
		if _, err := s.repo.Lock(ctx, id); err != nil {
			return err
		}
		if g, err = s.repo.Get(ctx, id); err != nil {
			return err
		}
		if g.HasMember(actor) {
			return nil
		}
		if g.ArchivedAt != nil {
			return errs.Conflict(g.Name + " is archived, so nobody new can join it.")
		}
		if len(g.Members) >= MaxMembers {
			return errs.Conflict(fmt.Sprintf("This group already has %d members, the most a group can have.", MaxMembers))
		}
		if err := s.room(ctx, actor, me); err != nil {
			return err
		}
		if err := s.repo.AddMember(ctx, g.ID, actor, s.clock.Now()); err != nil {
			return err
		}
		g.Members = append(g.Members, actor)
		return s.activity.Record(ctx, activity.Activity{ActorID: actor, GroupID: &g.ID, Kind: activity.GroupMemberJoined,
			Involved: g.Members, Data: map[string]any{"groupName": g.Name, "targetName": me.Name, "targetId": me.ID.String()}})
	})
	if err != nil {
		return nil, err
	}
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
	target, err := s.users.Get(ctx, member)
	if err != nil {
		return err
	}
	return s.tx.Do(ctx, func(ctx context.Context) error {
		// Locked so two members leaving at once cannot both pass the check,
		// and so an expense booked meanwhile either counts in the balance or
		// waits and is refused.
		g, err := s.locked(ctx, actor, g.ID)
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
		if err := s.keepsReachable(ctx, g, member); err != nil {
			return err
		}
		if err := s.repo.RemoveMember(ctx, g.ID, member, s.clock.Now()); err != nil {
			return err
		}
		return s.activity.Record(ctx, activity.Activity{ActorID: actor, GroupID: &g.ID, Kind: activity.GroupMemberRemoved,
			Involved: g.Members, Data: map[string]any{"groupName": g.Name, "targetName": target.Name, "targetId": target.ID.String()}})
	})
}

// locked locks the group for the rest of the transaction in ctx and returns
// it as it is once the lock is held, if actor is still a member. The locking
// statement sees the members as they were before it waited, so they are read
// again.
func (s *Service) locked(ctx context.Context, actor, id uuid.UUID) (*Group, error) {
	if _, err := s.repo.Lock(ctx, id); err != nil {
		return nil, err
	}
	g, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if !g.HasMember(actor) {
		return nil, errs.NotFound("Group")
	}
	return g, nil
}

// keepsReachable refuses to remove the last member who can sign in: nobody
// could open or delete the group after that.
func (s *Service) keepsReachable(ctx context.Context, g *Group, member uuid.UUID) error {
	others := make([]uuid.UUID, 0, len(g.Members))
	for _, m := range g.Members {
		if m != member {
			others = append(others, m)
		}
	}
	people, err := s.users.List(ctx, others)
	if err != nil {
		return err
	}
	for _, u := range people {
		if u.Status != user.StatusPlaceholder {
			return nil
		}
	}
	return errs.Conflict("You are the last member who can open this group. Delete the group instead, or add someone first.")
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
