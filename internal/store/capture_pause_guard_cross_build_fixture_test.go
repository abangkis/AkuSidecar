//go:build auth_session_fixture || auth_session_dev

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
	case "dev-pause":
		seedCapturePauseQADatabase(t, s)
		if err := s.PauseFixtureCaptureSession(ctx, capturePauseMain); err != nil {
			t.Fatal(err)
		}
		capturePauseQAEnsureMarkerValue(t, s, capturePauseMain, "1")
		assertCapturePauseQAQueuedUnchanged(t, s)
	case "fixture-inspect":
		ids, err := s.ListFixturePausedCaptureSessions(ctx)
		if err != nil || len(ids) != 1 || ids[0] != capturePauseMain {
			t.Fatalf("persisted pause list=%v err=%v", ids, err)
		}
		paused, err := s.FixtureCaptureSessionPaused(ctx, capturePauseMain)
		if err != nil || !paused {
			t.Fatalf("persisted paused state=%t err=%v", paused, err)
		}
		assertCapturePauseQAQueuedUnchanged(t, s)
	case "fixture-resume":
		capturePauseQAUnknownMarkerSession(t, s)
		mainBefore := capturePauseQASnapshots(t, s, "cross-build-command-one", "cross-build-command-two")
		unknownBefore := capturePauseQASnapshots(t, s, "future-marker-command")
		if err := s.ResumeFixtureCaptureSession(ctx, "future-marker-session", "must-not-unpause", 50); err == nil {
			t.Fatal("resume accepted an unrecognized marker value")
		}
		if err := s.PauseFixtureCaptureSession(ctx, "future-marker-session"); err == nil {
			t.Fatal("pause rewrote an unrecognized marker value")
		}
		capturePauseQAEnsureMarkerValue(t, s, "future-marker-session", "future-v2")
		assertCapturePauseQASnapshotsEqual(t, s, unknownBefore)
		capturePauseQAEnsureMarkerValue(t, s, capturePauseMain, "1")
		for _, invalid := range []struct {
			epoch      string
			generation uint64
		}{
			{epoch: "", generation: 41},
			{epoch: "invalid-generation", generation: 0},
		} {
			if err := s.ResumeFixtureCaptureSession(ctx, capturePauseMain, invalid.epoch, invalid.generation); err == nil {
				t.Fatalf("invalid resume accepted: %+v", invalid)
			}
			assertCapturePauseQASnapshotsEqual(t, s, mainBefore)
			capturePauseQAEnsureMarkerValue(t, s, capturePauseMain, "1")
		}
		if err := s.ResumeFixtureCaptureSession(ctx, capturePauseMain, "fixture-owner-next", 41); err != nil {
			t.Fatal(err)
		}
		if capturePauseQAMarkerExists(t, s, capturePauseMain) {
			t.Fatal("valid fixture resume retained the marker")
		}
		assertCapturePauseQAResumed(t, s, "cross-build-command-one", "keep-one", 41)
		assertCapturePauseQAResumed(t, s, "cross-build-command-two", "keep-two", 41)
		resumed := capturePauseQASnapshots(t, s, "cross-build-command-one", "cross-build-command-two")
		if err := s.ResumeFixtureCaptureSession(ctx, capturePauseMain, "duplicate-resume", 99); err == nil {
			t.Fatal("duplicate resume accepted")
		}
		assertCapturePauseQASnapshotsEqual(t, s, resumed)

		insertCapturePauseGuardSession(t, s, "cross-build-cancelled", "cross-build-cancelled-run", "cross-build-cancelled-command", `{"captureCollector":{"version":1,"backend":"headless"},"captureRuntime":{"driver":"headless","epoch":"cancel-old","generation":3},"continuation":{"cursor":"cancel-me"}}`)
		if err := s.PauseFixtureCaptureSession(ctx, "cross-build-cancelled"); err != nil {
			t.Fatal(err)
		}
		if err := s.CancelSession(ctx, "cross-build-cancelled"); err != nil {
			t.Fatal(err)
		}
		cancelled := capturePauseQASnapshots(t, s, "cross-build-cancelled-command")
		if cancelled["cross-build-cancelled-command"].status != "cancelled" {
			t.Fatalf("cancelled command status=%+v", cancelled)
		}
		if err := s.ResumeFixtureCaptureSession(ctx, "cross-build-cancelled", "must-not-revive", 4); err != nil {
			t.Fatal(err)
		}
		assertCapturePauseQASnapshotsEqual(t, s, cancelled)
		if capturePauseQAMarkerExists(t, s, "cross-build-cancelled") {
			t.Fatal("terminal cancellation left a stale pause marker")
		}
	default:
		t.Fatalf("stage %q is not supported by a fixture-enabled build", stage)
	}
}
