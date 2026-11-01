package engine

import (
	"context"
	"time"
)

// autoUpdateLifecycle owns the start-once handle and an immutable wake channel.
// Engine.mu protects start/cancel state. Notifications use the channel directly
// so producers retain their existing lock scopes and nonblocking behavior.
type autoUpdateLifecycle struct {
	cancel context.CancelFunc
	wake   chan struct{}
}

func newAutoUpdateLifecycle() autoUpdateLifecycle {
	return autoUpdateLifecycle{wake: make(chan struct{}, 1)}
}

// hasStarted remains true after cancellation: this is an admission fact, not a
// running-status projection, and is also consumed by the maintenance guard.
func (s *autoUpdateLifecycle) hasStarted() bool { return s.cancel != nil }

func (s *autoUpdateLifecycle) startOnce() (context.Context, bool) {
	if s.hasStarted() {
		return nil, false
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	return ctx, true
}

func (s *autoUpdateLifecycle) stop() {
	if s.cancel != nil {
		s.cancel()
	}
}

func (s *autoUpdateLifecycle) notify() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (e *Engine) StartAutoUpdateScheduler() {
	e.mu.Lock()
	ctx, started := e.scheduler.startOnce()
	if !started {
		e.mu.Unlock()
		return
	}
	e.mu.Unlock()
	go e.autoUpdateLoop(ctx)
}

func (e *Engine) autoUpdateLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	e.autoUpdateLoopOnTicks(ctx, ticker.C)
}

// The private tick-channel seam permits accountless loop tests without changing
// the production ticker, cancellation select or introducing a drain contract.
func (e *Engine) autoUpdateLoopOnTicks(ctx context.Context, ticks <-chan time.Time) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticks:
		case <-e.scheduler.wake:
		}
		// Recovery must also run when the Bridge is silent or Auto Update is disabled.
		if err := e.recoverExpiredBridgeCommands(ctx, time.Now()); err != nil {
			e.logger.Printf("recover expired Bridge command: %v", err)
		}
		if err := e.maybeStartAutoUpdate(ctx); err != nil {
			e.logger.Printf("Auto Update deferred: %v", err)
		}
	}
}
