package contentcontext

import (
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func testTimeline() domain.TimelineItem {
	return domain.TimelineItem{
		Item: domain.ReasonedItem{
			WhatChanged:  "GoPro video stabilization with gyroscope data",
			WhyItMatters: "The footage can be corrected using motion metadata",
		},
		Assessment: domain.CandidateAssessment{
			TopicTags: []string{"gyro", "stabilization"},
		},
	}
}

func testMemory(id, title, summary string) domain.MemoryItem {
	return domain.MemoryItem{
		ID: id, Title: title, Summary: summary,
		LifecycleState: domain.MemoryStateActive,
		UpdatedAt:      "2026-08-30T00:00:00Z",
	}
}

func TestExtractIsBoundedAndIncludesStrongPhraseFeatures(t *testing.T) {
	query := NewEngine().Extract(testTimeline())
	if len(query.Terms) == 0 || len(query.Terms) > MaxQueryTerms {
		t.Fatalf("query terms=%v", query.Terms)
	}
	found := false
	for _, phrase := range query.Phrases {
		if phrase == "video stabilization" {
			found = true
		}
	}
	if !found {
		t.Fatalf("query phrases=%v", query.Phrases)
	}
}

func TestMatchRejectsGenericOnlyRecordsAndAdmitsTwoMeaningfulOverlaps(t *testing.T) {
	engine := NewEngine()
	query := engine.Extract(testTimeline())
	result := engine.Match(query, []Candidate{
		{Item: testMemory("generic", "Camera data movement", "A project update about video data")},
		{Item: testMemory("strong", "Gyroscope stabilization guide", "Correct shaky footage with motion metadata")},
	}, 5)
	if len(result) != 1 || result[0].Item.ID != "strong" {
		t.Fatalf("matches=%+v", result)
	}
	if !strings.Contains(result[0].MatchReason, "gyroscope") || !strings.Contains(result[0].MatchReason, "stabilization") {
		t.Fatalf("reason=%q", result[0].MatchReason)
	}
}

func TestOfflineScreenshotRelevanceCorpus(t *testing.T) {
	engine := NewEngine()
	query := engine.Extract(testTimeline())
	corpus := []struct {
		name  string
		item  domain.MemoryItem
		admit bool
	}{
		{name: "gyro stabilization tutorial", item: testMemory("gyro", "Gyroscope video stabilization", "Correct shaky footage with motion metadata"), admit: true},
		{name: "generic camera movement", item: testMemory("camera", "Camera movement data", "A general project update about video"), admit: false},
		{name: "generic data record", item: testMemory("data", "Camera data", "Movement and video details"), admit: false},
	}
	for _, example := range corpus {
		t.Run(example.name, func(t *testing.T) {
			result := engine.Match(query, []Candidate{{Item: example.item}}, 3)
			if (len(result) > 0) != example.admit {
				t.Fatalf("admit=%v result=%+v", example.admit, result)
			}
		})
	}
}

func TestMatchRejectsNavToorGenericCandidateThemes(t *testing.T) {
	engine := NewEngine()
	query := engine.Extract(domain.TimelineItem{
		Item: domain.ReasonedItem{
			WhatChanged:  "Every GoPro you own already knows how to fix your shaky footage",
			WhyItMatters: "Video stabilization uses gyroscope data",
		},
		Assessment: domain.CandidateAssessment{TopicTags: []string{"gopro", "stabilization"}},
	})
	for _, common := range []string{"every", "you", "your", "provides", "published", "shared", "released", "open", "source"} {
		for _, anchor := range query.Anchors {
			if anchor == common {
				t.Fatalf("common boilerplate became an anchor: %q in %v", common, query.Anchors)
			}
		}
	}

	// These mirror the weak Nav Toor/GoPro matches observed in the live UI:
	// generic overlap from S2Vec, Clearcam, and OrcaRouter must not fill the
	// drawer merely because FTS returned them.
	candidates := []Candidate{
		{Item: testMemory("s2vec", "Google Research S2Vec", "A foundation model provides every user with results you can use.")},
		{Item: testMemory("clearcam", "Clearcam, a new open source AI tool", "An open source camera project.")},
		{Item: testMemory("orcarouter", "OrcaRouter GLM weights", "Released as open source for every user.")},
	}
	if result := engine.Match(query, candidates, 5); len(result) != 0 {
		t.Fatalf("generic Nav Toor candidates admitted: %+v", result)
	}

	// A real GoPro anchor can still admit a focused record, while boilerplate
	// words remain absent from the public relationship explanation.
	result := engine.Match(query, []Candidate{{Item: testMemory("focused", "GoPro provides every user", "GoPro stabilization guide")}}, 5)
	if len(result) != 1 {
		t.Fatalf("focused anchored candidate was rejected: %+v", result)
	}
	for _, common := range []string{"every", "you", "provides", "published", "released"} {
		if strings.Contains(strings.ToLower(result[0].MatchReason), common) {
			t.Fatalf("public reason exposed boilerplate %q: %q", common, result[0].MatchReason)
		}
	}
}

func TestMatchAdmitsOneStrongPhraseOrEntityAndReturnsZeroWhenNothingQualifies(t *testing.T) {
	engine := NewEngine()
	if result := engine.Match(Query{Terms: []string{"video", "stabilization"}, Anchors: []string{"stabilization"}, Phrases: []string{"video stabilization"}}, []Candidate{
		{Item: testMemory("phrase", "Video stabilization", "A focused guide")},
	}, 3); len(result) != 1 || !strings.Contains(result[0].MatchReason, "Shared phrase") {
		t.Fatalf("phrase result=%+v", result)
	}
	if result := engine.Match(Query{Terms: []string{"gyroscope"}, Anchors: []string{"gyroscope"}}, []Candidate{
		{Item: testMemory("entity", "Gyroscope calibration", "A focused guide")},
	}, 3); len(result) != 1 {
		t.Fatalf("entity result=%+v", result)
	}
	if result := engine.Match(Query{Terms: []string{"camera", "data", "movement"}}, []Candidate{
		{Item: testMemory("generic", "Camera movement", "Data update")},
	}, 3); len(result) != 0 {
		t.Fatalf("generic result=%+v", result)
	}
}

func TestMatchIsBoundedAndUsesBM25OnlyAsTieBreaker(t *testing.T) {
	engine := NewEngine()
	query := Query{Terms: []string{"gyroscope", "stabilization"}, Anchors: []string{"gyroscope", "stabilization"}}
	result := engine.Match(query, []Candidate{
		{Item: testMemory("older", "Gyroscope stabilization", "guide"), BM25: -100},
		{Item: testMemory("newer", "Gyroscope stabilization", "guide"), BM25: 100},
	}, 1)
	if len(result) != 1 || result[0].Item.ID != "older" {
		t.Fatalf("bounded ranking=%+v", result)
	}
}

func TestMatchAppliesPairwiseFeedbackWithoutAdmittingWeakCandidates(t *testing.T) {
	engine := NewEngine()
	query := Query{Terms: []string{"gyroscope", "stabilization"}, Anchors: []string{"gyroscope", "stabilization"}}
	result := engine.Match(query, []Candidate{
		{Item: testMemory("negative", "Gyroscope stabilization", "guide"), BM25: -100, Feedback: domain.ContentContextFeedbackNotRelevant, FeedbackID: "feedback-negative"},
		{Item: testMemory("positive", "Gyroscope stabilization", "guide"), BM25: 100, Feedback: domain.ContentContextFeedbackRelevant, FeedbackID: "feedback-positive"},
		{Item: testMemory("neutral", "Gyroscope stabilization", "guide"), BM25: -50},
		{Item: testMemory("weak", "Unrelated camera", "general notes"), Feedback: domain.ContentContextFeedbackRelevant, FeedbackID: "feedback-weak"},
	}, 5)
	if len(result) != 2 || result[0].Item.ID != "positive" || result[1].Item.ID != "neutral" {
		t.Fatalf("feedback ranking=%+v", result)
	}
	if result[0].Feedback == nil || result[0].Feedback.ID != "feedback-positive" || result[0].Feedback.Verdict != domain.ContentContextFeedbackRelevant {
		t.Fatalf("feedback projection=%+v", result[0].Feedback)
	}
}

func TestTopicIdentityMatchesRejectsNarrowSiblingOnOneSharedToken(t *testing.T) {
	query := Query{Terms: []string{"codex", "chatgpt", "repository", "tunnel"}, Anchors: []string{"codex", "chatgpt"}}
	if !TopicIdentityMatches(query, "Codex", nil) {
		t.Fatal("single-token parent topic should match its substantive identity")
	}
	if TopicIdentityMatches(query, "Codex Reset", nil) {
		t.Fatal("multi-token sibling must not match when reset is absent")
	}
	if !TopicIdentityMatches(query, "Codex Usage Limits", []string{"Codex"}) {
		t.Fatal("an explicit single-token alias should preserve a broader match")
	}
	if !TopicIdentityMatches(Query{Terms: []string{"gpt", "astra", "capabilities"}}, "OpenAI GPT Astra", nil) {
		t.Fatal("two topic identity tokens should admit a focused multi-token topic")
	}
}
