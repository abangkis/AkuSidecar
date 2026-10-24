package engine

import (
	"context"
	"errors"

	"github.com/abangkis/AkuSidecar/internal/collection/headless"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/store"
)

// Batch admission is independent of scheduling demand and DOM mechanics. A
// challenge blocks the local X scope across collectors until explicit recovery.
func (e *Engine) captureBehaviorSources(ctx context.Context, sources []domain.Source) ([]domain.Source, error) {
	pause, err := e.store.SourceCapturePause(ctx)
	if err != nil {
		return nil, err
	}
	if pause == nil {
		return sources, nil
	}
	var allowed []domain.Source
	for _, source := range sources {
		if source != domain.SourceX {
			allowed = append(allowed, source)
		}
	}
	if len(allowed) == 0 {
		return nil, store.ErrSourceCapturePaused
	}
	return allowed, nil
}

func (e *Engine) beginCaptureBehavior(ctx context.Context, source domain.Source) (context.Context, func(), error) {
	if source != domain.SourceX {
		return ctx, func() {}, nil
	}
	e.captureBehaviorMu.Lock()
	defer e.captureBehaviorMu.Unlock()
	if pause, err := e.store.SourceCapturePause(ctx); err != nil {
		return nil, nil, err
	} else if pause != nil {
		return nil, nil, store.ErrSourceCapturePaused
	}
	work, cancel := context.WithCancel(ctx)
	key := domain.NewID("capture")
	if e.captureWork == nil {
		e.captureWork = map[string]context.CancelFunc{}
	}
	e.captureWork[key] = cancel
	return work, func() { cancel(); e.captureBehaviorMu.Lock(); delete(e.captureWork, key); e.captureBehaviorMu.Unlock() }, nil
}

func normalizeXChallenge(source domain.Source, err error) error {
	var capture *headless.CaptureError
	if source != domain.SourceX || !errors.As(err, &capture) || !domain.IsAccessChallenge(capture.Code) {
		return err
	}
	// The failure handler persists the latch with the exact active command.
	// Do not revoke that command here before its first failure can be committed.
	return &headless.CaptureError{Code: "challenge_required", Message: domain.XChallengeMessage, ChallengeCategory: capture.ChallengeCategory}
}

func (e *Engine) ConfirmXCaptureRecovery(ctx context.Context, id string) error {
	e.captureBehaviorMu.Lock()
	defer e.captureBehaviorMu.Unlock()
	if err := e.store.ConfirmXCaptureRecovery(ctx, id); err != nil {
		return err
	}
	select {
	case e.autoWake <- struct{}{}:
	default:
	}
	return nil
}
