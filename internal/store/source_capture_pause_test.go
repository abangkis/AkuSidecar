package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestSourceCapturePausePersistsAndRejectsOldWorkAfterRecovery(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "synthetic.db")
	settings := domain.DefaultSettings("expanded", "quiet", "guarded_live", true)
	settings.ActiveSources = []domain.Source{domain.SourceX, domain.SourceFacebook}
	s, err := Open(path, settings)
	if err != nil {
		t.Fatal(err)
	}
	session, err := createVisibleUpdateSession(s, ctx, "pause fixture", settings)
	if err != nil {
		t.Fatal(err)
	}
	x, other := session.Runs[0], session.Runs[1]
	command, err := s.StartRun(ctx, x.ID, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ClaimCommand(ctx, x.ID, "fixture"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.StartRun(ctx, other.ID, map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.FailCommand(ctx, command.ID, x.ID, domain.Failure{Code: "challenge_required", Message: "untrusted page text", Retryable: true, Details: map[string]any{"challengeCategory": "cloudflare", "secret": "must be discarded"}}); err != nil {
		t.Fatal(err)
	}
	pause, err := s.SourceCapturePause(ctx)
	if err != nil || pause == nil || pause.Message != domain.XChallengeMessage {
		t.Fatalf("pause=%+v err=%v", pause, err)
	}
	if pause.Category != "cloudflare" {
		t.Fatal("safe category lost")
	}
	stored, _ := s.GetRun(ctx, x.ID)
	if stored.Error == nil || len(stored.Error.Details) != 1 || stored.Error.Details["secret"] != nil {
		t.Fatal("untrusted details persisted")
	}
	if _, err = s.ClaimCommand(ctx, x.ID, "fixture"); !errors.Is(err, ErrSourceCapturePaused) {
		t.Fatalf("claim admitted: %v", err)
	}
	if c, err := s.ClaimCommand(ctx, other.ID, "fixture"); err != nil || c == nil {
		t.Fatalf("unrelated source blocked: %v", err)
	}
	if err = s.PauseXCapture(ctx); err != nil {
		t.Fatal(err)
	}
	if p, _ := s.SourceCapturePause(ctx); p.ID != pause.ID {
		t.Fatal("first pause replaced")
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path, settings)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if p, _ := s.SourceCapturePause(ctx); p == nil || p.ID != pause.ID {
		t.Fatal("restart cleared pause")
	}
	if err = s.ConfirmXCaptureRecovery(ctx, "old-ui"); err == nil {
		t.Fatal("stale UI cleared pause")
	}
	if err = s.ConfirmXCaptureRecovery(ctx, pause.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveObservation(ctx, command.ID, x.ID, domain.Observation{Source: domain.SourceX}); err == nil {
		t.Fatal("old command accepted after resume")
	}
	if _, err = s.FailCommand(ctx, command.ID, x.ID, domain.Failure{Code: "challenge_required"}); err == nil {
		t.Fatal("duplicate failure accepted")
	}
	if p, _ := s.SourceCapturePause(ctx); p != nil {
		t.Fatal("stale failure re-latched")
	}
}

func TestSourceCapturePauseBlocksFreshClaimsAndFollowups(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	settings, _ := s.GetSettings(ctx)
	settings.ActiveSources = []domain.Source{domain.SourceX}
	session, err := createVisibleUpdateSession(s, ctx, "queued fixture", settings)
	if err != nil {
		t.Fatal(err)
	}
	run := session.Runs[0]
	if err = s.PauseXCapture(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = s.StartRun(ctx, run.ID, nil); !errors.Is(err, ErrSourceCapturePaused) {
		t.Fatalf("new run admitted %v", err)
	}
	if _, err = s.QueueFollowUp(ctx, run.ID, nil); !errors.Is(err, ErrSourceCapturePaused) {
		t.Fatalf("followup admitted %v", err)
	}
}

func TestSourceCapturePauseRecaptureFailureIsAtomicAndScoped(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	timelineID, _ := insertUnavailableMediaFixture(t, s)
	job, err := s.CreateMediaRecapture(ctx, timelineID, domain.MediaRecaptureBackground)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ClaimMediaRecapture(ctx, job.ID, "fixture"); err != nil {
		t.Fatal(err)
	}
	failed, err := s.FailMediaRecapture(ctx, job.ID, domain.Failure{Code: "challenge_required", Message: "private page", Details: map[string]any{"challengeCategory": "x_human_verification", "private": "discard"}, Retryable: true})
	if err != nil || failed.Error == nil || failed.Error.Retryable || failed.Error.Message != domain.XChallengeMessage {
		t.Fatalf("failure=%+v err=%v", failed, err)
	}
	pause, err := s.SourceCapturePause(ctx)
	if err != nil || pause == nil || pause.Category != "x_human_verification" {
		t.Fatalf("pause=%+v err=%v", pause, err)
	}
	if _, err = s.CompleteMediaRecapture(ctx, job.ID, domain.Observation{Source: domain.SourceX}); !errors.Is(err, ErrSourceCapturePaused) {
		t.Fatalf("late result admitted: %v", err)
	}
	if err = s.ConfirmXCaptureRecovery(ctx, pause.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CompleteMediaRecapture(ctx, job.ID, domain.Observation{Source: domain.SourceX}); err == nil {
		t.Fatal("late result revived after recovery")
	}
}

func TestSourceCapturePauseFollowupPreservesAcceptedObservation(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	settings, _ := s.GetSettings(ctx)
	settings.ActiveSources = []domain.Source{domain.SourceX}
	session, err := createVisibleUpdateSession(s, ctx, "followup fixture", settings)
	if err != nil {
		t.Fatal(err)
	}
	run := session.Runs[0]
	command, err := s.StartRun(ctx, run.ID, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ClaimCommand(ctx, run.ID, "fixture"); err != nil {
		t.Fatal(err)
	}
	observation := domain.Observation{Source: domain.SourceX, CapturedAt: domain.Now(), Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{EvidenceKey: "x:000000000000000000000099", Text: "Accepted synthetic evidence"}}}}}
	if err = s.SaveObservation(ctx, command.ID, run.ID, observation); err != nil {
		t.Fatal(err)
	}
	followup, err := s.QueueFollowUp(ctx, run.ID, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ClaimCommand(ctx, run.ID, "fixture"); err != nil {
		t.Fatal(err)
	}
	fallback, err := s.FailCommand(ctx, followup.ID, run.ID, domain.Failure{Code: "challenge_required"})
	if err != nil || !fallback {
		t.Fatalf("accepted evidence not retained: %v", err)
	}
	stored, _ := s.GetRun(ctx, run.ID)
	observations, _ := s.Observations(ctx, run.ID)
	if stored.Status != "reasoning" || len(observations) != 1 {
		t.Fatal("accepted observations were discarded")
	}
	if pause, _ := s.SourceCapturePause(ctx); pause == nil {
		t.Fatal("followup did not pause new X capture")
	}
}
