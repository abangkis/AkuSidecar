package engine

import (
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

const (
	adaptiveMinimumReadyItemsPerBatch = 3
	adaptiveReadingGrace              = 90 * time.Second
)

// adaptiveReadinessPolicy keeps the content-runway and post-reveal quiet-time
// rules private to adaptive scheduling. Continuous and manual update paths do
// not call this policy.
type adaptiveReadinessPolicy struct {
	PreparedBatches   int
	PreparedItems     int
	RequiredBatches   int
	RequiredItems     int
	BufferReady       bool
	CapacityBounded   bool
	ReadingGraceUntil time.Time
}

func evaluateAdaptiveReadiness(settings domain.Settings, batches []domain.PreparedBatch, target int, lastRevealAt, now time.Time) adaptiveReadinessPolicy {
	if target < 1 {
		target = 1
	}
	if settings.PreparedBatchLimit > 0 && target > settings.PreparedBatchLimit {
		target = settings.PreparedBatchLimit
	}

	itemsPerBatch := adaptiveMinimumReadyItemsPerBatch
	if settings.MaxItemsTotal > 0 && settings.MaxItemsTotal < itemsPerBatch {
		itemsPerBatch = settings.MaxItemsTotal
	}
	if itemsPerBatch < 1 {
		itemsPerBatch = 1
	}

	policy := adaptiveReadinessPolicy{
		PreparedBatches: len(batches),
		RequiredBatches: target,
		RequiredItems:   target * itemsPerBatch,
	}
	for _, batch := range batches {
		if batch.ItemCount > 0 {
			policy.PreparedItems += batch.ItemCount
		}
	}

	policy.CapacityBounded = settings.PreparedBatchLimit > 0 && policy.PreparedBatches >= settings.PreparedBatchLimit
	policy.BufferReady = policy.CapacityBounded || (policy.PreparedBatches >= policy.RequiredBatches && policy.PreparedItems >= policy.RequiredItems)
	if !lastRevealAt.IsZero() {
		graceUntil := lastRevealAt.Add(adaptiveReadingGrace)
		if now.Before(graceUntil) {
			policy.ReadingGraceUntil = graceUntil
		}
	}
	return policy
}
