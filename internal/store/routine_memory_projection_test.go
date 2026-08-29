package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

type routineMemoryTimelineFixture struct {
	Session domain.Session
	Run     domain.Run
	Item    domain.TimelineItem
}

func createRoutineMemoryTimelineFixture(t *testing.T, state *Store, ctx context.Context, sessionStatus string, evidenceKey string) routineMemoryTimelineFixture {
	t.Helper()
	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	session, err := createVisibleUpdateSession(state, ctx, "routine memory fixture", settings)
	if err != nil {
		t.Fatal(err)
	}
	runs, err := state.listRuns(ctx, session.ID)
	if err != nil || len(runs) == 0 {
		t.Fatalf("runs=%+v err=%v", runs, err)
	}
	run := runs[0]
	command, err := state.StartRun(ctx, run.ID, map[string]any{"source": run.Source})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.ClaimCommand(ctx, run.ID, "routine-memory-test"); err != nil {
		t.Fatal(err)
	}
	publishedAt := "2026-08-29T10:00:00Z"
	statusID := strings.TrimPrefix(evidenceKey, "x:routine-memory:")
	block := domain.Block{
		EvidenceKey: evidenceKey,
		PlatformID:  statusID,
		Author:      "Example Author",
		Text:        "A bounded source body retained only as recall metadata for " + statusID + ".",
		Permalink:   "https://x.com/example/status/" + statusID + "?utm_source=feed",
		PublishedAt: &publishedAt,
		Media: []map[string]any{{
			"kind": "image", "url": "https://pbs.twimg.com/media/example-" + statusID + ".jpg",
			"altText": "bounded preview",
		}},
	}
	if err := state.SaveObservation(ctx, command.ID, run.ID, domain.Observation{
		Source: run.Source, CapturedAt: publishedAt,
		Snapshots: []domain.Snapshot{{Blocks: []domain.Block{block}}},
		Coverage:  map[string]any{"status": "complete"},
	}); err != nil {
		t.Fatal(err)
	}
	assessment := domain.CandidateAssessment{
		EvidenceKey: evidenceKey, TopicTags: []string{"local ai", "memory"},
		TopicFacets: []string{"ai_models"},
	}
	item := domain.ReasonedItem{
		ID: evidenceKey, EvidenceKey: evidenceKey, Source: run.Source,
		WhatChanged: "A bounded source update.", WhyItMatters: "It is useful to the user's workflow.",
		SourceURL: "https://x.com/example/status/" + statusID, Author: "Example Author",
		PublishedAt: &publishedAt,
	}
	itemRaw, _ := json.Marshal(item)
	assessmentRaw, _ := json.Marshal(assessment)
	if _, err := state.db.ExecContext(ctx, `
		INSERT INTO candidate_assessments(run_id,evidence_key,source,assessment_json,item_json,base_score,preference_score,final_score,selected,created_at)
		VALUES(?,?,?,?,?,?,?,?,?,?)`, run.ID, evidenceKey, run.Source, string(assessmentRaw), string(itemRaw), .8, 0, .8, 1, publishedAt); err != nil {
		t.Fatal(err)
	}
	timelineID := "timeline-routine-memory-" + evidenceKey
	if _, err := state.db.ExecContext(ctx, `
		INSERT INTO timeline_items(id,session_id,run_id,source,evidence_key,rank,item_json,assessment_json,coverage_json,created_at)
		VALUES(?,?,?,?,?,?,?,?,?,?)`, timelineID, session.ID, run.ID, run.Source, evidenceKey, 0, string(itemRaw), string(assessmentRaw), "{}", publishedAt); err != nil {
		t.Fatal(err)
	}
	if sessionStatus == "completed" || sessionStatus == "partial" {
		if _, err := state.db.ExecContext(ctx, `UPDATE runs SET status='completed',stage='completed',completed_at=? WHERE id=?`, publishedAt, run.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := state.db.ExecContext(ctx, `UPDATE sessions SET status=?,completed_at=? WHERE id=?`, sessionStatus, publishedAt, session.ID); err != nil {
			t.Fatal(err)
		}
	}
	loaded, err := state.TimelineItem(ctx, timelineID)
	if err != nil {
		t.Fatal(err)
	}
	return routineMemoryTimelineFixture{Session: session, Run: run, Item: loaded}
}

func TestRoutineMoreProjectsFinalTimelineSurvivorAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	fixture := createRoutineMemoryTimelineFixture(t, state, ctx, "completed", "x:routine-memory:1001")

	first, err := state.AddFeedback(ctx, fixture.Item.ID, domain.Feedback{Direction: "more"})
	if err != nil {
		t.Fatal(err)
	}
	if first.Origin != "routine" {
		t.Fatalf("feedback origin=%q", first.Origin)
	}
	items, err := state.ListMemoryItems(ctx, false, 10)
	if err != nil || len(items) != 1 {
		t.Fatalf("memory items=%+v err=%v", items, err)
	}
	memory := items[0]
	if memory.RetentionTier != domain.MemoryTierRecall || memory.FullContent != nil || memory.Title != "A bounded source update." || memory.Summary != "It is useful to the user's workflow." {
		t.Fatalf("memory projection=%+v", memory)
	}
	if memory.CanonicalEvidenceKey != fixture.Item.EvidenceKey || memory.CanonicalPermalink != "https://x.com/example/status/1001" || memory.Author != "Example Author" || len(memory.Media) != 1 {
		t.Fatalf("memory identity/metadata=%+v", memory)
	}
	if memory.Media[0].URL != "https://pbs.twimg.com/media/example-1001.jpg" || memory.Media[0].AltText != "bounded preview" {
		t.Fatalf("memory media=%+v", memory.Media)
	}
	var provenance, actions, aliases int
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_provenance WHERE memory_item_id=?`, memory.ID).Scan(&provenance); err != nil {
		t.Fatal(err)
	}
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_actions WHERE memory_item_id=?`, memory.ID).Scan(&actions); err != nil {
		t.Fatal(err)
	}
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_identity_aliases WHERE memory_item_id=?`, memory.ID).Scan(&aliases); err != nil {
		t.Fatal(err)
	}
	if provenance != 1 || actions != 1 || aliases < 3 {
		t.Fatalf("initial audit provenance=%d actions=%d aliases=%d", provenance, actions, aliases)
	}

	repeat, err := state.AddFeedback(ctx, fixture.Item.ID, domain.Feedback{Direction: "more"})
	if err != nil {
		t.Fatal(err)
	}
	items, err = state.ListMemoryItems(ctx, false, 10)
	if err != nil || len(items) != 1 || items[0].ID != memory.ID {
		t.Fatalf("repeat memory items=%+v err=%v", items, err)
	}
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_provenance WHERE memory_item_id=?`, memory.ID).Scan(&provenance); err != nil {
		t.Fatal(err)
	}
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_actions WHERE memory_item_id=?`, memory.ID).Scan(&actions); err != nil {
		t.Fatal(err)
	}
	if provenance != 2 || actions != 2 {
		t.Fatalf("repeat audit provenance=%d actions=%d", provenance, actions)
	}

	reason := "not_interested"
	less, err := state.AddFeedback(ctx, fixture.Item.ID, domain.Feedback{Direction: "less", Reason: &reason})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.ExecContext(ctx, `UPDATE feedback_events SET created_at=CASE id WHEN ? THEN '2026-08-29T10:00:00.000000001Z' WHEN ? THEN '2026-08-29T10:00:00.000000002Z' WHEN ? THEN '2026-08-29T10:00:00.000000003Z' END WHERE id IN (?,?,?)`, first.ID, repeat.ID, less.ID, first.ID, repeat.ID, less.ID); err != nil {
		t.Fatal(err)
	}
	retained, err := state.MemoryItem(ctx, memory.ID)
	if err != nil || retained.LifecycleState != domain.MemoryStateActive {
		t.Fatalf("Less removed memory=%+v err=%v", retained, err)
	}
	signals, err := state.PreferenceSignals(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(signals) != 1 || signals[0].Direction != "less" || signals[0].Origin != "routine" {
		t.Fatalf("preference authority=%+v", signals)
	}
}

func TestRoutineMoreSkipsNonFinalButIgnoresClientOrigin(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	notFinal := createRoutineMemoryTimelineFixture(t, state, ctx, "running", "x:routine-memory:1002")
	if _, err := state.AddFeedback(ctx, notFinal.Item.ID, domain.Feedback{Direction: "more"}); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_items`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("non-final item projected memory count=%d", count)
	}
	if _, err := state.db.ExecContext(ctx, `UPDATE runs SET status='completed',stage='completed',completed_at=? WHERE id=?`, domain.Now(), notFinal.Run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.ExecContext(ctx, `UPDATE sessions SET status='completed',completed_at=? WHERE id=?`, domain.Now(), notFinal.Session.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := state.AddFeedback(ctx, notFinal.Item.ID, domain.Feedback{Direction: "more"}); err != nil {
		t.Fatal(err)
	}
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_items`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("final survivor was not projected count=%d", count)
	}
	nonRoutine := createRoutineMemoryTimelineFixture(t, state, ctx, "completed", "x:routine-memory:1003")
	if _, err := state.AddFeedback(ctx, nonRoutine.Item.ID, domain.Feedback{Direction: "more", Origin: "calibration"}); err != nil {
		t.Fatal(err)
	}
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_items`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("client origin suppressed authoritative routine projection count=%d", count)
	}
	if _, err := state.AddFeedback(ctx, nonRoutine.Item.ID, domain.Feedback{Direction: "less", Reason: stringPointer("not_interested")}); err != nil {
		t.Fatal(err)
	}
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_items`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("Less changed memory count=%d", count)
	}
}

func TestRoutineMoreProjectionRollsBackAndRecovers(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	fixture := createRoutineMemoryTimelineFixture(t, state, ctx, "completed", "x:routine-memory:1004")
	if _, err := state.db.ExecContext(ctx, `
		CREATE TRIGGER routine_memory_projection_failure
		BEFORE INSERT ON memory_items
		BEGIN SELECT RAISE(ABORT, 'injected memory projection failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := state.AddFeedback(ctx, fixture.Item.ID, domain.Feedback{Direction: "more"}); err == nil {
		t.Fatal("injected projection failure unexpectedly succeeded")
	}
	var feedbacks, memories int
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM feedback_events WHERE timeline_id=?`, fixture.Item.ID).Scan(&feedbacks); err != nil {
		t.Fatal(err)
	}
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_items`).Scan(&memories); err != nil {
		t.Fatal(err)
	}
	if feedbacks != 0 || memories != 0 {
		t.Fatalf("failed projection left feedbacks=%d memories=%d", feedbacks, memories)
	}
	if _, err := state.db.ExecContext(ctx, `DROP TRIGGER routine_memory_projection_failure`); err != nil {
		t.Fatal(err)
	}
	if _, err := state.AddFeedback(ctx, fixture.Item.ID, domain.Feedback{Direction: "more"}); err != nil {
		t.Fatal(err)
	}
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM feedback_events WHERE timeline_id=?`, fixture.Item.ID).Scan(&feedbacks); err != nil {
		t.Fatal(err)
	}
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_items`).Scan(&memories); err != nil {
		t.Fatal(err)
	}
	if feedbacks != 1 || memories != 1 {
		t.Fatalf("recovery feedbacks=%d memories=%d", feedbacks, memories)
	}
}

func TestRoutineMoreRejectsDeletedSurvivorWithoutCreatingMemory(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	fixture := createRoutineMemoryTimelineFixture(t, state, ctx, "completed", "x:routine-memory:1005")
	if _, err := state.db.ExecContext(ctx, `DELETE FROM timeline_items WHERE id=?`, fixture.Item.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := state.AddFeedback(ctx, fixture.Item.ID, domain.Feedback{Direction: "more"}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("deleted survivor err=%v", err)
	}
	var memories int
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_items`).Scan(&memories); err != nil {
		t.Fatal(err)
	}
	if memories != 0 {
		t.Fatalf("deleted survivor projected memory count=%d", memories)
	}
}

func TestRoutineMoreMemorySurvivesOperationalRetention(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	retained := createRoutineMemoryTimelineFixture(t, state, ctx, "completed", "x:routine-memory:1006")
	if _, err := state.AddFeedback(ctx, retained.Item.ID, domain.Feedback{Direction: "more"}); err != nil {
		t.Fatal(err)
	}
	memoryItems, err := state.ListMemoryItems(ctx, false, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(memoryItems) != 1 {
		t.Fatalf("memory items=%+v", memoryItems)
	}
	memoryID := memoryItems[0].ID
	other := createRoutineMemoryTimelineFixture(t, state, ctx, "completed", "x:routine-memory:1007")
	if _, err := state.db.ExecContext(ctx, `UPDATE sessions SET completed_at='2000-01-01T00:00:00Z' WHERE id=?`, retained.Session.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.ExecContext(ctx, `UPDATE sessions SET completed_at='2000-01-02T00:00:00Z' WHERE id=?`, other.Session.ID); err != nil {
		t.Fatal(err)
	}
	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	result, err := state.EnforceRetention(ctx, settings)
	if err != nil {
		t.Fatal(err)
	}
	if result.RemovedSessions == 0 {
		t.Fatalf("retention did not remove the old operational session: %+v", result)
	}
	if _, err := state.MemoryItem(ctx, memoryID); err != nil {
		t.Fatalf("retention removed personal memory: %v", err)
	}
}

func stringPointer(value string) *string { return &value }
