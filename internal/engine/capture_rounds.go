package engine

import (
	"math"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func mergeObservations(values []domain.Observation) domain.Observation {
	result := values[len(values)-1]
	result.Snapshots = nil
	// Candidate evidence spans all rounds; navigation and planning telemetry
	// belongs exclusively to the last round. Missing latest fields stay unknown.
	result.Coverage = cloneCoverageMap(result.Coverage)
	result.Coverage["acquisitionRounds"] = len(values)
	rounds := make([]map[string]any, 0, len(values))
	for _, value := range values {
		result.Snapshots = append(result.Snapshots, value.Snapshots...)
		rounds = append(rounds, value.Coverage)
	}
	result.Snapshots = reconcileCapturedSnapshots(result.Source, result.Snapshots)
	result.Coverage["rounds"] = rounds
	return result
}

func mergeDurableRunCoverage(target, durable map[string]any) {
	captureFields := map[string]bool{"rounds": true, "acquisitionRounds": true}
	if rounds, ok := target["rounds"].([]map[string]any); ok {
		for _, round := range rounds {
			for key := range round {
				captureFields[key] = true
			}
		}
	}
	for key, value := range durable {
		if captureFields[key] {
			continue
		}
		target[key] = value
	}
}

func cloneCoverageMap(value map[string]any) map[string]any {
	result := make(map[string]any, len(value))
	for key, item := range value {
		result[key] = item
	}
	return result
}

func followUpNewCandidateCount(observations []domain.Observation) int {
	if len(observations) < 2 {
		return 0
	}
	first := observationEvidenceKeys(observations[0].Source, observations[0].Snapshots)
	allSnapshots := []domain.Snapshot{}
	for _, observation := range observations {
		allSnapshots = append(allSnapshots, observation.Snapshots...)
	}
	all := observationEvidenceKeys(observations[0].Source, allSnapshots)
	count := 0
	for key := range all {
		if !first[key] {
			count++
		}
	}
	return count
}

func observationEvidenceKeys(source domain.Source, snapshots []domain.Snapshot) map[string]bool {
	result := map[string]bool{}
	for _, snapshot := range reconcileCapturedSnapshots(source, snapshots) {
		for _, block := range snapshot.Blocks {
			if key := strings.TrimSpace(block.EvidenceKey); key != "" {
				result[key] = true
			}
		}
	}
	return result
}

func continuationFrom(value domain.Observation) map[string]any {
	startScrollY := 0.0
	anchors := continuationAnchors(value.Coverage)
	frontier, hasFrontier := value.Coverage["frontier"].(map[string]any)
	if value.Source == domain.SourceLinkedIn && value.Coverage["captureMode"] == "headless_worker" {
		if len(value.Snapshots) == 0 || frontier["continuationReady"] != true {
			return nil
		}
		// The retained checkpoint's anchors survive candidate dedup.
		// Older workers retained only the terminal frontier.
		if checkpoint, exists := frontier["continuationCheckpoint"]; exists {
			var ok bool
			frontier, ok = checkpoint.(map[string]any)
			if !ok {
				return nil
			}
		}
		return retainedHeadlessContinuation(frontier)
	}
	if hasFrontier {
		if readiness, explicit := frontier["continuationReady"]; explicit {
			if ready, valid := readiness.(bool); !valid || !ready {
				return nil
			}
		}
		if scrollY, ok := frontier["scrollY"].(float64); ok && scrollY >= 0 {
			startScrollY = scrollY
		}
	}
	if len(value.Snapshots) == 0 {
		return nil
	}
	// Sources that virtualize and remeasure their feed after restoration resume
	// from the preceding observed snapshot. This proves continuity against a
	// bounded overlap without placing source-specific behavior in orchestration.
	descriptor, _ := domain.SourceByID(value.Source)
	if descriptor.ContinuationOverlapRequired && len(value.Snapshots) > 1 {
		for index := len(value.Snapshots) - 2; index >= 0; index-- {
			checkpoint := value.Snapshots[index]
			checkpointAnchors := snapshotContinuationAnchors(checkpoint)
			if len(checkpointAnchors) == 0 {
				continue
			}
			startScrollY = float64(checkpoint.ScrollY)
			anchors = checkpointAnchors
			break
		}
	}
	last := value.Snapshots[len(value.Snapshots)-1]
	if startScrollY == 0 && last.ScrollY > 0 {
		startScrollY = float64(last.ScrollY)
	}
	if len(anchors) == 0 {
		for _, block := range last.Blocks {
			identity := strings.TrimSpace(block.PlatformID)
			if identity == "" {
				identity = strings.TrimSpace(block.Permalink)
			}
			if identity == "" {
				identity = strings.ToLower(strings.Join(strings.Fields(block.Text), " "))
				if len(identity) > 300 {
					identity = identity[:300]
				}
			}
			if identity != "" {
				anchors = append(anchors, identity)
			}
			if len(anchors) == 3 {
				break
			}
		}
	}
	if len(anchors) == 0 {
		return nil
	}
	return map[string]any{"startScrollY": int(startScrollY), "anchorKeys": anchors, "settleMs": 900}
}

func retainedHeadlessContinuation(checkpoint map[string]any) map[string]any {
	scrollY, ok := checkpoint["scrollY"].(float64)
	if !ok || scrollY < 0 || scrollY > float64(1<<53-1) || scrollY != math.Trunc(scrollY) {
		return nil
	}
	// Preserve the worker's anchor order and values exactly across JSON.
	var anchors []string
	switch raw := checkpoint["anchorKeys"].(type) {
	case []any:
		if len(raw) > 3 {
			return nil
		}
		for _, entry := range raw {
			anchor, ok := entry.(string)
			if !ok {
				return nil
			}
			anchors = append(anchors, anchor)
		}
	case []string:
		anchors = raw
	}
	if len(anchors) == 0 || len(anchors) > 3 {
		return nil
	}
	for _, anchor := range anchors {
		if strings.TrimSpace(anchor) == "" || len(anchor) > 512 {
			return nil
		}
	}
	return map[string]any{"startScrollY": int64(scrollY), "anchorKeys": anchors, "settleMs": 900}
}

func snapshotContinuationAnchors(snapshot domain.Snapshot) []string {
	anchors := make([]string, 0, 3)
	for _, block := range snapshot.Blocks {
		identity := strings.TrimSpace(block.PlatformID)
		if identity == "" {
			identity = strings.TrimSpace(block.Permalink)
		}
		if identity == "" {
			identity = strings.ToLower(strings.Join(strings.Fields(block.Text), " "))
			if len(identity) > 300 {
				identity = identity[:300]
			}
		}
		if identity != "" {
			anchors = append(anchors, identity)
		}
		if len(anchors) == 3 {
			break
		}
	}
	return anchors
}

func continuationAnchors(coverage map[string]any) []string {
	frontier, ok := coverage["frontier"].(map[string]any)
	if !ok {
		return nil
	}
	var values []string
	switch anchors := frontier["anchorKeys"].(type) {
	case []any:
		for _, value := range anchors {
			if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
				values = append(values, strings.TrimSpace(text))
			}
			if len(values) == 3 {
				break
			}
		}
	case []string:
		for _, value := range anchors {
			if strings.TrimSpace(value) != "" {
				values = append(values, strings.TrimSpace(value))
			}
			if len(values) == 3 {
				break
			}
		}
	}
	return values
}
