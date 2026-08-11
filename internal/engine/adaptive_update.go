package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/store"
)

const adaptivePresenceWindow = 30 * time.Minute
const adaptiveRefillCadence = 5 * time.Minute
const adaptiveGenerationWindow = 30 * time.Minute
const adaptiveDefaultPreparationLead = 8 * time.Minute

type adaptiveAutoUpdatePlan struct {
	Target                int
	RecentDemand          bool
	ConsumptionPace       time.Duration
	ConsumptionSamples    int
	PreparationLead       time.Duration
	AllowanceUsed         int
	AllowanceLimit        int
	NextAllowanceAt       time.Time
	LastYieldItems        int
	EmptyYieldStreak      int
	SupplyBackoffUntil    time.Time
	LastOutcome           store.AutoUpdateAdaptiveOutcome
	TechnicalStreak       int
	TechnicalBackoffUntil time.Time
	NextCheckAt           time.Time
	Eligible              bool
	Reason                string
}

func (e *Engine) adaptiveUpdatePlan(ctx context.Context, settings domain.Settings, schedule store.AutoUpdateScheduleState, preparedCount int, now time.Time) (adaptiveAutoUpdatePlan, error) {
	signals, err := e.store.AutoUpdateAdaptiveSignals(ctx, adaptiveGenerationWindow, adaptiveDefaultPreparationLead)
	if err != nil {
		return adaptiveAutoUpdatePlan{}, err
	}
	return buildAdaptiveUpdatePlan(settings, schedule, preparedCount, now, signals), nil
}

func buildAdaptiveUpdatePlan(settings domain.Settings, schedule store.AutoUpdateScheduleState, preparedCount int, now time.Time, signals store.AutoUpdateAdaptiveSignals) adaptiveAutoUpdatePlan {
	plan := adaptiveAutoUpdatePlan{
		Target:                adaptivePreparedTarget(settings.PreparedBatchLimit, signals.ConsumptionPace, signals.ConsumptionSamples, signals.PreparationLead),
		ConsumptionPace:       signals.ConsumptionPace,
		ConsumptionSamples:    signals.ConsumptionSamples,
		PreparationLead:       signals.PreparationLead,
		AllowanceUsed:         len(signals.GenerationAttempts),
		AllowanceLimit:        settings.PreparedBatchLimit,
		LastYieldItems:        signals.LastYieldItems,
		EmptyYieldStreak:      signals.EmptyYieldStreak,
		SupplyBackoffUntil:    signals.SupplyBackoffUntil,
		LastOutcome:           signals.LastOutcome,
		TechnicalStreak:       signals.TechnicalStreak,
		TechnicalBackoffUntil: signals.TechnicalBackoffUntil,
	}
	if len(signals.GenerationAttempts) >= plan.AllowanceLimit && len(signals.GenerationAttempts) > 0 {
		plan.NextAllowanceAt = signals.GenerationAttempts[0].Add(adaptiveGenerationWindow)
	}
	if access, parseErr := time.Parse(time.RFC3339Nano, schedule.LastUIAccessAt); parseErr == nil {
		age := now.Sub(access)
		plan.RecentDemand = age >= 0 && age <= adaptivePresenceWindow
	}

	switch {
	case !plan.RecentDemand:
		plan.Reason = "Waiting for recent Timeline demand"
		return plan
	case preparedCount >= plan.Target:
		plan.Reason = fmt.Sprintf("Adaptive buffer ready (%d of %d target)", preparedCount, plan.Target)
		return plan
	case !plan.SupplyBackoffUntil.IsZero() && now.Before(plan.SupplyBackoffUntil):
		plan.NextCheckAt = plan.SupplyBackoffUntil
		plan.Reason = "Fresh-content supply is cooling down after an empty update"
		return plan
	case !plan.TechnicalBackoffUntil.IsZero() && now.Before(plan.TechnicalBackoffUntil):
		plan.NextCheckAt = plan.TechnicalBackoffUntil
		plan.Reason = "Previous update failed technically; waiting before retry"
		return plan
	case plan.AllowanceUsed >= plan.AllowanceLimit:
		plan.NextCheckAt = plan.NextAllowanceAt
		plan.Reason = "Bounded generation allowance reached"
		return plan
	}

	next, due := nextScheduledAutoUpdateTick(schedule, adaptiveRefillCadence, now)
	plan.NextCheckAt = next
	if !due {
		plan.Reason = "Waiting for the next adaptive refill opportunity"
		return plan
	}
	plan.NextCheckAt = now
	plan.Eligible = true
	plan.Reason = "Adaptive refill is ready"
	return plan
}

func adaptivePreparedTarget(limit int, consumptionPace time.Duration, samples int, preparationLead time.Duration) int {
	if limit < 1 {
		return 1
	}
	if samples < 1 || consumptionPace <= 0 || preparationLead <= 0 {
		return 1
	}
	target := int((preparationLead + consumptionPace - 1) / consumptionPace)
	if target < 1 {
		target = 1
	}
	if target > limit {
		target = limit
	}
	return target
}

func applyAdaptiveStatus(status *domain.AutoUpdateStatus, plan adaptiveAutoUpdatePlan) {
	status.RecentUserActivity = plan.RecentDemand
	status.ActivityWindowMinutes = int(adaptivePresenceWindow / time.Minute)
	status.CadenceTier = "standby"
	status.CadenceMinutes = 0
	if plan.RecentDemand {
		status.CadenceTier = "demand"
		status.CadenceMinutes = int(adaptiveRefillCadence / time.Minute)
	}
	status.AdaptiveTargetBatches = plan.Target
	status.ConsumptionPaceMinutes = roundedDurationMinutes(plan.ConsumptionPace)
	status.ConsumptionSamples = plan.ConsumptionSamples
	status.PreparationLeadMinutes = roundedDurationMinutes(plan.PreparationLead)
	status.GenerationWindowMinutes = int(adaptiveGenerationWindow / time.Minute)
	status.GenerationAllowanceUsed = plan.AllowanceUsed
	status.GenerationAllowanceLimit = plan.AllowanceLimit
	status.LastPreparedYieldItems = plan.LastYieldItems
	status.EmptyYieldStreak = plan.EmptyYieldStreak
	if !plan.SupplyBackoffUntil.IsZero() {
		status.SupplyBackoffUntil = plan.SupplyBackoffUntil.Format(time.RFC3339Nano)
	}
	status.LastAdaptiveOutcome = plan.LastOutcome.Kind
	if !plan.LastOutcome.CompletedAt.IsZero() {
		status.LastAdaptiveOutcomeAt = plan.LastOutcome.CompletedAt.Format(time.RFC3339Nano)
	}
	status.LastAdaptiveOutcomeItems = plan.LastOutcome.ItemCount
	status.LastAdaptiveOutcomeTrigger = plan.LastOutcome.Trigger
	status.LastAdaptiveOutcomeCompletedSources = plan.LastOutcome.CompletedRuns
	status.LastAdaptiveOutcomeFailedSources = plan.LastOutcome.FailedRuns
	status.LastAdaptiveOutcomeCancelledSources = plan.LastOutcome.CancelledRuns
	status.TechnicalFailureStreak = plan.TechnicalStreak
	if !plan.TechnicalBackoffUntil.IsZero() {
		status.TechnicalBackoffUntil = plan.TechnicalBackoffUntil.Format(time.RFC3339Nano)
	}
	if !plan.NextCheckAt.IsZero() {
		status.NextCheckAt = plan.NextCheckAt.Format(time.RFC3339Nano)
	}
	status.Reason = plan.Reason
}

func roundedDurationMinutes(value time.Duration) int {
	if value <= 0 {
		return 0
	}
	return int((value + 30*time.Second) / time.Minute)
}
