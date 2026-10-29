package config

import "testing"

func TestInstagramCaptureSelectionDefaultsToBrowser(t *testing.T) {
	for _, tc := range []struct {
		mode  string
		valid bool
	}{{"", true}, {"browser", true}, {"concurrent", true}, {"exclusive", false}, {"native", false}} {
		cfg := CaptureConfig{InstagramMode: tc.mode}
		if (cfg.ValidateXMode() == nil) != tc.valid {
			t.Fatalf("Instagram mode %q validation", tc.mode)
		}
		if tc.mode == "" && cfg.InstagramCaptureMode() != "browser" {
			t.Fatal("Instagram default changed")
		}
		if cfg.XCaptureMode() != XCaptureExclusive {
			t.Fatal("Instagram selector changed X default")
		}
	}
}
