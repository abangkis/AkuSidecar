package engine

import (
	"context"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/AkuSidecar/internal/store"
)

func geminiTestProvider(name string) config.ProviderConfig {
	return config.ProviderConfig{
		Endpoint:      "https://generativelanguage.googleapis.com/v1",
		CredentialRef: "gemini.primary",
		TimeoutMS:     30000,
		Planning:      config.ModelConfig{ModelID: "gemini-3.5-flash-lite", MinReasoningTier: "high", MaxOutputTokens: 512},
		Evaluation:    config.ModelConfig{ModelID: "gemini-3.5-flash-lite", MinReasoningTier: "high", MaxOutputTokens: 8192, Assurance: "provider_strict"},
		SemanticEvent: config.ModelConfig{ModelID: "gemini-3.5-flash-lite", MinReasoningTier: "high", MaxOutputTokens: 8192, Assurance: "provider_strict"},
		AIDetection:   config.ModelConfig{ModelID: "gemini-3.5-flash-lite", MinReasoningTier: "high", MaxOutputTokens: 4096, Assurance: "provider_strict"},
	}
}

var (
	swapSchemaNames = []string{"acquisition-plan.schema.json", "reasoning-result.schema.json", "semantic-event-resolution.schema.json", "ai-deep-detection.schema.json"}
)

// copySwapSchemas stages the tracked workload schemas beneath root so eager
// provider construction and resolver building can read them like startup does.
func copySwapSchemas(t *testing.T, root string) {
	t.Helper()
	schemaDir := filepath.Join(root, "schemas")
	if err := os.MkdirAll(schemaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range swapSchemaNames {
		source, err := os.ReadFile(filepath.Join("..", "..", "schemas", name))
		if err != nil {
			t.Fatalf("read tracked schema %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(schemaDir, name), source, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func swapTestEngine(t *testing.T) (*Engine, *store.Store) {
	t.Helper()
	root := t.TempDir()
	credentialDir := filepath.Join(root, "runtime", "config")
	if err := os.MkdirAll(credentialDir, 0o755); err != nil {
		t.Fatal(err)
	}
	credentialBody := `{"schemaVersion":1,"credentialStore":{"type":"inline","values":{"gemini.primary":"test-key"}}}`
	if err := os.WriteFile(filepath.Join(credentialDir, "credentials.local.json"), []byte(credentialBody), 0o600); err != nil {
		t.Fatal(err)
	}
	copySwapSchemas(t, root)
	settings := domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", true)
	settings.ReasoningProvider = "deterministic"
	state, err := store.Open(filepath.Join(root, "sidecar.db"), settings)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { state.Close() })
	cfg := config.Config{
		Root:     root,
		Dev:      true,
		Server:   config.ServerConfig{Host: "127.0.0.1", Port: 11122},
		Database: config.DatabaseConfig{Path: filepath.Join(root, "sidecar.db")},
		Bridge:   config.BridgeConfig{TrustedExtensionOrigins: []string{"chrome-extension://mfeebfabkhmoaepbcdbbeefpobkedfmp"}},
		Capture: config.CaptureConfig{
			Profile:              "expanded",
			Visibility:           "quiet",
			OpenMissingSource:    true,
			MaxAcquisitionRounds: 2,
		},
		Preference: config.PreferenceConfig{Mode: "promote_unused_budget"},
		Reasoning:  config.ReasoningConfig{ActiveProvider: "deterministic"},
	}
	cfg.Reasoning.Providers = map[string]config.ProviderConfig{
		"deterministic":     {},
		"gemini-flash-lite": geminiTestProvider("gemini-flash-lite"),
	}
	runtime := New(state, reasoning.Deterministic{}, cfg, log.New(io.Discard, "", 0))
	runtime.RecordHeartbeat(ExpectedHeartbeat())
	return runtime, state
}

func TestSaveSettingsSwapsProviderWhenIdle(t *testing.T) {
	runtime, state := swapTestEngine(t)
	if runtime.ProviderName() != "deterministic" {
		t.Fatalf("startup provider=%s", runtime.ProviderName())
	}
	current, err := state.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	target := current
	target.ReasoningProvider = "gemini-flash-lite"
	saved, err := runtime.SaveSettings(context.Background(), target)
	if err != nil {
		t.Fatalf("SaveSettings swap: %v", err)
	}
	if saved.ReasoningProvider != "gemini-flash-lite" {
		t.Fatalf("persisted provider=%s", saved.ReasoningProvider)
	}
	if runtime.ProviderName() != "gemini-flash-lite" {
		t.Fatalf("active provider after swap=%s", runtime.ProviderName())
	}
	persisted, err := state.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if persisted.ReasoningProvider != "gemini-flash-lite" {
		t.Fatalf("store provider=%s", persisted.ReasoningProvider)
	}
	if _, ok := persisted.ReasoningProfilesByProvider["gemini-flash-lite"]; !ok {
		t.Fatalf("swapped provider did not remember a profile set: %+v", persisted.ReasoningProfilesByProvider)
	}
}

func TestSaveSettingsRejectsSwapWhileBusy(t *testing.T) {
	runtime, state := swapTestEngine(t)
	runtime.mu.Lock()
	runtime.active["busy-run"] = func() {}
	runtime.mu.Unlock()
	t.Cleanup(func() {
		runtime.mu.Lock()
		delete(runtime.active, "busy-run")
		runtime.mu.Unlock()
	})
	current, err := state.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	target := current
	target.ReasoningProvider = "gemini-flash-lite"
	if _, err := runtime.SaveSettings(context.Background(), target); err == nil {
		t.Fatal("expected busy swap rejection")
	} else if !strings.Contains(err.Error(), "finish active reasoning work") {
		t.Fatalf("unexpected error: %v", err)
	}
	if runtime.ProviderName() != "deterministic" {
		t.Fatalf("busy swap changed provider to %s", runtime.ProviderName())
	}
	persisted, err := state.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if persisted.ReasoningProvider != "deterministic" {
		t.Fatalf("busy swap persisted provider=%s", persisted.ReasoningProvider)
	}
}

func TestPlanProviderSwapMigratesUnknownProfiles(t *testing.T) {
	runtime, state := swapTestEngine(t)
	current, err := state.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	current.ReasoningProfilesByProvider = map[string]domain.ReasoningProfileSet{
		"gemini-flash-lite": {Acquisition: "nonexistent-profile", Evaluation: "also-missing", Semantic: "gone", AIDeep: "vanished"},
	}
	plan, err := runtime.planProviderSwap(context.Background(), "gemini-flash-lite", &current)
	if err != nil {
		t.Fatalf("planProviderSwap: %v", err)
	}
	if plan == nil {
		t.Fatal("expected a swap plan")
	}
	defer plan.closeCandidate()
	for name, profileID := range map[string]string{
		"acquisition": current.ReasoningAcquisitionProfile,
		"evaluation":  current.ReasoningEvaluationProfile,
		"semantic":    current.ReasoningSemanticProfile,
		"aiDeep":      current.ReasoningAIDeepProfile,
	} {
		catalog := plan.candidate.(reasoning.ProfileProvider)
		if _, found := catalog.ResolveProfile(profileID); !found {
			t.Fatalf("%s profile %q was not migrated to a resolvable ID", name, profileID)
		}
	}
}

func TestSaveSettingsRevertsProviderOnConstructionFailure(t *testing.T) {
	runtime, state := swapTestEngine(t)
	delete(runtime.config.Reasoning.Providers, "gemini-flash-lite")
	current, err := state.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	target := current
	target.ReasoningProvider = "gemini-flash-lite"
	if _, err := runtime.SaveSettings(context.Background(), target); err == nil {
		t.Fatal("expected undeclared provider rejection")
	}
	if runtime.ProviderName() != "deterministic" {
		t.Fatalf("failed swap changed provider to %s", runtime.ProviderName())
	}
}
