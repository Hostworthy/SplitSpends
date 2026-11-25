// Package admin is server administration: who may sign up, approving people
// who wait to join, roles, and disabling accounts.
package admin

import (
	"context"
	"strings"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/auth"
	"github.com/Hostworthy/SplitSpends/packages/core/domains/user"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/google/uuid"
)

// MaxAllowed bounds the allowed list.
const MaxAllowed = 500

// Settings are the sign-up rules an administrator chose.
type Settings struct {
	auth.Signup
	UpdatedAt time.Time
	UpdatedBy *uuid.UUID
}

// Status of a request to join.
type Status string

const (
	StatusPending  Status = "pending"
	StatusApproved Status = "approved"
	StatusDeclined Status = "declined"
)

// Request is someone who proved their email and waits for approval.
type Request struct {
	ID          uuid.UUID
	Email       string
	Name        string
	Status      Status
	RequestedAt time.Time
	DecidedAt   *time.Time
	DecidedBy   *uuid.UUID
}

// SettingsRepository stores the one row of sign-up settings.
type SettingsRepository interface {
	// Get returns nil when an administrator never changed the settings.
	Get(ctx context.Context) (*Settings, error)
	Save(ctx context.Context, s *Settings) error
}

// RequestRepository stores requests to join, one per email.
type RequestRepository interface {
	// Find returns nil when email never asked to join.
	Find(ctx context.Context, email string) (*Request, error)
	Get(ctx context.Context, id uuid.UUID) (*Request, error)
	Save(ctx context.Context, r *Request) error
	List(ctx context.Context, status Status) ([]*Request, error)
}

// Accounts are the user queries administration needs.
type Accounts interface {
	Staff(ctx context.Context) ([]*user.User, error)
	Admins(ctx context.Context) (int, error)
	Accounts(ctx context.Context, status, q string, limit, offset int) ([]*user.User, int, error)
}

// Credentials removes everything that lets a person in: sessions, API
// tokens, push devices and the login codes sent to their email.
type Credentials interface {
	Revoke(ctx context.Context, userID uuid.UUID, email string) error
}

// Normalize checks the mode and cleans the allowed list: addresses are
// lowercased, "example.com" becomes "@example.com", duplicates go.
func Normalize(mode string, allowed []string) (auth.Signup, error) {
	s := auth.Signup{Mode: auth.SignupMode(strings.ToLower(strings.TrimSpace(mode)))}
	valid := false
	for _, m := range auth.Modes {
		valid = valid || m == s.Mode
	}
	if !valid {
		return s, errs.Validation("mode", "Choose open, invite, approval or closed.")
	}
	if len(allowed) > MaxAllowed {
		return s, errs.Validation("allowed", "The allowed list is limited to 500 entries.")
	}
	seen := map[string]bool{}
	s.Allowed = []string{}
	for _, raw := range allowed {
		entry := strings.ToLower(strings.TrimSpace(raw))
		if entry == "" {
			continue
		}
		switch {
		case strings.HasPrefix(entry, "@") || !strings.Contains(entry, "@"):
			domain := strings.TrimPrefix(entry, "@")
			if !domainPattern(domain) {
				return s, errs.Validation("allowed", "“"+raw+"” is not an email address or a domain like @example.com.")
			}
			entry = "@" + domain
		default:
			email, err := user.NormalizeEmail(entry)
			if err != nil {
				return s, errs.Validation("allowed", "“"+raw+"” is not an email address or a domain like @example.com.")
			}
			entry = email
		}
		if !seen[entry] {
			seen[entry] = true
			s.Allowed = append(s.Allowed, entry)
		}
	}
	return s, nil
}

func domainPattern(d string) bool {
	if len(d) < 3 || len(d) > 253 || !strings.Contains(d, ".") || strings.HasPrefix(d, ".") || strings.HasSuffix(d, ".") {
		return false
	}
	for _, r := range d {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '.':
		default:
			return false
		}
	}
	return true
}
