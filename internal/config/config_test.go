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
      "ollama": {
        "endpoint": "http://127.0.0.1:11434",
        "maxRetries": 1,
        "timeoutMs": 300000,
        "planning": {"model": "nemotron-3.5-lightning:latest", "effort": "high"},
        "evaluation": {"model": "nemotron-3.5-lightning:latest", "effort": "high"},
        "semanticEvent": {"model": "nemotron-3.5-lightning:latest", "effort": "high"},
        "aiDetection": {"model": "nemotron-3.5-lightning:latest", "effort": "high"}
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
	if len(summaries) != 2 || summaries[0].Name != "codex-app-server" || summaries[0].Label != "Codex App Server" || summaries[1].Name != "ollama" || summaries[1].Label != "Ollama" {
		t.Fatalf("provider summaries=%+v", summaries)
	}
	if err := cfg.Reasoning.Select("ollama"); err != nil {
		t.Fatal(err)
	}
	if cfg.Reasoning.Provider != "ollama" || cfg.Reasoning.Endpoint != "http://127.0.0.1:11434" || cfg.Reasoning.MaxRetries != 1 || cfg.Reasoning.TimeoutMS != 300000 || cfg.Reasoning.Planning.Model != "nemotron-3.5-lightning:latest" {
		t.Fatalf("re-projected ollama=%+v", cfg.Reasoning)
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
			ActiveProvider: "ollama",
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
	if err := base.Validate(); err != nil {
		t.Fatalf("valid ollama configuration rejected: %v", err)
	}
	missing := base
	entry := missing.Reasoning.Providers["ollama"]
	entry.Planning = ModelConfig{Model: "nemotron"}
	missing.Reasoning.Providers["ollama"] = entry
	if err := missing.Validate(); err == nil || !strings.Contains(err.Error(), "planning model and effort") {
		t.Fatalf("missing planning model error=%v", err)
	}
	retries := base
	entry = retries.Reasoning.Providers["ollama"]
	entry.MaxRetries = 6
	retries.Reasoning.Providers["ollama"] = entry
	if err := retries.Validate(); err == nil || !strings.Contains(err.Error(), "max retries") {
		t.Fatalf("out of range retries error=%v", err)
	}
	unsupported := base
	unsupported.Reasoning.Providers = map[string]ProviderConfig{"claude": unsupported.Reasoning.Providers["ollama"]}
	unsupported.Reasoning.ActiveProvider = "claude"
	if err := unsupported.Validate(); err == nil || !strings.Contains(err.Error(), "unsupported reasoning provider") {
		t.Fatalf("unsupported provider error=%v", err)
	}
}
