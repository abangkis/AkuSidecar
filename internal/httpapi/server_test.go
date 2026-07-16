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
	response, err = client.Get("http://" + address.String() + "/api/inbox?limit=5&offset=0")
	if err != nil {
		t.Fatal(err)
	}
	var inbox struct {
		Sessions []domain.InboxSession `json:"sessions"`
		Total    int                   `json:"total"`
		Limit    int                   `json:"limit"`
	}
	if err := json.NewDecoder(response.Body).Decode(&inbox); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || inbox.Total != 0 || inbox.Limit != 5 || len(inbox.Sessions) != 0 {
		t.Fatalf("inbox=%+v status=%d", inbox, response.StatusCode)
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
	var onboardingResponse map[string]any
	if err := json.NewDecoder(response.Body).Decode(&onboardingResponse); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("onboarding status=%d", response.StatusCode)
	}
	calibration := onboardingResponse["calibration"].(map[string]any)
	if calibration["firstRunStatus"] != "pending" || calibration["enabled"] != true || calibration["batchSize"] != float64(10) {
		t.Fatalf("onboarding calibration=%+v", calibration)
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

func TestFirstRunHTTPFlowEndsInForcedCalibration(t *testing.T) {
	settings := domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", true)
	state, err := store.Open(filepath.Join(t.TempDir(), "sidecar.db"), settings)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	cfg := config.Config{Server: config.ServerConfig{Host: "127.0.0.1", Port: 0}, Capture: config.CaptureConfig{Profile: "expanded", Visibility: "quiet", OpenMissingSource: true, MaxAcquisitionRounds: 1}, Preference: config.PreferenceConfig{Mode: "promote_unused_budget"}}
	runtime := engine.New(state, reasoning.Deterministic{}, cfg, log.New(io.Discard, "", 0))
	runtime.RecordHeartbeat(engine.ExpectedHeartbeat())
	server, err := New(cfg, state, runtime, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	address, err := server.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer server.Stop(context.Background())
	origin := "http://" + address.String()
	client := http.Client{Timeout: 2 * time.Second}

	var onboarding map[string]any
	requestJSON(t, client, http.MethodPut, origin+"/api/onboarding", `{"activeSources":["x","linkedin"]}`, &onboarding)
	if onboarding["calibration"].(map[string]any)["firstRunStatus"] != "pending" {
		t.Fatalf("onboarding=%+v", onboarding)
	}
	var started struct {
		Session domain.Session `json:"session"`
	}
	requestJSON(t, client, http.MethodPost, origin+"/api/sessions", `{"intent":"What changed?"}`, &started)
	completeHTTPTestRun(t, runtime, started.Session.ID, domain.SourceX, "x-http-1")
	completeHTTPTestRun(t, runtime, started.Session.ID, domain.SourceLinkedIn, "linkedin-http-1")
	waitHTTPTestSession(t, runtime, started.Session.ID, "completed")

	var bootstrapWithCalibration struct {
		Calibration domain.CalibrationOverview `json:"calibration"`
	}
	requestJSON(t, client, http.MethodGet, origin+"/api/bootstrap", "", &bootstrapWithCalibration)
	if bootstrapWithCalibration.Calibration.Active == nil || bootstrapWithCalibration.Calibration.Active.Status != "reviewing" || bootstrapWithCalibration.Calibration.Active.SampleCount != 2 {
		t.Fatalf("automatic calibration=%+v", bootstrapWithCalibration.Calibration)
	}
	created := *bootstrapWithCalibration.Calibration.Active
	var decided struct {
		Calibration domain.CalibrationSession `json:"calibration"`
	}
	requestJSON(t, client, http.MethodPut, origin+"/api/calibration/sessions/"+created.ID+"/samples/0", `{"label":"more_like_this"}`, &decided)
	requestJSON(t, client, http.MethodPut, origin+"/api/calibration/sessions/"+created.ID+"/samples/1", `{"label":"neutral"}`, &decided)
	if decided.Calibration.Status != "completed" || decided.Calibration.Snapshot == nil {
		t.Fatalf("completed calibration=%+v", decided.Calibration)
	}

	var bootstrap map[string]any
	requestJSON(t, client, http.MethodGet, origin+"/api/bootstrap", "", &bootstrap)
	calibration := bootstrap["calibration"].(map[string]any)
	if calibration["firstRunStatus"] != "completed" || calibration["active"] != nil {
		t.Fatalf("bootstrap calibration=%+v", calibration)
	}
}

func requestJSON(t *testing.T, client http.Client, method, url, body string, target any) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = bytes.NewBufferString(body)
	}
	request, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("%s %s status=%d body=%s", method, url, response.StatusCode, payload)
	}
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		t.Fatal(err)
	}
}

func completeHTTPTestRun(t *testing.T, runtime *engine.Engine, sessionID string, source domain.Source, platformID string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		session, err := runtime.Session(context.Background(), sessionID)
		if err != nil {
			t.Fatal(err)
		}
		for _, run := range session.Runs {
			if run.Source != source || run.Status != "waiting_for_bridge" {
				continue
			}
			command, err := runtime.ClaimCommand(context.Background(), run.ID, "http-flow-test")
			if err != nil || command == nil {
				t.Fatalf("claim command=%+v err=%v", command, err)
			}
			observation := domain.Observation{Source: source, PageURL: "https://example.test", CapturedAt: domain.Now(), Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{PlatformID: platformID, Text: "Material source update", Author: "author", Permalink: "https://example.test/post", FeedPosition: 1}}}}, Coverage: map[string]any{"quality": "complete"}}
			if _, err := runtime.AcceptObservation(context.Background(), command.ID, run.ID, observation); err != nil {
				t.Fatal(err)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("%s run did not become ready", source)
}

func waitHTTPTestSession(t *testing.T, runtime *engine.Engine, sessionID, status string) domain.Session {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		session, err := runtime.Session(context.Background(), sessionID)
		if err != nil {
			t.Fatal(err)
		}
		if session.Status == status {
			return session
		}
		time.Sleep(20 * time.Millisecond)
	}
	session, _ := runtime.Session(context.Background(), sessionID)
	t.Fatalf("session status=%s, wanted %s", session.Status, status)
	return domain.Session{}
}
