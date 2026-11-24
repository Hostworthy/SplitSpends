// Package money holds currency metadata and exact integer allocation helpers.
// All amounts are int64 minor units; floats never touch money.
package money

import (
	"sort"
	"strings"
)

// Currency describes an ISO 4217 currency.
type Currency struct {
	Code     string
	Name     string
	Symbol   string
	Exponent int
}

var currencies = []Currency{
	{"AED", "UAE Dirham", "AED", 2},
	{"ARS", "Argentine Peso", "$", 2},
	{"AUD", "Australian Dollar", "A$", 2},
	{"BDT", "Bangladeshi Taka", "৳", 2},
	{"BGN", "Bulgarian Lev", "BGN", 2},
	{"BHD", "Bahraini Dinar", "BD", 3},
	{"BRL", "Brazilian Real", "R$", 2},
	{"CAD", "Canadian Dollar", "C$", 2},
	{"CHF", "Swiss Franc", "CHF", 2},
	{"CLP", "Chilean Peso", "$", 0},
	{"CNY", "Chinese Yuan", "¥", 2},
	{"COP", "Colombian Peso", "$", 2},
	{"CZK", "Czech Koruna", "CZK", 2},
	{"DKK", "Danish Krone", "kr", 2},
	{"EGP", "Egyptian Pound", "E£", 2},
	{"EUR", "Euro", "€", 2},
	{"GBP", "British Pound", "£", 2},
	{"HKD", "Hong Kong Dollar", "HK$", 2},
	{"HUF", "Hungarian Forint", "Ft", 2},
	{"IDR", "Indonesian Rupiah", "Rp", 2},
	{"ILS", "Israeli New Shekel", "₪", 2},
	{"INR", "Indian Rupee", "₹", 2},
	{"ISK", "Icelandic Krona", "kr", 0},
	{"JOD", "Jordanian Dinar", "JD", 3},
	{"JPY", "Japanese Yen", "¥", 0},
	{"KES", "Kenyan Shilling", "KSh", 2},
	{"KRW", "South Korean Won", "₩", 0},
	{"KWD", "Kuwaiti Dinar", "KD", 3},
	{"LKR", "Sri Lankan Rupee", "Rs", 2},
	{"MAD", "Moroccan Dirham", "MAD", 2},
	{"MXN", "Mexican Peso", "MX$", 2},
	{"MYR", "Malaysian Ringgit", "RM", 2},
	{"NGN", "Nigerian Naira", "₦", 2},
	{"NOK", "Norwegian Krone", "kr", 2},
	{"NPR", "Nepalese Rupee", "Rs", 2},
	{"NZD", "New Zealand Dollar", "NZ$", 2},
	{"OMR", "Omani Rial", "OMR", 3},
	{"PEN", "Peruvian Sol", "S/", 2},
	{"PHP", "Philippine Peso", "₱", 2},
	{"PKR", "Pakistani Rupee", "Rs", 2},
	{"PLN", "Polish Zloty", "PLN", 2},
	{"QAR", "Qatari Riyal", "QR", 2},
	{"RON", "Romanian Leu", "lei", 2},
	{"SAR", "Saudi Riyal", "SR", 2},
	{"SEK", "Swedish Krona", "kr", 2},
	{"SGD", "Singapore Dollar", "S$", 2},
	{"THB", "Thai Baht", "฿", 2},
	{"TRY", "Turkish Lira", "₺", 2},
	{"TWD", "New Taiwan Dollar", "NT$", 2},
	{"UAH", "Ukrainian Hryvnia", "₴", 2},
	{"USD", "US Dollar", "$", 2},
	{"VND", "Vietnamese Dong", "₫", 0},
	{"ZAR", "South African Rand", "R", 2},
}

var byCode = func() map[string]Currency {
	m := make(map[string]Currency, len(currencies))
	for _, c := range currencies {
		m[c.Code] = c
	}
	return m
}()

// Currencies returns all supported currencies sorted by code.
func Currencies() []Currency {
	out := make([]Currency, len(currencies))
	copy(out, currencies)
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out
}

// Lookup normalizes code and returns the currency if supported.
func Lookup(code string) (Currency, bool) {
	c, ok := byCode[strings.ToUpper(strings.TrimSpace(code))]
	return c, ok
}
