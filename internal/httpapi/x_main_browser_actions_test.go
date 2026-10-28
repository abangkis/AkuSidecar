//go:build windows && auth_session_dev

package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/collection"
	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/engine"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/AkuSidecar/internal/store"
)

func TestXMainBrowserActionsOpenSourceUsesBridgeQueueWithoutDailyConsent(t *testing.T) {
	s, token, manager, _ := xMainBrowserActionFixture(t)
	if !s.engine.XUsesOrdinaryBrowserActions() {
		t.Fatal("main-profile concurrent X should use ordinary Browser actions")
	}
	settingsBefore, scheduleBefore, usageBefore, receiptsBefore := xMainActionState(t, s)
	bridgeBefore := s.engine.BridgeStatus().Actual
	status := s.engine.XCaptureStatus()
	if status.DerivationAuthorized || status.EffectiveMode == config.XCaptureConcurrent {
		t.Fatalf("ordinary browser action acquired daily derivation authority: %+v", status)
	}

	call := startXMainAction(t, s, token, `{"type":"open_source","source":"x"}`, time.Second)
	waitXMainQueued(t, s, "")
	claimed := splitRequest(s, token, s.splitCapture.key, http.MethodGet, "/api/bridge/split-capture/next", "")
	if claimed.Code != http.StatusOK {
		t.Fatalf("claim status=%d body=%s", claimed.Code, claimed.Body)
	}
	var envelope struct {
		Action splitCaptureAction `json:"action"`
	}
	if err := json.Unmarshal(claimed.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Action.Type != "open_source" || envelope.Action.Source != "x" {
		t.Fatalf("wrong ordinary Bridge action: %+v", envelope.Action)
	}
	result := splitRequest(s, token, s.splitCapture.key, http.MethodPost, "/api/bridge/split-capture/results/"+envelope.Action.ID, `{"ok":true,"result":{"state":"opened"}}`)
	if result.Code != http.StatusNoContent {
		t.Fatalf("result status=%d body=%s", result.Code, result.Body)
	}
	response := awaitXMainAction(t, call)
	if response.Code != http.StatusOK || !response.Result.OK {
		t.Fatalf("open_source status=%d result=%+v body=%s", response.Code, response.Result, response.Body)
	}
	if got := manager.Snapshot().ActiveLeases; got != 0 {
		t.Fatalf("open_source leaked %d Browser leases", got)
	}
	xMainActionStateUnchanged(t, s, settingsBefore, scheduleBefore, usageBefore, receiptsBefore, bridgeBefore)
}

func TestXMainBrowserActionsNativePostUsesMatchedBrokerAndForeground(t *testing.T) {
	for _, tc := range []struct {
		name           string
		reply          readerbroker.Reply
		wantForeground int
		wantSuccess    bool
	}{
		{name: "verified foreground", reply: readerbroker.Reply{OK: true, Readback: true}, wantForeground: http.StatusOK, wantSuccess: true},
		{name: "rejected foreground", reply: readerbroker.Reply{}, wantForeground: http.StatusConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, token, manager, _ := xMainBrowserActionFixture(t)
			request := readerbroker.Request{
				RequestID: "broker_" + strings.Repeat("a", 32),
				Source:    "x",
				URL:       "https://x.com/synthetic/status/1",
			}
			verified := 0
			s.SetSplitReaderBroker(func(context.Context, string) (readerbroker.Target, func(context.Context) error, error) {
				return readerbroker.Target{HWND: 707, PID: 808, Value: 1, Action: "synthetic-reader", Expires: time.Now().Add(3 * time.Second)}, func(context.Context) error {
					verified++
					return nil
				}, nil
			})
			settingsBefore, scheduleBefore, usageBefore, receiptsBefore := xMainActionState(t, s)
			bridgeBefore := s.engine.BridgeStatus().Actual
			body := fmt.Sprintf(`{"type":"open_native_post","source":"x","url":%q,"requestId":%q}`, request.URL, request.RequestID)
			call := startXMainAction(t, s, token, body, 3*time.Second)
			waitXMainQueued(t, s, request.RequestID)

			brokerCtx, brokerCancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer brokerCancel()
			brokerDone := make(chan error, 1)
			go func() {
				brokerDone <- s.HandleReaderBroker(brokerCtx, request, func(target readerbroker.Target) (readerbroker.Reply, error) {
					if target.HWND != 707 || target.PID != 808 {
						return readerbroker.Reply{}, errors.New("synthetic reader target mismatch")
					}
					return tc.reply, nil
				})
			}()

			claimed := splitRequest(s, token, s.splitCapture.key, http.MethodGet, "/api/bridge/split-capture/next", "")
			if claimed.Code != http.StatusOK {
				t.Fatalf("claim status=%d body=%s", claimed.Code, claimed.Body)
			}
			var envelope struct {
				Action splitCaptureAction `json:"action"`
			}
			if err := json.Unmarshal(claimed.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if envelope.Action.Type != "open_native_post" || envelope.Action.Source != "x" || envelope.Action.RequestID != request.RequestID {
				t.Fatalf("wrong ordinary Bridge reader action: %+v", envelope.Action)
			}
			prepared := splitRequest(s, token, s.splitCapture.key, http.MethodPost, "/api/bridge/split-capture/reader/prepare/"+envelope.Action.ID, "{}")
			if prepared.Code != http.StatusOK {
				t.Fatalf("prepare status=%d body=%s", prepared.Code, prepared.Body)
			}
			foreground := splitRequest(s, token, s.splitCapture.key, http.MethodPost, "/api/bridge/split-capture/reader/foreground/"+envelope.Action.ID, "{}")
			if foreground.Code != tc.wantForeground {
				t.Fatalf("foreground status=%d body=%s, want %d", foreground.Code, foreground.Body, tc.wantForeground)
			}
			if err := <-brokerDone; (err == nil) != tc.wantSuccess {
				t.Fatalf("broker result=%v, wantSuccess=%t", err, tc.wantSuccess)
			}
			if tc.wantSuccess && verified != 1 {
				t.Fatalf("foreground verifier calls=%d", verified)
			}

			bridgeResult := `{"ok":false,"message":"foreground rejected"}`
			if tc.wantSuccess {
				bridgeResult = `{"ok":true,"result":{"state":"opened"}}`
			}
			result := splitRequest(s, token, s.splitCapture.key, http.MethodPost, "/api/bridge/split-capture/results/"+envelope.Action.ID, bridgeResult)
			if result.Code != http.StatusNoContent {
				t.Fatalf("result status=%d body=%s", result.Code, result.Body)
			}
			response := awaitXMainAction(t, call)
			if response.Code != http.StatusOK || response.Result.OK != tc.wantSuccess {
				t.Fatalf("native action status=%d result=%+v body=%s", response.Code, response.Result, response.Body)
			}
			if got := manager.Snapshot().ActiveLeases; got != 0 {
				t.Fatalf("native action leaked %d Browser leases", got)
			}
			xMainActionStateUnchanged(t, s, settingsBefore, scheduleBefore, usageBefore, receiptsBefore, bridgeBefore)
		})
	}
}

func TestXMainBrowserActionsNativePostRejectsInvalidMissingAndMismatchedBroker(t *testing.T) {
	for _, tc := range []struct {
		name          string
		requestID     string
		brokerRequest bool
		mismatch      bool
	}{
		{name: "invalid request ID", requestID: "bad"},
		{name: "missing broker", requestID: "broker_" + strings.Repeat("b", 32)},
		{name: "mismatched broker", requestID: "broker_" + strings.Repeat("c", 32), brokerRequest: true, mismatch: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, token, manager, _ := xMainBrowserActionFixture(t)
			s.SetSplitReaderBroker(func(context.Context, string) (readerbroker.Target, func(context.Context) error, error) {
				return readerbroker.Target{HWND: 909, PID: 1010, Expires: time.Now().Add(time.Second)}, func(context.Context) error { return nil }, nil
			})
			url := "https://x.com/synthetic/status/2"
			body := fmt.Sprintf(`{"type":"open_native_post","source":"x","url":%q,"requestId":%q}`, url, tc.requestID)
			call := startXMainAction(t, s, token, body, 180*time.Millisecond)
			if tc.requestID != "bad" {
				waitXMainQueued(t, s, tc.requestID)
			}
			var brokerDone <-chan error
			var cancel context.CancelFunc
			if tc.brokerRequest {
				brokerCtx, brokerCancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
				cancel = brokerCancel
				result := make(chan error, 1)
				brokerReq := readerbroker.Request{RequestID: "broker_" + strings.Repeat("d", 32), Source: "x", URL: url}
				if !tc.mismatch {
					brokerReq.RequestID = tc.requestID
				}
				go func() {
					result <- s.HandleReaderBroker(brokerCtx, brokerReq, func(readerbroker.Target) (readerbroker.Reply, error) {
						return readerbroker.Reply{OK: true, Readback: true}, nil
					})
				}()
				brokerDone = result
			}
			response := awaitXMainAction(t, call)
			if response.Code == http.StatusOK && response.Result.OK {
				t.Fatalf("invalid/unmatched request succeeded: %+v", response)
			}
			if cancel != nil {
				cancel()
				select {
				case err := <-brokerDone:
					if err == nil {
						t.Fatal("mismatched broker was accepted")
					}
				case <-time.After(time.Second):
					t.Fatal("mismatched broker did not stop")
				}
			}
			if got := manager.Snapshot().ActiveLeases; got != 0 {
				t.Fatalf("denied native action leaked %d Browser leases", got)
			}
			s.splitCapture.mu.Lock()
			remaining := len(s.splitCapture.actions)
			s.splitCapture.mu.Unlock()
			if remaining != 0 {
				t.Fatalf("denied native action left %d queued actions", remaining)
			}
		})
	}
}

func TestXMainBrowserActionsRejectDeadOwnerWithoutLeasing(t *testing.T) {
	s, token, manager, process := xMainBrowserActionFixture(t)
	process.Terminate()
	deadline := time.Now().Add(time.Second)
	for manager.Snapshot().State != captureruntime.Failed && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if manager.Snapshot().State != captureruntime.Failed {
		t.Fatalf("synthetic owner did not fail: %+v", manager.Snapshot())
	}
	w := splitRequest(s, token, s.splitCapture.key, http.MethodPost, "/api/split-capture/actions", `{"type":"open_source","source":"x"}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("dead owner action status=%d body=%s", w.Code, w.Body)
	}
	if got := manager.Snapshot().ActiveLeases; got != 0 {
		t.Fatalf("dead owner acquired %d leases", got)
	}
	s.splitCapture.mu.Lock()
	remaining := len(s.splitCapture.actions)
	s.splitCapture.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("dead owner queued %d actions", remaining)
	}
}

func TestXMainBrowserActionsRejectMismatchedStaleOwnerWithoutLeasing(t *testing.T) {
	s, token, owner, _ := xMainBrowserActionFixture(t)
	staleManager, err := captureruntime.New(&splitLeaseProcess{done: make(chan error, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(staleManager.Terminate)
	if err := staleManager.RetainForProcess(); err != nil {
		t.Fatal(err)
	}
	if staleManager.Snapshot().Generation != owner.Snapshot().Generation {
		t.Fatal("fixture must model two different owners with equal generation numbers")
	}
	s.engine.AttachCollectionCoordinator(collection.NewCoordinator(staleManager, func(context.Context, string, uint64) (captureruntime.Process, error) {
		t.Error("stale owner must not be replaced to serve an X action")
		return nil, errors.New("unexpected stale owner replacement")
	}, func() error { return nil }))

	w := splitRequest(s, token, s.splitCapture.key, http.MethodPost, "/api/split-capture/actions", `{"type":"open_source","source":"x"}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("mismatched owner action status=%d body=%s", w.Code, w.Body)
	}
	if owner.Snapshot().ActiveLeases != 0 || staleManager.Snapshot().ActiveLeases != 0 {
		t.Fatalf("mismatched owner acquired leases: owner=%+v stale=%+v", owner.Snapshot(), staleManager.Snapshot())
	}
	s.splitCapture.mu.Lock()
	remaining := len(s.splitCapture.actions)
	s.splitCapture.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("mismatched owner queued %d actions", remaining)
	}
}

type xMainActionResponse struct {
	Code   int
	Body   string
	Result splitActionResult
}

func startXMainAction(t *testing.T, s *Server, token, body string, timeout time.Duration) <-chan xMainActionResponse {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)
	r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:11122/api/split-capture/actions", strings.NewReader(body)).WithContext(ctx)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Aku-Bridge-Contract", domain.BridgeContractVersion)
	r.Header.Set("X-Aku-Bridge-Token", token)
	r.Header.Set("X-Aku-Capture-Instance", s.splitCapture.key)
	r.Header.Set("X-Aku-Split-Epoch", s.engine.Epoch())
	done := make(chan xMainActionResponse, 1)
	go func() {
		w := httptest.NewRecorder()
		s.api().ServeHTTP(w, r)
		var result splitActionResult
		_ = json.Unmarshal(w.Body.Bytes(), &result)
		done <- xMainActionResponse{Code: w.Code, Body: w.Body.String(), Result: result}
	}()
	return done
}

func awaitXMainAction(t *testing.T, done <-chan xMainActionResponse) xMainActionResponse {
	t.Helper()
	select {
	case response := <-done:
		return response
	case <-time.After(2 * time.Second):
		t.Fatal("ordinary X Browser action did not finish")
		return xMainActionResponse{}
	}
}

func waitXMainQueued(t *testing.T, s *Server, requestID string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		s.splitCapture.mu.Lock()
		found := false
		for _, entry := range s.splitCapture.actions {
			if entry != nil && entry.action.RequestID == requestID {
				found = true
				break
			}
		}
		s.splitCapture.mu.Unlock()
		if found {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("native action %s was not queued", requestID)
}

func xMainBrowserActionFixture(t *testing.T) (*Server, string, *captureruntime.Manager, *splitLeaseProcess) {
	t.Helper()
	s, token := splitTestServer(t)
	cfg := s.config
	cfg.Dev = true
	cfg.Capture.XMode = config.XCaptureConcurrent
	cfg.Capture.XReaderSource = "main"
	cfg.WindowsCaptureSplit = true
	s.config = cfg
	s.engine = engine.New(s.store, reasoning.Deterministic{}, cfg, s.logger)

	process := &splitLeaseProcess{done: make(chan error, 1)}
	manager, err := captureruntime.New(process)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetSplitCaptureRuntime(manager); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Terminate)
	if err := manager.RetainForProcess(); err != nil {
		t.Fatal(err)
	}
	if err := s.engine.AttachCaptureRuntime(context.Background(), manager); err != nil {
		t.Fatal(err)
	}
	coordinator := collection.NewCoordinator(manager, func(context.Context, string, uint64) (captureruntime.Process, error) {
		t.Error("ordinary X Browser actions must not replace the retained main owner")
		return nil, errors.New("unexpected main owner replacement")
	}, func() error { return nil })
	s.engine.AttachCollectionCoordinator(coordinator)
	return s, token, manager, process
}

func xMainActionState(t *testing.T, s *Server) (domain.Settings, store.AutoUpdateScheduleState, store.AutoUpdateBudgetUsage, []domain.AutoUpdateTickReceipt) {
	t.Helper()
	ctx := context.Background()
	settings, err := s.engine.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	schedule, err := s.store.AutoUpdateScheduleState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	usage, err := s.store.AutoUpdateBudgetUsage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	receipts, err := s.store.AutoUpdateSchedulerReceipts(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	return settings, schedule, usage, receipts
}

func xMainActionStateUnchanged(t *testing.T, s *Server, settings domain.Settings, schedule store.AutoUpdateScheduleState, usage store.AutoUpdateBudgetUsage, receipts []domain.AutoUpdateTickReceipt, bridge *domain.BridgeHeartbeat) {
	t.Helper()
	settingsAfter, scheduleAfter, usageAfter, receiptsAfter := xMainActionState(t, s)
	if !reflect.DeepEqual(settingsAfter, settings) {
		t.Fatal("ordinary X action mutated settings or source selection")
	}
	if !reflect.DeepEqual(scheduleAfter, schedule) || !reflect.DeepEqual(usageAfter, usage) || !reflect.DeepEqual(receiptsAfter, receipts) {
		t.Fatalf("ordinary X action mutated AutoUpdate state: schedule before=%+v after=%+v usage before=%+v after=%+v receipts before=%+v after=%+v", schedule, scheduleAfter, usage, usageAfter, receipts, receiptsAfter)
	}
	if !reflect.DeepEqual(s.engine.BridgeStatus().Actual, bridge) {
		t.Fatal("ordinary X action mutated Bridge source access state")
	}
	if s.engine.XCaptureStatus().DerivationAuthorized {
		t.Fatal("ordinary X action granted process-scoped derivation authority")
	}
}
