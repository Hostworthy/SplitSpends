// Package user owns identity and profile settings.
package user

import (
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/money"
	"github.com/google/uuid"
)

// Status of a user account.
type Status string

const (
	// StatusActive users have logged in at least once.
	StatusActive Status = "active"
	// StatusInvited users were added by someone else and never logged in.
	StatusInvited Status = "invited"
	// StatusPlaceholder people were added by name only and cannot sign in
	// until someone attaches an email.
	StatusPlaceholder Status = "placeholder"
	// StatusDisabled accounts were turned off by an administrator: they
	// cannot sign in and keep only a hash of their email.
	StatusDisabled Status = "disabled"
)

// User is a person who can take part in expenses.
type User struct {
	ID              uuid.UUID
	Email           string
	Name            string
	DefaultCurrency string
	AvatarColor     string
	// Gravatar shows the picture gravatar.com has for Email, when there is
	// one, instead of initials. Off until the person turns it on.
	Gravatar bool
	Timezone string
	Status   Status
	Role     Role
	// EmailHash identifies the email of a disabled account (see HashEmail)
	// after the address itself was removed.
	EmailHash     string
	Notifications Notifications
	// CreatedBy is who added a placeholder; nil for everyone else.
	CreatedBy *uuid.UUID
	CreatedAt time.Time
	UpdatedAt time.Time
}

var palette = []string{"#00897B", "#5E35B1", "#E53935", "#FB8C00", "#3949AB", "#43A047", "#D81B60", "#00ACC1", "#6D4C41", "#8E24AA"}

var colorPattern = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

// NormalizeEmail lowercases and validates an email address.
func NormalizeEmail(raw string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(raw))
	if email == "" {
		return "", errs.Validation("email", "Email is required.")
	}
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email || len(email) > 254 {
		return "", errs.Validation("email", "Enter a valid email address.")
	}
	return email, nil
}

// NormalizeName trims and validates a display name. Empty falls back to the
// email's local part.
func NormalizeName(raw, email string) (string, error) {
	name := strings.Join(strings.Fields(raw), " ")
	if name == "" && email == "" {
		return "", errs.Validation("name", "Add a name.")
	}
	if name == "" {
		name = strings.SplitN(email, "@", 2)[0]
	}
	if utf8.RuneCountInString(name) > 80 {
		return "", errs.Validation("name", "Name must be 80 characters or fewer.")
	}
	return name, nil
}

// New creates a user.
func New(email, name string, status Status, now time.Time) (*User, error) {
	email, err := NormalizeEmail(email)
	if err != nil {
		return nil, err
	}
	name, err = NormalizeName(name, email)
	if err != nil {
		return nil, err
	}
	id := uuid.New()
	return &User{
		ID:              id,
		Email:           email,
		Name:            name,
		DefaultCurrency: "USD",
		AvatarColor:     palette[int(id[0])%len(palette)],
		Timezone:        "UTC",
		Status:          status,
		Role:            RoleUser,
		Notifications:   DefaultNotifications(),
		CreatedAt:       now,
		UpdatedAt:       now,
	}, nil
}

// NewPlaceholder creates a person known only by name. id may be nil.
func NewPlaceholder(id *uuid.UUID, name string, createdBy uuid.UUID, now time.Time) (*User, error) {
	name, err := NormalizeName(name, "")
	if err != nil {
		return nil, err
	}
	u := &User{ID: uuid.New(), Name: name, DefaultCurrency: "USD", Timezone: "UTC", Status: StatusPlaceholder, Role: RoleUser,
		Notifications: DefaultNotifications(), CreatedBy: &createdBy, CreatedAt: now, UpdatedAt: now}
	if id != nil && *id != uuid.Nil {
		u.ID = *id
	}
	u.AvatarColor = palette[int(u.ID[0])%len(palette)]
	return u, nil
}

// Patch is a partial profile update.
type Patch struct {
	Name            *string
	DefaultCurrency *string
	Timezone        *string
	AvatarColor     *string
	Gravatar        *bool
	Notifications   *NotificationsPatch
}

// Apply validates and applies p.
func (u *User) Apply(p Patch, now time.Time) error {
	if p.Name != nil {
		name, err := NormalizeName(*p.Name, u.Email)
		if err != nil {
			return err
		}
		u.Name = name
	}
	if p.DefaultCurrency != nil {
		c, ok := money.Lookup(*p.DefaultCurrency)
		if !ok {
			return errs.Validation("defaultCurrency", "Unsupported currency.")
		}
		u.DefaultCurrency = c.Code
	}
	if p.Timezone != nil {
		if _, err := time.LoadLocation(*p.Timezone); err != nil || *p.Timezone == "" {
			return errs.Validation("timezone", "Unknown time zone.")
		}
		u.Timezone = *p.Timezone
	}
	if p.AvatarColor != nil {
		if !colorPattern.MatchString(*p.AvatarColor) {
			return errs.Validation("avatarColor", "Use a hex color such as #00897B.")
		}
		u.AvatarColor = strings.ToUpper(*p.AvatarColor)
	}
	if p.Gravatar != nil {
		if *p.Gravatar && u.Email == "" {
			return errs.Validation("gravatar", "Add an email address before using a Gravatar picture.")
		}
		u.Gravatar = *p.Gravatar
	}
	if p.Notifications != nil {
		u.Notifications.Apply(*p.Notifications)
	}
	u.UpdatedAt = now
	return nil
}

// Activate marks an invited user as active after their first login. A
// disabled account comes back only through Enable.
func (u *User) Activate(name string, now time.Time) error {
	if u.Status == StatusDisabled {
		return errs.Forbidden("This account is disabled. Ask an administrator of this server to enable it.")
	}
	if name != "" {
		n, err := NormalizeName(name, u.Email)
		if err != nil {
			return err
		}
		u.Name = n
	}
	u.Status = StatusActive
	u.UpdatedAt = now
	return nil
}
