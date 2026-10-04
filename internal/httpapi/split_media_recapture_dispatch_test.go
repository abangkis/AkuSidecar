package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
)

func TestBrowserRecaptureDispatchKeepsClaimAndAcceptsLateMatchingResult(t *testing.T) {
	s, token := splitTestServer(t)
	owner, err := captureruntime.New(&splitLeaseProcess{done: make(chan error, 1)})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetSplitCaptureRuntime(owner); err != nil {
		t.Fatal(err)
	}
	generation := owner.Snapshot().Generation
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- s.DispatchBrowserMediaRecapture(ctx, "recapture_fixture", generation) }()
	response := splitRequest(s, token, s.splitCapture.key, "GET", "/api/bridge/split-capture/next", "")
	var payload struct {
		Action splitCaptureAction `json:"action"`
	}
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &payload) != nil || payload.Action.Type != "media_recapture" || payload.Action.RecaptureID != "recapture_fixture" {
		t.Fatalf("claim: %d %s", response.Code, response.Body.String())
	}
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled wait: %v", err)
	}
	if owner.Snapshot().ActiveLeases != 1 {
		t.Fatal("claimed dispatch did not retain Browser ownership")
	}
	if got := splitRequest(s, token, s.splitCapture.key, "POST", "/api/bridge/split-capture/results/"+payload.Action.ID,
		`{"ok":true,"result":{"recapture":{"id":"recapture_fixture","status":"completed"}}}`).Code; got != 204 {
		t.Fatalf("late acknowledgement: %d", got)
	}
	if err := s.DispatchBrowserMediaRecapture(context.Background(), "recapture_fixture", generation); err != nil {
		t.Fatal(err)
	}
	if len(s.splitCapture.actions) != 1 || owner.Snapshot().ActiveLeases != 0 {
		t.Fatal("late result replayed or retained its action lease")
	}
	if err := s.DispatchBrowserMediaRecapture(context.Background(), "recapture_fixture", generation+1); err == nil {
		t.Fatal("receipt accepted by another owner generation")
	}
}

func TestBrowserRecaptureDispatchRejectsMismatchedAndUnfinishedJobs(t *testing.T) {
	for _, raw := range []string{`{}`, `{"recapture":{"id":"other","status":"completed"}}`, `{"recapture":{"id":"recapture_fixture","status":"queued"}}`, `{"recapture":{"id":"recapture_fixture","status":"claimed"}}`} {
		if err := browserRecaptureDispatchResult("recapture_fixture", &splitActionResult{OK: true, Result: json.RawMessage(raw)}); err == nil {
			t.Errorf("unverified job accepted: %s", raw)
		}
	}
}
