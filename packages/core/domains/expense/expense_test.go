package expense

import (
	"testing"
	"time"
)

func date(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestNextMonthEnd(t *testing.T) {
	cases := []struct {
		name       string
		recurrence Recurrence
		start      string
		want       []string
	}{
		{"monthly from the 31st", RecurMonthly, "2027-01-31", []string{"2027-02-28", "2027-03-31", "2027-04-30", "2027-05-31"}},
		{"monthly from the 30th in a leap year", RecurMonthly, "2028-01-30", []string{"2028-02-29", "2028-03-30"}},
		{"monthly across the year end", RecurMonthly, "2027-12-31", []string{"2028-01-31", "2028-02-29"}},
		{"yearly from a leap day", RecurYearly, "2028-02-29", []string{"2029-02-28", "2030-02-28", "2031-02-28", "2032-02-29"}},
		{"weekly ignores the day", RecurWeekly, "2027-01-31", []string{"2027-02-07", "2027-02-14"}},
	}
	for _, c := range cases {
		start := date(t, c.start)
		at := start
		for i, want := range c.want {
			at = c.recurrence.Next(at, start.Day())
			if got := at.Format(time.DateOnly); got != want {
				t.Errorf("%s: occurrence %d is %s, want %s", c.name, i+1, got, want)
				break
			}
		}
	}
}

func TestMissed(t *testing.T) {
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
		{"month ends keep the series day", RecurMonthly, "2026-01-31", "2026-04-30", 4, "2026-01-31", "2026-04-30", "2026-05-31"},
		{"years behind keeps the latest", RecurWeekly, "2024-11-25", "2026-11-25", CatchUp, "2026-09-07", "2026-11-23", "2026-11-30"},
		{"from year one", RecurWeekly, "0001-01-01", "2026-11-25", CatchUp, "", "", ""},
		{"none never repeats", RecurNone, "2026-01-01", "2026-03-15", 0, "", "", "2026-01-01"},
	}
	for _, c := range cases {
		next := date(t, c.next)
		dates, after := c.recurrence.Missed(next, date(t, c.today), next.Day())
		if len(dates) != c.count {
			t.Errorf("%s: %d dates, want %d", c.name, len(dates), c.count)
			continue
		}
		if !after.After(date(t, c.today)) && c.recurrence != RecurNone {
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
