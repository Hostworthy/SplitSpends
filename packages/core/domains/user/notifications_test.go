package user

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

func active(t *testing.T) *User {
	t.Helper()
	u, err := New("ada@example.com", "Ada", StatusActive, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestNoticeChannels(t *testing.T) {
	off := false
	cases := []struct {
		name      string
		patch     NotificationsPatch
		status    Status
		kind      Kind
		email     bool
		push      bool
		delivered bool
	}{
		{name: "defaults send both", status: StatusActive, kind: ExpenseAdded, email: true, push: true, delivered: true},
		{name: "email off keeps push", patch: NotificationsPatch{SwitchesPatch: SwitchesPatch{ExpenseAdded: &off}},
			status: StatusActive, kind: ExpenseAdded, push: true, delivered: true},
		{name: "push off keeps email", patch: NotificationsPatch{Push: &SwitchesPatch{PaymentReceived: &off}},
			status: StatusActive, kind: PaymentReceived, email: true, delivered: true},
		{name: "both off sends nothing", patch: NotificationsPatch{SwitchesPatch: SwitchesPatch{AddedToGroup: &off}, Push: &SwitchesPatch{AddedToGroup: &off}},
			status: StatusActive, kind: AddedToGroup},
		{name: "other kinds unaffected", patch: NotificationsPatch{SwitchesPatch: SwitchesPatch{ExpenseAdded: &off}, Push: &SwitchesPatch{ExpenseAdded: &off}},
			status: StatusActive, kind: ExpenseUpdated, email: true, push: true, delivered: true},
		{name: "approvals can be turned off", patch: NotificationsPatch{SwitchesPatch: SwitchesPatch{Approvals: &off}, Push: &SwitchesPatch{Approvals: &off}},
			status: StatusActive, kind: Approvals},
		{name: "invited users get nothing", status: StatusInvited, kind: ExpenseAdded},
		{name: "unknown kinds default on", status: StatusActive, kind: Kind("settleUpReminder"), email: true, push: true, delivered: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u := active(t)
			u.Status = tc.status
			u.Notifications.Apply(tc.patch)
			n, ok := u.Notice(tc.kind, "Subject", []string{"Line"}, "https://split.example.com")
			if ok != tc.delivered {
				t.Fatalf("delivered = %v, want %v", ok, tc.delivered)
			}
			if got := n.To != ""; got != tc.email {
				t.Fatalf("email = %v, want %v", got, tc.email)
			}
			if got := n.UserID != uuid.Nil; got != tc.push {
				t.Fatalf("push = %v, want %v", got, tc.push)
			}
			if n.Kind != string(tc.kind) || n.Subject != "Subject" || n.Name != "Ada" {
				t.Fatalf("unexpected notification %+v", n)
			}
		})
	}
}

func TestNotificationsJSON(t *testing.T) {
	// Rows saved before push existed hold only the email switches; push
	// starts fully on and the email choices survive.
	n := DefaultNotifications()
	if err := json.Unmarshal([]byte(`{"expenseAdded":false,"expenseUpdated":true,"paymentReceived":true,"addedToGroup":false}`), &n); err != nil {
		t.Fatal(err)
	}
	if n.ExpenseAdded || n.AddedToGroup || !n.ExpenseUpdated {
		t.Fatalf("email switches lost: %+v", n.Switches)
	}
	if n.Push != DefaultNotifications().Push {
		t.Fatalf("push should default on: %+v", n.Push)
	}
	if !n.Approvals {
		t.Fatalf("switches added later should default on: %+v", n.Switches)
	}
	b, err := json.Marshal(n)
	if err != nil {
		t.Fatal(err)
	}
	var shape map[string]any
	if err := json.Unmarshal(b, &shape); err != nil {
		t.Fatal(err)
	}
	if shape["expenseAdded"] != false || shape["push"].(map[string]any)["expenseAdded"] != true {
		t.Fatalf("email keys must stay top-level and push nested: %s", b)
	}
}

func TestPatchKeepsOmitted(t *testing.T) {
	u := active(t)
	off := false
	if err := u.Apply(Patch{Notifications: &NotificationsPatch{Push: &SwitchesPatch{ExpenseUpdated: &off}}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if !u.Notifications.ExpenseUpdated || u.Notifications.Push.ExpenseUpdated || !u.Notifications.Push.ExpenseAdded {
		t.Fatalf("patch touched more than it named: %+v", u.Notifications)
	}
}
