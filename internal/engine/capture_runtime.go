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
		if source.Ready && source.PermissionGranted && source.ScriptRegistered && (source.Source == "x" || source.Source == "facebook") {
			sources = append(sources, domain.Source(source.Source))
		}
	}
	return sources
}

func (e *Engine) AttachCollectionCoordinator(runtime *collection.Coordinator) {
	e.collectionRuntime = runtime
	runtime.SetHeadlessReadiness(func() error {
		status := e.BridgeStatus()
		if status.Compatible && status.Actual != nil {
			for _, source := range status.Actual.SourceAccess.Sources {
				if source.Ready && (source.Source == "x" || source.Source == "facebook") {
					return nil
				}
			}
		}
		e.mu.RLock()
		retained := len(e.headlessAccess) > 0
		e.mu.RUnlock()
		if retained && e.captureOwner != nil && e.captureOwner.Snapshot().Driver == "headless" {
			return nil
		}
		return errors.New("waiting for browser source-access confirmation before headless handoff")
	})
}
func (e *Engine) BorrowInteractiveCapture(ctx context.Context) (*captureruntime.Lease, func(), error) {
	if e.collectionRuntime == nil {
		return nil, nil, nil
	}
	return e.collectionRuntime.BorrowBrowser(ctx)
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
		lease, err := owner.Acquire()
		if err != nil {
			return err
		}
		sessions[active.ID] = lease
	}
	ids, err := e.store.ActiveMediaRecaptureIDs(ctx)
	if err != nil {
		return err
	}
	for _, id := range ids {
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

// Release only after durable terminal state AND worker drain. In particular,
// accepting the first observation or requesting cancellation is not release.
func (e *Engine) releaseTerminalCaptureSessions(ctx context.Context) {
	e.captureMu.Lock()
	defer e.captureMu.Unlock()
	for id, lease := range e.captureSessions {
		session, err := e.store.GetSession(ctx, id)
		if err != nil {
			continue
		} // Unknown state keeps ownership pinned.
		switch session.Status {
		case "completed", "failed", "cancelled":
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
		lease.Release()
		delete(e.captureSessions, id)
	}
	for id, lease := range e.captureRecaptures {
		job, err := e.store.MediaRecapture(ctx, id)
		if err != nil || job.Status == "queued" || job.Status == "claimed" {
			continue
		}
		lease.Release()
		delete(e.captureRecaptures, id)
	}
}

func (e *Engine) ownedCapturePayload(run domain.Run, leaseID string, settings domain.Settings, round int, continuation map[string]any, reason string) (map[string]any, error) {
	if err := e.ensureCaptureSession(run.SessionID); err != nil {
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
		collector = e.selectCaptureCollector(run.Source, settings, driver)
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
