package config

import "testing"

func TestMainProfileSelectionIsExplicitAndUnambiguous(t *testing.T) {
	for _, c := range []CaptureConfig{{}, {XMode: "concurrent", XReaderProfile: "dedicated"}, {XMode: "concurrent", XReaderSource: "dedicated"}} {
		if c.XUsesMainProfile() || c.ValidateXMode() != nil {
			t.Fatalf("dedicated/default changed: %+v", c)
		}
	}
	valid := CaptureConfig{XMode: "concurrent", XReaderSource: "main"}
	if !valid.XUsesMainProfile() || valid.ValidateXMode() != nil {
		t.Fatal("main selection rejected")
	}
	for _, c := range []CaptureConfig{{XReaderSource: "main"}, {XMode: "concurrent", XReaderSource: "main", XReaderProfile: "ambiguous"}, {XMode: "concurrent", XReaderSource: "other"}} {
		if c.ValidateXMode() == nil {
			t.Fatal("ambiguous selection accepted")
		}
	}
}
