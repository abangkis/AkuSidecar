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
			if !e.headlessEffective() {
				continue
			}
			lease, err := e.acquireCaptureLease()
			if err != nil {
				continue
			}
			if lease == nil || lease.Driver() != "headless" {
				lease.Release()
				continue
			}
			func() {
				defer lease.Release()
				id, err := e.PendingBridgeRunID(ctx)
				if err != nil || id == "" {
					e.captureHeadlessMediaRecapture(ctx)
					return
				}
				command, err := e.claimCommandForDriver(ctx, id, "aku-headless-v1", "headless")
				if err != nil || command == nil {
					return
				}
				run, err := e.store.GetRun(ctx, id)
				if err != nil {
					return
				}
				observation, err := e.collectionRuntime.Capture(ctx, run.Source, command.Payload)
				if ctx.Err() != nil {
					return
				}
				if err == nil {
					_, err = e.AcceptObservation(ctx, command.ID, id, observation)
				}
				if err != nil {
					_, _ = e.FailCommand(ctx, command.ID, id, headlessFailure(err))
				}
			}()
		}
	}()
}

func (e *Engine) captureHeadlessMediaRecapture(ctx context.Context) {
	ids, err := e.store.ActiveMediaRecaptureIDs(ctx)
	if err != nil {
		return
	}
	for _, id := range ids {
		job, err := e.store.MediaRecapture(ctx, id)
		if err != nil || job.Status != "queued" {
			continue
		}
		job, err = e.claimMediaRecaptureForDriver(ctx, id, "aku-headless-v1", "headless")
		if err != nil {
			continue
		}
		payload := map[string]any{}
		for key, value := range job.Payload {
			payload[key] = value
		}
		payload["pageUrl"] = job.TargetURL
		observation, err := e.collectionRuntime.Capture(ctx, job.Source, payload)
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			_, err = e.AcceptMediaRecapture(ctx, id, observation)
		}
		if err != nil {
			_, _ = e.FailMediaRecapture(ctx, id, headlessFailure(err))
		}
		return
	}
}

func headlessFailure(err error) domain.Failure {
	failure := domain.Failure{Code: "headless_capture_failed", Stage: "capture", Message: err.Error(), Retryable: true}
	var typed *headless.CaptureError
	if errors.As(err, &typed) {
		failure.Code = "headless_" + typed.Code
		if typed.Code == "login_required" || typed.Code == "challenge_required" || typed.Code == "challenge_detected" || typed.Code == "unsupported_source" || typed.Code == "unsupported_continuation" {
			failure.Retryable = false
		}
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
	defer e.captureMu.Unlock()
	if lease := e.captureSessions[run.SessionID]; lease != nil {
		payload["captureRuntime"] = map[string]any{"driver": lease.Driver(), "epoch": e.epoch, "generation": int(lease.Generation())}
	}
	return payload, nil
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
