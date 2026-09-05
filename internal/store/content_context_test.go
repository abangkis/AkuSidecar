package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
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
	if result.Matches[0].MatchReason != "Shared topics: quantum; supported by title." {
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

func TestContentContextSurfacesOnlyCurrentSupportedLivingTopicKnowledge(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	timelineID := insertContentContextTimelineFixture(t, state, false)
	topic, err := state.CreateLivingTopic(ctx, "Quantum Systems")
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := state.CreateMemoryRecallStub(ctx, libraryInput("topic-insight", domain.SourceLinkedIn, "Quantum systems evidence", "A supported research result", "2026-08-29T00:00:00Z"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.AddLivingTopicMember(ctx, topic.ID, evidence.ID); err != nil {
		t.Fatal(err)
	}
	snapshot, err := state.SaveLivingTopicSnapshot(ctx, domain.LivingTopicSnapshot{
		TopicID: topic.ID, Status: "ready", Overview: "Quantum systems research has a source-backed result.",
		Claims: []domain.LivingTopicClaim{
			{Text: "The quantum systems result is supported.", Assessment: "supported", TemporalStatus: "current", EventStatus: "ongoing", EvidenceIDs: []string{evidence.ID}},
			{Text: "The earlier quantum systems result was completed.", Assessment: "supported", TemporalStatus: "historical", EventStatus: "completed", EvidenceIDs: []string{evidence.ID}},
			{Text: "The timing of a quantum systems result is unknown.", Assessment: "supported", TemporalStatus: "unknown", EventStatus: "unknown", EvidenceIDs: []string{evidence.ID}},
			{Text: "A legacy quantum systems claim has no temporal classification.", Assessment: "supported", EventStatus: "ongoing", EvidenceIDs: []string{evidence.ID}},
			{Text: "The current quantum systems follow-up completed.", Assessment: "supported", TemporalStatus: "current", EventStatus: "completed", EvidenceIDs: []string{evidence.ID}},
			{Text: "A second claim remains uncertain.", Assessment: "uncertain", EvidenceIDs: []string{evidence.ID}},
		},
		EvidenceIDs: []string{evidence.ID}, InputDigest: "topic-insight-digest", CreatedAt: "2026-09-05T10:00:00Z", EvidenceAsOf: "2026-08-29T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.ExecContext(ctx, `UPDATE living_topics SET understanding_status='current',understanding_input_digest=? WHERE id=?`, snapshot.InputDigest, topic.ID); err != nil {
		t.Fatal(err)
	}
	result, err := state.ContentContext(ctx, timelineID, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.TopicInsights) != 1 {
		t.Fatalf("topic insights=%+v", result.TopicInsights)
	}
	insight := result.TopicInsights[0]
	if insight.TopicID != topic.ID || insight.TopicName != topic.Name || insight.EvidenceCount != 1 || insight.SnapshotVersion != snapshot.Version || insight.UpdatedAt != snapshot.CreatedAt || insight.EvidenceAsOf != snapshot.EvidenceAsOf || insight.MatchReason == "" {
		t.Fatalf("topic insight=%+v", insight)
	}
	if len(insight.Claims) != 2 || insight.Claims[0].Assessment != "supported" || insight.Claims[0].TemporalStatus != "current" || insight.Claims[1].EventStatus != "completed" {
		t.Fatalf("only current supported claims should be projected, including completed events: %+v", insight.Claims)
	}
	if _, err := state.RemoveLivingTopicMember(ctx, topic.ID, evidence.ID); err != nil {
		t.Fatal(err)
	}
	result, err = state.ContentContext(ctx, timelineID, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.TopicInsights) != 0 {
		t.Fatalf("historical topic knowledge leaked into Related Context: %+v", result.TopicInsights)
	}
}

func TestLibrarySearchUsesCurrentSupportedLivingTopicKnowledgeReadOnly(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	topic, err := state.CreateLivingTopic(ctx, "OpenAI GPT Astra")
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := state.CreateMemoryRecallStub(ctx, libraryInput("library-topic-knowledge", domain.SourceX, "GPT Astra evidence", "Agent coordination details", "2026-08-30T00:00:00Z"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.AddLivingTopicMember(ctx, topic.ID, evidence.ID); err != nil {
		t.Fatal(err)
	}
	snapshot, err := state.SaveLivingTopicSnapshot(ctx, domain.LivingTopicSnapshot{
		TopicID: topic.ID, Status: "ready", Overview: "The current evidence describes advanced orchestration capabilities.",
		Claims: []domain.LivingTopicClaim{
			{Text: "Academic agent coordination is supported by the current evidence.", Assessment: "supported", TemporalStatus: "current", EventStatus: "ongoing", EvidenceIDs: []string{evidence.ID}},
			{Text: "An earlier agent coordination release was completed.", Assessment: "supported", TemporalStatus: "historical", EventStatus: "completed", EvidenceIDs: []string{evidence.ID}},
			{Text: "The timing of another agent coordination release is unknown.", Assessment: "supported", TemporalStatus: "unknown", EventStatus: "unknown", EvidenceIDs: []string{evidence.ID}},
			{Text: "The current agent coordination follow-up completed.", Assessment: "supported", TemporalStatus: "current", EventStatus: "completed", EvidenceIDs: []string{evidence.ID}},
			{Text: "A release date remains uncertain.", Assessment: "uncertain", EvidenceIDs: []string{evidence.ID}},
		},
		EvidenceIDs: []string{evidence.ID}, InputDigest: "library-topic-knowledge-digest", CreatedAt: "2026-09-05T10:00:00Z", EvidenceAsOf: "2026-08-30T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.ExecContext(ctx, `UPDATE living_topics SET understanding_status='current',understanding_input_digest=? WHERE id=?`, snapshot.InputDigest, topic.ID); err != nil {
		t.Fatal(err)
	}
	var beforeActions, beforeSnapshots int
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_actions`).Scan(&beforeActions); err != nil {
		t.Fatal(err)
	}
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM living_topic_snapshots`).Scan(&beforeSnapshots); err != nil {
		t.Fatal(err)
	}

	insights, err := state.SearchLivingTopicKnowledge(ctx, "agent coordination", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(insights) != 1 || insights[0].TopicID != topic.ID || insights[0].SnapshotVersion != snapshot.Version || insights[0].UpdatedAt != snapshot.CreatedAt || insights[0].EvidenceAsOf != snapshot.EvidenceAsOf || insights[0].MatchReason == "" {
		t.Fatalf("topic knowledge=%+v", insights)
	}
	if len(insights[0].Claims) != 2 || insights[0].Claims[0].TemporalStatus != "current" || insights[0].Claims[1].EventStatus != "completed" {
		t.Fatalf("only current supported claims should be searchable, including completed events: %+v", insights[0].Claims)
	}
	var afterActions, afterSnapshots int
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_actions`).Scan(&afterActions); err != nil {
		t.Fatal(err)
	}
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM living_topic_snapshots`).Scan(&afterSnapshots); err != nil {
		t.Fatal(err)
	}
	if beforeActions != afterActions || beforeSnapshots != afterSnapshots {
		t.Fatalf("Library topic search wrote state: actions %d->%d snapshots %d->%d", beforeActions, afterActions, beforeSnapshots, afterSnapshots)
	}

	if _, err := state.RemoveLivingTopicMember(ctx, topic.ID, evidence.ID); err != nil {
		t.Fatal(err)
	}
	insights, err = state.SearchLivingTopicKnowledge(ctx, "agent coordination", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(insights) != 0 {
		t.Fatalf("historical topic knowledge leaked into Library search: %+v", insights)
	}
}

func TestLibrarySearchSuppressesLessSpecificLivingTopicSibling(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	parent, _ := state.CreateLivingTopic(ctx, "Codex")
	sibling, _ := state.CreateLivingTopic(ctx, "Codex Reset")
	for index, topic := range []domain.LivingTopic{parent, sibling} {
		input := libraryInput(fmt.Sprintf("sibling-%d", index), domain.SourceX, topic.Name+" evidence", "Codex reset performance schedule", "2026-08-30T00:00:00Z")
		item, err := state.CreateMemoryRecallStub(ctx, input)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := state.AddLivingTopicMember(ctx, topic.ID, item.ID); err != nil {
			t.Fatal(err)
		}
		snapshot, err := state.SaveLivingTopicSnapshot(ctx, domain.LivingTopicSnapshot{TopicID: topic.ID, Status: "ready", Overview: topic.Name + " current knowledge.", Claims: []domain.LivingTopicClaim{{Text: "Codex reset performance schedule is tracked.", Assessment: "supported", Centrality: "central", TemporalStatus: "current", EventStatus: "ongoing", EvidenceIDs: []string{item.ID}}}, EvidenceIDs: []string{item.ID}, InputDigest: fmt.Sprintf("digest-%d", index)})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := state.db.ExecContext(ctx, `UPDATE living_topics SET understanding_status='current',understanding_input_digest=? WHERE id=?`, snapshot.InputDigest, topic.ID); err != nil {
			t.Fatal(err)
		}
	}
	insights, err := state.SearchLivingTopicKnowledge(ctx, "Codex reset schedule", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(insights) != 1 || insights[0].TopicID != sibling.ID {
		t.Fatalf("reset query=%+v", insights)
	}
	insights, err = state.SearchLivingTopicKnowledge(ctx, "Codex performance", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(insights) != 1 || insights[0].TopicID != parent.ID {
		t.Fatalf("performance query=%+v", insights)
	}
}

func TestContentContextFeedbackIsPairwiseAppendOnlyAndUndoable(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	timelineID := insertContentContextTimelineFixture(t, state, false)
	matched, err := state.CreateMemoryRecallStub(ctx, libraryInput("feedback-match", domain.SourceX, "Quantum research notes", "Local context for systems", "2026-08-02T00:00:00Z"))
	if err != nil {
		t.Fatal(err)
	}

	negative, err := state.AddContentContextFeedback(ctx, timelineID, domain.ContentContextFeedbackInput{
		MemoryItemID: matched.ID,
		Verdict:      domain.ContentContextFeedbackNotRelevant,
	})
	if err != nil {
		t.Fatal(err)
	}
	if negative.EngineVersion != domain.ContentContextEngineVersion || negative.ResultRank != 1 || negative.MatchReason == "" {
		t.Fatalf("negative feedback=%+v", negative)
	}
	result, err := state.ContentContext(ctx, timelineID, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Matches) != 0 {
		t.Fatalf("negative pair must be suppressed on the next retrieval: %+v", result.Matches)
	}
	if _, err := state.AddContentContextFeedback(ctx, timelineID, domain.ContentContextFeedbackInput{
		MemoryItemID: "memory-not-surfaced",
		Verdict:      domain.ContentContextFeedbackRelevant,
	}); err == nil {
		t.Fatal("feedback for a non-surfaced pair must fail closed")
	}

	cleared, err := state.UndoContentContextFeedback(ctx, negative.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cleared.Verdict != domain.ContentContextFeedbackClear || cleared.SupersedesID != negative.ID {
		t.Fatalf("clear feedback=%+v", cleared)
	}
	result, err = state.ContentContext(ctx, timelineID, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Matches) != 1 || result.Matches[0].Item.ID != matched.ID || result.Matches[0].Feedback != nil {
		t.Fatalf("cleared pair did not return without active feedback: %+v", result.Matches)
	}

	positive, err := state.AddContentContextFeedback(ctx, timelineID, domain.ContentContextFeedbackInput{
		MemoryItemID: matched.ID,
		Verdict:      domain.ContentContextFeedbackRelevant,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err = state.ContentContext(ctx, timelineID, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Matches) != 1 || result.Matches[0].Feedback == nil || result.Matches[0].Feedback.ID != positive.ID || result.Matches[0].Feedback.Verdict != domain.ContentContextFeedbackRelevant {
		t.Fatalf("positive feedback projection=%+v", result.Matches)
	}
	if _, err := state.UndoContentContextFeedback(ctx, negative.ID); !errors.Is(err, ErrContentContextFeedbackNotCurrent) {
		t.Fatalf("stale feedback undo error=%v", err)
	}
	var rows int
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM content_context_feedback_events`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 3 {
		t.Fatalf("append-only feedback rows=%d want=3", rows)
	}
	if err := state.ResetLearning(ctx); err != nil {
		t.Fatal(err)
	}
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM content_context_feedback_events`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("reset learning feedback rows=%d err=%v", rows, err)
	}
}
