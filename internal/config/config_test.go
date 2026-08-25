package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodexAppServerRequiresSeparateAIDetectionProfile(t *testing.T) {
	cfg := Config{
		Server: ServerConfig{Host: "127.0.0.1", Port: 11122}, Database: DatabaseConfig{Path: "test.db"},
		Reasoning: ReasoningConfig{
			ActiveProvider: "codex-app-server",
			Providers: map[string]ProviderConfig{
				"codex-app-server": {
					TimeoutMS: 5000,
					Planning:  ModelConfig{Model: "planner", Effort: "high"}, Evaluation: ModelConfig{Model: "evaluator", Effort: "high"},
					SemanticEvent: ModelConfig{Model: "event", Effort: "high"},
				},
			},
		},
		Capture: CaptureConfig{MaxAcquisitionRounds: 1},
		Bridge:  BridgeConfig{TrustedExtensionOrigins: []string{"chrome-extension://abcdefghijklmnopabcdefghijklmnop/"}},
	}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "AI detection model") {
		t.Fatalf("missing AI Detector profile error=%v", err)
	}
	entry := cfg.Reasoning.Providers["codex-app-server"]
	entry.AIDetection = ModelConfig{Model: "detector", Effort: "medium"}
	cfg.Reasoning.Providers["codex-app-server"] = entry
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestBridgeExtensionOriginFlagCompletesBaseConfiguration(t *testing.T) {
	temp := t.TempDir()
	configPath := filepath.Join(temp, "config", "sidecar.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	data := `{
  "version": 1,
  "server": {"host": "127.0.0.1", "port": 11122},
  "database": {"path": "runtime/test.db"},
  "reasoning": {
    "provider": "deterministic",
    "executable": "",
    "timeoutMs": 5000,
    "planning": {"model": "planner", "effort": "high"},
    "evaluation": {"model": "evaluator", "effort": "high"},
    "semanticEvent": {"model": "event", "effort": "high"},
    "aiDetection": {"model": "detector", "effort": "high"}
  },
  "capture": {"profile": "standard", "visibility": "quiet", "openMissingSource": true, "maxAcquisitionRounds": 1},
  "bridge": {"trustedExtensionOrigins": []},
  "preference": {"mode": "guarded_live"}
}`
	if err := os.WriteFile(configPath, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	const origin = "chrome-extension://abcdefghijklmnopabcdefghijklmnop/"
	cfg, err := Load(Options{ConfigPath: configPath, BridgeExtensionOrigin: origin})
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Bridge.TrustedExtensionOrigins) != 1 || cfg.Bridge.TrustedExtensionOrigins[0] != origin {
		t.Fatalf("trusted origins=%v", cfg.Bridge.TrustedExtensionOrigins)
	}
	if cfg.Reasoning.Provider != "deterministic" || cfg.Reasoning.ActiveProvider != "deterministic" {
		t.Fatalf("legacy flat shape did not select the declared provider: %+v", cfg.Reasoning)
	}
	if len(cfg.Reasoning.Providers) != 1 || cfg.Reasoning.Providers["deterministic"].Planning.Model != "planner" {
		t.Fatalf("legacy flat shape was not projected into a provider entry: %+v", cfg.Reasoning.Providers)
	}
}

func TestMultiProviderConfigurationSelectsActiveProvider(t *testing.T) {
	temp := t.TempDir()
	configPath := filepath.Join(temp, "config", "sidecar.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	data := `{
  "version": 1,
  "server": {"host": "127.0.0.1", "port": 11122},
  "database": {"path": "runtime/test.db"},
  "reasoning": {
    "activeProvider": "codex-app-server",
    "providers": {
      "codex-app-server": {
        "executable": "C:/codex.exe",
        "timeoutMs": 120000,
        "planning": {"model": "gpt-5.6-luna", "effort": "high"},
        "evaluation": {"model": "gpt-5.6-luna", "effort": "high"},
        "semanticEvent": {"model": "gpt-5.6-luna", "effort": "high"},
        "aiDetection": {"model": "gpt-5.6-luna", "effort": "high"}
      },
      "ollama-nemotron": {
        "endpoint": "http://127.0.0.1:11434",
        "maxRetries": 1,
        "timeoutMs": 300000,
        "planning": {"model": "nemotron-3.5-lightning:latest", "effort": "high"},
        "evaluation": {"model": "nemotron-3.5-lightning:latest", "effort": "high"},
        "semanticEvent": {"model": "nemotron-3.5-lightning:latest", "effort": "high"},
        "aiDetection": {"model": "nemotron-3.5-lightning:latest", "effort": "high"}
      },
      "ollama-qwen": {
        "endpoint": "http://127.0.0.1:11434",
        "maxRetries": 1,
        "timeoutMs": 300000,
        "warmupTimeoutMs": 300000,
        "keepAliveMinutes": 300,
        "numCtx": 32768,
        "planning": {"model": "qwen3.8:27b", "effort": "high"},
        "evaluation": {"model": "qwen3.8:27b", "effort": "high"},
        "semanticEvent": {"model": "qwen3.8:27b", "effort": "high"},
        "aiDetection": {"model": "qwen3.8:27b", "effort": "high"}
      }
    }
  },
  "capture": {"profile": "standard", "visibility": "quiet", "openMissingSource": true, "maxAcquisitionRounds": 1},
  "bridge": {"trustedExtensionOrigins": ["chrome-extension://abcdefghijklmnopabcdefghijklmnop/"]},
  "preference": {"mode": "guarded_live"}
}`
	if err := os.WriteFile(configPath, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(Options{ConfigPath: configPath})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Reasoning.Provider != "codex-app-server" {
		t.Fatalf("active provider=%q", cfg.Reasoning.Provider)
	}
	if cfg.Reasoning.Executable != "C:/codex.exe" || cfg.Reasoning.Planning.Model != "gpt-5.6-luna" {
		t.Fatalf("active projection=%+v", cfg.Reasoning)
	}
	summaries := cfg.Reasoning.ProviderSummary()
	if len(summaries) != 3 ||
		summaries[0].Name != "codex-app-server" || summaries[0].Label != "Codex App Server" ||
		summaries[1].Name != "ollama-nemotron" || summaries[1].Label != "Ollama · Nemotron 3.5 Lightning" ||
		summaries[2].Name != "ollama-qwen" || summaries[2].Label != "Ollama · Qwen 3.8 27B" {
		t.Fatalf("provider summaries=%+v", summaries)
	}
	if err := cfg.Reasoning.Select("ollama-nemotron"); err != nil {
		t.Fatal(err)
	}
	if cfg.Reasoning.Provider != "ollama-nemotron" || cfg.Reasoning.Endpoint != "http://127.0.0.1:11434" || cfg.Reasoning.MaxRetries != 1 || cfg.Reasoning.TimeoutMS != 300000 || cfg.Reasoning.Planning.Model != "nemotron-3.5-lightning:latest" {
		t.Fatalf("re-projected ollama-nemotron=%+v", cfg.Reasoning)
	}
	if err := cfg.Reasoning.Select("ollama-qwen"); err != nil {
		t.Fatal(err)
	}
	if cfg.Reasoning.Provider != "ollama-qwen" || cfg.Reasoning.Planning.Model != "qwen3.8:27b" ||
		cfg.Reasoning.WarmupTimeoutMS != 300000 || cfg.Reasoning.KeepAliveMinutes != 300 || cfg.Reasoning.NumCtx != 32768 {
		t.Fatalf("re-projected ollama-qwen=%+v", cfg.Reasoning)
	}
	if err := cfg.Reasoning.Select("missing"); err == nil || !strings.Contains(err.Error(), "not declared") {
		t.Fatalf("undeclared provider selection error=%v", err)
	}
}

func TestActiveReasoningProviderMustBeDeclared(t *testing.T) {
	cfg := Config{
		Server: ServerConfig{Host: "127.0.0.1", Port: 11122}, Database: DatabaseConfig{Path: "test.db"},
		Reasoning: ReasoningConfig{
			ActiveProvider: "claude",
			Providers: map[string]ProviderConfig{
				"ollama": {
					TimeoutMS:     5000,
					Endpoint:      "http://127.0.0.1:11434",
					Planning:      ModelConfig{Model: "nemotron", Effort: "high"},
					Evaluation:    ModelConfig{Model: "nemotron", Effort: "high"},
					SemanticEvent: ModelConfig{Model: "nemotron", Effort: "high"},
					AIDetection:   ModelConfig{Model: "nemotron", Effort: "high"},
				},
			},
		},
		Capture: CaptureConfig{MaxAcquisitionRounds: 1},
		Bridge:  BridgeConfig{TrustedExtensionOrigins: []string{"chrome-extension://abcdefghijklmnopabcdefghijklmnop/"}},
	}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "active reasoning provider") {
		t.Fatalf("undeclared active provider error=%v", err)
	}
}

func TestGroqRequiresReferenceWithoutResolvingSecretDuringConfigLoad(t *testing.T) {
	model := ModelConfig{ModelID: "openai/gpt-oss-120b", MinReasoningTier: "high", ReasoningOptionID: "high"}
	provider := ProviderConfig{CredentialRef: "groq.primary", TimeoutMS: 120000, Planning: model, Evaluation: model, SemanticEvent: model, AIDetection: model}
	if err := provider.Validate("groq"); err != nil {
		t.Fatal(err)
	}
	if label := ProviderLabel("groq"); label != "Groq · GPT-OSS 120B" {
		t.Fatalf("label=%q", label)
	}
	provider.CredentialRef = ""
	if err := provider.Validate("groq"); err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("missing reference error=%v", err)
	}
	provider.CredentialRef = "gemini.primary"
	if err := provider.Validate("groq"); err == nil || !strings.Contains(err.Error(), "wrong provider prefix") {
		t.Fatalf("unexpected reference error=%v", err)
	}
}

func TestAssessmentProviderCanRemainHiddenFromSettings(t *testing.T) {
	reasoning := ReasoningConfig{Providers: map[string]ProviderConfig{
		"codex-app-server": {},
		"groq":             {HideFromSettings: true},
	}}
	summaries := reasoning.ProviderSummary()
	if len(summaries) != 1 || summaries[0].Name != "codex-app-server" {
		t.Fatalf("summaries=%+v", summaries)
	}
}

func TestGeminiProviderRequiresCanonicalCredentialReference(t *testing.T) {
	model := ModelConfig{ModelID: "gemini-3.5-flash-lite", MinReasoningTier: "high", ReasoningOptionID: "high", MaxOutputTokens: 512}
	provider := ProviderConfig{CredentialRef: "gemini.primary", TimeoutMS: 120000, Planning: model, Evaluation: model, SemanticEvent: model, AIDetection: model}
	if err := provider.Validate("gemini-flash-lite"); err != nil {
		t.Fatal(err)
	}
	if label := ProviderLabel("gemini-flash-lite"); label != "Gemini · 3.5 Flash Lite" {
		t.Fatalf("label=%q", label)
	}
	reasoning := ReasoningConfig{Providers: map[string]ProviderConfig{
		"codex-app-server":  {},
		"gemini-flash-lite": provider,
	}}
	summaries := reasoning.ProviderSummary()
	if len(summaries) != 2 || summaries[1].Name != "gemini-flash-lite" || summaries[1].RuntimeKind != "remote_api" || !summaries[1].Configured {
		t.Fatalf("Gemini provider summary=%+v", summaries)
	}
	provider.CredentialRef = "groq.primary"
	if err := provider.Validate("gemini-flash-lite"); err == nil || !strings.Contains(err.Error(), "wrong provider prefix") {
		t.Fatalf("unexpected reference error=%v", err)
	}
}

func TestConcurrencyConfigurationDefaultsAndOptInValidation(t *testing.T) {
	ollama := ProviderConfig{TimeoutMS: 5000,
		Planning: ModelConfig{Model: "nemotron", Effort: "high"}, Evaluation: ModelConfig{Model: "nemotron", Effort: "high"},
		SemanticEvent: ModelConfig{Model: "nemotron", Effort: "high"}, AIDetection: ModelConfig{Model: "nemotron", Effort: "high"}}
	if err := ollama.Validate("ollama"); err != nil {
		t.Fatal(err)
	}
	if err := (ProviderConfig{TimeoutMS: 5000, MaxConcurrentInvocations: 65}).Validate("ollama"); err == nil {
		t.Fatal("Ollama concurrency above the SDK bound must fail")
	}
	if err := (ProviderConfig{TimeoutMS: 5000, CodexSessionPoolSize: 2}).Validate("codex-app-server"); err == nil {
		// The test intentionally supplies required model profiles below; this
		// branch only guards accidental acceptance of an incomplete provider.
		t.Fatal("incomplete Codex provider unexpectedly validated")
	}
	codex := ProviderConfig{TimeoutMS: 5000, CodexSessionPoolSize: 2,
		Planning: ModelConfig{Model: "codex", Effort: "high"}, Evaluation: ModelConfig{Model: "codex", Effort: "high"},
		SemanticEvent: ModelConfig{Model: "codex", Effort: "high"}, AIDetection: ModelConfig{Model: "codex", Effort: "high"}}
	if err := codex.Validate("codex-app-server"); err != nil {
		t.Fatal(err)
	}
}

func TestRepositorySidecarConfigLoads(t *testing.T) {
	configPath := filepath.Join("..", "..", "config", "sidecar.json")
	if _, err := os.Stat(configPath); err != nil {
		t.Skip("repository config not present")
	}
	cfg, err := Load(Options{ConfigPath: configPath, BridgeExtensionOrigin: "chrome-extension://abcdefghijklmnopabcdefghijklmnop/"})
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Reasoning.Providers) == 0 || cfg.Reasoning.Provider == "" {
		t.Fatalf("repository config did not resolve a provider: %+v", cfg.Reasoning)
	}
}

func TestDeploymentIdentityCombinations(t *testing.T) {
	valid := []DeploymentConfig{
		{Mode: "development", RuntimeInstallKind: "workspace", BridgeIdentityProfile: "development"},
		{Mode: "acceptance", RuntimeInstallKind: "installed", BridgeIdentityProfile: "acceptance", ReleaseVersion: "0.8.0"},
		{Mode: "production-store", RuntimeInstallKind: "installed", BridgeIdentityProfile: "production-store", ReleaseVersion: "0.8.0"},
		{Mode: "production-offline", RuntimeInstallKind: "portable", BridgeIdentityProfile: "production-offline", ReleaseVersion: "0.8.0"},
		{Mode: "production-installed-app", RuntimeInstallKind: "installed", BridgeIdentityProfile: "production-app", ReleaseVersion: "0.8.0"},
	}
	for _, deployment := range valid {
		if err := deployment.Validate(); err != nil {
			t.Fatalf("valid deployment %+v: %v", deployment, err)
		}
	}

	invalid := []DeploymentConfig{
		{Mode: "acceptance", RuntimeInstallKind: "installed", BridgeIdentityProfile: "development", ReleaseVersion: "0.8.0"},
		{Mode: "production-store", RuntimeInstallKind: "portable", BridgeIdentityProfile: "production-store", ReleaseVersion: "0.8.0"},
		{Mode: "production-offline", RuntimeInstallKind: "portable", BridgeIdentityProfile: "production-offline"},
		{Mode: "production-installed-app", RuntimeInstallKind: "portable", BridgeIdentityProfile: "production-app", ReleaseVersion: "0.8.0"},
		{Mode: "production-installed-app", RuntimeInstallKind: "installed", BridgeIdentityProfile: "production-offline", ReleaseVersion: "0.8.0"},
	}
	for _, deployment := range invalid {
		if err := deployment.Validate(); err == nil {
			t.Fatalf("invalid deployment accepted: %+v", deployment)
		}
	}
}

func TestOllamaRequiresModelsAndBoundsRetries(t *testing.T) {
	base := Config{
		Server: ServerConfig{Host: "127.0.0.1", Port: 11122}, Database: DatabaseConfig{Path: "test.db"},
		Reasoning: ReasoningConfig{
			ActiveProvider: "ollama-nemotron",
			Providers: map[string]ProviderConfig{
				"ollama-nemotron": {
					TimeoutMS:     5000,
					Endpoint:      "http://127.0.0.1:11434",
					Planning:      ModelConfig{Model: "nemotron", Effort: "high"},
					Evaluation:    ModelConfig{Model: "nemotron", Effort: "high"},
					SemanticEvent: ModelConfig{Model: "nemotron", Effort: "high"},
					AIDetection:   ModelConfig{Model: "nemotron", Effort: "high"},
				},
				"ollama-qwen": {
					TimeoutMS:     5000,
					Endpoint:      "http://127.0.0.1:11434",
					Planning:      ModelConfig{Model: "qwen3.8:27b", Effort: "high"},
					Evaluation:    ModelConfig{Model: "qwen3.8:27b", Effort: "high"},
					SemanticEvent: ModelConfig{Model: "qwen3.8:27b", Effort: "high"},
					AIDetection:   ModelConfig{Model: "qwen3.8:27b", Effort: "high"},
				},
			},
		},
		Capture: CaptureConfig{MaxAcquisitionRounds: 1},
		Bridge:  BridgeConfig{TrustedExtensionOrigins: []string{"chrome-extension://abcdefghijklmnopabcdefghijklmnop/"}},
	}
	if err := base.Validate(); err != nil {
		t.Fatalf("valid ollama configuration rejected: %v", err)
	}
	if err := base.Reasoning.Select("ollama-qwen"); err != nil {
		t.Fatal(err)
	}
	if base.Reasoning.Planning.Model != "qwen3.8:27b" {
		t.Fatalf("selected ollama-qwen projection=%+v", base.Reasoning)
	}
	missing := base
	missing.Reasoning.Providers = copyProviders(base.Reasoning.Providers)
	entry := missing.Reasoning.Providers["ollama-nemotron"]
	entry.Planning = ModelConfig{Model: "nemotron"}
	missing.Reasoning.Providers["ollama-nemotron"] = entry
	if err := missing.Validate(); err == nil || !strings.Contains(err.Error(), "minimum reasoning tier") {
		t.Fatalf("missing planning model error=%v", err)
	}
	qwenMissing := base
	qwenMissing.Reasoning.Providers = copyProviders(base.Reasoning.Providers)
	qwenEntry := qwenMissing.Reasoning.Providers["ollama-qwen"]
	qwenEntry.AIDetection = ModelConfig{Model: "qwen3.8:27b"}
	qwenMissing.Reasoning.Providers["ollama-qwen"] = qwenEntry
	if err := qwenMissing.Validate(); err == nil || !strings.Contains(err.Error(), "minimum reasoning tier") {
		t.Fatalf("missing qwen ai detection model error=%v", err)
	}
	retries := base
	retries.Reasoning.Providers = copyProviders(base.Reasoning.Providers)
	entry = retries.Reasoning.Providers["ollama-nemotron"]
	entry.MaxRetries = 6
	retries.Reasoning.Providers["ollama-nemotron"] = entry
	if err := retries.Validate(); err == nil || !strings.Contains(err.Error(), "max retries") {
		t.Fatalf("out of range retries error=%v", err)
	}
	unsupported := base
	unsupported.Reasoning.Providers = copyProviders(base.Reasoning.Providers)
	unsupported.Reasoning.Providers = map[string]ProviderConfig{"claude": unsupported.Reasoning.Providers["ollama-nemotron"]}
	unsupported.Reasoning.ActiveProvider = "claude"
	if err := unsupported.Validate(); err == nil || !strings.Contains(err.Error(), "unsupported reasoning provider") {
		t.Fatalf("unsupported provider error=%v", err)
	}
}

func TestProviderSpecificWarmAndKeepAliveOnlyOnOllama(t *testing.T) {
	base := Config{
		Server: ServerConfig{Host: "127.0.0.1", Port: 11122}, Database: DatabaseConfig{Path: "test.db"},
		Reasoning: ReasoningConfig{
			ActiveProvider: "codex-app-server",
			Providers: map[string]ProviderConfig{
				"codex-app-server": {
					TimeoutMS:        5000,
					WarmupTimeoutMS:  1000,
					KeepAliveMinutes: 60,
					NumCtx:           32768,
					Planning:         ModelConfig{Model: "gpt-5.6-luna", Effort: "high"},
					Evaluation:       ModelConfig{Model: "gpt-5.6-luna", Effort: "high"},
					SemanticEvent:    ModelConfig{Model: "gpt-5.6-luna", Effort: "high"},
					AIDetection:      ModelConfig{Model: "gpt-5.6-luna", Effort: "high"},
				},
			},
		},
		Capture: CaptureConfig{MaxAcquisitionRounds: 1},
		Bridge:  BridgeConfig{TrustedExtensionOrigins: []string{"chrome-extension://abcdefghijklmnopabcdefghijklmnop/"}},
	}
	err := base.Validate()
	if err == nil || !strings.Contains(err.Error(), "apply only to ollama providers") {
		t.Fatalf("expected ollama-only restriction, got %v", err)
	}
	ollamaBase := base
	ollamaBase.Reasoning.ActiveProvider = "ollama-nemotron"
	ollamaBase.Reasoning.Providers = map[string]ProviderConfig{
		"ollama-nemotron": {
			TimeoutMS:        5000,
			WarmupTimeoutMS:  1000,
			KeepAliveMinutes: 60,
			NumCtx:           32768,
			Endpoint:         "http://127.0.0.1:11434",
			Planning:         ModelConfig{Model: "nemotron", Effort: "high"},
			Evaluation:       ModelConfig{Model: "nemotron", Effort: "high"},
			SemanticEvent:    ModelConfig{Model: "nemotron", Effort: "high"},
			AIDetection:      ModelConfig{Model: "nemotron", Effort: "high"},
		},
	}
	if err := ollamaBase.Validate(); err != nil {
		t.Fatalf("valid ollama warm/keepalive config rejected: %v", err)
	}
	if err := ollamaBase.Reasoning.Select("ollama-nemotron"); err != nil {
		t.Fatal(err)
	}
	if ollamaBase.Reasoning.WarmupTimeoutMS != 1000 || ollamaBase.Reasoning.KeepAliveMinutes != 60 || ollamaBase.Reasoning.NumCtx != 32768 {
		t.Fatalf("warm/keepalive/numCtx not projected: %+v", ollamaBase.Reasoning)
	}
}

func copyProviders(providers map[string]ProviderConfig) map[string]ProviderConfig {
	cloned := make(map[string]ProviderConfig, len(providers))
	for name, provider := range providers {
		cloned[name] = provider
	}
	return cloned
}
