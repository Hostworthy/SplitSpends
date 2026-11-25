// Package transfer imports data someone recorded in local-only mode into a
// server account, in one transaction and without sending notifications.
// Retrying an import is safe: groups, expenses, custom categories and
// name-only people keep the ids the device chose, and people with an email
// resolve to the same user.
package transfer

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/category"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/expense"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/friend"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/group"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/user"
	"github.com/Hostworthy/SplitSpends/packages/core/ports"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/google/uuid"
)

// Limits keep a single request bounded.
const (
	MaxPeople     = 1000
	MaxGroups     = 500
	MaxExpenses   = 20000
	MaxCategories = category.MaxPerUser
	// MaxRunning is how many imports the server runs at once. Each holds a
	// database connection until it ends, so more would starve every other
	// request.
	MaxRunning = 2
)

// Person is someone from the device's address book.
type Person struct {
	ID    uuid.UUID
	Name  string
	Email string
}

// Category is a custom category from the device. Deleted ones are imported
// too, so expenses that still use them keep their id, and are deleted once
// the expenses are in.
type Category struct {
	Input   category.Input
	Deleted bool
}

// Group is a local group; Members and the keys of Patch.DefaultSplit are
// local person ids.
type Group struct {
	ID      uuid.UUID
	Patch   group.Patch
	Members []uuid.UUID
}

// Expense is a local expense or payment whose user ids are local person ids.
type Expense struct {
	Kind  expense.Kind
	Input expense.Input
}

// Bundle is everything recorded on the device. Me is the local id that
// stands for the device owner.
type Bundle struct {
	Me         uuid.UUID
	People     []Person
	Categories []Category
	Groups     []Group
	Expenses   []Expense
}

// Result maps local person ids to server user ids.
type Result struct {
	People     map[uuid.UUID]uuid.UUID
	Categories int
	Groups     int
	Expenses   int
}

// Service runs imports.
type Service struct {
	users      *user.Service
	friends    friend.Repository
	groups     *group.Service
	expenses   *expense.Service
	categories *category.Service
	tx         ports.Tx
	locks      ports.Locker
}

// NewService builds a Service.
func NewService(users *user.Service, friends friend.Repository, groups *group.Service, expenses *expense.Service, categories *category.Service, tx ports.Tx, locks ports.Locker) *Service {
	return &Service{users: users, friends: friends, groups: groups, expenses: expenses, categories: categories, tx: tx, locks: locks}
}

// Import copies b into actor's account.
func (s *Service) Import(ctx context.Context, actor uuid.UUID, b Bundle) (*Result, error) {
	if len(b.People) > MaxPeople || len(b.Groups) > MaxGroups || len(b.Expenses) > MaxExpenses || len(b.Categories) > MaxCategories {
		return nil, errs.Validation("", fmt.Sprintf("Import at most %d people, %d categories, %d groups and %d expenses at a time.",
			MaxPeople, MaxCategories, MaxGroups, MaxExpenses))
	}
	me, err := s.users.Get(ctx, actor)
	if err != nil {
		return nil, err
	}
	res := &Result{People: map[uuid.UUID]uuid.UUID{b.Me: actor}}
	ctx = ports.Quiet(ctx)
	err = s.tx.Do(ctx, func(ctx context.Context) error {
		if err := s.claim(ctx, actor); err != nil {
			return err
		}
		for i, p := range b.People {
			id, err := s.person(ctx, me, p)
			if err != nil {
				return wrap(err, fmt.Sprintf("people[%d]", i), p.Name)
			}
			res.People[p.ID] = id
		}
		for i, c := range b.Categories {
			if _, err := s.categories.Create(ctx, actor, c.Input); err != nil {
				return wrap(err, fmt.Sprintf("categories[%d]", i), name(c.Input.Name))
			}
			res.Categories++
		}
		for i, g := range b.Groups {
			patch, members, err := remapGroup(res.People, g)
			if err != nil {
				return wrap(err, fmt.Sprintf("groups[%d]", i), name(g.Patch.Name))
			}
			id := g.ID
			if _, err := s.groups.Create(ctx, actor, group.CreateInput{ID: &id, Patch: patch, People: members}); err != nil {
				return wrap(err, fmt.Sprintf("groups[%d]", i), name(g.Patch.Name))
			}
			res.Groups++
		}
		for i, e := range b.Expenses {
			if err := s.expense(ctx, actor, res.People, e); err != nil {
				return wrap(err, fmt.Sprintf("expenses[%d]", i), e.Input.Description)
			}
			res.Expenses++
		}
		for i, c := range b.Categories {
			if !c.Deleted || c.Input.ID == nil {
				continue
			}
			err := s.categories.Delete(ctx, actor, *c.Input.ID)
			if err != nil && !errs.Is(err, errs.KindNotFound) {
				return wrap(err, fmt.Sprintf("categories[%d]", i), name(c.Input.Name))
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// claim lets each person run one import at a time and the server at most
// MaxRunning. The locks are released when the transaction ends.
func (s *Service) claim(ctx context.Context, actor uuid.UUID) error {
	ok, err := s.locks.TryLock(ctx, "import:user:"+actor.String())
	if err != nil {
		return err
	}
	if !ok {
		return errs.Conflict("An import into your account is already running. Wait for it to finish, then try again.")
	}
	for slot := range MaxRunning {
		ok, err := s.locks.TryLock(ctx, fmt.Sprintf("import:slot:%d", slot))
		if err != nil || ok {
			return err
		}
	}
	return errs.RateLimited("The server is busy with other imports. Try again in a minute.")
}

func (s *Service) person(ctx context.Context, me *user.User, p Person) (uuid.UUID, error) {
	email := strings.TrimSpace(p.Email)
	if email != "" {
		normalized, err := user.NormalizeEmail(email)
		if err != nil {
			return uuid.Nil, err
		}
		if normalized == me.Email {
			return me.ID, nil
		}
		u, _, err := s.users.Ensure(ctx, normalized, p.Name, me)
		if err != nil {
			return uuid.Nil, err
		}
		return u.ID, s.friends.Add(ctx, me.ID, u.ID)
	}
	id := p.ID
	u, err := s.users.Placeholder(ctx, &id, p.Name, me.ID)
	if err != nil {
		return uuid.Nil, err
	}
	return u.ID, s.friends.Add(ctx, me.ID, u.ID)
}

func (s *Service) expense(ctx context.Context, actor uuid.UUID, people map[uuid.UUID]uuid.UUID, e Expense) error {
	in := e.Input
	// Copy before remapping so the bundle stays intact (retries, shared slices).
	in.Payers, in.Splits = slices.Clone(in.Payers), slices.Clone(in.Splits)
	var err error
	for i := range in.Payers {
		if in.Payers[i].UserID, err = remap(people, in.Payers[i].UserID); err != nil {
			return err
		}
	}
	for i := range in.Splits {
		if in.Splits[i].UserID, err = remap(people, in.Splits[i].UserID); err != nil {
			return err
		}
	}
	items := make([]expense.Item, len(in.Items))
	for i, it := range in.Items {
		ids, err := remapAll(people, it.UserIDs)
		if err != nil {
			return err
		}
		items[i] = expense.Item{Name: it.Name, Amount: it.Amount, UserIDs: ids}
	}
	in.Items = items

	if e.Kind == expense.KindPayment {
		if len(in.Payers) != 1 || len(in.Splits) != 1 {
			return errs.Validation("splits", "A payment needs exactly one payer and one recipient.")
		}
		_, err = s.expenses.RecordPayment(ctx, actor, expense.PaymentInput{ID: in.ID, GroupID: in.GroupID, From: in.Payers[0].UserID,
			To: in.Splits[0].UserID, Amount: in.Total, Currency: in.Currency, Date: in.Date, Notes: in.Notes})
		return err
	}
	_, err = s.expenses.Create(ctx, actor, in)
	return err
}

// remapGroup rewrites local person ids in a group's members and default
// split.
func remapGroup(people map[uuid.UUID]uuid.UUID, g Group) (group.Patch, []uuid.UUID, error) {
	members, err := remapAll(people, g.Members)
	if err != nil {
		return group.Patch{}, nil, err
	}
	patch := g.Patch
	if d := patch.DefaultSplit; d != nil && d.Method != "" {
		values := make(map[uuid.UUID]string, len(d.Values))
		for local, v := range d.Values {
			id, err := remap(people, local)
			if err != nil {
				return group.Patch{}, nil, err
			}
			values[id] = v
		}
		patch.DefaultSplit = &group.DefaultSplit{Method: d.Method, Values: values}
	}
	return patch, members, nil
}

func remap(people map[uuid.UUID]uuid.UUID, local uuid.UUID) (uuid.UUID, error) {
	if id, ok := people[local]; ok {
		return id, nil
	}
	return uuid.Nil, errs.Validation("people", "Refers to a person who is not in the import.")
}

func remapAll(people map[uuid.UUID]uuid.UUID, locals []uuid.UUID) ([]uuid.UUID, error) {
	out := make([]uuid.UUID, 0, len(locals))
	for _, l := range locals {
		id, err := remap(people, l)
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}

func name(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// wrap prefixes a domain error with the failing item so the app can point at it.
func wrap(err error, path, label string) error {
	de, ok := errs.As(err)
	if !ok {
		return err
	}
	field := path
	if de.Field != "" {
		field += "." + de.Field
	}
	msg := de.Message
	if label != "" {
		msg = "“" + label + "”: " + msg
	}
	return &errs.Error{Kind: de.Kind, Field: field, Message: msg, Cause: de.Cause}
}
