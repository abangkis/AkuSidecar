package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/engine"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/AkuSidecar/internal/store"
)

func TestSplitReaderForegroundRequiresAuthenticatedClaimedExplicitIntent(t *testing.T) {
	s, token := splitTestServer(t)
	prepared, foregrounded := 0, 0
	s.SetSplitReaderPreparation(func(_ context.Context, marker string) (func(context.Context) error, error) {
		prepared++
		if marker != "AkuBrowser reader split_reader" {
			t.Fatalf("marker=%q", marker)
		}
		return func(context.Context) error { foregrounded++; return nil }, nil
	})
	entry := &pendingSplitAction{action: splitCaptureAction{ID: "split_reader", Type: "open_native_post"}, claimed: true}
	s.splitCapture.actions = []*pendingSplitAction{entry}
	prepare := "/api/bridge/split-capture/reader/prepare/split_reader"
	foreground := "/api/bridge/split-capture/reader/foreground/split_reader"
	for _, credentials := range []struct{ token, key string }{{token, "stale"}, {"wrong", s.splitCapture.key}, {token, ""}} {
		if w := splitRequest(s, credentials.token, credentials.key, "POST", prepare, "{}"); w.Code < 400 {
			t.Fatal("unauthenticated preparation accepted")
		}
	}
	if prepared != 0 {
		t.Fatal("authentication failure reached native code")
	}
	for _, action := range []string{"dispatch", "open_source", "ping"} {
		entry.action.Type = action
		if w := splitRequest(s, token, s.splitCapture.key, "POST", prepare, "{}"); w.Code != 404 {
			t.Fatalf("background action %s=%d", action, w.Code)
		}
	}
	entry.action.Type = "open_native_post"
	entry.claimed = false
	if w := splitRequest(s, token, s.splitCapture.key, "POST", prepare, "{}"); w.Code != 404 {
		t.Fatal("unclaimed reader accepted")
	}
	entry.claimed = true
	if w := splitRequest(s, token, s.splitCapture.key, "POST", foreground, "{}"); w.Code != 409 {
		t.Fatal("foreground before binding accepted")
	}
	if w := splitRequest(s, token, s.splitCapture.key, "POST", prepare, "{}"); w.Code != 200 {
		t.Fatalf("prepare=%d %s", w.Code, w.Body)
	}
	if w := splitRequest(s, token, s.splitCapture.key, "POST", prepare, "{}"); w.Code != 409 {
		t.Fatal("duplicate preparation accepted")
	}
	if w := splitRequest(s, token, s.splitCapture.key, "POST", foreground, "{}"); w.Code != 200 {
		t.Fatalf("foreground=%d %s", w.Code, w.Body)
	}
	if w := splitRequest(s, token, s.splitCapture.key, "POST", foreground, "{}"); w.Code != 409 {
		t.Fatal("foreground replay accepted")
	}
	if prepared != 1 || foregrounded != 1 {
		t.Fatalf("prepared=%d foregrounded=%d", prepared, foregrounded)
	}
	s.splitCapture.actions = nil
	if w := splitRequest(s, token, s.splitCapture.key, "POST", foreground, "{}"); w.Code != 404 {
		t.Fatal("expired action accepted")
	}
}

func TestSplitReaderForegroundFailureIsVisibleAndConsumed(t *testing.T) {
	s, token := splitTestServer(t)
	s.SetSplitReaderPreparation(func(context.Context, string) (func(context.Context) error, error) { return nil, nil })
	calls := 0
	s.splitCapture.actions = []*pendingSplitAction{{action: splitCaptureAction{ID: "split_reader", Type: "open_native_post"}, claimed: true, readerForeground: func(context.Context) error { calls++; return errors.New("Windows refused foreground") }}}
	path := "/api/bridge/split-capture/reader/foreground/split_reader"
	w := splitRequest(s, token, s.splitCapture.key, "POST", path, "{}")
	if w.Code != 409 || !strings.Contains(w.Body.String(), "Windows refused foreground") {
		t.Fatalf("failure=%d %s", w.Code, w.Body)
	}
	if w := splitRequest(s, token, s.splitCapture.key, "POST", path, "{}"); w.Code != 409 || calls != 1 {
		t.Fatal("failed foreground was replayed")
	}
}

func TestSplitReaderMarkerPageRequiresActiveClaimAndDoesNotExposeSecrets(t *testing.T) {
	s, _ := splitTestServer(t)
	path := "http://127.0.0.1:11122/split-reader-intent?id=split_reader"
	w := httptest.NewRecorder()
	s.serveSplitCaptureAsset(w, httptest.NewRequest("GET", path, nil))
	if w.Code != 404 {
		t.Fatal("marker available without action")
	}
	s.splitCapture.actions = []*pendingSplitAction{{action: splitCaptureAction{ID: "split_reader", Type: "open_native_post"}, claimed: true}}
	w = httptest.NewRecorder()
	s.serveSplitCaptureAsset(w, httptest.NewRequest("GET", path, nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "<title>AkuBrowser reader split_reader</title>") || strings.Contains(w.Body.String(), s.splitCapture.key) {
		t.Fatalf("marker page=%s", w.Body)
	}
}

func TestSplitReaderResultRequiresThisActionsSuccessfulForeground(t *testing.T) {
	for _, phase := range []string{"missing", "failed", "succeeded", "other_action"} {
		t.Run(phase, func(t *testing.T) {
			s, token := splitTestServer(t)
			var logs strings.Builder
			s.logger = log.New(&logs, "", 0)
			s.SetSplitReaderPreparation(func(context.Context, string) (func(context.Context) error, error) { return nil, nil })
			entry := &pendingSplitAction{action: splitCaptureAction{ID: "split_reader", Type: "open_native_post"}, claimed: true, result: make(chan splitActionResult, 1)}
			other := &pendingSplitAction{action: splitCaptureAction{ID: "split_other", Type: "open_native_post"}, claimed: true}
			s.splitCapture.actions = []*pendingSplitAction{entry, other}
			if phase != "missing" {
				target := entry
				if phase == "other_action" {
					target = other
				}
				target.readerForeground = func(context.Context) error {
					if phase == "failed" {
						return errors.New("native readback failed")
					}
					return nil
				}
				w := splitRequest(s, token, s.splitCapture.key, "POST", "/api/bridge/split-capture/reader/foreground/"+target.action.ID, "{}")
				if (w.Code == 200) != (phase != "failed") {
					t.Fatalf("foreground=%d %s", w.Code, w.Body)
				}
			}
			w := splitRequest(s, token, s.splitCapture.key, "POST", "/api/bridge/split-capture/results/split_reader", `{"ok":true,"result":{"windowId":2}}`)
			if w.Code != 204 {
				t.Fatalf("result=%d %s", w.Code, w.Body)
			}
			select {
			case result := <-entry.result:
				if result.OK != (phase == "succeeded") {
					t.Fatalf("result=%+v", result)
				}
				if !result.OK && (!strings.Contains(result.Message, "Reload AkuBridge") || result.Result != nil || !strings.Contains(logs.String(), "phase=result outcome=rejected")) {
					t.Fatalf("failure not actionable or logged: %+v logs=%s", result, logs.String())
				}
			default:
				t.Fatal("waiting UI did not receive terminal result immediately")
			}
		})
	}
}

func splitTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	state, err := store.Open(filepath.Join(t.TempDir(), "split.db"), domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", true))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = state.Close() })
	cfg := config.Config{Server: config.ServerConfig{Host: "127.0.0.1", Port: 11122}, ExperimentalWindowsCaptureSplit: true}
	logger := log.New(io.Discard, "", 0)
	e := engine.New(state, reasoning.Deterministic{}, cfg, logger)
	s, err := New(cfg, state, e, logger)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if s.splitCapture != nil {
			t.Fatal("split activated outside Windows")
		}
		s.splitCapture = newSplitCaptureTransport() // Test the transport independent of launch gate.
	}
	t.Cleanup(s.splitCapture.close)
	token, err := state.BridgeToken(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return s, token
}
func splitRequest(s *Server, token, key, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://127.0.0.1:11122"+path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Aku-Bridge-Contract", domain.BridgeContractVersion)
	r.Header.Set("X-Aku-Bridge-Token", token)
	r.Header.Set("X-Aku-Capture-Instance", key)
	r.Header.Set("X-Aku-Split-Epoch", s.engine.Epoch())
	w := httptest.NewRecorder()
	s.api().ServeHTTP(w, r)
	return w
}

func TestSplitCaptureFencesWorkersAndUIHeartbeat(t *testing.T) {
	s, token := splitTestServer(t)
	if w := splitRequest(s, "", "stale", "POST", "/api/bridge/split-capture/bootstrap", "{}"); w.Code != 403 {
		t.Fatalf("bootstrap: %d", w.Code)
	}
	if w := splitRequest(s, "", s.splitCapture.key, "POST", "/api/bridge/split-capture/bootstrap", "{}"); w.Code != 200 {
		t.Fatalf("bootstrap: %d %s", w.Code, w.Body)
	}
	for _, route := range []string{"/api/bridge/commands/next?runId=x", "/api/bridge/commands/pending", "/api/bridge/split-capture/next"} {
		if w := splitRequest(s, token, "stale", "GET", route, ""); w.Code != 409 {
			t.Fatalf("unfenced %s: %d", route, w.Code)
		}
	}
	if w := splitRequest(s, token, "", "POST", "/api/bridge/heartbeat", `{"capabilities":{"bridgeId":"forged-ui"}}`); w.Code != 202 {
		t.Fatal(w.Code)
	}
	if s.engine.BridgeStatus().Actual != nil {
		t.Fatal("UI manufactured capture readiness")
	}
	if w := splitRequest(s, token, s.splitCapture.key, "POST", "/api/bridge/heartbeat", `{"capabilities":{"bridgeId":"capture"}}`); w.Code != 202 {
		t.Fatal(w.Code)
	}
	if s.engine.BridgeStatus().Actual.BridgeID != "capture" {
		t.Fatal("capture heartbeat missing")
	}
}

func TestSplitCaptureFenceLeavesUIRelayRoutesAvailable(t *testing.T) {
	for _, route := range []string{
		"/api/operations/bridge/actions/next",
		"/api/operations/bridge/actions/reload-self",
		"/api/operations/bridge/actions/action-1",
		"/api/bridge/timeline/timeline-1/media-evidence",
	} {
		r := httptest.NewRequest("GET", "http://127.0.0.1:11122"+route, nil)
		if splitCaptureOwnedRoute(r) {
			t.Fatalf("UI relay route was capture-fenced: %s", route)
		}
	}
	for _, route := range []string{
		"/api/bridge/commands/next",
		"/api/bridge/media-recaptures/recapture-1/result",
		"/api/bridge/capture-surfaces/events",
		"/api/bridge/split-capture/next",
		"/api/operations/bridge/actions/action-1/accept",
	} {
		r := httptest.NewRequest("GET", "http://127.0.0.1:11122"+route, nil)
		if !splitCaptureOwnedRoute(r) {
			t.Fatalf("capture worker route was not fenced: %s", route)
		}
	}
}

func TestSplitCaptureActionRoundTripAndBoundedQueue(t *testing.T) {
	s, token := splitTestServer(t)
	ui := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		ui <- splitRequest(s, token, "", "POST", "/api/split-capture/actions", `{"type":"probe_source_sessions"}`)
	}()
	w := splitRequest(s, token, s.splitCapture.key, "GET", "/api/bridge/split-capture/next", "")
	if w.Code != 200 {
		t.Fatalf("claim: %d %s", w.Code, w.Body)
	}
	var claim struct {
		Action splitCaptureAction `json:"action"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &claim); err != nil {
		t.Fatal(err)
	}
	if claim.Action.Type != "probe_source_sessions" || claim.Action.ID == "" {
		t.Fatal(claim)
	}
	resultPath := "/api/bridge/split-capture/results/" + claim.Action.ID
	if w := splitRequest(s, token, "stale", "POST", resultPath, `{"ok":true}`); w.Code != 409 {
		t.Fatalf("stale result %d", w.Code)
	}
	if w := splitRequest(s, token, s.splitCapture.key, "POST", resultPath, `{"ok":true,"result":{"sessions":{}}}`); w.Code != 204 {
		t.Fatalf("result %d %s", w.Code, w.Body)
	}
	select {
	case w := <-ui:
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"sessions"`) {
			t.Fatal(w.Body)
		}
	case <-time.After(time.Second):
		t.Fatal("UI reply missing")
	}
	if w := splitRequest(s, token, s.splitCapture.key, "POST", resultPath, `{"ok":true}`); w.Code != 404 {
		t.Fatalf("completed action replay %d", w.Code)
	}
	if w := splitRequest(s, token, "", "POST", "/api/split-capture/actions", `{"type":"eval","url":"javascript:alert(1)"}`); w.Code != 400 {
		t.Fatalf("unknown operation %d", w.Code)
	}
	s.splitCapture.mu.Lock()
	s.splitCapture.actions = make([]*pendingSplitAction, splitActionLimit)
	s.splitCapture.mu.Unlock()
	if w := splitRequest(s, token, "", "POST", "/api/split-capture/actions", `{"type":"ping"}`); w.Code != 503 {
		t.Fatalf("unbounded queue %d", w.Code)
	}
}

func TestSplitAssetsAreAbsentByDefault(t *testing.T) {
	s := &Server{}
	r := httptest.NewRequest("GET", "http://127.0.0.1:11122/", nil)
	if s.serveSplitCaptureAsset(httptest.NewRecorder(), r) {
		t.Fatal("default UI intercepted")
	}
	s.splitCapture = newSplitCaptureTransport()
	w := httptest.NewRecorder()
	if !s.serveSplitCaptureAsset(w, r) || !strings.Contains(w.Body.String(), "/split-ui-bridge.js") {
		t.Fatal("split UI adapter absent")
	}
	if strings.Contains(w.Body.String(), s.splitCapture.key) {
		t.Fatal("capture capability leaked into UI")
	}
}
