package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestAutoUpdateAdaptiveSignalsMeasureDemandLeadAllowanceAndSupply(t *testing.T) {
	location := time.FixedZone("WIB", 7*60*60)
	now := time.Date(2026, time.August, 10, 12, 0, 0, 0, location)
	clock := &mutableStoreClock{now: now}
	state := openTestStoreWithClock(t, clock)
	ctx := context.Background()
	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.CompleteOnboarding(ctx, settings.ActiveSources); err != nil {
		t.Fatal(err)
	}

	completedOffsets := []time.Duration{-20 * time.Minute, -10 * time.Minute, -time.Minute}
	revealedOffsets := []time.Duration{-10 * time.Minute, -5 * time.Minute, 0}
	createdOffsets := []time.Duration{-25 * time.Minute, -15 * time.Minute, -5 * time.Minute}
	for index := range completedOffsets {
		session, err := createPreparedUpdateSession(state, ctx, fmt.Sprintf("adaptive fixture %d", index), settings)
		if err != nil {
			t.Fatal(err)
		}
		runs, err := state.listRuns(ctx, session.ID)
		if err != nil || len(runs) == 0 {
			t.Fatalf("runs=%+v err=%v", runs, err)
		}
		completedAt := now.Add(completedOffsets[index])
		startedAt := completedAt.Add(-6 * time.Minute)
		if _, err := state.db.ExecContext(ctx, `UPDATE sessions SET status='completed',started_at=?,completed_at=? WHERE id=?`, startedAt.UTC().Format(time.RFC3339Nano), completedAt.UTC().Format(time.RFC3339Nano), session.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := state.db.ExecContext(ctx, `UPDATE runs SET status='completed',stage='completed',started_at=?,completed_at=? WHERE session_id=?`, startedAt.UTC().Format(time.RFC3339Nano), completedAt.UTC().Format(time.RFC3339Nano), session.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := state.db.ExecContext(ctx, `UPDATE auto_update_batches SET state='visible',created_at=?,prepared_at=?,revealed_at=? WHERE session_id=?`, now.Add(createdOffsets[index]).UTC().Format(time.RFC3339Nano), completedAt.UTC().Format(time.RFC3339Nano), now.Add(revealedOffsets[index]).UTC().Format(time.RFC3339Nano), session.ID); err != nil {
			t.Fatal(err)
		}
		if index == 0 {
			if _, err := state.db.ExecContext(ctx, `INSERT INTO timeline_items(id,session_id,run_id,source,evidence_key,rank,item_json,assessment_json,created_at) VALUES(?,?,?,?,?,0,'{}','{}',?)`, "adaptive-yield", session.ID, runs[0].ID, runs[0].Source, "x:adaptive-yield", completedAt.UTC().Format(time.RFC3339Nano)); err != nil {
				t.Fatal(err)
			}
		}
	}

	signals, err := state.AutoUpdateAdaptiveSignals(ctx, 30*time.Minute, 60*time.Minute, 30*time.Minute, 8*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if signals.ConsumptionPace != 5*time.Minute || signals.ConsumptionSamples != 2 {
		t.Fatalf("consumption signals=%+v", signals)
	}
	if signals.PreparationLead != 8*time.Minute {
		t.Fatalf("preparation lead=%v", signals.PreparationLead)
	}
	if len(signals.GenerationAttempts) != 3 {
		t.Fatalf("generation attempts=%+v", signals.GenerationAttempts)
	}
	wantBackoff := now.Add(29 * time.Minute)
	if signals.LastYieldItems != 0 || signals.EmptyYieldStreak != 2 || !signals.SupplyBackoffUntil.Equal(wantBackoff) {
		t.Fatalf("supply signals=%+v want backoff=%v", signals, wantBackoff)
	}
	if signals.LastOutcome.Kind != "valid_empty" || signals.LastOutcome.ItemCount != 0 || signals.LastOutcome.Trigger != "scheduler" {
		t.Fatalf("last outcome=%+v", signals.LastOutcome)
	}
	if signals.ReplenishmentPressure < 65 || signals.PressureFromReveals == 0 || signals.PressureFromUpdates == 0 || signals.PressureFromYield == 0 {
		t.Fatalf("replenishment pressure=%+v", signals)
	}
}

func TestAutoUpdateAdaptiveSignalsIgnoreManualPreparedAttempts(t *testing.T) {
	clock := &mutableStoreClock{now: time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)}
	state := openTestStoreWithClock(t, clock)
	ctx := context.Background()
	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	session, err := state.CreateUpdateSession(ctx, "manual prepared fixture", settings, domain.UpdatePolicy{Trigger: domain.UpdateTriggerUser, Delivery: domain.UpdateDeliveryPrepared, BudgetAuthority: domain.BudgetAuthorityAutomatic})
	if err != nil {
		t.Fatal(err)
	}
	completedAt := clock.Now().UTC().Format(time.RFC3339Nano)
	if _, err := state.db.ExecContext(ctx, `UPDATE sessions SET status='completed',started_at=?,completed_at=? WHERE id=?`, completedAt, completedAt, session.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.ExecContext(ctx, `UPDATE runs SET status='completed',stage='completed',started_at=?,completed_at=? WHERE session_id=?`, completedAt, completedAt, session.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.ExecContext(ctx, `UPDATE auto_update_batches SET state='expired',prepared_at=? WHERE session_id=?`, completedAt, session.ID); err != nil {
		t.Fatal(err)
	}
	signals, err := state.AutoUpdateAdaptiveSignals(ctx, 30*time.Minute, 60*time.Minute, 30*time.Minute, 8*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(signals.GenerationAttempts) != 0 {
		t.Fatalf("manual prepared action consumed adaptive allowance: %+v", signals.GenerationAttempts)
	}
	if signals.EmptyYieldStreak != 1 {
		t.Fatalf("manual prepared outcome did not inform supply backoff: %+v", signals)
	}
}

func TestAutoUpdateAdaptiveSignalsSeparateTechnicalFailureFromValidEmpty(t *testing.T) {
	clock := &mutableStoreClock{now: time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)}
	state := openTestStoreWithClock(t, clock)
	ctx := context.Background()
	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.CompleteOnboarding(ctx, settings.ActiveSources); err != nil {
		t.Fatal(err)
	}

	failed, err := createPreparedUpdateSession(state, ctx, "technical fixture", settings)
	if err != nil {
		t.Fatal(err)
	}
	completedAt := clock.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano)
	if _, err := state.db.ExecContext(ctx, `UPDATE sessions SET status='partial',started_at=?,completed_at=? WHERE id=?`, completedAt, completedAt, failed.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.ExecContext(ctx, `UPDATE runs SET status='failed',stage='reasoning',started_at=?,completed_at=?,error_json=? WHERE session_id=? AND ordinal=0`, completedAt, completedAt, `{"code":"reasoning_failed","retryable":true}`, failed.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.ExecContext(ctx, `UPDATE runs SET status='completed',stage='completed',started_at=?,completed_at=? WHERE session_id=? AND ordinal>0`, completedAt, completedAt, failed.ID); err != nil {
		t.Fatal(err)
	}

	signals, err := state.AutoUpdateAdaptiveSignals(ctx, 30*time.Minute, 60*time.Minute, 30*time.Minute, 8*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if signals.LastOutcome.Kind != "technical_failure" || signals.EmptyYieldStreak != 0 || signals.TechnicalStreak != 1 || signals.TechnicalBackoffUntil.IsZero() {
		t.Fatalf("technical outcome=%+v", signals)
	}
	if !signals.TechnicalBackoffUntil.Equal(clock.Now().Add(4 * time.Minute)) {
		t.Fatalf("technical backoff=%v", signals.TechnicalBackoffUntil)
	}
}

func TestAutoUpdateAdaptiveSignalsUseProductiveUserUpdateToClearSupplyBackoff(t *testing.T) {
	clock := &mutableStoreClock{now: time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)}
	state := openTestStoreWithClock(t, clock)
	ctx := context.Background()
	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.CompleteOnboarding(ctx, settings.ActiveSources); err != nil {
		t.Fatal(err)
	}

	empty, err := createPreparedUpdateSession(state, ctx, "empty scheduler fixture", settings)
	if err != nil {
		t.Fatal(err)
	}
	emptyAt := clock.Now().Add(-20 * time.Minute).UTC().Format(time.RFC3339Nano)
	if _, err := state.db.ExecContext(ctx, `UPDATE sessions SET status='completed',started_at=?,completed_at=? WHERE id=?`, emptyAt, emptyAt, empty.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.ExecContext(ctx, `UPDATE runs SET status='completed',stage='completed',started_at=?,completed_at=? WHERE session_id=?`, emptyAt, emptyAt, empty.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.ExecContext(ctx, `UPDATE auto_update_batches SET state='expired',prepared_at=? WHERE session_id=?`, emptyAt, empty.ID); err != nil {
		t.Fatal(err)
	}

	productive, err := state.CreateUpdateSession(ctx, "productive user fixture", settings, domain.UpdatePolicy{Trigger: domain.UpdateTriggerUser, Delivery: domain.UpdateDeliveryVisible, BudgetAuthority: domain.BudgetAuthorityUser})
	if err != nil {
		t.Fatal(err)
	}
	productiveAt := clock.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano)
	if _, err := state.db.ExecContext(ctx, `UPDATE sessions SET status='completed',started_at=?,completed_at=? WHERE id=?`, productiveAt, productiveAt, productive.ID); err != nil {
		t.Fatal(err)
	}
	runs, err := state.listRuns(ctx, productive.ID)
	if err != nil || len(runs) == 0 {
		t.Fatalf("productive runs=%+v err=%v", runs, err)
	}
	if _, err := state.db.ExecContext(ctx, `UPDATE runs SET status='completed',stage='completed',started_at=?,completed_at=? WHERE session_id=?`, productiveAt, productiveAt, productive.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.ExecContext(ctx, `INSERT INTO timeline_items(id,session_id,run_id,source,evidence_key,rank,item_json,assessment_json,created_at) VALUES(?,?,?,?,?,0,'{}','{}',?)`, "productive-user-item", productive.ID, runs[0].ID, runs[0].Source, "x:productive-user-item", productiveAt); err != nil {
		t.Fatal(err)
	}

	signals, err := state.AutoUpdateAdaptiveSignals(ctx, 30*time.Minute, 60*time.Minute, 30*time.Minute, 8*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if signals.LastOutcome.Kind != "productive" || signals.LastOutcome.Trigger != "user" || signals.LastOutcome.ItemCount != 1 || signals.EmptyYieldStreak != 0 || !signals.SupplyBackoffUntil.IsZero() {
		t.Fatalf("productive user outcome=%+v", signals)
	}
}

func TestReplenishmentPressureDecaysAndRewardsHealthyYield(t *testing.T) {
	now := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)
	reveals := []time.Time{now.Add(-10 * time.Minute), now.Add(-5 * time.Minute)}
	outcomes := []AutoUpdateAdaptiveOutcome{
		{CompletedAt: now.Add(-4 * time.Minute), ItemCount: 5},
	}
	reveal, updates, yield := replenishmentPressureComponents(now, 30*time.Minute, reveals, outcomes)
	if reveal <= 0 || updates <= 0 || yield >= 0 {
		t.Fatalf("unexpected pressure components reveal=%d updates=%d yield=%d", reveal, updates, yield)
	}
	lateReveal, lateUpdates, lateYield := replenishmentPressureComponents(now.Add(45*time.Minute), 30*time.Minute, reveals, outcomes)
	if lateReveal >= reveal || lateUpdates >= updates || lateYield <= yield {
		t.Fatalf("pressure did not decay: now=(%d,%d,%d) later=(%d,%d,%d)", reveal, updates, yield, lateReveal, lateUpdates, lateYield)
	}
}
