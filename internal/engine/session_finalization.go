package engine

import (
	"context"
	"fmt"

	"github.com/abangkis/AkuSidecar/internal/diagnostics/operational"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

// finalizeSessionLocked runs finalization after every run is terminal.
// The caller holds e.schedule; capture-lease cleanup remains with startNextLocked.
func (e *Engine) finalizeSessionLocked(ctx context.Context, sessionID string) error {
	settings, settingsErr := e.store.GetSettings(ctx)
	if settingsErr != nil {
		return settingsErr
	}
	calibrationFirstRunStatus, calibrationStatusErr := e.store.CalibrationFirstRunStatus(ctx)
	if calibrationStatusErr != nil {
		return calibrationStatusErr
	}
	onboardingFastPath := calibrationFirstRunStatus == "pending"
	if e.events != nil && settings.SemanticEventMode != "show_all" {
		if stageErr := e.store.SetSessionPipelineStage(ctx, sessionID, "semantic_event_resolution"); stageErr != nil {
			return stageErr
		}
		var eventErr error
		if onboardingFastPath {
			_, eventErr = e.events.ProcessOnboardingSession(ctx, sessionID, settings)
		} else {
			_, eventErr = e.events.ProcessSession(ctx, sessionID, settings)
		}
		if eventErr != nil {
			operational.Problem(e.logger, "session_finalization", "semantic_resolution", "degraded", eventErr, sessionID)
			e.pauseAutoUpdateForUsageLimit(context.Background(), sessionID, eventErr)
		}
	}
	if stageErr := e.store.SetSessionPipelineStage(ctx, sessionID, "timeline_composition"); stageErr != nil {
		return stageErr
	}
	if composeErr := e.store.ComposeSession(ctx, sessionID); composeErr != nil {
		return fmt.Errorf("compose unified Timeline: %w", composeErr)
	}
	if settings.AIDetectionEnabled && !onboardingFastPath {
		if stageErr := e.store.SetSessionPipelineStage(ctx, sessionID, "ai_fast_detection"); stageErr != nil {
			return stageErr
		}
		if detectionErr := e.runFastDetection(ctx, sessionID); detectionErr != nil {
			operational.Problem(e.logger, "session_finalization", "fast_detection", "degraded", detectionErr, sessionID)
		}
	}
	if stageErr := e.store.SetSessionPipelineStage(ctx, sessionID, "finalizing"); stageErr != nil {
		return stageErr
	}
	if finalizeErr := e.store.FinalizeSession(ctx, sessionID); finalizeErr != nil {
		return fmt.Errorf("finalize unified session: %w", finalizeErr)
	}
	completedSession, policyErr := e.store.GetSession(ctx, sessionID)
	if policyErr != nil {
		return policyErr
	}
	e.clearAutoUpdateUsageLimitAfterSuccessfulUserSession(ctx, completedSession)
	if completedSession.Delivery == domain.UpdateDeliveryPrepared {
		if recordErr := e.store.RecordAutoUpdateSuccess(ctx); recordErr != nil {
			operational.Problem(e.logger, "session_finalization", "record_update", "failed", recordErr, sessionID)
		}
		e.scheduler.notify()
	}
	if _, retentionErr := e.store.EnforceRetention(ctx, settings); retentionErr != nil {
		operational.Problem(e.logger, "session_finalization", "retention", "failed", retentionErr, sessionID)
	}
	if _, calibrationErr := e.ensurePendingFirstCalibration(ctx, sessionID); calibrationErr != nil {
		operational.Problem(e.logger, "session_finalization", "start_calibration", "failed", calibrationErr, sessionID)
	}
	if settings.AIDetectionEnabled && !onboardingFastPath && (completedSession.BudgetAuthority != domain.BudgetAuthorityAutomatic || e.autoDeepDetectionAllowed(ctx, settings)) {
		e.launchDeepDetection(sessionID)
	}
	if settings.AIDetectionEnabled && !onboardingFastPath {
		e.launchMediaProvenance(sessionID)
	}
	e.launchLivingTopicRouting(sessionID)
	return nil
}
