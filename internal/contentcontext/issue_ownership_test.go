package contentcontext

import (
	"strings"
	"testing"
)

// Paraphrases retain the live failure's subject/issue structure without storing
// private Library text. Only the third record actually concerns Codex resets.
func TestRelatedConcreteIssueRejectsLiveProseMatches(t *testing.T) {
	engine := NewEngine()
	query := engine.Extract(relatedTimeline("Codex can now use a reset on its own when explicitly requested with 10% remaining", "", "", "", "Codex"))
	factory := testMemory("factory", "Codex coding in an internal software factory", "")
	text := "Codex writes code. A QA engineer can use the app when a regression appears."
	factory.FullContent = &text
	result := engine.Match(query, []Candidate{
		{Item: factory, BM25: -100},
		{Item: testMemory("dots", "Codex limits can use Dots for extended usage", ""), BM25: -90},
		{Item: testMemory("reset", "Codex resets are suspended for a month", ""), BM25: 100},
		{Item: testMemory("sync", "Context synchronization when moving between Codex and ChatGPT", ""), BM25: -80},
		{Item: testMemory("cua", "Dots use cases for controlling Codex through CUA", ""), BM25: -70},
	}, 5)
	if len(result) != 1 || result[0].Item.ID != "reset" || !strings.Contains(result[0].MatchReason, "reset") {
		t.Fatalf("ordinary prose admitted or outranked the issue: %+v", result)
	}
}

func TestRelatedOrdinaryProseNeverBecomesAnIssue(t *testing.T) {
	engine := NewEngine()
	for _, prose := range []string{"can use when", "think consider explore", "enables permits supports", "regularly carefully effectively", "facilitates possibilities opportunities"} {
		query := engine.Extract(relatedTimeline("Codex licensing "+prose, "", "", "", "Codex"))
		candidate := testMemory("different", "Codex resets "+prose, "")
		if got := engine.Match(query, []Candidate{{Item: candidate}}, 3); len(got) != 0 {
			t.Fatalf("unlisted ordinary prose %q opened admission: %+v", prose, got)
		}
	}
	query := engine.Extract(relatedTimeline("Codex reset can use when think consider explore", "", "", "", "Codex"))
	plain := testMemory("plain", "Codex reset", "")
	verbose := testMemory("verbose", "Codex reset can use when think consider explore", "")
	left, right := scoreRelatedCandidate(query, plain), scoreRelatedCandidate(query, verbose)
	if !left.admitted || !right.admitted || left.strength != right.strength || left.reason != right.reason {
		t.Fatalf("prose changed concrete evidence or rank: plain=%+v verbose=%+v", left, right)
	}
}

func TestRelatedIssueOwnershipAcrossClausesAndModels(t *testing.T) {
	engine := NewEngine()
	cases := []struct {
		name, query, candidate string
		want                   bool
	}{
		{"another sentence", "Codex resets", "Codex licensing. ChatGPT resets", false},
		{"another product same sentence", "Codex resets", "Codex licensing and ChatGPT resets", false},
		{"unknown product boundary", "Codex resets", "Codex licensing, Atlas resets", false},
		{"lowercase product boundary", "Codex resets", "Codex licensing while chatgpt resets", false},
		{"lowercase product conjunction", "Codex resets", "Codex licensing and chatgpt resets", false},
		{"company boundary", "Codex pricing", "Codex licensing while OpenAI pricing changes", false},
		{"lowercase company boundary", "Codex pricing", "Codex licensing and openai pricing changes", false},
		{"issue belongs to later product", "Codex resets", "Codex notes on resets for ChatGPT", false},
		{"query cannot borrow either", "Codex licensing. ChatGPT resets", "Codex resets", false},
		{"explicit coordinated owners", "ChatGPT and Codex reset schedule", "Codex resets", true},
		{"conjunction cannot erase sentence boundary", "Codex resets", "Codex. And ChatGPT resets", false},
		{"pronoun continuation", "Codex licensing. Its resets are suspended", "Codex reset schedule", true},
		{"new owner changes pronoun", "Codex licensing. ChatGPT settings. Its resets are suspended", "Codex reset schedule", false},
		{"unattributed next sentence", "Codex licensing. Resets are suspended", "Codex resets", false},
		{"model issue before owner", "Coding improvements for GPT-6.1", "GPT-6.1 coding notes", true},
		{"incidental model", "GPT-6.1 licensing", "GPT-6.1 overview. Codex licensing", false},
		{"incidental model same clause", "GPT-6.1 licensing", "GPT-6.1 notes and Codex licensing", false},
		{"model must own exact feature", "GPT-6.1 licensing and GPT-6.2 coding", "GPT-6.1 coding", false},
		{"model pronoun", "GPT-6.1 notes. Its coding latency improved", "GPT-6.1 coding latency", true},
		{"different atomic version", "GPT-6.1 coding", "GPT-6.2 coding", false},
		{"same licensing", "Codex enterprise licensing", "Codex licensing details", true},
		{"same citation", "RAG citation grounding", "RAG citation verification", true},
		{"same error", "Opus 5.5 Arena error rate", "Opus 5.5 error analysis", true},
		{"same context window", "GPT-6.1 context window limits", "GPT-6.1 context window notes", true},
		{"same tool calling", "Step 5 Preview tool calling", "Step5Preview tool calling notes", true},
		{"calling alone is not tool calling", "Step 5 Preview tool calling", "Step5Preview calling someone", false},
		{"window alone is not context window", "GPT-6.1 context window", "GPT-6.1 browser window", false},
		{"performance alone", "Opus 5.5 performance", "Opus 5.5 performance", false},
	}
	for _, example := range cases {
		t.Run(example.name, func(t *testing.T) {
			query := engine.Extract(relatedTimeline(example.query, "", "", "", "Codex", "ChatGPT", "RAG"))
			got := engine.Match(query, []Candidate{{Item: testMemory("candidate", example.candidate, "")}}, 3)
			if (len(got) > 0) != example.want {
				t.Fatalf("want=%v got=%+v query=%+v", example.want, got, query)
			}
		})
	}
}

func TestRelatedSummaryCannotCreateIssueOwnership(t *testing.T) {
	engine := NewEngine()
	cases := []struct {
		name, title, summary, candidate, candidateSummary string
		want                                              bool
	}{
		{"ordinary query summary", "Codex licensing", "Codex resets", "Codex resets", "", false},
		{"ordinary candidate summary", "Codex resets", "", "Codex licensing", "Codex resets", false},
		{"summary competitor cannot own feature", "GoPro footage", "OpenUI video stabilization", "OpenUI video stabilization", "", false},
		{"ambiguous summary cannot pick product", "GoPro and OpenUI footage", "Video stabilization", "GoPro video stabilization", "", false},
		{"boundary-only owner blocks ordinary summary", "Codex context window", "", "Codex licensing while chatgpt adds context windows", "Context window improvements", false},
		{"boundary-only owner blocks technical summary", "GoPro video stabilization", "", "GoPro licensing while chatgpt handles footage", "Video stabilization improvements", false},
		{"technical summary sole owner", "GoPro footage", "Video stabilization", "GoPro recordings", "Video stabilization", true},
		{"general technical phrase", "Quantum systems measurement", "", "Quantum systems notes", "", true},
	}
	for _, example := range cases {
		t.Run(example.name, func(t *testing.T) {
			query := engine.Extract(relatedTimeline(example.title, example.summary, "", "", "Codex"))
			got := engine.Match(query, []Candidate{{Item: testMemory("candidate", example.candidate, example.candidateSummary)}}, 3)
			if (len(got) > 0) != example.want {
				t.Fatalf("want=%v got=%+v", example.want, got)
			}
		})
	}
}

func TestRelatedIssueProofRetainsFieldAndLocalSpan(t *testing.T) {
	field := "Codex licensing. ChatGPT resets. Its reset schedule changed."
	proofs := relatedIssueEvidence([]memoryField{{label: "retained text", value: field}}, []string{"codex", "chatgpt"}, nil, nil)
	if len(proofs) != 3 {
		t.Fatalf("proofs=%+v", proofs)
	}
	for index, proof := range proofs {
		if proof.field != "retained text" || concreteIssue(string([]rune(field)[proof.start:proof.end])) != proof.issue {
			t.Fatalf("issue span does not support proof: %+v", proof)
		}
		wantOwner := "chatgpt"
		if index == 0 {
			wantOwner = "codex"
		}
		if proof.owner != wantOwner || proof.continuation != (index == 2) {
			t.Fatalf("borrowed owner: %+v", proof)
		}
		if !proof.continuation && strings.ToLower(string([]rune(field)[proof.ownerStart:proof.ownerEnd])) != proof.owner {
			t.Fatalf("owner span does not support proof: %+v", proof)
		}
	}
}

func TestRelatedIssueEvidenceIsBoundedAndUnknownIssuesFailClosed(t *testing.T) {
	proofs := relatedIssueEvidence([]memoryField{{label: "title", value: strings.Repeat("Codex reset. ", 1000)}}, []string{"codex"}, nil, nil)
	if len(proofs) != 64 {
		t.Fatalf("unexpected bounded proof count: %d", len(proofs))
	}
	engine := NewEngine()
	query := engine.Extract(relatedTimeline("Codex unrecognizedfeature", "", "", "", "Codex"))
	if got := engine.Match(query, []Candidate{{Item: testMemory("unknown", "Codex unrecognizedfeature", "")}}, 3); len(got) != 0 {
		t.Fatalf("unknown issue was guessed: %+v", got)
	}
	if got := engine.Match(engine.ExtractSearch("Codex unrecognizedfeature"), []Candidate{{Item: testMemory("explicit", "Codex unrecognizedfeature", "")}}, 3); len(got) != 1 {
		t.Fatal("automatic issue vocabulary narrowed explicit Library search")
	}
}
