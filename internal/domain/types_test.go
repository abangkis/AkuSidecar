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
		})
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
