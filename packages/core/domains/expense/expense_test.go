package expense

import (
	"testing"
	"time"
)

func TestMissed(t *testing.T) {
	date := func(s string) time.Time {
		d, err := time.Parse(time.DateOnly, s)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	cases := []struct {
		name        string
		recurrence  Recurrence
		next, today string
		count       int
		first, last string
		after       string
	}{
		{"not due yet", RecurMonthly, "2026-04-01", "2026-03-15", 0, "", "", "2026-04-01"},
		{"due today", RecurMonthly, "2026-03-15", "2026-03-15", 1, "2026-03-15", "2026-03-15", "2026-04-15"},
		{"two months behind", RecurMonthly, "2026-02-01", "2026-03-15", 2, "2026-02-01", "2026-03-01", "2026-04-01"},
		{"months chain like Next", RecurMonthly, "2026-01-31", "2026-04-10", 3, "2026-01-31", "2026-04-03", "2026-05-03"},
		{"years behind keeps the latest", RecurWeekly, "2024-11-25", "2026-11-25", CatchUp, "2026-09-07", "2026-11-23", "2026-11-30"},
		{"from year one", RecurWeekly, "0001-01-01", "2026-11-25", CatchUp, "", "", ""},
		{"none never repeats", RecurNone, "2026-01-01", "2026-03-15", 0, "", "", "2026-01-01"},
	}
	for _, c := range cases {
		dates, after := c.recurrence.Missed(date(c.next), date(c.today))
		if len(dates) != c.count {
			t.Errorf("%s: %d dates, want %d", c.name, len(dates), c.count)
			continue
		}
		if !after.After(date(c.today)) && c.recurrence != RecurNone {
			t.Errorf("%s: continues on %s, not after today", c.name, after.Format(time.DateOnly))
		}
		if c.after != "" && after.Format(time.DateOnly) != c.after {
			t.Errorf("%s: continues on %s, want %s", c.name, after.Format(time.DateOnly), c.after)
		}
		if c.first != "" && (dates[0].Format(time.DateOnly) != c.first || dates[len(dates)-1].Format(time.DateOnly) != c.last) {
			t.Errorf("%s: dates %s to %s, want %s to %s", c.name, dates[0].Format(time.DateOnly),
				dates[len(dates)-1].Format(time.DateOnly), c.first, c.last)
		}
	}
}
