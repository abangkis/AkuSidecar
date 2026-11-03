package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

// completeRunWithoutCandidates persists a run that bypasses text reasoning.
// Session advancement and its schedule lock remain with the caller.
func (e *Engine) completeRunWithoutCandidates(ctx context.Context, runID string, run domain.Run, merged domain.Observation, continuity map[string]domain.ContentContinuityDecision, visionCandidateCount int) error {
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
	if visionCandidateCount > 0 {
		summary = fmt.Sprintf("%d Instagram media-only candidate%s held for bounded vision evaluation; no text-bearing candidate entered reasoning.", visionCandidateCount, map[bool]string{true: "", false: "s"}[visionCandidateCount == 1])
	}
	result := domain.ReasoningResult{
		Summary: summary,
		Items:   []domain.ReasonedItem{}, CandidateAssessments: []domain.CandidateAssessment{}, Limitations: []string{},
	}
	addedStarted := time.Now()
	if err := e.store.CompleteRun(ctx, run, result, nil, nil, merged.Coverage); err != nil {
		return err
	}
	_ = e.store.SaveRunStageTiming(context.Background(), runID, "added", time.Since(addedStarted))
	return nil
}
