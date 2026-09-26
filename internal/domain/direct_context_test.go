package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDirectContextTypedStrictDecodeBoundsAndMerge(t *testing.T) {
	raw := `{"directContext":[{"kind":"feed_comment","actor":"Alice","actorUrl":"https://www.linkedin.com/in/alice","observedText":"Alice commented","provenance":"observed_dom","target":{"kind":"comment","availability":"reference_only"}}]}`
	var b Block
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&b); err != nil {
		t.Fatal(err)
	}
	if err := ValidateDirectContext(SourceLinkedIn, b.DirectContext); err != nil {
		t.Fatal(err)
	}
	previous := append([]DirectContext{}, b.DirectContext...)
	b.DirectContext[0].ActorURL = "https://www.linkedin.com/in/bob"
	if len(MergeDirectContext(previous, b.DirectContext)) != 2 {
		t.Fatal("distinct actors lost")
	}
	repeated := MergeDirectContext(previous, previous)
	if len(repeated) != 1 {
		t.Fatal("duplicate retained")
	}
	b.DirectContext[0].Target.Text = strings.Repeat("a", 4001)
	if ValidateDirectContext(SourceLinkedIn, b.DirectContext) == nil {
		t.Fatal("unbounded text admitted")
	}
}

func TestDirectContextMergePreservesBodyAndParentOnlyForSameActorAndTarget(t *testing.T) {
	original := DirectContext{Kind: "feed_reply", ActorURL: "https://www.linkedin.com/in/alice", CapturedAt: "2026-01-01T00:00:00Z", Target: ContextObject{Kind: "comment", ID: "urn:li:comment:56789", Text: "Captured reply", Availability: "captured"}, Parent: &ContextObject{Kind: "comment", ID: "12345", Text: "Captured parent", Availability: "captured"}}
	reference := original
	reference.CapturedAt = "2026-02-01T00:00:00Z"
	reference.Target = ContextObject{Kind: "comment", ID: "56789", Availability: "reference_only"}
	reference.Parent = nil
	merged := MergeDirectContext([]DirectContext{original}, []DirectContext{reference})
	if len(merged) != 1 || merged[0].Target.Text != "Captured reply" || merged[0].Parent.Text != "Captured parent" || merged[0].Target.CapturedAt != original.CapturedAt {
		t.Fatalf("lost captured evidence: %+v", merged)
	}
	refreshed := original
	refreshed.Target.Text = "Updated reply"
	refreshed.CapturedAt = "2026-03-01T00:00:00Z"
	merged = MergeDirectContext(merged, []DirectContext{refreshed})
	if len(merged) != 1 || merged[0].Target.Text != "Updated reply" || merged[0].Target.CapturedAt != refreshed.CapturedAt {
		t.Fatalf("did not refresh: %+v", merged)
	}
	other := reference
	other.ActorURL = "https://www.linkedin.com/in/bob"
	merged = MergeDirectContext([]DirectContext{original}, []DirectContext{other})
	if len(merged) != 2 || merged[0].Target.Text != "" {
		t.Fatal("crossed actors")
	}
	other = reference
	other.Target.ID = "99999"
	merged = MergeDirectContext([]DirectContext{original}, []DirectContext{other})
	if len(merged) != 2 || merged[0].Target.Text != "" {
		t.Fatal("crossed targets")
	}
}
func TestDirectContextXAliasesDeduplicateAndRejectConflictingIDs(t *testing.T) {
	a := DirectContext{Kind: "quotes", Provenance: "observed_dom", Target: ContextObject{Kind: "post", Permalink: "https://x.com/alice/status/12345", Text: "Original", Availability: "captured"}}
	b := a
	b.Target = ContextObject{Kind: "post", ID: "x:status:12345", Permalink: "https://x.com/i/status/12345", Availability: "reference_only"}
	merged := MergeDirectContext([]DirectContext{a}, []DirectContext{b})
	if len(merged) != 1 || merged[0].Target.Text != "Original" {
		t.Fatalf("%+v", merged)
	}
	b.Target.ID = "67890"
	if ValidateDirectContext(SourceX, []DirectContext{b}) == nil {
		t.Fatal("conflicting ID/permalink accepted")
	}
}

func TestDirectContextMergeKeepsEvidenceWhenPartialHasNoBody(t *testing.T) {
	original := DirectContext{Kind: "quotes", Target: ContextObject{Kind: "post", ID: "12345", Text: "Retained", Availability: "captured"}}
	partial := original
	partial.Target.Text = ""
	partial.Target.Availability = "partial"
	result := MergeDirectContext([]DirectContext{original}, []DirectContext{partial})
	if len(result) != 1 || result[0].Target.Text != "Retained" {
		t.Fatalf("%+v", result)
	}
}

func TestDirectContextOlderObservationCannotOverwriteNewerCapture(t *testing.T) {
	newer := DirectContext{Kind: "quotes", CapturedAt: "2026-09-26T00:00:00Z", Target: ContextObject{Kind: "post", ID: "12345", Text: "New override", Availability: "captured"}}
	older := newer
	older.CapturedAt = "2026-08-01T00:00:00Z"
	older.Target.Text = "Old observation"
	for _, candidate := range []DirectContext{older, {Kind: older.Kind, Target: older.Target}} {
		result := MergeDirectContext([]DirectContext{newer}, []DirectContext{candidate})
		if len(result) != 1 || result[0].Target.Text != "New override" || result[0].Target.CapturedAt != newer.CapturedAt {
			t.Fatalf("freshness overwritten: %+v", result)
		}
	}
}
