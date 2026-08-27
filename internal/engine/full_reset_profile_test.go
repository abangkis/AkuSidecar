package engine

import (
	"context"
	"io"
	"log"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/AkuSidecar/internal/store"
)

func TestFullResetDoesNotStageAppProfileWipe(t *testing.T) {
	settings := domain.DefaultSettings("standard", "quiet", "guarded_live", true)
	state, err := store.Open(t.TempDir()+"/sidecar.db", settings)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()

	pending, err := state.PendingAppProfileReset(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if pending {
		t.Fatal("fresh store must not stage a profile reset")
	}

	if _, err := state.FullReset(context.Background(), domain.DefaultSettings("standard", "quiet", "guarded_live", true)); err != nil {
		t.Fatalf("FullReset: %v", err)
	}

	pending, err = state.PendingAppProfileReset(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if pending {
		t.Fatal("FullReset must preserve the browser profile")
	}
}

func TestFullResetKeepsProfileWipeAbsentThroughOnboardingCycle(t *testing.T) {
	settings := domain.DefaultSettings("standard", "quiet", "guarded_live", true)
	state, err := store.Open(t.TempDir()+"/sidecar.db", settings)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	if _, err := state.FullReset(context.Background(), settings); err != nil {
		t.Fatalf("FullReset: %v", err)
	}
	// Onboarding writes must not introduce the retired profile-wipe marker.
	if _, err := state.CompleteOnboarding(context.Background(), settings.ActiveSources); err != nil {
		t.Fatalf("CompleteOnboarding: %v", err)
	}
	pending, err := state.PendingAppProfileReset(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if pending {
		t.Fatal("onboarding lifecycle must not stage a profile wipe")
	}
}

// Guard the unrelated helper surface the engine tests rely on.
func TestFullResetPreservesBridgeIdentityContract(t *testing.T) {
	settings := domain.DefaultSettings("standard", "quiet", "guarded_live", true)
	state, err := store.Open(t.TempDir()+"/sidecar.db", settings)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	cfg := config.Config{Capture: config.CaptureConfig{Profile: "standard", Visibility: "quiet", MaxAcquisitionRounds: 2}, Preference: config.PreferenceConfig{Mode: "guarded_live"}}
	runtime := New(state, reasoning.Deterministic{}, cfg, log.New(io.Discard, "", 0))
	if _, err := runtime.FullReset(context.Background()); err != nil {
		t.Fatalf("engine FullReset: %v", err)
	}
	pending, err := state.PendingAppProfileReset(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if pending {
		t.Fatal("engine FullReset must preserve the browser profile")
	}
}

func TestFullResetRestoresConfiguredDefaultProviderInStoreAndRuntime(t *testing.T) {
	runtime, state := swapTestEngine(t)
	settings, err := state.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	settings.ReasoningProvider = "gemini-flash-lite"
	if _, err := runtime.SaveSettings(context.Background(), settings); err != nil {
		t.Fatalf("switch to Gemini before reset: %v", err)
	}
	if runtime.ProviderName() != "gemini-flash-lite" {
		t.Fatalf("provider before reset=%s", runtime.ProviderName())
	}

	if _, err := runtime.FullReset(context.Background()); err != nil {
		t.Fatalf("FullReset: %v", err)
	}
	if runtime.ProviderName() != "deterministic" {
		t.Fatalf("runtime provider after reset=%s, want configured default deterministic", runtime.ProviderName())
	}
	persisted, err := state.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if persisted.ReasoningProvider != "deterministic" {
		t.Fatalf("persisted provider after reset=%s, want configured default deterministic", persisted.ReasoningProvider)
	}
}
