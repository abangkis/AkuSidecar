package engine

import (
	"strings"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func filterResurfacedObservation(observation domain.Observation, decisions map[string]domain.ContentContinuityDecision) domain.Observation {
	result := observation
	result.Snapshots = make([]domain.Snapshot, 0, len(observation.Snapshots))
	continuity := map[string]any{}
	for _, snapshot := range observation.Snapshots {
		copy := snapshot
		copy.Blocks = make([]domain.Block, 0, len(snapshot.Blocks))
		for _, block := range snapshot.Blocks {
			decision, ok := decisions[strings.TrimSpace(block.EvidenceKey)]
			if ok && decision.Status != "fresh" {
				continuity[block.EvidenceKey] = map[string]any{
					"status":         decision.Status,
					"previousSeenAt": decision.PreviousSeenAt,
					"reason":         decision.Reason,
				}
			}
			if ok && decision.Action == "fail_fast" {
				continue
			}
			copy.Blocks = append(copy.Blocks, block)
		}
		result.Snapshots = append(result.Snapshots, copy)
	}
	if len(continuity) > 0 {
		if result.Coverage == nil {
			result.Coverage = map[string]any{}
		}
		result.Coverage["contentContinuity"] = continuity
	}
	return result
}

func observationCandidateCount(observation domain.Observation) int {
	seen := map[string]bool{}
	for _, snapshot := range observation.Snapshots {
		for _, block := range snapshot.Blocks {
			if key := strings.TrimSpace(block.EvidenceKey); key != "" {
				seen[key] = true
			}
		}
	}
	return len(seen)
}

func skippedResurfaceCount(decisions map[string]domain.ContentContinuityDecision) int {
	count := 0
	for _, decision := range decisions {
		if decision.Action == "fail_fast" {
			count++
		}
	}
	return count
}
