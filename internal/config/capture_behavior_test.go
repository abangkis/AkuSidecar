package config

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCaptureBehaviorAbsentRemainsDefaultOff(t *testing.T) {
	var capture CaptureConfig
	if err := json.Unmarshal([]byte(`{"profile":"standard","maxAcquisitionRounds":2}`), &capture); err != nil {
		t.Fatal(err)
	}
	policy := capture.Behavior.Effective()
	if policy.Enabled || policy.MinCaptureIntervalMS != 5000 || policy.AdditionalJitterMS != 0 || policy.ScrollFraction != 0.75 || policy.ScrollSettleMS != 900 {
		t.Fatalf("unexpected default %+v", policy)
	}
	if err := policy.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestCaptureBehaviorTypedStartupLoad(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "config", "sidecar.json")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	data := `{"version":1,"server":{"host":"127.0.0.1","port":11122},"database":{"path":"runtime/synthetic.db"},
"reasoning":{"provider":"deterministic","timeoutMs":5000,"planning":{"model":"planner","effort":"high"},"evaluation":{"model":"evaluator","effort":"high"},"semanticEvent":{"model":"event","effort":"high"},"aiDetection":{"model":"detector","effort":"high"}},
"capture":{"profile":"standard","maxAcquisitionRounds":1,"behavior":{"enabled":true,"minCaptureIntervalMs":8000,"additionalJitterMs":3000,"scrollFraction":0.5,"scrollSettleMs":1400}},
"bridge":{"trustedExtensionOrigins":[]},"preference":{"mode":"guarded_live"}}`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	options := Options{ConfigPath: path, BridgeExtensionOrigin: "chrome-extension://abcdefghijklmnopabcdefghijklmnop/"}
	cfg, err := Load(options)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Capture.Behavior.Enabled || cfg.Capture.Behavior.ScrollSettleMS != 1400 {
		t.Fatal("startup ignored tuning")
	}
	for _, invalid := range []string{
		strings.Replace(data, `"minCaptureIntervalMs":8000`, `"minCaptureIntervalMs":-1`, 1),
		strings.Replace(data, `"minCaptureIntervalMs":8000`, `"minimumCaptureIntervalMs":8000`, 1),
		strings.Replace(data, `"scrollSettleMs":1400`, `"scrollSettleMs":2001`, 1),
	} {
		if err := os.WriteFile(path, []byte(invalid), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(options); err == nil {
			t.Fatal("malformed startup tuning accepted")
		}
	}
}

func TestCaptureBehaviorTuningRoundTripAndBounds(t *testing.T) {
	input := `{"behavior":{"enabled":true,"minCaptureIntervalMs":8000,"additionalJitterMs":3000,"scrollFraction":0.5,"scrollSettleMs":1400,"temporaryBackoffBaseMs":20000,"temporaryBackoffMaxMs":180000}}`
	var capture CaptureConfig
	if err := json.Unmarshal([]byte(input), &capture); err != nil {
		t.Fatal(err)
	}
	if err := capture.Behavior.Validate(); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(capture)
	if err != nil {
		t.Fatal(err)
	}
	var persisted CaptureConfig
	if err := json.Unmarshal(encoded, &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted.Behavior != capture.Behavior {
		t.Fatal("tuning did not survive serialization")
	}
	for _, change := range []func(*CaptureBehaviorConfig){
		func(b *CaptureBehaviorConfig) { b.MinCaptureIntervalMS = -1 },
		func(b *CaptureBehaviorConfig) { b.MinCaptureIntervalMS = 999 },
		func(b *CaptureBehaviorConfig) { b.MinCaptureIntervalMS = 900001 },
		func(b *CaptureBehaviorConfig) { b.AdditionalJitterMS = -1 },
		func(b *CaptureBehaviorConfig) { b.AdditionalJitterMS = 60001 },
		func(b *CaptureBehaviorConfig) { b.ScrollFraction = 0.24 },
		func(b *CaptureBehaviorConfig) { b.ScrollFraction = 0.49 },
		func(b *CaptureBehaviorConfig) { b.ScrollFraction = math.NaN() },
		func(b *CaptureBehaviorConfig) { b.ScrollFraction = math.Inf(1) },
		func(b *CaptureBehaviorConfig) { b.ScrollSettleMS = 99 },
		func(b *CaptureBehaviorConfig) { b.ScrollSettleMS = 5001 },
		func(b *CaptureBehaviorConfig) { b.ScrollSettleMS = 2001 },
		func(b *CaptureBehaviorConfig) { b.TemporaryBackoffBaseMS = 999 },
		func(b *CaptureBehaviorConfig) { b.TemporaryBackoffMaxMS = 1000 },
		func(b *CaptureBehaviorConfig) { b.TemporaryBackoffMaxMS = 900001 },
	} {
		invalid := capture.Behavior
		change(&invalid)
		if err := invalid.Validate(); err == nil {
			t.Fatalf("invalid policy accepted: %+v", invalid)
		}
	}
}
