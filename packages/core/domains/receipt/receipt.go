// Package receipt stores receipt images and reads them with the optional AI
// adapter.
package receipt

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math/big"
	"strings"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/expense"
	"github.com/Hostworthy/SplitSpends/packages/core/ports"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/clock"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/money"
	"github.com/google/uuid"
)

// MaxSize is the largest accepted upload.
const MaxSize = 10 << 20

// Receipts no expense uses are kept for a day, long enough to finish the
// expense they were uploaded for, and each person may hold only so many of
// them at once.
const (
	MaxPending     = 50
	MaxPendingSize = 200 << 20
	PendingTTL     = 24 * time.Hour
)

// cleanupBatch is how many receipts Cleanup removes per query.
const cleanupBatch = 100

var allowedTypes = map[string]string{
	"image/jpeg": "jpg",
	"image/png":  "png",
	"image/webp": "webp",
	"image/heic": "heic",
	"image/heif": "heif",
}

// Receipt is an uploaded image.
type Receipt struct {
	ID          uuid.UUID
	UploadedBy  uuid.UUID
	ContentType string
	Size        int64
	StorageKey  string
	CreatedAt   time.Time
}

// Repository persists receipt metadata.
type Repository interface {
	Create(ctx context.Context, r *Receipt) error
	Get(ctx context.Context, id uuid.UUID) (*Receipt, error)
	// Visible reports whether userID uploaded the receipt or can see an
	// expense that uses it.
	Visible(ctx context.Context, id, userID uuid.UUID) (bool, error)
	// Owned reports whether userID uploaded the receipt.
	Owned(ctx context.Context, id, userID uuid.UUID) (bool, error)
	// Pending counts the receipts userID uploaded that no expense uses,
	// including deleted expenses, and their total size in bytes.
	Pending(ctx context.Context, userID uuid.UUID) (count int, size int64, err error)
	// Stale lists up to limit receipts created before cutoff that no
	// expense uses, including deleted expenses.
	Stale(ctx context.Context, cutoff time.Time, limit int) ([]*Receipt, error)
	// Delete removes the receipt unless an expense uses it, and reports
	// whether it did.
	Delete(ctx context.Context, id uuid.UUID) (bool, error)
}

// Scan is a receipt read into minor units.
type Scan struct {
	Merchant string
	Date     string
	Currency string
	Category string
	Items    []ScanItem
	Subtotal int64
	Tax      int64
	Tip      int64
	Total    int64
}

// ScanItem is one receipt line.
type ScanItem struct {
	Name     string
	Quantity int
	Amount   int64
}

// Service handles uploads and scans.
type Service struct {
	repo   Repository
	blobs  ports.BlobStore
	reader ports.ReceiptReader
	tx     ports.Tx
	clock  clock.Clock
}

// NewService builds a Service. reader may be nil when AI is disabled.
func NewService(repo Repository, blobs ports.BlobStore, reader ports.ReceiptReader, tx ports.Tx, c clock.Clock) *Service {
	return &Service{repo: repo, blobs: blobs, reader: reader, tx: tx, clock: c}
}

// AIEnabled reports whether scanning is available.
func (s *Service) AIEnabled() bool { return s.reader != nil }

// Upload stores an image after sniffing its real type.
func (s *Service) Upload(ctx context.Context, actor uuid.UUID, data []byte) (*Receipt, error) {
	if len(data) == 0 {
		return nil, errs.Validation("file", "The file is empty.")
	}
	if len(data) > MaxSize {
		return nil, errs.Validation("file", "Receipts are limited to 10 MB.")
	}
	ct := sniff(data)
	ext, ok := allowedTypes[ct]
	if !ok {
		return nil, errs.Validation("file", "Upload a JPEG, PNG, WebP or HEIC image.")
	}
	count, size, err := s.repo.Pending(ctx, actor)
	if err != nil {
		return nil, err
	}
	if count >= MaxPending || size+int64(len(data)) > MaxPendingSize {
		return nil, errs.Conflict("You have too many receipts that are not on an expense yet. Add them to expenses, or try again tomorrow.")
	}
	now := s.clock.Now()
	r := &Receipt{ID: uuid.New(), UploadedBy: actor, ContentType: ct, Size: int64(len(data)), CreatedAt: now}
	r.StorageKey = "receipts/" + now.Format("2006/01") + "/" + r.ID.String() + "." + ext
	// The row comes first so that Cleanup finds the image even when storing
	// it fails halfway or the server stops before the row is removed again.
	if err := s.repo.Create(ctx, r); err != nil {
		return nil, err
	}
	if err := s.blobs.Put(ctx, r.StorageKey, bytes.NewReader(data)); err != nil {
		// When part of the image may remain, the row stays so that Cleanup
		// removes both later.
		if undo := s.blobs.Delete(ctx, r.StorageKey); undo != nil {
			return nil, errors.Join(err, undo)
		}
		if _, undo := s.repo.Delete(ctx, r.ID); undo != nil {
			return nil, errors.Join(err, undo)
		}
		return nil, err
	}
	return r, nil
}

// Cleanup removes receipts that no expense has used for PendingTTL after
// their upload, image first, and returns how many it removed. A receipt on a
// deleted expense stays, since the expense can be restored.
func (s *Service) Cleanup(ctx context.Context) (int, error) {
	cutoff := s.clock.Now().Add(-PendingTTL)
	removed := 0
	for {
		stale, err := s.repo.Stale(ctx, cutoff, cleanupBatch)
		if err != nil || len(stale) == 0 {
			return removed, err
		}
		// A receipt that cannot be removed does not hold up the others; the
		// run stops after this batch so that it is not retried at once.
		var failed []error
		for _, r := range stale {
			// The row is deleted and the image removed in one transaction:
			// when removing the image fails the row stays for the next run,
			// and an expense attaching the receipt meanwhile keeps it.
			deleted := false
			err := s.tx.Do(ctx, func(ctx context.Context) error {
				ok, err := s.repo.Delete(ctx, r.ID)
				if err != nil || !ok {
					return err
				}
				deleted = true
				return s.blobs.Delete(ctx, r.StorageKey)
			})
			if err != nil {
				failed = append(failed, err)
				continue
			}
			if deleted {
				removed++
			}
		}
		if len(failed) > 0 {
			return removed, errors.Join(failed...)
		}
		if len(stale) < cleanupBatch {
			return removed, nil
		}
	}
}

// Open returns a receipt and its bytes if the actor may see it.
func (s *Service) Open(ctx context.Context, actor, id uuid.UUID) (*Receipt, io.ReadCloser, error) {
	ok, err := s.repo.Visible(ctx, id, actor)
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		return nil, nil, errs.NotFound("Receipt")
	}
	r, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	body, err := s.blobs.Open(ctx, r.StorageKey)
	if err != nil {
		return nil, nil, err
	}
	return r, body, nil
}

// Scan extracts line items with the AI reader.
func (s *Service) Scan(ctx context.Context, actor, id uuid.UUID, currency string) (*Scan, error) {
	if s.reader == nil {
		return nil, errs.AIDisabled("AI features are not enabled on this server.")
	}
	r, body, err := s.Open(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	data, err := io.ReadAll(io.LimitReader(body, MaxSize+1))
	if err != nil {
		return nil, err
	}
	ext, err := s.reader.ReadReceipt(ctx, ports.ReceiptInput{Image: data, ContentType: r.ContentType, Currency: currency, Categories: expense.CategoryIDs()})
	if err != nil {
		return nil, err
	}
	return toScan(ext, currency), nil
}

func toScan(ext ports.ReceiptExtract, fallback string) *Scan {
	cur, ok := money.Lookup(ext.Currency)
	if !ok {
		if cur, ok = money.Lookup(fallback); !ok {
			cur, _ = money.Lookup("USD")
		}
	}
	amount := func(s string) int64 {
		v, _ := expense.ParseDecimal(s, cur.Exponent)
		return v
	}
	out := &Scan{Merchant: strings.TrimSpace(ext.Merchant), Currency: cur.Code, Category: ext.Category,
		Subtotal: amount(ext.Subtotal), Tax: amount(ext.Tax), Tip: amount(ext.Tip), Total: amount(ext.Total)}
	if _, err := time.Parse(time.DateOnly, ext.Date); err == nil {
		out.Date = ext.Date
	}
	if !expense.KnownCategory(out.Category) || out.Category == expense.PaymentCategory {
		out.Category = expense.DefaultCategory
	}
	var items int64
	for _, it := range ext.Items {
		a := amount(it.Amount)
		if a == 0 {
			continue
		}
		q := it.Quantity
		if q < 1 {
			q = 1
		}
		out.Items = append(out.Items, ScanItem{Name: strings.TrimSpace(it.Name), Quantity: q, Amount: a})
		items += a
	}
	if out.Subtotal == 0 {
		out.Subtotal = items
	}
	if out.Total == 0 {
		out.Total = out.Subtotal + out.Tax + out.Tip
	}
	// Discounts and rounding lines often go unlisted; fold the gap into tax so
	// items + tax + tip always equals the total the user sees.
	if gap := out.Total - items - out.Tax - out.Tip; gap != 0 && len(out.Items) > 0 {
		out.Tax += gap
		if out.Tax < 0 {
			discount(out.Items, out.Tax)
			out.Tax = 0
		}
	}
	return out
}

// discount spreads a negative amount over items in proportion to their price,
// so a receipt-level discount still yields positive, assignable items.
func discount(items []ScanItem, amount int64) {
	weights := make([]*big.Rat, len(items))
	for i, it := range items {
		weights[i] = big.NewRat(max(it.Amount, 0), 1)
	}
	parts, err := money.Allocate(amount, weights)
	if err != nil {
		return
	}
	for i := range items {
		items[i].Amount = max(items[i].Amount+parts[i], 1)
	}
}

func sniff(data []byte) string {
	switch {
	case bytes.HasPrefix(data, []byte{0xFF, 0xD8, 0xFF}):
		return "image/jpeg"
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		return "image/png"
	case len(data) > 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		return "image/webp"
	case len(data) > 12 && string(data[4:8]) == "ftyp":
		brand := string(data[8:12])
		if strings.HasPrefix(brand, "hei") || strings.HasPrefix(brand, "hev") || brand == "mif1" || brand == "msf1" {
			return "image/heic"
		}
	}
	return ""
}
