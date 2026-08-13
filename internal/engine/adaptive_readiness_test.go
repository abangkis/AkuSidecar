package engine

import (
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/store"
)

func adaptiveTestBatches(itemCounts ...int) []domain.PreparedBatch {
	batches := make([]domain.PreparedBatch, 0, len(itemCounts))
	for _, itemCount := range itemCounts {
		batches = append(batches, domain.PreparedBatch{ItemCount: itemCount})
	}
	return batches
}

func TestAdaptiveReadinessRequiresContentRunway(t *testing.T) {
	settings := domain.DefaultSettings("standard", "quiet", "guarded_live", true)
	settings.PreparedBatchLimit = 3

	thin := evaluateAdaptiveReadiness(settings, adaptiveTestBatches(1), 1, time.Time{}, time.Now())
	if thin.BufferReady || thin.PreparedItems != 1 || thin.RequiredItems != 3 {
		t.Fatalf("thin readiness=%+v", thin)
	}

	healthy := evaluateAdaptiveReadiness(settings, adaptiveTestBatches(3), 1, time.Time{}, time.Now())
	if !healthy.BufferReady || healthy.PreparedItems != 3 {
		t.Fatalf("healthy readiness=%+v", healthy)
	}

	bounded := evaluateAdaptiveReadiness(settings, adaptiveTestBatches(1, 1, 1), 1, time.Time{}, time.Now())
	if !bounded.BufferReady || !bounded.CapacityBounded {
		t.Fatalf("bounded readiness=%+v", bounded)
	}
}

func TestAdaptivePlanDoesNotDelayRefillAfterReveal(t *testing.T) {
	now := time.Date(2026, time.August, 11, 9, 9, 42, 0, time.UTC)
	settings := domain.DefaultSettings("standard", "quiet", "guarded_live", true)
	settings.AutoUpdateMode = "adaptive"
	settings.PreparedBatchLimit = 3
	schedule := store.AutoUpdateScheduleState{
		LastUIAccessAt:      now.Format(time.RFC3339Nano),
		LastSchedulerTickAt: now.Add(-20 * time.Minute).Format(time.RFC3339Nano),
	}
	signals := store.AutoUpdateAdaptiveSignals{
		PreparationLead: 8 * time.Minute,
		LastRevealAt:    now,
	}

	immediate := buildAdaptiveUpdatePlan(settings, schedule, nil, now, signals)
	if !immediate.Eligible || !immediate.ReadingGraceUntil.IsZero() || !immediate.PriorityRefill {
		t.Fatalf("immediate refill plan=%+v", immediate)
	}
}

func TestAdaptivePlanSupplementsThinPreparedBatch(t *testing.T) {
	now := time.Date(2026, time.August, 11, 9, 0, 0, 0, time.UTC)
	settings := domain.DefaultSettings("standard", "quiet", "guarded_live", true)
	settings.AutoUpdateMode = "adaptive"
	settings.PreparedBatchLimit = 3
	schedule := store.AutoUpdateScheduleState{
		LastUIAccessAt:      now.Format(time.RFC3339Nano),
		LastSchedulerTickAt: now.Add(-6 * time.Minute).Format(time.RFC3339Nano),
	}
	signals := store.AutoUpdateAdaptiveSignals{PreparationLead: 8 * time.Minute}

	plan := buildAdaptiveUpdatePlan(settings, schedule, adaptiveTestBatches(1), now, signals)
	if !plan.Eligible || plan.PreparedItems != 1 || plan.RequiredReadyItems != 3 {
		t.Fatalf("thin-buffer plan=%+v", plan)
	}
}

func TestAdaptiveIdleMaintainsOneHealthyStandbyBatch(t *testing.T) {
	now := time.Date(2026, time.August, 12, 8, 0, 0, 0, time.UTC)
	settings := domain.DefaultSettings("standard", "quiet", "guarded_live", true)
	settings.AutoUpdateMode = "adaptive"
	settings.PreparedBatchLimit = 3
	signals := store.AutoUpdateAdaptiveSignals{
		ConsumptionPace:       2 * time.Minute,
		ConsumptionSamples:    3,
		PreparationLead:       8 * time.Minute,
		ReplenishmentPressure: 80,
		LastOutcome: store.AutoUpdateAdaptiveOutcome{
			CompletedAt: now.Add(-time.Minute),
			Kind:        "productive",
		},
	}

	empty := buildAdaptiveUpdatePlan(settings, store.AutoUpdateScheduleState{}, nil, now, signals)
	if !empty.Eligible || !empty.StandbyFloor || empty.RecentDemand || empty.BaseTarget != 3 || empty.Target != 1 || empty.RequiredReadyItems != 3 {
		t.Fatalf("empty standby plan=%+v", empty)
	}

	healthy := buildAdaptiveUpdatePlan(settings, store.AutoUpdateScheduleState{}, adaptiveTestBatches(3), now, signals)
	if healthy.Eligible || !healthy.StandbyFloor || !strings.Contains(healthy.Reason, "buffer ready") {
		t.Fatalf("healthy standby plan=%+v", healthy)
	}

	lastTick := now.Add(-10 * time.Minute)
	waiting := buildAdaptiveUpdatePlan(settings, store.AutoUpdateScheduleState{
		LastSchedulerTickAt: lastTick.Format(time.RFC3339Nano),
	}, nil, now, signals)
	if waiting.Eligible || !waiting.NextCheckAt.Equal(lastTick.Add(adaptiveStandbyCadence)) || !strings.Contains(waiting.Reason, "standby refill opportunity") {
		t.Fatalf("waiting standby plan=%+v", waiting)
	}
}

func TestAdaptiveRevealGetsOneImmediatePriorityRefill(t *testing.T) {
	now := time.Date(2026, time.August, 12, 9, 0, 0, 0, time.UTC)
	settings := domain.DefaultSettings("standard", "quiet", "guarded_live", true)
	settings.AutoUpdateMode = "adaptive"
	settings.PreparedBatchLimit = 3
	signals := store.AutoUpdateAdaptiveSignals{
		PreparationLead:       8 * time.Minute,
		LastRevealAt:          now,
		ReplenishmentPressure: 80,
		LastOutcome: store.AutoUpdateAdaptiveOutcome{
			CompletedAt: now.Add(-time.Minute),
			Kind:        "productive",
		},
	}
	schedule := store.AutoUpdateScheduleState{
		LastUIAccessAt:      now.Format(time.RFC3339Nano),
		LastSchedulerTickAt: now.Add(-time.Minute).Format(time.RFC3339Nano),
	}

	immediate := buildAdaptiveUpdatePlan(settings, schedule, nil, now, signals)
	if !immediate.Eligible || !immediate.PriorityRefill || immediate.Reason != "Post-reveal priority refill is ready" {
		t.Fatalf("priority plan=%+v", immediate)
	}

	schedule.LastSchedulerTickAt = now.Format(time.RFC3339Nano)
	consumed := buildAdaptiveUpdatePlan(settings, schedule, nil, now, signals)
	if consumed.Eligible || consumed.PriorityRefill || consumed.Reason != "Replenishment pressure is spacing the next refill" {
		t.Fatalf("consumed priority plan=%+v", consumed)
	}
}
