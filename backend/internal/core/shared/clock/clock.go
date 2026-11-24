// Package clock abstracts time so services stay deterministic in tests.
package clock

import "time"

// Clock returns the current time.
type Clock interface {
	Now() time.Time
}

// System is the wall clock in UTC.
type System struct{}

func (System) Now() time.Time { return time.Now().UTC() }

// Fixed always returns the same instant. Useful in tests.
type Fixed struct{ At time.Time }

func (f Fixed) Now() time.Time { return f.At }

// Today returns the calendar date of now in UTC, truncated to midnight.
func Today(c Clock) time.Time {
	n := c.Now()
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
}
