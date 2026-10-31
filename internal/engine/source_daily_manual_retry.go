package engine

import (
	"context"
	"errors"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

// Returned only after an exact retry request was proven not to have reached
// the source owner. Timeouts and generic transport errors are never equivalent.
var errInstagramRetryNotDispatched = errors.New("instagram retry not dispatched")

// Manual recovery is a separate, source-owned capability. A status projection
// or diagnostic string cannot create it; the closed batch must establish it.
type dailyManualRetryOwner interface {
	manualRetry(context.Context, int) error
}

type dailyManualRetryBatch interface {
	manualRetryReady() bool
}

// The caller holds state.mu (at least for reading).
func (d dailyController) manualRetryAvailableLocked() bool {
	_, supported := d.state.owner.(dailyManualRetryOwner)
	return d.source == domain.SourceInstagram && d.configured() && d.engine.config.Dev &&
		d.engine.config.Capture.InstagramTrial && supported && d.state.manualRetryEligible &&
		!d.state.manualRetryInProgress && d.state.manualRetryCount < dailyRetryLimit &&
		!d.state.cleanupUncertain && !d.state.closed && !d.state.authorized &&
		d.state.reason == "batch-failed" && d.state.rememberedPermission &&
		d.state.done == nil && d.state.workers == 0 && d.state.parent != nil &&
		d.state.parent.Err() == nil && !d.maintenanceHeld() &&
		xDailyClosedFailureCode(d.state.firstFailure) == "native_source_evidence_document_changed"
}

// RetryInstagramTrial only re-arms the source; the next update must still
// create a new run, generation, collector and fresh account/session proofs.
func (e *Engine) RetryInstagramTrial(ctx context.Context, request XDailyStartRequest, acknowledgeUnqualified bool) error {
	d := e.dailyController(domain.SourceInstagram)
	if ctx == nil || ctx.Err() != nil || !request.AcknowledgeLimitations ||
		!request.AllowProcessSessionDerivation || !acknowledgeUnqualified || e.store == nil {
		return d.failure("retry consent required")
	}
	if !e.instagramReaderDiagnosticMu.TryRLock() {
		return d.failure("reader diagnostic in progress")
	}
	defer e.instagramReaderDiagnosticMu.RUnlock()
	d.state.permissionMu.Lock()
	d.state.mu.Lock()
	if !d.manualRetryAvailableLocked() {
		d.state.mu.Unlock()
		d.state.permissionMu.Unlock()
		return d.failure("manual retry unavailable")
	}
	sourceOwner := d.state.owner
	owner := sourceOwner.(dailyManualRetryOwner)
	parent, profile, revision := d.state.parent, d.state.profileScope, d.state.permissionRevision
	call, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	d.state.manualRetryEligible = false // A lost reply cannot authorize a replay.
	d.state.manualRetryInProgress = true
	d.state.manualRetryCanceled = false
	d.state.manualRetryCount++
	count := d.state.manualRetryCount
	d.state.startDone, d.state.cancelStart = done, cancel
	d.state.mu.Unlock()
	d.state.permissionMu.Unlock()
	stopParent := context.AfterFunc(parent, cancel)
	defer func() {
		stopParent()
		cancel()
		d.state.mu.Lock()
		if d.state.startDone == done {
			d.state.cancelStart = nil
		}
		d.state.manualRetryInProgress = false
		close(done)
		d.state.mu.Unlock()
	}()
	// The in-progress fence remains held while restoring a reservation. A stop,
	// revocation, changed owner or canceled request always wins over restoration.
	restoreUndispatched := func() {
		d.state.mu.Lock()
		defer d.state.mu.Unlock()
		if call.Err() != nil || parent.Err() != nil || d.state.closed || d.state.manualRetryCanceled ||
			d.maintenanceHeld() || d.state.owner != sourceOwner || d.state.permissionRevision != revision ||
			!d.state.rememberedPermission || d.state.cleanupUncertain || d.state.done != nil ||
			d.state.workers != 0 || d.state.reason != "batch-failed" || !d.state.manualRetryInProgress ||
			d.state.startDone != done || d.state.manualRetryEligible || d.state.manualRetryCount != count {
			return
		}
		d.state.manualRetryCount--
		d.state.manualRetryEligible = true
	}
	granted, err := e.store.DailySourcePermission(call, domain.SourceInstagram, profile)
	if err != nil || !granted || call.Err() != nil {
		if err != nil && call.Err() == nil {
			restoreUndispatched()
		}
		return d.failure("saved permission unavailable")
	}
	if err := owner.manualRetry(call, count); err != nil || call.Err() != nil {
		if errors.Is(err, errInstagramRetryNotDispatched) && call.Err() == nil {
			restoreUndispatched()
		}
		return d.failure("manual retry could not be verified")
	}
	d.state.mu.Lock()
	defer d.state.mu.Unlock()
	if call.Err() != nil || parent.Err() != nil || d.state.closed || d.state.manualRetryCanceled || d.maintenanceHeld() ||
		d.state.owner != sourceOwner || d.state.permissionRevision != revision || !d.state.rememberedPermission ||
		d.state.cleanupUncertain || d.state.done != nil || d.state.workers != 0 || d.state.reason != "batch-failed" {
		return d.failure("manual retry superseded")
	}
	d.state.reason = ""
	d.state.authorized = true
	d.state.firstFailure, d.state.failureCode, d.state.failurePhase = nil, "", ""
	return nil
}
