package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
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
	var output strings.Builder
	s.logger = log.New(&output, "", 0)
	req := readerbroker.Request{RequestID: "broker_" + strings.Repeat("b", 32), Source: "x", URL: "https://x.com/a/status/1"}
	s.splitCapture.actions = []*pendingSplitAction{{action: splitCaptureAction{Type: "dispatch", RequestID: req.RequestID, Source: req.Source, URL: req.URL}, brokerReady: make(chan struct{}), brokerDone: make(chan error, 1)}}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	called := false
	err := s.HandleReaderBroker(ctx, req, func(readerbroker.Target) (readerbroker.Reply, error) { called = true; return readerbroker.Reply{}, nil })
	if err == nil || called || s.splitCapture.actions[0].brokerAttached {
		t.Fatal("background action acquired native activation")
	}
	events := readActivityLines(t, output.String())
	if !errors.Is(err, context.DeadlineExceeded) || len(events) != 2 || events[0].Action != "" || events[1].Action != "" ||
		events[1].RequestID != req.RequestID || events[1].Outcome != "timed_out" || events[1].QueueWait != nil {
		t.Fatal(events)
	}
}

func TestReaderBrokerActivityConcurrentReceiptsBeforeQueue(t *testing.T) {
	s, _ := splitTestServer(t)
	var output strings.Builder
	s.logger = log.New(&output, "", 0)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	requests := []readerbroker.Request{
		{RequestID: domain.NewID("broker"), Source: "x", URL: "https://x.com/account/status/101"},
		{RequestID: domain.NewID("broker"), Source: "x", URL: "https://x.com/account/status/102"},
	}
	entries := make([]*pendingSplitAction, len(requests))
	verifyFailure := errors.New("PRIVATE_VERIFY")
	wantErrors := []error{nil, verifyFailure}
	done := make([]chan error, len(requests))
	for i, req := range requests {
		id := domain.NewID("split")
		entry := &pendingSplitAction{action: splitCaptureAction{Type: "open_native_post", ID: id, RequestID: req.RequestID, Source: req.Source, URL: req.URL},
			brokerReady: make(chan struct{}), brokerDone: make(chan error, 1), readerForeground: func(context.Context) error { return wantErrors[i] },
			brokerTarget: readerbroker.Target{HWND: uint64(i + 1), Action: id, Expires: time.Now().Add(3 * time.Second)}}
		close(entry.brokerReady)
		entries[i] = entry
		done[i] = make(chan error, 1)
		go func() {
			done[i] <- s.HandleReaderBroker(ctx, req, func(target readerbroker.Target) (readerbroker.Reply, error) {
				if target.Action != id {
					return readerbroker.Reply{}, errors.New("wrong action attached")
				}
				return readerbroker.Reply{OK: true, Applied: true, Readback: true, Ticket: "PRIVATE_TICKET"}, nil
			})
		}()
	}
	transport := s.splitCapture
	waitForSplitTest(t, "two broker receipts", func() bool {
		transport.mu.Lock()
		defer transport.mu.Unlock()
		return len(transport.readerBrokers) == 2
	})
	mismatch := requests[0]
	mismatch.URL = requests[1].URL
	if err := s.HandleReaderBroker(ctx, mismatch, nil); err == nil {
		t.Fatal("conflicting registration accepted")
	}
	transport.mu.Lock()
	for i := len(entries) - 1; i >= 0; i-- {
		entries[i].queuedAt = time.Now()
		transport.actions = append(transport.actions, entries[i])
	}
	transport.mu.Unlock()
	for i, result := range done {
		if err := <-result; err != wantErrors[i] {
			t.Fatal(err)
		}
	}

	events := readActivityLines(t, output.String())
	conflicts := 0
	for _, event := range events {
		if event.Operation == "reader_broker_admission" {
			conflicts++
			if event.RequestID != mismatch.RequestID || event.Action != "" || event.Phase != "not_admitted" || event.Outcome != "rejected" {
				t.Fatal(event)
			}
		}
	}
	if conflicts != 1 {
		t.Fatal("registration conflict count", conflicts)
	}
	rows, err := s.engine.ReadSplitActionAudit(ctx, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	for i, req := range requests {
		id := entries[i].action.ID
		wantOutcome := "accepted"
		if wantErrors[i] != nil {
			wantOutcome = "rejected"
		}
		var phases []string
		for _, e := range events {
			if e.RequestID != req.RequestID || e.Operation != "reader_broker" {
				continue
			}
			phases = append(phases, e.Phase)
			if e.Sequence != len(phases) || e.Producer != "sidecar" || e.Elapsed == nil {
				t.Fatal(e)
			}
			if e.Phase == "received" {
				if e.Action != "" || e.QueueWait != nil {
					t.Fatal(e)
				}
			} else if e.Action != id {
				t.Fatal("wrong join", e)
			}
			if (e.Phase == "attached" || e.Phase == "terminal") && (e.QueueWait == nil || *e.QueueWait < 0) {
				t.Fatal(e)
			}
			if e.Phase == "activation" && (e.Verified == nil || *e.Verified != (wantErrors[i] == nil)) {
				t.Fatal(e)
			}
			if e.Phase == "terminal" && e.Outcome != wantOutcome {
				t.Fatal(e)
			}
		}
		if strings.Join(phases, ",") != "received,attached,activation_started,helper_reply,activation,terminal" {
			t.Fatal(phases)
		}
		seen := map[string]int{}
		for _, row := range rows {
			if row.ActionID == id {
				seen[row.Phase+"/"+row.Outcome]++
			}
		}
		if len(seen) != 3 || seen["reader_broker_attached/accepted"] != 1 || seen["reader_foreground/pending"] != 1 || seen["reader_foreground/"+wantOutcome] != 1 {
			t.Fatal(seen)
		}
	}
	for _, private := range []string{"PRIVATE_TICKET", verifyFailure.Error(), requests[0].URL, requests[1].URL} {
		if strings.Contains(output.String(), private) {
			t.Fatal("private field in log")
		}
	}
}
