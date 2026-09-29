package httpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/readerbroker"
)

func TestSplitHostWakeCapabilityAndNoClaims(t *testing.T) {
	s, _ := splitTestServer(t)
	for _, key := range []string{"", "stale"} {
		w := splitRequest(s, "", key, "GET", "/api/bridge/split-capture/wake", "")
		if w.Code != 403 {
			t.Fatalf("unauthorized wake status=%d", w.Code)
		}
	}
	tpt := s.splitCapture
	entry := &pendingSplitAction{action: splitCaptureAction{ID: "split_wake", Type: "open_native_post", URL: "https://x.com/private/status/123"}}
	tpt.actions = []*pendingSplitAction{entry}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := s.routeSplitCapture(w, r, r.URL.Path); err != nil {
			s.writeError(w, err)
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	r, _ := http.NewRequestWithContext(ctx, "GET", server.URL+"/api/bridge/split-capture/wake", nil)
	r.Header.Set("X-Aku-Capture-Instance", tpt.key)
	response, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	reader := bufio.NewReader(response.Body)
	readHint := func() {
		t.Helper()
		line, err := reader.ReadString('\n')
		if err != nil || line != "wake\n" {
			t.Fatalf("hint=%q err=%v", line, err)
		}
	}
	readHint()
	// Draining the worker poll notification must not steal the host notification.
	tpt.notifyCapture()
	<-tpt.wake
	readHint()
	tpt.mu.Lock()
	claimed := entry.claimed
	tpt.mu.Unlock()
	if claimed {
		t.Fatal("wake stream claimed an action")
	}
	tpt.close()
	if _, err := reader.ReadByte(); err == nil {
		t.Fatal("closed transport retained stream")
	}
}

func TestSplitNativeClickWakesIdleHostBeforeClaim(t *testing.T) {
	s, token := splitTestServer(t)
	tpt := s.splitCapture
	s.SetSplitReaderBroker(func(context.Context, string) (readerbroker.Target, func(context.Context) error, error) {
		t.Fatal("wake or claim attempted foreground preparation")
		return readerbroker.Target{}, nil, nil
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := s.routeSplitCapture(w, r, r.URL.Path); err != nil {
			s.writeError(w, err)
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	r, _ := http.NewRequestWithContext(ctx, "GET", server.URL+"/api/bridge/split-capture/wake", nil)
	r.Header.Set("X-Aku-Capture-Instance", tpt.key)
	response, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	reader := bufio.NewReader(response.Body)
	if _, err := reader.ReadString('\n'); err != nil {
		t.Fatal(err)
	}
	// No /next request exists until the explicit click produces a network hint.
	requestID := "broker_" + strings.Repeat("c", 32)
	uiDone := make(chan int, 1)
	go func() {
		w := splitRequest(s, token, tpt.key, "POST", "/api/split-capture/actions", `{"type":"open_native_post","source":"x","url":"https://x.com/a/status/1","requestId":"`+requestID+`"}`)
		uiDone <- w.Code
	}()
	if hint, err := reader.ReadString('\n'); err != nil || hint != "wake\n" {
		t.Fatalf("click wake=%q err=%v", hint, err)
	}
	brokerDone := make(chan error, 1)
	go func() {
		brokerDone <- s.HandleReaderBroker(ctx, readerbroker.Request{RequestID: requestID, Source: "x", URL: "https://x.com/a/status/1"}, func(readerbroker.Target) (readerbroker.Reply, error) {
			t.Error("claim attempted foreground activation")
			return readerbroker.Reply{}, nil
		})
	}()
	if hint, err := reader.ReadString('\n'); err != nil || hint != "wake\n" {
		t.Fatalf("broker wake=%q err=%v", hint, err)
	}
	claim := splitRequest(s, token, tpt.key, "GET", "/api/bridge/split-capture/next", "")
	var payload struct {
		Action splitCaptureAction `json:"action"`
	}
	if claim.Code != 200 {
		t.Fatalf("claim status=%d", claim.Code)
	}
	if err := json.Unmarshal(claim.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Action.RequestID != requestID {
		t.Fatal("wrong native click claimed")
	}
	result := splitRequest(s, token, tpt.key, "POST", "/api/bridge/split-capture/results/"+payload.Action.ID, `{"ok":false,"message":"test stopped after claim"}`)
	if result.Code != 204 {
		t.Fatalf("result status=%d", result.Code)
	}
	select {
	case status := <-uiDone:
		if status != 200 {
			t.Fatalf("UI status=%d", status)
		}
	case <-ctx.Done():
		t.Fatal("click did not finish within two seconds")
	}
	cancel()
	<-brokerDone
}

func TestSplitHostWakeBoundedAndClosed(t *testing.T) {
	for _, closed := range []bool{false, true} {
		tpt := newSplitCaptureTransport()
		tpt.closed = closed
		tpt.hostWakeStreams = 4
		r := httptest.NewRequest("GET", "/api/bridge/split-capture/wake", nil)
		r.Header.Set("X-Aku-Capture-Instance", tpt.key)
		err := tpt.serveHostWake(httptest.NewRecorder(), r)
		if value, ok := err.(apiError); !ok || value.Status != 503 {
			t.Fatalf("closed=%v error=%v", closed, err)
		}
	}
}
