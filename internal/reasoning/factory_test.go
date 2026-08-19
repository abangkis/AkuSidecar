package reasoning

import (
	"os"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/config"
)

func TestNewProviderRoutesModelScopedOllamaEntries(t *testing.T) {
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
	for _, provider := range []string{"ollama", "ollama-nemotron", "ollama-qwen"} {
		cfg := base
		if err := cfg.Reasoning.Select(provider); err != nil {
			t.Fatal(err)
		}
		got, err := NewProvider(cfg)
		if err != nil {
			t.Fatalf("NewProvider(%q) error=%v", provider, err)
		}
		if _, ok := got.(*Ollama); !ok {
			t.Fatalf("NewProvider(%q) type=%T, want *Ollama", provider, got)
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
