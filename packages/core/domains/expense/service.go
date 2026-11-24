package expense

import (
	"context"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/activity"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/group"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/user"
	"github.com/Hostworthy/SplitSpends/packages/core/ports"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/clock"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/money"
	"github.com/google/uuid"
)

// Connections answers whether two users may share non-group expenses.
type Connections interface {
	Connected(ctx context.Context, a, b uuid.UUID) (bool, error)
}

// Service implements expense, payment and comment use cases.
type Service struct {
	repo        Repository
	comments    CommentRepository
	groups      group.Repository
	connections Connections
	receipts    Receipts
	users       *user.Service
	activity    *activity.Service
	notifier    ports.Notifier
	tx          ports.Tx
	clock       clock.Clock
	baseURL     string
}

// Deps groups Service dependencies.
type Deps struct {
	Repo        Repository
	Comments    CommentRepository
	Groups      group.Repository
	Connections Connections
	Receipts    Receipts
	Users       *user.Service
	Activity    *activity.Service
	Notifier    ports.Notifier
	Tx          ports.Tx
	Clock       clock.Clock
	BaseURL     string
}

// NewService builds a Service.
func NewService(d Deps) *Service {
	return &Service{repo: d.Repo, comments: d.Comments, groups: d.Groups, connections: d.Connections, receipts: d.Receipts,
		users: d.Users, activity: d.Activity, notifier: d.Notifier, tx: d.Tx, clock: d.Clock, baseURL: d.BaseURL}
}

// load fetches an expense the actor may see, plus its group if any.
func (s *Service) load(ctx context.Context, actor, id uuid.UUID) (*Expense, *group.Group, error) {
	e, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if e.GroupID == nil {
		if !e.Involves(actor) {
			return nil, nil, errs.NotFound("Expense")
		}
		return e, nil, nil
	}
	g, err := s.groups.Get(ctx, *e.GroupID)
	if err != nil {
		return nil, nil, err
	}
	if !g.HasMember(actor) && !e.Involves(actor) {
		return nil, nil, errs.NotFound("Expense")
	}
	return e, g, nil
}

// authorize checks that the actor may book an expense with these participants.
func (s *Service) authorize(ctx context.Context, actor uuid.UUID, groupID *uuid.UUID, shares []Share, receiptID *uuid.UUID) (*group.Group, error) {
	if receiptID != nil {
		ok, err := s.receipts.Owned(ctx, *receiptID, actor)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, errs.Validation("receiptId", "Receipt not found.")
		}
	}
	if groupID != nil {
		g, err := s.groups.Get(ctx, *groupID)
		if err != nil {
			if errs.Is(err, errs.KindNotFound) {
				return nil, errs.Validation("groupId", "Group not found.")
			}
			return nil, err
		}
		if !g.HasMember(actor) {
			return nil, errs.Validation("groupId", "Group not found.")
		}
		for _, sh := range shares {
			if !g.HasMember(sh.UserID) {
				return nil, errs.Validation("splits", "Everyone in the expense must be a member of the group.")
			}
		}
		return g, nil
	}
	involved := false
	for _, sh := range shares {
		if sh.UserID == actor {
			involved = true
			continue
		}
		ok, err := s.connections.Connected(ctx, actor, sh.UserID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, errs.Validation("splits", "You can only split non-group expenses with friends.")
		}
	}
	if !involved {
		return nil, errs.Validation("splits", "You must be part of an expense outside a group.")
	}
	return nil, nil
}

func (s *Service) build(e *Expense, n *Normalized, actor uuid.UUID, now time.Time) {
	e.Description, e.Category, e.Currency, e.Total = n.Description, n.Category, n.Currency, n.Total
	e.Date, e.Notes, e.Method, e.Shares = n.Date, strings.TrimSpace(n.Notes), n.Method, n.Shares
	e.Items, e.Tax, e.Tip, e.ReceiptID, e.Recurrence = n.Items, n.Tax, n.Tip, n.ReceiptID, n.Recurrence
	if e.Method == "" {
		e.Method = MethodEqual
	}
	e.NextDate = nil
	if e.Recurrence != RecurNone {
		next := e.Recurrence.Next(e.Date)
		e.NextDate = &next
	}
	e.UpdatedBy, e.UpdatedAt = actor, now
}

func (s *Service) record(ctx context.Context, actor uuid.UUID, kind activity.Kind, e *Expense, g *group.Group, extra map[string]any) error {
	data := map[string]any{"description": e.Description, "amount": e.Total, "currency": e.Currency}
	involved := e.Participants()
	if g != nil {
		data["groupName"] = g.Name
		involved = append(involved, g.Members...)
	}
	for k, v := range extra {
		data[k] = v
	}
	return s.activity.Record(ctx, activity.Activity{ActorID: actor, GroupID: e.GroupID, ExpenseID: &e.ID, Kind: kind, Data: data, Involved: involved})
}

// existing returns the expense with a client-chosen id when it already
// exists and the actor may see it, so retried creates are idempotent.
func (s *Service) existing(ctx context.Context, actor uuid.UUID, id *uuid.UUID) (*Expense, error) {
	if id == nil || *id == uuid.Nil {
		return nil, nil
	}
	e, _, err := s.load(ctx, actor, *id)
	if err == nil {
		return e, nil
	}
	if !errs.Is(err, errs.KindNotFound) {
		return nil, err
	}
	if _, err := s.repo.Get(ctx, *id); err == nil {
		return nil, errs.Conflict("That expense id is already in use.")
	}
	return nil, nil
}

func newID(id *uuid.UUID) uuid.UUID {
	if id != nil && *id != uuid.Nil {
		return *id
	}
	return uuid.New()
}

// currency is the one a new expense or payment uses when the client leaves
// it out: the group's, or the actor's own outside groups. People never pick
// a currency per expense in the app.
func (s *Service) currency(ctx context.Context, actor uuid.UUID, groupID *uuid.UUID, given string) (string, error) {
	if given != "" {
		return given, nil
	}
	if groupID != nil {
		g, err := s.groups.Get(ctx, *groupID)
		if err != nil {
			return "", err
		}
		return g.DefaultCurrency, nil
	}
	u, err := s.users.Get(ctx, actor)
	if err != nil {
		return "", err
	}
	return u.DefaultCurrency, nil
}

// Create books a new expense.
func (s *Service) Create(ctx context.Context, actor uuid.UUID, in Input) (*Expense, error) {
	if e, err := s.existing(ctx, actor, in.ID); e != nil || err != nil {
		return e, err
	}
	var err error
	if in.Currency, err = s.currency(ctx, actor, in.GroupID, in.Currency); err != nil {
		return nil, err
	}
	now := s.clock.Now()
	n, err := Normalize(in, clock.Today(s.clock))
	if err != nil {
		return nil, err
	}
	g, err := s.authorize(ctx, actor, in.GroupID, n.Shares, in.ReceiptID)
	if err != nil {
		return nil, err
	}
	e := &Expense{ID: newID(in.ID), GroupID: in.GroupID, Kind: KindExpense, CreatedBy: actor, CreatedAt: now}
	s.build(e, n, actor, now)
	err = s.tx.Do(ctx, func(ctx context.Context) error {
		if err := s.repo.Create(ctx, e); err != nil {
			return err
		}
		return s.record(ctx, actor, activity.ExpenseCreated, e, g, nil)
	})
	if err != nil {
		return nil, err
	}
	s.notify(ctx, actor, e, g, false)
	return e, nil
}

// Update replaces an expense. The group cannot change.
func (s *Service) Update(ctx context.Context, actor, id uuid.UUID, in Input) (*Expense, error) {
	e, _, err := s.load(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	if e.DeletedAt != nil {
		return nil, errs.Conflict("Restore the expense before editing it.")
	}
	if e.Kind == KindPayment {
		in.Method, in.Category = MethodExact, PaymentCategory
	}
	in.GroupID = e.GroupID
	if in.Currency == "" {
		in.Currency = e.Currency
	}
	n, err := Normalize(in, e.Date)
	if err != nil {
		return nil, err
	}
	g, err := s.authorize(ctx, actor, e.GroupID, n.Shares, in.ReceiptID)
	if err != nil {
		return nil, err
	}
	now := s.clock.Now()
	s.build(e, n, actor, now)
	err = s.tx.Do(ctx, func(ctx context.Context) error {
		if err := s.repo.Update(ctx, e); err != nil {
			return err
		}
		return s.record(ctx, actor, activity.ExpenseUpdated, e, g, nil)
	})
	if err != nil {
		return nil, err
	}
	s.notify(ctx, actor, e, g, true)
	return e, nil
}

// Get returns an expense the actor may see.
func (s *Service) Get(ctx context.Context, actor, id uuid.UUID) (*Expense, error) {
	e, _, err := s.load(ctx, actor, id)
	return e, err
}

// List returns expenses visible to the actor.
func (s *Service) List(ctx context.Context, f Filter) ([]*Expense, int, error) {
	if f.GroupID != nil {
		g, err := s.groups.Get(ctx, *f.GroupID)
		if err != nil {
			return nil, 0, err
		}
		if !g.HasMember(f.UserID) {
			return nil, 0, errs.NotFound("Group")
		}
	}
	return s.repo.List(ctx, f)
}

func (s *Service) setDeleted(ctx context.Context, actor, id uuid.UUID, deleted bool) (*Expense, error) {
	e, g, err := s.load(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	if (e.DeletedAt != nil) == deleted {
		return e, nil
	}
	if !deleted && g != nil {
		for _, sh := range e.Shares {
			if !g.HasMember(sh.UserID) {
				return nil, errs.Conflict("Someone in this expense has left the group, so it cannot be restored.")
			}
		}
	}
	now := s.clock.Now()
	kind := activity.ExpenseRestored
	e.DeletedAt = nil
	if deleted {
		kind = activity.ExpenseDeleted
		e.DeletedAt = &now
	}
	e.UpdatedBy, e.UpdatedAt = actor, now
	err = s.tx.Do(ctx, func(ctx context.Context) error {
		if err := s.repo.Update(ctx, e); err != nil {
			return err
		}
		return s.record(ctx, actor, kind, e, g, nil)
	})
	return e, err
}

// Delete soft-deletes an expense.
func (s *Service) Delete(ctx context.Context, actor, id uuid.UUID) error {
	_, err := s.setDeleted(ctx, actor, id, true)
	return err
}

// Restore undoes a delete.
func (s *Service) Restore(ctx context.Context, actor, id uuid.UUID) (*Expense, error) {
	return s.setDeleted(ctx, actor, id, false)
}

// PaymentInput records money changing hands.
type PaymentInput struct {
	ID       *uuid.UUID
	GroupID  *uuid.UUID
	From     uuid.UUID
	To       uuid.UUID
	Amount   int64
	Currency string
	Date     string
	Notes    string
}

// RecordPayment books a settle-up payment.
func (s *Service) RecordPayment(ctx context.Context, actor uuid.UUID, in PaymentInput) (*Expense, error) {
	if e, err := s.existing(ctx, actor, in.ID); e != nil || err != nil {
		return e, err
	}
	if in.From == in.To || in.From == uuid.Nil || in.To == uuid.Nil {
		return nil, errs.Validation("toUserId", "Choose two different people.")
	}
	if in.GroupID == nil && actor != in.From && actor != in.To {
		return nil, errs.Validation("fromUserId", "You must be part of a payment outside a group.")
	}
	people, err := s.users.Index(ctx, []uuid.UUID{in.From, in.To})
	if err != nil {
		return nil, err
	}
	if people[in.From] == nil || people[in.To] == nil {
		return nil, errs.Validation("toUserId", "Unknown person.")
	}
	if in.Currency, err = s.currency(ctx, actor, in.GroupID, in.Currency); err != nil {
		return nil, err
	}
	n, err := Normalize(Input{
		GroupID: in.GroupID, Description: "Payment", Currency: in.Currency, Total: in.Amount, Date: in.Date,
		Category: PaymentCategory, Notes: in.Notes, Method: MethodExact,
		Payers: []Payer{{UserID: in.From, Amount: in.Amount}},
		Splits: []SplitInput{{UserID: in.To, Value: strconv.FormatInt(in.Amount, 10)}},
	}, clock.Today(s.clock))
	if err != nil {
		if de, ok := errs.As(err); ok && de.Field == "total" {
			de.Field = "amount"
		}
		return nil, err
	}
	g, err := s.authorize(ctx, actor, in.GroupID, n.Shares, nil)
	if err != nil {
		return nil, err
	}
	now := s.clock.Now()
	e := &Expense{ID: newID(in.ID), GroupID: in.GroupID, Kind: KindPayment, CreatedBy: actor, CreatedAt: now}
	s.build(e, n, actor, now)
	extra := map[string]any{"fromName": people[in.From].Name, "toName": people[in.To].Name}
	err = s.tx.Do(ctx, func(ctx context.Context) error {
		if err := s.repo.Create(ctx, e); err != nil {
			return err
		}
		return s.record(ctx, actor, activity.PaymentCreated, e, g, extra)
	})
	if err != nil {
		return nil, err
	}
	if to := people[in.To]; !ports.IsQuiet(ctx) && to.ID != actor {
		n, ok := to.Notice(user.PaymentReceived, people[in.From].Name+" paid you "+money.Format(e.Total, e.Currency),
			[]string{people[in.From].Name + " recorded a payment of " + money.Format(e.Total, e.Currency) + " to you."}, s.baseURL)
		if ok {
			s.notifier.Notify(ctx, n)
		}
	}
	return e, nil
}

func (s *Service) notify(ctx context.Context, actor uuid.UUID, e *Expense, g *group.Group, updated bool) {
	if ports.IsQuiet(ctx) {
		return
	}
	people, err := s.users.List(ctx, e.Participants())
	if err != nil {
		return
	}
	var actorName string
	for _, u := range people {
		if u.ID == actor {
			actorName = u.Name
		}
	}
	where := ""
	if g != nil {
		where = " in " + g.Name
	}
	for _, u := range people {
		if u.ID == actor {
			continue
		}
		verb, kind := "added", user.ExpenseAdded
		if updated {
			verb, kind = "updated", user.ExpenseUpdated
		}
		net := e.Net(u.ID)
		line := "You are settled on this one."
		if net < 0 {
			line = "Your share: " + money.Format(-net, e.Currency) + "."
		} else if net > 0 {
			line = "You get back " + money.Format(net, e.Currency) + "."
		}
		n, ok := u.Notice(kind, actorName+" "+verb+" “"+e.Description+"”"+where,
			[]string{actorName + " " + verb + " “" + e.Description + "” (" + money.Format(e.Total, e.Currency) + ")" + where + ".", line},
			s.baseURL)
		if ok {
			s.notifier.Notify(ctx, n)
		}
	}
}

// Comments lists comments on an expense.
func (s *Service) Comments(ctx context.Context, actor, id uuid.UUID) ([]*Comment, error) {
	if _, _, err := s.load(ctx, actor, id); err != nil {
		return nil, err
	}
	return s.comments.List(ctx, id)
}

// Comment adds a comment.
func (s *Service) Comment(ctx context.Context, actor, id uuid.UUID, body string) (*Comment, error) {
	e, g, err := s.load(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, errs.Validation("body", "Write a comment first.")
	}
	if utf8.RuneCountInString(body) > 2000 {
		return nil, errs.Validation("body", "Comments are limited to 2000 characters.")
	}
	c := &Comment{ID: uuid.New(), ExpenseID: e.ID, UserID: actor, Body: body, CreatedAt: s.clock.Now()}
	err = s.tx.Do(ctx, func(ctx context.Context) error {
		if err := s.comments.Create(ctx, c); err != nil {
			return err
		}
		return s.record(ctx, actor, activity.CommentCreated, e, g, map[string]any{"comment": truncate(body, 140)})
	})
	return c, err
}

// DeleteComment removes the actor's own comment.
func (s *Service) DeleteComment(ctx context.Context, actor, id uuid.UUID) error {
	c, err := s.comments.Get(ctx, id)
	if err != nil {
		return err
	}
	if c.UserID != actor {
		return errs.Forbidden("You can only delete your own comments.")
	}
	return s.comments.Delete(ctx, id)
}

// RunRecurring creates due copies of recurring expenses. The schedule moves
// to the newest copy so editing the latest expense edits the series.
func (s *Service) RunRecurring(ctx context.Context) (int, error) {
	today := clock.Today(s.clock)
	due, err := s.repo.Due(ctx, today, 100)
	if err != nil {
		return 0, err
	}
	created := 0
	for _, src := range due {
		err := s.tx.Do(ctx, func(ctx context.Context) error {
			now := s.clock.Now()
			copyExp := *src
			copyExp.ID = uuid.New()
			copyExp.Date = *src.NextDate
			next := src.Recurrence.Next(copyExp.Date)
			copyExp.NextDate = &next
			copyExp.CreatedAt, copyExp.UpdatedAt = now, now
			copyExp.ReceiptID = nil
			src.Recurrence, src.NextDate, src.UpdatedAt = RecurNone, nil, now
			if err := s.repo.Update(ctx, src); err != nil {
				return err
			}
			if err := s.repo.Create(ctx, &copyExp); err != nil {
				return err
			}
			var g *group.Group
			if copyExp.GroupID != nil {
				var err error
				if g, err = s.groups.Get(ctx, *copyExp.GroupID); err != nil {
					return err
				}
			}
			return s.record(ctx, copyExp.CreatedBy, activity.ExpenseCreated, &copyExp, g, map[string]any{"recurring": true})
		})
		if err != nil {
			return created, err
		}
		created++
	}
	return created, nil
}

func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}
