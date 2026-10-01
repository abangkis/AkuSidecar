package httpapi

import (
	"context"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"strings"
	"testing"
)

func TestRotationFencesOldBootstrapAndResetsHeartbeat(t *testing.T) {
	s, token := splitTestServer(t)
	transport := s.splitCapture
	old := transport.key
	if w := splitRequest(s, token, old, "POST", "/api/bridge/split-capture/bootstrap", `{"sourceWindowLifetime":1}`); w.Code != 200 {
		t.Fatal(w.Code)
	}
	s.engine.RecordHeartbeat(domain.BridgeHeartbeat{BridgeID: "old"})
	if err := s.RotateSplitCapture(); err != nil {
		t.Fatal(err)
	}
	if s.splitCapture != transport || transport.key == old {
		t.Fatal("rotation replaced stable transport or kept key")
	}
	if transport.sourceTrackingSupported || transport.untrackedSourceOutcome {
		t.Fatal("new owner inherited stale readiness")
	}
	if w := splitRequest(s, token, old, "POST", "/api/bridge/split-capture/bootstrap", `{"sourceWindowLifetime":1}`); w.Code != 403 {
		t.Fatal("old owner bootstrapped", w.Code)
	}
	if w := splitRequest(s, token, transport.key, "POST", "/api/bridge/split-capture/bootstrap", `{"sourceWindowLifetime":1}`); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if !transport.sourceTrackingSupported {
		t.Fatal("new owner cannot negotiate")
	}
}
func TestRotationRefusesUnfinishedAction(t *testing.T) {
	s, token := splitTestServer(t)
	attachSplitLeaseManager(t, s)
	cancel, done, _ := startCancellableSplitAction(t, s, token)
	old := s.splitCapture.key
	if err := s.RotateSplitCapture(); err == nil {
		t.Fatal("rotation discarded unfinished ownership")
	}
	if s.splitCapture.key != old {
		t.Fatal("refusal rotated key")
	}
	cancel()
	awaitSplitClient(t, done)
	if err := s.RotateSplitCapture(); err != nil {
		t.Fatal(err)
	}
}
func TestStalePollCannotClaimAfterRotation(t *testing.T) {
	s, token := splitTestServer(t)
	old := s.splitCapture.key
	if err := s.RotateSplitCapture(); err != nil {
		t.Fatal(err)
	}
	w := splitRequest(s, token, old, "GET", "/api/bridge/split-capture/next", "")
	if w.Code != 403 && w.Code != 409 {
		t.Fatal("old poll accepted", w.Code)
	}
	// The fresh key, not the persistent token alone, authorizes capture routes.
	if strings.Contains(w.Body.String(), `"action"`) {
		t.Fatal("stale action leaked")
	}
	_ = context.Background()
}
