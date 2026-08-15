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
			Provider: "codex-app-server", TimeoutMS: 5000,
			Planning: ModelConfig{Model: "planner", Effort: "high"}, Evaluation: ModelConfig{Model: "evaluator", Effort: "high"},
			SemanticEvent: ModelConfig{Model: "event", Effort: "high"},
		},
		Capture: CaptureConfig{MaxAcquisitionRounds: 1},
		Bridge:  BridgeConfig{TrustedExtensionOrigins: []string{"chrome-extension://abcdefghijklmnopabcdefghijklmnop/"}},
	}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "AI detection model") {
		t.Fatalf("missing AI Detector profile error=%v", err)
	}
	cfg.Reasoning.AIDetection = ModelConfig{Model: "detector", Effort: "medium"}
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
