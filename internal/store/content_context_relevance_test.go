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
