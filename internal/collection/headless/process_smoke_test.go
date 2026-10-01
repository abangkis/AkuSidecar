package headless

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Optional, explicit local smoke: uses an isolated test profile, never the
// installed authenticated profile. It exercises real Node/CDP/Job cleanup.
func TestOwnedChromeProfileReuseSmoke(t *testing.T) {
	runtime := os.Getenv("AKU_HEADLESS_SMOKE_RUNTIME")
	chrome := os.Getenv("AKU_HEADLESS_SMOKE_CHROME")
	bridge := os.Getenv("AKU_HEADLESS_SMOKE_BRIDGE")
	if runtime == "" || chrome == "" || bridge == "" {
		t.Skip("explicit staged runtime, Chrome and Bridge paths required")
	}
	options := Options{Node: filepath.Join(runtime, "node.exe"), Worker: filepath.Join(runtime, "worker.mjs"), Pin: filepath.Join(runtime, "node.pin.json"), Chrome: chrome, BridgePath: bridge, Profile: filepath.Join(t.TempDir(), "capture-profile")}
	ctx, cancel := context.WithTimeout(context.Background(), 55*time.Second)
	defer cancel()
	first, err := Launch(ctx, options)
	if first != nil {
		defer first.Terminate()
	}
	if err != nil {
		t.Fatal(err)
	}
	if first.PID() <= 0 {
		t.Fatal("worker owner missing")
	}
	duplicate, err := Launch(ctx, options)
	if duplicate != nil {
		if cleanupErr := duplicate.CloseForRetry(ctx); cleanupErr != nil {
			t.Fatal(cleanupErr)
		}
	}
	if err == nil {
		t.Fatal("second owner accepted same profile")
	}
	if err := first.CloseForRetry(ctx); err != nil {
		t.Fatal(err)
	}
	reused, err := Launch(ctx, options)
	if reused != nil {
		defer reused.Terminate()
	}
	if err != nil {
		t.Fatalf("verified release could not reuse profile: %v", err)
	}
	if err := reused.CloseForRetry(ctx); err != nil {
		t.Fatal(err)
	}
}
