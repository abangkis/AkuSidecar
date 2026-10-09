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

func relatedTimeline(whatChanged, whyItMatters, author, evidenceText string, tags ...string) domain.TimelineItem {
	item := domain.TimelineItem{
		Item:       domain.ReasonedItem{WhatChanged: whatChanged, WhyItMatters: whyItMatters, Author: author},
		Assessment: domain.CandidateAssessment{TopicTags: tags},
	}
	if evidenceText != "" || author != "" {
		item.Evidence = &domain.Block{Text: evidenceText, Author: author}
	}
	return item
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
	if len(query.Terms) == 0 || len(query.Terms)+len(query.IdentityPhrases) > MaxQueryTerms {
		t.Fatalf("query terms=%v", query.Terms)
	}
	found := false
	for _, phrase := range query.FocusPhrases {
		if phrase == "video stabilization" {
			found = true
		}
	}
	if !found {
		t.Fatalf("query focus phrases=%v", query.FocusPhrases)
	}
	for _, phrase := range query.FocusPhrases {
		if phrase == "video gyroscope" || phrase == "video metadata" || phrase == "stabilization gyroscope" {
			t.Fatalf("feature extraction invented a non-adjacent phrase: %q in %v", phrase, query.FocusPhrases)
		}
	}
}

func TestExtractSearchTreatsExplicitTermsAsBoundedAnchors(t *testing.T) {
	query := NewEngine().ExtractSearch("What do I know about Codex reset and AI?")
	if len(query.Terms) == 0 || len(query.Terms) > MaxQueryTerms {
		t.Fatalf("query terms=%v", query.Terms)
	}
	for _, expected := range []string{"codex", "reset", "ai"} {
		found := false
		for _, anchor := range query.Anchors {
			if anchor == expected {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("explicit search anchor %q missing from %v", expected, query.Anchors)
		}
	}
	if len(NewEngine().ExtractSearch("the latest update").Anchors) != 0 {
		t.Fatal("generic-only search must not produce topic knowledge anchors")
	}
}

func TestMatchRejectsGenericOnlyRecordsAndAdmitsTwoMeaningfulOverlaps(t *testing.T) {
	engine := NewEngine()
	query := engine.Extract(testTimeline())
	result := engine.Match(query, []Candidate{
		{Item: testMemory("generic", "Camera data movement", "A project update about video data")},
		{Item: testMemory("strong", "Gyroscope video stabilization guide", "Correct shaky footage with motion metadata")},
	}, 5)
	if len(result) != 1 || result[0].Item.ID != "strong" {
		t.Fatalf("matches=%+v", result)
	}
	if !strings.Contains(result[0].MatchReason, "video stabilization") {
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

func TestMatchUsesBroadExplicitSearchButStrictRelatedQueriesNeedCorroboration(t *testing.T) {
	engine := NewEngine()
	if result := engine.Match(Query{Terms: []string{"video", "stabilization"}, Anchors: []string{"stabilization"}, Phrases: []string{"video stabilization"}}, []Candidate{
		{Item: testMemory("phrase", "Video stabilization", "A focused guide")},
	}, 3); len(result) != 0 {
		t.Fatalf("zero-value automatic query admitted a single shared topic: %+v", result)
	}
	if result := engine.Match(engine.ExtractSearch("video stabilization"), []Candidate{
		{Item: testMemory("phrase", "Video stabilization", "A focused guide")},
	}, 3); len(result) != 1 || !strings.Contains(result[0].MatchReason, "Shared phrase") {
		t.Fatalf("explicit search phrase result=%+v", result)
	}
	if result := engine.Match(engine.ExtractSearch("gyroscope"), []Candidate{
		{Item: testMemory("entity", "Gyroscope calibration", "A focused guide")},
	}, 3); len(result) != 1 {
		t.Fatalf("explicit search entity result=%+v", result)
	}
	if result := engine.Match(Query{Terms: []string{"camera", "data", "movement"}}, []Candidate{
		{Item: testMemory("generic", "Camera movement", "Data update")},
	}, 3); len(result) != 0 {
		t.Fatalf("generic result=%+v", result)
	}
}

func TestAutomaticRelatedUsesSpecificContentAndRejectsAuthorAndBroadMetadataOverlap(t *testing.T) {
	engine := NewEngine()
	query := engine.Extract(relatedTimeline(
		"OpenUI introduces generative UI for local design work",
		"Generative interfaces help people prototype product flows",
		"Andrew Curran",
		"Andrew Curran describes OpenUI generative interface design.",
		"OpenUI", "generative UI",
	))
	if query.Mode != QueryRelated {
		t.Fatalf("automatic query mode=%v", query.Mode)
	}
	result := engine.Match(query, []Candidate{
		{Item: testMemory("openui", "OpenUI generative UI examples", "A product design guide")},
		{Item: testMemory("football", "Football team roster", "League standings and players")},
		{Item: testMemory("tag-only", "OpenUI examples", "A collection of unrelated illustrations")},
	}, 5)
	if len(result) != 1 || result[0].Item.ID != "openui" {
		t.Fatalf("focused OpenUI result=%+v", result)
	}

	authorQuery := engine.Extract(relatedTimeline(
		"Andrew Curran explains mathematical ideas for company Dot",
		"Matrix math and dot products are central to the note",
		"Andrew Curran",
		"Andrew Curran discusses matrix math and dot products.",
	))
	authorCandidates := []Candidate{
		{Item: func() domain.MemoryItem {
			item := testMemory("same-author-company", "Andrew Curran and Dot company", "An outlined company overview for the team")
			item.Author = "Andrew Curran"
			return item
		}()},
		{Item: testMemory("author-only", "Andrew Curran", "A short author profile")},
	}
	if result := engine.Match(authorQuery, authorCandidates, 5); len(result) != 0 {
		t.Fatalf("author/company overlap was admitted: %+v", result)
	}
	candidateAuthorQuery := engine.Extract(relatedTimeline("Andrew Curran quadratic method", "Andrew Curran quadratic method", "Reporter", "Andrew Curran quadratic method"))
	candidateAuthor := testMemory("candidate-author", "Andrew Curran quadratic", "A short note")
	candidateAuthor.Author = "Andrew Curran"
	if result := engine.Match(candidateAuthorQuery, []Candidate{{Item: candidateAuthor}}, 5); len(result) != 0 {
		t.Fatalf("candidate attribution was counted as topical evidence: %+v", result)
	}
	fallbackAuthor := relatedTimeline("Andrew Curran discusses mathematical ideas", "Matrix math and dot products", "Andrew Curran", "")
	fallbackAuthor.Evidence.Author = ""
	fallbackQuery := engine.Extract(fallbackAuthor)
	for _, term := range fallbackQuery.Terms {
		if term == "andrew" || term == "curran" {
			t.Fatalf("ReasonedItem.Author was not used as an attribution exclusion: %v", fallbackQuery.Terms)
		}
	}
	versionWithAuthorBoundary := engine.Extract(relatedTimeline("Step Andrew 5 Preview", "A note", "Andrew", ""))
	for _, identity := range versionWithAuthorBoundary.IdentityPhrases {
		if identity == "step 5 preview" {
			t.Fatalf("author suppression created a synthetic model identity across a name: %v", versionWithAuthorBoundary.IdentityPhrases)
		}
	}

	broadQuery := engine.Extract(relatedTimeline(
		"OpenAI usage quotas for an AI team",
		"The company outlined user usage and quota information",
		"",
		"OpenAI usage quotas, AI teams, and general product information.",
	))
	if len(broadQuery.Terms) != 0 {
		t.Fatalf("broad-only terms became automatic retrieval features: %v", broadQuery.Terms)
	}
	if result := engine.Match(broadQuery, []Candidate{{Item: testMemory("broad", "OpenAI usage quotas", "AI team overview outlined in general terms")}}, 5); len(result) != 0 {
		t.Fatalf("broad company/category/metadata overlap was admitted: %+v", result)
	}
	for _, company := range []string{"Google", "Microsoft", "Meta", "NVIDIA"} {
		query := engine.Extract(relatedTimeline(
			company+" announces AI usage quotas",
			company+" launches a general AI product update",
			"", "", company,
		))
		candidate := testMemory("company-"+strings.ToLower(company), company+" announces AI usage quotas", "A general company update")
		if result := engine.Match(query, []Candidate{{Item: candidate}}, 5); len(result) != 0 {
			t.Fatalf("broad company/category/reporting overlap for %s was admitted: %+v (query=%+v)", company, result, query)
		}
	}
}

func TestAutomaticRelatedRequiresCorroboratedIntactSubjectOrFocusedTechnicalPhrase(t *testing.T) {
	engine := NewEngine()
	for _, tags := range [][]string{{"Codex", "performance"}, {"Codex", "Coding Performance"}} {
		query := engine.Extract(relatedTimeline("Codex coding performance details", "Coding performance report", "", "", tags...))
		if containsString(query.SubjectFeatures, "performance") || containsString(query.SubjectFeatures, "coding performance") {
			t.Fatalf("common feature label became a subject hint for tags %v: %v", tags, query.SubjectFeatures)
		}
		other := testMemory("openui-tag-label", "OpenUI coding performance details", "Coding performance notes")
		if result := engine.Match(query, []Candidate{{Item: other}}, 5); len(result) != 0 {
			t.Fatalf("feature-label tag bypassed named product ownership for %v: %+v", tags, result)
		}
		same := testMemory("codex-tag-label", "Codex coding performance details", "Coding performance notes")
		if result := engine.Match(query, []Candidate{{Item: same}}, 5); len(result) != 1 {
			t.Fatalf("same-product match was lost with feature-label tags %v: %+v", tags, result)
		}
	}
	unconfirmedTag := engine.Extract(relatedTimeline(
		"Replit coding performance report", "A general coding performance update", "", "", "Replit agent",
	))
	if containsString(unconfirmedTag.SubjectFeatures, "replit agent") {
		t.Fatalf("multiword subject tag was accepted without exact content corroboration: %v", unconfirmedTag.SubjectFeatures)
	}
	otherProduct := testMemory("openui-performance", "OpenUI coding performance details", "Coding performance notes")
	if result := engine.Match(unconfirmedTag, []Candidate{{Item: otherProduct}}, 5); len(result) != 0 {
		t.Fatalf("generic feature phrase crossed product ownership: %+v", result)
	}

	confirmedTag := engine.Extract(relatedTimeline(
		"Replit Agent coding performance report", "Coding performance changes for Replit Agent", "", "", "Replit Agent",
	))
	if !containsString(confirmedTag.SubjectFeatures, "replit agent") {
		t.Fatalf("intact multiword subject tag was not corroborated: %v", confirmedTag.SubjectFeatures)
	}
	if result := engine.Match(confirmedTag, []Candidate{{Item: otherProduct}}, 5); len(result) != 0 {
		t.Fatalf("different named product inherited a matching generic feature phrase: %+v", result)
	}
	sameProduct := testMemory("replit-performance", "Replit Agent coding performance details", "Coding performance notes")
	if result := engine.Match(confirmedTag, []Candidate{{Item: sameProduct}}, 5); len(result) != 1 {
		t.Fatalf("corroborated same-product feature was rejected: %+v", result)
	}
	splitSubject := testMemory("split-subject", "Replit", "Agent coding performance notes")
	if result := engine.Match(confirmedTag, []Candidate{{Item: splitSubject}}, 5); len(result) != 0 {
		t.Fatalf("subject phrase was synthesized across title and summary: %+v", result)
	}

	codexCLI := engine.Extract(relatedTimeline(
		"Codex CLI reset schedule", "Codex CLI token reset schedule", "", "", "Codex CLI",
	))
	if !containsString(codexCLI.SubjectFeatures, "codex cli") {
		t.Fatalf("intact product/feature tag was not preserved: %v", codexCLI.SubjectFeatures)
	}
	if result := engine.Match(codexCLI, []Candidate{{Item: testMemory("openui-cli", "OpenUI CLI reset schedule", "CLI reset schedule details")}}, 5); len(result) != 0 {
		t.Fatalf("different product borrowed the CLI topic: %+v", result)
	}
	if result := engine.Match(codexCLI, []Candidate{{Item: testMemory("codex-cli", "Codex CLI reset schedule", "CLI reset schedule details")}}, 5); len(result) != 1 {
		t.Fatalf("corroborated Codex CLI subject was rejected: %+v", result)
	}

	caseSensitiveSource := engine.Extract(relatedTimeline(
		"Codex enterprise licensing changes", "enterprise licensing changed", "", "",
		"Codex", "licensing", "Enterprise Licensing",
	))
	if !containsString(caseSensitiveSource.SubjectFeatures, "codex") || containsString(caseSensitiveSource.SubjectFeatures, "licensing") || containsString(caseSensitiveSource.SubjectFeatures, "enterprise licensing") {
		t.Fatalf("tag capitalization overrode lowercase source casing: %v", caseSensitiveSource.SubjectFeatures)
	}
	if result := engine.Match(caseSensitiveSource, []Candidate{{Item: testMemory("openui-enterprise", "OpenUI enterprise licensing changes", "Enterprise licensing notes")}}, 5); len(result) != 0 {
		t.Fatalf("feature-tag capitalization bypassed named product ownership: %+v", result)
	}
	lowercaseName := engine.Extract(relatedTimeline(
		"codex enterprise licensing changes", "enterprise licensing changed", "", "", "Codex",
	))
	if containsString(lowercaseName.SubjectFeatures, "codex") {
		t.Fatalf("tag casing alone established a subject without source name casing: %v", lowercaseName.SubjectFeatures)
	}
	mcp := engine.Extract(relatedTimeline("MCP tooling update", "MCP server tools", "", "", "MCP"))
	if !containsString(mcp.SubjectFeatures, "mcp") {
		t.Fatalf("source-cased uppercase acronym tag was not preserved: %v", mcp.SubjectFeatures)
	}
}

func TestResearchCorroboratesSpecificQuantumSubjectButSystemsAloneStaysWeak(t *testing.T) {
	engine := NewEngine()
	query := engine.Extract(relatedTimeline(
		"Context-specific quantum systems",
		"Quantum systems research update",
		"",
		"",
		"quantum science",
	))
	result := engine.Match(query, []Candidate{{Item: testMemory("quantum", "Quantum systems research notes", "A technical research summary")}}, 3)
	if len(result) != 1 || result[0].Item.ID != "quantum" {
		t.Fatalf("focused Quantum Systems relation was rejected: %+v (terms=%v)", result, query.Terms)
	}
	if result := engine.Match(query, []Candidate{{Item: testMemory("systems", "Systems research notes", "A technical research summary")}}, 3); len(result) != 0 {
		t.Fatalf("generic systems plus research was admitted: %+v", result)
	}
	if TopicIdentitySpecificity(query, "Quantum Systems", nil) == 0 {
		t.Fatal("specific Quantum Systems topic identity should match the query")
	}
}

func TestTopicIdentitySpecificityPrefersExactQuantumIdentityOverSuffixSibling(t *testing.T) {
	query := NewEngine().Extract(relatedTimeline(
		"Context-specific quantum systems", "Local research memory", "", "", "quantum", "science",
	))
	exact := TopicIdentitySpecificity(query, "Quantum Systems", nil)
	suffixSibling := TopicIdentitySpecificity(query, "Quantum Systems 1", nil)
	if exact <= suffixSibling {
		t.Fatalf("exact full topic identity must outrank suffix sibling: exact=%d sibling=%d query=%+v", exact, suffixSibling, query)
	}
}

func TestAutomaticRelatedRequiresCompleteAtomicModelIdentityAndFeature(t *testing.T) {
	engine := NewEngine()
	tests := []struct {
		name        string
		whatChanged string
		positive    string
		negative    []string
		identity    string
	}{
		{
			name:        "GPT version and separator normalization",
			whatChanged: "GPT-6.1 improves coding quality",
			positive:    "gpt 6.1 coding notes",
			negative:    []string{"GPT-6.2 coding notes", "GPT coding notes"},
			identity:    "gpt 6 1",
		},
		{
			name:        "Step preview qualifier is atomic",
			whatChanged: "Step 5 Preview improves coding quality",
			positive:    "Step-5-preview coding notes",
			negative:    []string{"Step 5 coding notes", "Step 5 Beta coding notes"},
			identity:    "step 5 preview",
		},
		{
			name:        "Opus decimal version is atomic",
			whatChanged: "opus5.5 improves coding quality",
			positive:    "Opus 5.5 coding notes",
			negative:    []string{"Opus 5.6 coding notes", "Opus coding notes"},
			identity:    "opus 5 5",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			query := engine.Extract(relatedTimeline(test.whatChanged, "Coding is the focused feature", "", ""))
			if len(query.Terms)+len(query.IdentityPhrases) > MaxQueryTerms {
				t.Fatalf("combined retrieval features exceed bound: terms=%v identities=%v", query.Terms, query.IdentityPhrases)
			}
			foundIdentity := false
			for _, identity := range query.IdentityPhrases {
				if identity == test.identity {
					foundIdentity = true
				}
			}
			if !foundIdentity {
				t.Fatalf("complete identity %q missing from %v", test.identity, query.IdentityPhrases)
			}
			for _, fragment := range []string{"gpt", "step", "opus", "5", "6", "1"} {
				for _, term := range query.Terms {
					if term == fragment && (fragment == "5" || fragment == "6" || fragment == "1" || fragment == "gpt" || fragment == "step" || fragment == "opus") {
						t.Fatalf("identity fragment leaked into ordinary FTS terms: %q in %v", fragment, query.Terms)
					}
				}
			}
			candidate := testMemory("positive", test.positive, "A focused coding report")
			if result := engine.Match(query, []Candidate{{Item: candidate}}, 3); len(result) != 1 {
				t.Fatalf("complete model identity was rejected: %+v", result)
			}
			for _, title := range test.negative {
				if result := engine.Match(query, []Candidate{{Item: testMemory(title, title, "A focused coding report")}}, 3); len(result) != 0 {
					t.Fatalf("conflicting or partial identity %q was admitted: %+v", title, result)
				}
			}
		})
	}

	query := engine.Extract(relatedTimeline("Anthropic Claude Sonnet 4.5 safety policy", "Model safety and policy", "", ""))
	if result := engine.Match(query, []Candidate{{Item: testMemory("same-company-different-model", "Anthropic Claude Opus 4.5 safety policy", "Safety policy notes")}}, 3); len(result) != 0 {
		t.Fatalf("same-company different-model overlap was admitted: %+v", result)
	}
}

func TestMatchIsBoundedAndUsesBM25OnlyAsTieBreaker(t *testing.T) {
	engine := NewEngine()
	query := Query{Terms: []string{"gyroscope", "stabilization"}, FocusPhrases: []string{"gyroscope stabilization"}}
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
	query := Query{Terms: []string{"gyroscope", "stabilization"}, FocusPhrases: []string{"gyroscope stabilization"}}
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

func TestTopicIdentitySpecificityPrefersTheMostSpecificSibling(t *testing.T) {
	query := Query{Terms: []string{"codex", "reset", "schedule"}}
	if TopicIdentitySpecificity(query, "Codex Reset", nil) <= TopicIdentitySpecificity(query, "Codex", nil) {
		t.Fatal("the complete sibling identity should outrank its broad parent")
	}
	query = Query{Terms: []string{"codex", "performance"}}
	if TopicIdentitySpecificity(query, "Codex", nil) <= TopicIdentitySpecificity(query, "Codex Reset", nil) {
		t.Fatal("the broad topic should win when the sibling qualifier is absent")
	}
}
