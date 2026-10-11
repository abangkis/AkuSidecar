package contentcontext

import (
	"strings"
	"testing"
)

func TestAutomaticFeatureRequiresPrimaryTextCorroboration(t *testing.T) {
	engine := NewEngine()
	cases := []struct {
		name, title, summary, candidate, candidateSummary string
		want                                              bool
	}{
		{name: "query significance is not a feature", title: "Codex Composer Predictions with goal execution", summary: "Potential coding workflows", candidate: "Codex coding review latency", candidateSummary: "Potential developer workflows"},
		{name: "candidate summary alone is not a feature", title: "Codex reset schedule", candidate: "Codex plugin installation", candidateSummary: "A reset perspective"},
		{name: "summary product cannot own another product feature", title: "Codex reset schedule", candidate: "OpenUI reset schedule", candidateSummary: "Also mentions Codex"},
		{name: "country identity adjective is not an extra issue", title: "Indonesian girl group music achievements", summary: "Artists from Indonesia", candidate: "Indonesian weightlifter wins gold", candidateSummary: "An international milestone for Indonesia"},
		{name: "time words are not product features", title: "ChatGPT reset by end of day", candidate: "ChatGPT mobile app on day five", candidateSummary: "A day end update"},
		{name: "now is not a feature", title: "ChatGPT reset now", candidate: "ChatGPT mobile app now"},
		{name: "pronouns are not features", title: "Codex Predictions combining them with goals", candidate: "Codex coding reviews for them"},
		{name: "RAG requires more than human and long", title: "RAG long retrieval queries reviewed by human readers", candidate: "RAG long screenshots selected by human readers"},
		{name: "performance alone is too broad", title: "Opus 5.5 performance and error rate formula", candidate: "Opus 5.5 performance and pricing tiers", candidateSummary: "Competitive performance"},
		{name: "same concrete error analysis", title: "Opus 5.5 error rate formula", candidate: "Opus 5.5 error analysis", want: true},
		{name: "same licensing issue", title: "Codex enterprise licensing", candidate: "Codex licensing details", want: true},
		{name: "same RAG citation issue", title: "RAG citation grounding", candidate: "RAG citation verification", want: true},
		{name: "prefer owning subject over incidental product", title: "ChatGPT and Codex reset schedule", candidate: "Codex resets", candidateSummary: "Also mentions ChatGPT", want: true},
		{name: "technical summary phrase remains supported", title: "GoPro footage", summary: "Video stabilization", candidate: "GoPro recordings", candidateSummary: "Video stabilization", want: true},
	}
	for _, example := range cases {
		t.Run(example.name, func(t *testing.T) {
			query := engine.Extract(relatedTimeline(example.title, example.summary, "", "", "Codex", "ChatGPT", "Indonesia", "GoPro", "RAG"))
			matches := engine.Match(query, []Candidate{{Item: testMemory("candidate", example.candidate, example.candidateSummary)}}, 3)
			if (len(matches) == 1) != example.want {
				t.Fatalf("matches=%+v want=%v query=%+v", matches, example.want, query)
			}
			if len(matches) > 0 && strings.Contains(matches[0].MatchReason, `feature ""`) {
				t.Fatalf("empty feature reason: %s", matches[0].MatchReason)
			}
		})
	}
}

func TestPrimaryIssueKeepsBoundedRetrievalBudget(t *testing.T) {
	query := NewEngine().Extract(relatedTimeline("Codex reset schedule", "Potential evolving audience sentiment regarding recurring events and changing perspectives across creative cultural landscapes", "", "", "Codex"))
	if !containsString(query.Terms, "reset") || containsString(query.Terms, "evolving") {
		t.Fatalf("significance prose crowded primary features: %+v", query)
	}
}
