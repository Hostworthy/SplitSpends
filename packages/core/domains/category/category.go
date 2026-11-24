// Package category owns user-defined expense categories. Built-in categories
// live in the expense package; a custom category adds a name, an icon and a
// display group for one user, under an id of the form custom.<uuid>.
//
// Deleting a custom category hides it from its owner's picker and from
// listings, but never rewrites history: expenses keep the id and clients show
// them as General (the stats merge them into general too). Expenses that
// already use a category can keep it when edited, by anyone who may edit
// them.
package category

import (
	"context"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/expense"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/google/uuid"
)

// MaxPerUser bounds how many live custom categories one person keeps.
const MaxPerUser = 200

// Icons are the Material Symbols names a custom category may use: the icons
// of the built-in categories plus a few general ones. Clients map each to a
// glyph, so the list only grows together with the clients.
var Icons = []string{
	"restaurant", "local_grocery_store", "local_bar", "local_cafe",
	"home", "bolt", "wifi", "cleaning_services", "chair", "build", "pets",
	"local_gas_station", "local_parking", "local_taxi", "directions_bus", "directions_car",
	"flight", "hotel", "attractions",
	"movie", "sports_esports", "music_note", "sports_soccer", "subscriptions",
	"checkroom", "redeem", "medical_services", "school", "child_care", "shield", "account_balance",
	"shopping_bag", "receipt_long",
	"label", "star", "favorite", "work", "fitness_center", "park", "celebration", "cake",
	"savings", "devices", "menu_book", "palette", "volunteer_activism", "local_laundry_service", "content_cut",
}

// DefaultIcon is used when none is given.
const DefaultIcon = "label"

// DefaultGroup is the display group used when none is given.
const DefaultGroup = "Other"

// Groups returns the display groups a custom category may join: the groups
// of the built-in categories, in display order.
func Groups() []string {
	var out []string
	for _, c := range expense.Categories() {
		if !slices.Contains(out, c.Group) {
			out = append(out, c.Group)
		}
	}
	return out
}

// Category is a spending category. Built-in categories have no owner.
type Category struct {
	ID        string
	OwnerID   uuid.UUID
	Name      string
	Icon      string
	Group     string
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
}

// Custom reports whether c is user-defined.
func (c *Category) Custom() bool { return c.OwnerID != uuid.Nil }

// Input creates or changes a custom category. Nil fields stay unchanged.
type Input struct {
	// ID lets offline clients choose the id (custom.<uuid>) so retries and
	// imports are idempotent.
	ID    *string
	Name  *string
	Icon  *string
	Group *string
}

// NewID returns a fresh custom category id.
func NewID() string { return expense.CustomPrefix + uuid.NewString() }

// New creates a custom category owned by owner.
func New(owner uuid.UUID, in Input, now time.Time) (*Category, error) {
	id := NewID()
	if in.ID != nil && *in.ID != "" {
		id = strings.ToLower(strings.TrimSpace(*in.ID))
		if !expense.CustomCategory(id) {
			return nil, errs.Validation("id", "Custom category ids look like custom.<uuid>.")
		}
	}
	c := &Category{ID: id, OwnerID: owner, Icon: DefaultIcon, Group: DefaultGroup, CreatedAt: now}
	name := ""
	if in.Name != nil {
		name = *in.Name
	}
	in.Name = &name
	if err := c.Apply(in, now); err != nil {
		return nil, err
	}
	return c, nil
}

// Apply validates and applies in.
func (c *Category) Apply(in Input, now time.Time) error {
	if in.Name != nil {
		name := strings.Join(strings.Fields(*in.Name), " ")
		if name == "" {
			return errs.Validation("name", "Give the category a name.")
		}
		if utf8.RuneCountInString(name) > 40 {
			return errs.Validation("name", "Category names are limited to 40 characters.")
		}
		c.Name = name
	}
	if in.Icon != nil {
		icon := strings.TrimSpace(*in.Icon)
		if icon == "" {
			icon = DefaultIcon
		}
		if !slices.Contains(Icons, icon) {
			return errs.Validation("icon", "Choose one of the offered icons.")
		}
		c.Icon = icon
	}
	if in.Group != nil {
		group := strings.TrimSpace(*in.Group)
		if group == "" {
			group = DefaultGroup
		}
		if !slices.Contains(Groups(), group) {
			return errs.Validation("group", "Choose one of the category groups.")
		}
		c.Group = group
	}
	c.UpdatedAt = now
	return nil
}

// Repository persists custom categories.
type Repository interface {
	Create(ctx context.Context, c *Category) error
	// Get returns errs.NotFound when missing. Deleted categories are returned.
	Get(ctx context.Context, id string) (*Category, error)
	// Update saves every field, including DeletedAt.
	Update(ctx context.Context, c *Category) error
	// Owned lists the owner's live categories.
	Owned(ctx context.Context, owner uuid.UUID) ([]*Category, error)
	// Visible lists live categories owned by userID plus live categories used
	// by expenses userID can see.
	Visible(ctx context.Context, userID uuid.UUID) ([]*Category, error)
	// Live returns the live categories among ids.
	Live(ctx context.Context, ids []string) ([]*Category, error)
}
