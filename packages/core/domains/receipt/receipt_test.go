package receipt

import (
	"bytes"
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/domains/expense"
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

func TestReconcile(t *testing.T) {
	cases := []struct {
		name  string
		items []int64
		tip   int64
		total int64
		want  []int64
		tax   int64
		ok    bool
	}{
		{"gap goes to tax", []int64{1000, 500}, 100, 1800, []int64{1000, 500}, 200, true},
		{"lines match", []int64{1000}, 0, 1000, []int64{1000}, 0, true},
		{"order discount spread by price", []int64{3000, 2000}, 0, 4500, []int64{2700, 1800}, 0, true},
		{"discount keeps a cent per item", []int64{1000, 1}, 0, 2, []int64{1, 1}, 0, true},
		{"discount larger than items", []int64{1000, 1}, 0, 1, nil, 0, false},
		{"tip above total", []int64{1000}, 500, 100, nil, 0, false},
		{"zero item", []int64{1000, 0}, 0, 1000, nil, 0, false},
		{"no items", nil, 0, 1000, nil, 0, false},
		{"unknown total", []int64{1000}, 0, 0, nil, 0, false},
		{"items above the maximum", []int64{expense.MaxTotal, 1}, 0, expense.MaxTotal, nil, 0, false},
	}
	for _, c := range cases {
		got, tax, ok := Reconcile(c.items, c.tip, c.total)
		if ok != c.ok || tax != c.tax || !slices.Equal(got, c.want) {
			t.Errorf("%s: got %v tax %d ok %v, want %v tax %d ok %v", c.name, got, tax, ok, c.want, c.tax, c.ok)
			continue
		}
		if ok {
			sum := tax + c.tip
			for _, a := range got {
				sum += a
			}
			if sum != c.total {
				t.Errorf("%s: items + tax + tip = %d, want %d", c.name, sum, c.total)
			}
		}
	}
}

func TestToScanRejectsBadAmounts(t *testing.T) {
	item := func(amounts ...string) []ports.ExtractItem {
		var list []ports.ExtractItem
		for _, a := range amounts {
			list = append(list, ports.ExtractItem{Name: "Line", Amount: a})
		}
		return list
	}
	cases := []struct {
		name     string
		ext      ports.ReceiptExtract
		items    int
		itemized bool
	}{
		{"negative item", ports.ReceiptExtract{Items: item("10.00", "-3.00"), Total: "4.00"}, 1, true},
		{"negative tax and tip", ports.ReceiptExtract{Items: item("10.00"), Tax: "-1.00", Tip: "-2", Total: "12.00"}, 1, true},
		{"no positive item", ports.ReceiptExtract{Items: item("-10.00", "0"), Tip: "20", Total: "5.00"}, 0, false},
		{"tip above total", ports.ReceiptExtract{Items: item("10.00"), Tip: "5", Total: "1.00"}, 1, true},
		{"sum beyond int64", ports.ReceiptExtract{Items: item("90000000000000000", "90000000000000000")}, 0, false},
		{"sum beyond the maximum", ports.ReceiptExtract{Items: item("9000000000", "9000000000"), Total: "1.00"}, 2, true},
		{"exponent", ports.ReceiptExtract{Items: item("1e3", "2.00"), Tax: "1/3", Total: "1e3"}, 1, true},
	}
	for _, c := range cases {
		s := toScan(c.ext, "USD")
		var items int64
		for _, it := range s.Items {
			if it.Amount <= 0 {
				t.Errorf("%s: item %d is not positive", c.name, it.Amount)
			}
			items += it.Amount
		}
		if len(s.Items) != c.items || s.Tax < 0 || s.Tip < 0 || s.Total < 0 || s.Subtotal < 0 {
			t.Errorf("%s: %+v", c.name, s)
		}
		if c.itemized && items+s.Tax+s.Tip != s.Total {
			t.Errorf("%s: items %d + tax %d + tip %d != total %d", c.name, items, s.Tax, s.Tip, s.Total)
		}
	}
}

// ftyp is the start of an ISO media file with a major brand and compatible
// brands.
func ftyp(major string, compatible ...string) []byte {
	box := []byte{0, 0, 0, byte(16 + 4*len(compatible))}
	box = append(box, "ftyp"+major+"\x00\x00\x00\x00"...)
	for _, c := range compatible {
		box = append(box, c...)
	}
	return append(box, "\x00\x00\x00\x08meta"...)
}

func TestSniff(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
		want string
	}{
		{"jpeg", []byte{0xFF, 0xD8, 0xFF, 0xE0}, "image/jpeg"},
		{"gif", []byte("GIF89a"), ""},
		{"heic", ftyp("heic", "mif1", "heic"), "image/heic"},
		{"heix", ftyp("heix", "mif1"), "image/heic"},
		{"heif with heic inside", ftyp("mif1", "mif1", "heic"), "image/heic"},
		{"avif", ftyp("avif", "mif1", "avif"), ""},
		{"heif with avif inside", ftyp("mif1", "mif1", "avif", "miaf"), ""},
		{"video", ftyp("isom", "isom", "mp41"), ""},
		{"truncated box", ftyp("mif1", "heic")[:18], ""},
	} {
		if got := sniff(tc.data); got != tc.want {
			t.Errorf("%s: want %q, got %q", tc.name, tc.want, got)
		}
	}
}

// TestScanNeedsReadableImage checks that a HEIC receipt, which is stored
// but which AI providers cannot read, is refused before the reader is asked.
func TestScanNeedsReadableImage(t *testing.T) {
	ctx := context.Background()
	ada := uuid.New()
	ai := &reader{}
	s := NewService(&shown{}, stored{}, ai, noTx{}, clock.Fixed{At: time.Now()})
	r, err := s.Upload(ctx, ada, ftyp("heic", "mif1", "heic"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Scan(ctx, ada, r.ID, "")
	if e, ok := errs.As(err); !ok || e.Kind != errs.KindValidation || e.Field != "file" || !strings.Contains(e.Message, "JPEG, PNG and WebP") {
		t.Fatalf("want a validation error naming the readable formats, got %v", err)
	}
	if len(ai.calls) != 0 {
		t.Fatalf("a HEIC receipt must not reach the AI reader: %d calls", len(ai.calls))
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

func TestUploadRejectsBadFiles(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"empty", nil},
		{"too large", jpeg(MaxSize + 1)},
		{"gif", []byte("GIF89a\x01\x00\x01\x00\x00\x00\x00")},
		{"svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)},
		{"html", []byte("<!doctype html><html><body>receipt</body></html>")},
		{"pdf", []byte("%PDF-1.7\n%\xe2\xe3\xcf\xd3\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, store := &pending{}, &blobs{}
			s := NewService(repo, store, nil, noTx{}, clock.Fixed{At: time.Now()})
			_, err := s.Upload(context.Background(), uuid.New(), tc.data)
			if e, ok := errs.As(err); !ok || e.Kind != errs.KindValidation || e.Field != "file" {
				t.Fatalf("want a validation error on file, got %v", err)
			}
			if store.puts != 0 || len(repo.rows) != 0 {
				t.Fatalf("a refused upload must store nothing: %d puts, %d rows", store.puts, len(repo.rows))
			}
		})
	}
}

// shown is a repository whose receipts only their uploader sees.
type shown struct{ pending }

func (p *shown) Get(_ context.Context, id uuid.UUID) (*Receipt, error) {
	for _, r := range p.rows {
		if r.ID == id {
			return r, nil
		}
	}
	return nil, errs.NotFound("Receipt")
}

func (p *shown) Visible(_ context.Context, id, userID uuid.UUID) (bool, error) {
	r, err := p.Get(context.Background(), id)
	return err == nil && r.UploadedBy == userID, nil
}

// reader records the receipts it is asked to read.
type reader struct {
	ports.ReceiptReader
	calls []ports.ReceiptInput
}

func (r *reader) ReadReceipt(_ context.Context, in ports.ReceiptInput) (ports.ReceiptExtract, error) {
	r.calls = append(r.calls, in)
	return ports.ReceiptExtract{Total: "12.50"}, nil
}

// stored keeps the bytes of every image put into it.
type stored map[string][]byte

func (b stored) Put(_ context.Context, key string, r io.Reader) error {
	data, err := io.ReadAll(r)
	b[key] = data
	return err
}

func (b stored) Open(_ context.Context, key string) (io.ReadCloser, error) {
	data, ok := b[key]
	if !ok {
		return nil, errs.NotFound("File")
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (b stored) Delete(_ context.Context, key string) error {
	delete(b, key)
	return nil
}

func TestOpenAndScanNeedAccess(t *testing.T) {
	ctx := context.Background()
	ada, mal := uuid.New(), uuid.New()
	repo, ai := &shown{}, &reader{}
	s := NewService(repo, stored{}, ai, noTx{}, clock.Fixed{At: time.Now()})
	r, err := s.Upload(ctx, ada, jpeg(100))
	if err != nil {
		t.Fatal(err)
	}
	if _, body, err := s.Open(ctx, ada, r.ID); err != nil {
		t.Fatalf("the uploader opens the receipt: %v", err)
	} else {
		_ = body.Close()
	}
	for name, call := range map[string]func() error{
		"open":    func() error { _, _, err := s.Open(ctx, mal, r.ID); return err },
		"scan":    func() error { _, err := s.Scan(ctx, mal, r.ID, ""); return err },
		"unknown": func() error { _, _, err := s.Open(ctx, ada, uuid.New()); return err },
	} {
		if err := call(); !errs.Is(err, errs.KindNotFound) {
			t.Errorf("%s: want not found, got %v", name, err)
		}
	}
	if len(ai.calls) != 0 {
		t.Fatalf("nothing may reach the AI reader for someone who cannot see the receipt: %d calls", len(ai.calls))
	}
	scan, err := s.Scan(ctx, ada, r.ID, "")
	if err != nil || scan.Total != 1250 || len(ai.calls) != 1 || len(ai.calls[0].Image) != 100 {
		t.Fatalf("the uploader scans the receipt: %+v, %v, %d calls", scan, err, len(ai.calls))
	}
}

func TestScanWithoutAI(t *testing.T) {
	ctx := context.Background()
	ada := uuid.New()
	s := NewService(&shown{}, stored{}, nil, noTx{}, clock.Fixed{At: time.Now()})
	r, err := s.Upload(ctx, ada, jpeg(100))
	if err != nil {
		t.Fatal(err)
	}
	if s.AIEnabled() {
		t.Fatal("AI is off without a reader")
	}
	if _, err := s.Scan(ctx, ada, r.ID, "EUR"); !errs.Is(err, errs.KindAIDisabled) {
		t.Fatalf("want AI disabled, got %v", err)
	}
}

func TestToScanCurrencyAndEmptyReceipts(t *testing.T) {
	for _, tc := range []struct {
		name, read, fallback, want string
	}{
		{"read from the receipt", "jpy", "EUR", "JPY"},
		{"unknown falls back to the request", "XXX1", "eur", "EUR"},
		{"then to dollars", "", "nope", "USD"},
	} {
		if got := toScan(ports.ReceiptExtract{Currency: tc.read}, tc.fallback).Currency; got != tc.want {
			t.Errorf("%s: want %s, got %s", tc.name, tc.want, got)
		}
	}
	s := toScan(ports.ReceiptExtract{Currency: "JPY", Subtotal: "1200", Tax: "120", Tip: "80"}, "")
	if len(s.Items) != 0 || s.Subtotal != 1200 || s.Total != 1400 {
		t.Fatalf("a receipt without lines totals its parts: %+v", s)
	}
	if s := toScan(ports.ReceiptExtract{Total: "-5", Date: "20.11.2026", Category: expense.PaymentCategory}, "EUR"); s.Total != 0 ||
		s.Date != "" || s.Category != expense.DefaultCategory {
		t.Fatalf("bad total, date and category are dropped: %+v", s)
	}
}

// TestScanChecksCurrency checks that the currency hint is a known code,
// normalised, before anything reaches the AI reader.
func TestScanChecksCurrency(t *testing.T) {
	ctx := context.Background()
	ada := uuid.New()
	ai := &reader{}
	s := NewService(&shown{}, stored{}, ai, noTx{}, clock.Fixed{At: time.Now()})
	r, err := s.Upload(ctx, ada, jpeg(100))
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"not a code", "EURO", strings.Repeat("A", 1<<20)} {
		_, err := s.Scan(ctx, ada, r.ID, bad)
		if e, ok := errs.As(err); !ok || e.Kind != errs.KindValidation || e.Field != "currency" {
			t.Errorf("%.20q: want a validation error on currency, got %v", bad, err)
		}
	}
	if len(ai.calls) != 0 {
		t.Fatalf("an unknown currency must not reach the AI reader: %d calls", len(ai.calls))
	}
	for _, tc := range []struct{ in, want string }{{" eur ", "EUR"}, {"", ""}} {
		if _, err := s.Scan(ctx, ada, r.ID, tc.in); err != nil {
			t.Fatal(err)
		}
		if got := ai.calls[len(ai.calls)-1].Currency; got != tc.want {
			t.Errorf("%q: want %q passed to the reader, got %q", tc.in, tc.want, got)
		}
	}
}
