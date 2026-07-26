package store

import "testing"

func TestContinuityEngagementScoreParsesRenderedSocialCounts(t *testing.T) {
	value := map[string]any{
		"like":    "1.2K",
		"comment": "1,234",
		"share":   "2,5K",
		"nested":  map[string]any{"repost": float64(16)},
	}
	if got, want := continuityEngagementScore(value), int64(4_950); got != want {
		t.Fatalf("continuity engagement score=%d want=%d", got, want)
	}
}

func TestMaterialEngagementChangeUsesNormalizedCounts(t *testing.T) {
	if !materialEngagementChange(
		continuityEngagementScore(map[string]any{"like": "1.0K"}),
		continuityEngagementScore(map[string]any{"like": "1.3K"}),
	) {
		t.Fatal("a 30 percent rendered engagement increase should be material")
	}
	if materialEngagementChange(
		continuityEngagementScore(map[string]any{"like": "1.0K"}),
		continuityEngagementScore(map[string]any{"like": "1.1K"}),
	) {
		t.Fatal("a 10 percent rendered engagement increase should remain below the gate")
	}
}
