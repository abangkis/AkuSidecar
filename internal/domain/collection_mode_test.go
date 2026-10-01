package domain

import (
	"encoding/json"
	"testing"
)

func TestCollectionModeLegacyDefaultAndValidation(t *testing.T) {
	settings := DefaultSettings("standard", "quiet", "guarded_live", true)
	if err := json.Unmarshal([]byte(`{"captureVisibility":"quiet"}`), &settings); err != nil {
		t.Fatal(err)
	}
	settings.CollectionMode = ""
	settings.Normalize()
	if settings.CollectionMode != "browser" {
		t.Fatal("legacy default changed")
	}
	settings.CollectionMode = "headless"
	if err := settings.Validate(); err != nil {
		t.Fatal(err)
	}
	settings.CollectionMode = "invalid"
	if settings.Validate() == nil {
		t.Fatal("invalid mode accepted")
	}
}
