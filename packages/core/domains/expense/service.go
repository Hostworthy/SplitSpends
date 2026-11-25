package expense

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
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
	// Hold keeps the answer for a and b until the transaction in ctx ends.
	Hold(ctx context.Context, a, b uuid.UUID) error
}

// Groups is what expenses need from the group context. Member is the one
// membership check; it returns errs.NotFound unless actor is a current member.
type Groups interface {
	Member(ctx context.Context, actor, id uuid.UUID) (*group.Group, error)
	// Lookup returns a group without asking who wants it, for work done on
	// nobody's behalf.
	Lookup(ctx context.Context, id uuid.UUID) (*group.Group, error)
	// Hold keeps the group's members as they are until the transaction in
	// ctx ends. It returns errs.NotFound when the group is missing.
	Hold(ctx context.Context, id uuid.UUID) error
}

// Users reads the people expenses name.
type Users interface {
	Get(ctx context.Context, id uuid.UUID) (*user.User, error)
	List(ctx context.Context, ids []uuid.UUID) ([]*user.User, error)
	// Index is List keyed by id; ids that do not exist are missing.
	Index(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]*user.User, error)
}

// Recorder adds to the activity feed.
type Recorder interface {
	Record(ctx context.Context, a activity.Activity) error
}

// Service implements expense, payment and comment use cases.
type Service struct {
	repo        Repository
	comments    CommentRepository
	groups      Groups
	connections Connections
	receipts    Receipts
	catalog     Catalog
	users       Users
	activity    Recorder
	notifier    ports.Notifier
	tx          ports.Tx
	clock       clock.Clock
	log         *slog.Logger
	baseURL     string
}

// Deps groups Service dependencies.
type Deps struct {
	Repo        Repository
	Comments    CommentRepository
	Groups      Groups
	Connections Connections
	Receipts    Receipts
	Catalog     Catalog
	Users       Users
	Activity    Recorder
	Notifier    ports.Notifier
	Tx          ports.Tx
	Clock       clock.Clock
	Log         *slog.Logger
	BaseURL     string
}

// NewService builds a Service.
func NewService(d Deps) *Service {
	return &Service{repo: d.Repo, comments: d.Comments, groups: d.Groups, connections: d.Connections, receipts: d.Receipts,
		catalog: d.Catalog, users: d.Users, activity: d.Activity, notifier: d.Notifier, tx: d.Tx, clock: d.Clock, log: d.Log, baseURL: d.BaseURL}
}

// load fetches an expense the actor may see, plus its group when the actor is
// a current member of it.
func (s *Service) load(ctx context.Context, actor, id uuid.UUID) (*Expense, *group.Group, error) {
	e, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	return s.visible(ctx, actor, e)
}

// lock is load for a change, inside the caller's transaction. The row stays
// locked until the change is saved, so the checks made on it still hold and
// concurrent changes apply one after the other instead of overwriting each
// other. The group is held first, in the order deleting a group locks the
// two, so its members cannot change meanwhile and the two never deadlock.
func (s *Service) lock(ctx context.Context, actor, id uuid.UUID) (*Expense, *group.Group, error) {
	e, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if e.GroupID != nil {
		if err := s.groups.Hold(ctx, *e.GroupID); err != nil {
			if errs.Is(err, errs.KindNotFound) {
				return nil, nil, errs.NotFound("Expense")
			}
			return nil, nil, err
		}
	}
	if e, err = s.repo.Lock(ctx, id); err != nil {
		return nil, nil, err
	}
	return s.visible(ctx, actor, e)
}

// visible returns e if the actor may see it, and its group when the actor is
// a current member of it (nil otherwise).
func (s *Service) visible(ctx context.Context, actor uuid.UUID, e *Expense) (*Expense, *group.Group, error) {
	var g *group.Group
	if e.GroupID != nil {
		var err error
		if g, err = s.groups.Member(ctx, actor, *e.GroupID); err != nil && !errs.Is(err, errs.KindNotFound) {
			return nil, nil, err
		}
	}
	if !canRead(actor, e, g != nil) {
		return nil, nil, errs.NotFound("Expense")
	}
	return e, g, nil
}

// canRead reports whether actor may see e, given whether they are a current
// member of its group. People who left a group keep seeing the expenses they
// were part of. Repositories filter expense lists by the same rule in SQL,
// so the two change together.
func canRead(actor uuid.UUID, e *Expense, member bool) bool {
	return (e.GroupID != nil && member) || e.Involves(actor)
}

// canWrite reports whether actor may change, delete, restore or comment on e,
// given whether they are a current member of its group. Outside groups
// everyone in the expense may; inside one only current members may, since a
// change moves the balances of everyone still in it.
func canWrite(actor uuid.UUID, e *Expense, member bool) bool {
	if e.GroupID == nil {
		return e.Involves(actor)
	}
	return member
}

// editable checks that the actor may change an expense they can see; g is
// what load or lock returned with it.
func editable(actor uuid.UUID, e *Expense, g *group.Group) error {
	if !canWrite(actor, e, g != nil) {
		return errs.NotFound("Expense")
	}
	return nil
}

// authorize checks that the actor may book an expense with these participants.
// Inside Tx.Do it holds the group, or the connections between the people
// outside groups, until the transaction ends, so nobody leaves or stops being
// a friend between the check and the save; outside one it only checks. Only
// the uploader may attach a receipt, but an edit may keep the one the expense
// already has (kept is nil for a new expense).
func (s *Service) authorize(ctx context.Context, actor uuid.UUID, groupID *uuid.UUID, shares []Share, receiptID, kept *uuid.UUID) (*group.Group, error) {
	if receiptID != nil && (kept == nil || *receiptID != *kept) {
		ok, err := s.receipts.Owned(ctx, *receiptID, actor)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, errs.Validation("receiptId", "Receipt not found.")
		}
	}
	if groupID != nil {
		err := s.groups.Hold(ctx, *groupID)
		var g *group.Group
		if err == nil {
			g, err = s.groups.Member(ctx, actor, *groupID)
		}
		if err != nil {
			return nil, unknownGroup(err)
		}
		if !members(g, shares) {
			return nil, errs.Validation("splits", "Everyone in the expense must be a member of the group.")
		}
		return g, nil
	}
	ok, err := s.connected(ctx, actor, shares)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errs.Validation("splits", "You can only split non-group expenses with friends.")
	}
	if ok, err = s.linked(ctx, shares); err != nil {
		return nil, err
	}
	if !ok {
		return nil, errs.Validation("splits", "Outside a group, everyone who owes money must be friends with the people they owe it to.")
	}
	if !slices.ContainsFunc(shares, func(sh Share) bool { return sh.UserID == actor }) {
		return nil, errs.Validation("splits", "You must be part of an expense outside a group.")
	}
	return nil, nil
}

// unknownGroup answers a group the actor cannot book into, missing or not
// theirs, as a mistake in the request.
func unknownGroup(err error) error {
	if errs.Is(err, errs.KindNotFound) {
		return errs.Validation("groupId", "Group not found.")
	}
	return err
}

// members reports whether everyone in shares still belongs to g. Booking,
// deleting, restoring and repeating an expense all require it.
func members(g *group.Group, shares []Share) bool {
	for _, sh := range shares {
		if !g.HasMember(sh.UserID) {
			return false
		}
	}
	return true
}

// connected reports whether actor may share an expense outside groups with
// everyone in shares.
func (s *Service) connected(ctx context.Context, actor uuid.UUID, shares []Share) (bool, error) {
	for _, sh := range shares {
		if sh.UserID == actor {
			continue
		}
		ok, err := s.pair(ctx, actor, sh.UserID)
		if err != nil || !ok {
			return false, err
		}
	}
	return true, nil
}

// linked reports whether everyone who gets money back from an expense
// outside groups is connected to everyone who owes it, since those are the
// pairs the expense leaves owing each other.
func (s *Service) linked(ctx context.Context, shares []Share) (bool, error) {
	for _, c := range shares {
		if c.Paid <= c.Owed {
			continue
		}
		for _, d := range shares {
			if d.Paid >= d.Owed {
				continue
			}
			ok, err := s.pair(ctx, c.UserID, d.UserID)
			if err != nil || !ok {
				return false, err
			}
		}
	}
	return true, nil
}

// pair reports whether a and b are connected and keeps them so until the
// transaction in ctx ends.
func (s *Service) pair(ctx context.Context, a, b uuid.UUID) (bool, error) {
	if err := s.connections.Hold(ctx, a, b); err != nil {
		return false, err
	}
	return s.connections.Connected(ctx, a, b)
}

// category checks that the actor may file an expense under a custom
// category: their own live one, or the one the expense already has (kept is
// empty for new expenses), so editing never fails because a category was
// deleted or belongs to another member.
func (s *Service) category(ctx context.Context, actor uuid.UUID, id, kept string) error {
	if !CustomCategory(id) || id == kept {
		return nil
	}
	ok, err := s.catalog.Usable(ctx, actor, id)
	if err != nil {
		return err
	}
	if !ok {
		return errs.Validation("category", "Unknown category.")
	}
	return nil
}

func (s *Service) build(e *Expense, n *Normalized, actor uuid.UUID, now time.Time) {
	// Editing the expense that carries a series keeps the series' day unless
	// the date changes, so an edit of the copy booked on 28 February still
	// returns to the 31st.
	if e.AnchorDay == 0 || !n.Date.Equal(e.Date) {
		e.AnchorDay = n.Date.Day()
	}
	e.Description, e.Category, e.Currency, e.Total = n.Description, n.Category, n.Currency, n.Total
	e.Date, e.Notes, e.Method, e.Shares = n.Date, n.Notes, n.Method, n.Shares
	e.Items, e.Tax, e.Tip, e.ReceiptID, e.Recurrence = n.Items, n.Tax, n.Tip, n.ReceiptID, n.Recurrence
	e.NextDate = nil
	if e.Recurrence == RecurNone {
		e.AnchorDay = 0
	} else {
		next := e.Recurrence.Next(e.Date, e.AnchorDay)
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
// exists and the actor may see it, so retried creates are idempotent. A retry
// asks for the same kind of expense in the same group; an id used for
// anything else is a conflict.
func (s *Service) existing(ctx context.Context, actor uuid.UUID, id *uuid.UUID, kind Kind, groupID *uuid.UUID) (*Expense, error) {
	if id == nil || *id == uuid.Nil {
		return nil, nil
	}
	e, err := s.repo.Get(ctx, *id)
	if errs.Is(err, errs.KindNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if _, _, err := s.visible(ctx, actor, e); err != nil {
		if errs.Is(err, errs.KindNotFound) {
			return nil, errs.Taken()
		}
		return nil, err
	}
	if e.Kind != kind || (e.GroupID == nil) != (groupID == nil) || (groupID != nil && *e.GroupID != *groupID) {
		return nil, errs.Taken()
	}
	return e, nil
}

// insert authorizes and saves a new expense and its activity, and returns its
// group. When a request with the same client-chosen id saved its expense
// first, insert returns that one instead; it returns nil when e was saved.
func (s *Service) insert(ctx context.Context, actor uuid.UUID, e *Expense, kind activity.Kind, extra map[string]any) (*Expense, *group.Group, error) {
	var g *group.Group
	err := s.tx.Do(ctx, func(ctx context.Context) error {
		var err error
		if g, err = s.authorize(ctx, actor, e.GroupID, e.Shares, e.ReceiptID, nil); err != nil {
			return err
		}
		if err := s.repo.Create(ctx, e); err != nil {
			return err
		}
		return s.record(ctx, actor, kind, e, g, extra)
	})
	if errs.Is(err, errs.KindConflict) {
		if prior, err := s.existing(ctx, actor, &e.ID, e.Kind, e.GroupID); prior != nil || err != nil {
			return prior, nil, err
		}
	}
	return nil, g, err
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
		g, err := s.groups.Member(ctx, actor, *groupID)
		if err != nil {
			return "", unknownGroup(err)
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
	if e, err := s.existing(ctx, actor, in.ID, KindExpense, in.GroupID); e != nil || err != nil {
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
	if err := s.category(ctx, actor, n.Category, ""); err != nil {
		return nil, err
	}
	e := &Expense{ID: newID(in.ID), GroupID: in.GroupID, Kind: KindExpense, CreatedBy: actor, CreatedAt: now}
	s.build(e, n, actor, now)
	prior, g, err := s.insert(ctx, actor, e, activity.ExpenseCreated, nil)
	if prior != nil || err != nil {
		return prior, err
	}
	s.notify(ctx, actor, e, g, false)
	return e, nil
}

// Update replaces an expense. The group cannot change.
func (s *Service) Update(ctx context.Context, actor, id uuid.UUID, in Input) (*Expense, error) {
	var e *Expense
	var g *group.Group
	err := s.tx.Do(ctx, func(ctx context.Context) error {
		var err error
		if e, g, err = s.lock(ctx, actor, id); err != nil {
			return err
		}
		if err := editable(actor, e, g); err != nil {
			return err
		}
		if e.DeletedAt != nil {
			return errs.Conflict("Restore the expense before editing it.")
		}
		// Clients see updatedAt to the second, so that is what is compared.
		if in.UpdatedAt != nil && !e.UpdatedAt.Truncate(time.Second).Equal(in.UpdatedAt.Truncate(time.Second)) {
			return errs.Conflict("Someone else changed this expense. Reload it and try again.")
		}
		in.GroupID = e.GroupID
		if in.Currency == "" {
			in.Currency = e.Currency
		}
		if in.Date == "" {
			in.Date = e.Date.Format(time.DateOnly)
		}
		var n *Normalized
		if e.Kind == KindPayment {
			p, err := repayment(in)
			if err != nil {
				return err
			}
			n, err = payment(p, clock.Today(s.clock))
			if err != nil {
				return err
			}
		} else if n, err = Normalize(in, clock.Today(s.clock)); err != nil {
			return err
		}
		if err := s.category(ctx, actor, n.Category, e.Category); err != nil {
			return err
		}
		if g, err = s.authorize(ctx, actor, e.GroupID, n.Shares, in.ReceiptID, e.ReceiptID); err != nil {
			return err
		}
		s.build(e, n, actor, s.clock.Now())
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

// List returns the expenses the actor may see. The repository applies
// canRead in its query; a group filter needs a current member.
func (s *Service) List(ctx context.Context, f Filter) ([]*Expense, int, error) {
	if f.GroupID != nil {
		if _, err := s.groups.Member(ctx, f.UserID, *f.GroupID); err != nil {
			return nil, 0, err
		}
	}
	return s.repo.List(ctx, f)
}

func (s *Service) setDeleted(ctx context.Context, actor, id uuid.UUID, deleted bool) (*Expense, error) {
	var e *Expense
	err := s.tx.Do(ctx, func(ctx context.Context) error {
		var g *group.Group
		var err error
		if e, g, err = s.lock(ctx, actor, id); err != nil {
			return err
		}
		if err := editable(actor, e, g); err != nil {
			return err
		}
		if (e.DeletedAt != nil) == deleted {
			return nil
		}
		// Leaving requires a settled balance, so deleting or restoring an
		// expense of someone who has left would give them a balance in a
		// group they can no longer act in.
		if g != nil && !members(g, e.Shares) {
			if deleted {
				return errs.Conflict("Someone in this expense has left the group, so it cannot be deleted.")
			}
			return errs.Conflict("Someone in this expense has left the group, so it cannot be restored.")
		}
		now := s.clock.Now()
		kind := activity.ExpenseRestored
		e.DeletedAt = nil
		if deleted {
			kind = activity.ExpenseDeleted
			e.DeletedAt = &now
		}
		e.UpdatedBy, e.UpdatedAt = actor, now
		if err := s.repo.Update(ctx, e); err != nil {
			return err
		}
		return s.record(ctx, actor, kind, e, g, nil)
	})
	if err != nil {
		return nil, err
	}
	return e, nil
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

// RecordPayment books a settle-up payment.
func (s *Service) RecordPayment(ctx context.Context, actor uuid.UUID, in PaymentInput) (*Expense, error) {
	if e, err := s.existing(ctx, actor, in.ID, KindPayment, in.GroupID); e != nil || err != nil {
		return e, err
	}
	var err error
	if in.Currency, err = s.currency(ctx, actor, in.GroupID, in.Currency); err != nil {
		return nil, err
	}
	n, err := payment(in, clock.Today(s.clock))
	if err != nil {
		return nil, err
	}
	if in.GroupID == nil && actor != in.From && actor != in.To {
		return nil, errs.Validation("fromUserId", "You must be part of a payment outside a group.")
	}
	// Authorizing first answers an id that does not exist like a stranger's.
	// insert authorizes again, holding the people until the payment is saved.
	if _, err := s.authorize(ctx, actor, in.GroupID, n.Shares, nil, nil); err != nil {
		return nil, err
	}
	people, err := s.users.Index(ctx, []uuid.UUID{in.From, in.To})
	if err != nil {
		return nil, err
	}
	if people[in.From] == nil || people[in.To] == nil {
		return nil, errs.Validation("toUserId", "Unknown person.")
	}
	now := s.clock.Now()
	e := &Expense{ID: newID(in.ID), GroupID: in.GroupID, Kind: KindPayment, CreatedBy: actor, CreatedAt: now}
	s.build(e, n, actor, now)
	extra := map[string]any{"fromName": people[in.From].Name, "toName": people[in.To].Name}
	if prior, _, err := s.insert(ctx, actor, e, activity.PaymentCreated, extra); prior != nil || err != nil {
		return prior, err
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
	// The expense is saved already, so a failed lookup is logged rather
	// than returned.
	people, err := s.users.List(ctx, e.Participants())
	if err != nil {
		s.log.ErrorContext(ctx, "expense.notify.failed", "expense", e.ID, "error", err)
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
	var notes []ports.Notification
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
			notes = append(notes, n)
		}
	}
	s.notifier.Notify(ctx, notes...)
}

// Comments returns a page of the comments on an expense, oldest first, and
// how many it has. A limit of 0 returns them all.
func (s *Service) Comments(ctx context.Context, actor, id uuid.UUID, limit, offset int) ([]*Comment, int, error) {
	if _, _, err := s.load(ctx, actor, id); err != nil {
		return nil, 0, err
	}
	return s.comments.List(ctx, id, limit, offset)
}

// Comment adds a comment.
func (s *Service) Comment(ctx context.Context, actor, id uuid.UUID, body string) (*Comment, error) {
	e, g, err := s.load(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	if err := editable(actor, e, g); err != nil {
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
		// Locked so that comments posted at the same time cannot pass the
		// limit together.
		if _, err := s.repo.Lock(ctx, e.ID); err != nil {
			return err
		}
		n, err := s.comments.Count(ctx, e.ID)
		if err != nil {
			return err
		}
		if n >= MaxComments {
			return errs.Conflict(fmt.Sprintf("This expense already has %d comments, the most one expense can hold.", MaxComments))
		}
		if err := s.comments.Create(ctx, c); err != nil {
			return err
		}
		return s.record(ctx, actor, activity.CommentCreated, e, g, map[string]any{"comment": truncate(body, 140)})
	})
	return c, err
}

// DeleteComment removes the actor's own comment. A comment on an expense the
// actor cannot see is not found, like a comment that does not exist.
func (s *Service) DeleteComment(ctx context.Context, actor, id uuid.UUID) error {
	c, err := s.comments.Get(ctx, id)
	if err != nil {
		return err
	}
	if _, _, err := s.load(ctx, actor, c.ExpenseID); err != nil {
		if errs.Is(err, errs.KindNotFound) {
			return errs.NotFound("Comment")
		}
		return err
	}
	if c.UserID != actor {
		return errs.Forbidden("You can only delete your own comments.")
	}
	return s.comments.Delete(ctx, id)
}

// RunRecurring creates due copies of recurring expenses. The schedule moves
// to the newest copy so editing the latest expense edits the series. A series
// that fails is skipped, so it never holds up the others; it stays due and is
// tried again on the next run. The error joins a *Skipped for each one.
func (s *Service) RunRecurring(ctx context.Context) (int, error) {
	today := clock.Today(s.clock)
	due, err := s.repo.Due(ctx, today, 100)
	if err != nil {
		return 0, err
	}
	created := 0
	var skipped []error
	for _, d := range due {
		var n int
		err := s.tx.Do(ctx, func(ctx context.Context) error {
			var err error
			n, err = s.repeat(ctx, d, today)
			return err
		})
		if err != nil {
			skipped = append(skipped, &Skipped{ID: d.ID, Err: err})
			continue
		}
		created += n
	}
	return created, errors.Join(skipped...)
}

// Skipped is a recurring expense RunRecurring could not repeat.
type Skipped struct {
	ID  uuid.UUID
	Err error
}

func (s *Skipped) Error() string { return "expense.recurring.skipped: " + s.Err.Error() }

func (s *Skipped) Unwrap() error { return s.Err }

// repeat books the missed occurrences of one series inside the caller's
// transaction and returns how many it booked. The row is claimed first, so
// runners working at the same time book each occurrence once, and the copies
// come from the stored row, so an edit saved after Due read it is kept. The
// group is held before the claim, like lock does, so its members stay as
// checked until the copies are saved.
func (s *Service) repeat(ctx context.Context, due *Expense, today time.Time) (int, error) {
	if due.GroupID != nil {
		if err := s.groups.Hold(ctx, *due.GroupID); err != nil {
			// A deleted group took its expenses with it.
			if errs.Is(err, errs.KindNotFound) {
				return 0, nil
			}
			return 0, err
		}
	}
	src, err := s.repo.Claim(ctx, due.ID, today)
	if err != nil || src == nil {
		return 0, err
	}
	now := s.clock.Now()
	if err := s.repo.EndSeries(ctx, src.ID, now); err != nil {
		return 0, err
	}
	var g *group.Group
	var ok bool
	if src.GroupID != nil {
		if g, err = s.groups.Lookup(ctx, *src.GroupID); err != nil {
			return 0, err
		}
		ok = members(g, src.Shares)
	} else if ok, err = s.connected(ctx, src.CreatedBy, src.Shares); err != nil {
		return 0, err
	} else if ok {
		if ok, err = s.linked(ctx, src.Shares); err != nil {
			return 0, err
		}
	}
	// Copies never bill someone who has left the group or is no longer
	// connected; the series ends and the activity feed says why.
	if !ok {
		return 0, s.record(ctx, src.CreatedBy, activity.ExpenseStopped, src, g, nil)
	}
	day := src.AnchorDay
	if day == 0 {
		day = src.Date.Day()
	}
	dates, next := src.Recurrence.Missed(*src.NextDate, today, day)
	for i, date := range dates {
		c := *src
		c.ID, c.Date, c.ReceiptID = uuid.New(), date, nil
		c.CreatedAt, c.UpdatedAt = now, now
		c.Recurrence, c.NextDate, c.AnchorDay = RecurNone, nil, 0
		if i == len(dates)-1 {
			c.Recurrence, c.NextDate, c.AnchorDay = src.Recurrence, &next, day
		}
		if err := s.repo.Create(ctx, &c); err != nil {
			return 0, err
		}
		if err := s.record(ctx, c.CreatedBy, activity.ExpenseCreated, &c, g, map[string]any{"recurring": true}); err != nil {
			return 0, err
		}
	}
	return len(dates), nil
}

func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}
