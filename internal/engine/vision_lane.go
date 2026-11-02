package engine

import "github.com/abangkis/AkuSidecar/internal/domain"

func splitVisionLaneCandidates(observation domain.Observation) (domain.Observation, []domain.VisionEvaluationInput) {
	result := observation
	result.Snapshots = make([]domain.Snapshot, len(observation.Snapshots))
	inputs := []domain.VisionEvaluationInput{}
	for snapshotIndex, snapshot := range observation.Snapshots {
		result.Snapshots[snapshotIndex] = snapshot
		result.Snapshots[snapshotIndex].Blocks = make([]domain.Block, 0, len(snapshot.Blocks))
		for _, block := range snapshot.Blocks {
			if domain.MediaOnlyVisionRequired(observation.Source, block) {
				inputs = append(inputs, domain.VisionEvaluationInput{EvidenceKey: block.EvidenceKey, Candidate: block})
				continue
			}
			result.Snapshots[snapshotIndex].Blocks = append(result.Snapshots[snapshotIndex].Blocks, block)
		}
	}
	return result, inputs
}

func visionEvaluationCoverage(value domain.InboxVisionEvaluation) map[string]any {
	return map[string]any{
		"policy": value.Policy, "available": value.Available, "availability": value.Availability,
		"capacity": value.Capacity, "active": value.Active, "pending": value.Pending,
		"deferred": value.Deferred, "evaluating": value.Evaluating, "retrying": value.Retrying,
		"ready": value.Ready, "failed": value.Failed,
	}
}
