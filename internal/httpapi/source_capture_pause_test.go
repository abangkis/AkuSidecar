package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/engine"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/AkuSidecar/internal/store"
)

func TestSourceCapturePauseHTTPRequiresCurrentExplicitConfirmation(t *testing.T) {
	ctx := context.Background()
	settings := domain.DefaultSettings("expanded", "quiet", "guarded_live", true)
	settings.AutoUpdateEnabled = false
	state, err := store.Open(filepath.Join(t.TempDir(), "synthetic.db"), settings)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	logger := log.New(io.Discard, "", 0)
	cfg := config.Config{}
	runtime := engine.New(state, reasoning.Deterministic{}, cfg, logger)
	server, err := New(cfg, state, runtime, logger)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.PauseXCapture(ctx); err != nil {
		t.Fatal(err)
	}
	pause, _ := state.SourceCapturePause(ctx)
	for _, body := range []string{`{}`, `{"confirmed":true,"pauseId":"stale"}`, `{"confirmed":false,"pauseId":"` + pause.ID + `"}`} {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/capture/x/confirm-recovery", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		server.api().ServeHTTP(response, request)
		if response.Code < 400 {
			t.Fatalf("invalid confirmation accepted: %s", body)
		}
		if p, _ := state.SourceCapturePause(ctx); p == nil {
			t.Fatal("pause cleared")
		}
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/auto-update/status", nil)
	server.api().ServeHTTP(response, request)
	var status struct {
		AutoUpdate domain.AutoUpdateStatus `json:"autoUpdate"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil || status.AutoUpdate.SourceCapturePause == nil {
		t.Fatalf("missing error projection: %s", response.Body.String())
	}
	response = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/capture/x/confirm-recovery", strings.NewReader(`{"confirmed":true,"pauseId":"`+pause.ID+`"}`))
	request.Header.Set("Content-Type", "application/json")
	server.api().ServeHTTP(response, request)
	if response.Code != 200 {
		t.Fatalf("recovery failed: %s", response.Body.String())
	}
	if p, _ := state.SourceCapturePause(ctx); p != nil {
		t.Fatal("confirmation did not clear pause")
	}
	if after, _ := state.GetSettings(ctx); after.AutoUpdateEnabled {
		t.Fatal("recovery silently enabled Auto Update")
	}
}

func TestCaptureBehaviorHTTPStatusKeepsChallengeOutsideTimedRecovery(t *testing.T) {
	ctx := context.Background()
	settings := domain.DefaultSettings("expanded", "quiet", "guarded_live", true)
	settings.ActiveSources = []domain.Source{domain.SourceX}
	settings.AutoUpdateEnabled = false
	state, err := store.Open(filepath.Join(t.TempDir(), "synthetic.db"), settings)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	logger := log.New(io.Discard, "", 0)
	cfg := config.Config{Capture: config.CaptureConfig{Behavior: config.CaptureBehaviorConfig{Enabled: true}}}
	runtime := engine.New(state, reasoning.Deterministic{}, cfg, logger)
	server, err := New(cfg, state, runtime, logger)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.PauseXCapture(ctx); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/auto-update/status", nil)
	server.api().ServeHTTP(response, request)
	var value struct {
		AutoUpdate domain.AutoUpdateStatus `json:"autoUpdate"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || len(value.AutoUpdate.CaptureBehavior) != 1 {
		t.Fatalf("missing status %s", response.Body.String())
	}
	status := value.AutoUpdate.CaptureBehavior[0]
	if !status.Enabled || status.Allowed || status.Reason != "challenge_required" || status.NotBefore != "" {
		t.Fatalf("challenge turned into a timer: %+v", status)
	}
	if value.AutoUpdate.Enabled {
		t.Fatal("status read enabled Auto Update")
	}
}
