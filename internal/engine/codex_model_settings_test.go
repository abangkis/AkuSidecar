package engine

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/AkuSidecar/internal/store"
)

func codexModelSettingsFixture(t *testing.T) (*Engine, *store.Store) {
	t.Helper()
	settings := domain.DefaultSettings("standard", "quiet", "guarded_live", true)
	settings.ReasoningProvider = "codex-app-server"
	state, err := store.Open(filepath.Join(t.TempDir(), "settings.db"), settings)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = state.Close() })
	// No transport is constructed or invoked by settings catalog/save tests.
	provider := &reasoning.CodexAppServer{}
	return New(state, provider, config.Config{}, log.New(io.Discard, "", 0)), state
}

func TestCodexModelSettingsProjectionAndSave(t *testing.T) {
	engine, state := codexModelSettingsFixture(t)
	settings, err := state.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	provider := &reasoning.CodexAppServer{}
	for _, option := range provider.ProfileOptions() {
		settings.ReasoningAcquisitionProfile = option.ID
		settings.ReasoningEvaluationProfile = option.ID
		settings.ReasoningSemanticProfile = option.ID
		settings.ReasoningAIDeepProfile = option.ID
		saved, err := engine.SaveSettings(context.Background(), settings)
		if err != nil {
			t.Fatalf("save %s: %v", option.ID, err)
		}
		settings, err = state.GetSettings(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(saved, settings) {
			t.Fatal("settings did not round-trip")
		}
		for _, process := range engine.ReasoningProcesses(settings) {
			model, found := provider.ResolveProfile(option.ID)
			if !found || process.ProfileID != option.ID || process.Model != model.StableModelID() || process.Effort != option.Effort || len(process.Options) != 15 {
				t.Fatalf("projection=%+v option=%+v", process, option)
			}
		}
	}
	before := settings
	for _, rejected := range []string{"luna_high", "gpt56_sol_high", "gpt6_sol_high", "astra_high", "gpt61_sol_ultra"} {
		draft := before
		draft.ReasoningAcquisitionProfile = rejected
		if _, err := engine.SaveSettings(context.Background(), draft); err == nil {
			t.Fatalf("unsupported profile saved: %s", rejected)
		}
		after, err := state.GetSettings(context.Background())
		if err != nil || !reflect.DeepEqual(before, after) {
			t.Fatalf("rejection changed persisted state: %v", err)
		}
	}
}

func TestCodexModelSettingsBrowserFixture(t *testing.T) {
	output := os.Getenv("AKU_CODEX_MODEL_UI_FIXTURE")
	if output == "" {
		t.Skip("explicit isolated browser fixture export only")
	}
	engine, state := codexModelSettingsFixture(t)
	settings, err := state.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	settings.ReasoningAcquisitionProfile = "gpt61_sol_high"
	settings.ReasoningEvaluationProfile = "gpt61_terra_high"
	settings.ReasoningSemanticProfile = "gpt6_luna_xhigh"
	settings.ReasoningAIDeepProfile = "gpt61_sol_medium"
	fixture := map[string]any{"settings": settings, "processes": engine.ReasoningProcesses(settings)}
	raw, err := json.MarshalIndent(fixture, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(output, raw, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestCodexModelProviderSwitchMigratesPersistedLegacySelections(t *testing.T) {
	for _, tc := range []struct{ old, canonical string }{
		{"luna_high", "gpt6_luna_high"}, {"luna_xhigh", "gpt6_luna_xhigh"}, {"luna_max", "gpt6_luna_max"},
		{"terra_high", "gpt61_terra_high"}, {"terra_xhigh", "gpt61_terra_xhigh"}, {"sol_medium", "gpt61_sol_medium"},
	} {
		t.Run(tc.old, func(t *testing.T) {
			engine, state := codexModelSettingsFixture(t)
			engine.provider = reasoning.Deterministic{}
			engine.config.Root = t.TempDir()
			copySwapSchemas(t, engine.config.Root)
			engine.config.Reasoning.Providers = map[string]config.ProviderConfig{"codex-app-server": readinessProvider("", "codex-gpt-6-luna")}
			settings, _ := state.GetSettings(context.Background())
			settings.ReasoningProvider = "codex-app-server"
			settings.ReasoningProfilesByProvider = map[string]domain.ReasoningProfileSet{
				"codex-app-server": {Acquisition: tc.old, Evaluation: tc.old, Semantic: tc.old, AIDeep: tc.old},
			}
			plan, err := engine.planProviderSwapWithFactory(context.Background(), "codex-app-server", &settings, func(config.Config) (reasoning.Provider, error) { return &reasoning.CodexAppServer{}, nil })
			if err != nil || plan == nil {
				t.Fatalf("plan=%v err=%v", plan, err)
			}
			defer plan.closeCandidate()
			want := domain.ReasoningProfileSet{Acquisition: tc.canonical, Evaluation: tc.canonical, Semantic: tc.canonical, AIDeep: tc.canonical}
			if settings.ActiveReasoningProfileSet() != want || settings.ReasoningProfilesByProvider["codex-app-server"] != want {
				t.Fatalf("migration=%+v", settings)
			}
			if err := state.SaveSettings(context.Background(), settings); err != nil {
				t.Fatal(err)
			}
			persisted, err := state.GetSettings(context.Background())
			if err != nil || persisted.ActiveReasoningProfileSet() != want {
				t.Fatalf("persisted migration=%+v err=%v", persisted, err)
			}
		})
	}
}
