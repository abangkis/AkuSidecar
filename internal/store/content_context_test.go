package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func insertContentContextTimelineFixture(t *testing.T, state *Store, prepared bool) string {
	t.Helper()
	ctx := context.Background()
	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var session domain.Session
	if prepared {
		session, err = createPreparedUpdateSession(state, ctx, "content context prepared", settings)
	} else {
		session, err = createVisibleUpdateSession(state, ctx, "content context visible", settings)
	}
	if err != nil {
		t.Fatal(err)
	}
	runs, err := state.listRuns(ctx, session.ID)
	if err != nil || len(runs) == 0 {
		t.Fatalf("runs=%+v err=%v", runs, err)
	}
	now := domain.Now()
	item := domain.ReasonedItem{
		EvidenceKey:  "x:content-context-same",
		Source:       domain.SourceX,
		WhatChanged:  "Context-specific quantum systems",
		WhyItMatters: "Local research memory",
		SourceURL:    "https://x.com/reader/status/context-context",
	}
	assessment := domain.CandidateAssessment{
		EvidenceKey: "x:content-context-same",
		TopicTags:   []string{"quantum", "science"},
		TopicFacets: []string{"research"},
	}
	itemRaw, _ := json.Marshal(item)
	assessmentRaw, _ := json.Marshal(assessment)
	timelineID := "timeline-content-context-visible"
	if prepared {
		timelineID = "timeline-content-context-prepared"
	}
	if _, err := state.db.ExecContext(ctx, `
		INSERT INTO timeline_items(id,session_id,run_id,source,evidence_key,rank,item_json,assessment_json,coverage_json,created_at)
		VALUES(?,?,?,?,?,?,?,?,?,?)`, timelineID, session.ID, runs[0].ID, domain.SourceX, item.EvidenceKey, 0, string(itemRaw), string(assessmentRaw), "{}", now); err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.ExecContext(ctx, `UPDATE sessions SET status='completed',completed_at=? WHERE id=?`, now, session.ID); err != nil {
		t.Fatal(err)
	}
	if prepared {
		if _, err := state.db.ExecContext(ctx, `UPDATE auto_update_batches SET state='prepared',prepared_at=? WHERE session_id=?`, now, session.ID); err != nil {
			t.Fatal(err)
		}
	}
	return timelineID
}

func TestContentContextSearchesLocalFTSExcludesCurrentIdentityAndDoesNotWrite(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	timelineID := insertContentContextTimelineFixture(t, state, false)
	sameInput := libraryInput("context-same", domain.SourceX, "Context-specific quantum systems", "Local research memory", "2026-08-01T00:00:00Z")
	sameInput.Identity.CanonicalEvidenceKey = "x:content-context-same"
	current, err := state.CreateMemoryRecallStub(ctx, sameInput)
	if err != nil {
		t.Fatal(err)
	}
	other, err := state.CreateMemoryRecallStub(ctx, libraryInput("context-other", domain.SourceX, "Quantum research notes", "Local context for systems", "2026-08-02T00:00:00Z"))
	if err != nil {
		t.Fatal(err)
	}
	var beforeActions, beforeProvenance, beforeItems int
	for query, target := range map[string]*int{
		"SELECT COUNT(*) FROM memory_actions":    &beforeActions,
		"SELECT COUNT(*) FROM memory_provenance": &beforeProvenance,
		"SELECT COUNT(*) FROM memory_items":      &beforeItems,
	} {
		if err := state.db.QueryRowContext(ctx, query).Scan(target); err != nil {
			t.Fatal(err)
		}
	}

	result, err := state.ContentContext(ctx, timelineID, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Matches) != 1 || result.Matches[0].Item.ID != other.ID {
		t.Fatalf("context matches=%+v", result.Matches)
	}
	for _, match := range result.Matches {
		if match.Item.ID == current.ID {
			t.Fatalf("current Timeline identity leaked into context matches: %+v", result.Matches)
		}
	}
	if result.Matches[0].MatchReason != "Matches title, summary, author, tags, facets" {
		t.Fatalf("context reason=%q", result.Matches[0].MatchReason)
	}
	var afterActions, afterProvenance, afterItems int
	for query, target := range map[string]*int{
		"SELECT COUNT(*) FROM memory_actions":    &afterActions,
		"SELECT COUNT(*) FROM memory_provenance": &afterProvenance,
		"SELECT COUNT(*) FROM memory_items":      &afterItems,
	} {
		if err := state.db.QueryRowContext(ctx, query).Scan(target); err != nil {
			t.Fatal(err)
		}
	}
	if beforeActions != afterActions || beforeProvenance != afterProvenance || beforeItems != afterItems {
		t.Fatalf("content context wrote memory rows: before actions=%d provenance=%d items=%d after actions=%d provenance=%d items=%d", beforeActions, beforeProvenance, beforeItems, afterActions, afterProvenance, afterItems)
	}
}

func TestContentContextRequiresFinalVisibleTimelineAndBoundsLimit(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	visibleID := insertContentContextTimelineFixture(t, state, false)
	preparedID := insertContentContextTimelineFixture(t, state, true)
	if _, err := state.ContentContext(ctx, visibleID, 6); err == nil {
		t.Fatal("limit above five must fail")
	}
	if _, err := state.ContentContext(ctx, visibleID, -1); err == nil {
		t.Fatal("negative limit must fail")
	}
	for _, id := range []string{preparedID, "missing-timeline"} {
		_, err := state.ContentContext(ctx, id, 3)
		if id == preparedID && !errors.Is(err, ErrContentContextNotEligible) {
			t.Fatalf("prepared Timeline error=%v", err)
		}
		if id == "missing-timeline" && !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("missing Timeline error=%v", err)
		}
	}
}
