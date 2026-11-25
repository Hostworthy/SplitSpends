package money

import "testing"

func TestPlain(t *testing.T) {
	for _, c := range []struct {
		amount int64
		code   string
		want   string
	}{
		{0, "EUR", "0.00"},
		{5, "EUR", "0.05"},
		{-100, "EUR", "-1.00"},
		{123456789, "EUR", "1234567.89"},
		{-1500, "JPY", "-1500"},
		{1234567, "JPY", "1234567"},
		{1, "KWD", "0.001"},
		{-12345, "BHD", "-12.345"},
		{250, "XYZ", "2.50"},
	} {
		if got := Plain(c.amount, c.code); got != c.want {
			t.Errorf("Plain(%d, %q) = %q, want %q", c.amount, c.code, got, c.want)
		}
	}
}

func TestFormat(t *testing.T) {
	for _, c := range []struct {
		amount int64
		code   string
		want   string
	}{
		{1250, "EUR", "€12.50"},
		{-123456789, "EUR", "-€1,234,567.89"},
		{1234567, "JPY", "¥1,234,567"},
		{1, "KWD", "KD0.001"},
		{250, "XYZ", "XYZ 2.50"},
	} {
		if got := Format(c.amount, c.code); got != c.want {
			t.Errorf("Format(%d, %q) = %q, want %q", c.amount, c.code, got, c.want)
		}
	}
}
