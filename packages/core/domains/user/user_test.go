package user

import (
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
)

func TestActivate(t *testing.T) {
	now := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		status Status
		want   Status
		kind   errs.Kind
	}{
		{name: "invited", status: StatusInvited, want: StatusActive},
		{name: "disabled", status: StatusDisabled, want: StatusDisabled, kind: errs.KindForbidden},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			u := &User{Name: "Ada", Email: "ada@example.com", Status: tc.status}
			err := u.Activate("Ada", now)
			if tc.kind == "" && err != nil || tc.kind != "" && !errs.Is(err, tc.kind) {
				t.Fatalf("activate: %v", err)
			}
			if u.Status != tc.want {
				t.Fatalf("status = %s, want %s", u.Status, tc.want)
			}
		})
	}
}

func TestReenabled(t *testing.T) {
	now := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	active := func() *User {
		return &User{Name: "Ben", Email: "ben@example.com", Status: StatusActive}
	}
	disabled := func() *User {
		u := active()
		if err := u.Disable(now); err != nil {
			t.Fatal(err)
		}
		return u
	}
	enabled := func() *User {
		u := disabled()
		if err := u.Enable(now); err != nil {
			t.Fatal(err)
		}
		return u
	}
	signedIn := func() *User {
		u := enabled()
		u.Attach("ben@example.com")
		if err := u.Activate("", now); err != nil {
			t.Fatal(err)
		}
		return u
	}
	for _, tc := range []struct {
		name string
		user *User
		want bool
	}{
		{"active", active(), false},
		{"invited", &User{Name: "Ben", Email: "ben@example.com", Status: StatusInvited}, false},
		{"disabled", disabled(), false},
		{"enabled again", enabled(), true},
		{"signed in after being enabled", signedIn(), false},
	} {
		if got := tc.user.Reenabled(); got != tc.want {
			t.Errorf("%s: Reenabled = %v, want %v (%+v)", tc.name, got, tc.want, tc.user)
		}
	}
}
