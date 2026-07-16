package preference

import (
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestFitSeparatesPreferenceFromDiagnostics(t *testing.T) {
	notInterested := "not_interested"
	alreadyKnew := "already_knew"
	signals := []Signal{
		{Direction: "more", Facets: []string{"ai_models"}},
		{Direction: "less", Reason: &notInterested, Facets: []string{"sports"}},
		{Direction: "less", Reason: &notInterested, Facets: []string{"sports"}},
		{Direction: "less", Reason: &alreadyKnew, Facets: []string{"ai_models"}},
	}
	profile := Fit(signals)
	if profile.EffectiveSignals != 3 || profile.NeutralSignals != 1 {
		t.Fatalf("unexpected counts: %+v", profile)
	}
	if profile.Weights["ai_models"] <= 0 || profile.Weights["sports"] >= 0 {
		t.Fatalf("unexpected weights: %+v", profile.Weights)
	}
	if !profile.SuppressionReady || !profile.AuthorityReady {
		t.Fatal("repeated direct negative feedback must enable suppression authority")
	}
}

func TestCalibrationMoreSignalHasExplicitBoundedWeight(t *testing.T) {
	routine := Fit([]Signal{{Direction: "more", Facets: []string{"finance"}}})
	calibration := Fit([]Signal{{Direction: "more", Facets: []string{"finance"}, Origin: "calibration"}})
	if calibration.Weights["finance"] <= routine.Weights["finance"] || calibration.Weights["finance"] > 1 {
		t.Fatalf("routine=%v calibration=%v", routine.Weights["finance"], calibration.Weights["finance"])
	}
}

func TestHighAuthorityScoreCanMeaningfullyChangeOrdering(t *testing.T) {
	profile := Fit([]Signal{
		{Direction: "more", Facets: []string{"ai_models"}},
		{Direction: "more", Facets: []string{"ai_models"}, Origin: "calibration"},
		{Direction: "less", Facets: []string{"sports"}, Origin: "calibration"},
	})
	if !profile.PromotionReady || !profile.AuthorityReady {
		t.Fatalf("profile not ready: %+v", profile)
	}
	positive := Score(profile, candidate("ai_models"))
	negative := Score(profile, candidate("sports"))
	if positive < 0.25 || negative > -0.10 {
		t.Fatalf("positive=%v negative=%v profile=%+v", positive, negative, profile)
	}
}

func candidate(facet string) domain.CandidateAssessment {
	return domain.CandidateAssessment{TopicFacets: []string{facet}}
}
