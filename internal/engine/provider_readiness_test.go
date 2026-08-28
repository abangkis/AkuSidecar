package engine

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/AkuSidecar/internal/store"
)

type readinessTestProvider struct {
	reasoning.Deterministic
	err   error
	calls int
}

func (p *readinessTestProvider) Name() string { return "codex-app-server" }
func (p *readinessTestProvider) Preflight(context.Context) error {
	p.calls++
	return p.err
}

func readinessModel(id string) config.ModelConfig {
	return config.ModelConfig{ModelID: id, MinReasoningTier: "high"}
}

func readinessProvider(endpoint, model string) config.ProviderConfig {
	return config.ProviderConfig{
		Endpoint: endpoint, TimeoutMS: 5000,
		Planning: readinessModel(model), Evaluation: readinessModel(model),
		SemanticEvent: readinessModel(model), AIDetection: readinessModel(model),
	}
}

func readinessEngine(t *testing.T, providers map[string]config.ProviderConfig, active string) (*Engine, *store.Store) {
	t.Helper()
	settings := domain.DefaultSettings("standard", "quiet", "guarded_live", true)
	settings.ReasoningProvider = active
	state, err := store.Open(filepath.Join(t.TempDir(), "sidecar.db"), settings)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { state.Close() })
	cfg := config.Config{Root: t.TempDir(), Reasoning: config.ReasoningConfig{ActiveProvider: active, Providers: providers}}
	return New(state, reasoning.Deterministic{}, cfg, log.New(io.Discard, "", 0)), state
}

func TestOllamaReadinessChecksEndpointOnceAndMatchesProviderModels(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/api/tags" {
			t.Errorf("path=%s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"models":[{"name":"nemotron-3.5-lightning:latest"},{"model":"qwen3.8:27b"}]}`)
	}))
	defer server.Close()
	runtime, _ := readinessEngine(t, map[string]config.ProviderConfig{
		"ollama-nemotron": readinessProvider(server.URL, "nemotron-3.5-lightning"),
		"ollama-qwen":     readinessProvider(server.URL, "qwen3.8-27b"),
	}, "ollama-nemotron")

	providers, err := runtime.ReasoningProviderReadiness(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 1 {
		t.Fatalf("Ollama tags requests=%d want=1", requests.Load())
	}
	for _, provider := range providers {
		if !provider.AvailabilityRequired || !provider.AvailabilityChecked || !provider.Available || provider.AvailabilityStatus != "model_ready" {
			t.Fatalf("provider readiness=%+v", provider)
		}
	}
}

func TestOllamaReadinessDistinguishesMissingModelFromEndpointFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"models":[{"name":"nemotron-3.5-lightning:latest"}]}`)
	}))
	runtime, _ := readinessEngine(t, map[string]config.ProviderConfig{
		"ollama-qwen": readinessProvider(server.URL, "qwen3.8-27b"),
	}, "ollama-qwen")
	providers, err := runtime.ReasoningProviderReadiness(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(providers) != 1 || providers[0].Available || providers[0].AvailabilityStatus != "model_missing" || !strings.Contains(providers[0].AvailabilityMessage, "qwen3.8:27b") {
		t.Fatalf("missing model readiness=%+v", providers)
	}

	server.Close()
	providers, err = runtime.ReasoningProviderReadiness(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if providers[0].Available || providers[0].AvailabilityStatus != "endpoint_unreachable" {
		t.Fatalf("unreachable readiness=%+v", providers[0])
	}
}

func TestCodexReadinessRejectsMissingExplicitRuntime(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing-codex.exe")
	provider := readinessProvider("", "codex-luna")
	provider.Executable = missing
	runtime, _ := readinessEngine(t, map[string]config.ProviderConfig{"codex-app-server": provider}, "codex-app-server")
	providers, err := runtime.ReasoningProviderReadiness(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(providers) != 1 || providers[0].Available || !providers[0].AvailabilityChecked || providers[0].AvailabilityStatus != "runtime_missing" {
		t.Fatalf("Codex readiness=%+v", providers)
	}
}

func TestCodexReadinessRequiresSuccessfulNonGenerativePreflight(t *testing.T) {
	settings := domain.DefaultSettings("standard", "quiet", "guarded_live", true)
	settings.ReasoningProvider = "codex-app-server"
	state, err := store.Open(filepath.Join(t.TempDir(), "sidecar.db"), settings)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	providerConfig := readinessProvider("", "codex-luna")
	provider := &readinessTestProvider{}
	runtime := New(state, provider, config.Config{Reasoning: config.ReasoningConfig{
		ActiveProvider: "codex-app-server",
		Providers:      map[string]config.ProviderConfig{"codex-app-server": providerConfig},
	}}, log.New(io.Discard, "", 0))

	providers, err := runtime.ReasoningProviderReadiness(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(providers) != 1 || !providers[0].Available || providers[0].AvailabilityStatus != "session_ready" || provider.calls != 1 {
		t.Fatalf("successful preflight providers=%+v calls=%d", providers, provider.calls)
	}

	provider.err = errors.New("authentication required")
	providers, err = runtime.ReasoningProviderReadiness(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if providers[0].Available || providers[0].AvailabilityStatus != "session_unavailable" || provider.calls != 2 {
		t.Fatalf("failed preflight providers=%+v calls=%d", providers, provider.calls)
	}
}

func TestProviderSwitchAndFirstOnboardingRejectUnavailableLocalRuntime(t *testing.T) {
	provider := readinessProvider("http://127.0.0.1:1", "nemotron-3.5-lightning")
	runtime, state := readinessEngine(t, map[string]config.ProviderConfig{
		"deterministic":   {},
		"ollama-nemotron": provider,
	}, "deterministic")
	settings, err := state.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	target := settings
	target.ReasoningProvider = "ollama-nemotron"
	if _, err := runtime.SaveSettings(context.Background(), target); err == nil || !strings.Contains(err.Error(), "not available") {
		t.Fatalf("provider switch error=%v", err)
	}
	persisted, _ := state.GetSettings(context.Background())
	if persisted.ReasoningProvider != "deterministic" {
		t.Fatalf("failed readiness persisted provider=%s", persisted.ReasoningProvider)
	}

	settings.ReasoningProvider = "ollama-nemotron"
	if err := state.SaveSettings(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.CompleteOnboarding(context.Background(), []domain.Source{domain.SourceX}); err == nil || !strings.Contains(err.Error(), "not available") {
		t.Fatalf("first onboarding error=%v", err)
	}
}
