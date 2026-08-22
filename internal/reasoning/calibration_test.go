package reasoning

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/config"
)

func calibrationTestConfig(t *testing.T, serverURL string) config.Config {
	t.Helper()
	return config.Config{
		Root: filepathRoot(t),
		Reasoning: config.ReasoningConfig{
			Provider:   "ollama-nemotron",
			Endpoint:   serverURL,
			TimeoutMS:  5000,
			Evaluation: config.ModelConfig{Model: "nemotron-3.5-lightning", Effort: "high"},
		},
	}
}

func TestRunCalibrationOllamaSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/show" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, "{\"capabilities\":[\"completion\",\"thinking\"]}")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, "{\"message\":{\"content\":\"{\\\"ok\\\":true,\\\"label\\\":\\\"calibration\\\"}\"},\"done\":true,\"done_reason\":\"stop\",\"prompt_eval_count\":4,\"eval_count\":6}")
	}))
	defer server.Close()

	report, err := RunCalibration(context.Background(), calibrationTestConfig(t, server.URL))
	if err != nil {
		t.Fatalf("calibration error=%v report=%+v", err, report)
	}
	if !report.Result.Success {
		t.Fatalf("expected success report=%+v", report)
	}
	if report.ModelID != "nemotron-3.5-lightning" || report.AdapterID != "ollama" {
		t.Fatalf("report identity=%+v", report)
	}
	if report.Capability.ReasoningTier == "" {
		t.Fatalf("capability observations missing: %+v", report.Capability)
	}
}

func TestRunCalibrationUnsupportedProviderFailsClosed(t *testing.T) {
	cfg := calibrationTestConfig(t, "http://127.0.0.1:9")
	cfg.Reasoning.Provider = "deterministic"
	report, err := RunCalibration(context.Background(), cfg)
	if err == nil {
		t.Fatal("deterministic provider must not calibrate")
	}
	if !strings.Contains(err.Error(), "unavailable for provider") {
		t.Fatalf("unexpected error=%v", err)
	}
	if report.ModelID != "" {
		t.Fatalf("failed configuration must not emit a report: %+v", report)
	}
}
