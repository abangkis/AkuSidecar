package reasoning

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/config"
)

func TestNewProviderRejectsOllamaEntriesWithoutStableModels(t *testing.T) {
	// Codex's local runtime probe is satisfied by the package's fake executable;
	// no installed account or provider process is consulted.
	t.Setenv("AKU_FAKE_CODEX_APP_SERVER", "1")
	base := config.Config{
		Root: filepathRoot(t),
		Reasoning: config.ReasoningConfig{
			Providers: map[string]config.ProviderConfig{
				"ollama":          {Endpoint: "http://127.0.0.1:11434", TimeoutMS: 5000, Planning: config.ModelConfig{Model: "nemotron", Effort: "high"}},
				"ollama-nemotron": {Endpoint: "http://127.0.0.1:11434", TimeoutMS: 5000, Planning: config.ModelConfig{Model: "nemotron", Effort: "high"}},
				"ollama-qwen":     {Endpoint: "http://127.0.0.1:11434", TimeoutMS: 5000, Planning: config.ModelConfig{Model: "qwen3.8:27b", Effort: "high"}},
			},
		},
	}
	for provider, modelID := range map[string]string{
		"ollama": "nemotron", "ollama-nemotron": "nemotron", "ollama-qwen": "qwen3.8:27b",
	} {
		cfg := base
		if err := cfg.Reasoning.Select(provider); err != nil {
			t.Fatal(err)
		}
		_, err := NewProvider(cfg)
		var unsupported *UnsupportedModelError
		if !errors.As(err, &unsupported) || unsupported.Provider != "ollama" || unsupported.ModelID != modelID {
			t.Fatalf("NewProvider(%q) error=%T %v, want unsupported model %q", provider, err, err, modelID)
		}
		if cfg.Reasoning.Planning.StableModelID() != modelID {
			t.Fatalf("provider %q configured model changed to %q", provider, cfg.Reasoning.Planning.StableModelID())
		}
	}

	codex := base
	if err := codex.Reasoning.Select("ollama-nemotron"); err != nil {
		t.Fatal(err)
	}
	codex.Reasoning.Providers = map[string]config.ProviderConfig{
		"codex-app-server": {Executable: os.Args[0], TimeoutMS: 120000, Planning: config.ModelConfig{Model: "gpt-5.6-luna", Effort: "high"}},
	}
	codex.Reasoning.ActiveProvider = "codex-app-server"
	if err := codex.Reasoning.Select("codex-app-server"); err != nil {
		t.Fatal(err)
	}
	got, err := NewProvider(codex)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.(*CodexAppServer); !ok {
		t.Fatalf("codex-app-server type=%T, want *CodexAppServer", got)
	}

	deterministic := base
	if err := deterministic.Reasoning.Select("ollama-nemotron"); err != nil {
		t.Fatal(err)
	}
	deterministic.Reasoning.Providers = map[string]config.ProviderConfig{"deterministic": {}}
	deterministic.Reasoning.ActiveProvider = "deterministic"
	if err := deterministic.Reasoning.Select("deterministic"); err != nil {
		t.Fatal(err)
	}
	if _, err := NewProvider(deterministic); err != nil {
		t.Fatalf("deterministic error=%v", err)
	}

	unsupported := base
	if err := unsupported.Reasoning.Select("ollama-nemotron"); err != nil {
		t.Fatal(err)
	}
	unsupported.Reasoning.Provider = "claude"
	if _, err := NewProvider(unsupported); err == nil || !strings.Contains(err.Error(), "unsupported reasoning provider") {
		t.Fatalf("unsupported provider error=%v", err)
	}
}

func TestNewProviderRoutesGeminiEntries(t *testing.T) {
	server := geminiTestServer(t, `{}`)
	cfg := geminiTestConfig(t, server.URL)
	root := t.TempDir()
	for _, relative := range []string{
		"schemas/acquisition-plan.schema.json",
		"schemas/reasoning-result.schema.json",
	} {
		source, err := os.ReadFile(filepath.Join(cfg.Root, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, source, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	credentialPath := filepath.Join(root, "runtime", "config", "credentials.local.json")
	if err := os.MkdirAll(filepath.Dir(credentialPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(credentialPath, []byte(`{"schemaVersion":1,"credentialStore":{"type":"inline","values":{"gemini.primary":"gemini-test-key"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg.Root = root
	cfg.Dev = true

	got, err := NewProvider(cfg)
	if err != nil {
		t.Fatalf("NewProvider(gemini-flash-lite) error=%v", err)
	}
	gemini, ok := got.(*Gemini)
	if !ok {
		t.Fatalf("NewProvider(gemini-flash-lite) type=%T, want *Gemini", got)
	}
	defer gemini.Close()
}
