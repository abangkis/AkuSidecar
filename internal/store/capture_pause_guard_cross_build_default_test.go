//go:build !auth_session_fixture && !auth_session_dev

package store

import (
	"context"
	"testing"
)

func runCapturePauseGuardStage(t *testing.T, root, stage string) {
	t.Helper()
	s := openCapturePauseQAStore(t, root)
	ctx := context.Background()
	switch stage {
	case "default-block":
		capturePauseQAEnsureMarkerValue(t, s, capturePauseMain, "1")
		assertCapturePauseQAQueuedUnchanged(t, s)
		before := capturePauseQASnapshots(t, s, "cross-build-command-one", "cross-build-command-two")
		for _, claim := range []struct {
			runID     string
			collector string
		}{
			{runID: "cross-build-run-one"},
			{runID: "cross-build-run-two", collector: "headless"},
		} {
			var commandID string
			var err error
			if claim.collector == "" {
				claimed, claimErr := s.ClaimCommand(ctx, claim.runID, "should-remain-queued")
				err = claimErr
				if claimed != nil {
					commandID = claimed.ID
				}
			} else {
				claimed, claimErr := s.ClaimCommandForCollector(ctx, claim.runID, "should-remain-queued", claim.collector)
				err = claimErr
				if claimed != nil {
					commandID = claimed.ID
				}
			}
			if err != nil || commandID != "" {
				t.Fatalf("paused run %s claimed command %q err=%v", claim.runID, commandID, err)
			}
		}
		assertCapturePauseQASnapshotsEqual(t, s, before)
		eligible, err := s.ClaimCommandForCollector(ctx, "cross-build-open-run", "eligible-default", "bridge")
		if err != nil || eligible == nil || eligible.ID != "cross-build-open-command" || eligible.Status != "claimed" {
			t.Fatalf("unmarked session was not eligible: command=%+v err=%v", eligible, err)
		}
		capturePauseQAEnsureMarkerValue(t, s, capturePauseMain, "1")
	case "default-claim":
		if capturePauseQAMarkerExists(t, s, capturePauseMain) {
			t.Fatal("verified resume left the primary session paused")
		}
		assertCapturePauseQAResumed(t, s, "cross-build-command-one", "keep-one", 41)
		assertCapturePauseQAResumed(t, s, "cross-build-command-two", "keep-two", 41)
		command, err := s.ClaimCommandForCollector(ctx, "cross-build-run-one", "fixture-default-consumer", "headless")
		if err != nil || command == nil || command.ID != "cross-build-command-one" || command.Status != "claimed" {
			t.Fatalf("resumed command was not claimable: command=%+v err=%v", command, err)
		}
		stamp, ok := command.Payload["captureRuntime"].(map[string]any)
		if !ok || stamp["epoch"] != "fixture-owner-next" || stamp["generation"] != float64(41) {
			t.Fatalf("claimed payload lacks verified owner stamp: %+v", command.Payload)
		}
		remaining := loadCapturePauseCommandSnapshot(t, s, "cross-build-command-two")
		if remaining.status != "queued" || remaining.claimedAt.Valid || remaining.claimedBy.Valid {
			t.Fatalf("second resumed command changed during first claim: %+v", remaining)
		}
		if got := capturePauseQAMarker(t, s, "future-marker-session"); got != "future-v2" {
			t.Fatalf("unknown marker was changed: %q", got)
		}
		unknownBefore := loadCapturePauseCommandSnapshot(t, s, "future-marker-command")
		unknown, err := s.ClaimCommand(ctx, "future-marker-run", "must-stay-queued")
		if err != nil || unknown != nil {
			t.Fatalf("unknown marker admitted a claim: command=%+v err=%v", unknown, err)
		}
		if got := loadCapturePauseCommandSnapshot(t, s, "future-marker-command"); got != unknownBefore {
			t.Fatalf("unknown-marker command changed: got=%+v want=%+v", got, unknownBefore)
		}
	default:
		t.Fatalf("stage %q is not supported by the ordinary build", stage)
	}
}
