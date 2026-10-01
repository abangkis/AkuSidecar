package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestSplitQuietDispatchSchedulesInternalConsumerOnly(t *testing.T) {
	s, token := splitTestServer(t)
	ctx := context.Background()
	settings, _ := s.store.GetSettings(ctx)
	session, err := s.store.CreateUpdateSession(ctx, "quiet transport", settings, domain.UpdatePolicy{Trigger: domain.UpdateTriggerUser, Delivery: domain.UpdateDeliveryVisible, BudgetAuthority: domain.BudgetAuthorityUser})
	if err != nil {
		t.Fatal(err)
	}
	run := session.Runs[0]
	if _, err := s.store.StartRun(ctx, run.ID, map[string]any{"captureCollector": map[string]any{"backend": "browser_quiet_hidden", "version": 1}}); err != nil {
		t.Fatal(err)
	}
	response := splitRequest(s, token, s.splitCapture.key, "POST", "/api/split-capture/actions", fmt.Sprintf(`{"type":"dispatch","runId":%q}`, run.ID))
	if response.Code != 200 {
		t.Fatalf("quiet dispatch=%d %s", response.Code, response.Body)
	}
	var result splitActionResult
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || !result.OK {
		t.Fatalf("quiet scheduling ACK=%+v %v", result, err)
	}
	if len(s.splitCapture.actions) != 0 {
		t.Fatal("quiet dispatch queued duplicate Bridge action")
	}
	response = splitRequest(s, token, s.splitCapture.key, "GET", "/api/bridge/commands/next?runId="+run.ID, "")
	if response.Code != 204 {
		t.Fatalf("Bridge exposed quiet command=%d %s", response.Code, response.Body)
	}
	stored, err := s.store.GetRun(ctx, run.ID)
	if err != nil || stored.BridgeCommandStatus != "queued" {
		t.Fatalf("Bridge claim changed quiet queue=%+v %v", stored, err)
	}
	// Browser configuration must still reach Bridge even while Quiet work exists.
	done := make(chan int, 1)
	go func() {
		done <- splitRequest(s, token, s.splitCapture.key, "POST", "/api/split-capture/actions", `{"type":"configure_background"}`).Code
	}()
	claim := splitRequest(s, token, s.splitCapture.key, "GET", "/api/bridge/split-capture/next", "")
	if claim.Code != 200 {
		t.Fatalf("configuration claim=%d %s", claim.Code, claim.Body)
	}
	var payload struct {
		Action splitCaptureAction `json:"action"`
	}
	if err := json.Unmarshal(claim.Body.Bytes(), &payload); err != nil || payload.Action.Type != "configure_background" {
		t.Fatalf("configuration bypassed Bridge: %+v %v", payload, err)
	}
	if response := splitRequest(s, token, s.splitCapture.key, "POST", "/api/bridge/split-capture/results/"+payload.Action.ID, `{"ok":true,"result":{}}`); response.Code != 204 {
		t.Fatalf("configuration result=%d %s", response.Code, response.Body)
	}
	select {
	case code := <-done:
		if code != 200 {
			t.Fatalf("configuration ACK=%d", code)
		}
	case <-time.After(time.Second):
		t.Fatal("configuration action did not drain")
	}
}
