package preference

import "testing"

func TestFitSeparatesPreferenceFromDiagnostics(t *testing.T) {
	notInterested := "not_interested"
	alreadyKnew := "already_knew"
	signals := []Signal{{Direction: "more", Facets: []string{"ai_models"}}, {Direction: "less", Reason: &notInterested, Facets: []string{"sports"}}, {Direction: "less", Reason: &alreadyKnew, Facets: []string{"ai_models"}}}
	profile := Fit(signals)
	if profile.EffectiveSignals != 2 || profile.NeutralSignals != 1 {
		t.Fatalf("unexpected counts: %+v", profile)
	}
	if profile.Weights["ai_models"] <= 0 || profile.Weights["sports"] >= 0 {
		t.Fatalf("unexpected weights: %+v", profile.Weights)
	}
	if profile.SuppressionReady {
		t.Fatal("signal counts alone must never enable suppression")
	}
}

func TestCalibrationMoreSignalHasExplicitBoundedWeight(t *testing.T) {
	routine := Fit([]Signal{{Direction: "more", Facets: []string{"finance"}}})
	calibration := Fit([]Signal{{Direction: "more", Facets: []string{"finance"}, Origin: "calibration"}})
	if calibration.Weights["finance"] <= routine.Weights["finance"] || calibration.Weights["finance"] > 1 {
		t.Fatalf("routine=%v calibration=%v", routine.Weights["finance"], calibration.Weights["finance"])
	}
}
