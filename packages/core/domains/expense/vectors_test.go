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
				Description string
				Notes       string
				Currency    string
				Category    string
				Total       int64
				Date        string
				Method      Method
				Recurrence  Recurrence
				Payers      []struct {
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
			Owed        map[uuid.UUID]int64
			Values      map[uuid.UUID]string
			Description string
			Notes       string
			Method      Method
			Currency    string
			Error       string
			Message     string
		}
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Splits) == 0 {
		t.Fatal("no split vectors")
	}
	for _, c := range doc.Splits {
		in := Input{Description: c.Input.Description, Notes: c.Input.Notes, Currency: c.Input.Currency, Category: c.Input.Category,
			Total: c.Input.Total, Method: c.Input.Method, Recurrence: c.Input.Recurrence, Date: c.Input.Date, Items: c.Input.Items, Tax: c.Input.Tax, Tip: c.Input.Tip}
		if in.Description == "" {
			in.Description = c.Name
		}
		if in.Currency == "" {
			in.Currency = "USD"
		}
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
			if !ok || de.Field != c.Error || de.Message != c.Message {
				t.Errorf("%s: want error on %q saying %q, got %v", c.Name, c.Error, c.Message, err)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", c.Name, err)
			continue
		}
		paid := map[uuid.UUID]int64{}
		for _, p := range in.Payers {
			paid[p.UserID] += p.Amount
		}
		if len(n.Shares) != len(c.Owed) {
			t.Errorf("%s: %d shares, want %d", c.Name, len(n.Shares), len(c.Owed))
		}
		values := map[uuid.UUID]string{}
		var sum int64
		for i, s := range n.Shares {
			if want, ok := c.Owed[s.UserID]; !ok {
				t.Errorf("%s: unexpected share for %s", c.Name, s.UserID)
			} else if s.Owed != want {
				t.Errorf("%s: %s owes %d, want %d", c.Name, s.UserID, s.Owed, want)
			}
			if s.Paid != paid[s.UserID] {
				t.Errorf("%s: %s paid %d, want %d", c.Name, s.UserID, s.Paid, paid[s.UserID])
			}
			if i > 0 && s.UserID.String() <= n.Shares[i-1].UserID.String() {
				t.Errorf("%s: shares not sorted by user id", c.Name)
			}
			values[s.UserID] = s.Value
			sum += s.Owed
		}
		if sum != in.Total {
			t.Errorf("%s: owed adds up to %d, want %d", c.Name, sum, in.Total)
		}
		if c.Description != "" && n.Description != c.Description {
			t.Errorf("%s: description %q, want %q", c.Name, n.Description, c.Description)
		}
		if c.Notes != "" && n.Notes != c.Notes {
			t.Errorf("%s: notes %q, want %q", c.Name, n.Notes, c.Notes)
		}
		if c.Method != "" && n.Method != c.Method {
			t.Errorf("%s: method %q, want %q", c.Name, n.Method, c.Method)
		}
		if c.Currency != "" && n.Currency != c.Currency {
			t.Errorf("%s: currency %q, want %q", c.Name, n.Currency, c.Currency)
		}
		for id, want := range c.Values {
			if values[id] != want {
				t.Errorf("%s: %s value %q, want %q", c.Name, id, values[id], want)
			}
		}
	}
}

func TestRecurrenceVectors(t *testing.T) {
	raw, err := os.ReadFile("../../../../spec/vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Recurrence []struct {
			Name       string
			Recurrence Recurrence
			Start      string
			Dates      []string
		}
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Recurrence) == 0 {
		t.Fatal("no recurrence vectors")
	}
	for _, c := range doc.Recurrence {
		start := date(t, c.Start)
		at := start
		for i, want := range c.Dates {
			at = c.Recurrence.Next(at, start.Day())
			if got := at.Format(time.DateOnly); got != want {
				t.Errorf("%s: occurrence %d is %s, want %s", c.Name, i+1, got, want)
				break
			}
		}
	}
}
