package selection

import (
	"sort"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/preference"
	"github.com/abangkis/AkuSidecar/internal/store"
)

type candidate struct {
	index  int
	scored store.ScoredAssessment
}

func Select(assessments []domain.CandidateAssessment, profile preference.Profile, limit int, mode string) []store.ScoredAssessment {
	candidates := make([]candidate, 0, len(assessments))
	for index, assessment := range assessments {
		base := baseScore(assessment)
		pref := preference.Score(profile, assessment)
		candidates = append(candidates, candidate{index: index, scored: store.ScoredAssessment{Assessment: assessment, BaseScore: base, PreferenceScore: pref, FinalScore: base + pref}})
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].scored.FinalScore > candidates[j].scored.FinalScore })
	// Preference is intentionally bounded to two positions in either direction.
	for i := 0; i < len(candidates); i++ {
		original := candidates[i].index
		if i < original-2 {
			target := original - 2
			if target >= len(candidates) {
				target = len(candidates) - 1
			}
			value := candidates[i]
			copy(candidates[i:target], candidates[i+1:target+1])
			candidates[target] = value
		}
	}
	selected := 0
	for i := range candidates {
		eligible := candidates[i].scored.Assessment.EvidenceStrength >= 0.35 && candidates[i].scored.Assessment.Materiality >= 0.25
		if eligible && selected < limit {
			candidates[i].scored.Selected = true
			selected++
		}
	}
	if mode == "promote_unused_budget" && selected < limit && profile.PromotionReady {
		for i := range candidates {
			if !candidates[i].scored.Selected && candidates[i].scored.FinalScore >= 0.55 {
				candidates[i].scored.Selected = true
				break
			}
		}
	}
	result := make([]store.ScoredAssessment, len(candidates))
	for i, value := range candidates {
		result[i] = value.scored
	}
	return result
}

func baseScore(value domain.CandidateAssessment) float64 {
	return 0.30*value.Materiality + 0.20*value.Novelty + 0.15*value.Urgency + 0.15*value.Actionability + 0.20*value.EvidenceStrength
}
