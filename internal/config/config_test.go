package config

import (
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
	}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "AI detection model") {
		t.Fatalf("missing AI Detector profile error=%v", err)
	}
	cfg.Reasoning.AIDetection = ModelConfig{Model: "detector", Effort: "medium"}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}
