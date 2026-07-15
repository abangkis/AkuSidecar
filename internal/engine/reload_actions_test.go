package engine

import (
	"errors"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestReloadActionCompletesOnlyOnExpectedBuild(t *testing.T) {
	actions := NewReloadActions(time.Second)
	actor := map[string]any{"kind": "supervisor", "instance": "test"}
	requested, err := actions.Request("request-1", actor, "apply AkuBridge v2", "old-build")
	if err != nil {
		t.Fatal(err)
	}
	if requested.Status != "pending" || requested.ExpectedBuildID != ExpectedBridgeBuildID {
		t.Fatalf("requested=%+v", requested)
	}

	delivered, err := actions.Next(0, make(chan struct{}))
	if err != nil || delivered == nil || delivered.ID != requested.ID || delivered.Status != "delivered" {
		t.Fatalf("delivered=%+v err=%v", delivered, err)
	}
	accepted, err := actions.Accept(requested.ID)
	if err != nil || accepted.Status != "accepted" {
		t.Fatalf("accepted=%+v err=%v", accepted, err)
	}

	mismatch := actions.Observe(domain.BridgeHeartbeat{BuildID: "unexpected-build"})
	if mismatch == nil || mismatch.Status != "accepted" || mismatch.ObservedBuildID != "unexpected-build" {
		t.Fatalf("mismatch=%+v", mismatch)
	}
	completed := actions.Observe(domain.BridgeHeartbeat{BuildID: ExpectedBridgeBuildID})
	if completed == nil || completed.Status != "completed" || completed.CompletedAt == nil {
		t.Fatalf("completed=%+v", completed)
	}
}

func TestReloadActionReplayRequiresSameActorAndReason(t *testing.T) {
	actions := NewReloadActions(time.Second)
	actor := map[string]any{"kind": "supervisor"}
	first, err := actions.Request("request-1", actor, "reload", "old-build")
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := actions.Request("request-1", actor, "reload", "ignored-build")
	if err != nil || replayed.ID != first.ID {
		t.Fatalf("replayed=%+v err=%v", replayed, err)
	}
	if _, err := actions.Request("request-1", map[string]any{"kind": "other"}, "reload", "old-build"); !errors.Is(err, ErrActionConflict) {
		t.Fatalf("actor conflict err=%v", err)
	}
	if _, err := actions.Request("request-1", actor, "different", "old-build"); !errors.Is(err, ErrActionConflict) {
		t.Fatalf("reason conflict err=%v", err)
	}
	if _, err := actions.Request("request-2", actor, "reload", "old-build"); !errors.Is(err, ErrActionConflict) {
		t.Fatalf("single-flight err=%v", err)
	}
}
