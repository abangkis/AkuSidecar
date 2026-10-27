package engine

import (
	"context"
	"errors"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
)

// Private trusted composition supplies these ports. The process reader and
// its authorization outlive successful batches; every collector is disposable.
type xDailyNativeOwner interface {
	BeginBatch(context.Context, context.Context, uint64, domain.Run) (xDailyNativeBatch, error)
	PrepareReader(context.Context, readerbroker.Request, string) (readerbroker.NativePostPreparation, error)
	CheckReader(context.Context) error
	Close(context.Context) error
}
type xDailyNativeBatch interface {
	StartedAt() time.Time
	Deadline() time.Time
	Capture(context.Context, string) (domain.Run, error)
	Close(context.Context) error
}

type XDailyStartRequest struct {
	AcknowledgeLimitations        bool `json:"acknowledgeLimitations"`
	AllowProcessSessionDerivation bool `json:"allowProcessSessionDerivation"`
}

func (e *Engine) configureXDailyFactory(parent context.Context, factory func(context.Context, context.Context) (xDailyNativeOwner, error)) error {
	if !e.XDailyConfigured() || !e.config.Dev || parent == nil || factory == nil || parent.Err() != nil {
		return errors.New("daily X factory rejected")
	}
	e.xDaily.mu.Lock()
	defer e.xDaily.mu.Unlock()
	if !e.xDaily.attached || e.xDaily.factory != nil || e.xDaily.owner != nil || e.xDaily.closed {
		return errors.New("daily X factory already selected")
	}
	e.xDaily.parent = parent
	e.xDaily.factory = factory
	return nil
}

func (e *Engine) StartXDaily(ctx context.Context, request XDailyStartRequest) error {
	if !e.XDailyConfigured() || !e.config.Dev || ctx == nil || ctx.Err() != nil || !request.AcknowledgeLimitations || !request.AllowProcessSessionDerivation {
		return errors.New("daily X process consent required")
	}
	e.xDaily.mu.Lock()
	if e.xDaily.closed || e.xDaily.reason != "" || e.xDaily.startUsed || e.xDaily.factory == nil {
		e.xDaily.mu.Unlock()
		return errors.New("daily X process start unavailable or consumed")
	}
	e.xDaily.startUsed = true
	factory, parent := e.xDaily.factory, e.xDaily.parent
	call, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	e.xDaily.startDone = done
	e.xDaily.cancelStart = cancel
	e.xDaily.mu.Unlock()
	stopParent := context.AfterFunc(parent, cancel)
	defer func() {
		stopParent()
		cancel()
		e.xDaily.mu.Lock()
		e.xDaily.cancelStart = nil
		close(done)
		e.xDaily.mu.Unlock()
	}()
	owner, err := factory(call, parent)
	if err == nil && call.Err() != nil {
		err = call.Err()
	}
	if err == nil {
		err = e.installXDailyOwner(parent, owner)
	}
	if err != nil {
		cleanupFailed := false
		if owner != nil {
			cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			cleanupFailed = owner.Close(cleanup) != nil
			cancel()
		}
		e.xDaily.mu.Lock()
		if cleanupFailed {
			e.xDaily.owner = owner
			e.xDaily.cleanupUncertain = true
		}
		if e.xDaily.reason == "" {
			e.xDaily.reason = "reader-start-failed"
			e.xDaily.failurePhase = "reader-startup"
			e.xDaily.firstFailure = err
			e.xDaily.failureCode = xDailyClosedFailureCode(err)
		}
		e.xDaily.mu.Unlock()
		return errors.New("daily X reader startup failed")
	}
	return nil
}

func (e *Engine) installXDailyOwner(parent context.Context, owner xDailyNativeOwner) error {
	if !e.XDailyConfigured() || !e.config.Dev || parent == nil || parent.Err() != nil || owner == nil {
		return errors.New("daily X owner selection rejected")
	}
	if err := owner.CheckReader(parent); err != nil {
		return errors.New("daily X reader unavailable")
	}
	e.xDaily.mu.Lock()
	defer e.xDaily.mu.Unlock()
	if !e.xDaily.attached || e.xDaily.closed || e.xDaily.reason != "" || e.xDaily.owner != nil {
		return errors.New("daily X process authorization unavailable")
	}
	e.xDaily.owner = owner
	e.xDaily.parent = parent
	e.xDaily.authorized = true
	return nil
}

func (e *Engine) xDailyReady() bool {
	e.xDaily.mu.RLock()
	defer e.xDaily.mu.RUnlock()
	return e.XDailyConfigured() && e.xDaily.owner != nil && e.xDaily.authorized && !e.xDaily.closed && e.xDaily.parent != nil && e.xDaily.parent.Err() == nil
}

func (e *Engine) prepareXDailyReader(ctx context.Context, req readerbroker.Request, marker string) (readerbroker.NativePostPreparation, error) {
	e.xDaily.mu.RLock()
	owner := e.xDaily.owner
	parent := e.xDaily.parent
	e.xDaily.mu.RUnlock()
	if owner == nil || parent == nil || parent.Err() != nil {
		return readerbroker.NativePostPreparation{}, errors.New("daily X reader unavailable")
	}
	if err := owner.CheckReader(ctx); err != nil {
		return readerbroker.NativePostPreparation{}, errors.New("daily X reader ownership changed")
	}
	return owner.PrepareReader(ctx, req, marker)
}

func (e *Engine) queueXDailyBatch(run domain.Run, payload map[string]any) error {
	e.xDaily.mu.Lock()
	if e.xDaily.done != nil && e.xDaily.runID == run.ID {
		e.xDaily.mu.Unlock()
		return nil
	}
	if !e.xDaily.authorized || e.xDaily.closed || e.xDaily.owner == nil || e.xDaily.done != nil || e.xDaily.generation == ^uint64(0) {
		e.xDaily.mu.Unlock()
		return errors.New("daily X batch unavailable")
	}
	e.xDaily.generation++
	generation := e.xDaily.generation
	owner := e.xDaily.owner
	parent := e.xDaily.parent
	work, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	e.xDaily.done = done
	e.xDaily.runID = run.ID
	if e.xDaily.workers == 0 {
		e.xDaily.drain = make(chan struct{})
	}
	e.xDaily.workers++
	e.xDaily.cancel = cancel
	e.xDaily.mu.Unlock()
	e.mu.Lock()
	e.processWorkers++
	e.mu.Unlock()
	go func() {
		defer func() {
			e.mu.Lock()
			e.processWorkers--
			e.mu.Unlock()
			e.xDaily.mu.Lock()
			e.xDaily.workers--
			if e.xDaily.workers == 0 {
				close(e.xDaily.drain)
			}
			e.xDaily.mu.Unlock()
		}()
		var batch xDailyNativeBatch
		var cause error
		phase := "reader-check"
		defer func() {
			cancel()
			cleanup, stop := context.WithTimeout(context.Background(), 30*time.Second)
			cleanupFailed := false
			if batch != nil {
				if err := batch.Close(cleanup); err != nil {
					cleanupFailed = true
					if cause == nil {
						cause = errors.New("daily X collector cleanup uncertain")
						phase = "cleanup"
					}
				}
			}
			if cause != nil && !cleanupFailed {
				if err := e.store.ReconcileClosedDailyXCapture(cleanup, run.ID); err != nil {
					cleanupFailed = true
				}
			}
			stop()
			e.xDaily.mu.Lock()
			if cleanupFailed {
				e.xDaily.cleanupUncertain = true
			}
			if cause != nil {
				e.xDaily.authorized = false
				if e.xDaily.reason == "" {
					e.xDaily.reason = "batch-failed"
					e.xDaily.failurePhase = phase
					e.xDaily.firstFailure = cause
					e.xDaily.failureCode = xDailyClosedFailureCode(cause)
				}
			}
			// Generation and done identity prevent a late completion retiring a successor.
			if e.xDaily.generation == generation && e.xDaily.done == done {
				e.xDaily.done = nil
				e.xDaily.runID = ""
				e.xDaily.cancel = nil
			}
			close(done)
			e.xDaily.mu.Unlock()
			if cause != nil {
				e.logger.Printf("x_daily_batch phase=terminal reason=batch-failed generation=%d", generation)
			}
			if !cleanupFailed {
				_, _ = e.startNext(context.Background(), run.SessionID)
			}
		}()
		if err := owner.CheckReader(work); err != nil {
			cause = err
			return
		}
		// Register the cancel/join worker before persisting its command. Stop
		// therefore always has an owner for SQL/startup and cannot orphan work
		// between durable StartRun and batch registration.
		phase = "command"
		command, err := e.store.StartRun(work, run.ID, payload)
		if err != nil {
			cause = err
			return
		}
		// Native startup starts the original clock at seed, not at queue admission.
		startup, stop := context.WithCancel(work)
		phase = "startup"
		batch, cause = owner.BeginBatch(startup, parent, generation, run)
		stop()
		if cause != nil || batch == nil {
			if cause == nil {
				cause = errors.New("daily X batch absent")
			}
			return
		}
		start, end := batch.StartedAt(), batch.Deadline()
		phase = "deadline"
		if start.IsZero() || start.After(e.store.Now()) || !end.After(e.store.Now()) || end.After(start.Add(60*time.Second)) {
			cause = errors.New("daily X batch deadline rejected")
			return
		}
		capture, stop := context.WithDeadline(work, end)
		phase = "capture"
		_, cause = batch.Capture(capture, command.ID)
		stop()
	}()
	return nil
}

func xDailyClosedFailureCode(err error) string {
	var carrier interface{ dailyFailureCode() string }
	if errors.As(err, &carrier) {
		return carrier.dailyFailureCode()
	}
	return ""
}

func (e *Engine) stopXDaily(ctx context.Context) error {
	e.xDaily.mu.Lock()
	e.xDaily.authorized = false
	if e.xDaily.reason == "" {
		e.xDaily.reason = "opted-out"
	}
	cancel, done, startCancel, startDone := e.xDaily.cancel, e.xDaily.done, e.xDaily.cancelStart, e.xDaily.startDone
	e.xDaily.mu.Unlock()
	if startCancel != nil {
		startCancel()
	}
	if startDone != nil {
		select {
		case <-startDone:
		case <-ctx.Done():
			return errors.New("daily X startup cleanup pending")
		}
	}
	if cancel != nil {
		cancel()
	}
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			return errors.New("daily X batch cleanup pending")
		}
	}
	e.xDaily.mu.RLock()
	uncertain := e.xDaily.cleanupUncertain
	e.xDaily.mu.RUnlock()
	if uncertain {
		return errors.New("daily X batch cleanup uncertain")
	}
	return nil
}

func (e *Engine) closeXDaily(ctx context.Context) error {
	stopErr := e.stopXDaily(ctx)
	e.xDaily.mu.Lock()
	e.xDaily.closed = true
	owner := e.xDaily.owner
	e.xDaily.mu.Unlock()
	if owner != nil {
		if err := owner.Close(ctx); err != nil {
			return err
		}
		e.xDaily.mu.RLock()
		done, start, drain := e.xDaily.done, e.xDaily.startDone, e.xDaily.drain
		e.xDaily.mu.RUnlock()
		for _, ended := range []<-chan struct{}{start, done, drain} {
			if ended != nil {
				select {
				case <-ended:
				case <-ctx.Done():
					return errors.New("daily X process cleanup pending")
				}
			}
		}
		e.xDaily.mu.Lock()
		e.xDaily.cleanupUncertain = false
		e.xDaily.mu.Unlock()
		return nil
	}
	if stopErr != nil {
		return stopErr
	}
	return nil
}
