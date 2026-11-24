package reminder

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/activity"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/balance"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/group"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/user"
	"github.com/Hostworthy/SplitSpends/packages/core/ports"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/money"
	"github.com/google/uuid"
)

type memUsers map[uuid.UUID]*user.User

func (m memUsers) Get(_ context.Context, id uuid.UUID) (*user.User, error) {
	if u := m[id]; u != nil {
		return u, nil
	}
	return nil, errs.NotFound("User")
}
func (m memUsers) FindByEmail(context.Context, string) (*user.User, error) { return nil, nil }
func (m memUsers) List(_ context.Context, ids []uuid.UUID) ([]*user.User, error) {
	var out []*user.User
	for _, id := range ids {
		if u := m[id]; u != nil {
			out = append(out, u)
		}
	}
	return out, nil
}
func (m memUsers) Create(_ context.Context, u *user.User) error { m[u.ID] = u; return nil }
func (m memUsers) Update(_ context.Context, u *user.User) error { m[u.ID] = u; return nil }
func (m memUsers) Count(context.Context) (int, error)           { return len(m), nil }

// ledger holds one group's entries plus non-group entries.
type ledger struct {
	group    group.Group
	entries  []balance.Entry
	nonGroup []balance.Entry
}

func (l *ledger) Group(context.Context, uuid.UUID) ([]balance.Entry, error) { return l.entries, nil }
func (l *ledger) Groups(_ context.Context, userID uuid.UUID) ([]balance.GroupRef, error) {
	if !l.group.HasMember(userID) {
		return nil, nil
	}
	return []balance.GroupRef{{ID: l.group.ID, Name: l.group.Name, Simplify: true}}, nil
}
func (l *ledger) GroupsEntries(_ context.Context, ids []uuid.UUID) (map[uuid.UUID][]balance.Entry, error) {
	out := map[uuid.UUID][]balance.Entry{}
	for _, id := range ids {
		if id == l.group.ID {
			out[id] = l.entries
		}
	}
	return out, nil
}
func (l *ledger) NonGroup(context.Context, uuid.UUID) ([]balance.Entry, error) {
	return l.nonGroup, nil
}
func (l *ledger) Member(_ context.Context, actor, id uuid.UUID) (*group.Group, error) {
	if id != l.group.ID || !l.group.HasMember(actor) {
		return nil, errs.NotFound("Group")
	}
	g := l.group
	return &g, nil
}

type links map[[2]uuid.UUID]bool

func (l links) Connected(_ context.Context, a, b uuid.UUID) (bool, error) {
	return l[[2]uuid.UUID{a, b}] || l[[2]uuid.UUID{b, a}], nil
}

type key struct {
	from, to, group uuid.UUID
}

// memReminders mirrors the database rule: one reminder per key after since.
type memReminders map[key]time.Time

func (m memReminders) Claim(_ context.Context, r *Reminder, since time.Time) (bool, error) {
	k := key{from: r.FromID, to: r.ToID}
	if r.GroupID != nil {
		k.group = *r.GroupID
	}
	if last, ok := m[k]; ok && last.After(since) {
		return false, nil
	}
	m[k] = r.SentAt
	return true, nil
}

type memActivity struct{ list []*activity.Activity }

func (m *memActivity) Create(_ context.Context, a *activity.Activity) error {
	m.list = append(m.list, a)
	return nil
}
func (m *memActivity) List(context.Context, activity.Filter) ([]*activity.Activity, int, error) {
	return m.list, len(m.list), nil
}

type outbox struct{ sent []ports.Notification }

func (o *outbox) LoginCode(context.Context, string, string, time.Duration) error { return nil }
func (o *outbox) Invite(context.Context, string, string) error                   { return nil }
func (o *outbox) Notify(_ context.Context, n ports.Notification)                 { o.sent = append(o.sent, n) }

type noTx struct{}

func (noTx) Do(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

type clockAt struct{ at time.Time }

func (c *clockAt) Now() time.Time { return c.at }

type fixture struct {
	svc               *Service
	ada, ben, sam, xi *user.User
	group             *group.Group
	ledger            *ledger
	mail              *outbox
	feed              *memActivity
	clock             *clockAt
}

func person(t *testing.T, users memUsers, email, name string, status user.Status) *user.User {
	t.Helper()
	u, err := user.New(email, name, status, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	users[u.ID] = u
	return u
}

// setup: Ada paid 90 EUR for Flat, split with Ben and Sam (each owes 30).
// Sam is a placeholder without email. Xi is a stranger.
func setup(t *testing.T) *fixture {
	t.Helper()
	users := memUsers{}
	f := &fixture{mail: &outbox{}, feed: &memActivity{}, clock: &clockAt{at: time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)}}
	f.ada = person(t, users, "ada@example.com", "Ada", user.StatusActive)
	f.ben = person(t, users, "ben@example.com", "Ben", user.StatusInvited)
	f.xi = person(t, users, "xi@example.com", "Xi", user.StatusActive)
	sam, err := user.NewPlaceholder(nil, "Sam", f.ada.ID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	users[sam.ID] = sam
	f.sam = sam
	f.group = &group.Group{ID: uuid.New(), Name: "Flat", Members: []uuid.UUID{f.ada.ID, f.ben.ID, f.sam.ID}}
	f.ledger = &ledger{group: *f.group, entries: []balance.Entry{{GroupID: &f.group.ID, Currency: "EUR", Shares: []balance.Share{
		{UserID: f.ada.ID, Paid: 9000, Owed: 3000}, {UserID: f.ben.ID, Owed: 3000}, {UserID: f.sam.ID, Owed: 3000},
	}}}}
	f.svc = NewService(Deps{Repo: memReminders{}, Groups: f.ledger, Connections: links{{f.ada.ID, f.ben.ID}: true},
		Users: user.NewService(users, nil, f.clock), Balances: balance.NewService(f.ledger),
		Activity: activity.NewService(f.feed, f.clock), Notifier: f.mail, Tx: noTx{}, Clock: f.clock, BaseURL: "https://split.test"})
	return f
}

func TestSendEmailsWhatIsOwed(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	r, err := f.svc.Send(ctx, f.ada.ID, f.ben.ID, &f.group.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !r.Emailed || r.Owed["EUR"] != 3000 {
		t.Fatalf("unexpected reminder: %+v", r)
	}
	if len(f.mail.sent) != 1 {
		t.Fatalf("want one email, got %d", len(f.mail.sent))
	}
	n := f.mail.sent[0]
	body := strings.Join(n.Lines, " ")
	if n.To != "ben@example.com" || !strings.Contains(body, "You owe Ada €30.00") || !strings.Contains(body, "in Flat") {
		t.Fatalf("unexpected email: %+v", n)
	}
	if n.Link != "https://split.test/groups/"+f.group.ID.String() {
		t.Fatalf("link should open the group, got %s", n.Link)
	}
	if len(f.feed.list) != 1 {
		t.Fatalf("want one activity, got %d", len(f.feed.list))
	}
	a := f.feed.list[0]
	if a.Kind != activity.ReminderSent || len(a.Involved) != 2 || activity.Describe(a, "Ada") != "Ada reminded Ben to settle up in Flat" {
		t.Fatalf("activity must be visible to both: %+v", a)
	}
}

func TestSendIsRateLimitedPerScope(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	if _, err := f.svc.Send(ctx, f.ada.ID, f.ben.ID, &f.group.ID); err != nil {
		t.Fatal(err)
	}
	f.clock.at = f.clock.at.Add(23 * time.Hour)
	if _, err := f.svc.Send(ctx, f.ada.ID, f.ben.ID, &f.group.ID); !errs.Is(err, errs.KindRateLimited) {
		t.Fatalf("second reminder within 24 hours must be limited, got %v", err)
	}
	// The overall scope is separate from the group scope.
	if _, err := f.svc.Send(ctx, f.ada.ID, f.ben.ID, nil); err != nil {
		t.Fatalf("overall reminder is a different scope: %v", err)
	}
	f.clock.at = f.clock.at.Add(2 * time.Hour)
	if _, err := f.svc.Send(ctx, f.ada.ID, f.ben.ID, &f.group.ID); err != nil {
		t.Fatalf("a day later the reminder is allowed again: %v", err)
	}
	if len(f.mail.sent) != 3 {
		t.Fatalf("want three emails, got %d", len(f.mail.sent))
	}
}

func TestSendNeedsSomethingOwed(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	// Ben owes Ada, not the other way around.
	if _, err := f.svc.Send(ctx, f.ben.ID, f.ada.ID, &f.group.ID); !errs.Is(err, errs.KindConflict) {
		t.Fatalf("cannot remind someone you owe, got %v", err)
	}
	if _, err := f.svc.Send(ctx, f.ada.ID, f.ada.ID, nil); !errs.Is(err, errs.KindValidation) {
		t.Fatalf("cannot remind yourself, got %v", err)
	}
	if _, err := f.svc.Send(ctx, f.ada.ID, f.xi.ID, nil); !errs.Is(err, errs.KindNotFound) {
		t.Fatalf("strangers must look unknown, got %v", err)
	}
	if _, err := f.svc.Send(ctx, f.xi.ID, f.ada.ID, &f.group.ID); !errs.Is(err, errs.KindValidation) {
		t.Fatalf("non-members cannot use the group, got %v", err)
	}
	if len(f.mail.sent) != 0 || len(f.feed.list) != 0 {
		t.Fatal("failed reminders must not send or record anything")
	}
}

func TestSendNeedsAnEmail(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	_, err := f.svc.Send(ctx, f.ada.ID, f.sam.ID, &f.group.ID)
	if !errs.Is(err, errs.KindConflict) || !strings.Contains(err.Error(), "no email") {
		t.Fatalf("placeholders cannot receive reminders, got %v", err)
	}
}

func TestSendRespectsThePreference(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	f.ben.Notifications.Reminders = false
	r, err := f.svc.Send(ctx, f.ada.ID, f.ben.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Emailed || len(f.mail.sent) != 0 {
		t.Fatal("no email when reminders are turned off")
	}
	if len(f.feed.list) != 1 {
		t.Fatal("the reminder still shows in the activity feed")
	}
	if f.feed.list[0].GroupID != nil {
		t.Fatal("an overall reminder has no group")
	}
}

func TestOverallLinkOpensTheSender(t *testing.T) {
	ctx := context.Background()
	f := setup(t)
	if _, err := f.svc.Send(ctx, f.ada.ID, f.ben.ID, nil); err != nil {
		t.Fatal(err)
	}
	if got := f.mail.sent[0].Link; got != "https://split.test/friends/"+f.ada.ID.String() {
		t.Fatalf("unexpected link %s", got)
	}
}

func TestOwedAndAmounts(t *testing.T) {
	owed := Owed(money.Balances{"EUR": 1250, "USD": -500, "GBP": 300, "JPY": 0})
	if len(owed) != 2 || owed["EUR"] != 1250 || owed["GBP"] != 300 {
		t.Fatalf("only positive balances are owed: %v", owed)
	}
	if got := Amounts(owed); got != "€12.50 and £3.00" {
		t.Fatalf("got %q", got)
	}
	if got := Amounts(money.Balances{"EUR": 100, "GBP": 200, "USD": 300}); got != "€1.00, £2.00 and $3.00" {
		t.Fatalf("got %q", got)
	}
	m := Compose("Ada", "", money.Balances{"EUR": 100}, "https://x")
	if m.Subject != "Ada sent you a reminder to settle up" || strings.Contains(strings.Join(m.Lines, " "), " in ") {
		t.Fatalf("overall message must not name a group: %+v", m)
	}
}

func TestReminderChannels(t *testing.T) {
	m := Message{Subject: "Reminder", Lines: []string{"You owe Ada $10.00."}, Link: "https://x"}
	off := false
	active := &user.User{ID: uuid.New(), Name: "Ben", Email: "ben@example.com", Status: user.StatusActive, Notifications: user.DefaultNotifications()}
	active.Notifications.Apply(user.NotificationsPatch{SwitchesPatch: user.SwitchesPatch{Reminders: &off}})
	n, ok := m.Notification(active)
	if !ok || n.To != "" || n.UserID != active.ID || n.Kind != "reminders" {
		t.Fatalf("email off, push on: want push only, got %+v %v", n, ok)
	}
	invited := &user.User{ID: uuid.New(), Name: "Cleo", Email: "cleo@example.com", Status: user.StatusInvited, Notifications: user.DefaultNotifications()}
	if n, ok := m.Notification(invited); !ok || n.To != "cleo@example.com" || n.UserID != uuid.Nil {
		t.Fatalf("invited people get the email only, got %+v %v", n, ok)
	}
	invited.Notifications.Apply(user.NotificationsPatch{SwitchesPatch: user.SwitchesPatch{Reminders: &off}})
	if _, ok := m.Notification(invited); ok {
		t.Fatal("reminders turned off must send nothing")
	}
}
