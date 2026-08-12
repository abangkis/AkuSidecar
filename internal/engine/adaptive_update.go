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
const adaptiveStandbyCadence = 15 * time.Minute
const adaptiveGenerationWindow = 30 * time.Minute
const adaptiveDefaultPreparationLead = 8 * time.Minute
const adaptivePressureWindow = 60 * time.Minute
const adaptivePressureHalfLife = 30 * time.Minute

type adaptiveAutoUpdatePlan struct {
	BaseTarget            int
	Target                int
	RecentDemand          bool
	StandbyFloor          bool
	PriorityRefill        bool
	ConsumptionPace       time.Duration
	ConsumptionSamples    int
	PreparationLead       time.Duration
	PreparedItems         int
	RequiredReadyItems    int
	ReadingGraceUntil     time.Time
	AllowanceUsed         int
	AllowanceLimit        int
	NextAllowanceAt       time.Time
	LastYieldItems        int
	EmptyYieldStreak      int
	SupplyBackoffUntil    time.Time
	LastOutcome           store.AutoUpdateAdaptiveOutcome
	TechnicalStreak       int
	TechnicalBackoffUntil time.Time
	Pressure              int
	PressureTier          string
	PressureFromReveals   int
	PressureFromUpdates   int
	PressureFromYield     int
	PressureDelay         time.Duration
	PressureNotBefore     time.Time
	NextCheckAt           time.Time
	Eligible              bool
	Reason                string
}

func (e *Engine) adaptiveUpdatePlan(ctx context.Context, settings domain.Settings, schedule store.AutoUpdateScheduleState, batches []domain.PreparedBatch, now time.Time) (adaptiveAutoUpdatePlan, error) {
	signals, err := e.store.AutoUpdateAdaptiveSignals(ctx, adaptiveGenerationWindow, adaptivePressureWindow, adaptivePressureHalfLife, adaptiveDefaultPreparationLead)
	if err != nil {
		return adaptiveAutoUpdatePlan{}, err
	}
	return buildAdaptiveUpdatePlan(settings, schedule, batches, now, signals), nil
}

func buildAdaptiveUpdatePlan(settings domain.Settings, schedule store.AutoUpdateScheduleState, batches []domain.PreparedBatch, now time.Time, signals store.AutoUpdateAdaptiveSignals) adaptiveAutoUpdatePlan {
	recentDemand := recentTimelineDemand(schedule, now)
	baseTarget := adaptivePreparedTarget(settings.PreparedBatchLimit, signals.ConsumptionPace, signals.ConsumptionSamples, signals.PreparationLead)
	target, pressureTier, pressureDelay := adaptivePressurePolicy(baseTarget, signals.ReplenishmentPressure)
	if !recentDemand {
		target = 1
	}
	readiness := evaluateAdaptiveReadiness(settings, batches, target, signals.LastRevealAt, now)
	plan := adaptiveAutoUpdatePlan{
		BaseTarget:            baseTarget,
		Target:                target,
		RecentDemand:          recentDemand,
		StandbyFloor:          !recentDemand,
		ConsumptionPace:       signals.ConsumptionPace,
		ConsumptionSamples:    signals.ConsumptionSamples,
		PreparationLead:       signals.PreparationLead,
		PreparedItems:         readiness.PreparedItems,
		RequiredReadyItems:    readiness.RequiredItems,
		ReadingGraceUntil:     readiness.ReadingGraceUntil,
		AllowanceUsed:         len(signals.GenerationAttempts),
		AllowanceLimit:        settings.PreparedBatchLimit,
		LastYieldItems:        signals.LastYieldItems,
		EmptyYieldStreak:      signals.EmptyYieldStreak,
		SupplyBackoffUntil:    signals.SupplyBackoffUntil,
		LastOutcome:           signals.LastOutcome,
		TechnicalStreak:       signals.TechnicalStreak,
		TechnicalBackoffUntil: signals.TechnicalBackoffUntil,
		Pressure:              signals.ReplenishmentPressure,
		PressureTier:          pressureTier,
		PressureFromReveals:   signals.PressureFromReveals,
		PressureFromUpdates:   signals.PressureFromUpdates,
		PressureFromYield:     signals.PressureFromYield,
		PressureDelay:         pressureDelay,
	}
	if pressureDelay > 0 && !signals.LastOutcome.CompletedAt.IsZero() {
		plan.PressureNotBefore = signals.LastOutcome.CompletedAt.Add(pressureDelay)
	}
	if len(signals.GenerationAttempts) >= plan.AllowanceLimit && len(signals.GenerationAttempts) > 0 {
		plan.NextAllowanceAt = signals.GenerationAttempts[0].Add(adaptiveGenerationWindow)
	}
	if plan.RecentDemand && !signals.LastRevealAt.IsZero() {
		revealAge := now.Sub(signals.LastRevealAt)
		if revealAge >= 0 && revealAge <= adaptivePresenceWindow {
			lastTick, tickErr := time.Parse(time.RFC3339Nano, schedule.LastSchedulerTickAt)
			plan.PriorityRefill = tickErr != nil || signals.LastRevealAt.After(lastTick)
		}
	}

	switch {
	case readiness.BufferReady:
		if readiness.CapacityBounded && (readiness.PreparedBatches < readiness.RequiredBatches || readiness.PreparedItems < readiness.RequiredItems) {
			plan.Reason = fmt.Sprintf("Adaptive buffer reached bounded capacity (%d batches, %d items)", readiness.PreparedBatches, readiness.PreparedItems)
		} else {
			plan.Reason = fmt.Sprintf("Adaptive buffer ready (%d batches, %d items; target %d batches, %d items)", readiness.PreparedBatches, readiness.PreparedItems, readiness.RequiredBatches, readiness.RequiredItems)
		}
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
	case !plan.ReadingGraceUntil.IsZero() && now.Before(plan.ReadingGraceUntil):
		plan.NextCheckAt = plan.ReadingGraceUntil
		plan.Reason = "Reading grace is spacing refill after batch reveal"
		return plan
	case plan.PriorityRefill:
		plan.NextCheckAt = now
		plan.Eligible = true
		plan.Reason = "Post-reveal priority refill is ready"
		return plan
	case plan.StandbyFloor:
		next, due := nextScheduledAutoUpdateTick(schedule, adaptiveStandbyCadence, now)
		plan.NextCheckAt = next
		if !due {
			plan.Reason = "Waiting for the next standby refill opportunity"
			return plan
		}
		plan.NextCheckAt = now
		plan.Eligible = true
		plan.Reason = "Adaptive standby refill is ready"
		return plan
	case !plan.PressureNotBefore.IsZero() && now.Before(plan.PressureNotBefore):
		plan.NextCheckAt = plan.PressureNotBefore
		plan.Reason = "Replenishment pressure is spacing the next refill"
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

func recentTimelineDemand(schedule store.AutoUpdateScheduleState, now time.Time) bool {
	access, err := time.Parse(time.RFC3339Nano, schedule.LastUIAccessAt)
	if err != nil {
		return false
	}
	age := now.Sub(access)
	return age >= 0 && age <= adaptivePresenceWindow
}

func adaptivePressurePolicy(baseTarget, pressure int) (int, string, time.Duration) {
	if baseTarget < 1 {
		baseTarget = 1
	}
	switch {
	case pressure < 25:
		return baseTarget, "low", 0
	case pressure < 45:
		return baseTarget, "moderate", 5 * time.Minute
	case pressure < 65:
		return 1, "high", 10 * time.Minute
	default:
		return 1, "elevated", 15 * time.Minute
	}
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
	status.CadenceMinutes = int(adaptiveStandbyCadence / time.Minute)
	if plan.RecentDemand {
		status.CadenceTier = "demand"
		status.CadenceMinutes = int(adaptiveRefillCadence / time.Minute)
	}
	status.AdaptiveTargetBatches = plan.Target
	status.AdaptiveBaseTargetBatches = plan.BaseTarget
	status.ConsumptionPaceMinutes = roundedDurationMinutes(plan.ConsumptionPace)
	status.ConsumptionSamples = plan.ConsumptionSamples
	status.PreparationLeadMinutes = roundedDurationMinutes(plan.PreparationLead)
	status.AdaptiveReadyItems = plan.PreparedItems
	status.AdaptiveReadyItemTarget = plan.RequiredReadyItems
	if !plan.ReadingGraceUntil.IsZero() {
		status.AdaptiveReadingGraceUntil = plan.ReadingGraceUntil.Format(time.RFC3339Nano)
	}
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
	status.ReplenishmentPressure = plan.Pressure
	status.ReplenishmentPressureTier = plan.PressureTier
	status.PressureWindowMinutes = int(adaptivePressureWindow / time.Minute)
	status.PressureHalfLifeMinutes = int(adaptivePressureHalfLife / time.Minute)
	status.PressureFromReveals = plan.PressureFromReveals
	status.PressureFromUpdates = plan.PressureFromUpdates
	status.PressureFromYield = plan.PressureFromYield
	status.PressureAdditionalDelayMinutes = roundedDurationMinutes(plan.PressureDelay)
	if !plan.PressureNotBefore.IsZero() {
		status.PressureRefillNotBefore = plan.PressureNotBefore.Format(time.RFC3339Nano)
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
