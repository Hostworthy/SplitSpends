// Package user owns identity and profile settings.
package user

import (
	"net/mail"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/atb00ker/splitspends/backend/internal/core/shared/errs"
	"github.com/atb00ker/splitspends/backend/internal/core/shared/money"
	"github.com/google/uuid"
)

// Status of a user account.
type Status string

const (
	// StatusActive users have logged in at least once.
	StatusActive Status = "active"
	// StatusInvited users were added by someone else and never logged in.
	StatusInvited Status = "invited"
)

// Notifications are per-user email preferences.
type Notifications struct {
	ExpenseAdded    bool `json:"expenseAdded"`
	ExpenseUpdated  bool `json:"expenseUpdated"`
	PaymentReceived bool `json:"paymentReceived"`
	AddedToGroup    bool `json:"addedToGroup"`
}

// DefaultNotifications enables every email.
func DefaultNotifications() Notifications {
	return Notifications{ExpenseAdded: true, ExpenseUpdated: true, PaymentReceived: true, AddedToGroup: true}
}

// User is a person who can take part in expenses.
type User struct {
	ID              uuid.UUID
	Email           string
	Name            string
	DefaultCurrency string
	AvatarColor     string
	Timezone        string
	Status          Status
	Notifications   Notifications
	CreatedAt       time.Time
	UpdatedAt       time.Time
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
		Notifications:   DefaultNotifications(),
		CreatedAt:       now,
		UpdatedAt:       now,
	}, nil
}

// Patch is a partial profile update.
type Patch struct {
	Name            *string
	DefaultCurrency *string
	Timezone        *string
	AvatarColor     *string
	Notifications   *Notifications
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
	if p.Notifications != nil {
		u.Notifications = *p.Notifications
	}
	u.UpdatedAt = now
	return nil
}

// Activate marks an invited user as active after their first login.
func (u *User) Activate(name string, now time.Time) error {
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
