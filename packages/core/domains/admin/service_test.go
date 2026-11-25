package admin

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/auth"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/user"
	"github.com/Hostworthy/SplitSpends/packages/core/ports"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/clock"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/google/uuid"
)

type memUsers struct {
	byID map[uuid.UUID]*user.User
	// fail makes Staff fail.
	fail error
}

func (m *memUsers) Get(_ context.Context, id uuid.UUID) (*user.User, error) {
	u, ok := m.byID[id]
	if !ok {
		return nil, errs.NotFound("User")
	}
	cp := *u
	return &cp, nil
}
func (m *memUsers) FindByEmail(context.Context, string) (*user.User, error) { return nil, nil }
func (m *memUsers) List(context.Context, []uuid.UUID) ([]*user.User, error) { return nil, nil }
func (m *memUsers) Create(_ context.Context, u *user.User) error            { m.byID[u.ID] = u; return nil }
func (m *memUsers) Update(_ context.Context, u *user.User) error            { m.byID[u.ID] = u; return nil }
func (m *memUsers) Count(context.Context) (int, error)                      { return len(m.byID), nil }
func (m *memUsers) Lock(context.Context) error                              { return nil }
func (m *memUsers) Placeholders(context.Context, uuid.UUID) (int, error)    { return 0, nil }
func (m *memUsers) Staff(context.Context) ([]*user.User, error) {
	if m.fail != nil {
		return nil, m.fail
	}
	var staff []*user.User
	for _, u := range m.byID {
		if u.CanApprove() {
			staff = append(staff, u)
		}
	}
	return staff, nil
}
func (m *memUsers) Accounts(context.Context, string, string, int, int) ([]*user.User, int, error) {
	return nil, 0, nil
}
func (m *memUsers) Admins(context.Context) (int, error) {
	n := 0
	for _, u := range m.byID {
		if u.IsAdmin() {
			n++
		}
	}
	return n, nil
}

type memRequests struct{ list []*Request }

func (m *memRequests) Find(_ context.Context, email string) (*Request, error) {
	for _, r := range m.list {
		if r.Email == email {
			return r, nil
		}
	}
	return nil, nil
}
func (m *memRequests) Get(_ context.Context, id uuid.UUID) (*Request, error) {
	for _, r := range m.list {
		if r.ID == id {
			cp := *r
			return &cp, nil
		}
	}
	return nil, errs.NotFound("Request")
}
func (m *memRequests) Save(_ context.Context, r *Request) error {
	for i, old := range m.list {
		if old.ID == r.ID {
			m.list[i] = r
			return nil
		}
	}
	m.list = append(m.list, r)
	return nil
}
func (m *memRequests) List(_ context.Context, status Status, _, _ int) ([]*Request, int, error) {
	var list []*Request
	for _, r := range m.list {
		if r.Status == status {
			list = append(list, r)
		}
	}
	return list, len(list), nil
}
func (m *memRequests) Expire(context.Context, time.Time) error { return nil }
func (m *memRequests) Delete(context.Context, string) error    { return nil }

type memSettings struct {
	stored *Settings
	fail   error
}

func (m *memSettings) Get(context.Context) (*Settings, error)    { return m.stored, m.fail }
func (m *memSettings) Save(_ context.Context, s *Settings) error { m.stored = s; return nil }

type revoker struct{}

func (revoker) Revoke(context.Context, uuid.UUID, string) error { return nil }

// outbox records notifications instead of sending them.
type outbox struct{ sent []ports.Notification }

func (*outbox) LoginCode(context.Context, string, string, time.Duration) error { return nil }
func (*outbox) Invite(context.Context, string, string) error                   { return nil }
func (o *outbox) Notify(_ context.Context, n ...ports.Notification)            { o.sent = append(o.sent, n...) }

type direct struct{}

func (direct) Do(ctx context.Context, fn func(ctx context.Context) error) error { return fn(ctx) }

// world is a server with an actor in a given role, a second administrator,
// an active account, a disabled account and a pending request.
type world struct {
	svc                    *Service
	actor, other, disabled uuid.UUID
	request                uuid.UUID
	sent                   *outbox
	logs                   *bytes.Buffer
}

func newWorld(role user.Role, status user.Status) world {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	users := &memUsers{byID: map[uuid.UUID]*user.User{}}
	add := func(name string, role user.Role, status user.Status) uuid.UUID {
		u := &user.User{ID: uuid.New(), Name: name, Email: name + "@example.com", Role: role, Status: status,
			Notifications: user.DefaultNotifications(), CreatedAt: now}
		users.byID[u.ID] = u
		return u.ID
	}
	w := world{
		actor:    add("actor", role, status),
		other:    add("ben", user.RoleUser, user.StatusActive),
		disabled: add("cleo", user.RoleUser, user.StatusDisabled),
	}
	add("ada", user.RoleAdmin, user.StatusActive)
	requests := &memRequests{}
	w.request = uuid.New()
	requests.list = append(requests.list, &Request{ID: w.request, Email: "dan@else.org", Status: StatusPending, RequestedAt: now})
	w.sent, w.logs = &outbox{}, &bytes.Buffer{}
	w.svc = NewService(Deps{Settings: &memSettings{}, Requests: requests, Users: users, Accounts: users,
		Credentials: revoker{}, Notifier: w.sent, Tx: direct{}, Clock: clock.Fixed{At: now},
		Log:      slog.New(slog.NewJSONHandler(w.logs, nil)),
		Defaults: auth.Signup{Mode: auth.SignupApproval}})
	return w
}

// TestRoles checks who may call each administration method: administrators
// everything, managers only the requests to join, everyone else nothing.
func TestRoles(t *testing.T) {
	ctx := context.Background()
	methods := []struct {
		name      string
		approvals bool
		call      func(w world) error
	}{
		{"Settings", false, func(w world) error { _, err := w.svc.Settings(ctx, w.actor); return err }},
		{"UpdateSettings", false, func(w world) error {
			_, err := w.svc.UpdateSettings(ctx, w.actor, "invite", nil)
			return err
		}},
		{"Requests", true, func(w world) error { _, _, err := w.svc.Requests(ctx, w.actor, "", 50, 0); return err }},
		{"Approve", true, func(w world) error { _, err := w.svc.Approve(ctx, w.actor, w.request); return err }},
		{"Decline", true, func(w world) error { _, err := w.svc.Decline(ctx, w.actor, w.request); return err }},
		{"Accounts", false, func(w world) error { _, _, err := w.svc.Accounts(ctx, w.actor, "", "", 50, 0); return err }},
		{"SetRole", false, func(w world) error { _, err := w.svc.SetRole(ctx, w.actor, w.other, "manager"); return err }},
		{"Disable", false, func(w world) error { _, err := w.svc.Disable(ctx, w.actor, w.other); return err }},
		{"Enable", false, func(w world) error { _, err := w.svc.Enable(ctx, w.actor, w.disabled); return err }},
	}
	actors := []struct {
		name   string
		role   user.Role
		status user.Status
		admin  bool
		staff  bool
	}{
		{"user", user.RoleUser, user.StatusActive, false, false},
		{"manager", user.RoleManager, user.StatusActive, false, true},
		{"admin", user.RoleAdmin, user.StatusActive, true, true},
		{"disabled admin", user.RoleAdmin, user.StatusDisabled, false, false},
		{"disabled manager", user.RoleManager, user.StatusDisabled, false, false},
	}
	for _, m := range methods {
		for _, a := range actors {
			t.Run(m.name+"/"+a.name, func(t *testing.T) {
				allowed := a.admin || (m.approvals && a.staff)
				err := m.call(newWorld(a.role, a.status))
				switch {
				case allowed && err != nil:
					t.Fatalf("must be allowed, got %v", err)
				case !allowed && !errs.Is(err, errs.KindForbidden):
					t.Fatalf("must be forbidden, got %v", err)
				}
			})
		}
	}
}

func TestAccountChangesRefused(t *testing.T) {
	ctx := context.Background()
	w := newWorld(user.RoleAdmin, user.StatusActive)
	users := w.svc.d.Users.(*memUsers)
	placeholder := uuid.New()
	users.byID[placeholder] = &user.User{ID: placeholder, Name: "Kid", Status: user.StatusPlaceholder}
	for _, tc := range []struct {
		name string
		want errs.Kind
		call func() error
	}{
		{"role of a placeholder", errs.KindValidation, func() error {
			_, err := w.svc.SetRole(ctx, w.actor, placeholder, "manager")
			return err
		}},
		{"role of a disabled account", errs.KindConflict, func() error {
			_, err := w.svc.SetRole(ctx, w.actor, w.disabled, "manager")
			return err
		}},
		{"unknown role", errs.KindValidation, func() error {
			_, err := w.svc.SetRole(ctx, w.actor, w.other, "owner")
			return err
		}},
		{"enable an active account", errs.KindConflict, func() error {
			_, err := w.svc.Enable(ctx, w.actor, w.other)
			return err
		}},
		{"disable a placeholder", errs.KindValidation, func() error {
			_, err := w.svc.Disable(ctx, w.actor, placeholder)
			return err
		}},
		{"disable yourself", errs.KindValidation, func() error {
			_, err := w.svc.Disable(ctx, w.actor, w.actor)
			return err
		}},
		{"requests with an unknown status", errs.KindValidation, func() error {
			_, _, err := w.svc.Requests(ctx, w.actor, "waiting", 50, 0)
			return err
		}},
	} {
		if err := tc.call(); !errs.Is(err, tc.want) {
			t.Errorf("%s: got %v, want %s", tc.name, err, tc.want)
		}
	}
}

func TestLastAdminStays(t *testing.T) {
	ctx := context.Background()
	w := newWorld(user.RoleAdmin, user.StatusActive)
	users := w.svc.d.Users.(*memUsers)
	var ada uuid.UUID
	for id, u := range users.byID {
		if u.Name == "ada" {
			ada = id
		}
	}
	if _, err := w.svc.Disable(ctx, w.actor, ada); err != nil {
		t.Fatalf("with two administrators one may be disabled: %v", err)
	}
	// The actor is now the only administrator. Another administrator who is
	// already disabled does not count.
	if _, err := w.svc.SetRole(ctx, w.actor, w.actor, "manager"); !errs.Is(err, errs.KindConflict) {
		t.Fatalf("the last administrator must not step down: %v", err)
	}
	if u, _ := users.Get(ctx, w.actor); !u.IsAdmin() {
		t.Fatalf("the refused change must not be stored: %+v", u)
	}
}

func TestRequest(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name, email, typed string
		stored             string // the stored name; "-" when nothing new is stored
		declined           bool
		want               errs.Kind
	}{
		{"new request", "fay@else.org", "  Fay\r\n Evans\t", "Fay Evans", false, ""},
		{"no name", "fay@else.org", "", "fay", false, ""},
		{"name too long", "fay@else.org", strings.Repeat("x", 81), "-", false, errs.KindValidation},
		{"already pending", "dan@else.org", "Dan", "-", false, ""},
		{"already approved", "ed@else.org", "Ed", "-", false, ""},
		{"declined", "gil@else.org", "Gil", "-", true, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newWorld(user.RoleAdmin, user.StatusActive)
			requests := w.svc.d.Requests.(*memRequests)
			requests.list = append(requests.list,
				&Request{ID: uuid.New(), Email: "ed@else.org", Status: StatusApproved},
				&Request{ID: uuid.New(), Email: "gil@else.org", Status: StatusDeclined})
			before := len(requests.list)
			declined, err := w.svc.Request(ctx, tc.email, tc.typed)
			if tc.want != "" {
				if !errs.Is(err, tc.want) {
					t.Fatalf("got %v, want %s", err, tc.want)
				}
			} else if err != nil || declined != tc.declined {
				t.Fatalf("got (%v, %v), want (%v, nil)", declined, err, tc.declined)
			}
			if tc.stored == "-" {
				if len(requests.list) != before {
					t.Fatalf("nothing new may be stored, got %+v", requests.list[before:])
				}
				return
			}
			if len(requests.list) != before+1 || requests.list[before].Name != tc.stored {
				t.Fatalf("stored %+v, want one request named %q", requests.list[before:], tc.stored)
			}
		})
	}
}

func TestAnnounce(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name   string
		ctx    context.Context
		fail   error
		notes  int
		logged bool
	}{
		{"staff hear about it", ctx, nil, 2, false},
		{"lookup fails", ctx, errors.New("database is down"), 0, true},
		{"quiet", ports.Quiet(ctx), errors.New("database is down"), 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newWorld(user.RoleManager, user.StatusActive)
			w.svc.d.Accounts.(*memUsers).fail = tc.fail
			w.svc.d.Requests.(*memRequests).list = nil
			if declined, err := w.svc.Request(tc.ctx, "fay@else.org", "Fay"); declined || err != nil {
				t.Fatalf("the request must be filed whatever happens to the notice: %v, %v", declined, err)
			}
			if len(w.sent.sent) != tc.notes {
				t.Fatalf("%d notices sent, want %d (the manager and ada)", len(w.sent.sent), tc.notes)
			}
			if got := strings.Contains(w.logs.String(), `"msg":"admin.announce.failed"`); got != tc.logged {
				t.Fatalf("admin.announce.failed logged = %v, want %v: %s", got, tc.logged, w.logs)
			}
		})
	}
}

func TestSignupFails(t *testing.T) {
	w := newWorld(user.RoleAdmin, user.StatusActive)
	down := errors.New("database is down")
	w.svc.d.Settings.(*memSettings).fail = down
	got, err := w.svc.Signup(context.Background())
	if !errors.Is(err, down) || got.Mode != "" || got.Allowed != nil {
		t.Fatalf("Signup = %+v, %v; want nothing and the error", got, err)
	}
}
