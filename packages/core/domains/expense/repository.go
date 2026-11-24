package expense

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Filter narrows expense lists. Results are always limited to what UserID
// may see: group expenses of their active groups and non-group expenses they
// take part in.
type Filter struct {
	UserID         uuid.UUID
	GroupID        *uuid.UUID
	FriendID       *uuid.UUID
	Query          string
	Category       string
	Kind           Kind
	From           *time.Time
	To             *time.Time
	IncludeDeleted bool
	Limit          int
	Offset         int
}

// Repository persists expenses and their shares.
type Repository interface {
	Create(ctx context.Context, e *Expense) error
	// Get returns errs.NotFound when missing. Deleted expenses are returned.
	Get(ctx context.Context, id uuid.UUID) (*Expense, error)
	// Update replaces the expense and its shares.
	Update(ctx context.Context, e *Expense) error
	List(ctx context.Context, f Filter) ([]*Expense, int, error)
	// Due returns live recurring expenses whose next date is on or before day.
	Due(ctx context.Context, day time.Time, limit int) ([]*Expense, error)
}

// Comment is a note on an expense.
type Comment struct {
	ID        uuid.UUID
	ExpenseID uuid.UUID
	UserID    uuid.UUID
	Body      string
	CreatedAt time.Time
}

// CommentRepository persists comments.
type CommentRepository interface {
	Create(ctx context.Context, c *Comment) error
	Get(ctx context.Context, id uuid.UUID) (*Comment, error)
	List(ctx context.Context, expenseID uuid.UUID) ([]*Comment, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

// Receipts checks that a receipt may be attached by a user.
type Receipts interface {
	Owned(ctx context.Context, receiptID, userID uuid.UUID) (bool, error)
}

// Catalog says who may file expenses under a custom category.
type Catalog interface {
	// Usable reports whether userID owns the live custom category id.
	Usable(ctx context.Context, userID uuid.UUID, id string) (bool, error)
}
