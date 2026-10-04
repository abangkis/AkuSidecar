package engine

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/collection"
	"github.com/abangkis/AkuSidecar/internal/collection/headless"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/store"
)

var errStaleCaptureRuntime = errors.New("capture command belongs to another runtime owner")

var ErrFacebookRecaptureUnavailable = errors.New("Facebook recapture is unavailable until Browser collection is ready")

func (e *Engine) ResetCaptureHeartbeat() {
	status := e.BridgeStatus()
	e.mu.Lock()
	defer e.mu.Unlock()
	if status.Actual != nil {
		e.headlessAccess = headlessGrantedSources(status)
	}
	e.heartbeat = nil
}

func headlessGrantedSources(status BridgeStatus) []domain.Source {
	if !status.Compatible || status.Actual == nil {
		return nil
	}
	var sources []domain.Source
	for _, source := range status.Actual.SourceAccess.Sources {
		if source.Ready && source.PermissionGranted && source.ScriptRegistered && collection.HeadlessSourceSupported(domain.Source(source.Source)) {
			sources = append(sources, domain.Source(source.Source))
		}
	}
	return sources
}

func (e *Engine) facebookSourceAuthorizedNow() bool {
	status := e.BridgeStatus()
	if !status.Compatible || status.Actual == nil {
		return false
	}
	now := time.Now()
	if !recentBridgeAccessObservation(status.Actual.ReceivedAt, now) || !recentBridgeAccessObservation(status.Actual.SourceAccess.ObservedAt, now) {
		return false
	}
	granted := false
	for _, source := range status.Actual.SourceAccess.GrantedSources {
		if source == string(domain.SourceFacebook) {
			granted = true
			break
		}
	}
	if !granted {
		return false
	}
	for _, source := range status.Actual.SourceAccess.Sources {
		if domain.Source(source.Source) == domain.SourceFacebook {
			return source.Ready && source.PermissionGranted && source.ScriptRegistered
		}
	}
	return false
}

func (e *Engine) facebookSourcePermissionDeniedNow() bool {
	status := e.BridgeStatus()
	if !status.Compatible || status.Actual == nil || !recentBridgeAccessObservation(status.Actual.ReceivedAt, time.Now()) ||
		!recentBridgeAccessObservation(status.Actual.SourceAccess.ObservedAt, time.Now()) {
		return false
	}
	granted := false
	for _, source := range status.Actual.SourceAccess.GrantedSources {
		if source == string(domain.SourceFacebook) {
			granted = true
			break
		}
	}
	for _, source := range status.Actual.SourceAccess.Sources {
		if domain.Source(source.Source) == domain.SourceFacebook {
			return !granted || !source.Ready || !source.PermissionGranted || !source.ScriptRegistered
		}
	}
	return true
}

func recentBridgeAccessObservation(value string, now time.Time) bool {
	observed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return false
	}
	age := now.Sub(observed)
	return age >= -5*time.Second && age <= 2*time.Minute
}

func (e *Engine) AttachCollectionCoordinator(runtime *collection.Coordinator) {
	e.collectionRuntime = runtime
	runtime.SetHeadlessReadiness(func() error {
		status := e.BridgeStatus()
		if status.Compatible && status.Actual != nil {
			for _, source := range status.Actual.SourceAccess.Sources {
				if source.Ready && source.PermissionGranted && source.ScriptRegistered && collection.HeadlessSourceSupported(domain.Source(source.Source)) {
					return nil
				}
			}
		}
		e.mu.RLock()
		retained := len(e.headlessAccess) > 0
		e.mu.RUnlock()
		if retained && e.captureOwner != nil && (e.captureOwner.Snapshot().Driver == "headless" || runtime.Status().NativeReaderOnly) {
			return nil
		}
		return errors.New("waiting for browser source-access confirmation before headless handoff")
	})
	if ids, err := e.store.BrowserAdmissionMediaRecaptureIDs(context.Background()); err == nil {
		for _, id := range ids {
			job, loadErr := e.store.MediaRecapture(context.Background(), id)
			if loadErr != nil || job.Source != domain.SourceFacebook {
				continue
			}
			admission, exists, markerErr := store.MediaRecaptureAdmission(job.Payload)
			needsHold := job.Status == "queued" || job.Status == "claimed"
			if job.Status == "completed" || job.Status == "failed" || job.Status == "cancelled" {
				needsHold = admission.Phase == "admitted" && job.Payload["captureCleanup"] != "released"
			}
			if markerErr == nil && exists && needsHold && admission.Policy == domain.MediaRecaptureAdmissionHybridHeadlessV1 {
				if err := e.beginBrowserCollectionHold(id, domain.SourceFacebook); err != nil {
					e.logger.Printf("restore Facebook recapture collection intent for job %s: %v", id, err)
				}
			}
		}
	}
	// Restore the temporary Browser hold for a persisted hybrid Facebook run
	// before the coordinator can reconcile back to the saved headless mode.
	if active, err := e.store.ActiveSession(context.Background()); err == nil && active != nil && hybridCollectionSession(*active) {
		restored := false
		for _, run := range active.Runs {
			switch run.Status {
			case "queued":
				driver, ok := hybridDriverForRun(*active, run.Source)
				if ok && run.Source == domain.SourceFacebook && driver == "browser" {
					if err := e.beginHybridBrowserCollection(active.ID, run.Source); err != nil {
						e.logger.Printf("restore hybrid Facebook collection intent for session %s: %v", active.ID, err)
					}
				}
				restored = true
				return
			case "waiting_for_bridge", "reasoning":
				if run.Source == domain.SourceFacebook {
					driver, planned := hybridDriverForRun(*active, run.Source)
					stampedDriver, stamped, driverErr := e.store.FirstCommandCaptureDriver(context.Background(), run.ID)
					collector, hasCollector, collectorErr := e.store.FirstCommandCollector(context.Background(), run.ID)
					if driverErr == nil && collectorErr == nil && planned && stamped && hasCollector &&
						driver == "browser" && stampedDriver == driver && collector == collection.BackendBridge {
						if err := e.beginHybridBrowserCollection(active.ID, run.Source); err != nil {
							e.logger.Printf("restore hybrid Facebook collection intent for session %s: %v", active.ID, err)
						}
					}
				}
				restored = true
				return
			case "completed", "failed", "cancelled":
				continue
			}
		}
		if !restored && hybridFacebookRunStarted(*active) {
			for _, run := range active.Runs {
				if run.Source != domain.SourceFacebook || run.StartedAt == nil {
					continue
				}
				driver, planned := hybridDriverForRun(*active, run.Source)
				stampedDriver, stamped, driverErr := e.store.FirstCommandCaptureDriver(context.Background(), run.ID)
				collector, hasCollector, collectorErr := e.store.FirstCommandCollector(context.Background(), run.ID)
				if driverErr == nil && collectorErr == nil && planned && stamped && hasCollector &&
					driver == "browser" && stampedDriver == driver && collector == collection.BackendBridge &&
					e.captureOwner != nil && e.captureOwner.Snapshot().State == captureruntime.Ready && e.captureOwner.Snapshot().Driver == "browser" {
					if err := e.beginHybridBrowserCollection(active.ID, run.Source); err != nil {
						e.logger.Printf("restore terminal hybrid Facebook collection intent for session %s: %v", active.ID, err)
					}
				}
				break
			}
		}
	}
}

// SetBrowserCollectionCleanup binds the lease-bound, full-surface Bridge
// release action used before a hybrid Facebook collection borrow is returned.
func (e *Engine) SetBrowserCollectionCleanup(cleanup func(context.Context, string, uint64) error) {
	e.captureMu.Lock()
	e.browserCollectionCleanup = cleanup
	e.captureMu.Unlock()
}

// SetBrowserMediaRecaptureDispatch binds the asynchronous Bridge consumer for
// admitted hybrid Facebook recaptures. The callback must deduplicate by job ID
// and generation and return only after the matching job reaches terminal state.
func (e *Engine) SetBrowserMediaRecaptureDispatch(dispatch func(context.Context, string, uint64) error) {
	e.captureMu.Lock()
	e.browserRecaptureDispatch = dispatch
	e.captureMu.Unlock()
}
func (e *Engine) BorrowInteractiveCapture(ctx context.Context) (*captureruntime.Lease, func(), error) {
	if e.collectionRuntime == nil {
		return nil, nil, nil
	}
	return e.collectionRuntime.BorrowBrowser(ctx)
}
func (e *Engine) BorrowNativeReader(ctx context.Context) (*captureruntime.Lease, func(), error) {
	if e.collectionRuntime == nil {
		return nil, nil, nil
	}
	return e.collectionRuntime.BorrowNativeReader(ctx)
}
func (e *Engine) PrepareInteractiveCapture(ctx context.Context) error {
	lease, release, err := e.BorrowInteractiveCapture(ctx)
	if err != nil {
		return err
	}
	if lease != nil {
		time.AfterFunc(30*time.Second, func() { lease.Release(); release() })
	}
	return nil
}
func (e *Engine) CollectionRuntime() collection.RuntimeStatus {
	if e.collectionRuntime == nil {
		return collection.RuntimeStatus{Requested: "browser", Effective: "browser"}
	}
	status := e.collectionRuntime.Status()
	if status.Effective == "headless" {
		e.mu.RLock()
		status.AuthorizedSources = append([]domain.Source(nil), e.headlessAccess...)
		e.mu.RUnlock()
	}
	return status
}
func (e *Engine) headlessEffective() bool {
	return e.collectionRuntime != nil && e.collectionRuntime.Status().Effective == "headless"
}
func (e *Engine) StartHeadlessCollection(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			if e.collectionRuntime == nil || e.captureOwner == nil {
				continue
			}
			e.releaseTerminalCaptureSessions(ctx)
			// A native reader owns the signed-in profile solely for user interaction.
			// It has neither a Quiet collector nor Bridge command consumers.
			if e.collectionRuntime.Status().NativeReaderOnly {
				continue
			}
			e.processHybridBrowserMediaRecaptures(ctx)
			if active, err := e.store.ActiveSession(ctx); err == nil && active != nil && hybridCollectionSession(*active) {
				if _, err := e.startNext(ctx, active.ID); err != nil {
					e.logger.Printf("retry hybrid collection admission for session %s: %v", active.ID, err)
				}
			}
			collector, driver, workerID := collection.BackendQuiet, "browser", "aku-browser-quiet-v1"
			if e.headlessEffective() {
				collector, driver, workerID = collection.BackendHeadless, "headless", "aku-headless-v1"
			}
			lease, err := e.acquireCaptureLease()
			if err != nil {
				continue
			}
			if lease == nil || lease.Driver() != driver {
				lease.Release()
				continue
			}
			func() {
				defer lease.Release()
				id, err := e.store.PendingRunIDForCollector(ctx, collector)
				if err != nil || id == "" {
					e.captureInternalMediaRecapture(ctx, driver, collector, workerID)
					return
				}
				command, err := e.claimCommandForCollector(ctx, id, workerID, driver, collector)
				if err != nil || command == nil {
					return
				}
				run, err := e.store.GetRun(ctx, id)
				if err != nil {
					return
				}
				observation, err := e.captureForCollector(ctx, run.Source, command.Payload, collector)
				if ctx.Err() != nil {
					return
				}
				if err == nil {
					_, err = e.AcceptObservation(ctx, command.ID, id, observation)
				}
				if err != nil {
					_, _ = e.FailCommand(ctx, command.ID, id, internalCollectorFailure(err, collector))
				}
			}()
		}
	}()
}

func (e *Engine) processHybridBrowserMediaRecaptures(ctx context.Context) {
	if e.collectionRuntime == nil {
		return
	}
	ids, err := e.store.BrowserAdmissionMediaRecaptureIDs(ctx)
	if err != nil {
		return
	}
	for _, id := range ids {
		job, err := e.store.MediaRecapture(ctx, id)
		if err != nil || job.Source != domain.SourceFacebook {
			continue
		}
		admission, marked, err := store.MediaRecaptureAdmission(job.Payload)
		if err != nil {
			e.logger.Printf("Facebook recapture %s has an invalid Browser admission marker: %v", id, err)
			continue
		}
		if !marked || admission.Policy != domain.MediaRecaptureAdmissionHybridHeadlessV1 {
			continue
		}
		active := job.Status == "queued" || job.Status == "claimed"
		terminal := job.Status == "completed" || job.Status == "failed" || job.Status == "cancelled"
		if !active && !terminal {
			continue
		}
		if active {
			if err := e.beginBrowserCollectionHold(id, job.Source); err != nil {
				continue
			}
			if admission.Phase == "waiting" {
				if !e.facebookSourceAuthorizedNow() {
					if e.facebookSourcePermissionDeniedNow() {
						_, _ = e.store.FailMediaRecapture(ctx, id, domain.Failure{Code: "source_access_revoked", Stage: "capture", Message: "Facebook Browser permission is not confirmed; request a new recapture after granting access.", Retryable: true})
						e.releaseTerminalCaptureSessions(ctx)
					}
					continue
				}
				e.captureMu.Lock()
				owner := e.captureOwner
				if owner == nil {
					e.captureMu.Unlock()
					continue
				}
				snapshot := owner.Snapshot()
				if snapshot.State != captureruntime.Ready || snapshot.Driver != "browser" {
					e.captureMu.Unlock()
					continue
				}
				lease, acquireErr := owner.Acquire()
				if acquireErr != nil {
					e.captureMu.Unlock()
					continue
				}
				runtime := map[string]any{"driver": "browser", "epoch": e.epoch, "generation": int(lease.Generation())}
				admitted, bindErr := e.store.BindMediaRecaptureBrowserAdmission(ctx, id, runtime)
				if bindErr != nil || !admitted {
					lease.Release()
					e.captureMu.Unlock()
					if bindErr != nil {
						e.logger.Printf("admit Facebook recapture %s to Browser: %v", id, bindErr)
					}
					continue
				}
				if e.captureRecaptures == nil {
					e.captureRecaptures = map[string]*captureruntime.Lease{}
				}
				if existing := e.captureRecaptures[id]; existing == nil {
					e.captureRecaptures[id] = lease
					lease = nil
				}
				if lease != nil {
					lease.Release()
				}
				e.captureMu.Unlock()
				job, err = e.store.MediaRecapture(ctx, id)
				if err != nil {
					continue
				}
				admission, _, err = store.MediaRecaptureAdmission(job.Payload)
				if err != nil {
					continue
				}
			}
		}
		if admission.Phase != "admitted" {
			continue
		}
		if job.Status == "queued" && !e.facebookSourceAuthorizedNow() {
			_, _ = e.store.FailMediaRecapture(ctx, id, domain.Failure{Code: "source_access_revoked", Stage: "capture", Message: "Facebook Browser permission is no longer confirmed.", Retryable: true})
			continue
		}
		e.captureMu.Lock()
		owner := e.captureOwner
		if owner == nil {
			e.captureMu.Unlock()
			continue
		}
		snapshot := owner.Snapshot()
		stamp, _ := job.Payload["captureRuntime"].(map[string]any)
		stampEpoch, _ := stamp["epoch"].(string)
		if stampEpoch != e.epoch {
			e.captureMu.Unlock()
			if active {
				_, _ = e.store.FailMediaRecapture(ctx, id, domain.Failure{Code: "capture_runtime_changed", Stage: "capture", Message: "Media recapture owner changed; stale Browser work was not dispatched.", Retryable: true})
			}
			continue
		}
		if snapshot.State != captureruntime.Ready || snapshot.Driver != "browser" {
			e.captureMu.Unlock()
			continue
		}
		if !mediaRecaptureStampMatches(job.Payload, snapshot, e.epoch) {
			e.captureMu.Unlock()
			if active {
				_, _ = e.store.FailMediaRecapture(ctx, id, domain.Failure{Code: "capture_runtime_changed", Stage: "capture", Message: "Media recapture owner changed; stale Browser work was not dispatched.", Retryable: true})
			}
			continue
		}
		lease := e.captureRecaptures[id]
		if lease == nil {
			lease, err = owner.Acquire()
			if err == nil {
				if e.captureRecaptures == nil {
					e.captureRecaptures = map[string]*captureruntime.Lease{}
				}
				e.captureRecaptures[id] = lease
			}
		}
		if err != nil || lease == nil || lease.Driver() != "browser" || lease.Generation() != snapshot.Generation {
			e.captureMu.Unlock()
			continue
		}
		dispatch := e.browserRecaptureDispatch
		shouldDispatch := dispatch != nil && !e.browserRecaptureDispatchRunning[id] && !e.browserRecaptureDispatchConfirmed[id] &&
			(active || e.browserRecaptureDispatchStarted[id])
		generation := lease.Generation()
		if shouldDispatch {
			if e.browserRecaptureDispatchRunning == nil {
				e.browserRecaptureDispatchRunning = map[string]bool{}
			}
			if e.browserRecaptureDispatchStarted == nil {
				e.browserRecaptureDispatchStarted = map[string]bool{}
			}
			e.browserRecaptureDispatchRunning[id] = true
			e.browserRecaptureDispatchStarted[id] = true
		}
		e.captureMu.Unlock()
		if shouldDispatch {
			dispatchID := id
			dispatchGeneration := generation
			dispatchCallback := dispatch
			go func() {
				dispatchCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
				defer cancel()
				dispatchErr := dispatchCallback(dispatchCtx, dispatchID, dispatchGeneration)
				terminal := dispatchErr == nil
				if dispatchErr != nil {
					latest, loadErr := e.store.MediaRecapture(context.Background(), dispatchID)
					terminal = loadErr == nil && isTerminalMediaRecapture(latest.Status)
					if loadErr == nil && !terminal && !errors.Is(dispatchErr, context.DeadlineExceeded) && !errors.Is(dispatchErr, context.Canceled) {
						_, failErr := e.store.FailMediaRecapture(context.Background(), dispatchID, domain.Failure{Code: "browser_dispatch_rejected", Stage: "capture", Message: "Browser Bridge did not accept the Facebook recapture dispatch.", Retryable: true})
						terminal = failErr == nil
					}
				}
				e.captureMu.Lock()
				delete(e.browserRecaptureDispatchRunning, dispatchID)
				stillOwned := e.collectionIntents[dispatchID] != nil || e.captureRecaptures[dispatchID] != nil
				if terminal && stillOwned {
					if e.browserRecaptureDispatchConfirmed == nil {
						e.browserRecaptureDispatchConfirmed = map[string]bool{}
					}
					e.browserRecaptureDispatchConfirmed[dispatchID] = true
				}
				e.captureMu.Unlock()
				if dispatchErr != nil {
					e.logger.Printf("dispatch Browser Facebook recapture %s: %v", dispatchID, dispatchErr)
				}
				e.releaseTerminalCaptureSessions(context.Background())
			}()
		}
	}
}

func (e *Engine) captureInternalMediaRecapture(ctx context.Context, driver, collector, workerID string) {
	ids, err := e.store.ActiveMediaRecaptureIDs(ctx)
	if err != nil {
		return
	}
	for _, id := range ids {
		job, err := e.store.MediaRecapture(ctx, id)
		if err != nil || job.Status != "queued" {
			continue
		}
		route, err := store.CaptureCollector(job.Payload)
		if err != nil || route != collector {
			continue
		}
		job, err = e.claimMediaRecaptureForCollector(ctx, id, workerID, driver, collector)
		if err != nil || job.ID == "" {
			continue
		}
		payload := map[string]any{}
		for key, value := range job.Payload {
			payload[key] = value
		}
		payload["pageUrl"] = job.TargetURL
		observation, err := e.captureForCollector(ctx, job.Source, payload, collector)
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			_, err = e.acceptMediaRecapture(ctx, id, observation, collector == "headless")
		}
		if err != nil {
			_, _ = e.FailMediaRecapture(ctx, id, internalCollectorFailure(err, collector))
		}
		return
	}
}

func headlessFailure(err error) domain.Failure {
	failure := domain.Failure{Code: "headless_capture_failed", Stage: "capture", Message: err.Error(), Retryable: true}
	var typed *headless.CaptureError
	if errors.As(err, &typed) {
		failure.Code = "headless_" + typed.Code
		if typed.Code == "login_required" || typed.Code == "challenge_required" || typed.Code == "challenge_detected" || typed.Code == "unsupported_source" || typed.Code == "unsupported_continuation" || typed.Code == "source_access_revoked" || typed.Code == "target_unavailable" {
			failure.Retryable = false
		}
	}
	return failure
}

func internalCollectorFailure(err error, collector string) domain.Failure {
	failure := headlessFailure(err)
	if collector == collection.BackendQuiet {
		failure.Code = "quiet_" + failure.Code[len("headless_"):]
	}
	return failure
}

// AttachCaptureRuntime adopts durable active work before a replacement could be
// considered. Ordinary combined UI/capture launches have no separate manager.
func (e *Engine) AttachCaptureRuntime(ctx context.Context, owner *captureruntime.Manager) error {
	if owner == nil {
		return captureruntime.ErrUnavailable
	}
	e.operation.Lock()
	defer e.operation.Unlock()
	e.captureMu.Lock()
	if e.captureOwner != nil {
		e.captureMu.Unlock()
		return errors.New("capture runtime is already attached")
	}
	e.captureMu.Unlock()
	// Acquire existing durable ownership before publishing the manager. A
	// concurrent observation must never see an attached owner with no session
	// lease solely because startup adoption is still in progress.
	sessions := map[string]*captureruntime.Lease{}
	recaptures := map[string]*captureruntime.Lease{}
	attached := false
	defer func() {
		if !attached {
			for _, lease := range sessions {
				lease.Release()
			}
			for _, lease := range recaptures {
				lease.Release()
			}
		}
	}()
	active, err := e.store.ActiveSession(ctx)
	if err != nil {
		return err
	}
	if active != nil {
		adopt := true
		if hybridCollectionSession(*active) {
			decided := false
			for _, run := range active.Runs {
				switch run.Status {
				case "queued":
					driver, ok := hybridDriverForRun(*active, run.Source)
					adopt = ok && owner.Snapshot().State == captureruntime.Ready && owner.Snapshot().Driver == driver
					decided = true
				case "waiting_for_bridge", "reasoning":
					// A command already stamped before restart remains authoritative.
					// A mismatched process is not adopted as a lease for that work.
					planned, plannedOK := hybridDriverForRun(*active, run.Source)
					stampedDriver, stamped, driverErr := e.store.FirstCommandCaptureDriver(ctx, run.ID)
					if driverErr != nil {
						return driverErr
					}
					collector, hasCollector, collectorErr := e.store.FirstCommandCollector(ctx, run.ID)
					if collectorErr != nil {
						return collectorErr
					}
					plannedCollector := collection.BackendBridge
					if planned == "headless" {
						plannedCollector = collection.BackendHeadless
					}
					snapshot := owner.Snapshot()
					adopt = plannedOK && stamped && hasCollector && stampedDriver == planned && collector == plannedCollector &&
						snapshot.State == captureruntime.Ready && snapshot.Driver == stampedDriver
					decided = true
				case "completed", "failed", "cancelled":
					continue
				}
				if decided {
					break
				}
			}
			if !decided {
				adopt = false
				if hybridFacebookRunStarted(*active) {
					for _, run := range active.Runs {
						if run.Source != domain.SourceFacebook || run.StartedAt == nil {
							continue
						}
						planned, plannedOK := hybridDriverForRun(*active, run.Source)
						stampedDriver, stamped, driverErr := e.store.FirstCommandCaptureDriver(ctx, run.ID)
						collector, hasCollector, collectorErr := e.store.FirstCommandCollector(ctx, run.ID)
						snapshot := owner.Snapshot()
						adopt = plannedOK && stamped && hasCollector && driverErr == nil && collectorErr == nil &&
							planned == "browser" && stampedDriver == planned && collector == collection.BackendBridge &&
							snapshot.State == captureruntime.Ready && snapshot.Driver == "browser"
						break
					}
				}
			}
		}
		if adopt {
			lease, err := owner.Acquire()
			if err != nil {
				return err
			}
			sessions[active.ID] = lease
		}
	}
	ids, err := e.store.ActiveMediaRecaptureIDs(ctx)
	if err != nil {
		return err
	}
	for _, id := range ids {
		job, err := e.store.MediaRecapture(ctx, id)
		if err != nil {
			return err
		}
		admission, hasAdmission, err := store.MediaRecaptureAdmission(job.Payload)
		if err != nil {
			return err
		}
		if hasAdmission {
			if admission.Phase == "waiting" || !mediaRecaptureStampMatches(job.Payload, owner.Snapshot(), e.epoch) {
				// Waiting jobs have no owner yet. Admitted jobs from an earlier
				// epoch/generation stay unadopted and are failed closed by the tick.
				continue
			}
		}
		lease, err := owner.Acquire()
		if err != nil {
			return err
		}
		recaptures[id] = lease
	}
	e.captureMu.Lock()
	e.captureOwner, e.captureSessions, e.captureRecaptures = owner, sessions, recaptures
	e.captureMu.Unlock()
	attached = true
	e.releaseTerminalCaptureSessions(ctx)
	return nil
}

func mediaRecaptureStampMatches(payload map[string]any, snapshot captureruntime.Snapshot, epoch string) bool {
	if snapshot.State != captureruntime.Ready {
		return false
	}
	stamp, ok := payload["captureRuntime"].(map[string]any)
	return ok && stamp["driver"] == snapshot.Driver && stamp["epoch"] == epoch && captureGenerationMatches(stamp["generation"], snapshot.Generation)
}

func (e *Engine) acquireCaptureLease() (*captureruntime.Lease, error) {
	e.captureMu.Lock()
	defer e.captureMu.Unlock()
	if e.captureOwner == nil {
		return nil, nil
	}
	return e.captureOwner.Acquire()
}

func (e *Engine) rememberCaptureSession(id string, lease *captureruntime.Lease) {
	if lease == nil {
		return
	}
	e.captureMu.Lock()
	defer e.captureMu.Unlock()
	if e.captureSessions == nil {
		e.captureSessions = map[string]*captureruntime.Lease{}
	}
	if existing := e.captureSessions[id]; existing != nil {
		lease.Release()
		return
	}
	e.captureSessions[id] = lease
}

func (e *Engine) ensureCaptureSession(id string) error {
	e.captureMu.Lock()
	defer e.captureMu.Unlock()
	if e.captureOwner == nil || e.captureSessions[id] != nil {
		return nil
	}
	lease, err := e.captureOwner.Acquire()
	if err != nil {
		return err
	}
	if e.captureSessions == nil {
		e.captureSessions = map[string]*captureruntime.Lease{}
	}
	e.captureSessions[id] = lease
	return nil
}

func (e *Engine) ensureHybridSessionLease(sessionID string, driver string) error {
	e.captureMu.Lock()
	defer e.captureMu.Unlock()
	lease := e.captureSessions[sessionID]
	if lease == nil || lease.Driver() != driver || e.captureOwner == nil || !e.captureOwner.Accepts(lease.Generation()) {
		return errStaleCaptureRuntime
	}
	return nil
}

func hybridCollectionSession(session domain.Session) bool {
	policy, _ := session.Coverage["collectionPolicy"].(string)
	return policy == domain.SessionCollectionPolicyHybridHeadlessV1
}

func hybridDriverForRun(session domain.Session, source domain.Source) (string, bool) {
	if !hybridCollectionSession(session) {
		return "", false
	}
	drivers, ok := session.Coverage["collectionSourceDrivers"].(map[string]any)
	if !ok {
		return "", false
	}
	driver, ok := drivers[string(source)].(string)
	if !ok || (driver != "headless" && driver != "browser") {
		return "", false
	}
	return driver, true
}

func (e *Engine) beginHybridBrowserCollection(sessionID string, source domain.Source) error {
	return e.beginBrowserCollectionHold(sessionID, source)
}

func (e *Engine) beginBrowserCollectionHold(leaseID string, source domain.Source) error {
	if source != domain.SourceFacebook {
		return errors.New("only Facebook uses the hybrid Browser collection exception")
	}
	if e.collectionRuntime == nil {
		return errors.New("hybrid collection runtime is unavailable")
	}
	e.captureMu.Lock()
	if e.collectionIntents[leaseID] != nil {
		e.captureMu.Unlock()
		return nil
	}
	e.captureMu.Unlock()
	release, err := e.collectionRuntime.BeginBrowserCollection(source)
	if err != nil {
		return err
	}
	e.captureMu.Lock()
	if e.collectionIntents == nil {
		e.collectionIntents = map[string]func(){}
	}
	if e.collectionIntents[leaseID] == nil {
		e.collectionIntents[leaseID] = release
		release = nil
	}
	e.captureMu.Unlock()
	if release != nil {
		release()
	}
	return nil
}

func (e *Engine) releaseSessionCaptureLease(sessionID string) {
	e.captureMu.Lock()
	lease := e.captureSessions[sessionID]
	delete(e.captureSessions, sessionID)
	e.captureMu.Unlock()
	if lease != nil {
		lease.Release()
	}
}

func (e *Engine) admitHybridRun(ctx context.Context, session domain.Session, run domain.Run) (bool, error) {
	desired, ok := hybridDriverForRun(session, run.Source)
	if !ok {
		return false, errors.New("hybrid session source plan is missing or invalid")
	}
	if desired == "browser" {
		if err := e.beginHybridBrowserCollection(session.ID, run.Source); err != nil {
			return false, err
		}
	}

	e.captureMu.Lock()
	lease := e.captureSessions[session.ID]
	owner := e.captureOwner
	e.captureMu.Unlock()
	predecessorsDrained := false
	if desired == "browser" || (lease != nil && lease.Driver() != desired) {
		var err error
		predecessorsDrained, err = e.hybridPredecessorsDrained(ctx, session, run)
		if err != nil || !predecessorsDrained {
			return false, err
		}
	}
	if lease != nil && lease.Driver() != desired {
		e.releaseSessionCaptureLease(session.ID)
		lease = nil
	}
	if lease == nil {
		if owner == nil {
			return false, errors.New("hybrid capture runtime is unavailable")
		}
		snapshot := owner.Snapshot()
		if snapshot.State != captureruntime.Ready || snapshot.Driver != desired {
			return false, nil
		}
		var acquired *captureruntime.Lease
		e.captureMu.Lock()
		if existing := e.captureSessions[session.ID]; existing != nil {
			acquired = existing
		} else {
			var err error
			acquired, err = owner.Acquire()
			if err != nil {
				e.captureMu.Unlock()
				return false, err
			}
			if acquired.Driver() != desired {
				acquired.Release()
				e.captureMu.Unlock()
				return false, nil
			}
			if e.captureSessions == nil {
				e.captureSessions = map[string]*captureruntime.Lease{}
			}
			e.captureSessions[session.ID] = acquired
		}
		e.captureMu.Unlock()
		lease = acquired
	}
	if lease == nil || lease.Driver() != desired || owner == nil || !owner.Accepts(lease.Generation()) {
		return false, nil
	}
	if run.Source == domain.SourceFacebook && desired == "browser" && !e.facebookBrowserSourceReady() {
		return false, nil
	}
	if desired == "headless" && !e.headlessSourceAuthorized(run.Source) {
		return false, nil
	}
	return true, nil
}

func (e *Engine) hybridPredecessorsDrained(ctx context.Context, session domain.Session, run domain.Run) (bool, error) {
	for _, previous := range session.Runs {
		if previous.Ordinal >= run.Ordinal {
			break
		}
		switch previous.Status {
		case "completed", "failed", "cancelled":
		default:
			return false, nil
		}
		e.mu.RLock()
		_, active := e.active[previous.ID]
		_, pending := e.pending[previous.ID]
		e.mu.RUnlock()
		if active || pending {
			return false, nil
		}
	}
	drained, err := e.store.SessionCaptureCommandsDrained(ctx, session.ID, &run.Ordinal)
	return drained, err
}

func (e *Engine) facebookBrowserSourceReady() bool {
	status := e.BridgeStatus()
	if !status.Compatible || status.Actual == nil {
		return false
	}
	receivedAt, err := time.Parse(time.RFC3339Nano, status.Actual.ReceivedAt)
	if err != nil || time.Since(receivedAt) > 2*time.Minute || receivedAt.After(time.Now().Add(5*time.Second)) {
		return false
	}
	for _, source := range status.Actual.SourceAccess.Sources {
		if source.Source == string(domain.SourceFacebook) {
			return source.Ready && source.PermissionGranted && source.ScriptRegistered
		}
	}
	return false
}

func (e *Engine) headlessSourceAuthorized(source domain.Source) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, allowed := range e.headlessAccess {
		if allowed == source {
			return true
		}
	}
	return false
}

// Release only after durable terminal state AND worker drain. In particular,
// accepting the first observation or requesting cancellation is not release.
func (e *Engine) releaseTerminalCaptureSessions(ctx context.Context) {
	e.captureMu.Lock()
	ids := make(map[string]struct{}, len(e.captureSessions)+len(e.captureRecaptures)+len(e.collectionIntents))
	for id := range e.captureSessions {
		ids[id] = struct{}{}
	}
	for id := range e.captureRecaptures {
		ids[id] = struct{}{}
	}
	for id := range e.collectionIntents {
		ids[id] = struct{}{}
	}
	type cleanupRequest struct {
		leaseID    string
		generation uint64
		callback   func(context.Context, string, uint64) error
		recapture  bool
	}
	var cleanups []cleanupRequest
	for id := range ids {
		session, err := e.store.GetSession(ctx, id)
		if err != nil {
			continue
		} // Unknown state keeps ownership pinned.
		switch session.Status {
		case "completed", "partial", "failed", "cancelled":
		default:
			continue
		}
		e.mu.RLock()
		busy := false
		for _, run := range session.Runs {
			if _, active := e.active[run.ID]; active {
				busy = true
			}
			if _, pending := e.pending[run.ID]; pending {
				busy = true
			}
		}
		e.mu.RUnlock()
		if busy {
			continue
		}
		drained, err := e.store.SessionCaptureCommandsDrained(ctx, id, nil)
		if err != nil || !drained {
			continue
		}
		needsBrowserCleanup := hybridFacebookRunStarted(session) && e.collectionIntents[id] != nil
		if needsBrowserCleanup && !e.collectionCleanupDone[id] {
			lease := e.captureSessions[id]
			callback := e.browserCollectionCleanup
			if lease == nil || lease.Driver() != "browser" || callback == nil || e.collectionCleanupPending[id] {
				continue
			}
			if e.collectionCleanupPending == nil {
				e.collectionCleanupPending = map[string]bool{}
			}
			e.collectionCleanupPending[id] = true
			cleanups = append(cleanups, cleanupRequest{leaseID: id, generation: lease.Generation(), callback: callback})
			continue
		}
		if lease := e.captureSessions[id]; lease != nil {
			lease.Release()
			delete(e.captureSessions, id)
		}
		if release := e.collectionIntents[id]; release != nil {
			release()
			delete(e.collectionIntents, id)
		}
		delete(e.collectionCleanupPending, id)
		delete(e.collectionCleanupDone, id)
	}
	for id := range ids {
		job, err := e.store.MediaRecapture(ctx, id)
		if err != nil {
			continue
		}
		if job.Status == "queued" || job.Status == "claimed" {
			continue
		}
		admission, hybrid, markerErr := store.MediaRecaptureAdmission(job.Payload)
		if markerErr != nil {
			continue
		}
		if !hybrid {
			if lease := e.captureRecaptures[id]; lease != nil {
				lease.Release()
				delete(e.captureRecaptures, id)
			}
			if release := e.collectionIntents[id]; release != nil {
				release()
				delete(e.collectionIntents, id)
			}
			continue
		}
		if admission.Phase == "waiting" {
			if lease := e.captureRecaptures[id]; lease != nil {
				lease.Release()
				delete(e.captureRecaptures, id)
			}
			if release := e.collectionIntents[id]; release != nil {
				release()
				delete(e.collectionIntents, id)
			}
			delete(e.collectionCleanupPending, id)
			delete(e.collectionCleanupDone, id)
			delete(e.browserRecaptureDispatchRunning, id)
			delete(e.browserRecaptureDispatchStarted, id)
			delete(e.browserRecaptureDispatchConfirmed, id)
			continue
		}
		if job.Payload["captureCleanup"] == "released" {
			if lease := e.captureRecaptures[id]; lease != nil {
				lease.Release()
				delete(e.captureRecaptures, id)
			}
			if release := e.collectionIntents[id]; release != nil {
				release()
				delete(e.collectionIntents, id)
			}
			delete(e.collectionCleanupPending, id)
			delete(e.collectionCleanupDone, id)
			delete(e.browserRecaptureDispatchRunning, id)
			delete(e.browserRecaptureDispatchStarted, id)
			delete(e.browserRecaptureDispatchConfirmed, id)
			continue
		}
		if e.browserRecaptureDispatchRunning[id] || (e.browserRecaptureDispatchStarted[id] && !e.browserRecaptureDispatchConfirmed[id]) {
			continue
		}
		if job.Payload["captureCleanup"] != "pending" {
			if err := e.store.SetMediaRecaptureCaptureCleanupState(ctx, id, "pending"); err != nil {
				continue
			}
		}
		lease := e.captureRecaptures[id]
		if lease == nil && e.captureOwner != nil {
			snapshot := e.captureOwner.Snapshot()
			if snapshot.State == captureruntime.Ready && snapshot.Driver == "browser" {
				lease, err = e.captureOwner.Acquire()
				if err == nil {
					if e.captureRecaptures == nil {
						e.captureRecaptures = map[string]*captureruntime.Lease{}
					}
					e.captureRecaptures[id] = lease
				}
			}
		}
		callback := e.browserCollectionCleanup
		if lease == nil || lease.Driver() != "browser" || callback == nil || e.collectionCleanupPending[id] {
			continue
		}
		if e.collectionCleanupPending == nil {
			e.collectionCleanupPending = map[string]bool{}
		}
		e.collectionCleanupPending[id] = true
		cleanups = append(cleanups, cleanupRequest{leaseID: id, generation: lease.Generation(), callback: callback, recapture: true})
	}
	e.captureMu.Unlock()
	for _, cleanup := range cleanups {
		cleanup := cleanup
		go func() {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			err := cleanup.callback(cleanupCtx, cleanup.leaseID, cleanup.generation)
			if err == nil && cleanup.recapture {
				persistCtx, persistCancel := context.WithTimeout(context.Background(), 5*time.Second)
				persistErr := e.store.SetMediaRecaptureCaptureCleanupState(persistCtx, cleanup.leaseID, "released")
				persistCancel()
				if persistErr != nil {
					e.logger.Printf("persist Browser recapture cleanup acknowledgement for lease %s: %v", cleanup.leaseID, persistErr)
					err = errors.New("Browser cleanup acknowledgement could not be saved.")
				}
			}
			e.captureMu.Lock()
			delete(e.collectionCleanupPending, cleanup.leaseID)
			var releaseIntent func()
			var releaseLease *captureruntime.Lease
			if err == nil {
				if cleanup.recapture {
					// Late callback completion may clear existing ownership only. It
					// never recreates per-job state after another path has released it.
					if e.collectionIntents[cleanup.leaseID] != nil || e.captureRecaptures[cleanup.leaseID] != nil {
						releaseIntent = e.collectionIntents[cleanup.leaseID]
						delete(e.collectionIntents, cleanup.leaseID)
						releaseLease = e.captureRecaptures[cleanup.leaseID]
						delete(e.captureRecaptures, cleanup.leaseID)
						delete(e.browserRecaptureDispatchRunning, cleanup.leaseID)
						delete(e.browserRecaptureDispatchStarted, cleanup.leaseID)
						delete(e.browserRecaptureDispatchConfirmed, cleanup.leaseID)
					}
				} else {
					if e.collectionCleanupDone == nil {
						e.collectionCleanupDone = map[string]bool{}
					}
					e.collectionCleanupDone[cleanup.leaseID] = true
				}
			}
			e.captureMu.Unlock()
			if err != nil {
				if e.collectionRuntime != nil {
					e.collectionRuntime.SetBrowserCollectionFailure(cleanup.leaseID, err.Error())
				}
				e.logger.Printf("release hybrid Facebook capture surface for lease %s: %v", cleanup.leaseID, err)
			}
			if err == nil {
				if e.collectionRuntime != nil {
					e.collectionRuntime.SetBrowserCollectionFailure(cleanup.leaseID, "")
				}
				if releaseLease != nil {
					releaseLease.Release()
				}
				if releaseIntent != nil {
					releaseIntent()
				}
				e.releaseTerminalCaptureSessions(context.Background())
			}
		}()
	}
}

func hybridFacebookRunStarted(session domain.Session) bool {
	for _, run := range session.Runs {
		if run.Source == domain.SourceFacebook && run.StartedAt != nil {
			return true
		}
	}
	return false
}

func isTerminalMediaRecapture(status string) bool {
	return status == "completed" || status == "failed" || status == "cancelled"
}

func (e *Engine) ownedCapturePayload(run domain.Run, leaseID string, settings domain.Settings, round int, continuation map[string]any, reason string) (map[string]any, error) {
	session, err := e.store.GetSession(context.Background(), run.SessionID)
	if err != nil {
		return nil, err
	}
	hybrid := hybridCollectionSession(session)
	plannedDriver := ""
	if hybrid {
		var ok bool
		plannedDriver, ok = hybridDriverForRun(session, run.Source)
		if !ok {
			return nil, errors.New("hybrid session source plan is missing or invalid")
		}
		if err := e.ensureHybridSessionLease(run.SessionID, plannedDriver); err != nil {
			return nil, err
		}
	} else if err := e.ensureCaptureSession(run.SessionID); err != nil {
		return nil, err
	}
	payload := capturePayload(run, leaseID, settings, round, continuation, reason)
	e.captureMu.Lock()
	driver := "browser"
	if lease := e.captureSessions[run.SessionID]; lease != nil {
		driver = lease.Driver()
		payload["captureRuntime"] = map[string]any{"driver": lease.Driver(), "epoch": e.epoch, "generation": int(lease.Generation())}
	}
	e.captureMu.Unlock()
	collector, exists, err := e.store.FirstCommandCollector(context.Background(), run.ID)
	if err != nil {
		return nil, err
	}
	if !exists {
		if hybrid && plannedDriver == "headless" {
			collector = collection.BackendHeadless
		} else if hybrid && run.Source == domain.SourceFacebook {
			collector = collection.BackendBridge
		} else {
			collector = e.selectCaptureCollector(run.Source, settings, driver)
		}
	} else if hybrid {
		plannedCollector := collection.BackendBridge
		if plannedDriver == "headless" {
			plannedCollector = collection.BackendHeadless
		}
		if collector != plannedCollector {
			return nil, fmt.Errorf("%w: durable collector conflicts with the frozen hybrid source plan", errStaleCaptureRuntime)
		}
	}
	payload["captureCollector"] = collectorStamp(collector)
	return payload, nil
}

func collectorStamp(backend string) map[string]any {
	return map[string]any{"backend": backend, "version": 1}
}

func (e *Engine) selectCaptureCollector(source domain.Source, settings domain.Settings, driver string) string {
	if driver == "headless" {
		return collection.BackendHeadless
	}
	if (settings.CaptureVisibility != "quiet" && settings.CaptureVisibility != "quiet_multi_window") || e.collectionRuntime == nil || !e.collectionRuntime.BrowserCollectorAvailable(source) {
		return collection.BackendBridge
	}
	if e.quietSourceAuthorized(source) {
		return collection.BackendQuiet
	}
	return collection.BackendBridge
}

func (e *Engine) quietSourceAuthorized(source domain.Source) bool {
	status := e.BridgeStatus()
	if status.Compatible && status.Actual != nil {
		for _, access := range status.Actual.SourceAccess.Sources {
			if domain.Source(access.Source) == source && access.Ready && access.PermissionGranted && access.ScriptRegistered {
				return true
			}
		}
	}
	return false
}

func (e *Engine) captureForCollector(ctx context.Context, source domain.Source, payload map[string]any, collector string) (domain.Observation, error) {
	if collector == collection.BackendQuiet && !e.quietSourceAuthorized(source) {
		return domain.Observation{}, &headless.CaptureError{Code: "source_access_revoked", Message: "Browser source access is no longer confirmed; grant source access before retrying Quiet capture."}
	}
	return e.collectionRuntime.Capture(ctx, source, payload)
}

// RunCaptureCollector is read-only and uses the persisted command route rather
// than current settings, including for UI dispatch acknowledgements.
func (e *Engine) RunCaptureCollector(ctx context.Context, runID string) (string, error) {
	collector, exists, err := e.store.FirstCommandCollector(ctx, runID)
	if err != nil {
		return "", err
	}
	if !exists {
		// Preserve the existing Bridge action path when there is no durable
		// command yet; it still cannot claim or execute nonexistent work.
		return collection.BackendBridge, nil
	}
	return collector, nil
}

func (e *Engine) validateCaptureOwner(sessionID string, payload map[string]any) error {
	e.captureMu.Lock()
	defer e.captureMu.Unlock()
	if e.captureOwner == nil {
		if _, stamped := payload["captureRuntime"]; stamped {
			return errStaleCaptureRuntime
		}
		return nil
	}
	lease := e.captureSessions[sessionID]
	return e.validateCaptureStamp(lease, payload)
}

// Caller holds captureMu; durable command state is checked by the store.
func (e *Engine) validateCaptureStamp(lease *captureruntime.Lease, payload map[string]any) error {
	if lease == nil || !e.captureOwner.Accepts(lease.Generation()) {
		return errStaleCaptureRuntime
	}
	stamp, exists := payload["captureRuntime"]
	if !exists {
		// Legacy queued commands predating attachment are admitted only in the
		// first managed generation. They can never cross a process replacement.
		if lease.Generation() == 1 {
			return nil
		}
		return errStaleCaptureRuntime
	}
	identity, ok := stamp.(map[string]any)
	if !ok || identity["driver"] != lease.Driver() || identity["epoch"] != e.epoch || !captureGenerationMatches(identity["generation"], lease.Generation()) {
		return fmt.Errorf("%w: driver, epoch or generation mismatch", errStaleCaptureRuntime)
	}
	return nil
}

func captureGenerationMatches(value any, expected uint64) bool {
	switch generation := value.(type) {
	case int:
		return generation > 0 && uint64(generation) == expected
	case float64: // JSON decoding must not truncate fractional/unsafe integers.
		return expected > 0 && expected <= 1<<53 && generation == float64(expected)
	default:
		return false
	}
}

func (e *Engine) ensureCaptureRecapture(id string) error {
	e.captureMu.Lock()
	defer e.captureMu.Unlock()
	if e.captureOwner == nil || e.captureRecaptures[id] != nil {
		return nil
	}
	lease, err := e.captureOwner.Acquire()
	if err != nil {
		return err
	}
	if e.captureRecaptures == nil {
		e.captureRecaptures = map[string]*captureruntime.Lease{}
	}
	e.captureRecaptures[id] = lease
	return nil
}

func (e *Engine) validateRecaptureOwner(ctx context.Context, id string) error {
	job, err := e.store.MediaRecapture(ctx, id)
	if err != nil {
		return err
	}
	e.captureMu.Lock()
	defer e.captureMu.Unlock()
	if e.captureOwner == nil {
		if _, stamped := job.Payload["captureRuntime"]; stamped {
			return errStaleCaptureRuntime
		}
		return nil
	}
	return e.validateCaptureStamp(e.captureRecaptures[id], job.Payload)
}
