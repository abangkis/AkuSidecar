package httpapi

import (
	"context"
	"encoding/json"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
	"strings"
	"testing"
	"time"
)

func TestReaderBrokerCollectorCannotClaimUnattachedClick(t *testing.T) {
	s, token := splitTestServer(t)
	blocked := &pendingSplitAction{action: splitCaptureAction{ID: "split_reader", Type: "open_native_post"}, brokerReady: make(chan struct{}), brokerDone: make(chan error, 1)}
	background := &pendingSplitAction{action: splitCaptureAction{ID: "split_background", Type: "ping"}}
	s.splitCapture.actions = []*pendingSplitAction{blocked, background}
	w := splitRequest(s, token, s.splitCapture.key, "GET", "/api/bridge/split-capture/next", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "split_background") || blocked.claimed {
		t.Fatalf("unattached reader claimed: %d %s", w.Code, w.Body)
	}
}

func TestReaderBrokerForegroundReturnsManualRequiredAfterNavigationFallback(t *testing.T) {
	s, token := splitTestServer(t)
	s.SetSplitReaderBroker(func(context.Context, string) (readerbroker.Target, func(context.Context) error, error) {
		return readerbroker.Target{HWND: 1}, func(context.Context) error { return nil }, nil
	})
	entry := &pendingSplitAction{
		action:                         splitCaptureAction{ID: "split_reader", Type: "open_native_post"},
		claimed:                        true,
		directReader:                   true,
		completed:                      true,
		readerManualForegroundRequired: true,
		completionResult:               &splitActionResult{OK: true},
		brokerReady:                    make(chan struct{}),
		brokerDone:                     make(chan error, 1),
	}
	close(entry.brokerReady)
	entry.brokerDone <- nil
	s.splitCapture.actions = []*pendingSplitAction{entry}
	w := splitRequest(s, token, s.splitCapture.key, "POST", "/api/bridge/split-capture/reader/foreground/split_reader", "{}")
	if w.Code != 200 {
		t.Fatalf("foreground fallback=%d %s", w.Code, w.Body)
	}
	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["foreground"] != false || result["manualRequired"] != true || result["message"] != nativeReaderManualForegroundMessage {
		t.Fatalf("manual foreground result was not explicit: %+v", result)
	}
	if w := splitRequest(s, token, s.splitCapture.key, "POST", "/api/bridge/split-capture/reader/foreground/split_reader", "{}"); w.Code != 409 {
		t.Fatal("manual foreground response was replayed", w.Code, w.Body)
	}
}

func TestReaderBrokerMatchesClickAndVerifiesHelperBeforeCaptureSuccess(t *testing.T) {
	s, token := splitTestServer(t)
	req := readerbroker.Request{RequestID: "broker_" + strings.Repeat("a", 32), Source: "x", URL: "https://x.com/a/status/1"}
	verified := 0
	s.SetSplitReaderBroker(func(context.Context, string) (readerbroker.Target, func(context.Context) error, error) {
		return readerbroker.Target{HWND: 123, PID: 456, Value: 1, Action: "split_reader", Expires: time.Now().Add(time.Second)}, func(context.Context) error { verified++; return nil }, nil
	})
	entry := &pendingSplitAction{action: splitCaptureAction{ID: "split_reader", Type: "open_native_post", RequestID: req.RequestID, Source: req.Source, URL: req.URL}, brokerReady: make(chan struct{}), brokerDone: make(chan error, 1)}
	s.splitCapture.actions = []*pendingSplitAction{entry}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- s.HandleReaderBroker(ctx, req, func(target readerbroker.Target) (readerbroker.Reply, error) {
			if target.HWND != 123 {
				t.Error("wrong target")
			}
			return readerbroker.Reply{OK: true, Readback: true}, nil
		})
	}()
	w := splitRequest(s, token, s.splitCapture.key, "GET", "/api/bridge/split-capture/next", "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	w = splitRequest(s, token, s.splitCapture.key, "POST", "/api/bridge/split-capture/reader/prepare/split_reader", "{}")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	w = splitRequest(s, token, s.splitCapture.key, "POST", "/api/bridge/split-capture/reader/foreground/split_reader", "{}")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if verified != 1 || !entry.readerForegroundVerified {
		t.Fatal("server verification missing")
	}
}

func TestReaderBrokerRejectsUncorrelatedRequest(t *testing.T) {
	s, _ := splitTestServer(t)
	req := readerbroker.Request{RequestID: "broker_" + strings.Repeat("b", 32), Source: "x", URL: "https://x.com/a/status/1"}
	s.splitCapture.actions = []*pendingSplitAction{{action: splitCaptureAction{Type: "dispatch", RequestID: req.RequestID, Source: req.Source, URL: req.URL}, brokerReady: make(chan struct{}), brokerDone: make(chan error, 1)}}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	called := false
	err := s.HandleReaderBroker(ctx, req, func(readerbroker.Target) (readerbroker.Reply, error) { called = true; return readerbroker.Reply{}, nil })
	if err == nil || called || s.splitCapture.actions[0].brokerAttached {
		t.Fatal("background action acquired native activation")
	}
}
