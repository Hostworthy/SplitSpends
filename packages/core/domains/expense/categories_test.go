package expense

import (
	"encoding/json"
	"os"
	"slices"
	"testing"
)

// TestCatalogVectors keeps spec/vectors.json in step with the built-in
// categories, which the client bundles for on-device mode.
func TestCatalogVectors(t *testing.T) {
	raw, err := os.ReadFile("../../../../spec/vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct{ Categories []Category }
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	got := Categories()
	if !slices.Equal(got, doc.Categories) {
		t.Errorf("spec/vectors.json categories differ from the catalog; update it "+
			"and packages/client/lib/core/config/local_config.dart:\n got %+v\nwant %+v",
			got, doc.Categories)
	}
}
