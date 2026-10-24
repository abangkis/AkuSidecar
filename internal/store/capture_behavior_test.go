package store

import (
	"context"
	"github.com/abangkis/AkuSidecar/internal/batchbehavior"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestCaptureBehaviorFollowupWaitsBeforeClaim(t *testing.T) {
	ctx := context.Background()
	clock := &mutableStoreClock{now: time.Now().UTC()}
	s := openTestStoreWithClock(t, clock)
	settings, _ := s.GetSettings(ctx)
	settings.ActiveSources = []domain.Source{domain.SourceX}
	session, err := createVisibleUpdateSession(s, ctx, "followup pacing", settings)
	if err != nil {
		t.Fatal(err)
	}
	run := session.Runs[0]
	if _, err := s.StartRun(ctx, run.ID, nil); err != nil {
		t.Fatal(err)
	}
	policy := batchbehavior.Policy{Enabled: true, Interval: 5 * time.Second}
	first, err := s.ClaimCommandWithBehavior(ctx, run.ID, "fixture", "", policy)
	if err != nil || first == nil {
		t.Fatalf("%v %v", first, err)
	}
	observation := domain.Observation{Source: run.Source, CapturedAt: domain.Now(), Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{EvidenceKey: "x:synthetic", Text: "Synthetic pacing evidence"}}}}}
	if err := s.SaveObservation(ctx, first.ID, run.ID, observation); err != nil {
		t.Fatal(err)
	}
	follow, err := s.QueueFollowUp(ctx, run.ID, map[string]any{"round": 2})
	if err != nil {
		t.Fatal(err)
	}
	if c, err := s.ClaimCommandWithBehavior(ctx, run.ID, "fixture", "", policy); err != nil || c != nil {
		t.Fatalf("followup claimed early %v %v", c, err)
	}
	var claimedAt *string
	if err := s.db.QueryRowContext(ctx, `SELECT claimed_at FROM bridge_commands WHERE id=?`, follow.ID).Scan(&claimedAt); err != nil || claimedAt != nil {
		t.Fatalf("waiting consumed lease %v %v", claimedAt, err)
	}
	d, err := s.CaptureBehaviorDecision(ctx, run.Source, policy)
	if err != nil {
		t.Fatal(err)
	}
	clock.now = d.NotBefore
	c, err := s.ClaimCommandWithBehavior(ctx, run.ID, "fixture", "", policy)
	if err != nil || c == nil || c.ID != follow.ID || c.Payload["round"] != float64(2) {
		t.Fatalf("followup changed %v %v", c, err)
	}
}

func TestCaptureBehaviorDurableAdmission(t *testing.T) {
	ctx := context.Background()
	clock := &mutableStoreClock{now: time.Now().UTC()}
	path := filepath.Join(t.TempDir(), "pacing.db")
	settings := domain.DefaultSettings("expanded", "quiet", "guarded_live", true)
	settings.ActiveSources = []domain.Source{domain.SourceX, domain.SourceFacebook}
	s, err := OpenWithClock(path, settings, clock)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	policy := batchbehavior.Policy{Enabled: true, Interval: 5 * time.Second, Jitter: time.Second, BackoffBase: 10 * time.Second, BackoffMax: 40 * time.Second}
	makeRun := func() domain.Run {
		// Model durable work from an older session while a new session is
		// selected: admission must inspect commands, not only session state.
		if _, err := s.db.ExecContext(ctx, `UPDATE sessions SET status='completed'`); err != nil {
			t.Fatal(err)
		}
		session, err := createVisibleUpdateSession(s, ctx, "pacing", settings)
		if err != nil {
			t.Fatal(err)
		}
		run := session.Runs[0]
		if _, err := s.StartRun(ctx, run.ID, nil); err != nil {
			t.Fatal(err)
		}
		return run
	}
	first, second := makeRun(), makeRun()
	var wg sync.WaitGroup
	results := make(chan *domain.BridgeCommand, 2)
	failures := make(chan error, 2)
	for _, run := range []domain.Run{first, second} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			c, err := s.ClaimCommandWithBehavior(ctx, id, "fixture", "", policy)
			results <- c
			failures <- err
		}(run.ID)
	}
	wg.Wait()
	close(results)
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	var claimed *domain.BridgeCommand
	count := 0
	for c := range results {
		if c != nil {
			claimed = c
			count++
		}
	}
	if count != 1 {
		t.Fatalf("claims=%d", count)
	}
	pending := first
	if claimed.RunID == first.ID {
		pending = second
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenWithClock(path, settings, clock)
	if err != nil {
		t.Fatal(err)
	}
	if c, err := s.ClaimCommandWithBehavior(ctx, pending.ID, "fixture", "", policy); err != nil || c != nil {
		t.Fatalf("restart overlap %v %v", c, err)
	}
	activeRun, err := s.GetRun(ctx, claimed.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CancelSession(ctx, activeRun.SessionID); err != nil {
		t.Fatal(err)
	}
	decision, err := s.CaptureBehaviorDecision(ctx, domain.SourceX, policy)
	if err != nil || decision.Allowed || decision.NotBefore.IsZero() {
		t.Fatalf("cancel spacing %+v %v", decision, err)
	}
	before := decision.NotBefore
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenWithClock(path, settings, clock)
	if err != nil {
		t.Fatal(err)
	}
	decision, err = s.CaptureBehaviorDecision(ctx, domain.SourceX, policy)
	if err != nil || !decision.NotBefore.Equal(before) {
		t.Fatalf("restart redrew delay %+v %v", decision, err)
	}
	clock.now = before
	next, err := s.ClaimCommandWithBehavior(ctx, pending.ID, "fixture", "", policy)
	if err != nil || next == nil {
		t.Fatalf("boundary %v %v", next, err)
	}
	if _, err := s.FailCommand(ctx, claimed.ID, claimed.RunID, domain.Failure{Code: "late"}); err == nil {
		t.Fatal("stale command completed")
	}
	if d, err := s.CaptureBehaviorDecision(ctx, domain.SourceX, policy); err != nil || d.Reason != "capture_active" {
		t.Fatalf("stale freed slot %+v %v", d, err)
	}
	if d, err := s.CaptureBehaviorDecision(ctx, domain.SourceFacebook, policy); err != nil || !d.Allowed {
		t.Fatalf("other source blocked %+v %v", d, err)
	}
}

func TestCaptureBehaviorSharesRecaptureSlotAndBackoff(t *testing.T) {
	ctx := context.Background()
	clock := &mutableStoreClock{now: time.Now().UTC()}
	s := openTestStoreWithClock(t, clock)
	policy := batchbehavior.Policy{Enabled: true, Interval: time.Second, BackoffBase: 10 * time.Second, BackoffMax: 40 * time.Second}
	timelineID, _ := insertUnavailableMediaFixture(t, s)
	job, err := s.CreateMediaRecapture(ctx, timelineID, domain.MediaRecaptureBackground)
	if err != nil {
		t.Fatal(err)
	}
	settings, _ := s.GetSettings(ctx)
	if _, err := s.db.ExecContext(ctx, `UPDATE sessions SET status='completed'`); err != nil {
		t.Fatal(err)
	}
	settings.ActiveSources = []domain.Source{domain.SourceX}
	session, err := createVisibleUpdateSession(s, ctx, "pacing", settings)
	if err != nil {
		t.Fatal(err)
	}
	run := session.Runs[0]
	if _, err := s.StartRun(ctx, run.ID, nil); err != nil {
		t.Fatal(err)
	}
	claimed, err := s.ClaimMediaRecaptureWithBehavior(ctx, job.ID, "fixture", "", policy)
	if err != nil || claimed.ID == "" {
		t.Fatalf("recapture %v %v", claimed, err)
	}
	if c, err := s.ClaimCommandWithBehavior(ctx, run.ID, "fixture", "", policy); err != nil || c != nil {
		t.Fatalf("command overlapped %v %v", c, err)
	}
	if _, err := s.FailMediaRecapture(ctx, job.ID, domain.Failure{Code: "headless_capture_timeout", Retryable: true}); err != nil {
		t.Fatal(err)
	}
	d, err := s.CaptureBehaviorDecision(ctx, domain.SourceX, policy)
	if err != nil || d.Reason != "temporary_backoff" {
		t.Fatalf("backoff %+v %v", d, err)
	}
	clock.now = d.NotBefore
	c, err := s.ClaimCommandWithBehavior(ctx, run.ID, "fixture", "", policy)
	if err != nil || c == nil {
		t.Fatalf("after backoff %v %v", c, err)
	}
	// A stale terminal media callback cannot release the now-active command.
	if _, err := s.FailMediaRecapture(ctx, job.ID, domain.Failure{Code: "late"}); err == nil {
		t.Fatal("stale recapture accepted")
	}
	d, err = s.CaptureBehaviorDecision(ctx, domain.SourceX, policy)
	if err != nil || d.Reason != "capture_active" {
		t.Fatalf("slot released %+v %v", d, err)
	}
}

func TestCaptureBehaviorOrdersTerminalHistoryAtFullPrecision(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)
	clock := &mutableStoreClock{now: base}
	s := openTestStoreWithClock(t, clock)
	settings, _ := s.GetSettings(ctx)
	settings.ActiveSources = []domain.Source{domain.SourceX}
	session, err := createVisibleUpdateSession(s, ctx, "history precision", settings)
	if err != nil {
		t.Fatal(err)
	}
	for i, id := range []string{"z-older", "a-newer"} {
		command, err := s.StartRun(ctx, session.Runs[0].ID, nil)
		if err != nil {
			t.Fatal(err)
		}
		at := base.Add(time.Duration(i+1) * 100 * time.Microsecond).Format(time.RFC3339Nano)
		if _, err := s.db.ExecContext(ctx, `UPDATE bridge_commands SET id=?,status='completed',claimed_at=?,completed_at=? WHERE id=?`, id, at, at, command.ID); err != nil {
			t.Fatal(err)
		}
	}
	decision, err := s.CaptureBehaviorDecision(ctx, domain.SourceX, batchbehavior.Policy{Enabled: true, Interval: time.Second})
	if err != nil || !decision.NotBefore.Equal(base.Add(time.Second+200*time.Microsecond)) {
		t.Fatalf("selected older completion: %+v %v", decision, err)
	}
}
