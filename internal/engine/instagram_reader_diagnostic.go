package engine

import (
	"context"
	"encoding/json"
	"runtime"
	"time"

	"github.com/abangkis/AkuSidecar/internal/appshell"
)

// InstagramReaderDiagnosticError contains only a closed admission category.
// Browser errors and response text must never become operator-facing errors.
type InstagramReaderDiagnosticError struct{ Reason string }

func (*InstagramReaderDiagnosticError) Error() string {
	return "Instagram reader diagnostic is unavailable"
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
	return e.runInstagramDiagnostic(ctx, action, appshell.ValidateInstagramReaderDiagnostic)
}

func (e *Engine) RunInstagramBootstrapDiagnostic(ctx context.Context, action func(context.Context) (json.RawMessage, error)) (json.RawMessage, error) {
	return e.runInstagramDiagnostic(ctx, action, appshell.ValidateInstagramBootstrapDiagnostic)
}

func (e *Engine) runInstagramDiagnostic(ctx context.Context, action func(context.Context) (json.RawMessage, error), validate func(json.RawMessage) (json.RawMessage, error)) (json.RawMessage, error) {
	if !e.InstagramReaderDiagnosticConfigured() || action == nil {
		return nil, instagramDiagnosticRejected("unavailable")
	}
	if ctx == nil || ctx.Err() != nil {
		return nil, instagramDiagnosticRejected("canceled")
	}
	if !e.operation.TryLock() {
		return nil, instagramDiagnosticRejected("busy")
	}
	defer e.operation.Unlock()
	// Daily source startup has its own permission mutex and does not take the
	// normal operation barrier. Exclude it before inspecting either hold.
	if !e.instagramReaderDiagnosticMu.TryLock() {
		return nil, instagramDiagnosticRejected("busy")
	}
	defer e.instagramReaderDiagnosticMu.Unlock()
	if e.instagramReaderDiagnosticUsed {
		return nil, instagramDiagnosticRejected("already_used")
	}
	if !e.nativeDerivationHold.Held("x") || !e.nativeDerivationHold.Held("instagram") {
		return nil, instagramDiagnosticRejected("maintenance_required")
	}
	ready, _, err := e.RuntimeUpdateReadiness(ctx)
	if err != nil || !ready {
		return nil, instagramDiagnosticRejected("busy")
	}
	e.mu.RLock()
	busy := e.shuttingDown || e.processWorkers != 0 || len(e.active) != 0
	for _, pending := range e.pending {
		busy = busy || pending
	}
	e.mu.RUnlock()
	if busy || e.xManualStartupPending() {
		return nil, instagramDiagnosticRejected("busy")
	}
	for _, source := range dailySources {
		d := e.dailyController(source)
		d.state.mu.RLock()
		busy = d.state.done != nil || d.state.startDone != nil || d.state.cleanupUncertain || d.state.owner != nil
		d.state.mu.RUnlock()
		if busy {
			return nil, instagramDiagnosticRejected("source_busy")
		}
	}
	if e.collectionRuntime == nil || e.collectionRuntime.Status().Pending {
		return nil, instagramDiagnosticRejected("owner_unavailable")
	}
	// After admission, a disconnected HTTP client cannot skip the bounded cleanup
	// wait. The sealed operation remains limited to thirty seconds overall.
	probeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	e.mu.Lock()
	if e.shuttingDown || e.processWorkers != 0 || len(e.active) != 0 {
		e.mu.Unlock()
		return nil, instagramDiagnosticRejected("busy")
	}
	done := make(chan struct{})
	e.instagramReaderDiagnosticDone = done
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
		return nil, instagramDiagnosticRejected("diagnostic_rejected")
	}
	raw, err = validate(raw)
	if err != nil {
		return nil, instagramDiagnosticRejected("projection_invalid")
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
