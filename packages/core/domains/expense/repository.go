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
	// Create returns errs.Conflict when the id is taken.
	Create(ctx context.Context, e *Expense) error
	// Get returns errs.NotFound when missing. Deleted expenses are returned.
	Get(ctx context.Context, id uuid.UUID) (*Expense, error)
	// Lock is Get for a write: the row stays locked until the current
	// transaction ends.
	Lock(ctx context.Context, id uuid.UUID) (*Expense, error)
	// Update replaces the expense and its shares.
	Update(ctx context.Context, e *Expense) error
	List(ctx context.Context, f Filter) ([]*Expense, int, error)
	// Due returns live recurring expenses whose next date is on or before day.
	Due(ctx context.Context, day time.Time, limit int) ([]*Expense, error)
	// Claim locks the expense for the current transaction and returns it
	// while it is still due on day. It returns nil when the expense is no
	// longer due or another transaction holds it.
	Claim(ctx context.Context, id uuid.UUID, day time.Time) (*Expense, error)
	// EndSeries stops the expense from recurring. It changes nothing else.
	EndSeries(ctx context.Context, id uuid.UUID, at time.Time) error
}

// MaxComments is how many comments one expense can hold. Everyone in the
// group loads them when they open the expense.
const MaxComments = 500

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
	// List returns a page of an expense's comments, oldest first, and how
	// many it has. A limit of 0 returns them all.
	List(ctx context.Context, expenseID uuid.UUID, limit, offset int) ([]*Comment, int, error)
	// Count is how many comments the expense has.
	Count(ctx context.Context, expenseID uuid.UUID) (int, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

// Receipts checks that a receipt may be attached by a user.
type Receipts interface {
	// Owned reports whether userID uploaded the receipt.
	Owned(ctx context.Context, receiptID, userID uuid.UUID) (bool, error)
}

// Catalog says who may file expenses under a custom category.
type Catalog interface {
	// Usable reports whether userID owns the live custom category id.
	Usable(ctx context.Context, userID uuid.UUID, id string) (bool, error)
}
