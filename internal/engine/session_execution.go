package engine

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/selection"
)

func (e *Engine) startNext(ctx context.Context, sessionID string) (*domain.Run, error) {
	if !e.fixtureAdmissionAllowed() {
		return nil, nil
	}
	e.schedule.Lock()
	defer e.schedule.Unlock()
	return e.startNextLocked(ctx, sessionID)
}

func (e *Engine) startNextLocked(ctx context.Context, sessionID string) (*domain.Run, error) {
	if !e.fixtureAdmissionAllowed() {
		return nil, nil
	}
	defer e.releaseTerminalCaptureSessions(context.Background())
	session, err := e.store.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session.Status != "queued" && session.Status != "running" {
		return nil, nil
	}
	run, err := e.store.AdvanceSession(ctx, sessionID)
	if err != nil {
		return run, err
	}
	// A durable pre-restart X run must not strand later non-X work or fall
	// through to the ordinary collector while the daily adapter is unavailable.
	skippedDailyX := false
	for run != nil && e.xDailyUnavailable(run.Source) {
		skippedDailyX = true
		failure := domain.Failure{Code: "x_concurrent_not_ready", Stage: "capture", Message: "X concurrent capture is not ready: batch-adapter-required", Retryable: true}
		if run.Source == domain.SourceInstagram {
			failure.Code = "instagram_concurrent_not_ready"
			failure.Message = "Instagram concurrent capture is not ready: native-adapter-unqualified"
		}
		if err := e.store.FailRun(ctx, run.ID, failure); err != nil {
			return nil, err
		}
		run, err = e.store.AdvanceSession(ctx, sessionID)
		if err != nil {
			return nil, err
		}
	}
	if skippedDailyX {
		session, err = e.store.GetSession(ctx, sessionID)
		if err != nil {
			return nil, err
		}
	}
	if run == nil {
		for _, candidate := range session.Runs {
			switch candidate.Status {
			case "completed", "failed", "cancelled":
			default:
				return nil, nil
			}
		}
		return nil, e.finalizeSessionLocked(ctx, sessionID)
	}
	if e.dailySourceReady(run.Source) {
		// The daily collector owns a separate browser, but Store still serializes
		// this session's capture commands. Drain predecessors before registering
		// a worker or starting native derivation; do not borrow the ordinary owner.
		// This conservative MVP boundary also waits for predecessor reasoning.
		drained, err := e.hybridPredecessorsDrained(ctx, session, *run)
		if err != nil {
			return nil, err
		}
		if !drained {
			return nil, nil
		}
		settings, err := e.store.GetSettings(ctx)
		if err != nil {
			return nil, err
		}
		payload := capturePayload(*run, session.ID, settings, 1, nil, "")
		payload["captureRuntime"] = map[string]any{"driver": "headless"}
		payload["captureCollector"] = collectorStamp("headless")
		if behavior := e.config.Capture.Behavior.Effective(); behavior.Enabled {
			payload["scrollFraction"] = behavior.ScrollFraction
			payload["scrollSettleMs"] = behavior.ScrollSettleMS
		}
		if err = e.dailyController(run.Source).queue(*run, payload); err != nil {
			if failErr := e.store.RejectUnavailableSourceCapture(ctx, run.Source, run.ID); failErr != nil {
				return nil, failErr
			}
			return e.startNextLocked(ctx, sessionID)
		}
		started, err := e.store.GetRun(ctx, run.ID)
		return &started, err
	}
	if hybridCollectionSession(session) {
		admitted, admissionErr := e.admitHybridRun(ctx, session, *run)
		if admissionErr != nil {
			return nil, admissionErr
		}
		if !admitted {
			return nil, nil
		}
	}
	settings, err := e.store.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	payload, err := e.ownedCapturePayload(*run, sessionID, settings, 1, nil, "")
	if err != nil {
		return nil, err
	}
	if _, err = e.store.StartRun(ctx, run.ID, payload); err != nil {
		return nil, err
	}
	started, err := e.store.GetRun(ctx, run.ID)
	return &started, err
}

// afterAcceptedObservation schedules work after persistence has been confirmed.
func (e *Engine) afterAcceptedObservation(ctx context.Context, run domain.Run) (domain.Run, error) {
	runID := run.ID
	session, err := e.store.GetSession(ctx, run.SessionID)
	if err != nil {
		return domain.Run{}, err
	}
	if sessionSourceWaitMode(session) == "progressive_wait" {
		if _, err := e.startNext(ctx, run.SessionID); err != nil {
			return domain.Run{}, err
		}
	}
	allowPlanning := true
	if firstRunStatus, statusErr := e.store.CalibrationFirstRunStatus(ctx); statusErr != nil {
		return domain.Run{}, statusErr
	} else if firstRunStatus == "pending" {
		allowPlanning = false
	}
	e.launchProcessAfterObservation(runID, allowPlanning)
	return e.store.GetRun(ctx, runID)
}

func sessionSourceWaitMode(session domain.Session) string {
	if value, ok := session.Coverage["sourceWaitMode"].(string); ok && value != "" {
		return value
	}
	return "full_wait"
}

// ResumePendingReasoning continues only from already accepted observations.
// Planning is not repeated because a restart must not request another browser
// acquisition for evidence that is already durable in SQLite.
func (e *Engine) ResumePendingReasoning(ctx context.Context) (int, error) {
	ids, err := e.store.ResumableReasoningRunIDs(ctx)
	if err != nil {
		return 0, err
	}
	progressiveSessions := map[string]bool{}
	for _, id := range ids {
		run, runErr := e.store.GetRun(ctx, id)
		if runErr != nil {
			return 0, runErr
		}
		if !progressiveSessions[run.SessionID] {
			session, sessionErr := e.store.GetSession(ctx, run.SessionID)
			if sessionErr != nil {
				return 0, sessionErr
			}
			if sessionSourceWaitMode(session) == "progressive_wait" {
				if _, scheduleErr := e.startNext(ctx, run.SessionID); scheduleErr != nil {
					return 0, scheduleErr
				}
			}
			progressiveSessions[run.SessionID] = true
		}
		e.launchProcess(id, false)
	}
	return len(ids), nil
}

func (e *Engine) process(ctx context.Context, runID string, allowPlanning bool) error {
	run, err := e.store.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	observations, err := e.store.Observations(ctx, runID)
	if err != nil {
		return err
	}
	if len(observations) == 0 {
		return errors.New("no accepted observation")
	}
	merged := mergeObservations(observations)
	mergeDurableRunCoverage(merged.Coverage, run.Coverage)
	if len(observations) > 1 {
		if receipt, ok := merged.Coverage["acquisitionPlanning"].(map[string]any); ok {
			receipt = cloneCoverageMap(receipt)
			receipt["followUpNewCandidates"] = followUpNewCandidateCount(observations)
			merged.Coverage["acquisitionPlanning"] = receipt
			if err := e.store.SetRunCoverageField(ctx, runID, "acquisitionPlanning", receipt); err != nil {
				return fmt.Errorf("save acquisition planning yield: %w", err)
			}
		}
	}
	settings, err := e.store.GetSettings(ctx)
	if err != nil {
		return err
	}
	continuity, err := e.store.ClassifyContentContinuity(ctx, run, merged, settings)
	if err != nil {
		return fmt.Errorf("classify resurfaced content: %w", err)
	}
	merged = filterResurfacedObservation(merged, continuity)
	// Instagram media-only candidates are admitted by the capture contract but
	// are never sent to the text-only reasoning provider. Persist them in the
	// Sidecar-owned bounded vision lane first; mixed observations continue with
	// their text-bearing candidates below.
	visionObservation, visionInputs := splitVisionLaneCandidates(merged)
	if len(visionInputs) > 0 {
		descriptor, _ := domain.SourceByID(run.Source)
		if _, err := e.store.EnqueueVisionEvaluations(ctx, run, visionInputs, descriptor.VisionQueueLimit); err != nil {
			return fmt.Errorf("enqueue media-only vision candidates: %w", err)
		}
		visionSummary, err := e.store.VisionEvaluationSummary(ctx, run.ID, run.Source)
		if err != nil {
			return fmt.Errorf("read media-only vision queue: %w", err)
		}
		merged.Coverage["visionEvaluation"] = visionEvaluationCoverage(visionSummary)
		if err := e.store.SetRunCoverageField(ctx, runID, "visionEvaluation", merged.Coverage["visionEvaluation"]); err != nil {
			return fmt.Errorf("save media-only vision queue receipt: %w", err)
		}
		merged = visionObservation
	}
	if observationCandidateCount(merged) == 0 {
		skipped := skippedResurfaceCount(continuity)
		if _, exists := merged.Coverage["acquisitionPlanning"]; !exists {
			receipt := map[string]any{
				"mode":                  "continuity_bypass",
				"decision":              "finish",
				"reason":                "All captured candidates were resolved as unchanged native resurfaces before acquisition planning.",
				"followUpQueued":        false,
				"followUpNewCandidates": 0,
			}
			merged.Coverage["acquisitionPlanning"] = receipt
			if err := e.store.SetRunCoverageField(ctx, runID, "acquisitionPlanning", receipt); err != nil {
				return fmt.Errorf("save continuity bypass receipt: %w", err)
			}
		}
		summary := fmt.Sprintf("%d unchanged resurfaced item%s skipped before reasoning inside the configured cooldown.", skipped, map[bool]string{true: "", false: "s"}[skipped == 1])
		if len(visionInputs) > 0 {
			summary = fmt.Sprintf("%d Instagram media-only candidate%s held for bounded vision evaluation; no text-bearing candidate entered reasoning.", len(visionInputs), map[bool]string{true: "", false: "s"}[len(visionInputs) == 1])
		}
		result := domain.ReasoningResult{
			Summary: summary,
			Items:   []domain.ReasonedItem{}, CandidateAssessments: []domain.CandidateAssessment{}, Limitations: []string{},
		}
		addedStarted := time.Now()
		if err = e.store.CompleteRun(ctx, run, result, nil, nil, merged.Coverage); err != nil {
			return err
		}
		_ = e.store.SaveRunStageTiming(context.Background(), runID, "added", time.Since(addedStarted))
		_, err = e.startNext(ctx, run.SessionID)
		return err
	}
	followUpQueued, err := e.executeAcquisitionPlanning(ctx, runID, run, observations, merged, settings, allowPlanning)
	if err != nil || followUpQueued {
		return err
	}
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
	_, err = e.startNext(ctx, run.SessionID)
	return err
}
