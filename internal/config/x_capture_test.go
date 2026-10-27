package config

import (
	"encoding/json"
	"testing"
)

func TestXCaptureMode(t *testing.T) {
	for _, tc := range []struct {
		raw, mode string
		valid     bool
	}{
		{`{}`, XCaptureExclusive, true}, {`{"xMode":"exclusive"}`, XCaptureExclusive, true},
		{`{"xMode":"concurrent"}`, XCaptureConcurrent, true}, {`{"xMode":"Concurrent"}`, "Concurrent", false},
		{`{"xMode":"auto"}`, "auto", false}, {`{"xMode":" concurrent "}`, " concurrent ", false},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			var cfg CaptureConfig
			if err := json.Unmarshal([]byte(tc.raw), &cfg); err != nil {
				t.Fatal(err)
			}
			if cfg.XCaptureMode() != tc.mode || (cfg.ValidateXMode() == nil) != tc.valid {
				t.Fatalf("mode=%q error=%v", cfg.XCaptureMode(), cfg.ValidateXMode())
			}
		})
	}
}
