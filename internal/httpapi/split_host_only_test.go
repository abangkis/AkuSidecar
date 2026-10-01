package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
)

func TestHostOnlyRetirementRequiresCurrentCapabilityAcrossRotation(t *testing.T) {
	s, token := splitTestServer(t)
	ctx := context.Background()
	if err := s.RequireHostOnlyRetirement(); err != nil {
		t.Fatal(err)
	}
	s.SetSplitSourceWindowPreparation(func(context.Context, string) error { return nil })
	bootstrap := func(body string) {
		t.Helper()
		if got := splitRequest(s, token, s.splitCapture.key, "POST", "/api/bridge/split-capture/bootstrap", body).Code; got != 200 {
			t.Fatalf("bootstrap=%d", got)
		}
	}
	bootstrap(`{"sourceWindowLifetime":1,"captureHostClose":1}`)
	if err := s.SplitCaptureReplacementReadiness(ctx); err == nil || !strings.Contains(err.Error(), "host-only") {
		t.Fatalf("old Bridge readiness=%v", err)
	}
	bootstrap(`{"sourceWindowLifetime":1,"captureHostClose":1,"captureHostOnlyRetirement":2}`)
	if err := s.SplitCaptureReplacementReadiness(ctx); err == nil {
		t.Fatal("unknown host-only version admitted")
	}
	bootstrap(`{"sourceWindowLifetime":1,"captureHostClose":1,"captureHostOnlyRetirement":1}`)
	if err := s.SplitCaptureReplacementReadiness(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.RotateSplitCapture(); err != nil {
		t.Fatal(err)
	}
	s.SetSplitSourceWindowPreparation(func(context.Context, string) error { return nil })
	bootstrap(`{"sourceWindowLifetime":1,"captureHostClose":1}`)
	if err := s.SplitCaptureReplacementReadiness(ctx); err == nil || !strings.Contains(err.Error(), "host-only") {
		t.Fatalf("rotation forgot required policy=%v", err)
	}
}

func TestHostOnlyRetirementActionIsNegotiatedAndAtMostOnce(t *testing.T) {
	s, token := splitTestServer(t)
	p := &handoffTransportProcess{splitLeaseProcess: splitLeaseProcess{done: make(chan error, 1)}, entered: make(chan struct{}), release: make(chan struct{})}
	owner, err := captureruntime.New(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetSplitCaptureRuntime(owner); err != nil {
		t.Fatal(err)
	}
	if err := s.RequireHostOnlyRetirement(); err != nil {
		t.Fatal(err)
	}
	bootstrap := func(body string) {
		t.Helper()
		if got := splitRequest(s, token, s.splitCapture.key, "POST", "/api/bridge/split-capture/bootstrap", body).Code; got != 200 {
			t.Fatal(got)
		}
	}
	bootstrap(`{"sourceWindowLifetime":1,"captureHostClose":1}`)
	if err := s.CloseSplitCaptureHost(context.Background()); err == nil || !strings.Contains(err.Error(), "host-only") {
		t.Fatalf("old worker admitted retirement=%v", err)
	}
	if len(s.splitCapture.actions) != 0 {
		t.Fatal("unnegotiated retirement queued")
	}
	bootstrap(`{"sourceWindowLifetime":1,"captureHostClose":1,"captureHostOnlyRetirement":1}`)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	replaced := make(chan error, 1)
	go func() {
		replaced <- owner.Replace(ctx, func(context.Context, uint64) (captureruntime.Process, error) {
			return &splitLeaseProcess{done: make(chan error, 1)}, nil
		})
	}()
	select {
	case <-p.entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	retireCtx, cancelRetirement := context.WithCancel(ctx)
	result := make(chan error, 1)
	go func() { result <- s.CloseSplitCaptureHost(retireCtx) }()
	response := splitRequest(s, token, s.splitCapture.key, "GET", "/api/bridge/split-capture/next", "")
	var payload struct {
		Action   splitCaptureAction `json:"action"`
		HostOnly bool               `json:"captureHostOnlyRetirement"`
	}
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &payload) != nil || !payload.Action.HostOnly || !payload.HostOnly {
		t.Fatalf("host-only action=%d %s", response.Code, response.Body)
	}
	cancelRetirement()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if got := splitRequest(s, token, s.splitCapture.key, "POST", "/api/bridge/split-capture/results/"+payload.Action.ID, `{"ok":true}`).Code; got != 204 {
		t.Fatal(got)
	}
	if err := s.CloseSplitCaptureHost(ctx); err != nil {
		t.Fatal(err)
	}
	if len(s.splitCapture.actions) != 1 {
		t.Fatal("retirement replayed")
	}
	close(p.release)
	if err := <-replaced; err != nil {
		t.Fatal(err)
	}
}
