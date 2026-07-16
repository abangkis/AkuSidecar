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
}
