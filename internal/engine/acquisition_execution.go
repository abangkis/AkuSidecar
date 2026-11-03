package engine

import (
	"context"
	"errors"
	"fmt"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
)

// executeAcquisitionPlanning reports whether a queued follow-up must finish before
// candidate evaluation. The caller retains its existing lock and work ownership.
func (e *Engine) executeAcquisitionPlanning(ctx context.Context, runID string, run domain.Run, observations []domain.Observation, merged domain.Observation, settings domain.Settings, allowPlanning bool) (bool, error) {
	dailyRoundOne, err := e.store.IsDailySourceCapture(ctx, runID)
	if err != nil {
		return false, err
	}
	if allowPlanning && !dailyRoundOne && len(observations) == 1 && e.config.Capture.MaxAcquisitionRounds > 1 {
		localMode := "local_frontier"
		localReason, localFinish := localStructuredFallbackCompletionReason(run.Source, observations[0])
		if localFinish {
			localMode = "structured_fallback"
		} else {
			localReason, localFinish = localAcquisitionCompletionReason(run.Source, observations[0], observationCandidateCount(merged))
		}
		if localFinish {
			receipt := map[string]any{
				"mode":                  localMode,
				"decision":              "finish",
				"reason":                localReason,
				"followUpQueued":        false,
				"followUpNewCandidates": 0,
			}
			merged.Coverage["acquisitionPlanning"] = receipt
			if err := e.store.SetRunCoverageField(ctx, runID, "acquisitionPlanning", receipt); err != nil {
				return false, fmt.Errorf("save local acquisition planning receipt: %w", err)
			}
		} else if localReason, localFollowUp := localXFollowUpReason(run.Source, observations[0]); localFollowUp {
			continuation := continuationFrom(merged)
			payload, payloadErr := e.ownedCapturePayload(run, run.SessionID, settings, 2, continuation, localReason)
			if payloadErr != nil {
				return false, payloadErr
			}
			if _, err = e.store.QueueFollowUp(ctx, runID, payload); err != nil {
				return false, err
			}
			receipt := map[string]any{
				"mode":                  "local_frontier",
				"decision":              "request_follow_up",
				"reason":                localReason,
				"followUpQueued":        true,
				"followUpNewCandidates": 0,
			}
			merged.Coverage["acquisitionPlanning"] = receipt
			if err := e.store.SetRunCoverageField(ctx, runID, "acquisitionPlanning", receipt); err != nil {
				return false, fmt.Errorf("save local X acquisition planning receipt: %w", err)
			}
			return true, nil
		} else {
			if err := e.store.SetRunPipelineStage(ctx, runID, "acquisition_planning"); err != nil {
				return false, err
			}
			plan, telemetry, planErr := e.planWithProfile(ctx, run, merged, nil, settings.ReasoningAcquisitionProfile)
			_ = e.store.SaveTelemetry(context.Background(), telemetry)
			if planErr != nil {
				failure, typed := reasoning.ProviderFailureFrom(planErr)
				if ctx.Err() != nil || errors.Is(planErr, context.Canceled) || errors.Is(planErr, context.DeadlineExceeded) || !typed || failure.Code != "incomplete_response" || failure.Category != "response_missing" {
					return false, planErr
				}
				// Acquisition planning is optional. Never use the partial model
				// answer; retain accepted evidence and omit the extra capture.
				plan = reasoning.AcquisitionPlan{Decision: "finish", Reason: "The optional acquisition planner returned an incomplete response; evaluate the accepted capture without an extra acquisition round."}
			}
			receipt := map[string]any{
				"mode":                  "model",
				"decision":              plan.Decision,
				"reason":                plan.Reason,
				"followUpQueued":        false,
				"followUpNewCandidates": 0,
			}
			if planErr != nil {
				receipt["mode"] = "fallback"
				receipt["plannerStatus"] = "failed"
				receipt["failureCode"] = "incomplete_response"
				receipt["fallbackPolicy"] = "evaluate_captured_skip_follow_up"
			}
			if plan.Decision == "request_follow_up" {
				continuation := continuationFrom(merged)
				if continuation != nil {
					payload, payloadErr := e.ownedCapturePayload(run, run.SessionID, settings, 2, continuation, plan.Reason)
					if payloadErr != nil {
						return false, payloadErr
					}
					if _, err = e.store.QueueFollowUp(ctx, runID, payload); err != nil {
						return false, err
					}
					receipt["followUpQueued"] = true
				}
			}
			merged.Coverage["acquisitionPlanning"] = receipt
			if err := e.store.SetRunCoverageField(ctx, runID, "acquisitionPlanning", receipt); err != nil {
				return false, fmt.Errorf("save model acquisition planning receipt: %w", err)
			}
			if receipt["followUpQueued"] == true {
				return true, nil
			}
		}
	}
	return false, nil
}
