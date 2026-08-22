package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/engine"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/AkuSidecar/internal/store"
	"github.com/abangkis/ai4u-inference-sdk-go/calibration"
)

func mustUnmarshalReport(t *testing.T, raw []byte) calibration.Report {
	t.Helper()
	var report calibration.Report
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatal(err)
	}
	return report
}

func TestCalibrationLedgerRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "calibration-results.jsonl")
	if err := appendCalibrationReport(path, mustUnmarshalReport(t, []byte("{\"adapterId\":\"ollama\",\"modelId\":\"nemotron-3.5-lightning\",\"timestamp\":\"2026-08-22T00:00:00Z\",\"result\":{\"success\":true}}"))); err != nil {
		t.Fatal(err)
	}
	if err := appendCalibrationReport(path, mustUnmarshalReport(t, []byte("{\"adapterId\":\"ollama\",\"modelId\":\"qwen3.8:27b\",\"timestamp\":\"2026-08-22T01:00:00Z\",\"result\":{\"success\":false},\"error\":{\"code\":\"provider\"}}"))); err != nil {
		t.Fatal(err)
	}
	reports, err := readCalibrationReports(path, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 2 {
		t.Fatalf("reports=%d", len(reports))
	}
	if reports[0].ModelID != "qwen3.8:27b" || reports[0].Error == nil || reports[0].Error.Code != "provider" {
		t.Fatalf("newest-first ordering violated: %+v", reports[0])
	}
	limited, err := readCalibrationReports(path, 1)
	if err != nil || len(limited) != 1 || limited[0].ModelID != "qwen3.8:27b" {
		t.Fatalf("limit window wrong: %+v err=%v", limited, err)
	}
	empty, err := readCalibrationReports(filepath.Join(t.TempDir(), "missing.jsonl"), 5)
	if err != nil || len(empty) != 0 {
		t.Fatalf("missing ledger must read empty: %+v err=%v", empty, err)
	}
}

func TestCalibrationEndpointsRoundTrip(t *testing.T) {
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/show" {
			_, _ = fmt.Fprint(w, "{\"capabilities\":[\"completion\",\"thinking\"]}")
			return
		}
		_, _ = fmt.Fprint(w, "{\"message\":{\"content\":\"{\\\"ok\\\":true,\\\"label\\\":\\\"calibration\\\"}\"},\"done\":true,\"done_reason\":\"stop\",\"prompt_eval_count\":4,\"eval_count\":6}")
	}))
	defer fake.Close()

	settings := domain.DefaultSettings("standard", "quiet", "promote_unused_budget", true)
	state, err := store.Open(filepath.Join(t.TempDir(), "sidecar.db"), settings)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	cfg := config.Config{
		Root:       t.TempDir(),
		Server:     config.ServerConfig{Host: "127.0.0.1", Port: 0},
		Capture:    config.CaptureConfig{Profile: "standard", Visibility: "quiet", OpenMissingSource: true, MaxAcquisitionRounds: 2},
		Preference: config.PreferenceConfig{Mode: "promote_unused_budget"},
		Reasoning: config.ReasoningConfig{
			Provider: "ollama-nemotron", Endpoint: fake.URL, TimeoutMS: 5000,
			Evaluation: config.ModelConfig{Model: "nemotron-3.5-lightning", Effort: "high"},
		},
	}
	logger := log.New(io.Discard, "", 0)
	runtime := engine.New(state, reasoning.Deterministic{}, cfg, logger)
	server, err := New(cfg, state, runtime, logger)
	if err != nil {
		t.Fatal(err)
	}
	address, err := server.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer server.Stop(context.Background())
	client := http.Client{Timeout: 5 * time.Second}

	post, err := client.Post("http://"+address.String()+"/api/diagnostics/calibration", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer post.Body.Close()
	var payload struct {
		Report calibration.Report `json:"report"`
		Error  string             `json:"error,omitempty"`
	}
	if err := json.NewDecoder(post.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if post.StatusCode != http.StatusOK || !payload.Report.Result.Success || payload.Error != "" {
		t.Fatalf("post status=%d payload=%+v", post.StatusCode, payload)
	}

	get, err := client.Get("http://" + address.String() + "/api/diagnostics/calibration")
	if err != nil {
		t.Fatal(err)
	}
	defer get.Body.Close()
	var listed struct {
		Reports []calibration.Report `json:"reports"`
	}
	if err := json.NewDecoder(get.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Reports) != 1 || listed.Reports[0].ModelID != "nemotron-3.5-lightning" {
		t.Fatalf("ledger via API=%+v", listed.Reports)
	}
}
