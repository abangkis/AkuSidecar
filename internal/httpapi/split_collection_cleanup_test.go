package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
)

func TestBrowserCollectionCleanupKeepsClaimThroughTimeoutAndLateAck(t *testing.T) {
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
	go func() { result <- s.ReleaseBrowserCollectionSurfaces(ctx, "session_fixture", generation) }()
	response := splitRequest(s, token, s.splitCapture.key, "GET", "/api/bridge/split-capture/next", "")
	var payload struct {
		Action splitCaptureAction `json:"action"`
	}
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &payload) != nil {
		t.Fatalf("claim: %d %s", response.Code, response.Body.String())
	}
	if payload.Action.Type != "release" || payload.Action.LeaseID != "session_fixture" || payload.Action.Source != "" {
		t.Fatalf("cleanup must release the whole exact session: %+v", payload.Action)
	}
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled wait: %v", err)
	}
	if owner.Snapshot().ActiveLeases != 1 {
		t.Fatal("claimed cleanup must pin its Browser generation until acknowledgement")
	}
	if got := splitRequest(s, token, s.splitCapture.key, "POST", "/api/bridge/split-capture/results/"+payload.Action.ID,
		`{"ok":true,"result":{"outcome":{"released":true,"mode":"owned_window_closed"}}}`).Code; got != 204 {
		t.Fatalf("late acknowledgement: %d", got)
	}
	if owner.Snapshot().ActiveLeases != 0 {
		t.Fatal("acknowledged cleanup lease did not drain")
	}
	if err := s.ReleaseBrowserCollectionSurfaces(context.Background(), "session_fixture", generation); err != nil {
		t.Fatalf("late acknowledgement readback: %v", err)
	}
	if len(s.splitCapture.actions) != 1 {
		t.Fatal("cleanup was replayed after acknowledgement")
	}
}

func TestBrowserCollectionCleanupRejectsWrongOwnerBeforeAdmission(t *testing.T) {
	s, _ := splitTestServer(t)
	owner, err := captureruntime.New(&splitLeaseProcess{done: make(chan error, 1)})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetSplitCaptureRuntime(owner); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.ReleaseBrowserCollectionSurfaces(ctx, "session_fixture", owner.Snapshot().Generation+1); err == nil {
		t.Fatal("cleanup accepted a different profile owner")
	}
	if len(s.splitCapture.actions) != 0 || owner.Snapshot().ActiveLeases != 0 {
		t.Fatal("rejected cleanup leaked ownership")
	}
}

func TestBrowserCollectionCleanupRequiresVerifiedLeaseOutcome(t *testing.T) {
	for _, raw := range []string{``, `null`, `{"released":true,"mode":"owned_window_closed"}`, `{"outcome":null}`, `{"outcome":{"reason":"lease_mismatch"}}`, `{"outcome":{"released":false,"reason":"surface_unverified"}}`, `{"outcome":{"released":false,"reason":"lease_mismatch"}}`, `{"outcome":{"released":true,"mode":"unsupported"}}`} {
		if err := browserCollectionCleanupResult(&splitActionResult{OK: true, Result: json.RawMessage(raw)}); err == nil {
			t.Errorf("unverified outcome accepted: %q", raw)
		}
	}
	for _, raw := range []string{`{"outcome":{"released":true,"mode":"owned_window_closed"}}`, `{"outcome":{"released":false,"reason":"no_owned_surface"}}`, `{"outcome":{"released":false,"mode":"owned_transient_tabs_closed"}}`} {
		if err := browserCollectionCleanupResult(&splitActionResult{OK: true, Result: json.RawMessage(raw)}); err != nil {
			t.Errorf("valid cleanup rejected: %s: %v", raw, err)
		}
	}
}
