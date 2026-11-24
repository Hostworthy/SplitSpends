package expense

import (
	"strings"

	"github.com/google/uuid"
)

// Category is a spending category. Icon is a Material Symbols name the
// clients render.
type Category struct {
	ID    string
	Name  string
	Group string
	Icon  string
}

// DefaultCategory is used when none is given.
const DefaultCategory = "general"

// PaymentCategory marks settle-up payments.
const PaymentCategory = "payment"

var categories = []Category{
	{"food.dining", "Dining out", "Food and drink", "restaurant"},
	{"food.groceries", "Groceries", "Food and drink", "local_grocery_store"},
	{"food.drinks", "Drinks and nightlife", "Food and drink", "local_bar"},
	{"food.coffee", "Coffee", "Food and drink", "local_cafe"},
	{"home.rent", "Rent", "Home", "home"},
	{"home.utilities", "Utilities", "Home", "bolt"},
	{"home.internet", "Internet and phone", "Home", "wifi"},
	{"home.supplies", "Household supplies", "Home", "cleaning_services"},
	{"home.furniture", "Furniture", "Home", "chair"},
	{"home.maintenance", "Maintenance", "Home", "build"},
	{"home.pets", "Pets", "Home", "pets"},
	{"transport.fuel", "Fuel", "Transportation", "local_gas_station"},
	{"transport.parking", "Parking", "Transportation", "local_parking"},
	{"transport.taxi", "Taxi and rideshare", "Transportation", "local_taxi"},
	{"transport.transit", "Public transit", "Transportation", "directions_bus"},
	{"transport.car", "Car", "Transportation", "directions_car"},
	{"travel.flights", "Flights", "Travel", "flight"},
	{"travel.lodging", "Lodging", "Travel", "hotel"},
	{"travel.activities", "Activities", "Travel", "attractions"},
	{"entertainment.movies", "Movies", "Entertainment", "movie"},
	{"entertainment.games", "Games", "Entertainment", "sports_esports"},
	{"entertainment.music", "Music", "Entertainment", "music_note"},
	{"entertainment.sports", "Sports", "Entertainment", "sports_soccer"},
	{"entertainment.subscriptions", "Subscriptions", "Entertainment", "subscriptions"},
	{"life.clothing", "Clothing", "Life", "checkroom"},
	{"life.gifts", "Gifts", "Life", "redeem"},
	{"life.medical", "Medical", "Life", "medical_services"},
	{"life.education", "Education", "Life", "school"},
	{"life.childcare", "Childcare", "Life", "child_care"},
	{"life.insurance", "Insurance", "Life", "shield"},
	{"life.taxes", "Taxes", "Life", "account_balance"},
	{"shopping", "Shopping", "Other", "shopping_bag"},
	{DefaultCategory, "General", "Other", "receipt_long"},
	{PaymentCategory, "Payment", "Other", "payments"},
}

var categoryIndex = func() map[string]Category {
	m := make(map[string]Category, len(categories))
	for _, c := range categories {
		m[c.ID] = c
	}
	return m
}()

// Categories returns all categories in display order.
func Categories() []Category {
	out := make([]Category, len(categories))
	copy(out, categories)
	return out
}

// CategoryIDs returns category ids, excluding the payment category.
func CategoryIDs() []string {
	out := make([]string, 0, len(categories))
	for _, c := range categories {
		if c.ID != PaymentCategory {
			out = append(out, c.ID)
		}
	}
	return out
}

// KnownCategory reports whether id is a built-in category.
func KnownCategory(id string) bool {
	_, ok := categoryIndex[id]
	return ok
}

// BuiltIn returns the built-in category with id.
func BuiltIn(id string) (Category, bool) {
	c, ok := categoryIndex[id]
	return c, ok
}

// CustomPrefix starts the id of every user-defined category:
// custom.<uuid>.
const CustomPrefix = "custom."

// CustomCategory reports whether id is shaped like a user-defined category
// id (lowercase canonical UUID after the prefix). Whether the category exists
// and who may use it is checked by the service through Categories.
func CustomCategory(id string) bool {
	rest, ok := strings.CutPrefix(id, CustomPrefix)
	if !ok {
		return false
	}
	u, err := uuid.Parse(rest)
	return err == nil && u.String() == rest
}
