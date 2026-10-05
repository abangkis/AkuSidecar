package engine

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestRetryPreparedUpdateAfterReaderCloseBypassesOnlyRecentFixedCadence(t *testing.T) {
	clock := &mutableEngineClock{now: time.Date(2026, time.October, 5, 10, 0, 0, 0, time.UTC)}
	runtime, state := testEngineWithClock(t, clock)
	ctx := context.Background()
	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.AutoUpdateMode = "fixed"
	settings.AutoUpdateRefillMinutes = 30
	if err := state.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}

	if session, err := runtime.startAutoUpdate(ctx, false); err == nil || session.ID != "" || !strings.Contains(err.Error(), "AkuBridge is not ready") {
		t.Fatalf("first scheduled attempt session=%+v err=%v; wanted the shared Bridge guard", session, err)
	}
	afterTick, err := state.AutoUpdateScheduleState(ctx)
	if err != nil || afterTick.LastSchedulerTickAt == "" {
		t.Fatalf("first scheduled tick=%+v err=%v", afterTick, err)
	}
	if session, err := runtime.startAutoUpdate(ctx, false); err != nil || session.ID != "" {
		t.Fatalf("normal scheduler retried inside recent cadence: session=%+v err=%v", session, err)
	}
	stillDueLater, err := state.AutoUpdateScheduleState(ctx)
	if err != nil || stillDueLater.LastSchedulerTickAt != afterTick.LastSchedulerTickAt {
		t.Fatalf("normal retry consumed another tick: before=%+v after=%+v err=%v", afterTick, stillDueLater, err)
	}

	if session, err := runtime.RetryPreparedUpdateAfterReaderClose(ctx); err == nil || session.ID != "" || !strings.Contains(err.Error(), "AkuBridge is not ready") {
		t.Fatalf("reader-close retry session=%+v err=%v; wanted cadence bypass followed by the shared Bridge guard", session, err)
	}
	receipts, err := state.AutoUpdateSchedulerReceipts(ctx, 10)
	if err != nil || len(receipts) != 1 {
		t.Fatalf("explicit reader-close retry added a scheduler tick: receipts=%+v err=%v", receipts, err)
	}
}

func TestRetryPreparedUpdateAfterReaderCloseRetainsEnabledAndUsagePauseGuards(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		runtime, state := testEngine(t)
		ctx := context.Background()
		settings, err := state.GetSettings(ctx)
		if err != nil {
			t.Fatal(err)
		}
		settings.AutoUpdateEnabled = false
		if err := state.SaveSettings(ctx, settings); err != nil {
			t.Fatal(err)
		}
		if session, err := runtime.RetryPreparedUpdateAfterReaderClose(ctx); err == nil || session.ID != "" || !strings.Contains(err.Error(), "Auto Update is disabled") {
			t.Fatalf("session=%+v err=%v; disabled guard was bypassed", session, err)
		}
	})
	t.Run("provider pause", func(t *testing.T) {
		runtime, state := testEngine(t)
		ctx := context.Background()
		if err := state.PauseAutoUpdateForUsageLimit(ctx, domain.AutoUpdateUsageLimitPause{Message: "usage limit fixture"}); err != nil {
			t.Fatal(err)
		}
		if session, err := runtime.RetryPreparedUpdateAfterReaderClose(ctx); err == nil || session.ID != "" || !strings.Contains(err.Error(), "confirm restored usage") {
			t.Fatalf("session=%+v err=%v; provider pause was bypassed", session, err)
		}
	})
}

func TestRetryPreparedUpdateAfterReaderClosePreservesAdaptiveGenerationAllowance(t *testing.T) {
	clock := &mutableEngineClock{now: time.Now().UTC()}
	runtime, state := testEngineWithClock(t, clock)
	ctx := context.Background()
	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.AutoUpdateMode = "adaptive"
	settings.PreparedBatchLimit = 3
	if err := state.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	policy := domain.UpdatePolicy{
		Trigger: domain.UpdateTriggerScheduler, Delivery: domain.UpdateDeliveryPrepared,
		BudgetAuthority: domain.BudgetAuthorityAutomatic,
	}
	for i := 0; i < settings.PreparedBatchLimit; i++ {
		session, createErr := state.CreateUpdateSession(ctx, "adaptive allowance fixture", settings, policy)
		if createErr != nil {
			t.Fatal(createErr)
		}
		// Keep each scheduler attempt in the rolling generation window without
		// leaving an active session or a completed yield signal.
		if err := state.CancelSession(ctx, session.ID); err != nil {
			t.Fatal(err)
		}
	}

	status, err := runtime.AutoUpdateStatus(ctx)
	if err != nil || status.GenerationAllowanceUsed != settings.PreparedBatchLimit {
		t.Fatalf("adaptive status=%+v err=%v; wanted allowance fully used", status, err)
	}
	if session, err := runtime.RetryPreparedUpdateAfterReaderClose(ctx); err == nil || session.ID != "" || !strings.Contains(err.Error(), "Bounded generation allowance reached") {
		t.Fatalf("session=%+v err=%v; reader-close retry bypassed the adaptive allowance", session, err)
	}
}
