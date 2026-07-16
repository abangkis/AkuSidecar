package preference

import (
	"math"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestFitSeparatesPreferenceFromDiagnostics(t *testing.T) {
	notInterested := "not_interested"
	signals := []Signal{
		{Direction: "more", Tags: []string{"Codex"}, Facets: []string{"ai_models"}},
		{Direction: "less", Reason: &notInterested, Tags: []string{"football"}, Facets: []string{"sports"}},
		{Direction: "less", Reason: &notInterested, Tags: []string{"football"}, Facets: []string{"sports"}},
	}
	profile := Fit(signals)
	if profile.EffectiveSignals != 3 || profile.NeutralSignals != 0 {
		t.Fatalf("unexpected counts: %+v", profile)
	}
	if profile.Weights["facet:ai_models"] <= 0 || profile.Weights["facet:sports"] >= 0 || profile.Weights["tag:football"] >= 0 {
		t.Fatalf("unexpected weights: %+v", profile.Weights)
	}
	if !profile.SuppressionReady || !profile.AuthorityReady {
		t.Fatal("repeated direct negative feedback must enable suppression authority")
	}
}

func TestCalibrationMoreSignalHasExplicitBoundedWeight(t *testing.T) {
	routine := Fit([]Signal{{Direction: "more", Tags: []string{"interest rates"}, Facets: []string{"finance"}}})
	calibration := Fit([]Signal{{Direction: "more", Tags: []string{"interest rates"}, Facets: []string{"finance"}, Origin: "calibration"}})
	if calibration.Weights["tag:interest rates"] <= routine.Weights["tag:interest rates"] || calibration.Weights["tag:interest rates"] > 1 {
		t.Fatalf("routine=%v calibration=%v", routine.Weights["tag:interest rates"], calibration.Weights["tag:interest rates"])
	}
}

func TestHighAuthorityScoreCanMeaningfullyChangeOrdering(t *testing.T) {
	profile := Fit([]Signal{
		{Direction: "more", Tags: []string{"Codex"}, Facets: []string{"ai_models"}},
		{Direction: "more", Tags: []string{"Codex"}, Facets: []string{"ai_models"}, Origin: "calibration"},
		{Direction: "less", Tags: []string{"football"}, Facets: []string{"sports"}, Origin: "calibration"},
	})
	if !profile.PromotionReady || !profile.AuthorityReady {
		t.Fatalf("profile not ready: %+v", profile)
	}
	positive := Score(profile, candidate("Codex", "ai_models"))
	negative := Score(profile, candidate("football", "sports"))
	if positive < 0.25 || negative > -0.10 {
		t.Fatalf("positive=%v negative=%v profile=%+v", positive, negative, profile)
	}
}

func TestSpecificTopicTagsOutrankBroadFacetGeneralization(t *testing.T) {
	profile := Fit([]Signal{
		{Direction: "less", Tags: []string{"Spring Data JPA"}, Facets: []string{"developer_tools"}, Origin: "calibration"},
		{Direction: "less", Tags: []string{"Spring Data JPA"}, Facets: []string{"developer_tools"}, Origin: "calibration"},
		{Direction: "more", Tags: []string{"football"}, Facets: []string{"sports"}, Origin: "calibration"},
	})
	precise := Alignment(profile, candidate("Spring Data JPA", "developer_tools"))
	broad := Alignment(profile, candidate("Codex", "developer_tools"))
	if precise > -0.25 || broad <= -0.25 || math.Abs(precise) <= math.Abs(broad) {
		t.Fatalf("precise=%v broad=%v profile=%+v", precise, broad, profile)
	}
}

func TestCleanCalibrationDoesNotTurnNarrowLessIntoCategoryDislike(t *testing.T) {
	profile := Fit([]Signal{
		{Direction: "less", Tags: []string{"senior product designer", "senior UX designer", "remote work"}, Facets: []string{"career_hiring", "developer_tools"}, Origin: "calibration"},
		{Direction: "more", Tags: []string{"football", "Messi", "Aguero"}, Facets: []string{"sports"}, Origin: "calibration"},
		{Direction: "less", Tags: []string{"Spring Data JPA", "Java", "database performance"}, Facets: []string{"software_engineering", "developer_tools"}, Origin: "calibration"},
	})
	if !profile.SuppressionReady {
		t.Fatalf("clean profile must carry negative authority: %+v", profile)
	}
	codex := domain.CandidateAssessment{TopicTags: []string{"Codex", "Claude", "rate limits"}, TopicFacets: []string{"ai_models", "developer_tools"}}
	jobPost := domain.CandidateAssessment{TopicTags: []string{"senior product designer", "remote"}, TopicFacets: []string{"career_hiring", "developer_tools"}}
	if broad := Alignment(profile, codex); broad <= -0.25 {
		t.Fatalf("unrelated Codex update inherited a narrow dislike: alignment=%v profile=%+v", broad, profile)
	}
	if precise := Alignment(profile, jobPost); precise > -0.25 {
		t.Fatalf("matching job topic lost explicit negative authority: alignment=%v profile=%+v", precise, profile)
	}
}

func candidate(tag, facet string) domain.CandidateAssessment {
	return domain.CandidateAssessment{TopicTags: []string{tag}, TopicFacets: []string{facet}}
}
