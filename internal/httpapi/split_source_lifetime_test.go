package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"
)

func TestSplitSourcePreparationIsClaimedTypedAndSingleUse(t *testing.T) {
	s, token := splitTestServer(t)
	calls := 0
	s.SetSplitSourceWindowPreparation(func(_ context.Context, marker string) error {
		calls++
		if marker != "AkuBrowser source split_source" {
			t.Fatalf("marker: %s", marker)
		}
		return nil
	})
	entry := &pendingSplitAction{action: splitCaptureAction{ID: "split_source", Type: "open_source"}}
	s.splitCapture.actions = append(s.splitCapture.actions, entry)
	path := "/api/bridge/split-capture/source/prepare/split_source"
	request := func() int { return splitRequest(s, token, s.splitCapture.key, "POST", path, "{}").Code }
	if got := request(); got != 404 {
		t.Fatalf("unclaimed: %d", got)
	}
	entry.claimed = true
	entry.action.Type = "ping"
	if got := request(); got != 404 {
		t.Fatalf("wrong type: %d", got)
	}
	entry.action.Type = "open_source"
	if got := splitRequest(s, token, "wrong", "POST", path, "{}").Code; got != 409 {
		t.Fatalf("instance: %d", got)
	}
	if got := request(); got != 200 {
		t.Fatalf("prepare: %d", got)
	}
	if got := request(); got != 409 {
		t.Fatalf("duplicate: %d", got)
	}
	if calls != 1 {
		t.Fatalf("calls: %d", calls)
	}
	entry.completed = true
	if got := request(); got != 404 {
		t.Fatalf("completed: %d", got)
	}
}

func TestSplitSourceNegotiationAndMarkerLifetime(t *testing.T) {
	for _, available := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy", true: "native"}[available], func(t *testing.T) {
			s, token := splitTestServer(t)
			if available {
				s.SetSplitSourceWindowPreparation(func(context.Context, string) error { return nil })
			}
			entry := &pendingSplitAction{action: splitCaptureAction{ID: "split_source", Type: "open_source"}}
			s.splitCapture.actions = append(s.splitCapture.actions, entry)
			marker := func() int {
				w := httptest.NewRecorder()
				s.serveSplitCaptureAsset(w, httptest.NewRequest("GET", "/split-source-intent?id=split_source", nil))
				return w.Code
			}
			if got := marker(); got != 404 {
				t.Fatalf("unclaimed marker: %d", got)
			}
			result := splitRequest(s, token, s.splitCapture.key, "GET", "/api/bridge/split-capture/next", "")
			var payload struct {
				SourceWindowLifetime bool `json:"sourceWindowLifetime"`
			}
			if result.Code != 200 {
				t.Fatalf("claim: %d", result.Code)
			}
			if err := json.Unmarshal(result.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if payload.SourceWindowLifetime != available {
				t.Fatalf("negotiation: %t", payload.SourceWindowLifetime)
			}
			if got := marker(); got != 200 {
				t.Fatalf("claimed marker: %d", got)
			}
			entry.completed = true
			if got := marker(); got != 404 {
				t.Fatalf("completed marker: %d", got)
			}
		})
	}
}

func TestSplitSourceFailedBindingCannotReplay(t *testing.T) {
	s, token := splitTestServer(t)
	calls := 0
	s.SetSplitSourceWindowPreparation(func(context.Context, string) error { calls++; return errors.New("unowned marker") })
	s.splitCapture.actions = append(s.splitCapture.actions, &pendingSplitAction{claimed: true, action: splitCaptureAction{ID: "split_failed", Type: "open_source"}})
	for i := 0; i < 2; i++ {
		if got := splitRequest(s, token, s.splitCapture.key, "POST", "/api/bridge/split-capture/source/prepare/split_failed", "{}").Code; got != 409 {
			t.Fatalf("rejected binding: %d", got)
		}
	}
	if calls != 1 {
		t.Fatalf("native callback replayed: %d", calls)
	}
}
