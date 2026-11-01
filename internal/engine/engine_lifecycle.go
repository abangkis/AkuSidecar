package engine

import (
	"context"
	"errors"
	"time"
)

func (e *Engine) launchProcess(runID string, allowPlanning bool) {
	e.launchProcessWithPolicy(runID, allowPlanning, false)
}

func (e *Engine) launchProcessAfterObservation(runID string, allowPlanning bool) {
	e.launchProcessWithPolicy(runID, allowPlanning, true)
}

func (e *Engine) launchProcessWithPolicy(runID string, allowPlanning, queueIfActive bool) {
	e.mu.Lock()
	if !e.work.admitRun(runID, allowPlanning, queueIfActive) {
		e.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	e.work.registerRun(runID, cancel)
	e.mu.Unlock()
	go func() {
		succeeded := false
		defer func() {
			// Retiring active admission is not the end of the worker: deferred
			// session/capture cleanup below still uses SQLite. Keep shutdown's
			// drain count until those operations have actually returned.
			defer func() { e.mu.Lock(); e.work.workerFinished(); e.mu.Unlock() }()
			e.mu.Lock()
			pendingAllowPlanning, relaunch := e.work.retireRun(runID, succeeded)
			e.mu.Unlock()
			if relaunch {
				e.launchProcess(runID, pendingAllowPlanning)
			} else if run, runErr := e.store.GetRun(context.Background(), runID); runErr == nil {
				if session, sessionErr := e.store.GetSession(context.Background(), run.SessionID); sessionErr == nil && hybridCollectionSession(session) {
					if _, advanceErr := e.startNext(context.Background(), session.ID); advanceErr != nil {
						e.logger.Printf("retry hybrid collection admission for session %s after worker drain: %v", session.ID, advanceErr)
					}
				}
			}
			e.releaseTerminalCaptureSessions(context.Background())
		}()
		if err := e.process(ctx, runID, allowPlanning); err != nil {
			if e.shouldPauseForShutdown(ctx, err) {
				e.logger.Printf("run %s paused during shutdown and will resume from accepted capture", runID)
				return
			}
			e.logger.Printf("run %s failed: %v", runID, err)
			if failErr := e.failRunAfterProcessError(context.Background(), runID, err); failErr != nil {
				e.logger.Printf("record failed run %s: %v", runID, failErr)
			}
			return
		}
		succeeded = true
	}()
}

func (e *Engine) shouldPauseForShutdown(ctx context.Context, err error) bool {
	e.mu.RLock()
	shuttingDown := e.work.shuttingDown
	e.mu.RUnlock()
	return shuttingDown && (errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled))
}

func (e *Engine) CancelSession(ctx context.Context, id string) error {
	session, err := e.store.GetSession(ctx, id)
	if err != nil {
		return err
	}
	e.mu.Lock()
	e.work.prepareCancellation()
	for _, run := range session.Runs {
		e.work.cancelRun(run.ID)
	}
	e.mu.Unlock()
	if err := e.store.CancelSession(ctx, id); err != nil {
		return err
	}
	e.releaseTerminalCaptureSessions(ctx)
	return nil
}

func (e *Engine) Shutdown() {
	e.mu.Lock()
	e.work.beginShutdown()
	if e.autoCancel != nil {
		e.autoCancel()
	}
	e.work.cancelActive()
	e.mu.Unlock()
	e.shutdownXManual()
}

func (e *Engine) CloseProvider() error {
	if closer, ok := e.provider.(interface{ Close() error }); ok {
		return closer.Close()
	}
	return nil
}

func (e *Engine) WaitForIdle(timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		e.mu.RLock()
		work := e.work.snapshot()
		e.mu.RUnlock()
		if work.active+work.workers == 0 && !e.xManualStartupPending() {
			return true
		}
		time.Sleep(25 * time.Millisecond)
	}
	return false
}

func (e *Engine) RuntimeUpdateReadiness(ctx context.Context) (bool, string, error) {
	active, err := e.store.ActiveSession(ctx)
	if err != nil {
		return false, "state_unavailable", err
	}
	if active != nil {
		return false, "active_session", nil
	}
	calibration, err := e.store.ActiveCalibration(ctx)
	if err != nil {
		return false, "state_unavailable", err
	}
	if calibration != nil {
		return false, "active_calibration", nil
	}
	recapturing, err := e.store.ActiveMediaRecapture(ctx)
	if err != nil {
		return false, "state_unavailable", err
	}
	if recapturing {
		return false, "active_media_recapture", nil
	}
	e.mu.RLock()
	work := e.work.snapshot()
	e.mu.RUnlock()
	if work.shuttingDown {
		return false, "shutdown_in_progress", nil
	}
	if work.active+work.workers > 0 {
		return false, "background_work", nil
	}
	return true, "idle", nil
}
