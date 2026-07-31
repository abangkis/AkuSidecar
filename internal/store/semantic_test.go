package store

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func insertSemanticTimelineFixture(t *testing.T, state *Store, relation string) (domain.Session, string) {
	t.Helper()
	ctx := context.Background()
	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	session, err := createVisibleUpdateSession(state, ctx, "semantic fixture", settings)
	if err != nil {
		t.Fatal(err)
	}
	runs, err := state.listRuns(ctx, session.ID)
	if err != nil || len(runs) == 0 {
		t.Fatalf("runs=%+v err=%v", runs, err)
	}
	timelineID := domain.NewID("timeline")
	evidenceKey := "x:semantic-fixture"
	itemRaw, _ := json.Marshal(domain.ReasonedItem{EvidenceKey: evidenceKey, Source: domain.SourceX, WhatChanged: "OpenAI launches Codex App Server", Author: "Reporter", EventKey: "codex-app-server"})
	assessmentRaw, _ := json.Marshal(domain.CandidateAssessment{EvidenceKey: evidenceKey, TopicTags: []string{"codex", "app-server"}})
	now := domain.Now()
	if _, err := state.db.ExecContext(ctx, `INSERT INTO timeline_items(id,session_id,run_id,source,evidence_key,rank,item_json,assessment_json,coverage_json,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, timelineID, session.ID, runs[0].ID, domain.SourceX, evidenceKey, 0, string(itemRaw), string(assessmentRaw), "{}", now); err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.ExecContext(ctx, `INSERT INTO semantic_events(id,canonical_claim,actor,action,object,event_kind,aliases_json,first_seen_at,last_seen_at) VALUES(?,?,?,?,?,?,?,?,?)`, "event-existing", "OpenAI launches Codex App Server", "OpenAI", "launches", "Codex App Server", "release", "[]", now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.ExecContext(ctx, `INSERT INTO semantic_event_reports(id,event_id,timeline_id,session_id,run_id,evidence_key,source,relation,confidence,reason,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, domain.NewID("report"), "event-existing", timelineID, session.ID, runs[0].ID, evidenceKey, domain.SourceX, relation, .98, "fixture", now); err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.ExecContext(ctx, `UPDATE sessions SET status='completed',completed_at=? WHERE id=?`, now, session.ID); err != nil {
		t.Fatal(err)
	}
	return session, timelineID
}

func TestSemanticDisplayModesAndLatestUniqueCount(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	_, _ = insertSemanticTimelineFixture(t, state, "duplicate_report")

	items, err := state.ListTimeline(ctx, 10, 0)
	if err != nil || len(items) != 1 || items[0].SemanticEvent == nil {
		t.Fatalf("collapse items=%+v err=%v", items, err)
	}
	latest, err := state.LatestTimelineCheck(ctx)
	if err != nil || latest.AddedItems != 0 || latest.DuplicateReports != 1 {
		t.Fatalf("collapse latest=%+v err=%v", latest, err)
	}

	settings, _ := state.GetSettings(ctx)
	settings.SemanticEventMode = "hide"
	if err := state.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	items, err = state.ListTimeline(ctx, 10, 0)
	if err != nil || len(items) != 0 {
		t.Fatalf("hide items=%+v err=%v", items, err)
	}

	settings.SemanticEventMode = "show_all"
	if err := state.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	items, err = state.ListTimeline(ctx, 10, 0)
	if err != nil || len(items) != 1 || items[0].SemanticEvent != nil {
		t.Fatalf("show-all items=%+v err=%v", items, err)
	}
	latest, err = state.LatestTimelineCheck(ctx)
	if err != nil || latest.AddedItems != 1 || latest.DuplicateReports != 0 {
		t.Fatalf("show-all latest=%+v err=%v", latest, err)
	}
}

func TestExactSemanticEventIDsReturnsNewestNativeEvidenceAssignment(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	_, _ = insertSemanticTimelineFixture(t, state, "new_event")

	matches, err := state.ExactSemanticEventIDs(ctx, []string{"x:semantic-fixture", "x:missing"})
	if err != nil {
		t.Fatal(err)
	}
	if matches["x:semantic-fixture"] != "event-existing" {
		t.Fatalf("exact matches=%+v", matches)
	}
	if _, exists := matches["x:missing"]; exists {
		t.Fatalf("missing evidence received a match: %+v", matches)
	}
}

func TestSemanticCorrectionUndoRemovesConstraint(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	session, timelineID := insertSemanticTimelineFixture(t, state, "duplicate_report")
	if err := state.SaveEventResolutionSummary(ctx, domain.EventResolutionSummary{SessionID: session.ID, Status: "completed", Provider: "test", Model: "test", Effort: "none", CandidateCount: 1, UniqueItems: 0, DuplicateReports: 1, CreatedAt: domain.Now()}); err != nil {
		t.Fatal(err)
	}
	correction, err := state.CorrectSemanticEvent(ctx, timelineID, "not_same_event", "", "")
	if err != nil {
		t.Fatal(err)
	}
	var relation string
	if err := state.db.QueryRowContext(ctx, `SELECT relation FROM semantic_event_reports WHERE timeline_id=?`, timelineID).Scan(&relation); err != nil || relation != "new_event" {
		t.Fatalf("relation=%s err=%v", relation, err)
	}
	var constraints int
	_ = state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM semantic_event_constraints`).Scan(&constraints)
	if constraints != 1 {
		t.Fatalf("constraints=%d want=1", constraints)
	}
	summary, err := state.EventResolutionSummary(ctx, session.ID)
	if err != nil || summary == nil || summary.UserSplitCorrections != 1 || summary.UserMergeCorrections != 0 {
		t.Fatalf("post-hoc correction diagnostics=%+v err=%v", summary, err)
	}
	items, err := state.ListTimeline(ctx, 10, 0)
	if err != nil || len(items) != 1 || items[0].SemanticEvent == nil || items[0].SemanticEvent.CorrectionID != correction.ID {
		t.Fatalf("active correction is not exposed: items=%+v err=%v", items, err)
	}
	if _, err := state.UndoSemanticCorrection(ctx, correction.ID); err != nil {
		t.Fatal(err)
	}
	if err := state.db.QueryRowContext(ctx, `SELECT relation FROM semantic_event_reports WHERE timeline_id=?`, timelineID).Scan(&relation); err != nil || relation != "duplicate_report" {
		t.Fatalf("undo relation=%s err=%v", relation, err)
	}
	_ = state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM semantic_event_constraints`).Scan(&constraints)
	if constraints != 0 {
		t.Fatalf("constraints after undo=%d", constraints)
	}
	summary, err = state.EventResolutionSummary(ctx, session.ID)
	if err != nil || summary == nil || summary.UserSplitCorrections != 0 || summary.UserMergeCorrections != 0 {
		t.Fatalf("undone correction diagnostics=%+v err=%v", summary, err)
	}
	items, err = state.ListTimeline(ctx, 10, 0)
	if err != nil || len(items) != 1 || items[0].SemanticEvent.CorrectionID != "" {
		t.Fatalf("undone correction remains active: items=%+v err=%v", items, err)
	}
}

func TestSemanticCorrectionSummaryCountsUserMerge(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	session, timelineID := insertSemanticTimelineFixture(t, state, "new_event")
	now := domain.Now()
	targetEventID := "event-user-merge-target"
	if _, err := state.db.ExecContext(ctx, `INSERT INTO semantic_events(id,canonical_claim,actor,event_kind,aliases_json,first_seen_at,last_seen_at) VALUES(?,?,?,?,?,?,?)`, targetEventID, "Existing matching event", "Another author", "other", "[]", now, now); err != nil {
		t.Fatal(err)
	}
	if err := state.SaveEventResolutionSummary(ctx, domain.EventResolutionSummary{SessionID: session.ID, Status: "completed", Provider: "test", Model: "test", Effort: "none", CandidateCount: 1, UniqueItems: 1, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if _, err := state.CorrectSemanticEvent(ctx, timelineID, "same_event", targetEventID, "duplicate_report"); err != nil {
		t.Fatal(err)
	}
	summary, err := state.EventResolutionSummary(ctx, session.ID)
	if err != nil || summary == nil || summary.UserSplitCorrections != 0 || summary.UserMergeCorrections != 1 {
		t.Fatalf("user merge diagnostics=%+v err=%v", summary, err)
	}
}

func TestSemanticCorrectionStoresNoveltyAuthorityAndDelta(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	_, timelineID := insertSemanticTimelineFixture(t, state, "new_event")
	correction, err := state.CorrectSemanticEvent(ctx, timelineID, "same_event", "event-existing", "material_update")
	if err != nil {
		t.Fatal(err)
	}
	if correction.ToRelation != "material_update" {
		t.Fatalf("correction=%+v", correction)
	}
	var novelty string
	if err := state.db.QueryRowContext(ctx, `SELECT relation FROM semantic_novelty_constraints WHERE evidence_key=? AND event_id=?`, "x:semantic-fixture", "event-existing").Scan(&novelty); err != nil || novelty != "material_update" {
		t.Fatalf("novelty=%q err=%v", novelty, err)
	}
	var deltas int
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM semantic_event_deltas WHERE event_id=?`, "event-existing").Scan(&deltas); err != nil || deltas != 1 {
		t.Fatalf("deltas=%d err=%v", deltas, err)
	}
	events, err := state.ListSemanticEvents(ctx, "", 10)
	if err != nil || len(events) != 1 || len(events[0].KnownDeltas) != 1 {
		t.Fatalf("events=%+v err=%v", events, err)
	}
	if _, err := state.UndoSemanticCorrection(ctx, correction.ID); err != nil {
		t.Fatal(err)
	}
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM semantic_novelty_constraints`).Scan(&deltas); err != nil || deltas != 0 {
		t.Fatalf("novelty constraints after undo=%d err=%v", deltas, err)
	}
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM semantic_event_deltas`).Scan(&deltas); err != nil || deltas != 0 {
		t.Fatalf("deltas after undo=%d err=%v", deltas, err)
	}
}

func TestSemanticDeltaLedgerBackfillsRetainedSchemaSevenReports(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	insertSemanticTimelineFixture(t, state, "material_update")
	observedAt := "2026-07-30T17:56:01Z"
	if _, err := state.db.ExecContext(ctx, `UPDATE semantic_event_reports SET created_at=?`, observedAt); err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.ExecContext(ctx, `DELETE FROM meta WHERE key='semantic_delta_backfill_version'`); err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.ExecContext(ctx, `
		INSERT INTO semantic_event_deltas(id,event_id,report_id,fingerprint,claim,kind,source,evidence_key,confidence,first_seen_at,last_seen_at)
		SELECT 'delta-wrong',r.event_id,r.id,'wrong-fingerprint','wrong startup claim','material_update',r.source,r.evidence_key,r.confidence,'2026-07-31T00:00:00Z','2026-07-31T00:00:00Z'
		FROM semantic_event_reports r LIMIT 1`); err != nil {
		t.Fatal(err)
	}
	if err := state.backfillSemanticEventDeltas(ctx); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM semantic_event_deltas`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("backfilled deltas=%d err=%v", count, err)
	}
	var firstSeenAt, lastSeenAt, marker string
	if err := state.db.QueryRowContext(ctx, `SELECT first_seen_at,last_seen_at FROM semantic_event_deltas`).Scan(&firstSeenAt, &lastSeenAt); err != nil {
		t.Fatal(err)
	}
	if firstSeenAt != observedAt || lastSeenAt != observedAt {
		t.Fatalf("delta chronology first=%q last=%q want=%q", firstSeenAt, lastSeenAt, observedAt)
	}
	if err := state.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key='semantic_delta_backfill_version'`).Scan(&marker); err != nil || marker != semanticDeltaBackfillVersion {
		t.Fatalf("migration marker=%q err=%v", marker, err)
	}
	if err := state.backfillSemanticEventDeltas(ctx); err != nil {
		t.Fatal(err)
	}
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM semantic_event_deltas`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("idempotent deltas=%d err=%v", count, err)
	}
	if err := state.db.QueryRowContext(ctx, `SELECT first_seen_at,last_seen_at FROM semantic_event_deltas`).Scan(&firstSeenAt, &lastSeenAt); err != nil || firstSeenAt != observedAt || lastSeenAt != observedAt {
		t.Fatalf("idempotent chronology first=%q last=%q err=%v", firstSeenAt, lastSeenAt, err)
	}
}

func TestSemanticRetentionRemovesExpiredTerminalHistory(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	session, _ := insertSemanticTimelineFixture(t, state, "new_event")
	old := "2026-01-01T00:00:00Z"
	for _, statement := range []struct {
		query string
		args  []any
	}{
		{`UPDATE sessions SET completed_at=? WHERE id=?`, []any{old, session.ID}},
		{`UPDATE semantic_event_reports SET created_at=? WHERE session_id=?`, []any{old, session.ID}},
		{`UPDATE semantic_events SET first_seen_at=?,last_seen_at=?`, []any{old, old}},
	} {
		if _, err := state.db.ExecContext(ctx, statement.query, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	settings, _ := state.GetSettings(ctx)
	result, err := state.EnforceRetention(ctx, settings)
	if err != nil {
		t.Fatal(err)
	}
	if result.RemovedSessions != 1 || result.RemovedEvents != 1 {
		t.Fatalf("retention=%+v", result)
	}
	var sessions int
	_ = state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions WHERE id=?`, session.ID).Scan(&sessions)
	if sessions != 0 {
		t.Fatalf("expired session remains: %d", sessions)
	}
}

func TestEventResolutionDiagnosticsRoundTrip(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	settings, _ := state.GetSettings(ctx)
	session, err := createVisibleUpdateSession(state, ctx, "diagnostic fixture", settings)
	if err != nil {
		t.Fatal(err)
	}
	value := domain.EventResolutionSummary{
		SessionID:            session.ID,
		Status:               "completed",
		Provider:             "local-index",
		Model:                "none",
		Effort:               "none",
		CandidateCount:       3,
		HistoricalEventCount: 7,
		UniqueItems:          3,
		TriggerReason:        "weak_intra_check_overlap",
		StrongestOverlap:     2,
		TriggerTokens:        []string{"kimi", "model"},
		CreatedAt:            domain.Now(),
	}
	if err := state.SaveEventResolutionSummary(ctx, value); err != nil {
		t.Fatal(err)
	}
	loaded, err := state.EventResolutionSummary(ctx, session.ID)
	if err != nil || loaded == nil {
		t.Fatalf("loaded=%+v err=%v", loaded, err)
	}
	if loaded.ResolverInvoked || loaded.HistoricalEventCount != 7 || loaded.TriggerReason != value.TriggerReason || loaded.StrongestOverlap != 2 || len(loaded.TriggerTokens) != 2 {
		t.Fatalf("diagnostics=%+v", loaded)
	}
}
