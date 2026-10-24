package engine

import (
	"context"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/collection"
	"github.com/abangkis/AkuSidecar/internal/collection/headless"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
)

func TestBatchBehaviorChallengeStopsXUntilExplicitRecovery(t *testing.T) {
	ctx := context.Background()
	e, s := singleSourceEngine(t, reasoning.Deterministic{})
	session, err := e.StartVisibleUpdate(ctx, "synthetic challenge")
	if err != nil {
		t.Fatal(err)
	}
	run := session.Runs[0]
	command, err := e.ClaimCommand(ctx, run.ID, "fixture")
	if err != nil || command == nil {
		t.Fatalf("claim: %v", err)
	}
	work, done, err := e.beginCaptureBehavior(ctx, domain.SourceX)
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	other, otherDone, err := e.beginCaptureBehavior(ctx, domain.SourceFacebook)
	if err != nil {
		t.Fatal(err)
	}
	defer otherDone()
	failed, err := e.FailCommand(ctx, command.ID, run.ID, internalCollectorFailure(&headless.CaptureError{Code: "challenge_required", Message: "raw secret must not persist"}, collection.BackendHeadless))
	if err != nil || failed.Error == nil || failed.Error.Retryable || failed.Error.Message != domain.XChallengeMessage {
		t.Fatalf("failed=%+v err=%v", failed, err)
	}
	if work.Err() == nil || other.Err() != nil {
		t.Fatal("wrong cancellation scope")
	}
	if _, _, err = e.beginCaptureBehavior(ctx, domain.SourceX); err == nil {
		t.Fatal("X admitted while paused")
	}
	if _, err = e.StartVisibleUpdate(ctx, "blocked manual retry"); err == nil {
		t.Fatal("manual capture bypassed pause")
	}
	status, err := e.AutoUpdateStatus(ctx)
	if err != nil || status.SourceCapturePause == nil {
		t.Fatalf("missing UI projection: %v", err)
	}
	if err = e.ConfirmXCaptureRecovery(ctx, status.SourceCapturePause.ID); err != nil {
		t.Fatal(err)
	}
	if pause, _ := s.SourceCapturePause(ctx); pause != nil {
		t.Fatal("explicit recovery did not clear")
	}
	if next, err := e.StartVisibleUpdate(ctx, "new explicit attempt"); err != nil {
		t.Fatal(err)
	} else {
		_ = e.CancelSession(ctx, next.ID)
	}
}

func TestBatchBehaviorCollectorConversionPreservesChallengeAuthority(t *testing.T) {
	for _, collector := range []string{collection.BackendHeadless, collection.BackendQuiet} {
		for _, code := range []string{"challenge_required", "challenge_detected"} {
			failure := internalCollectorFailure(&headless.CaptureError{Code: code, Message: "safe fixture"}, collector)
			if !domain.IsAccessChallenge(failure.Code) || failure.Retryable {
				t.Fatalf("%s lost challenge authority: %+v", collector, failure)
			}
		}
	}
}
