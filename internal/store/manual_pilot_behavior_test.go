//go:build auth_session_dev

package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/batchbehavior"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

func ordinaryPilotNeighbor(t *testing.T, f manualFixture) domain.BridgeCommand {
	t.Helper()
	ctx := context.Background()
	settings, err := f.s.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.ActiveSources = []domain.Source{domain.SourceX}
	settings.CollectionMode = "headless"
	// Construct overlapping durable sessions to cover old queued work and route
	// handoff races; normal session creation otherwise enforces one active session.
	if _, err := f.s.db.ExecContext(ctx, `UPDATE sessions SET status='completed' WHERE id=?`, f.session.ID); err != nil {
		t.Fatal(err)
	}
	session, err := createVisibleUpdateSession(f.s, ctx, "ordinary neighboring capture", settings)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.db.ExecContext(ctx, `UPDATE sessions SET status='running' WHERE id=?`, f.session.ID); err != nil {
		t.Fatal(err)
	}
	command, err := f.s.StartRun(ctx, session.Runs[0].ID, map[string]any{"captureRuntime": map[string]any{"driver": "headless"}})
	if err != nil {
		t.Fatal(err)
	}
	return command
}

func TestManualPilotSharesSourceAdmission(t *testing.T) {
	ctx := context.Background()
	for _, pilotFirst := range []bool{false, true} {
		t.Run(map[bool]string{true: "pilot-first", false: "ordinary-first"}[pilotFirst], func(t *testing.T) {
			f := newManualFixture(t)
			normal := ordinaryPilotNeighbor(t, f)
			if pilotFirst {
				claimManual(t, f)
				got, err := f.s.ClaimCommandWithBehavior(ctx, normal.RunID, "ordinary", "headless", batchbehavior.Policy{})
				if err != nil || got != nil {
					t.Fatal("ordinary claim overlapped pilot", err)
				}
			} else {
				got, err := f.s.ClaimCommandWithBehavior(ctx, normal.RunID, "ordinary", "headless", batchbehavior.Policy{})
				if err != nil || got == nil {
					t.Fatal("ordinary claim missing", err)
				}
				result, err := f.s.ClaimManualPilot(ctx, f.p, f.c.ID, "pilot")
				if err == nil || result.Handle != nil {
					t.Fatal("pilot overlapped ordinary claim")
				}
			}
		})
	}
}

func TestManualPilotRespectsPersistentPauseAndRecovery(t *testing.T) {
	ctx := context.Background()
	f := newManualFixture(t)
	settings, err := f.s.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.AutoUpdateEnabled = true
	if err := f.s.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	claim := claimManual(t, f)
	if err := f.s.FailManualPilotChallenge(ctx, claim, "cloudflare"); err != nil {
		t.Fatal(err)
	}
	pause, err := f.s.SourceCapturePause(ctx)
	if err != nil || pause == nil || pause.Category != "cloudflare" {
		t.Fatal("challenge lost", err)
	}
	if result, err := f.s.CommitManualPilotResult(ctx, claim, "late", manualObservation()); err == nil || result != RejectedNoMutation {
		t.Fatal("challenge accepted late result")
	}
	if err := f.s.ConfirmXCaptureRecovery(ctx, pause.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.s.FailManualPilotChallenge(ctx, claim, "x_human_verification"); err == nil {
		t.Fatal("stale challenge relatched")
	}
	if pause, _ := f.s.SourceCapturePause(ctx); pause != nil {
		t.Fatal("recovery undone")
	}
	settings, err = f.s.GetSettings(ctx)
	if err != nil || !settings.AutoUpdateEnabled {
		t.Fatal("challenge changed Auto Update")
	}
}

func TestManualPilotPauseBeforeClaim(t *testing.T) {
	f := newManualFixture(t)
	ctx := context.Background()
	if err := f.s.PauseXCapture(ctx); err != nil {
		t.Fatal(err)
	}
	if result, err := f.s.ClaimManualPilot(ctx, f.p, f.c.ID, "paused"); !errors.Is(err, ErrSourceCapturePaused) || result.Handle != nil {
		t.Fatal("paused claim admitted", err)
	}
}

func TestManualPilotUsesOrdinaryCooldown(t *testing.T) {
	f := newManualFixture(t)
	ctx := context.Background()
	normal := ordinaryPilotNeighbor(t, f)
	if _, err := f.s.ClaimCommandWithBehavior(ctx, normal.RunID, "ordinary", "headless", batchbehavior.Policy{}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.FailCommand(ctx, normal.ID, normal.RunID, domain.Failure{Code: "capture_timeout", Stage: "capture", Retryable: true}); err != nil {
		t.Fatal(err)
	}
	policy := batchbehavior.Policy{Enabled: true, Interval: time.Second, BackoffBase: 5 * time.Second, BackoffMax: 10 * time.Second}
	if r, err := f.s.ClaimManualPilotWithBehavior(ctx, f.p, f.c.ID, "waiting", policy); err == nil || r.Handle != nil {
		t.Fatal("backoff bypassed")
	}
	f.clock.now = f.clock.now.Add(5 * time.Second)
	if r, err := f.s.ClaimManualPilotWithBehavior(ctx, f.p, f.c.ID, "ready", policy); err != nil || r.Handle == nil {
		t.Fatal("cooldown never released", err)
	}
}
