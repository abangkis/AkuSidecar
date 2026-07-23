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
const autoUpdateRecentActivityWindow = 15 * time.Minute

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
	if active, activeErr := e.store.ActiveSession(ctx); activeErr != nil {
		return domain.AutoUpdateStatus{}, activeErr
	} else if active != nil {
		if automaticSession, _ := e.store.IsAutoSession(ctx, active.ID); automaticSession {
			status.State, status.Reason = "running", "Preparing a bounded batch"
		} else {
			status.State, status.Reason = "paused", "Manual check is running"
		}
		return status, nil
	}
	if !settings.AutoUpdateEnabled {
		status.State, status.Reason = "disabled", "Auto Update is off"
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
	schedule, err := e.store.AutoUpdateScheduleState(ctx)
	if err != nil {
		return domain.AutoUpdateStatus{}, err
	}
	if settings.AutoUpdateMode == "adaptive" && !hasRecentAutoUpdateActivity(schedule, time.Now()) {
		status.State, status.Reason = "paused", "Waiting for recent user activity"
		return status, nil
	}
	if boundary, ok := latestAutoUpdateBoundary(schedule); ok {
		next := boundary.Add(autoUpdateRefillDuration(settings))
		if time.Now().Before(next) {
			status.Reason = "Waiting to refill an open prepared-batch slot"
			status.NextCheckAt = next.Format(time.RFC3339Nano)
		}
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

func (e *Engine) RevealPreparedBatch(ctx context.Context, sessionID string) (domain.PreparedBatch, error) {
	e.operation.Lock()
	defer e.operation.Unlock()
	batch, err := e.store.RevealPreparedBatch(ctx, sessionID)
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

// StartAutoUpdateNow starts an automatic session on explicit user request.
// It keeps all safety and budget gates, but intentionally bypasses the
// scheduler's cadence and adaptive recent-use gates.
func (e *Engine) StartAutoUpdateNow(ctx context.Context) (domain.Session, error) {
	return e.startAutoUpdate(ctx, true)
}

func (e *Engine) startAutoUpdate(ctx context.Context, force bool) (domain.Session, error) {
	settings, err := e.store.GetSettings(ctx)
	if err != nil || !settings.AutoUpdateEnabled {
		if err == nil {
			err = fmt.Errorf("Auto Update is disabled")
		}
		return domain.Session{}, err
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
	schedule, err := e.store.AutoUpdateScheduleState(ctx)
	if err != nil {
		return domain.Session{}, err
	}
	if !force {
		if boundary, ok := latestAutoUpdateBoundary(schedule); ok && time.Since(boundary) < autoUpdateRefillDuration(settings) {
			return domain.Session{}, nil
		}
		if settings.AutoUpdateMode == "adaptive" && !hasRecentAutoUpdateActivity(schedule, time.Now()) {
			return domain.Session{}, nil
		}
	}
	if err := e.store.RecordAutoUpdateAttempt(ctx, ""); err != nil {
		return domain.Session{}, err
	}
	session, err := e.startSession(context.Background(), "What materially changed since my last automatic check?", true)
	if err != nil {
		_ = e.store.RecordAutoUpdateAttempt(context.Background(), err.Error())
		return domain.Session{}, fmt.Errorf("start: %w", err)
	}
	return session, nil
}

func autoUpdateRefillDuration(settings domain.Settings) time.Duration {
	return time.Duration(settings.AutoUpdateRefillMinutes) * time.Minute
}

func latestAutoUpdateBoundary(schedule store.AutoUpdateScheduleState) (time.Time, bool) {
	var latest time.Time
	for _, raw := range []string{schedule.LastAttemptAt, schedule.LastQueueVacancyAt} {
		value, err := time.Parse(time.RFC3339Nano, raw)
		if err == nil && value.After(latest) {
			latest = value
		}
	}
	return latest, !latest.IsZero()
}

func hasRecentAutoUpdateActivity(schedule store.AutoUpdateScheduleState, now time.Time) bool {
	access, err := time.Parse(time.RFC3339Nano, schedule.LastUIAccessAt)
	return err == nil && now.Sub(access) <= autoUpdateRecentActivityWindow
}
