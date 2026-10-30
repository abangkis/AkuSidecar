package engine

import (
	"context"
	"encoding/json"
	"errors"
	"runtime"
	"time"

	"github.com/abangkis/AkuSidecar/internal/appshell"
	"github.com/abangkis/AkuSidecar/internal/igdiagnostic"
)

// InstagramReaderDiagnosticError contains only a closed admission category.
// Browser errors and response text must never become operator-facing errors.
type InstagramReaderDiagnosticError struct {
	Reason     string
	cause      error
	diagnostic json.RawMessage
}

func (*InstagramReaderDiagnosticError) Error() string {
	return "Instagram reader diagnostic is unavailable"
}
func (e *InstagramReaderDiagnosticError) String() string   { return e.Error() }
func (e *InstagramReaderDiagnosticError) GoString() string { return e.Error() }

func (e *InstagramReaderDiagnosticError) Unwrap() error { return e.cause }
func (e *InstagramReaderDiagnosticError) DiagnosticJSON() []byte {
	return append([]byte(nil), e.diagnostic...)
}

func instagramDiagnosticRejected(reason string) error {
	return &InstagramReaderDiagnosticError{Reason: reason}
}

func (e *Engine) InstagramReaderDiagnosticConfigured() bool {
	return runtime.GOOS == "windows" && xDailyBuildSupported && e.config.Dev &&
		e.config.Deployment.Mode == "development" && e.config.WindowsCaptureSplit &&
		e.config.Capture.XUsesMainProfile() && e.config.Capture.InstagramTrial
}

// RunInstagramReaderDiagnostic holds normal durable-work admission through one
// sealed diagnostic and cleanup. It never starts, stops or pauses a source.
func (e *Engine) RunInstagramReaderDiagnostic(ctx context.Context, action func(context.Context) (json.RawMessage, error)) (json.RawMessage, error) {
	return e.runInstagramDiagnostic(ctx, "reader", action, appshell.ValidateInstagramReaderDiagnostic)
}

func (e *Engine) RunInstagramBootstrapDiagnostic(ctx context.Context, action func(context.Context) (json.RawMessage, error)) (json.RawMessage, error) {
	return e.runInstagramDiagnostic(ctx, "bootstrap", action, appshell.ValidateInstagramBootstrapDiagnostic)
}

func (e *Engine) RunInstagramBootstrapCorroborationDiagnostic(ctx context.Context, action func(context.Context) (json.RawMessage, error)) (json.RawMessage, error) {
	return e.runInstagramDiagnostic(ctx, "corroboration", action, appshell.ValidateInstagramBootstrapCorroborationDiagnostic)
}

// InstagramDiagnosticSnapshot observes one retained, sanitized attempt. It never
// replays a browser operation, resets consent or declares cleanup complete.
func (e *Engine) InstagramDiagnosticSnapshot() json.RawMessage {
	e.mu.RLock()
	a := e.instagramDiagnosticAttempt
	e.mu.RUnlock()
	if a == nil {
		return nil
	}
	return a.SnapshotJSON()
}

func instagramFailureDiagnostic(err error) json.RawMessage {
	var carrier interface{ DiagnosticJSON() []byte }
	if !errors.As(err, &carrier) {
		return nil
	}
	if raw, ok := igdiagnostic.ValidateJSON(carrier.DiagnosticJSON()); ok {
		return raw
	}
	return nil
}

func (e *Engine) runInstagramDiagnostic(ctx context.Context, operation string, action func(context.Context) (json.RawMessage, error), validate func(json.RawMessage) (json.RawMessage, error)) (json.RawMessage, error) {
	nilContext := ctx == nil
	ctx, attempt := igdiagnostic.Ensure(ctx, operation)
	if attempt == nil {
		return nil, instagramDiagnosticRejected("unavailable")
	}
	igdiagnostic.Note(ctx, "admission")
	reject := func(reason, condition string, cause error) (json.RawMessage, error) {
		igdiagnostic.Fail(ctx, "admission", condition)
		return nil, &InstagramReaderDiagnosticError{Reason: reason, cause: cause, diagnostic: igdiagnostic.Projection(ctx)}
	}
	if !e.InstagramReaderDiagnosticConfigured() || action == nil {
		return reject("unavailable", "configuration", nil)
	}
	if nilContext || ctx.Err() != nil {
		return reject("canceled", "context_canceled", ctx.Err())
	}
	if !e.operation.TryLock() {
		return reject("busy", "operation_busy", nil)
	}
	defer e.operation.Unlock()
	// Daily source startup has its own permission mutex and does not take the
	// normal operation barrier. Exclude it before inspecting either hold.
	if !e.instagramReaderDiagnosticMu.TryLock() {
		return reject("busy", "permission_busy", nil)
	}
	defer e.instagramReaderDiagnosticMu.Unlock()
	if e.instagramReaderDiagnosticUsed {
		return reject("already_used", "already_used", nil)
	}
	if !e.nativeDerivationHold.Held("x") || !e.nativeDerivationHold.Held("instagram") {
		return reject("maintenance_required", "maintenance_required", nil)
	}
	ready, readiness, err := e.RuntimeUpdateReadiness(ctx)
	if err != nil {
		return reject("busy", "readiness_failed", err)
	}
	if !ready {
		condition := "readiness_busy"
		switch readiness {
		case "active_session", "active_calibration", "active_media_recapture", "state_unavailable":
			condition = readiness
		case "shutdown_in_progress":
			condition = "shutting_down"
		case "background_work":
			condition = "workers_active"
		}
		return reject("busy", condition, nil)
	}
	e.mu.RLock()
	condition := ""
	if e.shuttingDown {
		condition = "shutting_down"
	} else if e.processWorkers != 0 {
		condition = "workers_active"
	} else if len(e.active) != 0 {
		condition = "active_work"
	}
	for _, pending := range e.pending {
		if pending && condition == "" {
			condition = "pending_work"
		}
	}
	e.mu.RUnlock()
	if condition != "" {
		return reject("busy", condition, nil)
	}
	if e.xManualStartupPending() {
		return reject("busy", "startup_pending", nil)
	}
	for _, source := range dailySources {
		d := e.dailyController(source)
		d.state.mu.RLock()
		condition = ""
		if d.state.done != nil {
			condition = "source_running"
		} else if d.state.startDone != nil {
			condition = "source_starting"
		} else if d.state.cleanupUncertain {
			condition = "source_cleanup_uncertain"
		} else if d.state.owner != nil {
			condition = "source_owner_present"
		}
		d.state.mu.RUnlock()
		if condition != "" {
			return reject("source_busy", condition, nil)
		}
	}
	if e.collectionRuntime == nil {
		return reject("owner_unavailable", "runtime_missing", nil)
	}
	if e.collectionRuntime.Status().Pending {
		return reject("owner_unavailable", "runtime_pending", nil)
	}
	// After admission, a disconnected HTTP client cannot skip the bounded cleanup
	// wait. The sealed operation remains limited to thirty seconds overall.
	probeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	e.mu.Lock()
	if e.shuttingDown || e.processWorkers != 0 || len(e.active) != 0 {
		condition = "active_work"
		if e.shuttingDown {
			condition = "shutting_down"
		} else if e.processWorkers != 0 {
			condition = "workers_active"
		}
		e.mu.Unlock()
		return reject("busy", condition, nil)
	}
	done := make(chan struct{})
	e.instagramReaderDiagnosticDone = done
	e.instagramDiagnosticAttempt = attempt
	e.processWorkers++
	e.mu.Unlock()
	defer func() {
		e.mu.Lock()
		e.processWorkers--
		close(done)
		e.mu.Unlock()
	}()
	e.instagramReaderDiagnosticUsed = true
	raw, err := action(probeCtx)
	if err != nil {
		igdiagnostic.FailActive(probeCtx, "action_failed")
		return nil, &InstagramReaderDiagnosticError{Reason: "diagnostic_rejected", cause: err, diagnostic: igdiagnostic.Projection(probeCtx)}
	}
	igdiagnostic.Note(probeCtx, "projection")
	raw, err = validate(raw)
	if err != nil {
		igdiagnostic.Fail(probeCtx, "projection", "projection_invalid")
		return nil, &InstagramReaderDiagnosticError{Reason: "projection_invalid", cause: err, diagnostic: igdiagnostic.Projection(probeCtx)}
	}
	return raw, nil
}

// DrainInstagramReaderDiagnostic is called after Shutdown closes admission and
// before the main browser owner is terminated. It does not cancel the Go RPC,
// which would discard the reply while Node might still be retiring its target.
func (e *Engine) DrainInstagramReaderDiagnostic(ctx context.Context) error {
	e.mu.RLock()
	done := e.instagramReaderDiagnosticDone
	e.mu.RUnlock()
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return instagramDiagnosticRejected("cleanup_unsettled")
	}
}
