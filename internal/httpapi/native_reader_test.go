package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/readerbroker"
)

func TestDirectNativeReaderAPICompletesWithoutBridgeCollector(t *testing.T) {
	s, token := splitTestServer(t)
	req := readerbroker.Request{RequestID: "broker_" + strings.Repeat("e", 32), Source: "linkedin", URL: "https://www.linkedin.com/feed/update/urn:li:activity:1234567890/"}
	s.SetSplitDirectNativeReader(func(_ context.Context, id, url, marker string) (readerbroker.Target, func(context.Context) error, error) {
		if url != req.URL || marker != "/split-reader-intent?id="+id {
			t.Error("correlation changed")
		}
		return readerbroker.Target{HWND: 1, PID: 2, Expires: time.Now().Add(time.Second)}, func(context.Context) error { return nil }, nil
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
	for _, accepted := range []bool{true, false} {
		t.Run(map[bool]string{true: "accepted", false: "activation_rejected"}[accepted], func(t *testing.T) {
			s, token := splitTestServer(t)
			req := readerbroker.Request{RequestID: "broker_" + strings.Repeat("d", 32), Source: "x", URL: "https://x.com/a/status/1"}
			entry := &pendingSplitAction{action: splitCaptureAction{ID: "split_direct", Type: "open_native_post", RequestID: req.RequestID, Source: req.Source, URL: req.URL}, directReader: true, brokerReady: make(chan struct{}), brokerDone: make(chan error, 1), result: make(chan splitActionResult, 1)}
			s.splitCapture.actions = []*pendingSplitAction{entry, {action: splitCaptureAction{ID: "split_ping", Type: "ping"}}}
			prepared := make(chan struct{}, 1)
			prepare := func(_ context.Context, id, url, marker string) (readerbroker.Target, func(context.Context) error, error) {
				if id != "split_direct" || url != req.URL || marker != "/split-reader-intent?id=split_direct" {
					t.Error("wrong reader correlation")
				}
				prepared <- struct{}{}
				return readerbroker.Target{HWND: 123, PID: 456, Action: id, Expires: time.Now().Add(time.Second)}, func(context.Context) error { return nil }, nil
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
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
			err := s.HandleReaderBroker(ctx, req, func(target readerbroker.Target) (readerbroker.Reply, error) {
				if target.HWND != 123 {
					t.Error("wrong target")
				}
				return readerbroker.Reply{OK: accepted, Readback: accepted}, nil
			})
			if (err == nil) != accepted {
				t.Fatal("activation result mismatch", err)
			}
			select {
			case result := <-entry.result:
				if result.OK != accepted {
					t.Fatal("result did not require activation", result)
				}
			case <-ctx.Done():
				t.Fatal("direct action did not complete")
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
	s.runDirectNativeReader(ctx, s.splitCapture, entry, func(context.Context, string, string, string) (readerbroker.Target, func(context.Context) error, error) {
		t.Fatal("cancelled action opened reader")
		return readerbroker.Target{}, nil, nil
	})
	if released != 1 || len(s.splitCapture.actions) != 0 || (<-entry.result).OK {
		t.Fatal("cancelled direct reader leaked ownership")
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
