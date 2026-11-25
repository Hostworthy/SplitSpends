package admin

import (
	"context"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/auth"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/user"
	"github.com/Hostworthy/SplitSpends/packages/core/ports"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/clock"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/google/uuid"
)

type memUsers struct{ byID map[uuid.UUID]*user.User }

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
func (m *memUsers) Staff(context.Context) ([]*user.User, error)             { return nil, nil }
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

type memSettings struct{ stored *Settings }

func (m *memSettings) Get(context.Context) (*Settings, error)    { return m.stored, nil }
func (m *memSettings) Save(_ context.Context, s *Settings) error { m.stored = s; return nil }

type revoker struct{}

func (revoker) Revoke(context.Context, uuid.UUID, string) error { return nil }

type silent struct{}

func (silent) LoginCode(context.Context, string, string, time.Duration) error { return nil }
func (silent) Invite(context.Context, string, string) error                   { return nil }
func (silent) Notify(context.Context, ports.Notification)                     {}

type direct struct{}

func (direct) Do(ctx context.Context, fn func(ctx context.Context) error) error { return fn(ctx) }

// world is a server with an actor in a given role, a second administrator,
// an active account, a disabled account and a pending request.
type world struct {
	svc                    *Service
	actor, other, disabled uuid.UUID
	request                uuid.UUID
}

func newWorld(role user.Role, status user.Status) world {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	users := &memUsers{byID: map[uuid.UUID]*user.User{}}
	add := func(name string, role user.Role, status user.Status) uuid.UUID {
		u := &user.User{ID: uuid.New(), Name: name, Email: name + "@example.com", Role: role, Status: status, CreatedAt: now}
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
	w.svc = NewService(Deps{Settings: &memSettings{}, Requests: requests, Users: users, Accounts: users,
		Credentials: revoker{}, Notifier: silent{}, Tx: direct{}, Clock: clock.Fixed{At: now},
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
