package selection

import (
	"fmt"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/preference"
)

func TestSelectionIsFinite(t *testing.T) {
	var values []domain.CandidateAssessment
	for i := 0; i < 12; i++ {
		values = append(values, domain.CandidateAssessment{EvidenceKey: fmt.Sprintf("x:%024x", i), TopicFacets: []string{"ai_models"}, Materiality: .6, EvidenceStrength: .7, Novelty: float64(i) / 20})
	}
	result := Select(values, preference.Profile{Weights: map[string]float64{"ai_models": 1}, EffectiveSignals: 12, PositiveSignals: 12, PromotionReady: true}, 5, "promote_unused_budget")
	selected := 0
	for _, value := range result {
		if value.Selected {
			selected++
		}
	}
	if selected != 5 {
		t.Fatalf("selected %d, want 5", selected)
	}
}

func TestUnusedBudgetPromotionRequiresReadiness(t *testing.T) {
	value := domain.CandidateAssessment{EvidenceKey: "x:000000000000000000000001", TopicFacets: []string{"ai_models"}, Materiality: .2, EvidenceStrength: .9, Novelty: 1, Actionability: 1, Urgency: 1}
	without := Select([]domain.CandidateAssessment{value}, preference.Profile{Weights: map[string]float64{}}, 1, "promote_unused_budget")
	if without[0].Selected {
		t.Fatal("unready profile promoted candidate")
	}
	with := Select([]domain.CandidateAssessment{value}, preference.Profile{Weights: map[string]float64{"ai_models": 1}, PromotionReady: true}, 1, "promote_unused_budget")
	if !with[0].Selected {
		t.Fatal("ready profile did not fill unused budget")
	}
}
