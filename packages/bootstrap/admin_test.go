package bootstrap

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/admin"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/user"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
)

// TestAdministration walks through roles, sign-up modes, approvals and
// disabled accounts against a real database.
func TestAdministration(t *testing.T) {
	ctx := context.Background()
	svc, notes := setup(t, time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC))
	login := func(email, name string) (*user.User, string, error) {
		t.Helper()
		delete(notes.codes, email)
		if err := svc.Auth.RequestCode(ctx, email, ""); err != nil {
			return nil, "", err
		}
		code, sent := notes.codes[email]
		if !sent {
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
	notes.sent = nil
	if _, _, err := login("dan@else.org", "Dan"); !errs.Is(err, errs.KindPending) {
		t.Fatalf("stranger must wait for approval: %v", err)
	}
	if len(notes.sent) != 1 || notes.sent[0].To != "ada@example.com" || !strings.Contains(notes.sent[0].Subject, "Dan (dan@else.org)") {
		t.Fatalf("administrators must hear about the request: %+v", notes.sent)
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
	pending, err := svc.Admin.Requests(ctx, ben.ID, "")
	must(err)
	if len(pending) != 1 || pending[0].Email != "dan@else.org" || pending[0].Name != "Dan" {
		t.Fatalf("pending = %+v", pending)
	}
	notes.sent = nil
	if _, err := svc.Admin.Approve(ctx, ben.ID, pending[0].ID); err != nil {
		t.Fatal(err)
	}
	if len(notes.sent) != 1 || notes.sent[0].To != "dan@else.org" {
		t.Fatalf("the approved person must be emailed: %+v", notes.sent)
	}
	dan, _, err := login("dan@else.org", "Dan")
	if err != nil || dan.Name != "Dan" {
		t.Fatalf("approved person must sign in: %v %+v", err, dan)
	}

	// Declined people are told so.
	if _, _, err := login("eve@else.org", "Eve"); !errs.Is(err, errs.KindPending) {
		t.Fatal(err)
	}
	pending, _ = svc.Admin.Requests(ctx, ada.ID, "pending")
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
	_ = admin.StatusPending
}
