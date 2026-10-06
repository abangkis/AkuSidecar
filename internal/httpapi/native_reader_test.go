package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/collection"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
)

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func boolPointer(value bool) *bool { return &value }

func TestDirectNativeReaderAPICompletesWithoutBridgeCollector(t *testing.T) {
	s, token := splitTestServer(t)
	req := readerbroker.Request{RequestID: "broker_" + strings.Repeat("e", 32), Source: "linkedin", URL: "https://www.linkedin.com/feed/update/urn:li:activity:1234567890/"}
	s.SetSplitDirectNativeReader(func(_ context.Context, id, url, marker string) (readerbroker.NativePostPreparation, error) {
		if url != req.URL || marker != "/split-reader-intent?id="+id {
			t.Error("correlation changed")
		}
		return readerbroker.NativePostPreparation{
			Target:           readerbroker.Target{HWND: 1, PID: 2, Expires: time.Now().Add(time.Second)},
			VerifyForeground: func(context.Context) error { return nil },
			NavigatePost:     func(context.Context) error { return nil },
		}, nil
	}, func(context.Context) error { return nil })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	uiDone := make(chan splitActionResult, 1)
	go func() {
		w := splitRequest(s, token, s.splitCapture.key, "POST", "/api/split-capture/actions", `{"type":"open_native_post","source":"linkedin","url":"`+req.URL+`","requestId":"`+req.RequestID+`"}`)
		var result splitActionResult
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil {
			t.Errorf("direct API status=%d body=%s", w.Code, w.Body)
		}
		uiDone <- result
	}()
	if err := s.HandleReaderBroker(ctx, req, func(readerbroker.Target) (readerbroker.Reply, error) {
		return readerbroker.Reply{OK: true, Readback: true}, nil
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-uiDone:
		if !result.OK || !strings.Contains(string(result.Result), "native_post_opened") {
			t.Fatal("direct API result", result)
		}
	case <-ctx.Done():
		t.Fatal("direct API stalled without collector")
	}
	s.splitCapture.mu.Lock()
	remaining := len(s.splitCapture.actions)
	s.splitCapture.mu.Unlock()
	if remaining != 0 {
		t.Fatal("direct API left queued reader")
	}
}

func TestDirectNativeReaderRequiresMatchedBrokerAndVerifiedActivation(t *testing.T) {
	visible, notMinimized := true, false
	tests := []struct {
		name                  string
		reply                 readerbroker.Reply
		activationErr         error
		targetLifetime        time.Duration
		cancelDuringActivate  bool
		expectActionOK        bool
		expectManual          bool
		expectForegroundCheck bool
		expectNavigation      bool
	}{
		{name: "verified activation", reply: readerbroker.Reply{OK: true, Readback: true}, targetLifetime: time.Second, expectActionOK: true, expectForegroundCheck: true, expectNavigation: true},
		{name: "plain rejection", reply: readerbroker.Reply{}, targetLifetime: time.Second},
		{name: "authentic Windows rejection", reply: readerbroker.Reply{Message: "Windows rejected reader activation", FocusCategory: "ui", ReaderVisible: &visible, ReaderMinimized: &notMinimized}, targetLifetime: time.Second, expectActionOK: true, expectManual: true, expectNavigation: true},
		{name: "missing diagnostics", reply: readerbroker.Reply{Message: "Windows rejected reader activation"}, targetLifetime: time.Second},
		{name: "changed focus", reply: readerbroker.Reply{Message: "Windows rejected reader activation", FocusCategory: "other", ReaderVisible: &visible, ReaderMinimized: &notMinimized}, targetLifetime: time.Second},
		{name: "missing visibility", reply: readerbroker.Reply{Message: "Windows rejected reader activation", FocusCategory: "ui", ReaderMinimized: &notMinimized}, targetLifetime: time.Second},
		{name: "minimized reader", reply: readerbroker.Reply{Message: "Windows rejected reader activation", FocusCategory: "ui", ReaderVisible: &visible, ReaderMinimized: boolPointer(true)}, targetLifetime: time.Second},
		{name: "expired binding reply", reply: readerbroker.Reply{Message: "Reader binding expired or changed", FocusCategory: "ui", ReaderVisible: &visible, ReaderMinimized: &notMinimized}, targetLifetime: time.Second},
		{name: "changed UI reply", reply: readerbroker.Reply{Message: "Reader intent expired or UI foreground changed", FocusCategory: "other", ReaderVisible: &visible, ReaderMinimized: &notMinimized}, targetLifetime: time.Second},
		{name: "transport error", reply: readerbroker.Reply{Message: "Windows rejected reader activation", FocusCategory: "ui", ReaderVisible: &visible, ReaderMinimized: &notMinimized}, activationErr: errors.New("pipe failed"), targetLifetime: time.Second},
		{name: "binding expires during activation", reply: readerbroker.Reply{Message: "Windows rejected reader activation", FocusCategory: "ui", ReaderVisible: &visible, ReaderMinimized: &notMinimized}, targetLifetime: 200 * time.Millisecond},
		{name: "cancelled during activation", reply: readerbroker.Reply{Message: "Windows rejected reader activation", FocusCategory: "ui", ReaderVisible: &visible, ReaderMinimized: &notMinimized}, targetLifetime: time.Second, cancelDuringActivate: true},
	}
	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, token := splitTestServer(t)
			req := readerbroker.Request{RequestID: "broker_" + strings.Repeat(fmt.Sprintf("%x", i), 32), Source: "x", URL: "https://x.com/a/status/1"}
			entry := &pendingSplitAction{action: splitCaptureAction{ID: "split_direct", Type: "open_native_post", RequestID: req.RequestID, Source: req.Source, URL: req.URL}, directReader: true, brokerReady: make(chan struct{}), brokerDone: make(chan error, 1), result: make(chan splitActionResult, 1)}
			s.splitCapture.actions = []*pendingSplitAction{entry, {action: splitCaptureAction{ID: "split_ping", Type: "ping"}}}
			prepared := make(chan struct{}, 1)
			verifyCalls, navigateCalls := 0, 0
			prepare := func(_ context.Context, id, url, marker string) (readerbroker.NativePostPreparation, error) {
				if id != "split_direct" || url != req.URL || marker != "/split-reader-intent?id=split_direct" {
					t.Error("wrong reader correlation")
				}
				prepared <- struct{}{}
				return readerbroker.NativePostPreparation{
					Target:           readerbroker.Target{HWND: 123, PID: 456, Action: id, Expires: time.Now().Add(tc.targetLifetime)},
					VerifyForeground: func(context.Context) error { verifyCalls++; return nil },
					NavigatePost:     func(context.Context) error { navigateCalls++; return nil },
				}, nil
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			go s.runDirectNativeReader(ctx, s.splitCapture, entry, prepare)
			// Even an attached reader cannot be taken by a Bridge poll.
			claim := splitRequest(s, token, s.splitCapture.key, "GET", "/api/bridge/split-capture/next", "")
			if !strings.Contains(claim.Body.String(), "split_ping") {
				t.Fatal("Bridge stole direct reader", claim.Body)
			}
			select {
			case <-prepared:
				t.Fatal("API opened reader without broker")
			default:
			}
			activationErr := s.HandleReaderBroker(ctx, req, func(target readerbroker.Target) (readerbroker.Reply, error) {
				if target.HWND != 123 {
					t.Error("wrong target")
				}
				if tc.cancelDuringActivate {
					cancel()
				}
				if tc.name == "binding expires during activation" {
					remaining := time.Until(target.Expires)
					if remaining <= 0 {
						t.Error("test target expired before activation began")
					} else {
						time.Sleep(remaining + 10*time.Millisecond)
					}
				}
				return tc.reply, tc.activationErr
			})
			if (activationErr == nil) != (tc.expectActionOK || tc.expectForegroundCheck) {
				t.Fatalf("broker completion mismatch: %v", activationErr)
			}
			select {
			case result := <-entry.result:
				if result.OK != tc.expectActionOK {
					t.Fatalf("action ok=%t want %t: %+v", result.OK, tc.expectActionOK, result)
				}
				if tc.expectManual {
					var detail map[string]string
					if json.Unmarshal(result.Result, &detail) != nil || detail["foreground"] != "manual_required" || detail["message"] != nativeReaderManualForegroundMessage || result.Message != nativeReaderManualForegroundMessage {
						t.Fatalf("manual foreground feedback missing: %+v detail=%v", result, detail)
					}
					if entry.readerForegroundVerified || !entry.readerManualForegroundRequired {
						t.Fatal("fallback was recorded as foreground success")
					}
				} else if entry.readerManualForegroundRequired {
					t.Fatal("unexpected manual foreground result")
				}
			case <-time.After(500 * time.Millisecond):
				t.Fatal("direct action did not complete")
			}
			if verifyCalls != boolInt(tc.expectForegroundCheck) || navigateCalls != boolInt(tc.expectNavigation) {
				t.Fatalf("verify=%d navigate=%d", verifyCalls, navigateCalls)
			}
			if entry.readerForegroundVerified != tc.expectForegroundCheck {
				t.Fatalf("foreground verification state=%t", entry.readerForegroundVerified)
			}
			if !entry.completed {
				t.Fatal("direct action retained queue ownership")
			}
		})
	}
}

func TestDirectNativeReaderCancellationDrainsWithoutOpening(t *testing.T) {
	s, _ := splitTestServer(t)
	entry := &pendingSplitAction{action: splitCaptureAction{ID: "split_cancel", Type: "open_native_post"}, directReader: true, brokerReady: make(chan struct{}), brokerDone: make(chan error, 1), result: make(chan splitActionResult, 1), detached: true}
	released := 0
	entry.interactionRelease = func() { released++ }
	s.splitCapture.actions = []*pendingSplitAction{entry}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.runDirectNativeReader(ctx, s.splitCapture, entry, func(context.Context, string, string, string) (readerbroker.NativePostPreparation, error) {
		t.Fatal("cancelled action opened reader")
		return readerbroker.NativePostPreparation{}, nil
	})
	if released != 1 || len(s.splitCapture.actions) != 0 || (<-entry.result).OK {
		t.Fatal("cancelled direct reader leaked ownership")
	}
}

func TestReaderBrokerCancellationReleasesExactUnmatchedWait(t *testing.T) {
	s, token := splitTestServer(t)
	req := readerbroker.Request{RequestID: "broker_" + strings.Repeat("f", 32), Source: "x", URL: "https://x.com/a/status/1"}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- s.HandleReaderBroker(ctx, req, func(readerbroker.Target) (readerbroker.Reply, error) {
			t.Error("unmatched canceled click reached activation")
			return readerbroker.Reply{}, nil
		})
	}()
	deadline := time.Now().Add(time.Second)
	registered := false
	for time.Now().Before(deadline) {
		s.splitCapture.mu.Lock()
		active := s.splitCapture.readerBrokers[req.RequestID] != nil && s.splitCapture.readerBrokers[req.RequestID].cancel != nil
		s.splitCapture.mu.Unlock()
		if active {
			registered = true
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !registered {
		t.Fatal("native helper did not register its unmatched wait")
	}

	wrongTuple := req
	wrongTuple.URL = "https://x.com/a/status/2"
	if err := s.CancelReaderBroker(wrongTuple); err == nil {
		t.Fatal("cancellation with a different URL was accepted")
	}
	if w := splitRequest(s, "", s.splitCapture.key, "POST", "/api/split-capture/reader-broker/cancel", `{"requestId":"`+req.RequestID+`","source":"x","url":"`+req.URL+`"}`); w.Code != 401 {
		t.Fatalf("unauthenticated cancellation=%d %s", w.Code, w.Body)
	}
	stale := httptest.NewRequest("POST", "http://127.0.0.1:11122/api/split-capture/reader-broker/cancel", strings.NewReader(`{"requestId":"`+req.RequestID+`","source":"x","url":"`+req.URL+`"}`))
	stale.Header.Set("Content-Type", "application/json")
	stale.Header.Set("X-Aku-Bridge-Token", token)
	stale.Header.Set("X-Aku-Bridge-Contract", domain.BridgeContractVersion)
	stale.Header.Set("X-Aku-Split-Epoch", "stale-epoch")
	w := httptest.NewRecorder()
	s.api().ServeHTTP(w, stale)
	if w.Code != 409 {
		t.Fatalf("stale-epoch cancellation=%d %s", w.Code, w.Body)
	}
	if w = splitRequest(s, token, s.splitCapture.key, "POST", "/api/split-capture/reader-broker/cancel", `{"requestId":"`+req.RequestID+`","source":"x","url":"`+req.URL+`"}`); w.Code != 200 {
		t.Fatalf("cancellation=%d %s", w.Code, w.Body)
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("unmatched waiter returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation did not release the unmatched native pipe wait")
	}
	if err := s.HandleReaderBroker(ctx, req, func(readerbroker.Target) (readerbroker.Reply, error) {
		t.Error("late helper reached activation")
		return readerbroker.Reply{}, nil
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("late helper did not observe cancellation tombstone: %v", err)
	}
}

func TestReaderBrokerCancellationReleasesOnlyMatchingUnattachedAction(t *testing.T) {
	s, token := splitTestServer(t)
	m, err := captureruntime.New(&splitLeaseProcess{done: make(chan error, 1)})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetSplitCaptureRuntime(m); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Terminate)
	request := readerbroker.Request{RequestID: "broker_" + strings.Repeat("1", 32), Source: "x", URL: "https://x.com/a/status/1"}
	otherRequest := readerbroker.Request{RequestID: "broker_" + strings.Repeat("2", 32), Source: "x", URL: "https://x.com/a/status/2"}
	lease, err := m.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	otherLease, err := m.Acquire()
	if err != nil {
		lease.Release()
		t.Fatal(err)
	}
	releases := 0
	old := &pendingSplitAction{action: splitCaptureAction{ID: "split_old", Type: "open_native_post", RequestID: request.RequestID, Source: request.Source, URL: request.URL}, result: make(chan splitActionResult, 1), brokerReady: make(chan struct{}), brokerDone: make(chan error, 1), runtimeLease: lease, interactionRelease: func() { releases++ }}
	newer := &pendingSplitAction{action: splitCaptureAction{ID: "split_new", Type: "open_native_post", RequestID: otherRequest.RequestID, Source: otherRequest.Source, URL: otherRequest.URL}, result: make(chan splitActionResult, 1), brokerReady: make(chan struct{}), brokerDone: make(chan error, 1), runtimeLease: otherLease}
	s.splitCapture.actions = []*pendingSplitAction{old, newer}
	if w := splitRequest(s, token, s.splitCapture.key, "POST", "/api/split-capture/reader-broker/cancel", `{"requestId":"`+request.RequestID+`","source":"x","url":"`+request.URL+`"}`); w.Code != 200 {
		t.Fatalf("cancellation=%d %s", w.Code, w.Body)
	}
	s.splitCapture.mu.Lock()
	remaining := append([]*pendingSplitAction(nil), s.splitCapture.actions...)
	s.splitCapture.mu.Unlock()
	if len(remaining) != 1 || remaining[0] != newer || releases != 1 || m.Snapshot().ActiveLeases != 1 {
		t.Fatalf("canceled action cleanup affected another click: remaining=%v releases=%d leases=%d", len(remaining), releases, m.Snapshot().ActiveLeases)
	}
	select {
	case result := <-old.result:
		if result.OK || result.Error != "reader_broker_cancelled" {
			t.Fatalf("canceled action result=%+v", result)
		}
	default:
		t.Fatal("canceled action did not receive a terminal result")
	}
	body := `{"type":"open_native_post","source":"x","url":"` + request.URL + `","requestId":"` + request.RequestID + `"}`
	if w := splitRequest(s, token, s.splitCapture.key, "POST", "/api/split-capture/actions", body); w.Code != 409 {
		t.Fatalf("canceled click was queued after a later handoff: %d %s", w.Code, w.Body)
	}
	if m.Snapshot().ActiveLeases != 1 {
		t.Fatal("late API admission acquired a lease after cancellation")
	}
	brokerContext, brokerCancel := context.WithCancel(context.Background())
	defer brokerCancel()
	conversation := &readerBrokerConversation{source: otherRequest.Source, url: otherRequest.URL, cancel: brokerCancel, expires: time.Now().Add(readerbroker.PreparationLifetime)}
	s.splitCapture.mu.Lock()
	newer.brokerAttached = true
	s.splitCapture.readerBrokers[otherRequest.RequestID] = conversation
	s.splitCapture.mu.Unlock()
	if err := s.CancelReaderBroker(otherRequest); err != nil {
		t.Fatal(err)
	}
	s.splitCapture.mu.Lock()
	attachedPreserved := len(s.splitCapture.actions) == 1 && s.splitCapture.actions[0] == newer && !newer.completed &&
		s.splitCapture.readerBrokers[otherRequest.RequestID] == conversation && !conversation.cancelled && conversation.cancel != nil
	s.splitCapture.mu.Unlock()
	if !attachedPreserved || brokerContext.Err() != nil || m.Snapshot().ActiveLeases != 1 {
		t.Fatal("exact cancellation disturbed an already attached action or its broker conversation")
	}
	s.splitCapture.mu.Lock()
	newer.brokerAttached = false
	s.splitCapture.mu.Unlock()
	if err := s.CancelReaderBroker(otherRequest); err != nil {
		t.Fatal(err)
	}
	s.splitCapture.mu.Lock()
	newerReleased := len(s.splitCapture.actions) == 0 && newer.completed && conversation.cancelled && conversation.cancel == nil
	s.splitCapture.mu.Unlock()
	if !newerReleased || m.Snapshot().ActiveLeases != 0 {
		t.Fatal("newer exact cancellation did not release its lease")
	}
}

func TestReaderBrokerConversationCapacityPreservesLiveEntriesAndPrunesExpiredOnes(t *testing.T) {
	s, _ := splitTestServer(t)
	type testEntry struct {
		id  string
		req readerbroker.Request
	}
	entries := make([]testEntry, readerBrokerConversationLimit)
	for i := 0; i < readerBrokerConversationLimit; i++ {
		id := fmt.Sprintf("broker_%032x", i)
		req := readerbroker.Request{RequestID: id, Source: "x", URL: fmt.Sprintf("https://x.com/a/status/%d", i+1)}
		entries[i] = testEntry{id: id, req: req}
		s.splitCapture.readerBrokers[id] = &readerBrokerConversation{source: req.Source, url: req.URL, expires: time.Now().Add(time.Minute)}
	}
	full := readerbroker.Request{RequestID: "broker_" + strings.Repeat("a", 32), Source: "x", URL: "https://x.com/a/status/1000"}
	if err := s.CancelReaderBroker(full); err == nil {
		t.Fatal("new cancellation tombstone exceeded capacity")
	}
	if err := s.CancelReaderBroker(entries[0].req); err != nil {
		t.Fatalf("existing live conversation could not be canceled at capacity: %v", err)
	}
	if err := s.HandleReaderBroker(context.Background(), full, func(readerbroker.Target) (readerbroker.Reply, error) { return readerbroker.Reply{}, nil }); err == nil || err.Error() != "reader broker capacity reached" {
		t.Fatalf("new helper was not rejected at capacity: %v", err)
	}
	s.splitCapture.readerBrokers[entries[1].id].expires = time.Now().Add(-time.Second)
	if err := s.CancelReaderBroker(full); err != nil {
		t.Fatalf("expired tombstone was not pruned for a new request: %v", err)
	}
	if s.splitCapture.readerBrokers[entries[0].id] == nil || s.splitCapture.readerBrokers[full.RequestID] == nil {
		t.Fatal("capacity handling evicted a live conversation or lost the new tombstone")
	}
}

func TestDirectNativeReadinessBypassesOnlyBridgeNegotiation(t *testing.T) {
	s, _ := splitTestServer(t)
	want := errors.New("reader remains open")
	s.SetSplitDirectNativeReader(nil, func(context.Context) error { return want })
	if !errors.Is(s.SplitCaptureReplacementReadiness(context.Background()), want) {
		t.Fatal("reader lifetime boundary ignored")
	}
	if err := s.RotateSplitCapture(); err != nil {
		t.Fatal(err)
	}
	if s.SplitCaptureReplacementReadiness(context.Background()) == nil {
		t.Fatal("old direct readiness survived rotation")
	}
}

func TestNativeReaderCloseWithStaleStatusDoesNotCloseOtherWindows(t *testing.T) {
	s, _ := splitTestServer(t)
	called := false
	s.SetNativeReaderCloseAction(func(context.Context) error { called = true; return nil })
	result, err := s.closeNativeReaderAndResume(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if called || result["closed"] != false || result["resumeOutcome"] != "no_reader" {
		t.Fatalf("stale reader status invoked a broad close or claimed success: called=%t result=%v", called, result)
	}
}

func TestNativeReaderActionGateRejectsOpensRacingClose(t *testing.T) {
	s, _ := splitTestServer(t)
	openRelease, err := s.beginNativeReaderAction(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	queuedResult := make(chan error, 1)
	go func() {
		release, err := s.beginNativeReaderAction(context.Background())
		if release != nil {
			release()
		}
		queuedResult <- err
	}()
	waitersDeadline := time.Now().Add(time.Second)
	for {
		s.nativeReaderGateMu.Lock()
		waiters := s.nativeReaderWaiters
		s.nativeReaderGateMu.Unlock()
		if waiters > 0 {
			break
		}
		if time.Now().After(waitersDeadline) {
			t.Fatal("second open did not queue behind the active action")
		}
		time.Sleep(time.Millisecond)
	}
	closeReady := make(chan func(), 1)
	closeErr := make(chan error, 1)
	go func() {
		finish, err := s.beginNativeReaderClose(context.Background())
		if err != nil {
			closeErr <- err
			return
		}
		closeReady <- finish
	}()
	deadline := time.Now().Add(time.Second)
	for {
		s.nativeReaderGateMu.Lock()
		closing := s.nativeReaderClosing
		s.nativeReaderGateMu.Unlock()
		if closing {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("close did not fence new native reader actions")
		}
		time.Sleep(time.Millisecond)
	}
	if _, err := s.beginNativeReaderAction(context.Background()); err == nil {
		t.Fatal("native reader open entered during close")
	}
	openRelease()
	if err := <-queuedResult; err == nil {
		t.Fatal("queued native reader action crossed the close epoch")
	}
	select {
	case err := <-closeErr:
		t.Fatal(err)
	case finish := <-closeReady:
		finish()
	case <-time.After(time.Second):
		t.Fatal("close did not proceed after the in-flight action completed")
	}
}

type nativeResumeFixtureProcess struct {
	*collectionUIFixtureProcess
	readerOpen atomic.Bool
}

func (p *nativeResumeFixtureProcess) ReplacementReadiness(context.Context) error {
	if p.readerOpen.Load() {
		return errors.New("reader still open")
	}
	return nil
}

func TestNativeReaderCloseDoesNotStartSessionBeforeHeadlessReady(t *testing.T) {
	s, _ := splitTestServer(t)
	process := func(mode string) *collectionUIFixtureProcess {
		return &collectionUIFixtureProcess{splitLeaseProcess: splitLeaseProcess{done: make(chan error, 1)}, mode: mode}
	}
	owner, err := captureruntime.New(process("headless"))
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Terminate()
	if err := s.engine.AttachCaptureRuntime(context.Background(), owner); err != nil {
		t.Fatal(err)
	}
	var reader *nativeResumeFixtureProcess
	var coordinator *collection.Coordinator
	coordinator = collection.NewCoordinator(owner, func(_ context.Context, mode string, generation uint64) (captureruntime.Process, error) {
		if mode == "native_reader" {
			reader = &nativeResumeFixtureProcess{collectionUIFixtureProcess: process("browser")}
			reader.readerOpen.Store(true)
			return reader, nil
		}
		return process(mode), nil
	}, func() error { return nil })
	s.engine.AttachCollectionCoordinator(coordinator)
	coordinatorCtx, cancelCoordinator := context.WithCancel(context.Background())
	defer cancelCoordinator()
	coordinator.Start(coordinatorCtx)
	coordinator.Request("headless")
	borrowCtx, cancelBorrow := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelBorrow()
	type borrowed struct {
		lease   *captureruntime.Lease
		release func()
		err     error
	}
	borrowedResult := make(chan borrowed, 1)
	go func() {
		lease, release, err := coordinator.BorrowNativeReader(borrowCtx)
		borrowedResult <- borrowed{lease, release, err}
	}()
	var borrow borrowed
	select {
	case borrow = <-borrowedResult:
	case <-borrowCtx.Done():
		t.Fatal("native reader fixture did not open")
	}
	if borrow.err != nil {
		t.Fatal(borrow.err)
	}
	borrow.lease.Release()
	borrow.release()
	if !coordinator.Status().NativeReaderOnly {
		t.Fatal("native reader status was not active")
	}
	s.SetNativeReaderCloseAction(func(context.Context) error {
		return errors.New("graceful close rejected")
	})
	if _, err := s.closeNativeReaderAndResume(context.Background()); err == nil || !reader.readerOpen.Load() {
		t.Fatal("failed close released the reader or resumed collection", err)
	}
	s.SetNativeReaderCloseAction(func(context.Context) error {
		reader.readerOpen.Store(false)
		return nil
	})
	closeCtx, cancelClose := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancelClose()
	result, err := s.closeNativeReaderAndResume(closeCtx)
	if err != nil {
		t.Fatal(err)
	}
	if result["closed"] != true || result["resumeOutcome"] != "headless_not_ready" || result["session"] != nil {
		t.Fatalf("batch started or closure was misreported before readiness: %v", result)
	}
}
