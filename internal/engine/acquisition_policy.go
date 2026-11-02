package engine

import (
	"strings"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func localFrontierFinishesAcquisition(source domain.Source, observation domain.Observation) bool {
	_, finished := localAcquisitionCompletionReason(source, observation, observationCandidateCount(observation))
	return finished
}

func localStructuredFallbackCompletionReason(source domain.Source, observation domain.Observation) (string, bool) {
	if source != domain.SourceInstagram || observationCandidateCount(observation) == 0 {
		return "", false
	}
	if strings.TrimSpace(stringCoverageValue(observation.Coverage["captureMethod"])) != "instagram_structured_feed_json" ||
		strings.TrimSpace(stringCoverageValue(observation.Coverage["scrollStopReason"])) != "structured_fallback" ||
		integerCoverageValue(observation.Coverage["performedScrolls"]) != 0 {
		return "", false
	}
	frontier, ok := observation.Coverage["frontier"].(map[string]any)
	if !ok || booleanCoverageValue(frontier["hasMoreCandidateSignal"]) || len(continuationAnchors(observation.Coverage)) > 0 {
		return "", false
	}
	return "The structured Instagram fallback produced bounded candidates but no navigable continuation frontier; repeating the same fallback would not extend coverage.", true
}

func localAcquisitionCompletionReason(source domain.Source, observation domain.Observation, eligibleCandidateCount int) (string, bool) {
	descriptor, ok := domain.SourceByID(source)
	if !ok || descriptor.FollowUpPlanningPolicy != "local_frontier" {
		return "", false
	}
	if integerCoverageValue(observation.Coverage["performedScrolls"]) < 1 {
		return "", false
	}
	if descriptor.FrontierRequiresComplete {
		captureQuality, ok := observation.Coverage["captureQuality"].(map[string]any)
		if !ok || strings.TrimSpace(stringCoverageValue(captureQuality["verdict"])) != "complete" {
			return "", false
		}
		if strings.TrimSpace(stringCoverageValue(observation.Coverage["scrollStopReason"])) == "deadline" {
			return "", false
		}
	}
	frontier, ok := observation.Coverage["frontier"].(map[string]any)
	if !ok {
		return "", false
	}
	if descriptor.InitialRoundCandidateTarget > 0 &&
		eligibleCandidateCount >= descriptor.InitialRoundCandidateTarget &&
		integerCoverageValue(frontier["newCandidateCount"]) >= descriptor.InitialRoundCandidateTarget {
		return "The complete first capture round already produced the configured bounded candidate target; older-feed coverage is deferred to the next finite check.", true
	}
	if booleanCoverageValue(frontier["hasMoreCandidateSignal"]) {
		return "", false
	}
	if integerCoverageValue(frontier["newCandidateCount"]) == 0 {
		return "The complete bounded source frontier produced no new candidate after scrolling.", true
	}
	return "", false
}

// localXFollowUpReason handles only the complete, budget-exhausted X frontier
// shape whose outcome is mechanically encoded by Bridge telemetry. Any missing,
// degraded, contradictory, or otherwise ambiguous signal remains model-planned.
func localXFollowUpReason(source domain.Source, observation domain.Observation) (string, bool) {
	descriptor, ok := domain.SourceByID(source)
	if !ok || descriptor.FollowUpPlanningPolicy != "guarded_request_frontier" || integerCoverageValue(observation.Coverage["performedScrolls"]) < 1 ||
		strings.TrimSpace(stringCoverageValue(observation.Coverage["scrollStopReason"])) != "budget_exhausted" {
		return "", false
	}
	quality, ok := observation.Coverage["captureQuality"].(map[string]any)
	if !ok || strings.TrimSpace(stringCoverageValue(quality["verdict"])) != "complete" {
		return "", false
	}
	frontier, ok := observation.Coverage["frontier"].(map[string]any)
	if !ok || !booleanCoverageValue(frontier["hasMoreCandidateSignal"]) || integerCoverageValue(frontier["newCandidateCount"]) < 1 ||
		len(continuationAnchors(observation.Coverage)) == 0 || continuationFrom(observation) == nil {
		return "", false
	}
	return "The complete X capture exhausted its bounded scroll budget while reporting a navigable older-feed frontier; queue one adjacent follow-up round.", true
}
