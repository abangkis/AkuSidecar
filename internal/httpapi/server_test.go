package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
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
)

func TestHealthAndBootstrapExposeGoBoundary(t *testing.T) {
	settings := domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", true)
	state, err := store.Open(filepath.Join(t.TempDir(), "sidecar.db"), settings)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	cfg := config.Config{Server: config.ServerConfig{Host: "127.0.0.1", Port: 0}, Capture: config.CaptureConfig{Profile: "expanded", Visibility: "quiet", OpenMissingSource: true, MaxAcquisitionRounds: 2}, Preference: config.PreferenceConfig{Mode: "promote_unused_budget"}}
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
	client := http.Client{Timeout: 2 * time.Second}
	response, err := client.Get("http://" + address.String() + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var health map[string]any
	if err := json.NewDecoder(response.Body).Decode(&health); err != nil {
		t.Fatal(err)
	}
	if health["runtime"] != "go" || health["bridgeContractVersion"] != domain.BridgeContractVersion {
		t.Fatalf("health=%+v", health)
	}
	if response.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("security headers missing")
	}
	response, err = client.Get("http://" + address.String() + "/api/bootstrap")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var bootstrap map[string]any
	if err := json.NewDecoder(response.Body).Decode(&bootstrap); err != nil {
		t.Fatal(err)
	}
	if bootstrap["bridgeToken"] == "" || bootstrap["provider"] != "deterministic" {
		t.Fatalf("bootstrap=%+v", bootstrap)
	}
	onboarding := bootstrap["onboarding"].(map[string]any)
	if onboarding["status"] != "not_started" {
		t.Fatalf("fresh onboarding=%+v", onboarding)
	}
	request, err := http.NewRequest(http.MethodPut, "http://"+address.String()+"/api/onboarding", bytes.NewBufferString(`{"activeSources":["x","linkedin"]}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("onboarding status=%d", response.StatusCode)
	}
	heartbeat, _ := json.Marshal(map[string]any{"capabilities": engine.ExpectedHeartbeat()})
	request, err = http.NewRequest(http.MethodPost, "http://"+address.String()+"/api/bridge/heartbeat", bytes.NewReader(heartbeat))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Aku-Bridge-Token", bootstrap["bridgeToken"].(string))
	request.Header.Set("X-Aku-Bridge-Id", "http-test")
	request.Header.Set("X-Aku-Bridge-Contract", domain.BridgeContractVersion)
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("heartbeat status=%d", response.StatusCode)
	}
	response.Body.Close()
	request, _ = http.NewRequest(http.MethodPost, "http://"+address.String()+"/api/operations/full-reset", bytes.NewBufferString(`{"confirmation":"wrong"}`))
	request.Header.Set("Content-Type", "application/json")
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("wrong reset confirmation status=%d", response.StatusCode)
	}
	request, _ = http.NewRequest(http.MethodPost, "http://"+address.String()+"/api/operations/full-reset", bytes.NewBufferString(`{"confirmation":"RESET AKUBROWSER"}`))
	request.Header.Set("Content-Type", "application/json")
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("full reset status=%d", response.StatusCode)
	}
	var reset map[string]any
	if err := json.NewDecoder(response.Body).Decode(&reset); err != nil {
		t.Fatal(err)
	}
	if reset["operation"] != "full_reset" || reset["onboarding"].(map[string]any)["status"] != "not_started" {
		t.Fatalf("reset=%+v", reset)
	}
}

func TestBridgeV47ObservationShapeDecodesStrictly(t *testing.T) {
	raw := `{
		"source":"x","pageUrl":"https://x.com/home","pageTitle":"Home","capturedAt":"2026-07-15T00:00:00Z",
		"snapshots":[{
			"index":0,"adapterVersion":"x-dom-v16","selectorStrategy":"article","selectorCounts":{"article":1},
			"selectorCandidateCount":1,"visibleContainerCount":1,"capturedAt":"2026-07-15T00:00:00Z",
			"scrollY":0,"viewportHeight":900,"newCandidateCount":1,
			"blocks":[{
				"text":"Material source update","author":"author","avatarUrl":null,"publishedAt":null,
				"permalink":"https://x.com/author/status/1","platformId":"1","contentKind":"post",
				"relationshipType":"original","parentPermalink":null,"quotedPost":null,"engagement":{},
				"presentation":{},"media":[],"links":[],"mediaRecovery":{},"captureQuality":{},"feedPosition":1
			}],"qualityReports":[]
		}],"coverage":{"browserAdapter":"aku-bridge"}
	}`
	request := httptest.NewRequest(http.MethodPost, "/api/bridge/commands/example/observation", bytes.NewBufferString(raw))
	var observation domain.Observation
	if err := readJSON(request, &observation); err != nil {
		t.Fatalf("v47 observation must satisfy the strict Go shape: %v", err)
	}
	if observation.Snapshots[0].Blocks[0].PlatformID != "1" {
		t.Fatalf("observation=%+v", observation)
	}
}
