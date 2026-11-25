package bootstrap_test

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/bootstrap"
	"github.com/Hostworthy/SplitSpends/packages/bootstrap/bootstraptest"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/admin"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/group"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/push"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/user"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/Hostworthy/SplitSpends/packages/database"
	"github.com/Hostworthy/SplitSpends/packages/database/repositories"
	"github.com/google/uuid"
)

// TestAdministration walks through roles, sign-up modes, approvals and
// disabled accounts against a real database.
func TestAdministration(t *testing.T) {
	ctx := context.Background()
	env := bootstraptest.Open(t, bootstraptest.Options{Now: time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)})
	svc, notes := env.Services, env.Notifier
	login := func(email, name string) (*user.User, string, error) {
		t.Helper()
		notes.Forget(email)
		if err := svc.Auth.RequestCode(ctx, email, "", ""); err != nil {
			return nil, "", err
		}
		code := notes.Code(email)
		if code == "" {
			return nil, "", errs.Forbidden("no code sent")
		}
		l, err := svc.Auth.Verify(ctx, email, code, name, "test")
		if err != nil {
			return nil, "", err
		}
		return l.User, l.Token, nil
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	// The first account runs the server.
	ada, _, err := login("ada@example.com", "Ada")
	must(err)
	if ada.Role != user.RoleAdmin {
		t.Fatalf("first account role = %s", ada.Role)
	}
	if _, err := svc.Admin.UpdateSettings(ctx, ada.ID, "open", nil); err != nil {
		t.Fatal(err)
	}
	ben, benToken, err := login("ben@example.com", "Ben")
	must(err)
	if ben.Role != user.RoleUser {
		t.Fatalf("second account role = %s", ben.Role)
	}
	if _, err := svc.Admin.Settings(ctx, ben.ID); !errs.Is(err, errs.KindForbidden) {
		t.Fatalf("users must not see settings: %v", err)
	}

	// Approval mode with an allowed domain.
	if _, err := svc.Admin.UpdateSettings(ctx, ada.ID, "approval", []string{"family.com", "Cleo@Example.com", " "}); err != nil {
		t.Fatal(err)
	}
	if s, _ := svc.Admin.Settings(ctx, ada.ID); strings.Join(s.Allowed, ",") != "@family.com,cleo@example.com" {
		t.Fatalf("allowed list not normalized: %v", s.Allowed)
	}
	if _, _, err := login("kim@family.com", "Kim"); err != nil {
		t.Fatalf("allowed domain must sign in directly: %v", err)
	}
	notes.Reset()
	if _, _, err := login("dan@else.org", "Dan"); !errs.Is(err, errs.KindPending) {
		t.Fatalf("stranger must wait for approval: %v", err)
	}
	if sent := notes.Sent(); len(sent) != 1 || sent[0].To != "ada@example.com" || !strings.Contains(sent[0].Subject, "Dan (dan@else.org)") {
		t.Fatalf("administrators must hear about the request: %+v", sent)
	}

	// A manager can approve, but not change settings.
	if _, err := svc.Admin.SetRole(ctx, ben.ID, ben.ID, "admin"); !errs.Is(err, errs.KindForbidden) {
		t.Fatalf("users cannot promote themselves: %v", err)
	}
	if _, err := svc.Admin.SetRole(ctx, ada.ID, ben.ID, "manager"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Admin.UpdateSettings(ctx, ben.ID, "open", nil); !errs.Is(err, errs.KindForbidden) {
		t.Fatalf("managers cannot change settings: %v", err)
	}
	pending, _, err := svc.Admin.Requests(ctx, ben.ID, "", 50, 0)
	must(err)
	if len(pending) != 1 || pending[0].Email != "dan@else.org" || pending[0].Name != "Dan" {
		t.Fatalf("pending = %+v", pending)
	}
	notes.Reset()
	if _, err := svc.Admin.Approve(ctx, ben.ID, pending[0].ID); err != nil {
		t.Fatal(err)
	}
	if sent := notes.Sent(); len(sent) != 1 || sent[0].To != "dan@else.org" {
		t.Fatalf("the approved person must be emailed: %+v", sent)
	}
	dan, _, err := login("dan@else.org", "Dan")
	if err != nil || dan.Name != "Dan" {
		t.Fatalf("approved person must sign in: %v %+v", err, dan)
	}

	// Declined people are told so.
	if _, _, err := login("eve@else.org", "Eve"); !errs.Is(err, errs.KindPending) {
		t.Fatal(err)
	}
	pending, _, _ = svc.Admin.Requests(ctx, ada.ID, "pending", 50, 0)
	if _, err := svc.Admin.Decline(ctx, ada.ID, pending[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := login("eve@else.org", "Eve"); !errs.Is(err, errs.KindDeclined) {
		t.Fatalf("declined: %v", err)
	}

	// Closed: nobody new, not even the allowed list; accounts still sign in.
	if _, err := svc.Admin.UpdateSettings(ctx, ada.ID, "closed", []string{"@family.com"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := login("lee@family.com", "Lee"); err == nil {
		t.Fatal("closed mode must not create accounts")
	}
	if _, _, err := login("kim@family.com", ""); err != nil {
		t.Fatalf("existing accounts still sign in: %v", err)
	}

	// Disabling signs out, removes the email and keeps its hash.
	if _, err := svc.Admin.Disable(ctx, ada.ID, ada.ID); err == nil {
		t.Fatal("administrators cannot disable themselves")
	}
	if _, err := svc.Admin.Disable(ctx, ada.ID, ben.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Auth.Authenticate(ctx, benToken); err == nil {
		t.Fatal("a disabled account's sessions must stop working")
	}
	stored, err := svc.Users.Get(ctx, ben.ID)
	must(err)
	if stored.Email != "" || stored.EmailHash != user.HashEmail("ben@example.com") || stored.Status != user.StatusDisabled {
		t.Fatalf("disabled account = %+v", stored)
	}
	if _, _, err := login("ben@example.com", ""); err == nil {
		t.Fatal("a disabled account must not sign in")
	}

	// Disabling removes the address from requests to join and login codes.
	if err := svc.Auth.RequestCode(ctx, "dan@else.org", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Admin.Disable(ctx, ada.ID, dan.ID); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"signup_requests", "login_challenges"} {
		var left int
		if err := env.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE lower(email) = 'dan@else.org'`).Scan(&left); err != nil {
			t.Fatal(err)
		}
		if left != 0 {
			t.Fatalf("%s still holds a disabled account's address %d times", table, left)
		}
	}
	approved, _, err := svc.Admin.Requests(ctx, ada.ID, "approved", 50, 0)
	must(err)
	for _, r := range approved {
		if r.Email == "dan@else.org" {
			t.Fatalf("approved requests still list a disabled account: %+v", r)
		}
	}
	if _, err := svc.Admin.Enable(ctx, ada.ID, ben.ID); err != nil {
		t.Fatal(err)
	}
	again, _, err := login("ben@example.com", "")
	if err != nil || again.ID != ben.ID || again.Email != "ben@example.com" || again.Status != user.StatusActive {
		t.Fatalf("re-enabled account must sign in as itself: %v %+v", err, again)
	}

	// The last administrator stays.
	if _, err := svc.Admin.SetRole(ctx, ada.ID, ada.ID, "user"); !errs.Is(err, errs.KindConflict) {
		t.Fatalf("last admin stepped down: %v", err)
	}
	if _, err := svc.Admin.SetRole(ctx, ada.ID, ben.ID, "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Admin.SetRole(ctx, ada.ID, ada.ID, "user"); err != nil {
		t.Fatalf("with another admin, stepping down is fine: %v", err)
	}
	list, total, err := svc.Admin.Accounts(ctx, ben.ID, "", "", 50, 0)
	must(err)
	if total != 4 || list[0].ID != ben.ID {
		t.Fatalf("accounts = %d, first %s", total, list[0].Name)
	}
}

func TestDisabledAccountCannotUseEarlierCode(t *testing.T) {
	ctx := context.Background()
	env := bootstraptest.Open(t, bootstraptest.Options{})
	svc := env.Services
	ada := env.SignIn(t, "ada@example.com", "Ada").User
	ben := env.SignIn(t, "ben@example.com", "Ben").User
	if err := svc.Auth.RequestCode(ctx, "ben@example.com", "", ""); err != nil {
		t.Fatal(err)
	}
	code := env.Notifier.Code("ben@example.com")
	if _, err := svc.Admin.Disable(ctx, ada.ID, ben.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Auth.Verify(ctx, "ben@example.com", code, "", "test"); err == nil {
		t.Fatal("a code sent before disabling must not sign in")
	}
	stored, err := svc.Users.Get(ctx, ben.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != user.StatusDisabled || stored.Email != "" {
		t.Fatalf("the account must stay disabled: %+v", stored)
	}
	var codes int
	if err := env.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM login_challenges WHERE email = 'ben@example.com'`).Scan(&codes); err != nil {
		t.Fatal(err)
	}
	if codes != 0 {
		t.Fatalf("disabling must remove the account's login codes, %d left", codes)
	}
}

// TestLastAdminConcurrent runs administrator changes that would each leave
// one administrator at the same time: exactly one goes through, and the
// server always keeps an active administrator.
func TestLastAdminConcurrent(t *testing.T) {
	ctx := context.Background()
	env := bootstraptest.Open(t, bootstraptest.Options{})
	svc := env.Services
	for _, tc := range []struct {
		name string
		want errs.Kind
		act  func(a, b uuid.UUID) (func() error, func() error)
	}{
		{"demote each other", errs.KindForbidden, func(a, b uuid.UUID) (func() error, func() error) {
			return func() error { _, err := svc.Admin.SetRole(ctx, a, b, "user"); return err },
				func() error { _, err := svc.Admin.SetRole(ctx, b, a, "user"); return err }
		}},
		{"step down together", errs.KindConflict, func(a, b uuid.UUID) (func() error, func() error) {
			return func() error { _, err := svc.Admin.SetRole(ctx, a, a, "manager"); return err },
				func() error { _, err := svc.Admin.SetRole(ctx, b, b, "manager"); return err }
		}},
		{"disable each other", errs.KindForbidden, func(a, b uuid.UUID) (func() error, func() error) {
			return func() error { _, err := svc.Admin.Disable(ctx, a, b); return err },
				func() error { _, err := svc.Admin.Disable(ctx, b, a); return err }
		}},
	} {
		for run := range 20 {
			if _, err := env.DB.Pool.Exec(ctx, `TRUNCATE users, login_challenges CASCADE`); err != nil {
				t.Fatal(err)
			}
			ada := env.SignIn(t, "ada@example.com", "Ada").User
			ben := env.SignIn(t, "ben@example.com", "Ben").User
			if _, err := svc.Admin.SetRole(ctx, ada.ID, ben.ID, "admin"); err != nil {
				t.Fatal(err)
			}
			first, second := tc.act(ada.ID, ben.ID)
			calls := []func() error{first, second}
			done, refused := 0, 0
			for _, err := range concurrently(2, func(i int) error { return calls[i]() }) {
				switch {
				case err == nil:
					done++
				case errs.Is(err, tc.want):
					refused++
				default:
					t.Fatalf("%s, run %d: %v", tc.name, run, err)
				}
			}
			var admins int
			if err := env.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE status = 'active' AND role = 'admin'`).Scan(&admins); err != nil {
				t.Fatal(err)
			}
			if done != 1 || refused != 1 || admins != 1 {
				t.Fatalf("%s, run %d: %d done, %d refused, %d administrators left, want 1 each", tc.name, run, done, refused, admins)
			}
		}
	}
}

// pendingRequest files dan's request to join in approval mode and returns
// it, with ada as the administrator.
func pendingRequest(t *testing.T, env *bootstraptest.Env) (*user.User, *admin.Request) {
	t.Helper()
	ctx := context.Background()
	svc := env.Services
	ada := env.SignIn(t, "ada@example.com", "Ada").User
	if _, err := svc.Admin.UpdateSettings(ctx, ada.ID, "approval", nil); err != nil {
		t.Fatal(err)
	}
	if err := svc.Auth.RequestCode(ctx, "dan@else.org", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Auth.Verify(ctx, "dan@else.org", env.Notifier.Code("dan@else.org"), "Dan", "test"); !errs.Is(err, errs.KindPending) {
		t.Fatalf("dan must wait for approval: %v", err)
	}
	pending, _, err := svc.Admin.Requests(ctx, ada.ID, "", 50, 0)
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending = %+v, %v", pending, err)
	}
	return ada, pending[0]
}

func TestApprovalSurvivesModeChange(t *testing.T) {
	ctx := context.Background()
	env := bootstraptest.Open(t, bootstraptest.Options{})
	ada, request := pendingRequest(t, env)
	if _, err := env.Services.Admin.UpdateSettings(ctx, ada.ID, "invite", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Services.Admin.Approve(ctx, ada.ID, request.ID); err != nil {
		t.Fatal(err)
	}
	if dan := env.SignIn(t, "dan@else.org", "Dan").User; dan.Status != user.StatusActive {
		t.Fatalf("an approved person must sign in after the mode changed to invite: %+v", dan)
	}
}

func TestApproveRefusedWhenClosed(t *testing.T) {
	ctx := context.Background()
	env := bootstraptest.Open(t, bootstraptest.Options{})
	ada, request := pendingRequest(t, env)
	if _, err := env.Services.Admin.UpdateSettings(ctx, ada.ID, "closed", nil); err != nil {
		t.Fatal(err)
	}
	env.Notifier.Reset()
	if _, err := env.Services.Admin.Approve(ctx, ada.ID, request.ID); !errs.Is(err, errs.KindConflict) {
		t.Fatalf("approving while sign-up is closed must be refused, got %v", err)
	}
	if sent := env.Notifier.Sent(); len(sent) != 0 {
		t.Fatalf("nobody may be told they can sign in: %+v", sent)
	}
	pending, _, err := env.Services.Admin.Requests(ctx, ada.ID, "", 50, 0)
	if err != nil || len(pending) != 1 {
		t.Fatalf("the request must stay pending: %+v %v", pending, err)
	}
}

// ask files a request to join for email in approval mode.
func ask(t *testing.T, env *bootstraptest.Env, email string) {
	t.Helper()
	ctx := context.Background()
	if err := env.Services.Auth.RequestCode(ctx, email, "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Services.Auth.Verify(ctx, email, env.Notifier.Code(email), "", "test"); !errs.Is(err, errs.KindPending) {
		t.Fatalf("%s must wait for approval: %v", email, err)
	}
}

func TestJoinRequestsNotifyStaffOnce(t *testing.T) {
	ctx := context.Background()
	env := bootstraptest.Open(t, bootstraptest.Options{})
	ada := env.SignIn(t, "ada@example.com", "Ada").User
	if _, err := env.Services.Admin.UpdateSettings(ctx, ada.ID, "approval", nil); err != nil {
		t.Fatal(err)
	}
	env.Notifier.Reset()
	for _, email := range []string{"dan@else.org", "eve@else.org", "fay@else.org"} {
		ask(t, env, email)
		env.Clock.Add(time.Minute)
	}
	if got := env.Notifier.Subjects("ada@example.com"); len(got) != 1 {
		t.Fatalf("staff must hear once about requests filed together, got %q", got)
	}
	env.Clock.Add(time.Hour)
	ask(t, env, "gus@else.org")
	if got := env.Notifier.Subjects("ada@example.com"); len(got) != 2 {
		t.Fatalf("staff must hear about a request filed an hour after the last, got %q", got)
	}
}

// fillRequests stores n pending requests filed a second apart, the newest
// at the clock's time.
func fillRequests(t *testing.T, env *bootstraptest.Env, n int) {
	t.Helper()
	if _, err := env.DB.Pool.Exec(context.Background(), `INSERT INTO signup_requests (id, email, name, status, requested_at)
		SELECT gen_random_uuid(), 'person' || i || '@else.org', '', 'pending', $2::timestamptz - i * interval '1 second'
		FROM generate_series(1, $1) AS i`, n, env.Clock.Now()); err != nil {
		t.Fatal(err)
	}
}

func TestJoinRequestsPage(t *testing.T) {
	ctx := context.Background()
	env := bootstraptest.Open(t, bootstraptest.Options{})
	ada := env.SignIn(t, "ada@example.com", "Ada").User
	fillRequests(t, env, 600)
	page, total, err := env.Services.Admin.Requests(ctx, ada.ID, "", 200, 500)
	if err != nil {
		t.Fatal(err)
	}
	if total != 600 || len(page) != 100 || page[99].Email != "person600@else.org" {
		t.Fatalf("offset 500 = %d requests of %d, want the oldest 100 of 600", len(page), total)
	}
}

func TestJoinRequestsBounded(t *testing.T) {
	ctx := context.Background()
	env := bootstraptest.Open(t, bootstraptest.Options{})
	ada := env.SignIn(t, "ada@example.com", "Ada").User
	if _, err := env.Services.Admin.UpdateSettings(ctx, ada.ID, "approval", nil); err != nil {
		t.Fatal(err)
	}
	fillRequests(t, env, admin.MaxPending)
	if err := env.Services.Auth.RequestCode(ctx, "dan@else.org", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Services.Auth.Verify(ctx, "dan@else.org", env.Notifier.Code("dan@else.org"), "Dan", "test"); !errs.Is(err, errs.KindRateLimited) {
		t.Fatalf("a full queue must refuse new requests: %v", err)
	}

	// Undecided requests expire; decisions stay.
	pending, _, err := env.Services.Admin.Requests(ctx, ada.ID, "", 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Services.Admin.Decline(ctx, ada.ID, pending[0].ID); err != nil {
		t.Fatal(err)
	}
	env.Clock.Add(admin.PendingTTL)
	ask(t, env, "eve@else.org")
	env.Clock.Add(time.Second)
	rt := &bootstrap.Runtime{Config: env.Config, Log: slog.New(slog.DiscardHandler), Services: env.Services}
	if err := rt.RunJobsOnce(ctx); err != nil {
		t.Fatal(err)
	}
	for status, want := range map[string]int{"pending": 1, "declined": 1} {
		if _, total, err := env.Services.Admin.Requests(ctx, ada.ID, status, 50, 0); err != nil || total != want {
			t.Fatalf("%s requests after cleanup = %d, %v; want %d", status, total, err, want)
		}
	}
}

// filing is the request repository with a hook that runs right before a
// request is saved.
type filing struct {
	*repositories.SignupRequests
	before func()
}

func (f filing) Save(ctx context.Context, r *admin.Request) error {
	f.before()
	return f.SignupRequests.Save(ctx, r)
}

// TestJoinRequestLimitIsAtomic checks that requests filed at the same time
// cannot take the queue past admin.MaxPending.
func TestJoinRequestLimitIsAtomic(t *testing.T) {
	ctx := context.Background()
	env := bootstraptest.Open(t, bootstraptest.Options{})
	fillRequests(t, env, admin.MaxPending-1)
	// One place is left. Right before it is taken, someone else asks.
	other := make(chan error, 1)
	var once sync.Once
	repo := filing{repositories.NewSignupRequests(env.DB), func() {
		once.Do(func() {
			meanwhile(t, env, func() {
				_, err := env.Services.Admin.Request(ctx, "late@else.org", "Late")
				other <- err
			})
		})
	}}
	svc := admin.NewService(admin.Deps{Requests: repo, Accounts: repositories.NewUsers(env.DB), Notifier: env.Notifier,
		Tx: database.NewTx(env.DB), Clock: env.Clock, Log: slog.New(slog.DiscardHandler)})
	_, err := svc.Request(ctx, "last@else.org", "Last")
	late := <-other
	if (err == nil) == (late == nil) {
		t.Fatalf("exactly one request may take the last place, got %v and %v", err, late)
	}
	if _, n, err := repositories.NewSignupRequests(env.DB).List(ctx, admin.StatusPending, 1, 0); err != nil || n != admin.MaxPending {
		t.Fatalf("want %d requests waiting, got %d, %v", admin.MaxPending, n, err)
	}
}

// TestDisableRevokesTokensAndPush checks that disabling an account removes
// every way back in: interactive sessions, API tokens and push devices.
func TestDisableRevokesTokensAndPush(t *testing.T) {
	ctx := context.Background()
	env, _ := pushSetup(t, time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC))
	svc := env.Services
	ada := env.SignIn(t, "ada@example.com", "Ada").User
	login := env.SignIn(t, "ben@example.com", "Ben")
	ben := login.User
	p, err := svc.Auth.Authenticate(ctx, login.Token)
	if err != nil {
		t.Fatal(err)
	}
	token, err := svc.Auth.CreateToken(ctx, p, "Script")
	if err != nil {
		t.Fatal(err)
	}
	p256dh, keys := deviceKeys(t)
	if _, _, err := svc.Push.Register(ctx, ben.ID, p.SessionID, push.Input{Endpoint: "https://push.example.com/ben", P256dh: p256dh, Auth: keys}); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Admin.Disable(ctx, ada.ID, ben.ID); err != nil {
		t.Fatal(err)
	}
	for name, secret := range map[string]string{"session": login.Token, "API token": token.Token} {
		if _, err := svc.Auth.Authenticate(ctx, secret); !errs.Is(err, errs.KindUnauthorized) {
			t.Errorf("the %s of a disabled account must stop working: %v", name, err)
		}
	}
	var devices int
	if err := env.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM push_subscriptions WHERE user_id = $1`, ben.ID).Scan(&devices); err != nil {
		t.Fatal(err)
	}
	if devices != 0 {
		t.Fatalf("a disabled account keeps %d push devices", devices)
	}
}

// TestInviteLinksOfDisabledAccount checks that links someone shared stop
// working when their account is disabled, as they do when they leave.
func TestInviteLinksOfDisabledAccount(t *testing.T) {
	ctx := context.Background()
	env, ada, ben, g := flat(t)
	svc := env.Services
	if _, err := svc.Admin.UpdateSettings(ctx, ada, "invite", nil); err != nil {
		t.Fatal(err)
	}
	env.SignIn(t, "ben@example.com", "Ben")
	issued, err := svc.Invites.Create(ctx, ben, g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Admin.Disable(ctx, ada, ben); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Invites.Preview(ctx, issued.Token); !errs.Is(err, errs.KindNotFound) {
		t.Fatalf("preview: %v", err)
	}
	if ok, err := svc.Invites.Valid(ctx, issued.Token); ok || err != nil {
		t.Fatalf("the link of a disabled account must not let anyone sign up: %v", err)
	}
	if err := svc.Auth.RequestCode(ctx, "cleo@example.com", issued.Token, ""); err != nil {
		t.Fatal(err)
	}
	if code := env.Notifier.Code("cleo@example.com"); code != "" {
		t.Fatal("a code was sent to a stranger holding the link of a disabled account")
	}
}

// TestJoinRequestsConcurrent files two requests for the same address at the
// same time, as two sign-ins with codes for one address do: one is stored
// and neither fails.
func TestJoinRequestsConcurrent(t *testing.T) {
	ctx := context.Background()
	env := bootstraptest.Open(t, bootstraptest.Options{})
	requests := repositories.NewSignupRequests(env.DB)
	for run := range 20 {
		email := fmt.Sprintf("dan%d@else.org", run)
		results := concurrently(2, func(int) error {
			return requests.Save(ctx, &admin.Request{ID: uuid.New(), Email: email, Status: admin.StatusPending, RequestedAt: env.Clock.Now()})
		})
		for _, err := range results {
			if err != nil {
				t.Fatalf("run %d: %v", run, err)
			}
		}
		var stored int
		if err := env.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM signup_requests WHERE email = $1`, email).Scan(&stored); err != nil {
			t.Fatal(err)
		}
		if stored != 1 {
			t.Fatalf("run %d: %d requests stored for one address, want 1", run, stored)
		}
	}
}

// TestReenabledAccount follows an account from disabled to enabled: the
// administrator finds it by its old address throughout, sees it apart from
// people who were only invited, and the person signs in even while sign-up
// is closed.
func TestReenabledAccount(t *testing.T) {
	ctx := context.Background()
	env := bootstraptest.Open(t, bootstraptest.Options{})
	svc := env.Services
	ada := env.SignIn(t, "ada@example.com", "Ada").User
	ben := env.SignIn(t, "ben@example.com", "Ben").User
	if _, err := svc.Groups.Create(ctx, ada.ID, group.CreateInput{Patch: group.Patch{Name: str("Flat")},
		Members: []group.Invitee{{Email: "cleo@example.com", Name: "Cleo"}}}); err != nil {
		t.Fatal(err)
	}
	find := func(q string) []*user.User {
		t.Helper()
		list, _, err := svc.Admin.Accounts(ctx, ada.ID, "", q, 50, 0)
		if err != nil {
			t.Fatal(err)
		}
		return list
	}
	if _, err := svc.Admin.Disable(ctx, ada.ID, ben.ID); err != nil {
		t.Fatal(err)
	}
	if found := find(" Ben@Example.com "); len(found) != 1 || found[0].ID != ben.ID {
		t.Fatalf("a disabled account must be found by its address: %+v", found)
	}
	if _, err := svc.Admin.Enable(ctx, ada.ID, ben.ID); err != nil {
		t.Fatal(err)
	}
	if found := find("ben@example.com"); len(found) != 1 || found[0].ID != ben.ID || !found[0].Reenabled() {
		t.Fatalf("a re-enabled account must be found by its address and shown as re-enabled: %+v", found)
	}
	if cleo := find("cleo@example.com"); len(cleo) != 1 || cleo[0].Status != user.StatusInvited || cleo[0].Reenabled() {
		t.Fatalf("an invited person is not re-enabled: %+v", cleo)
	}
	if _, err := svc.Admin.UpdateSettings(ctx, ada.ID, "closed", nil); err != nil {
		t.Fatal(err)
	}
	if again := env.SignIn(t, "ben@example.com", "").User; again.ID != ben.ID || again.Reenabled() {
		t.Fatalf("a re-enabled account must sign in while sign-up is closed: %+v", again)
	}
}
