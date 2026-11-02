package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/collection"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/store"
)

func (e *Engine) QueueMediaRecapture(ctx context.Context, timelineID string, mode domain.MediaRecaptureMode) (domain.MediaRecapture, error) {
	return e.QueueMediaRecaptureForReason(ctx, timelineID, mode, domain.MediaRecaptureMissingMedia)
}

func (e *Engine) QueueMediaRecaptureForReason(ctx context.Context, timelineID string, mode domain.MediaRecaptureMode, reason domain.MediaRecaptureReason) (domain.MediaRecapture, error) {
	if e.developmentSources != nil {
		return domain.MediaRecapture{}, errors.New("development session media recapture unavailable")
	}
	e.operation.Lock()
	defer e.operation.Unlock()
	item, err := e.store.TimelineItem(ctx, timelineID)
	if err != nil {
		return domain.MediaRecapture{}, err
	}
	settings, err := e.store.GetSettings(ctx)
	if err != nil {
		return domain.MediaRecapture{}, err
	}
	hybridFacebook := e.collectionRuntime != nil && settings.CollectionMode == "headless" && item.Source == domain.SourceFacebook
	if e.collectionRuntime != nil && e.collectionRuntime.Status().Pending && !hybridFacebook {
		return domain.MediaRecapture{}, errors.New("wait for the collection mode transition before recapturing media")
	}
	if active, err := e.store.ActiveSession(ctx); err != nil {
		return domain.MediaRecapture{}, err
	} else if active != nil {
		return domain.MediaRecapture{}, errors.New("finish the active update before recapturing media")
	}
	status := e.BridgeStatus()
	if !hybridFacebook && !e.headlessEffective() && !status.Compatible {
		return domain.MediaRecapture{}, fmt.Errorf("AkuBridge v2 is not ready: %s", strings.Join(status.Reasons, "; "))
	}
	if e.headlessEffective() && !hybridFacebook {
		e.mu.RLock()
		allowed := false
		for _, source := range e.headlessAccess {
			if source == item.Source {
				allowed = true
			}
		}
		e.mu.RUnlock()
		if !allowed {
			return domain.MediaRecapture{}, errors.New("headless recapture source is unsupported or access has not been granted")
		}
	}
	if hybridFacebook {
		job, err := e.store.CreateOwnedMediaRecaptureWithAdmission(ctx, timelineID, mode, reason, collection.BackendBridge, domain.MediaRecaptureCaptureAdmission{
			Policy: domain.MediaRecaptureAdmissionHybridHeadlessV1,
			Driver: "browser",
			Phase:  "waiting",
		})
		if err != nil {
			return domain.MediaRecapture{}, err
		}
		if err := e.beginBrowserCollectionHold(job.ID, item.Source); err != nil {
			e.logger.Printf("Facebook recapture %s is queued pending Browser collection admission: %v", job.ID, err)
		}
		return job, nil
	}
	lease, err := e.acquireCaptureLease()
	if err != nil {
		return domain.MediaRecapture{}, err
	}
	var stamp map[string]any
	driver := "browser"
	if lease != nil {
		driver = lease.Driver()
		stamp = map[string]any{"driver": lease.Driver(), "epoch": e.epoch, "generation": int(lease.Generation())}
	}
	collector := e.selectCaptureCollector(item.Source, settings, driver)
	if (mode == domain.MediaRecaptureForeground || hybridFacebook) && driver == "browser" {
		collector = collection.BackendBridge
	}
	job, err := e.store.CreateOwnedMediaRecapture(ctx, timelineID, mode, reason, stamp, collector)
	if err != nil {
		lease.Release()
		return domain.MediaRecapture{}, err
	}
	if lease != nil {
		e.captureMu.Lock()
		if e.captureRecaptures == nil {
			e.captureRecaptures = map[string]*captureruntime.Lease{}
		}
		if e.captureRecaptures[job.ID] != nil {
			lease.Release()
		} else {
			e.captureRecaptures[job.ID] = lease
		}
		e.captureMu.Unlock()
	}
	return job, nil
}

func (e *Engine) ClaimMediaRecapture(ctx context.Context, id, bridgeID string) (domain.MediaRecapture, error) {
	return e.claimMediaRecaptureForDriver(ctx, id, bridgeID, "browser")
}
func (e *Engine) claimMediaRecaptureForDriver(ctx context.Context, id, bridgeID, driver string) (domain.MediaRecapture, error) {
	collector := collection.BackendBridge
	if driver == "headless" {
		collector = collection.BackendHeadless
	}
	return e.claimMediaRecaptureForCollector(ctx, id, bridgeID, driver, collector)
}

func (e *Engine) claimMediaRecaptureForCollector(ctx context.Context, id, bridgeID, driver, collector string) (domain.MediaRecapture, error) {
	if e.developmentSources != nil {
		return domain.MediaRecapture{}, errors.New("development session media recapture unavailable")
	}
	defer e.releaseTerminalCaptureSessions(context.Background())
	job, err := e.store.MediaRecapture(ctx, id)
	if err != nil {
		return domain.MediaRecapture{}, err
	}
	releaseRoute, allowed := e.ordinarySourceRoute(job.Source)
	if !allowed {
		return domain.MediaRecapture{}, nil
	}
	defer releaseRoute()
	admission, hybrid, err := store.MediaRecaptureAdmission(job.Payload)
	if err != nil {
		return domain.MediaRecapture{}, err
	}
	if hybrid {
		if job.Source != domain.SourceFacebook || admission.Policy != domain.MediaRecaptureAdmissionHybridHeadlessV1 {
			return domain.MediaRecapture{}, errors.New("hybrid Browser recapture marker is not valid for this source")
		}
		if admission.Phase == "waiting" {
			return domain.MediaRecapture{}, nil
		}
		if !e.facebookSourceAuthorizedNow() {
			failure := domain.Failure{Code: "source_access_revoked", Stage: "capture", Message: "Facebook Browser permission is no longer confirmed.", Retryable: true}
			if _, failErr := e.store.FailMediaRecapture(ctx, id, failure); failErr != nil {
				return domain.MediaRecapture{}, failErr
			}
			return domain.MediaRecapture{}, errors.New("Facebook Browser permission is no longer confirmed")
		}
		if err := e.ensureAdmittedBrowserRecapture(job); err != nil {
			failure := domain.Failure{Code: "capture_runtime_changed", Stage: "capture", Message: "Media recapture owner changed; stale request was not dispatched.", Retryable: true}
			if _, failErr := e.store.FailMediaRecapture(ctx, id, failure); failErr != nil {
				return domain.MediaRecapture{}, failErr
			}
			return domain.MediaRecapture{}, err
		}
	} else if err := e.ensureCaptureRecapture(id); err != nil {
		return domain.MediaRecapture{}, err
	}
	e.captureMu.Lock()
	lease := e.captureRecaptures[id]
	wrongDriver := lease != nil && lease.Driver() != driver
	e.captureMu.Unlock()
	if wrongDriver {
		return domain.MediaRecapture{}, fmt.Errorf("%w: media recapture belongs to another collection driver", errStaleCaptureRuntime)
	}
	if err := e.validateRecaptureOwner(ctx, id); err != nil {
		failure := domain.Failure{Code: "capture_runtime_changed", Stage: "capture", Message: "Media recapture owner changed; stale request was not dispatched.", Retryable: true}
		if _, failErr := e.store.FailMediaRecapture(ctx, id, failure); failErr != nil {
			return domain.MediaRecapture{}, failErr
		}
		return domain.MediaRecapture{}, err
	}
	return e.store.ClaimMediaRecaptureWithBehavior(ctx, id, bridgeID, collector, e.capturePacingPolicy())
}

func (e *Engine) ensureAdmittedBrowserRecapture(job domain.MediaRecapture) error {
	e.captureMu.Lock()
	defer e.captureMu.Unlock()
	if e.captureOwner == nil {
		return errStaleCaptureRuntime
	}
	snapshot := e.captureOwner.Snapshot()
	if !mediaRecaptureStampMatches(job.Payload, snapshot, e.epoch) {
		return errStaleCaptureRuntime
	}
	lease := e.captureRecaptures[job.ID]
	if lease == nil {
		var err error
		lease, err = e.captureOwner.Acquire()
		if err != nil {
			return err
		}
		if e.captureRecaptures == nil {
			e.captureRecaptures = map[string]*captureruntime.Lease{}
		}
		e.captureRecaptures[job.ID] = lease
	}
	if lease.Driver() != "browser" || lease.Generation() != snapshot.Generation {
		return errStaleCaptureRuntime
	}
	return nil
}

func (e *Engine) AcceptMediaRecapture(ctx context.Context, id string, observation domain.Observation) (domain.MediaRecapture, error) {
	return e.acceptMediaRecapture(ctx, id, observation, false)
}

func (e *Engine) MediaRecapture(ctx context.Context, id string) (domain.MediaRecapture, error) {
	return e.store.MediaRecapture(ctx, id)
}

func (e *Engine) acceptMediaRecapture(ctx context.Context, id string, observation domain.Observation, internalHeadless bool) (domain.MediaRecapture, error) {
	defer e.releaseTerminalCaptureSessions(context.Background())
	if err := e.validateRecaptureOwner(ctx, id); err != nil {
		return domain.MediaRecapture{}, err
	}
	normalizeObservation(&observation)
	if err := validateObservation(observation); err != nil {
		return domain.MediaRecapture{}, err
	}
	var value domain.MediaRecapture
	var err error
	if internalHeadless {
		value, err = e.store.CompleteHeadlessMediaRecapture(ctx, id, observation)
	} else {
		value, err = e.store.CompleteMediaRecapture(ctx, id, observation)
	}
	if err == nil {
		if item, loadErr := e.store.TimelineItem(ctx, value.TimelineID); loadErr == nil {
			e.launchMediaProvenanceItems([]domain.TimelineItem{item})
		}
	}
	return value, err
}

func (e *Engine) FailMediaRecapture(ctx context.Context, id string, failure domain.Failure) (domain.MediaRecapture, error) {
	defer e.releaseTerminalCaptureSessions(context.Background())
	if err := e.validateRecaptureOwner(ctx, id); err != nil {
		return domain.MediaRecapture{}, err
	}
	e.captureBehaviorMu.Lock()
	defer e.captureBehaviorMu.Unlock()
	job, err := e.store.FailMediaRecapture(ctx, id, failure)
	if err == nil && job.Source == domain.SourceX && domain.IsAccessChallenge(failure.Code) {
		for _, cancel := range e.captureWork {
			cancel()
		}
	}
	return job, err
}

func (e *Engine) ApplyPassiveXMediaEvidence(ctx context.Context, timelineID, bridgeID string, value domain.PassiveXMediaEvidence) (domain.MediaRecapture, bool, error) {
	if e.developmentSources != nil {
		return domain.MediaRecapture{}, false, errors.New("development session passive media recapture unavailable")
	}
	defer e.releaseTerminalCaptureSessions(context.Background())
	e.operation.Lock()
	defer e.operation.Unlock()
	recapture, updated, err := e.store.ApplyPassiveXMediaEvidence(ctx, timelineID, bridgeID, value)
	if err == nil && updated {
		if item, loadErr := e.store.TimelineItem(ctx, timelineID); loadErr == nil {
			e.launchMediaProvenanceItems([]domain.TimelineItem{item})
		}
	}
	return recapture, updated, err
}
