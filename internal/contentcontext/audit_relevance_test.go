package contentcontext

import (
	"reflect"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func auditMemory(id, title, summary string, tags ...string) domain.MemoryItem {
	item := testMemory(id, title, summary)
	item.Tags = tags
	return item
}

func TestAutomaticRelatedPrecisionAudit(t *testing.T) {
	engine := NewEngine()
	tests := []struct {
		name           string
		query          domain.TimelineItem
		candidate      domain.MemoryItem
		wantMatch      bool
		wantReasonPart string
	}{
		{
			name: "girlgroup does not relate to sports through global milestone prose",
			query: relatedTimeline(
				"Nona girlgroup announces an international milestone for a global audience",
				"The international milestone reached a global audience",
				"", "",
			),
			candidate: auditMemory("weightlifting", "Weightlifting milestone sets a global record", "International athletes compete in the championship"),
		},
		{
			name: "second product names do not become Codex reset features",
			query: relatedTimeline(
				"Codex reset guidance also mentions ChatGPT",
				"Developers use a custom workflow across the ecosystem",
				"", "", "Codex",
			),
			candidate: auditMemory("chatgpt-mobile", "Codex mobile for ChatGPT", "Developer ecosystem workflow with custom plugins", "Codex"),
		},
		{
			name: "Composer goals do not relate to general Codex workflows",
			query: relatedTimeline(
				"Composer Predictions goal",
				"The workflow advances the Composer Predictions goal",
				"", "",
			),
			candidate: auditMemory("codex-workflow", "Codex workflow overview", "General workflows for developers"),
		},
		{
			name: "Opus version mismatch is not rescued by competitive commentary",
			query: relatedTimeline(
				"Opus 5.5 Arena error",
				"The competitive benchmark records an Arena error",
				"", "",
			),
			candidate: auditMemory("tibo-dots", "Tibo Dots arena results", "Competitive community commentary"),
		},
		{
			name: "Tibo reset is not related to model advice through community prose",
			query: relatedTimeline(
				"Tibo reset notes",
				"The community discusses the reset",
				"", "", "Tibo",
			),
			candidate: auditMemory("model-advice", "GPT-6.1 model advice", "Community discussion of reset settings"),
		},
		{
			name:      "reset matches resets for the same Codex issue",
			query:     relatedTimeline("Codex reset cadence", "Codex reset interval", "", "", "Codex"),
			candidate: auditMemory("codex-resets", "Codex resets the monthly timer", "A monthly countdown", "Codex"),
			wantMatch: true, wantReasonPart: "reset",
		},
		{
			name:      "resets matches reset for the same Codex issue",
			query:     relatedTimeline("Codex resets monthly", "Codex resets the token timer", "", "", "Codex"),
			candidate: auditMemory("codex-reset", "Codex reset interval", "A token timer", "Codex"),
			wantMatch: true, wantReasonPart: "resets",
		},
		{
			name:      "same girlgroup line distribution issue stays related",
			query:     relatedTimeline("Nona distribution issue", "Members discuss line distribution changes", "", "", "Nona"),
			candidate: auditMemory("nona-distribution", "Nona line distribution", "Members discuss distribution changes", "Nona"),
			wantMatch: true, wantReasonPart: "distribution",
		},
		{
			name:      "same Opus 5.5 benchmark error stays related",
			query:     relatedTimeline("Opus 5.5 Arena error", "The benchmark reports an Arena error", "", ""),
			candidate: auditMemory("opus-arena-error", "Opus 5.5 Arena error", "Benchmark failure details"),
			wantMatch: true, wantReasonPart: "error",
		},
		{
			name:      "GoPro technical focus phrase stays related",
			query:     testTimeline(),
			candidate: auditMemory("gopro-stabilization", "GoPro video stabilization", "Gyroscope data corrects motion metadata"),
			wantMatch: true, wantReasonPart: "video stabilization",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			query := engine.Extract(test.query)
			matches := engine.Match(query, []Candidate{{Item: test.candidate}}, 3)
			gotMatch := len(matches) == 1
			if gotMatch != test.wantMatch {
				t.Fatalf("want match=%v, got %+v (query=%+v)", test.wantMatch, matches, query)
			}
			if test.wantReasonPart != "" && !strings.Contains(strings.ToLower(matches[0].MatchReason), strings.ToLower(test.wantReasonPart)) {
				t.Fatalf("reason %q does not name expected feature %q", matches[0].MatchReason, test.wantReasonPart)
			}
			if test.wantMatch && strings.Contains(matches[0].MatchReason, "focused feature") {
				t.Fatalf("reason did not name a distinctive feature: %q", matches[0].MatchReason)
			}
		})
	}
}

func TestFeatureVariantsAreExplicitAndBounded(t *testing.T) {
	tests := []struct {
		term string
		want []string
	}{
		{term: "reset", want: []string{"reset", "resets"}},
		{term: "resets", want: []string{"resets", "reset"}},
		{term: "gpt", want: []string{"gpt"}},
		{term: "gpt-6.1", want: []string{"gpt-6.1"}},
	}
	for _, test := range tests {
		if got := FeatureVariants(test.term); !reflect.DeepEqual(got, test.want) {
			t.Errorf("FeatureVariants(%q)=%v, want %v", test.term, got, test.want)
		}
	}
}

func TestLibrarySearchKeepsBroadExplicitTermsLiteral(t *testing.T) {
	engine := NewEngine()
	query := engine.ExtractSearch("developers community competitive")
	result := engine.Match(query, []Candidate{{Item: testMemory("explicit", "Developers community competitive gaming", "A broad Library note")}}, 3)
	if len(result) != 1 {
		t.Fatalf("explicit Library search lost user-supplied terms: %+v (query=%+v)", result, query)
	}

	resetQuery := engine.ExtractSearch("resets")
	if result := engine.Match(resetQuery, []Candidate{{Item: testMemory("literal", "Codex reset schedule", "A focused Library note")}}, 3); len(result) != 0 {
		t.Fatalf("Library search applied automatic reset variants: %+v", result)
	}
}
