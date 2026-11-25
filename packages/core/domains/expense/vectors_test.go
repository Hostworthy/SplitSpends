package expense

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/Hostworthy/SplitSpends/packages/core/shared/errs"
	"github.com/google/uuid"
)

// TestSharedVectors runs spec/vectors.json, which the Dart app also runs, so
// local-only and server modes split money identically.
func TestSharedVectors(t *testing.T) {
	raw, err := os.ReadFile("../../../../spec/vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Splits []struct {
			Name  string
			Today string
			Input struct {
				Total  int64
				Date   string
				Method Method
				Payers []struct {
					UserID uuid.UUID
					Amount int64
				}
				Splits []struct {
					UserID uuid.UUID
					Value  string
				}
				Items []Item
				Tax   int64
				Tip   int64
			}
			Owed   map[uuid.UUID]int64
			Values map[uuid.UUID]string
			Error  string
		}
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	for _, c := range doc.Splits {
		in := Input{Description: c.Name, Currency: "USD", Total: c.Input.Total, Method: c.Input.Method,
			Date: c.Input.Date, Items: c.Input.Items, Tax: c.Input.Tax, Tip: c.Input.Tip}
		for _, p := range c.Input.Payers {
			in.Payers = append(in.Payers, Payer{UserID: p.UserID, Amount: p.Amount})
		}
		for _, s := range c.Input.Splits {
			in.Splits = append(in.Splits, SplitInput{UserID: s.UserID, Value: s.Value})
		}
		today := time.Now()
		if c.Today != "" {
			if today, err = time.Parse(time.DateOnly, c.Today); err != nil {
				t.Fatal(err)
			}
		}
		n, err := Normalize(in, today)
		if c.Error != "" {
			de, ok := errs.As(err)
			if !ok || de.Field != c.Error {
				t.Errorf("%s: want error on %q, got %v", c.Name, c.Error, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", c.Name, err)
			continue
		}
		got := map[uuid.UUID]int64{}
		values := map[uuid.UUID]string{}
		for _, s := range n.Shares {
			got[s.UserID] = s.Owed
			values[s.UserID] = s.Value
		}
		for id, want := range c.Owed {
			if got[id] != want {
				t.Errorf("%s: %s owes %d, want %d", c.Name, id, got[id], want)
			}
		}
		for id, want := range c.Values {
			if values[id] != want {
				t.Errorf("%s: %s value %q, want %q", c.Name, id, values[id], want)
			}
		}
	}
}
