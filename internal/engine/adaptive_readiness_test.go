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

func TestAdaptivePlanUsesReadingGraceOnlyAfterReveal(t *testing.T) {
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

	grace := buildAdaptiveUpdatePlan(settings, schedule, nil, now, signals)
	if grace.Eligible || grace.ReadingGraceUntil.IsZero() || !grace.NextCheckAt.Equal(now.Add(adaptiveReadingGrace)) || !strings.Contains(grace.Reason, "Reading grace") {
		t.Fatalf("grace plan=%+v", grace)
	}

	afterGrace := buildAdaptiveUpdatePlan(settings, schedule, nil, now.Add(adaptiveReadingGrace), signals)
	if !afterGrace.Eligible || !afterGrace.ReadingGraceUntil.IsZero() {
		t.Fatalf("after-grace plan=%+v", afterGrace)
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
