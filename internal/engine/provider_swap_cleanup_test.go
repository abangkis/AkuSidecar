package engine

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/AkuSidecar/internal/store"
)

type swapCleanupProvider struct {
	reasoning.Deterministic
	name       string
	closeCalls int
	closeErr   error
}

func (p *swapCleanupProvider) Name() string { return p.name }
func (p *swapCleanupProvider) Close() error {
	p.closeCalls++
	return p.closeErr
}
func (p *swapCleanupProvider) ProfileOptions() []reasoning.ProfileOption { return nil }
func (p *swapCleanupProvider) ResolveProfile(string) (config.ModelConfig, bool) {
	return config.ModelConfig{Model: "test-model", Effort: "high"}, true
}
func (p *swapCleanupProvider) InvokeStructured(context.Context, string, string, any, config.ModelConfig) (string, domain.ModelUsage, time.Duration, error) {
	panic("provider swap planning must not invoke inference")
}

func swapCleanupEngine(t *testing.T) (*Engine, *store.Store, *swapCleanupProvider) {
	t.Helper()
	root := t.TempDir()
	copySwapSchemas(t, root)
	settings := domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", true)
	settings.ReasoningProvider = "deterministic"
	state, err := store.Open(filepath.Join(root, "sidecar.db"), settings)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { state.Close() })
	active := &swapCleanupProvider{name: "deterministic"}
	cfg := config.Config{Root: root, Reasoning: config.ReasoningConfig{
		ActiveProvider: "deterministic", Provider: "deterministic",
		Providers: map[string]config.ProviderConfig{"deterministic": {}, "gemini-flash-lite": {}},
	}}
	return New(state, active, cfg, log.New(io.Discard, "", 0)), state, active
}

func assertSwapPlanningUnchanged(t *testing.T, runtime *Engine, state *store.Store, active *swapCleanupProvider, before domain.Settings, configBefore []byte) {
	t.Helper()
	if runtime.provider != active || active.closeCalls != 0 {
		t.Fatalf("planning replaced or closed active provider: provider=%T closes=%d", runtime.provider, active.closeCalls)
	}
	if runtime.aiDeep != nil || runtime.topics != nil {
		t.Fatal("planning applied replacement resolvers")
	}
	after, err := state.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("planning changed durable settings: before=%+v after=%+v", before, after)
	}
	configAfter, err := json.Marshal(runtime.config)
	if err != nil {
		t.Fatal(err)
	}
	if string(configBefore) != string(configAfter) {
		t.Fatal("planning changed active runtime config")
	}
}

func TestPlanProviderSwapCleansCandidateOnPreparationFailure(t *testing.T) {
	for _, tc := range []struct {
		name, schema, prefix   string
		malformed, nilSettings bool
	}{
		{name: "profile_activation", nilSettings: true, prefix: "settings are nil"},
		{name: "semantic_read", schema: "semantic-event-resolution.schema.json", prefix: `construct semantic resolver for "gemini-flash-lite": read semantic event schema: `},
		{name: "semantic_decode", schema: "semantic-event-resolution.schema.json", malformed: true, prefix: `construct semantic resolver for "gemini-flash-lite": decode semantic event schema: `},
		{name: "ai_read", schema: "ai-deep-detection.schema.json", prefix: `construct AI Deep resolver for "gemini-flash-lite": read AI deep-detection schema: `},
		{name: "topics_read", schema: "living-topic-snapshot.schema.json", prefix: `construct Living Topics resolver for "gemini-flash-lite": read Living Topics snapshot schema: `},
		{name: "topics_routing_decode", schema: "living-topic-routing.schema.json", malformed: true, prefix: `construct Living Topics resolver for "gemini-flash-lite": decode Living Topics routing schema: `},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runtime, state, active := swapCleanupEngine(t)
			before, err := state.GetSettings(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			configBefore, err := json.Marshal(runtime.config)
			if err != nil {
				t.Fatal(err)
			}
			if tc.schema != "" {
				file := filepath.Join(runtime.config.Root, "schemas", tc.schema)
				if tc.malformed {
					err = os.WriteFile(file, []byte("{"), 0o600)
				} else {
					err = os.Remove(file)
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			closeErr := errors.New("candidate close failure must not replace planning error")
			candidate := &swapCleanupProvider{name: "gemini-flash-lite", closeErr: closeErr}
			factoryCalls := 0
			factory := func(cfg config.Config) (reasoning.Provider, error) {
				factoryCalls++
				if cfg.Reasoning.Provider != candidate.name || cfg.Root != runtime.config.Root {
					t.Fatalf("factory received incorrect target config: %+v", cfg.Reasoning)
				}
				return candidate, nil
			}
			settings := before
			settingsArg := &settings
			if tc.nilSettings {
				settingsArg = nil
			}
			plan, err := runtime.planProviderSwapWithFactory(context.Background(), candidate.name, settingsArg, factory)
			if plan != nil || err == nil || !strings.HasPrefix(err.Error(), tc.prefix) {
				t.Fatalf("planning error contract changed: plan=%+v err=%v", plan, err)
			}
			if tc.nilSettings && err.Error() != tc.prefix {
				t.Fatalf("profile activation error changed: %v", err)
			}
			if tc.schema != "" && !tc.malformed && !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("missing schema cause was not preserved: %v", err)
			}
			if tc.malformed {
				var syntaxErr *json.SyntaxError
				if !errors.As(err, &syntaxErr) {
					t.Fatalf("schema decode cause was not preserved: %v", err)
				}
			}
			if errors.Is(err, closeErr) {
				t.Fatal("cleanup error replaced the primary planning error")
			}
			if factoryCalls != 1 || candidate.closeCalls != 1 {
				t.Errorf("factory calls=%d candidate Close calls=%d; want 1 each", factoryCalls, candidate.closeCalls)
			}
			assertSwapPlanningUnchanged(t, runtime, state, active, before, configBefore)
		})
	}
}

func TestPlanProviderSwapTransfersCandidateOwnership(t *testing.T) {
	runtime, state, active := swapCleanupEngine(t)
	before, err := state.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	configBefore, err := json.Marshal(runtime.config)
	if err != nil {
		t.Fatal(err)
	}
	candidate := &swapCleanupProvider{name: "gemini-flash-lite"}
	settings := before
	plan, err := runtime.planProviderSwapWithFactory(context.Background(), candidate.name, &settings, func(config.Config) (reasoning.Provider, error) { return candidate, nil })
	if err != nil || plan == nil {
		t.Fatalf("planning failed: plan=%+v err=%v", plan, err)
	}
	if plan.candidate != candidate || candidate.closeCalls != 0 || plan.eventResolver == nil || plan.aiResolver == nil || plan.topicsResolver == nil {
		t.Fatalf("successful plan did not retain a ready open candidate: plan=%+v closes=%d", plan, candidate.closeCalls)
	}
	assertSwapPlanningUnchanged(t, runtime, state, active, before, configBefore)
	plan.closeCandidate()
	if candidate.closeCalls != 1 {
		t.Fatalf("caller could not dispose the returned candidate: closes=%d", candidate.closeCalls)
	}
}

func TestPlanProviderSwapConstructionFailurePreservesError(t *testing.T) {
	runtime, state, active := swapCleanupEngine(t)
	before, err := state.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	configBefore, err := json.Marshal(runtime.config)
	if err != nil {
		t.Fatal(err)
	}
	constructionErr := errors.New("provider construction failed")
	settings := before
	plan, err := runtime.planProviderSwapWithFactory(context.Background(), "gemini-flash-lite", &settings, func(config.Config) (reasoning.Provider, error) { return nil, constructionErr })
	if plan != nil || !errors.Is(err, constructionErr) || err.Error() != `construct reasoning provider "gemini-flash-lite": provider construction failed` {
		t.Fatalf("construction error contract changed: plan=%+v err=%v", plan, err)
	}
	assertSwapPlanningUnchanged(t, runtime, state, active, before, configBefore)
}
