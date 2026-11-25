package money

import (
	"encoding/json"
	"os"
	"slices"
	"testing"
)

// TestCatalogVectors keeps spec/vectors.json in step with the catalog. The
// client bundles the same list for on-device mode and checks it against the
// file, so an exponent changed here alone scales amounts there wrongly.
func TestCatalogVectors(t *testing.T) {
	raw, err := os.ReadFile("../../../../spec/vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct{ Currencies []Currency }
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	got := Currencies()
	if !slices.Equal(got, doc.Currencies) {
		t.Errorf("spec/vectors.json currencies differ from the catalog; update it "+
			"and packages/client/lib/core/config/local_config.dart:\n got %+v\nwant %+v",
			got, doc.Currencies)
	}
}
