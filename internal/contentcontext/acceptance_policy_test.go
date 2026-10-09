package contentcontext

import (
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

// These held-out pairs exercise subject ownership and source boundaries rather
// than merely asking whether the matcher can count two overlapping words.
func TestAutomaticSubjectOwnershipAndPhraseBoundaries(t *testing.T) {
	engine := NewEngine()
	cases := []struct {
		name, title, summary, candidate, candidateSummary string
		tags                                              []string
		want                                              bool
	}{
		{name: "different product same feature", title: "Codex coding performance details", candidate: "OpenUI coding performance details", tags: []string{"Codex", "coding performance"}},
		{name: "same product same feature", title: "Codex coding performance details", candidate: "Codex coding performance observations", tags: []string{"Codex", "coding performance"}, want: true},
		{name: "product alone different feature", title: "Codex enterprise licensing changes", candidate: "Codex reset schedule", tags: []string{"Codex", "enterprise licensing"}},
		{name: "real technical phrase", title: "Quantum systems measurement", candidate: "Quantum systems research notes", want: true},
		{name: "no artificial phrase across removed stopword", title: "Quantum and systems measurement", candidate: "Quantum systems research notes", tags: []string{"quantum systems"}},
		{name: "no artificial phrase across fields", title: "Quantum measurement", summary: "Systems observations", candidate: "Quantum systems research notes", tags: []string{"quantum systems"}},
		{name: "candidate phrase also requires adjacency", title: "Quantum systems measurement", candidate: "Quantum and systems research notes", tags: []string{"quantum systems"}},
		{name: "tag-only subject cannot corroborate itself", title: "Photo commentary", candidate: "Quantum systems research notes", tags: []string{"quantum", "systems", "quantum systems"}},
		{name: "identity is independent of feature capitalization", title: "GPT-6.1 Coding limits", candidate: "GPT-6.1 coding details", want: true},
	}
	for _, example := range cases {
		t.Run(example.name, func(t *testing.T) {
			query := engine.Extract(domain.TimelineItem{Item: domain.ReasonedItem{WhatChanged: example.title, WhyItMatters: example.summary},
				Assessment: domain.CandidateAssessment{TopicTags: example.tags}})
			matches := engine.Match(query, []Candidate{{Item: testMemory("candidate", example.candidate, example.candidateSummary)}}, 3)
			if (len(matches) == 1) != example.want {
				t.Fatalf("admission=%v want=%v query=%+v matches=%+v", len(matches) == 1, example.want, query, matches)
			}
		})
	}
}

func TestBroadLibraryIntentStillAllowsAuthorAndCompanySearch(t *testing.T) {
	engine := NewEngine()
	item := testMemory("library", "Different subject", "AI and OpenAI context")
	item.Author = "Andrew Curran"
	for _, text := range []string{"AI", "OpenAI", "Andrew"} {
		if matches := engine.Match(engine.ExtractSearch(text), []Candidate{{Item: item}}, 3); len(matches) != 1 {
			t.Fatalf("explicit Library query %q narrowed: %+v", text, matches)
		}
	}
}
