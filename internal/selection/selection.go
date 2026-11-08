package selection

import (
	"math"
	"sort"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/preference"
)

const (
	minimumEvidence          = 0.35
	baselineAdmission        = 0.40
	preferenceTrustEvidence  = 0.50
	preferenceAdmission      = 0.25
	promotionThreshold       = 0.25
	suppressionThreshold     = -0.25
	discoveryAlignmentWindow = 0.15
	policyVersion            = "selection-v1"
)

type Options struct {
	Limit             int
	Mode              string
	ExcludedEvidence  map[string]bool
	ProtectedEvidence map[string]bool
}

type candidate struct {
	scored           domain.ScoredAssessment
	excluded         bool
	baselineEligible bool
	protected        bool
	promotable       bool
	suppressed       bool
	discovery        bool
	displaced        domain.SelectionReason
}

func Select(assessments []domain.CandidateAssessment, profile preference.Profile, limit int, mode string) []domain.ScoredAssessment {
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
func SelectWithOptions(assessments []domain.CandidateAssessment, profile preference.Profile, options Options) []domain.ScoredAssessment {
	if options.Limit < 0 {
		options.Limit = 0
	}
	candidates := make([]candidate, 0, len(assessments))
	for _, assessment := range assessments {
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
			scored: domain.ScoredAssessment{
				Assessment: assessment,
				BaseScore:  base, PreferenceScore: pref, FinalScore: base + pref,
			},
			excluded: excluded, baselineEligible: baseline, protected: protected,
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

	selected := map[int]domain.SelectionReason{}
	pick := func(predicate func(candidate) bool) {
		for _, index := range order {
			if len(selected) >= options.Limit {
				return
			}
			if selected[index] == "" && predicate(candidates[index]) {
				selected[index] = candidates[index].admissionReason()
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

	result := make([]domain.ScoredAssessment, 0, len(candidates))
	for _, index := range order {
		if reason := selected[index]; reason != "" {
			result = append(result, candidates[index].withDecision(true, reason))
		}
	}
	for index := range candidates {
		if selected[index] == "" {
			value := candidates[index]
			result = append(result, value.withDecision(false, value.rejectionReason(options.Mode)))
		}
	}
	return result
}

func (c candidate) admissionReason() domain.SelectionReason {
	if c.protected {
		return domain.SelectionProtected
	}
	if c.baselineEligible {
		return domain.SelectionBaseline
	}
	return domain.SelectionPreferencePromotion
}

func (c candidate) rejectionReason(mode string) domain.SelectionReason {
	allowsPromotion := mode == "guarded_live" || mode == "promote_unused_budget"
	switch {
	case c.displaced != "":
		return c.displaced
	case c.excluded:
		return domain.SelectionPreviouslyDelivered
	case c.scored.Assessment.EvidenceStrength < minimumEvidence:
		return domain.SelectionInsufficientEvidence
	case c.suppressed:
		return domain.SelectionPreferenceSuppressed
	case !c.baselineEligible && !(allowsPromotion && c.promotable):
		return domain.SelectionBelowAdmission
	default:
		return domain.SelectionBudgetExhausted
	}
}

func (c candidate) withDecision(selected bool, reason domain.SelectionReason) domain.ScoredAssessment {
	result := c.scored
	result.Selected = selected
	result.Decision = &domain.SelectionDecision{PolicyVersion: policyVersion, Reason: reason, Selected: selected}
	return result
}

func enforceProtected(candidates []candidate, selected map[int]domain.SelectionReason, limit int) {
	if limit == 0 {
		return
	}
	for index, value := range candidates {
		if !value.protected || selected[index] != "" {
			continue
		}
		if len(selected) < limit {
			selected[index] = domain.SelectionProtected
			continue
		}
		replace := weakestSelected(candidates, selected, func(candidate candidate) bool { return !candidate.protected })
		if replace >= 0 {
			delete(selected, replace)
			candidates[replace].displaced = domain.SelectionDisplacedByProtection
			selected[index] = domain.SelectionProtected
		}
	}
}

func enforceDiscovery(candidates []candidate, selected map[int]domain.SelectionReason, limit int) {
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
		if !value.discovery || selected[index] != "" {
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
		selected[best] = domain.SelectionDiscovery
		return
	}
	replace := weakestSelected(candidates, selected, func(candidate candidate) bool {
		return !candidate.protected && !candidate.discovery
	})
	if replace >= 0 {
		delete(selected, replace)
		candidates[replace].displaced = domain.SelectionDisplacedByDiscovery
		selected[best] = domain.SelectionDiscovery
	}
}

func weakestSelected(candidates []candidate, selected map[int]domain.SelectionReason, allowed func(candidate) bool) int {
	result := -1
	// At equal scores, keep the earlier input candidate, as the stable ranker does.
	for index := len(candidates) - 1; index >= 0; index-- {
		if selected[index] == "" || !allowed(candidates[index]) {
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
