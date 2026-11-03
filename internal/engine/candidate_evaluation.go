package engine

import (
	"context"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/selection"
)

// evaluateCandidatesAndCompleteRun persists one evaluated run. Session advancement
// and its schedule lock remain with the caller.
func (e *Engine) evaluateCandidatesAndCompleteRun(ctx context.Context, runID string, run domain.Run, merged domain.Observation, settings domain.Settings, continuity map[string]domain.ContentContinuityDecision) error {
	knowledge, err := e.store.Knowledge(ctx, run.Source, 200)
	if err != nil {
		return err
	}
	if err := e.store.SetRunPipelineStage(ctx, runID, "candidate_evaluation"); err != nil {
		return err
	}
	result, telemetry, err := e.analyzeWithProfile(ctx, run, merged, knowledge, settings.ReasoningEvaluationProfile)
	_ = e.store.SaveTelemetry(context.Background(), telemetry)
	_ = e.store.SaveRunStageTiming(context.Background(), runID, "evaluated", time.Duration(telemetry.DurationMS)*time.Millisecond)
	if err != nil {
		return err
	}
	bindReasoningSourceURLs(merged, &result)
	if err = validateReasoning(merged, result); err != nil {
		return err
	}
	profile, err := e.preferenceProfile(ctx, true)
	if err != nil {
		return err
	}
	evidenceKeys := make([]string, 0, len(result.CandidateAssessments))
	for _, assessment := range result.CandidateAssessments {
		evidenceKeys = append(evidenceKeys, assessment.EvidenceKey)
	}
	excluded, err := e.store.PreviouslyDeliveredEvidence(ctx, run.Source, evidenceKeys)
	if err != nil {
		return err
	}
	for evidenceKey, decision := range continuity {
		if decision.Action == "evaluate" && (decision.Status == "resurfaced_changed" || decision.Status == "resurfaced_after_cooldown") {
			delete(excluded, evidenceKey)
		}
	}
	protected := map[string]bool{}
	eventKeys := make([]string, 0, len(result.Items))
	for _, item := range result.Items {
		if item.EventKey != "" {
			eventKeys = append(eventKeys, item.EventKey)
		}
	}
	priorEvents, err := e.store.PreviouslyKnownEvents(ctx, run.Source, eventKeys)
	if err != nil {
		return err
	}
	for _, item := range result.Items {
		if excluded[item.EvidenceKey] {
			continue
		}
		if item.KnowledgeDelta == "material_update" || item.KnowledgeDelta == "contradiction" {
			protected[item.EvidenceKey] = true
			continue
		}
		if item.EventKey != "" && priorEvents[item.EventKey] {
			excluded[item.EvidenceKey] = true
		}
	}
	selectionStarted := time.Now()
	scored := selection.SelectWithOptions(result.CandidateAssessments, profile, selection.Options{
		Limit: settings.MaxItemsPerSource, Mode: settings.PreferenceEligibilityMode,
		ExcludedEvidence: excluded, ProtectedEvidence: protected,
	})
	_ = e.store.SaveRunStageTiming(context.Background(), runID, "selected", time.Since(selectionStarted))
	items := selectedItems(run, result, scored, merged.Coverage)
	addedStarted := time.Now()
	if err = e.store.CompleteRun(ctx, run, result, scored, items, merged.Coverage); err != nil {
		return err
	}
	_ = e.store.SaveRunStageTiming(context.Background(), runID, "added", time.Since(addedStarted))
	return nil
}
