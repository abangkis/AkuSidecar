package domain

import (
	"encoding/json"
	"fmt"
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

func TestCommentCaptureDiagnosticsStrictContract(t *testing.T) {
	var block Block
	raw := `{"directContext":[{"kind":"feed_comment","actorUrl":"https://www.linkedin.com/in/synthetic","matchBasis":"actor_profile","omittedCommentCount":2,"commentReadErrorCount":1,"provenance":"observed_dom","target":{"kind":"comment","id":"12345","availability":"reference_only","captureReason":"comment_body_ambiguous"}}]}`
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&block); err != nil {
		t.Fatal(err)
	}
	valid := block.DirectContext[0]
	if err := ValidateDirectContext(SourceLinkedIn, []DirectContext{valid}); err != nil {
		t.Fatal(err)
	}
	for _, reason := range []string{"actor_identity_unresolved", "comments_not_observed", "comments_not_visible", "matching_comments_not_observed", "comment_author_unresolved", "comment_body_not_observed", "comment_body_ambiguous", "comment_identity_ambiguous", "comment_read_failed"} {
		candidate := valid
		candidate.Target.CaptureReason = reason
		if err := ValidateDirectContext(SourceLinkedIn, []DirectContext{candidate}); err != nil {
			t.Fatal(reason, err)
		}
	}
	for _, mutate := range []func(*DirectContext){
		func(v *DirectContext) { v.MatchBasis = "banner_trigger" },
		func(v *DirectContext) { v.ActorURL = "" },
		func(v *DirectContext) { v.OmittedCommentCount = -1 },
		func(v *DirectContext) { v.CommentReadErrorCount = -1 },
		func(v *DirectContext) { v.CommentReadErrorCount = 1000001 },
		func(v *DirectContext) { v.MatchBasis = "" },
		func(v *DirectContext) { v.Target.CaptureReason = "not_loaded_guess" },
		func(v *DirectContext) { v.Target.Availability = "captured"; v.Target.Text = "Body" },
	} {
		candidate := valid
		mutate(&candidate)
		if ValidateDirectContext(SourceLinkedIn, []DirectContext{candidate}) == nil {
			t.Fatalf("invalid diagnostics accepted: %+v", candidate)
		}
	}
	if ValidateDirectContext(SourceX, []DirectContext{valid}) == nil {
		t.Fatal("LinkedIn diagnostics crossed sources")
	}
}

func TestCommentMergeAtCapacityPreservesRetainedBodies(t *testing.T) {
	refs := make([]DirectContext, DirectContextLimit)
	for i := range refs {
		refs[i] = DirectContext{Kind: "feed_comment", ActorURL: "https://www.linkedin.com/in/synthetic", Provenance: "observed_dom", MatchBasis: "actor_profile", Target: ContextObject{Kind: "comment", ID: fmt.Sprint(10000 + i), Availability: "reference_only", CaptureReason: "comment_body_not_observed"}}
	}
	retained := refs[0]
	retained.Target = ContextObject{Kind: "comment", ID: "99999", Text: "Valid retained comment", Availability: "captured"}
	retained.CapturedAt = "2026-10-16T00:00:00Z"
	for _, previous := range [][]DirectContext{{retained}, {retained, refs[0]}} {
		merged := MergeDirectContext(previous, refs)
		if len(merged) != DirectContextLimit {
			t.Fatalf("capacity=%d", len(merged))
		}
		found := false
		for _, relation := range merged {
			if relation.Target.ID == retained.Target.ID {
				found = relation.Target.Text == retained.Target.Text && relation.Target.CapturedAt == retained.CapturedAt
			}
		}
		if !found {
			t.Fatalf("references displaced retained body: %+v", merged)
		}
		if err := ValidateDirectContext(SourceLinkedIn, merged); err != nil {
			t.Fatal(err)
		}
	}
	// Duplicate identity is resolved before capacity and cannot consume two slots.
	retained.Target.ID = refs[7].Target.ID
	merged := MergeDirectContext([]DirectContext{retained}, refs)
	if len(merged) != DirectContextLimit || merged[7].Target.Text != retained.Target.Text {
		t.Fatalf("duplicate body lost at capacity: %+v", merged)
	}
}

func TestCommentMergeRetainsDistinctBodiesAndDiagnosticFreshness(t *testing.T) {
	first := DirectContext{Kind: "feed_comment", ActorURL: "https://www.linkedin.com/in/synthetic", MatchBasis: "actor_profile", Provenance: "observed_dom", CapturedAt: "2026-10-16T00:00:00Z", Target: ContextObject{Kind: "comment", ID: "12345", Text: "First comment", Availability: "captured"}}
	second := first
	second.Target.ID = "67890"
	second.Target.Text = "Second comment"
	merged := MergeDirectContext(nil, []DirectContext{first, second, first})
	if len(merged) != 2 || merged[0].Target.Text != first.Target.Text || merged[1].Target.Text != second.Target.Text {
		t.Fatalf("lost distinct comment: %+v", merged)
	}
	reference := first
	reference.CapturedAt = "2026-10-17T00:00:00Z"
	reference.Target.Text = ""
	reference.Target.Availability = "reference_only"
	reference.Target.CaptureReason = "comment_body_not_observed"
	merged = MergeDirectContext(merged, []DirectContext{reference})
	if len(merged) != 2 || merged[0].Target.Text != "First comment" || merged[0].Target.CaptureReason != "" || merged[0].Target.CapturedAt != first.CapturedAt {
		t.Fatalf("missing-body reason attached to retained body: %+v", merged)
	}
}
