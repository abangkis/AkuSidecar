package store

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/contentcontext"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

func setRelatedContextSubject(t *testing.T, state *Store, id, title, summary string, tags []string) {
	t.Helper()
	item := domain.ReasonedItem{EvidenceKey: "x:content-context-same", Source: domain.SourceX,
		WhatChanged: title, WhyItMatters: summary, SourceURL: "https://x.com/reader/status/context-context"}
	assessment := domain.CandidateAssessment{EvidenceKey: item.EvidenceKey, TopicTags: tags}
	itemJSON, _ := json.Marshal(item)
	assessmentJSON, _ := json.Marshal(assessment)
	if _, err := state.db.Exec(`UPDATE timeline_items SET item_json=?,assessment_json=? WHERE id=?`, string(itemJSON), string(assessmentJSON), id); err != nil {
		t.Fatal(err)
	}
}

func TestRelatedContextResetMeasurementsReachRelevantMemoryThroughFTS(t *testing.T) {
	for _, example := range []struct{ title, summary, text, author string }{
		{
			title:   "TestingCatalog reported that Codex can now use a reset on its own when explicitly requested by users when the remaining limit is 10% or less, quoting Tibo's Day 6 update.",
			summary: "Informs developers of new autonomous reset capabilities in Codex for managing context limits.",
			text:    "ICYMI 👀: Codex can now use a reset on its own when users explicitly ask it to do so.\n\nAllow Codex to use resets - Give Codex the ability to use a reset when you explicitly request it during a conversation and your current limit has 10% or less remaining. Resets are never used automatically.\n\nThis setting is placed in the Usage & billing section.",
			author:  "AI News | TestingCatalog @testingcatalog · 53m",
		},
		{
			title:   "Kerdo reported an upcoming Codex usage reset in approximately 8 hours and 40 minutes, teasing Claude users.",
			summary: "Provides precise timing for Codex users anticipating their usage limits to reset.",
			text:    "🚨 CODEX USERS: USAGE RESET IN 8 HRS 40 MINNSSSSS🔥🔥🔥\n\nim so sad for claude users tbh",
			author:  "Kerdo @AbdoKerdawy · 1h",
		},
	} {
		t.Run(example.author, func(t *testing.T) {
			ctx := context.Background()
			state := openTestStore(t)
			id := insertContentContextTimelineFixture(t, state, false)
			setRelatedContextSubject(t, state, id, example.title, example.summary, []string{"openai", "codex", "reset", "usage_reset"})
			evidence, err := json.Marshal(domain.Block{Text: example.text, Author: example.author})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := state.db.Exec(`UPDATE timeline_items SET evidence_snapshot_json=? WHERE id=?`, string(evidence), id); err != nil {
				t.Fatal(err)
			}
			input := libraryInput("reset-note", domain.SourceX,
				"Ananth reported that Codex resets are suspended for the next month because OpenAI's dots usage is nearly unlimited right now.",
				"Informs developers and users about temporary relief from usage limits and capacity management.", "2026-10-10T00:00:00Z")
			input.Author = "Ananth @Ananth7e · 1h"
			input.Tags = []string{"openai", "codex", "dots", "ai_limits"}
			positive, err := state.CreateMemoryRecallStub(ctx, input)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := state.CreateMemoryRecallStub(ctx, libraryInput("plugins", domain.SourceX, "Codex plugin installation", "Developer usage workflows", "2026-10-10T00:00:00Z")); err != nil {
				t.Fatal(err)
			}
			result, err := state.ContentContext(ctx, id, 3)
			if err != nil || len(result.Matches) != 1 || result.Matches[0].Item.ID != positive.ID {
				t.Fatalf("real reset context still omitted or unrelated memory admitted: result=%+v err=%v", result, err)
			}
		})
	}
}

func TestRelatedContextRequiresSubjectRelationshipThroughFTSWhileLibraryRemainsBroad(t *testing.T) {
	cases := []struct {
		name, title, summary string
		tags                 []string
		memories             []domain.MemoryItemInput
		want                 int
	}{
		{name: "same model and issue", title: "GPT-6.1 context window limits", summary: "GPT-6.1 supports larger context windows",
			tags: []string{"GPT-6.1", "OpenAI", "AI", "context window"}, want: 1,
			memories: []domain.MemoryItemInput{
				libraryInput("exact", domain.SourceX, "GPT-6.1 context window notes", "GPT-6.1 context window limits explained", "2026-10-08T00:00:00Z"),
				libraryInput("sibling", domain.SourceX, "GPT-6.2 context window notes", "GPT-6.2 context window limits explained", "2026-10-08T00:00:00Z"),
				libraryInput("other", domain.SourceX, "Opus 5.5 context window notes", "Opus 5.5 context window limits explained", "2026-10-08T00:00:00Z"),
			}},
		{name: "full version and preview qualifier", title: "Step 5 Preview tool calling behavior", summary: "Step 5 Preview tool calling has a new restriction",
			tags: []string{"Step 5 Preview", "tool calling", "AI"}, want: 1,
			memories: []domain.MemoryItemInput{
				libraryInput("exact", domain.SourceX, "Step 5 Preview tool calling notes", "Step 5 Preview tool calling restriction", "2026-10-08T00:00:00Z"),
				libraryInput("sibling", domain.SourceX, "Step 5 tool calling notes", "Step 5 tool calling restriction", "2026-10-08T00:00:00Z"),
				libraryInput("other", domain.SourceX, "Opus 5.5 tool calling notes", "Opus 5.5 tool calling restriction", "2026-10-08T00:00:00Z"),
			}},
		{name: "broad company and usage are insufficient", title: "OpenAI AI usage update", summary: "OpenAI announces AI usage changes",
			tags: []string{"OpenAI", "AI", "usage"}, want: 0,
			memories: []domain.MemoryItemInput{
				libraryInput("company", domain.SourceX, "OpenAI funding announcements", "OpenAI AI usage update", "2026-10-08T00:00:00Z"),
			}},
		{name: "cross company issue", title: "Anthropic policy on automated cruelty", summary: "Anthropic AI usage policy addresses cruelty",
			tags: []string{"Anthropic", "AI", "usage", "cruelty"}, want: 0,
			memories: []domain.MemoryItemInput{
				libraryInput("company", domain.SourceX, "OpenAI usage quotas", "Anthropic is also discussed in an AI usage summary", "2026-10-08T00:00:00Z"),
			}},
		{name: "same country unrelated achievements", title: "Indonesian girl group Nona achieved a historic music milestone", summary: "International cultural achievement for Indonesia",
			tags: []string{"Indonesia", "Nona", "music"}, want: 0,
			memories: []domain.MemoryItemInput{
				libraryInput("sport", domain.SourceX, "Indonesia weightlifter wins gold", "A major international athletic milestone for Indonesia", "2026-10-10T00:00:00Z"),
			}},
		{name: "same product unrelated developer workflows", title: "Codex reset schedule", summary: "Informs developers about the reset timing",
			tags: []string{"Codex", "reset"}, want: 1,
			memories: []domain.MemoryItemInput{
				libraryInput("reset", domain.SourceX, "Codex resets suspended for a month", "Temporary relief from limits", "2026-10-10T00:00:00Z"),
				libraryInput("plugins", domain.SourceX, "Codex custom plugins", "Developers build custom software workflows", "2026-10-10T00:00:00Z"),
			}},
		{name: "exact model but competitive commentary insufficient", title: "Opus 5.5 error rate breakdown", summary: "Competitive frontier model evaluation",
			want: 1,
			memories: []domain.MemoryItemInput{
				libraryInput("error", domain.SourceX, "Opus 5.5 error rate analysis", "Benchmark failures", "2026-10-10T00:00:00Z"),
				libraryInput("advice", domain.SourceX, "Opus 5.5 powers Dots", "Competitive advice for model selection", "2026-10-10T00:00:00Z"),
			}},
	}
	for _, example := range cases {
		t.Run(example.name, func(t *testing.T) {
			ctx := context.Background()
			state := openTestStore(t)
			id := insertContentContextTimelineFixture(t, state, false)
			setRelatedContextSubject(t, state, id, example.title, example.summary, example.tags)
			var exact string
			for index, input := range example.memories {
				input.Tags = []string{"OpenAI", "AI", "usage"}
				// Fixture IDs such as "reset"/"error" are issue labels, not the
				// writer's name. Do not turn them into attribution exclusions.
				input.Author = "Fixture writer"
				memory, err := state.CreateMemoryRecallStub(ctx, input)
				if err != nil {
					t.Fatal(err)
				}
				if index == 0 {
					exact = memory.ID
				}
			}
			var before int
			if err := state.db.QueryRow(`SELECT COUNT(*) FROM memory_actions`).Scan(&before); err != nil {
				t.Fatal(err)
			}
			result, err := state.ContentContext(ctx, id, 5)
			if err != nil || len(result.Matches) != example.want {
				t.Fatalf("matches=%+v want=%d err=%v", result.Matches, example.want, err)
			}
			if example.want == 1 && result.Matches[0].Item.ID != exact {
				t.Fatalf("wrong subject surfaced: %+v", result.Matches)
			}
			// Intentional Library queries still discover broad categories and metadata.
			library, err := state.SearchMemoryLibrary(ctx, domain.MemoryLibraryQuery{Query: "OpenAI", Limit: 10})
			if err != nil || len(library.Items) != len(example.memories) {
				t.Fatalf("Library search narrowed: items=%d err=%v", len(library.Items), err)
			}
			var after int
			if err := state.db.QueryRow(`SELECT COUNT(*) FROM memory_actions`).Scan(&after); err != nil {
				t.Fatal(err)
			}
			if before != after {
				t.Fatal("Related Context or Library retrieval wrote actions")
			}
		})
	}
}

func TestRelatedContextFeatureEquivalentsReachFTSCandidatesWithoutBroadStemming(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	for _, title := range []string{"reset", "resets", "resetting", "Step 5 Preview"} {
		if _, err := state.CreateMemoryRecallStub(ctx, libraryInput(title, domain.SourceX, title, "Stored note", "2026-10-10T00:00:00Z")); err != nil {
			t.Fatal(err)
		}
	}
	for _, term := range []string{"reset", "resets"} {
		candidates, err := state.searchMemoryContextCandidates(ctx, contentcontext.Query{Terms: []string{term}}, 24)
		if err != nil || len(candidates) != 2 {
			t.Fatalf("automatic %q candidates=%+v err=%v", term, candidates, err)
		}
	}
	candidates, err := state.searchMemoryContextCandidates(ctx, contentcontext.Query{Terms: []string{"reset"}, Mode: contentcontext.QueryLibrarySearch}, 24)
	if err != nil || len(candidates) != 1 || candidates[0].Item.Title != "reset" {
		t.Fatalf("explicit search changed: %+v err=%v", candidates, err)
	}
}

func TestRelatedContextConcreteIssueGateAfterFTSRetrieval(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	id := insertContentContextTimelineFixture(t, state, false)
	setRelatedContextSubject(t, state, id, "Codex can use a reset when explicitly requested", "", []string{"Codex"})
	var wanted string
	for index, title := range []string{
		"Codex resets suspended for a month",
		"Codex coding in a software factory can use QA when needed",
		"Codex limits can use Dots for extended usage",
		"Synchronization when moving between Codex and ChatGPT",
		"Dots use cases for controlling Codex through CUA",
		"Codex licensing while chatgpt resets",
	} {
		input := libraryInput(domain.NewID("issue-fixture"), domain.SourceX, title, "", "2026-10-10T00:00:00Z")
		input.Author = "Fixture writer"
		input.Tags = []string{"Codex"}
		memory, err := state.CreateMemoryRecallStub(ctx, input)
		if err != nil {
			t.Fatal(err)
		}
		if index == 0 {
			wanted = memory.ID
		}
	}
	result, err := state.ContentContext(ctx, id, 5)
	if err != nil || len(result.Matches) != 1 || result.Matches[0].Item.ID != wanted {
		t.Fatalf("FTS prose/cross-product candidate escaped concrete issue gate: %+v err=%v", result, err)
	}
	library, err := state.SearchMemoryLibrary(ctx, domain.MemoryLibraryQuery{Query: "Codex", Limit: 10})
	if err != nil || len(library.Items) != 6 {
		t.Fatalf("explicit Library search narrowed: items=%d err=%v", len(library.Items), err)
	}
}

func TestRelatedContextAtomicFTSPhraseDoesNotRetrieveNumericFragments(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	for _, title := range []string{"Step 5 Preview tool calling", "Step5Preview tool calling", "Opus 5.5 tool calling", "Step 5 tool calling"} {
		if _, err := state.CreateMemoryRecallStub(ctx, libraryInput(title, domain.SourceX, title, "Model notes", "2026-10-08T00:00:00Z")); err != nil {
			t.Fatal(err)
		}
	}
	candidates, err := state.searchMemoryContextCandidates(ctx, contentcontext.Query{IdentityPhrases: []string{"step 5 preview"}}, 24)
	if err != nil || len(candidates) != 2 {
		t.Fatalf("atomic FTS candidates=%+v err=%v", candidates, err)
	}
	for _, candidate := range candidates {
		if candidate.Item.Title != "Step 5 Preview tool calling" && candidate.Item.Title != "Step5Preview tool calling" {
			t.Fatalf("numeric fragment retrieved: %+v", candidate)
		}
	}
	// FTS operators in a feature are tokenized and quoted, never executable syntax.
	if _, err := state.searchMemoryContextCandidates(ctx, contentcontext.Query{Terms: []string{`title:step OR * "`}}, 24); err != nil {
		t.Fatalf("unsafe FTS syntax escaped the feature boundary: %v", err)
	}
}
