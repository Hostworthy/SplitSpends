package receipt

import (
	"testing"

	"github.com/Hostworthy/SplitSpends/backend/internal/core/ports"
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
