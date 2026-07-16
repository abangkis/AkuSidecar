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
	session, err := state.CreateSession(ctx, "semantic fixture", settings)
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

func TestSemanticCorrectionUndoRemovesConstraint(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	_, timelineID := insertSemanticTimelineFixture(t, state, "duplicate_report")
	correction, err := state.CorrectSemanticEvent(ctx, timelineID, "not_same_event", "")
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
	items, err = state.ListTimeline(ctx, 10, 0)
	if err != nil || len(items) != 1 || items[0].SemanticEvent.CorrectionID != "" {
		t.Fatalf("undone correction remains active: items=%+v err=%v", items, err)
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
