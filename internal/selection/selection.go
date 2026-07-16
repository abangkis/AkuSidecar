package selection

import (
	"math"
	"sort"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/preference"
	"github.com/abangkis/AkuSidecar/internal/store"
)

const (
	minimumEvidence          = 0.35
	baselineAdmission        = 0.40
	preferenceTrustEvidence  = 0.50
	preferenceAdmission      = 0.25
	promotionThreshold       = 0.25
	suppressionThreshold     = -0.25
	discoveryAlignmentWindow = 0.15
)

type Options struct {
	Limit             int
	Mode              string
	ExcludedEvidence  map[string]bool
	ProtectedEvidence map[string]bool
}

type candidate struct {
	index            int
	scored           store.ScoredAssessment
	alignment        float64
	baselineEligible bool
	protected        bool
	promotable       bool
	suppressed       bool
	discovery        bool
}

func Select(assessments []domain.CandidateAssessment, profile preference.Profile, limit int, mode string) []store.ScoredAssessment {
	return SelectWithOptions(assessments, profile, Options{Limit: limit, Mode: mode})
}

// SelectWithOptions applies three independent layers:
//  1. generic trust/materiality admission,
//  2. direct user preference authority, and
//  3. a bounded discovery lane.
//
// Exact previously-delivered evidence is excluded before every layer. In
// guarded_live, direct feedback may promote, replace, demote, and suppress
// ordinary candidates, but it cannot suppress protected material updates.
func SelectWithOptions(assessments []domain.CandidateAssessment, profile preference.Profile, options Options) []store.ScoredAssessment {
	if options.Limit < 0 {
		options.Limit = 0
	}
	candidates := make([]candidate, 0, len(assessments))
	for index, assessment := range assessments {
		base := baseScore(assessment)
		alignment := 0.0
		if profile.AuthorityReady {
			alignment = preference.Alignment(profile, assessment)
		}
		pref := preference.Score(profile, assessment)
		excluded := options.ExcludedEvidence[assessment.EvidenceKey]
		protected := !excluded && assessment.EvidenceStrength >= minimumEvidence && (options.ProtectedEvidence[assessment.EvidenceKey] || assessment.Materiality >= 0.85 || assessment.Urgency >= 0.85 || assessment.Novelty >= 0.90)
		baseline := !excluded && assessment.EvidenceStrength >= minimumEvidence && (base >= baselineAdmission || protected)
		promotable := !excluded && profile.PromotionReady && assessment.EvidenceStrength >= preferenceTrustEvidence && base >= preferenceAdmission && alignment >= promotionThreshold
		suppressed := options.Mode == "guarded_live" && profile.SuppressionReady && alignment <= suppressionThreshold && !protected
		discovery := baseline && !suppressed && math.Abs(alignment) <= discoveryAlignmentWindow
		candidates = append(candidates, candidate{
			index: index,
			scored: store.ScoredAssessment{
				Assessment: assessment,
				BaseScore:  base, PreferenceScore: pref, FinalScore: base + pref,
			},
			alignment: alignment, baselineEligible: baseline, protected: protected,
			promotable: promotable, suppressed: suppressed, discovery: discovery,
		})
	}

	order := make([]int, len(candidates))
	for index := range candidates {
		order[index] = index
	}
	if profile.AuthorityReady {
		sort.SliceStable(order, func(i, j int) bool {
			return candidates[order[i]].scored.FinalScore > candidates[order[j]].scored.FinalScore
		})
	}

	selected := map[int]bool{}
	pick := func(predicate func(candidate) bool) {
		for _, index := range order {
			if len(selected) >= options.Limit {
				return
			}
			if !selected[index] && predicate(candidates[index]) {
				selected[index] = true
			}
		}
	}

	switch options.Mode {
	case "promote_unused_budget":
		pick(func(value candidate) bool { return value.baselineEligible })
		pick(func(value candidate) bool { return value.promotable })
	case "guarded_live":
		pick(func(value candidate) bool {
			return !value.suppressed && (value.baselineEligible || value.promotable)
		})
		enforceProtected(candidates, selected, options.Limit)
		enforceDiscovery(candidates, selected, options.Limit)
	default: // rank_only
		pick(func(value candidate) bool { return value.baselineEligible })
	}

	selectedOrder := make([]int, 0, len(selected))
	for _, index := range order {
		if selected[index] {
			selectedOrder = append(selectedOrder, index)
		}
	}
	result := make([]store.ScoredAssessment, 0, len(candidates))
	for _, index := range selectedOrder {
		value := candidates[index].scored
		value.Selected = true
		result = append(result, value)
	}
	for index := range candidates {
		if !selected[index] {
			result = append(result, candidates[index].scored)
		}
	}
	return result
}

func enforceProtected(candidates []candidate, selected map[int]bool, limit int) {
	if limit == 0 {
		return
	}
	for index, value := range candidates {
		if !value.protected || selected[index] {
			continue
		}
		if len(selected) < limit {
			selected[index] = true
			continue
		}
		replace := weakestSelected(candidates, selected, func(candidate candidate) bool { return !candidate.protected })
		if replace >= 0 {
			delete(selected, replace)
			selected[index] = true
		}
	}
}

func enforceDiscovery(candidates []candidate, selected map[int]bool, limit int) {
	if limit == 0 {
		return
	}
	for index := range selected {
		if candidates[index].discovery {
			return
		}
	}
	best := -1
	for index, value := range candidates {
		if !value.discovery || selected[index] {
			continue
		}
		if best < 0 || value.scored.BaseScore > candidates[best].scored.BaseScore {
			best = index
		}
	}
	if best < 0 {
		return
	}
	if len(selected) < limit {
		selected[best] = true
		return
	}
	replace := weakestSelected(candidates, selected, func(candidate candidate) bool {
		return !candidate.protected && !candidate.discovery
	})
	if replace >= 0 {
		delete(selected, replace)
		selected[best] = true
	}
}

func weakestSelected(candidates []candidate, selected map[int]bool, allowed func(candidate) bool) int {
	result := -1
	for index := range selected {
		if !allowed(candidates[index]) {
			continue
		}
		if result < 0 || candidates[index].scored.FinalScore < candidates[result].scored.FinalScore {
			result = index
		}
	}
	return result
}

func baseScore(value domain.CandidateAssessment) float64 {
	return 0.40*value.Materiality + 0.20*value.Novelty + 0.15*value.Actionability + 0.10*value.Urgency + 0.15*value.EvidenceStrength
}
