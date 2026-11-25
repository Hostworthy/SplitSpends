package receipt

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/ports"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/clock"
	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/google/uuid"
)

func TestToScanBalancesTotals(t *testing.T) {
	s := toScan(ports.ReceiptExtract{Currency: "eur", Items: []ports.ExtractItem{{Name: "Pizza", Amount: "12.00"}, {Name: "Beer", Amount: "4.50"}},
		Tax: "1.00", Total: "15.00", Category: "nope", Date: "2026-11-20"}, "USD")
	if s.Currency != "EUR" || s.Category != "general" || s.Date != "2026-11-20" {
		t.Fatalf("normalization: %+v", s)
	}
	var items int64
	for _, it := range s.Items {
		items += it.Amount
	}
	if items+s.Tax+s.Tip != s.Total {
		t.Fatalf("items %d + tax %d + tip %d != total %d", items, s.Tax, s.Tip, s.Total)
	}
	if s.Tax != 0 || s.Items[0].Amount >= 1200 {
		t.Fatalf("discount should reduce items: %+v tax %d", s.Items, s.Tax)
	}
}

func TestSniff(t *testing.T) {
	if sniff([]byte{0xFF, 0xD8, 0xFF, 0xE0}) != "image/jpeg" {
		t.Fatal("jpeg")
	}
	if sniff([]byte("GIF89a")) != "" {
		t.Fatal("gif must be rejected")
	}
}

// pending is a repository holding only receipts no expense uses.
type pending struct {
	Repository
	rows []*Receipt
}

func (p *pending) Create(_ context.Context, r *Receipt) error {
	p.rows = append(p.rows, r)
	return nil
}

func (p *pending) Pending(_ context.Context, userID uuid.UUID) (int, int64, error) {
	count, size := 0, int64(0)
	for _, r := range p.rows {
		if r.UploadedBy == userID {
			count++
			size += r.Size
		}
	}
	return count, size, nil
}

func (p *pending) Delete(_ context.Context, id uuid.UUID) (bool, error) {
	for i, r := range p.rows {
		if r.ID == id {
			p.rows = append(p.rows[:i], p.rows[i+1:]...)
			return true, nil
		}
	}
	return false, nil
}

// blobs counts stored images and can be made to fail.
type blobs struct {
	puts   int
	fail   error
	remove error
}

func (b *blobs) Put(context.Context, string, io.Reader) error {
	b.puts++
	return b.fail
}

func (b *blobs) Open(context.Context, string) (io.ReadCloser, error) {
	return nil, errs.NotFound("File")
}

func (b *blobs) Delete(context.Context, string) error { return b.remove }

type noTx struct{}

func (noTx) Do(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

func jpeg(size int) []byte {
	return append([]byte{0xFF, 0xD8, 0xFF}, bytes.Repeat([]byte{0}, size-3)...)
}

// TestUploadQuota checks that someone holding the most receipts no expense
// uses, by count or by size, cannot upload another, and that nothing is
// stored for the refused upload.
func TestUploadQuota(t *testing.T) {
	ctx := context.Background()
	ada, ben := uuid.New(), uuid.New()
	for _, tc := range []struct {
		name  string
		count int
		size  int64
	}{
		{"count", MaxPending, 1 << 10},
		{"size", 20, MaxPendingSize / 20},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, store := &pending{}, &blobs{}
			for range tc.count {
				repo.rows = append(repo.rows, &Receipt{ID: uuid.New(), UploadedBy: ada, Size: tc.size})
			}
			s := NewService(repo, store, nil, noTx{}, clock.Fixed{At: time.Now()})
			_, err := s.Upload(ctx, ada, jpeg(1<<10))
			var e *errs.Error
			if !errors.As(err, &e) || e.Kind != errs.KindConflict {
				t.Fatalf("want a conflict, got %v", err)
			}
			if store.puts != 0 || len(repo.rows) != tc.count {
				t.Fatalf("a refused upload must store nothing: %d puts, %d rows", store.puts, len(repo.rows))
			}
			if _, err := s.Upload(ctx, ben, jpeg(1<<10)); err != nil {
				t.Fatalf("someone else can still upload: %v", err)
			}
		})
	}
}

// TestUploadUndo checks that a receipt whose image could not be stored is
// removed again, and kept for Cleanup when part of the image may remain.
func TestUploadUndo(t *testing.T) {
	for _, tc := range []struct {
		name   string
		remove error
		rows   int
	}{
		{"removed", nil, 0},
		{"kept", errors.New("storage.delete.failed"), 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, store := &pending{}, &blobs{fail: errors.New("storage.put.failed"), remove: tc.remove}
			s := NewService(repo, store, nil, noTx{}, clock.Fixed{At: time.Now()})
			if _, err := s.Upload(context.Background(), uuid.New(), jpeg(100)); !errors.Is(err, store.fail) {
				t.Fatalf("want the storage error, got %v", err)
			}
			if len(repo.rows) != tc.rows {
				t.Fatalf("want %d receipts left, got %d", tc.rows, len(repo.rows))
			}
		})
	}
}
