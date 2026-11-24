// Package group owns groups and their membership.
package group

import (
	"context"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/money"
	"github.com/google/uuid"
)

// Kind is the purpose of a group, used for defaults and icons.
type Kind string

const (
	KindHome   Kind = "home"
	KindTrip   Kind = "trip"
	KindCouple Kind = "couple"
	KindOther  Kind = "other"
)

func (k Kind) valid() bool {
	switch k {
	case KindHome, KindTrip, KindCouple, KindOther:
		return true
	}
	return false
}

var colorPattern = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

// Group is a set of people who share expenses.
type Group struct {
	ID              uuid.UUID
	Name            string
	Kind            Kind
	DefaultCurrency string
	SimplifyDebts   bool
	Emoji           string
	Color           string
	CreatedBy       uuid.UUID
	ArchivedAt      *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
	// Members is a read-model field with active member ids.
	Members []uuid.UUID
}

// Patch is a partial update.
type Patch struct {
	Name            *string
	Kind            *Kind
	DefaultCurrency *string
	SimplifyDebts   *bool
	Emoji           *string
	Color           *string
	Archived        *bool
}

// New creates a group.
func New(name string, createdBy uuid.UUID, currency string, now time.Time) (*Group, error) {
	g := &Group{ID: uuid.New(), Kind: KindOther, SimplifyDebts: true, Emoji: "👥", Color: "#00897B", CreatedBy: createdBy, CreatedAt: now, UpdatedAt: now}
	if err := g.Apply(Patch{Name: &name, DefaultCurrency: &currency}, now); err != nil {
		return nil, err
	}
	return g, nil
}

// Apply validates and applies p.
func (g *Group) Apply(p Patch, now time.Time) error {
	if p.Name != nil {
		name := strings.Join(strings.Fields(*p.Name), " ")
		if name == "" {
			return errs.Validation("name", "Give the group a name.")
		}
		if utf8.RuneCountInString(name) > 60 {
			return errs.Validation("name", "Group names are limited to 60 characters.")
		}
		g.Name = name
	}
	if p.Kind != nil {
		if !p.Kind.valid() {
			return errs.Validation("kind", "Unknown group type.")
		}
		g.Kind = *p.Kind
	}
	if p.DefaultCurrency != nil {
		c, ok := money.Lookup(*p.DefaultCurrency)
		if !ok {
			return errs.Validation("defaultCurrency", "Unsupported currency.")
		}
		g.DefaultCurrency = c.Code
	}
	if p.SimplifyDebts != nil {
		g.SimplifyDebts = *p.SimplifyDebts
	}
	if p.Emoji != nil {
		e := strings.TrimSpace(*p.Emoji)
		if utf8.RuneCountInString(e) > 8 {
			return errs.Validation("emoji", "Use a single emoji.")
		}
		g.Emoji = e
	}
	if p.Color != nil {
		if !colorPattern.MatchString(*p.Color) {
			return errs.Validation("color", "Use a hex color such as #00897B.")
		}
		g.Color = strings.ToUpper(*p.Color)
	}
	if p.Archived != nil {
		if *p.Archived && g.ArchivedAt == nil {
			g.ArchivedAt = &now
		} else if !*p.Archived {
			g.ArchivedAt = nil
		}
	}
	g.UpdatedAt = now
	return nil
}

// HasMember reports whether id is an active member.
func (g *Group) HasMember(id uuid.UUID) bool {
	for _, m := range g.Members {
		if m == id {
			return true
		}
	}
	return false
}

// Repository persists groups and memberships.
type Repository interface {
	Create(ctx context.Context, g *Group) error
	// Get returns errs.NotFound when missing; Members is filled.
	Get(ctx context.Context, id uuid.UUID) (*Group, error)
	Update(ctx context.Context, g *Group) error
	Delete(ctx context.Context, id uuid.UUID) error
	// ForUser lists the user's groups with Members filled.
	ForUser(ctx context.Context, userID uuid.UUID, archived bool) ([]*Group, error)
	AddMember(ctx context.Context, groupID, userID uuid.UUID, at time.Time) error
	RemoveMember(ctx context.Context, groupID, userID uuid.UUID, at time.Time) error
}
