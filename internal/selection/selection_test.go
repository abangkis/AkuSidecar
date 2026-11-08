package selection

import (
	"fmt"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/preference"
)

func TestColdStartPreservesSourceOrderAndFiniteBudget(t *testing.T) {
	var values []domain.CandidateAssessment
	for i := 0; i < 12; i++ {
		values = append(values, assessment(fmt.Sprintf("x:%024x", i), "other", .65, .7))
	}
	result := Select(values, preference.Profile{Weights: map[string]float64{}}, 5, "guarded_live")
	selected := selectedKeys(result)
	if len(selected) != 5 || selected[0] != values[0].EvidenceKey || selected[4] != values[4].EvidenceKey {
		t.Fatalf("selected=%v", selected)
	}
}

func TestGuardedLiveCanPromoteReplaceAndSuppressOrdinaryCandidates(t *testing.T) {
	profile := preference.Profile{
		Weights:        map[string]float64{"tag:ai_models": 1, "tag:sports": -1},
		AuthorityReady: true, PromotionReady: true, SuppressionReady: true,
	}
	values := []domain.CandidateAssessment{
		assessment("x:000000000000000000000001", "sports", .65, .8),
		assessment("x:000000000000000000000002", "other", .62, .8),
		assessment("x:000000000000000000000003", "ai_models", .20, .8),
	}
	result := Select(values, profile, 2, "guarded_live")
	selected := selectedKeys(result)
	if contains(selected, values[0].EvidenceKey) || !contains(selected, values[1].EvidenceKey) || !contains(selected, values[2].EvidenceKey) {
		t.Fatalf("selected=%v", selected)
	}
	assertSelectionReasons(t, result, map[string]domain.SelectionReason{
		values[0].EvidenceKey: domain.SelectionPreferenceSuppressed,
		values[1].EvidenceKey: domain.SelectionBaseline,
		values[2].EvidenceKey: domain.SelectionPreferencePromotion,
	})
}

func TestProtectedMaterialUpdateCannotBeSuppressed(t *testing.T) {
	profile := preference.Profile{
		Weights:        map[string]float64{"tag:sports": -1},
		AuthorityReady: true, SuppressionReady: true,
	}
	protected := assessment("x:000000000000000000000010", "sports", .9, .8)
	ordinary := assessment("x:000000000000000000000011", "other", .65, .8)
	selected := selectedKeys(Select([]domain.CandidateAssessment{ordinary, protected}, profile, 1, "guarded_live"))
	if len(selected) != 1 || selected[0] != protected.EvidenceKey {
		t.Fatalf("selected=%v", selected)
	}
}

func TestGuardedLiveKeepsOneDiscoveryLane(t *testing.T) {
	profile := preference.Profile{
		Weights:        map[string]float64{"tag:ai_models": 1},
		AuthorityReady: true, PromotionReady: true,
	}
	values := []domain.CandidateAssessment{
		assessment("x:000000000000000000000020", "ai_models", .7, .8),
		assessment("x:000000000000000000000021", "ai_models", .68, .8),
		assessment("x:000000000000000000000022", "other", .6, .8),
	}
	result := Select(values, profile, 2, "guarded_live")
	selected := selectedKeys(result)
	if !contains(selected, values[2].EvidenceKey) {
		t.Fatalf("discovery candidate missing: %v", selected)
	}
	assertSelectionReasons(t, result, map[string]domain.SelectionReason{
		values[0].EvidenceKey: domain.SelectionBaseline,
		values[1].EvidenceKey: domain.SelectionDisplacedByDiscovery,
		values[2].EvidenceKey: domain.SelectionDiscovery,
	})
}

func TestPreviouslyDeliveredEvidenceIsAlwaysExcluded(t *testing.T) {
	value := assessment("x:000000000000000000000030", "ai_models", .9, .9)
	profile := preference.Profile{Weights: map[string]float64{"tag:ai_models": 1}, AuthorityReady: true, PromotionReady: true}
	result := SelectWithOptions([]domain.CandidateAssessment{value}, profile, Options{
		Limit: 1, Mode: "guarded_live", ExcludedEvidence: map[string]bool{value.EvidenceKey: true},
	})
	if len(selectedKeys(result)) != 0 {
		t.Fatalf("excluded evidence selected: %+v", result)
	}
}

func TestZeroAdditionsIsValidWhenNothingClearsAdmission(t *testing.T) {
	value := assessment("x:000000000000000000000040", "other", .1, .2)
	if selected := selectedKeys(Select([]domain.CandidateAssessment{value}, preference.Profile{}, 5, "guarded_live")); len(selected) != 0 {
		t.Fatalf("selected=%v", selected)
	}
}

func TestSelectionDistinguishesGuardsFromBudget(t *testing.T) {
	values := []domain.CandidateAssessment{
		assessment("baseline", "other", .7, .8),
		assessment("budget", "other", .65, .8),
		assessment("delivered", "other", .9, .9),
		assessment("weak", "other", .9, .2),
		assessment("below", "other", .05, .4),
	}
	result := SelectWithOptions(values, preference.Profile{}, Options{
		Limit: 1, Mode: "rank_only", ExcludedEvidence: map[string]bool{"delivered": true},
	})
	assertSelectionReasons(t, result, map[string]domain.SelectionReason{
		"baseline": domain.SelectionBaseline, "budget": domain.SelectionBudgetExhausted,
		"delivered": domain.SelectionPreviouslyDelivered, "weak": domain.SelectionInsufficientEvidence,
		"below": domain.SelectionBelowAdmission,
	})
}

func TestProtectedReplacementKeepsEarlierCandidateOnScoreTie(t *testing.T) {
	values := []domain.CandidateAssessment{
		assessment("earlier", "other", .7, .8),
		assessment("later", "other", .7, .8),
		assessment("update", "other", .2, .8),
	}
	result := SelectWithOptions(values, preference.Profile{}, Options{
		Limit: 2, Mode: "guarded_live", ProtectedEvidence: map[string]bool{"update": true},
	})
	if selected := selectedKeys(result); len(selected) != 2 || selected[0] != "earlier" || selected[1] != "update" {
		t.Fatalf("selected=%v, want earlier then update", selected)
	}
	assertSelectionReasons(t, result, map[string]domain.SelectionReason{
		"earlier": domain.SelectionBaseline, "later": domain.SelectionDisplacedByProtection, "update": domain.SelectionProtected,
	})
}

func assertSelectionReasons(t *testing.T, values []domain.ScoredAssessment, want map[string]domain.SelectionReason) {
	t.Helper()
	if len(values) != len(want) {
		t.Fatalf("decisions=%d, want %d", len(values), len(want))
	}
	for _, value := range values {
		key := value.Assessment.EvidenceKey
		if decision := value.Decision; decision == nil || decision.Reason != want[key] || decision.PolicyVersion == "" || decision.Selected != value.Selected {
			t.Errorf("%s decision=%+v, want reason %q and selected=%v", key, decision, want[key], value.Selected)
		}
	}
}

func assessment(key, facet string, materiality, evidence float64) domain.CandidateAssessment {
	return domain.CandidateAssessment{
		EvidenceKey: key, TopicTags: []string{facet}, TopicFacets: []string{facet}, Materiality: materiality,
		Novelty: .5, Actionability: .4, Urgency: .3, EvidenceStrength: evidence,
	}
}

func selectedKeys(values []domain.ScoredAssessment) []string {
	var result []string
	for _, value := range values {
		if value.Selected {
			result = append(result, value.Assessment.EvidenceKey)
		}
	}
	return result
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
