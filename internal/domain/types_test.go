package domain

import "testing"

func TestProfilesStayBounded(t *testing.T) {
	tests := []struct {
		name                                string
		scrolls, perSource, total, capacity int
	}{{"standard", 2, 5, 10, 12}, {"expanded", 4, 10, 20, 24}, {"stress", 6, 15, 30, 36}}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := DefaultSettings(test.name, "quiet", "promote_unused_budget", true)
			if value.MaxScrolls != test.scrolls || value.MaxItemsPerSource != test.perSource || value.MaxItemsTotal != test.total || value.TimelineCapacity != test.capacity {
				t.Fatalf("unexpected profile: %+v", value)
			}
			if err := value.Validate(); err != nil {
				t.Fatal(err)
			}
			if !value.CalibrationEnabled || value.CalibrationBatchSize != 10 {
				t.Fatalf("calibration defaults=%+v", value)
			}
		})
	}
}

func TestMissingOrUnknownProfileDefaultsToStandard(t *testing.T) {
	for _, profile := range []string{"", "unknown"} {
		value := DefaultSettings(profile, "quiet", "promote_unused_budget", true)
		if value.LoadProfile != "standard" || value.MaxScrolls != 2 || value.MaxItemsPerSource != 5 || value.MaxItemsTotal != 10 || value.TimelineCapacity != 12 {
			t.Fatalf("profile %q did not default to standard: %+v", profile, value)
		}
		if err := value.Validate(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCalibrationDecisionKeepsLabelsSeparateFromCaptureIssues(t *testing.T) {
	label := "neutral"
	if err := (CalibrationDecision{Label: &label}).Validate(); err != nil {
		t.Fatal(err)
	}
	issue := "capture_incomplete"
	if err := (CalibrationDecision{IssueCode: &issue}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (CalibrationDecision{Label: &label, IssueCode: &issue}).Validate(); err == nil {
		t.Fatal("a capture issue must not also become a preference label")
	}
}

func TestCustomProfileKeepsSupportedUIPreferences(t *testing.T) {
	value := DefaultSettings("expanded", "quiet", "promote_unused_budget", true)
	value.LoadProfile = "custom"
	value.MaxScrolls = 1
	value.MaxItemsPerSource = 3
	value.MaxItemsTotal = 6
	value.TimelineCapacity = 9
	value.QualityRetrySettleMS = 500
	value.DefaultPresentation = "brief"
	value.StreamWidth = "wide"
	value.Normalize()
	if err := value.Validate(); err != nil {
		t.Fatal(err)
	}
	if value.MaxScrolls != 1 || value.MaxItemsPerSource != 3 || value.TimelineCapacity != 9 {
		t.Fatalf("custom profile was overwritten: %+v", value)
	}
}

func TestTimelineBatchGapDefaultsAndStaysBounded(t *testing.T) {
	value := DefaultSettings("expanded", "quiet", "promote_unused_budget", true)
	if value.TimelineBatchGapPX != DefaultTimelineBatchGapPX {
		t.Fatalf("default timeline batch gap=%d", value.TimelineBatchGapPX)
	}

	value.TimelineBatchGapPX = 0
	value.Normalize()
	if value.TimelineBatchGapPX != DefaultTimelineBatchGapPX {
		t.Fatalf("normalized timeline batch gap=%d", value.TimelineBatchGapPX)
	}

	value.TimelineBatchGapPX = 52
	if err := value.Validate(); err != nil {
		t.Fatal(err)
	}
	value.TimelineBatchGapPX = 81
	if err := value.Validate(); err == nil {
		t.Fatal("out-of-range timeline batch gap must be rejected")
	}
}

func TestTimelineBoundaryCueDefaultsToFollowAndUsesLockedModes(t *testing.T) {
	value := DefaultSettings("expanded", "quiet", "promote_unused_budget", true)
	if value.TimelineBoundaryCueMode != DefaultTimelineBoundaryCueMode || value.TimelineBoundaryReturnMS != DefaultTimelineBoundaryReturnMS {
		t.Fatalf("default timeline boundary cue=%+v", value)
	}

	value.TimelineBoundaryCueMode = ""
	value.TimelineBoundaryReturnMS = 0
	value.Normalize()
	if value.TimelineBoundaryCueMode != "follow" || value.TimelineBoundaryReturnMS != 350 {
		t.Fatalf("normalized timeline boundary cue=%+v", value)
	}

	value.TimelineBoundaryCueMode = "static"
	value.TimelineBoundaryReturnMS = 650
	if err := value.Validate(); err != nil {
		t.Fatal(err)
	}
	value.TimelineBoundaryCueMode = "float"
	if err := value.Validate(); err == nil {
		t.Fatal("unsupported timeline boundary cue mode must be rejected")
	}
	value.TimelineBoundaryCueMode = "follow"
	value.TimelineBoundaryReturnMS = 1050
	if err := value.Validate(); err == nil {
		t.Fatal("out-of-range timeline boundary return duration must be rejected")
	}
}

func TestSemanticEventSettingsUseLockedChoices(t *testing.T) {
	value := DefaultSettings("expanded", "quiet", "promote_unused_budget", true)
	if value.SemanticEventMode != "collapse" || value.SemanticEventShortlist != 10 || value.SemanticEventMergeThreshold != .92 || value.KnowledgeRetentionDays != 30 || value.KnowledgeStorageLimitMB != 100 {
		t.Fatalf("semantic defaults=%+v", value)
	}

	value.SemanticEventMode = "hide"
	value.SemanticEventShortlist = 15
	value.SemanticEventMergeThreshold = .90
	value.KnowledgeRetentionDays = 90
	value.KnowledgeStorageLimitMB = 1024
	if err := value.Validate(); err != nil {
		t.Fatal(err)
	}

	value.SemanticEventShortlist = 11
	if err := value.Validate(); err == nil {
		t.Fatal("free-entry semantic shortlist must be rejected")
	}
	value.SemanticEventShortlist = 10
	for _, threshold := range []float64{.84, .96, 1, .905} {
		value.SemanticEventMergeThreshold = threshold
		if err := value.Validate(); err == nil {
			t.Fatalf("unsafe semantic merge threshold %.3f must be rejected", threshold)
		}
	}
	value.SemanticEventMergeThreshold = .95
	if err := value.Validate(); err != nil {
		t.Fatalf("upper bounded threshold rejected: %v", err)
	}
}

func TestAIDetectorPresentationDefaultsToInlineAndUsesLockedModes(t *testing.T) {
	value := DefaultSettings("standard", "quiet", "promote_unused_budget", true)
	if value.AIDetectionPresentation != "inline" {
		t.Fatalf("AI Detector default=%+v", value)
	}
	for _, mode := range []string{"inline", "drawer", "hide"} {
		value.AIDetectionPresentation = mode
		if err := value.Validate(); err != nil {
			t.Fatalf("mode %q rejected: %v", mode, err)
		}
	}
	value.AIDetectionPresentation = "remove"
	if err := value.Validate(); err == nil {
		t.Fatal("unrecoverable presentation mode must be rejected")
	}
}

func TestAIAssessmentRejectsInvalidStageStatusPair(t *testing.T) {
	value := AIAssessment{TimelineID: "timeline", SessionID: "session", Stage: "fast", Status: "strong_signals", ConfidenceBand: "medium"}
	if err := value.Validate(); err != nil {
		t.Fatal(err)
	}
	value.Stage = "user"
	if err := value.Validate(); err == nil {
		t.Fatal("user assessment must use an explicit user verdict")
	}
	value.Status = "user_marked_not_ai"
	if err := value.Validate(); err != nil {
		t.Fatal(err)
	}
	value.Stage = "deep"
	if err := value.Validate(); err == nil {
		t.Fatal("model assessment must not impersonate user authority")
	}
}

func TestFeedbackRejectsLegacyReason(t *testing.T) {
	reason := "wrong_topic"
	value := Feedback{Direction: "less", Reason: &reason}
	if err := value.Validate(); err == nil {
		t.Fatal("legacy reason must not cross the new contract")
	}
	current := "not_interested"
	value.Reason = &current
	if err := value.Validate(); err != nil {
		t.Fatal(err)
	}
	value.Reason = nil
	if err := value.Validate(); err == nil {
		t.Fatal("Less like this without Not interested must be rejected")
	}
	value.Direction = "more"
	value.Reason = &current
	if err := value.Validate(); err == nil {
		t.Fatal("More like this must not accept a reason")
	}
}
