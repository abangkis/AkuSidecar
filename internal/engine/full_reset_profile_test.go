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

func TestFullResetStagesAppProfileWipe(t *testing.T) {
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
	if !pending {
		t.Fatal("FullReset must stage the isolated browser profile wipe")
	}

	if err := state.ConsumePendingAppProfileReset(context.Background()); err != nil {
		t.Fatalf("consume staged reset: %v", err)
	}
	pending, err = state.PendingAppProfileReset(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if pending {
		t.Fatal("consumed profile reset must stay cleared")
	}
}

func TestFullResetStagedWipeSurvivesOnboardingCycle(t *testing.T) {
	settings := domain.DefaultSettings("standard", "quiet", "guarded_live", true)
	state, err := store.Open(t.TempDir()+"/sidecar.db", settings)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	if _, err := state.FullReset(context.Background(), settings); err != nil {
		t.Fatalf("FullReset: %v", err)
	}
	// Onboarding completes and resets again; the staged wipe marker must not
	// be cleared by unrelated lifecycle writes in between.
	if _, err := state.CompleteOnboarding(context.Background(), settings.ActiveSources); err != nil {
		t.Fatalf("CompleteOnboarding: %v", err)
	}
	pending, err := state.PendingAppProfileReset(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !pending {
		t.Fatal("onboarding lifecycle must not clear the staged profile wipe")
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
	if !pending {
		t.Fatal("engine FullReset must stage the profile wipe")
	}
}
