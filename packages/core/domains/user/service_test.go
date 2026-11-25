package user

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/ports"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/clock"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/google/uuid"
)

type memUsers map[uuid.UUID]*User

func (m memUsers) Get(_ context.Context, id uuid.UUID) (*User, error) {
	u, ok := m[id]
	if !ok {
		return nil, errs.NotFound("User")
	}
	return u, nil
}

func (m memUsers) FindByEmail(_ context.Context, email string) (*User, error) {
	for _, u := range m {
		if u.Email == email {
			return u, nil
		}
	}
	return nil, nil
}

func (m memUsers) List(_ context.Context, ids []uuid.UUID) ([]*User, error) {
	var out []*User
	for _, id := range ids {
		if u, ok := m[id]; ok {
			out = append(out, u)
		}
	}
	return out, nil
}

func (m memUsers) Create(_ context.Context, u *User) error { m[u.ID] = u; return nil }
func (m memUsers) Update(_ context.Context, u *User) error { m[u.ID] = u; return nil }
func (m memUsers) Count(context.Context) (int, error)      { return len(m), nil }
func (m memUsers) Lock(context.Context) error              { return nil }
func (m memUsers) Placeholders(context.Context, uuid.UUID) (int, error) {
	return 0, nil
}

type plenty struct{}

func (plenty) Take(context.Context, string, int, time.Duration, time.Time) (bool, error) {
	return true, nil
}

// downMail is a mail server that refuses every invitation.
type downMail struct{}

func (downMail) LoginCode(context.Context, string, string, time.Duration) error { return nil }
func (downMail) Invite(context.Context, string, string) error {
	return errors.New("connection refused")
}
func (downMail) Notify(context.Context, ports.Notification) {}

// TestInviteReportsMailFailure checks that a failed invitation email leaves
// the invited account in place and is logged, on both ways of inviting.
func TestInviteReportsMailFailure(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	ada := &User{ID: uuid.New(), Name: "Ada", Email: "ada@example.com", Status: StatusActive}
	for _, tc := range []struct {
		name   string
		invite func(s *Service, repo memUsers) (*User, error)
	}{
		{"by email", func(s *Service, _ memUsers) (*User, error) {
			u, _, err := s.Ensure(ctx, "ben@example.com", "Ben", ada)
			if err == nil {
				s.SendInvite(ctx, u, ada)
			}
			return u, err
		}},
		{"a placeholder given an email", func(s *Service, repo memUsers) (*User, error) {
			cy := &User{ID: uuid.New(), Name: "Cy", Status: StatusPlaceholder, CreatedBy: &ada.ID}
			repo[cy.ID] = cy
			return s.Invite(ctx, ada.ID, cy.ID, "cy@example.com")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := memUsers{ada.ID: ada}
			var logs bytes.Buffer
			s := NewService(repo, downMail{}, plenty{}, clock.Fixed{At: now}, slog.New(slog.NewJSONHandler(&logs, nil)))
			u, err := tc.invite(s, repo)
			if err != nil || u.Status != StatusInvited {
				t.Fatalf("the person is invited anyway: %v, %v", u, err)
			}
			if got := strings.Count(logs.String(), `"msg":"user.invite.failed"`); got != 1 {
				t.Fatalf("user.invite.failed logged %d times, want 1: %s", got, &logs)
			}
			if !strings.Contains(logs.String(), u.ID.String()) {
				t.Errorf("the log names the person: %s", &logs)
			}
		})
	}
}
