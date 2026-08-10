package store

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

type mutableStoreClock struct {
	now time.Time
}

func (clock *mutableStoreClock) Now() time.Time { return clock.now }

func openTestStoreWithClock(t *testing.T, clock Clock) *Store {
	t.Helper()
	settings := domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", true)
	state, err := OpenWithClock(filepath.Join(t.TempDir(), "sidecar.db"), settings, clock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { state.Close() })
	return state
}

func TestAutoUpdateSchedulerReceiptsPersistCompleteAndStayBounded(t *testing.T) {
	state := openTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	receipt := domain.AutoUpdateTickReceipt{
		ID: "tick-initial", TickAt: now.Format(time.RFC3339Nano),
		Mode: "adaptive", CadenceTier: "warm", CadenceMinutes: 15,
		NextTickAt: now.Add(15 * time.Minute).Format(time.RFC3339Nano), Outcome: "checking",
	}
	if err := state.RecordAutoUpdateSchedulerTick(ctx, receipt); err != nil {
		t.Fatal(err)
	}
	pending, err := state.AutoUpdateSchedulerReceipts(ctx, 10)
	if err != nil || len(pending) != 1 || pending[0].Outcome != "checking" || pending[0].DecidedAt != "" {
		t.Fatalf("pending receipts=%+v err=%v", pending, err)
	}
	schedule, err := state.AutoUpdateScheduleState(ctx)
	if err != nil || schedule.LastSchedulerTickAt != receipt.TickAt {
		t.Fatalf("schedule=%+v err=%v", schedule, err)
	}
	receipt.Outcome = "started"
	receipt.DecidedAt = now.Add(time.Second).Format(time.RFC3339Nano)
	receipt.SessionID = "session-test"
	if err := state.CompleteAutoUpdateSchedulerTick(ctx, receipt); err != nil {
		t.Fatal(err)
	}
	receipts, err := state.AutoUpdateSchedulerReceipts(ctx, 10)
	if err != nil || len(receipts) != 1 || receipts[0].Outcome != "started" || receipts[0].SessionID != "session-test" {
		t.Fatalf("receipts=%+v err=%v", receipts, err)
	}

	for index := 0; index < maxAutoUpdateSchedulerReceipts+3; index++ {
		tickAt := now.Add(time.Duration(index+1) * time.Minute)
		value := domain.AutoUpdateTickReceipt{
			ID: fmt.Sprintf("tick-%02d", index), TickAt: tickAt.Format(time.RFC3339Nano),
			Mode: "fixed", CadenceTier: "continuous", CadenceMinutes: 15,
			NextTickAt: tickAt.Add(15 * time.Minute).Format(time.RFC3339Nano), Outcome: "checking",
		}
		if err := state.RecordAutoUpdateSchedulerTick(ctx, value); err != nil {
			t.Fatal(err)
		}
	}
	receipts, err = state.AutoUpdateSchedulerReceipts(ctx, maxAutoUpdateSchedulerReceipts+10)
	if err != nil || len(receipts) != maxAutoUpdateSchedulerReceipts {
		t.Fatalf("bounded receipts=%+v err=%v", receipts, err)
	}
	if receipts[0].ID != "tick-34" {
		t.Fatalf("newest bounded receipt=%+v", receipts[0])
	}
}

func TestAutoUpdateQuotaUsesInjectedLocalDayAcrossMidnightAndDowntime(t *testing.T) {
	location := time.FixedZone("WIB", 7*60*60)
	clock := &mutableStoreClock{now: time.Date(2026, time.August, 10, 23, 58, 0, 0, location)}
	state := openTestStoreWithClock(t, clock)
	ctx := context.Background()
	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.CompleteOnboarding(ctx, settings.ActiveSources); err != nil {
		t.Fatal(err)
	}
	session, err := createPreparedUpdateSession(state, ctx, "clock quota fixture", settings)
	if err != nil {
		t.Fatal(err)
	}
	runs, err := state.listRuns(ctx, session.ID)
	if err != nil || len(runs) == 0 {
		t.Fatalf("runs=%+v err=%v", runs, err)
	}
	startedAt := clock.Now().UTC().Format(time.RFC3339Nano)
	if _, err := state.db.ExecContext(ctx, `UPDATE sessions SET status='completed',started_at=?,completed_at=? WHERE id=?`, startedAt, startedAt, session.ID); err != nil {
		t.Fatal(err)
	}
	insertUsage := func(id string, tokens int, at time.Time) {
		t.Helper()
		if _, err := state.db.ExecContext(ctx, `INSERT INTO reasoning_invocations(id,run_id,phase,provider,model,effort,duration_ms,status,input_tokens,output_tokens,reasoning_output_tokens,created_at) VALUES(?,?,'candidate_evaluation','fixture','fixture','high',1,'completed',?,0,0,?)`, id, runs[0].ID, tokens, at.UTC().Format(time.RFC3339Nano)); err != nil {
			t.Fatal(err)
		}
	}

	insertUsage("quota-before-midnight", 175, clock.Now())
	before, err := state.AutoUpdateBudgetUsage(ctx)
	if err != nil || before.ActualTotal != 175 || before.QuotaAutomatic != 175 {
		t.Fatalf("before reset=%+v err=%v", before, err)
	}
	if _, err := state.ResetAutoUpdateDailyQuota(ctx); err != nil {
		t.Fatal(err)
	}
	clock.now = time.Date(2026, time.August, 10, 23, 59, 0, 0, location)
	insertUsage("quota-after-reset", 25, clock.Now())
	afterReset, err := state.AutoUpdateBudgetUsage(ctx)
	if err != nil || afterReset.ActualTotal != 200 || afterReset.QuotaTotal != 25 || afterReset.QuotaAutomatic != 25 {
		t.Fatalf("after reset=%+v err=%v", afterReset, err)
	}

	clock.now = time.Date(2026, time.August, 11, 0, 1, 0, 0, location)
	afterMidnight, err := state.AutoUpdateBudgetUsage(ctx)
	if err != nil || afterMidnight.ActualTotal != 0 || afterMidnight.QuotaTotal != 0 || afterMidnight.LastManualResetAt != "" {
		t.Fatalf("after midnight=%+v err=%v", afterMidnight, err)
	}
	insertUsage("quota-crossing-session", 40, clock.Now())
	crossing, err := state.AutoUpdateBudgetUsage(ctx)
	if err != nil || crossing.ActualTotal != 40 || crossing.QuotaAutomatic != 40 {
		t.Fatalf("crossing midnight=%+v err=%v", crossing, err)
	}

	clock.now = time.Date(2026, time.August, 13, 9, 0, 0, 0, location)
	afterDowntime, err := state.AutoUpdateBudgetUsage(ctx)
	if err != nil || afterDowntime.ActualTotal != 0 || afterDowntime.QuotaTotal != 0 {
		t.Fatalf("after multi-day downtime=%+v err=%v", afterDowntime, err)
	}
	insertUsage("quota-after-downtime", 10, clock.Now())
	currentDay, err := state.AutoUpdateBudgetUsage(ctx)
	if err != nil || currentDay.ActualTotal != 10 || currentDay.QuotaAutomatic != 10 {
		t.Fatalf("current day=%+v err=%v", currentDay, err)
	}
}
