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
