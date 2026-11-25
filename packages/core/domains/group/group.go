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

// MaxMembers is how many people a group can have, the creator included.
const MaxMembers = 100

// MaxPerUser is how many groups one person can belong to, archived ones
// included. Their group list and balances load all of them.
const MaxPerUser = 1000

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
	// DefaultSplit is the stored default split; Effective says whether it
	// still applies to the current members.
	DefaultSplit *DefaultSplit
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
	// DefaultSplit sets the default split, validated against the current
	// members. One with an empty Method clears it.
	DefaultSplit *DefaultSplit
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
	if p.DefaultSplit != nil {
		if p.DefaultSplit.Method == "" {
			g.DefaultSplit = nil
		} else {
			d, err := p.DefaultSplit.Validate(g.Members)
			if err != nil {
				return err
			}
			g.DefaultSplit = d
		}
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
	// Create returns errs.Conflict when the id is taken.
	Create(ctx context.Context, g *Group) error
	// Get returns errs.NotFound when missing; Members is filled.
	Get(ctx context.Context, id uuid.UUID) (*Group, error)
	// Lock is Get inside the caller's transaction, with the group row locked
	// until it ends, so nothing can be added to the group meanwhile.
	Lock(ctx context.Context, id uuid.UUID) (*Group, error)
	// Hold keeps the group from being locked by Lock until the caller's
	// transaction ends, without blocking other holders, so the members stay
	// as they are while an expense is booked. It returns errs.NotFound when
	// the group is missing.
	Hold(ctx context.Context, id uuid.UUID) error
	Update(ctx context.Context, g *Group) error
	Delete(ctx context.Context, id uuid.UUID) error
	// ForUser lists the user's groups with Members filled.
	ForUser(ctx context.Context, userID uuid.UUID, archived bool) ([]*Group, error)
	// Joined counts the groups the user is an active member of.
	Joined(ctx context.Context, userID uuid.UUID) (int, error)
	AddMember(ctx context.Context, groupID, userID uuid.UUID, at time.Time) error
	RemoveMember(ctx context.Context, groupID, userID uuid.UUID, at time.Time) error
}
