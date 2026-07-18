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
	"strings"
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
	database := health["database"].(map[string]any)
	if database["status"] != "healthy" {
		t.Fatalf("database health=%+v", database)
	}
	if _, exposed := database["path"]; exposed {
		t.Fatalf("health must not expose the absolute database path: %+v", database)
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
	sources := bootstrap["sources"].([]any)
	if len(sources) != 3 || sources[2].(map[string]any)["id"] != "facebook" || sources[2].(map[string]any)["defaultActive"] != false {
		t.Fatalf("source descriptors=%+v", sources)
	}
	reasoningProcesses := bootstrap["reasoningProcesses"].([]any)
	if len(reasoningProcesses) != 4 || reasoningProcesses[3].(map[string]any)["id"] != "ai_deep_detection" || reasoningProcesses[3].(map[string]any)["model"] != "local-deterministic" {
		t.Fatalf("reasoning processes=%+v", reasoningProcesses)
	}
	reasoningRuntime := bootstrap["reasoningRuntime"].(map[string]any)
	if reasoningRuntime["provider"] != "deterministic" || reasoningRuntime["editable"] != false {
		t.Fatalf("reasoning runtime=%+v", reasoningRuntime)
	}
	bootstrapSettings := bootstrap["settings"].(map[string]any)
	if bootstrapSettings["timelineBoundaryCueMode"] != "follow" || bootstrapSettings["timelineBoundaryReturnMs"] != float64(350) || bootstrapSettings["semanticEventMergeThreshold"] != .92 || bootstrapSettings["aiDetectionPresentation"] != "drawer" {
		t.Fatalf("timeline boundary cue settings=%+v", bootstrapSettings)
	}
	if bootstrapSettings["reasoningAcquisitionProfile"] != "luna_high" || bootstrapSettings["reasoningEvaluationProfile"] != "luna_xhigh" || bootstrapSettings["reasoningSemanticProfile"] != "luna_high" || bootstrapSettings["reasoningAiDeepProfile"] != "luna_high" {
		t.Fatalf("reasoning defaults=%+v", bootstrapSettings)
	}
	onboarding := bootstrap["onboarding"].(map[string]any)
	if onboarding["status"] != "not_started" {
		t.Fatalf("fresh onboarding=%+v", onboarding)
	}
	for path, markers := range map[string][]string{
		"/":           {"Semantic event engine", "AI Detector", "Reasoning processes", "reasoning-processes", "reasoning-executable-path", "detect-reasoning-executable", "ai-detection-presentation", "timeline-side-pane", "semantic-event-shortlist", "semantic-event-merge-threshold", "reset-semantic-event-merge-threshold", "knowledge-retention-days", "knowledge-storage-limit", "timeline-boundary-follow", "timeline-boundary-return-ms"},
		"/app.js":     {"SOURCE_TEXT_COLLAPSE_CHARACTERS = 420", "function buildExpandableText", "function buildAttachments", "source-layout-attachments", "notice notice-complete", "timeline-history-boundary", "timeline-older-batch-marker", "syncBackToTopBoundaryPosition", "timelineBoundaryCueMode", "timelineBoundaryReturnMs", "DEFAULT_TIMELINE_BOUNDARY_RETURN_MS = 350", "DEFAULT_SEMANTIC_EVENT_MERGE_THRESHOLD = 0.92", "semanticEventMergeThreshold", "resetSemanticEventMergeThreshold", "is-following-boundary", "duplicate report", "function buildCollapsedDuplicate", "function showCorrectionNotice", "function buildMediaRecaptureButton", "function buildForegroundRecaptureOffer", "Try in foreground", "body: { captureMode }", "document.querySelectorAll(\".recapture-button\")", "AKU_BROWSER_MEDIA_RECAPTURE", "AKU_BROWSER_X_MEDIA_EVIDENCE_LOOKUP", "AKU_BROWSER_DISPATCH_FAILED", "authoritative Sidecar run outcome", "function enrichPassiveXMedia", "passive_x_cache", "/media-evidence", "\"not_interested\"", "Local fast path", "strongest overlap", "DEFAULT_TIMELINE_BATCH_GAP_PX = 36", "function buildInboxPreferenceDecisions", "The latest More or Less decision is authoritative.", "function buildInboxFlowInspector", "/api/inbox/runs/", "function buildInboxFlowItem", "Should have selected", "/selection-corrections", "Re-evaluate run", "Selected by you", "Semantic duplicate", "function routeAIDetectedItems", "function buildAIDetectionControls", "function buildSourceIcon", "timeline-source-icon-", "AI signal · Neutral", "Mark as not AI-generated", "Mark as AI-generated", "HIDE STRONG AI SIGNALS", "function timelineSidePaneAvailable", "function syncTimelineSidePaneVisibility", "state.timelineItems.length > 0", "function scheduleTimelineSidePanePosition", "--timeline-side-pane-top", "--timeline-side-pane-toggle-top", "ResizeObserver", "borderTopLeftRadius", "#result-items > [data-timeline-id]", "function detectReasoningExecutable", "/api/reasoning/runtime/discover", "reasoningExecutablePath", "function fetchFromSidecar", "AkuSidecar is offline or unreachable", "AkuSidecar offline", "sidecar_unavailable", "pollInFlight", "function describeSessionProgress", "AI Fast Detection", "AI Deep Detection continues asynchronously"},
		"/styles.css": {".notice-complete", ".expandable-text-copy.is-collapsed", ".content-expander", ".timeline-batch-marker", ".timeline-older-batch-marker", "--timeline-batch-gap", "--back-to-top-return-duration", ".semantic-duplicate-item", ".paired-setting-control", ".recapture-button", ".foreground-recapture-offer", ".inbox-preference-decision", ".inbox-flow-inspector", ".inbox-flow-filters", ".inbox-flow-item-actions", ".inbox-selection-correction-button", ".inbox-flow-outcome-user_selected", ".inbox-flow-outcome-collapsed_duplicate", ".ai-origin-badge", ".ai-origin-neutral", ".timeline-side-pane", ".timeline-source-icon-x { background: #050505; color: #fff; }"},
	} {
		response, err = client.Get("http://" + address.String() + path)
		if err != nil {
			t.Fatal(err)
		}
		payload, readErr := io.ReadAll(response.Body)
		response.Body.Close()
		if readErr != nil || response.StatusCode != http.StatusOK {
			t.Fatalf("asset %s status=%d err=%v", path, response.StatusCode, readErr)
		}
		for _, marker := range markers {
			if !strings.Contains(string(payload), marker) {
				t.Fatalf("asset %s missing %q", path, marker)
			}
		}
	}
	response, err = client.Get("http://" + address.String() + "/app.js")
	if err != nil {
		t.Fatal(err)
	}
	appPayload, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(appPayload), "entry.feedback?.direction") {
		t.Fatal("timeline feedback state is not restored after rendering")
	}
	if !strings.Contains(string(appPayload), "state.expandedTimelineText.has(expansionKey)") {
		t.Fatal("expanded Timeline text state is not restored after rendering")
	}
	if strings.Contains(string(appPayload), "Optional reason") || strings.Contains(string(appPayload), "already_knew") || strings.Contains(string(appPayload), "old_info") {
		t.Fatal("retired feedback reasons remain in the active UI")
	}
	if strings.Contains(string(appPayload), "Legacy run") || strings.Contains(string(appPayload), "trigger diagnostics unavailable") {
		t.Fatal("retired pre-trigger diagnostic UI remains in the active bundle")
	}
	if strings.Contains(string(appPayload), "function buildItemActionsMenu") || strings.Contains(string(appPayload), "AI origin correction") {
		t.Fatal("AI correction must stay with the toolbar badge instead of the footer actions menu")
	}
	for _, marker := range []string{"function reasoningProfileValue", "function syncTimelineSidePanePosition", "reasoningAcquisitionProfile", "reasoningAiDeepProfile"} {
		if !strings.Contains(string(appPayload), marker) {
			t.Fatalf("app.js missing runtime reasoning or drawer contract %q", marker)
		}
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
	response, err = client.Get("http://" + address.String() + "/api/inbox/runs/missing/trace?stage=captured")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("missing run trace status=%d", response.StatusCode)
	}
	response, err = client.Get("http://" + address.String() + "/api/inbox/runs/missing/trace?stage=raw")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid run trace stage status=%d", response.StatusCode)
	}
	correctionRequest, _ := http.NewRequest(http.MethodPost, "http://"+address.String()+"/api/inbox/runs/missing/selection-corrections", bytes.NewBufferString(`{"candidateRef":"candidate_missing"}`))
	correctionRequest.Header.Set("Content-Type", "application/json")
	response, err = client.Do(correctionRequest)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("missing selection correction candidate status=%d", response.StatusCode)
	}
	retryRequest, _ := http.NewRequest(http.MethodPost, "http://"+address.String()+"/api/inbox/runs/missing/re-evaluate", nil)
	response, err = client.Do(retryRequest)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("missing re-evaluation run status=%d", response.StatusCode)
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
	hideSettings := settings
	hideSettings.AIDetectionPresentation = "hide"
	badHidePayload, _ := json.Marshal(map[string]any{"settings": hideSettings, "confirmationPhrase": "wrong"})
	request, _ = http.NewRequest(http.MethodPut, "http://"+address.String()+"/api/settings", bytes.NewReader(badHidePayload))
	request.Header.Set("Content-Type", "application/json")
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("wrong AI Hide confirmation status=%d", response.StatusCode)
	}
	goodHidePayload, _ := json.Marshal(map[string]any{"settings": hideSettings, "confirmationPhrase": domain.AIHideConfirmationPhrase})
	request, _ = http.NewRequest(http.MethodPut, "http://"+address.String()+"/api/settings", bytes.NewReader(goodHidePayload))
	request.Header.Set("Content-Type", "application/json")
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("confirmed AI Hide status=%d", response.StatusCode)
	}
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

func TestLoopbackBoundaryRejectsForeignHostsAndOrigins(t *testing.T) {
	settings := domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", true)
	state, err := store.Open(filepath.Join(t.TempDir(), "sidecar.db"), settings)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	cfg := config.Config{Server: config.ServerConfig{Host: "127.0.0.1", Port: 0}}
	runtime := engine.New(state, reasoning.Deterministic{}, cfg, log.New(io.Discard, "", 0))
	server, err := New(cfg, state, runtime, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name        string
		method      string
		path        string
		host        string
		origin      string
		contentType string
		want        int
	}{
		{name: "foreign host", method: http.MethodGet, path: "/api/health", host: "attacker.example", want: http.StatusForbidden},
		{name: "foreign browser origin", method: http.MethodPut, path: "/api/onboarding", host: "127.0.0.1:11122", origin: "https://attacker.example", contentType: "application/json", want: http.StatusForbidden},
		{name: "extension cannot call UI mutation", method: http.MethodPut, path: "/api/onboarding", host: "127.0.0.1:11122", origin: "chrome-extension://abcdefghijklmnop", contentType: "application/json", want: http.StatusForbidden},
		{name: "same origin UI reaches route", method: http.MethodPut, path: "/api/onboarding", host: "127.0.0.1:11122", origin: "http://127.0.0.1:11122", contentType: "application/json", want: http.StatusOK},
		{name: "extension reaches bridge authentication", method: http.MethodPost, path: "/api/bridge/heartbeat", host: "localhost:11122", origin: "chrome-extension://abcdefghijklmnop", contentType: "application/json", want: http.StatusUnauthorized},
		{name: "JSON mutation rejects text content", method: http.MethodPut, path: "/api/onboarding", host: "127.0.0.1:11122", origin: "http://127.0.0.1:11122", contentType: "text/plain", want: http.StatusUnsupportedMediaType},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := ""
			if test.method == http.MethodPut {
				body = `{"activeSources":["x","linkedin"]}`
			} else if test.method == http.MethodPost {
				body = `{}`
			}
			request := httptest.NewRequest(test.method, "http://"+test.host+test.path, strings.NewReader(body))
			request.Host = test.host
			if test.origin != "" {
				request.Header.Set("Origin", test.origin)
			}
			if test.contentType != "" {
				request.Header.Set("Content-Type", test.contentType)
			}
			response := httptest.NewRecorder()
			server.http.Handler.ServeHTTP(response, request)
			if response.Code != test.want {
				t.Fatalf("status=%d want=%d body=%s", response.Code, test.want, response.Body.String())
			}
		})
	}
}

func TestStopClosesActiveHTTPConnectionsAfterDrainDeadline(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	fixture := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		close(entered)
		<-release
	}))
	fixture.Start()
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
		fixture.Close()
	})
	requestDone := make(chan struct{})
	go func() {
		defer close(requestDone)
		_, _ = fixture.Client().Get(fixture.URL)
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("fixture request did not become active")
	}
	server := &Server{http: fixture.Config, listener: fixture.Listener}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	started := time.Now()
	if err := server.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("HTTP stop exceeded bounded fallback: %s", elapsed)
	}
	close(release)
	select {
	case <-requestDone:
	case <-time.After(time.Second):
		t.Fatal("active request did not leave after connection close")
	}
}

func TestBridgeV51ObservationShapeDecodesStrictly(t *testing.T) {
	raw := `{
		"source":"x","pageUrl":"https://x.com/home","pageTitle":"Home","capturedAt":"2026-07-15T00:00:00Z",
		"snapshots":[{
			"index":0,"adapterVersion":"x-dom-v19","selectorStrategy":"article","selectorCounts":{"article":1},
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
	request.Header.Set("Content-Type", "application/json")
	var observation domain.Observation
	if err := readJSON(request, &observation); err != nil {
		t.Fatalf("v51 observation must satisfy the strict Go shape: %v", err)
	}
	if observation.Snapshots[0].Blocks[0].PlatformID != "1" {
		t.Fatalf("observation=%+v", observation)
	}
}

func TestPassiveMediaEvidenceEndpointRequiresBridgeAuthentication(t *testing.T) {
	settings := domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", true)
	state, err := store.Open(filepath.Join(t.TempDir(), "sidecar.db"), settings)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	cfg := config.Config{Server: config.ServerConfig{Host: "127.0.0.1", Port: 0}}
	logger := log.New(io.Discard, "", 0)
	runtime := engine.New(state, reasoning.Deterministic{}, cfg, logger)
	server, err := New(cfg, state, runtime, logger)
	if err != nil {
		t.Fatal(err)
	}
	body := `{"candidateId":"x:status:12345","media":[{"kind":"image","url":"https://pbs.twimg.com/media/example.jpg"}],"provenance":"passive_x_cache"}`

	request := httptest.NewRequest(http.MethodPost, "/api/bridge/timeline/missing/media-evidence", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.api().ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status=%d body=%s", response.Code, response.Body.String())
	}

	token, err := state.BridgeToken(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	request = httptest.NewRequest(http.MethodPost, "/api/bridge/timeline/missing/media-evidence", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Aku-Bridge-Token", token)
	request.Header.Set("X-Aku-Bridge-Contract", domain.BridgeContractVersion)
	request.Header.Set("X-Aku-Bridge-Id", "http-passive-test")
	response = httptest.NewRecorder()
	server.api().ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("authenticated status=%d body=%s", response.Code, response.Body.String())
	}

	textBearingBody := `{"candidateId":"x:status:12345","media":[{"kind":"image","url":"https://pbs.twimg.com/media/example.jpg","alt":"post text must not cross this contract"}],"provenance":"passive_x_cache"}`
	request = httptest.NewRequest(http.MethodPost, "/api/bridge/timeline/missing/media-evidence", strings.NewReader(textBearingBody))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Aku-Bridge-Token", token)
	request.Header.Set("X-Aku-Bridge-Contract", domain.BridgeContractVersion)
	response = httptest.NewRecorder()
	server.api().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("text-bearing media status=%d body=%s", response.Code, response.Body.String())
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
			permalink := "https://x.com/example/status/" + platformID
			if source == domain.SourceLinkedIn {
				permalink = "https://www.linkedin.com/feed/update/urn:li:activity:" + platformID
			}
			observation := domain.Observation{Source: source, PageURL: "https://example.test", CapturedAt: domain.Now(), Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{PlatformID: platformID, Text: "Material source update", Author: "author", Permalink: permalink, FeedPosition: 1}}}}, Coverage: map[string]any{"quality": "complete"}}
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
