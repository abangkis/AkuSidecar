package reasoning

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
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

func TestRunCalibrationRejectsRetiredOllamaModelBeforeEndpoint(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	cfg := calibrationTestConfig(t, server.URL)
	report, err := RunCalibration(context.Background(), cfg)
	var unsupported *UnsupportedModelError
	if !errors.As(err, &unsupported) || unsupported.Provider != "ollama" || unsupported.ModelID != "nemotron-3.5-lightning" {
		t.Fatalf("error=%T %v, want typed retired-model error", err, err)
	}
	if requests.Load() != 0 {
		t.Fatalf("unsupported calibration contacted endpoint %d times", requests.Load())
	}
	if report.ModelID != "" || cfg.Reasoning.Evaluation.Model != "nemotron-3.5-lightning" {
		t.Fatalf("failed calibration report=%+v config model=%q", report, cfg.Reasoning.Evaluation.Model)
	}
	if !strings.Contains(err.Error(), "no model substitution was made") {
		t.Fatalf("unsupported error is not actionable: %v", err)
	}
}

func TestRunCalibrationRejectsCodexBeforeExecutableResolution(t *testing.T) {
	cfg := config.Config{Reasoning: config.ReasoningConfig{Provider: "codex-app-server"}}
	report, err := RunCalibration(context.Background(), cfg)
	if !errors.Is(err, ErrCalibrationUnsupported) {
		t.Fatalf("error=%v, want ErrCalibrationUnsupported", err)
	}
	if report.ModelID != "" {
		t.Fatalf("unsupported calibration emitted report: %+v", report)
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
