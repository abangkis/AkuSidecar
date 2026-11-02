package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/abangkis/AkuSidecar/internal/collection"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

func (e *Engine) ClaimCommand(ctx context.Context, runID, bridgeID string) (*domain.BridgeCommand, error) {
	return e.claimCommandForDriver(ctx, runID, bridgeID, "browser")
}

func (e *Engine) claimCommandForDriver(ctx context.Context, runID, bridgeID, driver string) (*domain.BridgeCommand, error) {
	collector := collection.BackendBridge
	if driver == "headless" {
		collector = collection.BackendHeadless
	}
	return e.claimCommandForCollector(ctx, runID, bridgeID, driver, collector)
}

func (e *Engine) claimCommandForCollector(ctx context.Context, runID, bridgeID, driver, collector string) (*domain.BridgeCommand, error) {
	if e.developmentSources != nil && (driver != "headless" || collector != collection.BackendHeadless) {
		return nil, errors.New("development session collector unavailable")
	}
	if !e.fixtureAdmissionAllowed() {
		return nil, nil
	}
	defer e.releaseTerminalCaptureSessions(context.Background())
	if err := e.recoverExpiredBridgeCommands(ctx, time.Now()); err != nil {
		return nil, err
	}
	run, err := e.store.GetRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	if e.developmentSources != nil && !e.developmentSources[run.Source] {
		return nil, errors.New("development session source unsupported")
	}
	releaseRoute, allowed := e.ordinarySourceRoute(run.Source)
	if !allowed {
		return nil, nil
	}
	defer releaseRoute()
	session, err := e.store.GetSession(ctx, run.SessionID)
	if err != nil {
		return nil, err
	}
	if hybridCollectionSession(session) {
		plannedDriver, ok := hybridDriverForRun(session, run.Source)
		if !ok || plannedDriver != driver {
			return nil, fmt.Errorf("%w: requested driver conflicts with the frozen hybrid source plan", errStaleCaptureRuntime)
		}
		if err := e.ensureHybridSessionLease(run.SessionID, plannedDriver); err != nil {
			return nil, err
		}
	} else if err := e.ensureCaptureSession(run.SessionID); err != nil {
		return nil, err
	}
	e.captureMu.Lock()
	lease := e.captureSessions[run.SessionID]
	wrongDriver := lease != nil && lease.Driver() != driver
	e.captureMu.Unlock()
	if wrongDriver {
		return nil, fmt.Errorf("%w: command belongs to another collection driver", errStaleCaptureRuntime)
	}
	command, err := e.store.ClaimCommandWithBehavior(ctx, runID, bridgeID, collector, e.capturePacingPolicy())
	if err != nil || command == nil {
		return command, err
	}
	if err := e.validateCaptureOwner(run.SessionID, command.Payload); err != nil {
		failure := domain.Failure{Code: "capture_runtime_changed", Stage: "capture", Message: "Capture owner changed; stale command was not dispatched.", Retryable: true}
		if _, failErr := e.FailCommand(ctx, command.ID, runID, failure); failErr != nil {
			return nil, failErr
		}
		return nil, err
	}
	return command, nil
}

func (e *Engine) PendingBridgeRunID(ctx context.Context) (string, error) {
	if err := e.recoverExpiredBridgeCommands(ctx, time.Now()); err != nil {
		return "", err
	}
	return e.ordinaryPendingRun(ctx, collection.BackendBridge)
}

func (e *Engine) recoverExpiredBridgeCommands(ctx context.Context, now time.Time) error {
	commands, err := e.store.ExpiredBridgeCommands(ctx, now)
	if err != nil {
		return err
	}
	for _, command := range commands {
		failure := domain.Failure{
			Code:      "capture_lease_expired",
			Stage:     "capture",
			Message:   "AkuBridge did not finish the claimed capture before its bounded lease expired.",
			Retryable: true,
			Details: map[string]any{
				"commandId": command.ID,
				"claimedAt": command.ClaimedAt,
			},
		}
		if command.Status == "queued" {
			failure.Code = "capture_dispatch_timeout"
			failure.Message = "Browser capture command was not claimed within its dispatch budget."
			failure.Details = map[string]any{"commandId": command.ID, "createdAt": command.CreatedAt}
		}
		var failErr error
		if command.Status == "queued" {
			var fallback bool
			fallback, failErr = e.store.ExpireQueuedBridgeCommand(ctx, command.ID, command.RunID, failure, now)
			if failErr == nil {
				if fallback {
					e.launchProcess(command.RunID, false)
				} else if run, err := e.store.GetRun(ctx, command.RunID); err == nil {
					_, _ = e.startNext(ctx, run.SessionID)
				}
			}
		} else {
			_, failErr = e.FailCommand(ctx, command.ID, command.RunID, failure)
		}
		if err := failErr; err != nil {
			if strings.Contains(err.Error(), "bridge command is not active") {
				continue
			}
			return err
		}
	}
	return nil
}

func (e *Engine) AcceptObservation(ctx context.Context, commandID, runID string, value domain.Observation) (domain.Run, error) {
	run, err := e.store.GetRun(ctx, runID)
	if err != nil {
		return domain.Run{}, err
	}
	payload, err := e.store.CaptureCommandPayload(ctx, commandID, runID)
	if err != nil {
		return domain.Run{}, err
	}
	if err := e.validateCaptureOwner(run.SessionID, payload); err != nil {
		return domain.Run{}, err
	}
	if value.Source != run.Source {
		return domain.Run{}, errors.New("observation source does not match run")
	}
	normalizeObservation(&value)
	if err := validateObservation(value); err != nil {
		return domain.Run{}, err
	}
	if err := e.store.SaveObservation(ctx, commandID, runID, value); err != nil {
		return domain.Run{}, err
	}
	return e.afterAcceptedObservation(ctx, run)
}

func (e *Engine) FailCommand(ctx context.Context, commandID, runID string, failure domain.Failure) (domain.Run, error) {
	if failure.Code == "" {
		failure.Code = "bridge_failure"
	}
	if failure.Stage == "" {
		failure.Stage = "capture"
	}
	e.captureBehaviorMu.Lock()
	fallback, err := e.store.FailCommand(ctx, commandID, runID, failure)
	if err == nil && domain.IsAccessChallenge(failure.Code) {
		if run, loadErr := e.store.GetRun(ctx, runID); loadErr == nil && run.Source == domain.SourceX {
			for _, cancel := range e.captureWork {
				cancel()
			}
		}
	}
	e.captureBehaviorMu.Unlock()
	if err != nil {
		return domain.Run{}, err
	}
	run, err := e.store.GetRun(ctx, runID)
	if err == nil {
		if fallback {
			e.launchProcess(runID, false)
		} else {
			_, _ = e.startNext(ctx, run.SessionID)
		}
	}
	return run, err
}
