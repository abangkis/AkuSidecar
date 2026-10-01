package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
)

type handoffTransportProcess struct {
	splitLeaseProcess
	entered chan struct{}
	release chan struct{}
}

func (p *handoffTransportProcess) CloseForRetry(ctx context.Context) error {
	close(p.entered)
	select {
	case <-p.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestCaptureHostRetirementNegotiatedInternalAndAtMostOnce(t *testing.T) {
	s, token := splitTestServer(t)
	p := &handoffTransportProcess{splitLeaseProcess: splitLeaseProcess{done: make(chan error, 1)}, entered: make(chan struct{}), release: make(chan struct{})}
	owner, err := captureruntime.New(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetSplitCaptureRuntime(owner); err != nil {
		t.Fatal(err)
	}
	if err := validateSplitAction(splitCaptureAction{Type: "close_capture_host"}); err == nil {
		t.Fatal("retirement must not be exposed through UI actions")
	}
	if err := s.CloseSplitCaptureHost(context.Background()); err == nil {
		t.Fatal("unnegotiated host accepted")
	}
	if got := splitRequest(s, token, s.splitCapture.key, "POST", "/api/bridge/split-capture/bootstrap", `{"sourceWindowLifetime":1,"captureHostClose":1}`).Code; got != 200 {
		t.Fatalf("bootstrap: %d", got)
	}
	if err := s.CloseSplitCaptureHost(context.Background()); !errors.Is(err, captureruntime.ErrBusy) {
		t.Fatalf("ready owner: %v", err)
	}
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
	retireCtx, retireCancel := context.WithCancel(ctx)
	result := make(chan error, 1)
	go func() { result <- s.CloseSplitCaptureHost(retireCtx) }()
	response := splitRequest(s, token, s.splitCapture.key, "GET", "/api/bridge/split-capture/next", "")
	var payload struct {
		Action splitCaptureAction `json:"action"`
	}
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &payload) != nil || payload.Action.Type != "close_capture_host" {
		t.Fatalf("claim: %d %s", response.Code, response.Body.String())
	}
	retireCancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	s.splitCapture.mu.Lock()
	if len(s.splitCapture.actions) != 1 || !s.splitCapture.actions[0].claimed || s.splitCapture.actions[0].runtimeLease != nil {
		t.Fatal("claimed retirement must remain without an admission lease")
	}
	s.splitCapture.mu.Unlock()
	if got := splitRequest(s, token, s.splitCapture.key, "POST", "/api/bridge/split-capture/results/"+payload.Action.ID, `{"ok":true}`).Code; got != 204 {
		t.Fatalf("late ACK: %d", got)
	}
	if err := s.CloseSplitCaptureHost(ctx); err != nil {
		t.Fatalf("completed retry: %v", err)
	}
	if len(s.splitCapture.actions) != 1 {
		t.Fatal("retirement replayed")
	}
	s.splitCapture.mu.Lock()
	s.splitCapture.actions[0].completionResult = &splitActionResult{OK: false}
	s.splitCapture.mu.Unlock()
	if err := s.CloseSplitCaptureHost(ctx); err == nil {
		t.Fatal("a completed rejection must not become a successful retirement on retry")
	}
	close(p.release)
	if err := <-replaced; err != nil {
		t.Fatal(err)
	}
}
