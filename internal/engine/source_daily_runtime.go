package engine

import (
	"context"
	"errors"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
)

// dailyController owns source-scoped orchestration; providers own proof and session transport.
type dailyController struct {
	engine *Engine
	state  *sourceDailyRouting
	source domain.Source
}

func (d dailyController) configure(parent context.Context, factory func(context.Context, context.Context) (sourceDailyOwner, error), profileScopes ...string) error {
	e := d.engine
	if !d.configured() || !e.config.Dev || parent == nil || factory == nil || parent.Err() != nil {
		return d.failure("factory rejected")
	}
	if len(profileScopes) > 1 {
		return d.failure("factory profile scope rejected")
	}
	profileScope := xDailyProfileScope(e.config.Root, e.config.Database.Path, "synthetic-root", "synthetic-directory")
	if len(profileScopes) == 1 {
		profileScope = profileScopes[0]
	}
	if !validXDailyProfileScope(profileScope) {
		return d.failure("factory profile scope rejected")
	}
	d.state.mu.Lock()
	defer d.state.mu.Unlock()
	if !d.state.attached || d.state.factory != nil || d.state.owner != nil || d.state.closed {
		return d.failure("factory already selected")
	}
	d.state.parent = parent
	d.state.factory = factory
	d.state.profileScope = profileScope
	return nil
}

func (d dailyController) start(ctx context.Context, request XDailyStartRequest, restoreRevision *uint64) (bool, error) {
	e := d.engine
	if !d.configured() || !e.config.Dev || ctx == nil || ctx.Err() != nil || !request.AcknowledgeLimitations || !request.AllowProcessSessionDerivation {
		return false, d.failure("process consent required")
	}
	d.state.permissionMu.Lock()
	d.state.mu.Lock()
	if restoreRevision != nil && d.state.permissionRevision != *restoreRevision {
		d.state.mu.Unlock()
		d.state.permissionMu.Unlock()
		return false, nil
	}
	if d.state.closed || d.state.reason != "" || d.state.startUsed || d.state.factory == nil {
		d.state.mu.Unlock()
		d.state.permissionMu.Unlock()
		return false, d.failure("process start unavailable or consumed")
	}
	d.state.startUsed = true
	d.state.permissionRevision++
	factory, parent, profileScope := d.state.factory, d.state.parent, d.state.profileScope
	call, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	d.state.startDone = done
	d.state.cancelStart = cancel
	d.state.mu.Unlock()
	stopParent := context.AfterFunc(parent, cancel)
	defer func() {
		stopParent()
		cancel()
		d.state.mu.Lock()
		if d.state.startDone == done {
			d.state.cancelStart = nil
		}
		close(done)
		d.state.mu.Unlock()
	}()
	if err := e.store.SaveDailySourcePermission(ctx, d.source, profileScope, true); err != nil {
		d.state.mu.Lock()
		d.state.startUsed = false
		d.state.mu.Unlock()
		d.state.permissionMu.Unlock()
		return true, d.failure("permission could not be saved")
	}
	d.state.mu.Lock()
	d.state.rememberedPermission = true
	d.state.mu.Unlock()
	d.state.permissionMu.Unlock()
	owner, err := factory(call, parent)
	if err == nil && call.Err() != nil {
		err = call.Err()
	}
	if err == nil {
		err = d.install(parent, owner)
	}
	if err != nil {
		cleanupFailed := false
		if owner != nil {
			cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			if cleanupErr := owner.Close(cleanup); cleanupErr != nil {
				cleanupFailed = true
				d.rememberCleanup(xDailyCleanupFail(cleanupErr, "owner_close", "unverified"))
			}
			cancel()
		}
		d.state.mu.Lock()
		if cleanupFailed {
			d.state.owner = owner
			d.state.cleanupUncertain = true
		}
		if d.state.reason == "" {
			d.state.reason = "reader-start-failed"
			d.state.failurePhase = "reader-startup"
			d.state.firstFailure = err
			d.state.failureCode = xDailyClosedFailureCode(err)
		}
		d.state.mu.Unlock()
		return true, d.failure("reader startup failed")
	}
	return true, nil
}

func (d dailyController) revoke(ctx context.Context) error {
	e := d.engine
	if ctx == nil {
		return d.failure("revocation context required")
	}
	if !d.configured() {
		return nil
	}
	if e.store == nil {
		return d.failure("permission store unavailable")
	}
	d.state.permissionMu.Lock()
	d.state.mu.Lock()
	profileScope := d.state.profileScope
	d.state.permissionRevision++
	d.state.mu.Unlock()
	if !validXDailyProfileScope(profileScope) {
		stopErr := d.stop(ctx)
		d.state.permissionMu.Unlock()
		return errors.Join(d.failure("profile scope unavailable"), stopErr)
	}
	saveErr := e.store.SaveDailySourcePermission(ctx, d.source, profileScope, false)
	if saveErr == nil {
		d.state.mu.Lock()
		d.state.rememberedPermission = false
		d.state.mu.Unlock()
	}
	stopErr := d.stop(ctx)
	d.state.permissionMu.Unlock()
	if saveErr != nil || stopErr != nil {
		return errors.Join(saveErr, stopErr)
	}
	return nil
}

func (d dailyController) install(parent context.Context, owner sourceDailyOwner) error {
	e := d.engine
	if !d.configured() || !e.config.Dev || parent == nil || parent.Err() != nil || owner == nil {
		return d.failure("owner selection rejected")
	}
	if err := owner.CheckReader(parent); err != nil {
		return d.failure("reader unavailable")
	}
	d.state.mu.Lock()
	defer d.state.mu.Unlock()
	if !d.state.attached || d.state.closed || d.state.reason != "" || d.state.owner != nil {
		return d.failure("process authorization unavailable")
	}
	d.state.owner = owner
	d.state.parent = parent
	d.state.authorized = true
	return nil
}

func (d dailyController) ready() bool {
	d.state.mu.RLock()
	defer d.state.mu.RUnlock()
	return d.configured() && d.state.owner != nil && d.state.authorized && !d.state.closed && d.state.parent != nil && d.state.parent.Err() == nil
}

func (d dailyController) prepareReader(ctx context.Context, req readerbroker.Request, marker string) (readerbroker.NativePostPreparation, error) {
	d.state.mu.RLock()
	owner := d.state.owner
	parent := d.state.parent
	d.state.mu.RUnlock()
	if owner == nil || parent == nil || parent.Err() != nil {
		return readerbroker.NativePostPreparation{}, d.failure("reader unavailable")
	}
	if err := owner.CheckReader(ctx); err != nil {
		return readerbroker.NativePostPreparation{}, d.failure("reader ownership changed")
	}
	return owner.PrepareReader(ctx, req, marker)
}

func (d dailyController) queue(run domain.Run, payload map[string]any) error {
	if run.Source != d.source {
		return errors.New("daily capture source mismatch")
	}
	e := d.engine
	d.state.mu.Lock()
	if d.state.done != nil && d.state.runID == run.ID {
		d.state.mu.Unlock()
		return nil
	}
	if !d.state.authorized || d.state.closed || d.state.owner == nil || d.state.done != nil || d.state.generation == ^uint64(0) {
		d.state.mu.Unlock()
		return d.failure("batch unavailable")
	}
	d.state.generation++
	generation := d.state.generation
	owner := d.state.owner
	parent := d.state.parent
	work, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	d.state.done = done
	d.state.runID = run.ID
	if d.state.workers == 0 {
		d.state.drain = make(chan struct{})
	}
	d.state.workers++
	d.state.cancel = cancel
	d.state.mu.Unlock()
	e.mu.Lock()
	e.processWorkers++
	e.mu.Unlock()
	go func() {
		defer func() {
			e.mu.Lock()
			e.processWorkers--
			e.mu.Unlock()
			d.state.mu.Lock()
			d.state.workers--
			if d.state.workers == 0 {
				close(d.state.drain)
			}
			d.state.mu.Unlock()
		}()
		var batch xDailyNativeBatch
		var cause error
		phase := "reader-check"
		defer func() {
			// Freeze the operational cause before cancellation or cleanup can
			// produce a secondary failure. Diagnostics never grant authority.
			d.rememberFailure(cause, phase)
			cancel()
			cleanup, stop := context.WithTimeout(context.Background(), 30*time.Second)
			cleanupFailed := false
			if batch != nil {
				if err := batch.Close(cleanup); err != nil {
					d.rememberCleanup(err)
					cleanupFailed = true
					if cause == nil {
						cause = d.failure("collector cleanup uncertain")
						phase = "cleanup"
					}
				}
			}
			if cause != nil && !cleanupFailed {
				if err := e.store.ReconcileClosedDailySourceCapture(cleanup, d.source, run.ID); err != nil {
					d.rememberCleanup(xDailyCleanupFail(err, "store_reconcile", "unverified"))
					cleanupFailed = true
				}
			}
			stop()
			d.rememberFailure(cause, phase)
			d.state.mu.Lock()
			if cleanupFailed {
				d.state.cleanupUncertain = true
			}
			if cause != nil {
				d.state.authorized = false
				if d.state.reason == "" {
					d.state.reason = "batch-failed"
				}
			}
			// Generation and done identity prevent a late completion retiring a successor.
			if d.state.generation == generation && d.state.done == done {
				d.state.done = nil
				d.state.runID = ""
				d.state.cancel = nil
			}
			close(done)
			d.state.mu.Unlock()
			if cause != nil {
				e.logger.Printf(string(d.source)+"_daily_batch phase=terminal reason=batch-failed generation=%d", generation)
				if detail := d.status().StartupDiagnostic; detail != nil {
					e.logger.Printf(string(d.source)+"_daily_startup stage=%s condition=%s cause=%s broker_reason=%s %s", detail.Stage, detail.Condition, detail.Cause, detail.BrokerReason, detail.Origin)
				}
			}
			if detail := d.status().CleanupDiagnostic; detail != nil {
				e.logger.Printf(string(d.source)+"_daily_cleanup %s", detail.String())
			}
			if !cleanupFailed {
				_, _ = e.startNext(context.Background(), run.SessionID)
			}
		}()
		if err := owner.CheckReader(work); err != nil {
			cause = err
			if d.source == domain.SourceX {
				cause = xDailyStartupFail(err, "reader_check", "ownership")
			}
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
		// Each provider reports its original batch clock; queue admission cannot renew it.
		startup, stop := context.WithCancel(work)
		phase = "startup"
		batch, cause = owner.BeginBatch(startup, parent, generation, run)
		stop()
		if cause != nil || batch == nil {
			if cause == nil {
				cause = xDailyStartupFail(nil, "batch_begin_reply", "unavailable")
			}
			return
		}
		start, end := batch.StartedAt(), batch.Deadline()
		phase = "deadline"
		if start.IsZero() || start.After(e.store.Now()) || !end.After(e.store.Now()) || end.After(start.Add(60*time.Second)) {
			cause = d.failure("batch deadline rejected")
			return
		}
		capture, stop := context.WithDeadline(work, end)
		phase = "capture"
		_, cause = batch.Capture(capture, command.ID)
		stop()
	}()
	return nil
}

func (d dailyController) stop(ctx context.Context) error {
	d.state.mu.Lock()
	d.state.authorized = false
	if d.state.reason == "" {
		d.state.reason = "opted-out"
	}
	cancel, done, startCancel, startDone := d.state.cancel, d.state.done, d.state.cancelStart, d.state.startDone
	d.state.mu.Unlock()
	if startCancel != nil {
		startCancel()
	}
	if startDone != nil {
		select {
		case <-startDone:
		case <-ctx.Done():
			return d.failure("startup cleanup pending")
		}
	}
	if cancel != nil {
		cancel()
	}
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			return d.failure("batch cleanup pending")
		}
	}
	d.state.mu.RLock()
	uncertain := d.state.cleanupUncertain
	d.state.mu.RUnlock()
	if uncertain {
		return d.failure("batch cleanup uncertain")
	}
	return nil
}

func (d dailyController) close(ctx context.Context) error {
	stopErr := d.stop(ctx)
	d.state.mu.Lock()
	d.state.closed = true
	owner := d.state.owner
	d.state.mu.Unlock()
	if owner != nil {
		if err := owner.Close(ctx); err != nil {
			d.rememberCleanup(xDailyCleanupFail(err, "owner_close", "unverified"))
			return err
		}
		d.state.mu.RLock()
		done, start, drain := d.state.done, d.state.startDone, d.state.drain
		d.state.mu.RUnlock()
		for _, ended := range []<-chan struct{}{start, done, drain} {
			if ended != nil {
				select {
				case <-ended:
				case <-ctx.Done():
					return d.failure("process cleanup pending")
				}
			}
		}
		d.state.mu.Lock()
		d.state.cleanupUncertain = false
		d.state.mu.Unlock()
		return nil
	}
	if stopErr != nil {
		return stopErr
	}
	return nil
}

func (d dailyController) failure(reason string) error {
	label := "X"
	if d.source == domain.SourceInstagram {
		label = "Instagram"
	}
	return errors.New("daily " + label + " " + reason)
}
