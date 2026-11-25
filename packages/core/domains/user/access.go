package user

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
)

// Role is what a person may administer.
type Role string

const (
	// RoleUser has no administrative rights.
	RoleUser Role = "user"
	// RoleManager approves and declines people waiting to join.
	RoleManager Role = "manager"
	// RoleAdmin can do everything: sign-up settings, roles, approvals and
	// disabling accounts.
	RoleAdmin Role = "admin"
)

// ParseRole validates a role name.
func ParseRole(s string) (Role, error) {
	switch r := Role(strings.ToLower(strings.TrimSpace(s))); r {
	case RoleUser, RoleManager, RoleAdmin:
		return r, nil
	}
	return "", errs.Validation("role", "Choose user, manager or admin.")
}

// IsAdmin reports whether u is an active administrator.
func (u *User) IsAdmin() bool { return u.Status == StatusActive && u.Role == RoleAdmin }

// CanApprove reports whether u may approve or decline people waiting to
// join (administrators and managers).
func (u *User) CanApprove() bool {
	return u.Status == StatusActive && (u.Role == RoleAdmin || u.Role == RoleManager)
}

// HashEmail is the SHA-256 of the normalized address, kept for disabled
// accounts so the same email is recognised without storing it.
func HashEmail(email string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(email))))
	return hex.EncodeToString(sum[:])
}

// Disable turns the account off: the email is replaced by its hash and
// Gravatar is switched off. The caller removes sessions and devices.
func (u *User) Disable(now time.Time) error {
	switch u.Status {
	case StatusDisabled:
		return errs.Conflict(u.Name + " is already disabled.")
	case StatusPlaceholder:
		return errs.Validation("userId", "People added by name only have no account to disable.")
	}
	if u.Email != "" {
		u.EmailHash = HashEmail(u.Email)
	}
	u.Email, u.Gravatar, u.Status, u.UpdatedAt = "", false, StatusDisabled, now
	return nil
}

// Enable turns a disabled account back on. The person signs in again with
// the same email, which is recognised by its hash and stored again; until
// then the account is Reenabled.
func (u *User) Enable(now time.Time) error {
	if u.Status != StatusDisabled {
		return errs.Conflict(u.Name + " is not disabled.")
	}
	u.Status, u.UpdatedAt = StatusInvited, now
	return nil
}

// Reenabled reports whether an administrator enabled u again and the
// person has not signed in since. Such an account is invited but, unlike
// people someone added by email, has no address to write to: only the
// hash of its old one, which lets that address sign in whatever the
// sign-up mode.
func (u *User) Reenabled() bool {
	return u.Status == StatusInvited && u.Email == "" && u.EmailHash != ""
}

// Attach stores email again for an account known only by its hash.
func (u *User) Attach(email string) {
	if u.Email == "" && u.EmailHash != "" && HashEmail(email) == u.EmailHash {
		u.Email, u.EmailHash = strings.ToLower(strings.TrimSpace(email)), ""
	}
}
