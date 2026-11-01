package engine

import (
	"context"
	"fmt"

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
			e.logger.Printf("semantic event resolution for session %s degraded safely: %v", sessionID, eventErr)
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
			e.logger.Printf("AI Fast Detection for session %s degraded safely: %v", sessionID, detectionErr)
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
			e.logger.Printf("record auto update success for session %s failed: %v", sessionID, recordErr)
		}
		e.scheduler.notify()
	}
	if _, retentionErr := e.store.EnforceRetention(ctx, settings); retentionErr != nil {
		e.logger.Printf("retention after session %s failed: %v", sessionID, retentionErr)
	}
	if _, calibrationErr := e.ensurePendingFirstCalibration(ctx, sessionID); calibrationErr != nil {
		e.logger.Printf("first-run calibration for session %s could not start: %v", sessionID, calibrationErr)
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
