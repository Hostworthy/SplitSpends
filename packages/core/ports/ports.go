// Package ports declares the outbound interfaces the domain depends on.
// Adapters in database, notifications, storage and ai implement them; the
// composition root in bootstrap wires them together.
package ports

import (
	"context"
	"io"
	"time"
)

// Tx runs fn inside a single database transaction. Repositories called with
// the ctx passed to fn join that transaction.
type Tx interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) error
}

// Notifier delivers user-facing messages (email today). Implementations should
// not block the caller on slow delivery for non-critical messages.
type Notifier interface {
	LoginCode(ctx context.Context, to, code string, ttl time.Duration) error
	Invite(ctx context.Context, to, inviterName string) error
	Notify(ctx context.Context, n Notification)
}

// Notification is a fan-out message about something that happened.
type Notification struct {
	To      string
	Name    string
	Subject string
	Lines   []string
	Link    string
}

// BlobStore persists binary objects such as receipt images.
type BlobStore interface {
	Put(ctx context.Context, key string, r io.Reader) error
	Open(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}

// ReceiptReader turns a receipt image into structured line items.
type ReceiptReader interface {
	ReadReceipt(ctx context.Context, in ReceiptInput) (ReceiptExtract, error)
	DraftExpense(ctx context.Context, in DraftInput) (DraftExtract, error)
}

// ReceiptInput is the image plus hints for extraction.
type ReceiptInput struct {
	Image       []byte
	ContentType string
	Currency    string
	Categories  []string
}

// Decimal amounts in extracts are strings such as "12.50" so no float ever
// carries money. The domain converts them to minor units.
type ReceiptExtract struct {
	Merchant string
	Date     string
	Currency string
	Category string
	Items    []ExtractItem
	Subtotal string
	Tax      string
	Tip      string
	Total    string
}

// ExtractItem is a single receipt line.
type ExtractItem struct {
	Name     string
	Quantity int
	Amount   string
}

// DraftInput is everything the AI provider needs to turn the user's
// instructions (and optionally a receipt photo) into an expense draft.
// The application builds the system prompt from it; the model runs outside.
type DraftInput struct {
	Text        string
	Today       string
	Currency    string
	Me          Person
	People      []Person
	Groups      []GroupOption
	GroupID     string // preselected group, empty when the user did not pick one
	Categories  []string
	Image       []byte // optional receipt photo
	ContentType string
}

// Person is a candidate participant the model may reference by id.
type Person struct {
	ID    string
	Name  string
	Email string
}

// GroupOption is a group the user belongs to.
type GroupOption struct {
	ID        string
	Name      string
	Currency  string
	MemberIDs []string
}

// DraftExtract is the model's reading of the instructions. Amounts are
// decimal strings; ids must come from DraftInput.
type DraftExtract struct {
	Description    string
	Amount         string
	Currency       string
	Date           string
	Category       string
	GroupID        string
	PayerID        string
	ParticipantIDs []string
	Items          []DraftItem
	Tax            string
	Tip            string
	Unmatched      []string
}

// DraftItem is a receipt line and who shares it.
type DraftItem struct {
	Name           string
	Amount         string
	ParticipantIDs []string
}

type quietKey struct{}

// Quiet marks ctx so services skip notifications, for example during a bulk
// import of someone's offline history.
func Quiet(ctx context.Context) context.Context { return context.WithValue(ctx, quietKey{}, true) }

// IsQuiet reports whether notifications are suppressed for ctx.
func IsQuiet(ctx context.Context) bool { v, _ := ctx.Value(quietKey{}).(bool); return v }
