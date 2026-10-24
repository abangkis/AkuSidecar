package engine

import (
	"context"
	"github.com/abangkis/AkuSidecar/internal/batchbehavior"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"time"
)

func (e *Engine) capturePacingPolicy() batchbehavior.Policy {
	value := e.config.Capture.Behavior.Effective()
	return batchbehavior.Policy{Enabled: value.Enabled, Interval: time.Duration(value.MinCaptureIntervalMS) * time.Millisecond, Jitter: time.Duration(value.AdditionalJitterMS) * time.Millisecond, BackoffBase: time.Duration(value.TemporaryBackoffBaseMS) * time.Millisecond, BackoffMax: time.Duration(value.TemporaryBackoffMaxMS) * time.Millisecond}
}

func (e *Engine) captureBehaviorStatus(ctx context.Context, sources []domain.Source) ([]domain.CaptureBehaviorStatus, error) {
	policy := e.capturePacingPolicy()
	if !policy.Enabled {
		return nil, nil
	}
	var result []domain.CaptureBehaviorStatus
	for _, source := range sources {
		decision, err := e.store.CaptureBehaviorDecision(ctx, source, policy)
		if err != nil {
			return nil, err
		}
		status := domain.CaptureBehaviorStatus{Enabled: true, Source: source, Allowed: decision.Allowed, Reason: decision.Reason}
		if !decision.NotBefore.IsZero() {
			status.NotBefore = decision.NotBefore.UTC().Format(time.RFC3339Nano)
		}
		result = append(result, status)
	}
	return result, nil
}
