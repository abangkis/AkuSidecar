package engine

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/store"
)

const defaultAutoUpdateEstimatedTokens int64 = 100000
const autoDeepDetectionEstimatedTokens int64 = 20000
const autoUpdateActiveWindow = 5 * time.Minute
const autoUpdateWarmWindow = 30 * time.Minute
const autoUpdateActiveCadence = 5 * time.Minute
const autoUpdateWarmCadence = 15 * time.Minute
const autoUpdateIdleCadence = 60 * time.Minute

type autoUpdateCadence struct {
	Tier     string
	Duration time.Duration
}

func (e *Engine) autoDeepDetectionAllowed(ctx context.Context, settings domain.Settings) bool {
	usage, err := e.store.AutoUpdateBudgetUsage(ctx)
	if err != nil {
		return false
	}
	autoLimit := int64(settings.AutoUpdateDailyTokenBudget * (100 - settings.AutoUpdateManualReservePct) / 100)
	return usage.QuotaTotal+autoDeepDetectionEstimatedTokens <= int64(settings.AutoUpdateDailyTokenBudget) && usage.QuotaAutomatic+autoDeepDetectionEstimatedTokens <= autoLimit
}

func (e *Engine) estimatedAutoUpdateTokens(ctx context.Context) int64 {
	estimate, err := e.store.EstimatedSessionTokens(ctx)
	if err != nil || estimate <= 0 {
		return defaultAutoUpdateEstimatedTokens
	}
	if estimate < 50000 {
		return 50000
	}
	if estimate > 250000 {
		return 250000
	}
	return estimate
}

func (e *Engine) StartAutoUpdateScheduler() {
	e.mu.Lock()
	if e.autoCancel != nil {
		e.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	e.autoCancel = cancel
	e.mu.Unlock()
	go e.autoUpdateLoop(ctx)
}

func (e *Engine) RecordUIAccess(ctx context.Context) {
	if err := e.store.RecordAutoUpdateUIAccess(ctx); err != nil {
		e.logger.Printf("record UI access for Auto Update failed: %v", err)
	}
	select {
	case e.autoWake <- struct{}{}:
	default:
	}
}

func (e *Engine) AutoUpdateStatus(ctx context.Context) (domain.AutoUpdateStatus, error) {
	settings, err := e.store.GetSettings(ctx)
	if err != nil {
		return domain.AutoUpdateStatus{}, err
	}
	batches, err := e.store.PreparedBatches(ctx, settings.PreparedBatchMaxAgeHours)
	if err != nil {
		return domain.AutoUpdateStatus{}, err
	}
	usage, err := e.store.AutoUpdateBudgetUsage(ctx)
	if err != nil {
		return domain.AutoUpdateStatus{}, err
	}
	dailyBudget := int64(settings.AutoUpdateDailyTokenBudget)
	manualReserve := dailyBudget * int64(settings.AutoUpdateManualReservePct) / 100
	automaticLimit := dailyBudget - manualReserve
	status := domain.AutoUpdateStatus{
		Enabled: settings.AutoUpdateEnabled, Mode: settings.AutoUpdateMode, State: "idle",
		BudgetResetAt: localBudgetResetAt(), LastManualBudgetResetAt: usage.LastManualResetAt,
		DailyTokenBudget: dailyBudget, DailyTokensUsed: usage.ActualTotal,
		QuotaTokensUsed: usage.QuotaTotal, DailyTokensRemaining: maxNonNegative(dailyBudget - usage.QuotaTotal),
		AutomaticTokensUsed: usage.QuotaAutomatic, AutomaticTokensRemaining: maxNonNegative(automaticLimit - usage.QuotaAutomatic),
		ManualReserveTokens: manualReserve, AutomaticTokenLimit: automaticLimit,
		PreparedBatchLimit:     settings.PreparedBatchLimit,
		AvailablePreparedSlots: max(0, settings.PreparedBatchLimit-len(batches)),
		RefillIntervalMinutes:  settings.AutoUpdateRefillMinutes,
		PreparedBatches:        batches,
	}
	estimatedTokens := e.estimatedAutoUpdateTokens(ctx)
	status.EstimatedNextRunTokens = estimatedTokens
	schedule, err := e.store.AutoUpdateScheduleState(ctx)
	if err != nil {
		return domain.AutoUpdateStatus{}, err
	}
	now := time.Now()
	cadence := scheduledAutoUpdateCadence(settings, schedule, now)
	status.LastUserActivityAt = schedule.LastUIAccessAt
	status.RecentUserActivity = cadence.Tier == "active" || cadence.Tier == "warm"
	status.ActivityWindowMinutes = int(autoUpdateWarmWindow / time.Minute)
	status.LastSchedulerTickAt = schedule.LastSchedulerTickAt
	status.CadenceTier = cadence.Tier
	status.CadenceMinutes = int(cadence.Duration / time.Minute)
	if settings.AutoUpdateEnabled {
		next, due := nextScheduledAutoUpdateTick(schedule, cadence.Duration, now)
		status.NextCheckAt = next.Format(time.RFC3339Nano)
		if !due {
			status.Reason = fmt.Sprintf("Waiting for the next %s cadence tick", cadence.Tier)
		}
	}
	receipts, err := e.store.AutoUpdateSchedulerReceipts(ctx, 10)
	if err != nil {
		return domain.AutoUpdateStatus{}, err
	}
	status.SchedulerReceipts = receipts
	if active, activeErr := e.store.ActiveSession(ctx); activeErr != nil {
		return domain.AutoUpdateStatus{}, activeErr
	} else if active != nil {
		if active.Delivery == domain.UpdateDeliveryPrepared {
			status.State, status.Reason = "running", "Preparing a bounded batch"
		} else {
			status.State, status.Reason = "paused", "A visible update is running"
		}
		return status, nil
	}
	if !settings.AutoUpdateEnabled {
		status.State, status.Reason = "disabled", "Auto Update is off"
		return status, nil
	}
	if bridge := e.BridgeStatus(); bridge.Compatible && len(e.grantedActiveSources(settings)) == 0 {
		status.State, status.Reason = "paused", "No active source has AkuBridge permission"
		return status, nil
	}
	if len(batches) >= settings.PreparedBatchLimit {
		status.State, status.Reason = "paused", "Prepared batch limit reached"
		return status, nil
	}
	if usage.QuotaAutomatic+estimatedTokens > status.AutomaticTokenLimit || usage.QuotaTotal+estimatedTokens > dailyBudget {
		status.State, status.Reason = "budget_paused", "Automatic token allowance reached"
		return status, nil
	}
	return status, nil
}

func maxNonNegative(value int64) int64 {
	if value < 0 {
		return 0
	}
	return value
}

func localBudgetResetAt() string {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location()).Format(time.RFC3339Nano)
}

func (e *Engine) RevealPreparedBatch(ctx context.Context, sessionID, presentation string) (domain.PreparedBatch, error) {
	e.operation.Lock()
	defer e.operation.Unlock()
	batch, err := e.store.RevealPreparedBatch(ctx, sessionID, presentation)
	if err == nil {
		select {
		case e.autoWake <- struct{}{}:
		default:
		}
	}
	return batch, err
}

func (e *Engine) ResetAutoUpdateDailyQuota(ctx context.Context) (domain.AutoUpdateStatus, error) {
	e.operation.Lock()
	defer e.operation.Unlock()
	if _, err := e.store.ResetAutoUpdateDailyQuota(ctx); err != nil {
		return domain.AutoUpdateStatus{}, err
	}
	select {
	case e.autoWake <- struct{}{}:
	default:
	}
	return e.AutoUpdateStatus(ctx)
}

func (e *Engine) autoUpdateLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-e.autoWake:
		}
		if err := e.maybeStartAutoUpdate(ctx); err != nil {
			e.logger.Printf("Auto Update deferred: %v", err)
		}
	}
}

func (e *Engine) maybeStartAutoUpdate(ctx context.Context) error {
	_, err := e.startAutoUpdate(ctx, false)
	return err
}

// StartPreparedUpdateNow prepares a finite batch on explicit user request.
// It keeps all safety and budget gates, but intentionally bypasses the
// scheduler's cadence gate.
func (e *Engine) StartPreparedUpdateNow(ctx context.Context) (domain.Session, error) {
	return e.startAutoUpdate(ctx, true)
}

func (e *Engine) startAutoUpdate(ctx context.Context, force bool) (session domain.Session, resultErr error) {
	settings, err := e.store.GetSettings(ctx)
	if err != nil || !settings.AutoUpdateEnabled {
		if err == nil {
			err = fmt.Errorf("Auto Update is disabled")
		}
		return domain.Session{}, err
	}
	var schedule store.AutoUpdateScheduleState
	var tickReceipt *domain.AutoUpdateTickReceipt
	defer func() {
		if tickReceipt == nil {
			return
		}
		completed := *tickReceipt
		completed.DecidedAt = domain.Now()
		if resultErr != nil {
			completed.Outcome = "skipped"
			completed.Reason = resultErr.Error()
		} else {
			completed.Outcome = "started"
			completed.SessionID = session.ID
		}
		if err := e.store.CompleteAutoUpdateSchedulerTick(context.Background(), completed); err != nil {
			e.logger.Printf("complete Auto Update scheduler receipt: %v", err)
		}
	}()
	if !force {
		schedule, err = e.store.AutoUpdateScheduleState(ctx)
		if err != nil {
			return domain.Session{}, err
		}
		now := time.Now()
		cadence := scheduledAutoUpdateCadence(settings, schedule, now)
		if _, due := nextScheduledAutoUpdateTick(schedule, cadence.Duration, now); !due {
			return domain.Session{}, nil
		}
		// Every scheduled tick is consumed before the shared safety, capacity,
		// and quota stoppers below. A skipped tick waits for its next cadence.
		receipt := domain.AutoUpdateTickReceipt{
			ID: domain.NewID("tick"), TickAt: now.Format(time.RFC3339Nano),
			Mode: settings.AutoUpdateMode, CadenceTier: cadence.Tier,
			CadenceMinutes: int(cadence.Duration / time.Minute),
			NextTickAt:     now.Add(cadence.Duration).Format(time.RFC3339Nano), Outcome: "checking",
		}
		if err := e.store.RecordAutoUpdateSchedulerTick(ctx, receipt); err != nil {
			return domain.Session{}, err
		}
		tickReceipt = &receipt
	}
	onboarding, err := e.store.Onboarding(ctx)
	if err != nil || onboarding.Status != "completed" {
		if err == nil {
			err = errors.New("complete onboarding before starting Auto Update")
		}
		return domain.Session{}, err
	}
	if e.BridgeStatus().Compatible == false {
		return domain.Session{}, fmt.Errorf("AkuBridge is not ready")
	}
	if active, activeErr := e.store.ActiveSession(ctx); activeErr != nil || active != nil {
		if activeErr != nil {
			return domain.Session{}, activeErr
		}
		return domain.Session{}, errors.New("another check is already running")
	}
	if calibration, calibrationErr := e.store.ActiveCalibration(ctx); calibrationErr != nil || calibration != nil {
		if calibrationErr != nil {
			return domain.Session{}, calibrationErr
		}
		return domain.Session{}, errors.New("complete the active calibration before starting Auto Update")
	}
	if firstRun, firstRunErr := e.store.CalibrationFirstRunStatus(ctx); firstRunErr != nil || firstRun == "pending" {
		if firstRunErr != nil {
			return domain.Session{}, firstRunErr
		}
		return domain.Session{}, errors.New("complete first-run calibration before starting Auto Update")
	}
	batches, err := e.store.PreparedBatches(ctx, settings.PreparedBatchMaxAgeHours)
	if err != nil || len(batches) >= settings.PreparedBatchLimit {
		if err == nil {
			err = errors.New("prepared batch limit reached")
		}
		return domain.Session{}, err
	}
	usage, err := e.store.AutoUpdateBudgetUsage(ctx)
	if err != nil {
		return domain.Session{}, err
	}
	autoLimit := int64(settings.AutoUpdateDailyTokenBudget * (100 - settings.AutoUpdateManualReservePct) / 100)
	estimatedTokens := e.estimatedAutoUpdateTokens(ctx)
	if usage.QuotaTotal+estimatedTokens > int64(settings.AutoUpdateDailyTokenBudget) || usage.QuotaAutomatic+estimatedTokens > autoLimit {
		return domain.Session{}, errors.New("automatic token allowance reached")
	}
	if err := e.store.RecordAutoUpdateAttempt(ctx, ""); err != nil {
		return domain.Session{}, err
	}
	trigger := domain.UpdateTriggerScheduler
	if force {
		trigger = domain.UpdateTriggerUser
	}
	session, err = e.startSession(context.Background(), "What materially changed since my last prepared batch?", domain.UpdatePolicy{
		Trigger: trigger, Delivery: domain.UpdateDeliveryPrepared, BudgetAuthority: domain.BudgetAuthorityAutomatic,
	})
	if err != nil {
		_ = e.store.RecordAutoUpdateAttempt(context.Background(), err.Error())
		return domain.Session{}, fmt.Errorf("start: %w", err)
	}
	return session, nil
}

func nextScheduledAutoUpdateTick(schedule store.AutoUpdateScheduleState, cadence time.Duration, now time.Time) (time.Time, bool) {
	lastTick, err := time.Parse(time.RFC3339Nano, schedule.LastSchedulerTickAt)
	if err != nil {
		return now, true
	}
	next := lastTick.Add(cadence)
	return next, !now.Before(next)
}

func scheduledAutoUpdateCadence(settings domain.Settings, schedule store.AutoUpdateScheduleState, now time.Time) autoUpdateCadence {
	if settings.AutoUpdateMode == "fixed" {
		return autoUpdateCadence{Tier: "continuous", Duration: time.Duration(settings.AutoUpdateRefillMinutes) * time.Minute}
	}
	access, err := time.Parse(time.RFC3339Nano, schedule.LastUIAccessAt)
	if err != nil || now.Sub(access) > autoUpdateWarmWindow {
		return autoUpdateCadence{Tier: "idle", Duration: autoUpdateIdleCadence}
	}
	if now.Sub(access) <= autoUpdateActiveWindow {
		return autoUpdateCadence{Tier: "active", Duration: autoUpdateActiveCadence}
	}
	return autoUpdateCadence{Tier: "warm", Duration: autoUpdateWarmCadence}
}
