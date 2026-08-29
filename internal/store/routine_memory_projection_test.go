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
	if _, err := state.MemoryItem(ctx, memory.ID); !errors.Is(err, ErrMemoryNotFound) {
		t.Fatalf("Less did not retract the routine More stub: %v", err)
	}
	items, err = state.ListMemoryItems(ctx, false, 10)
	if err != nil || len(items) != 0 {
		t.Fatalf("retracted memory remains listed: items=%+v err=%v", items, err)
	}
	var tombstones int
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_tombstone_aliases`).Scan(&tombstones); err != nil {
		t.Fatal(err)
	}
	if tombstones != 0 {
		t.Fatalf("routine Less created permanent tombstones=%d", tombstones)
	}
	signals, err := state.PreferenceSignals(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(signals) != 1 || signals[0].Direction != "less" || signals[0].Origin != "routine" {
		t.Fatalf("preference authority=%+v", signals)
	}
	if _, err := state.AddFeedback(ctx, fixture.Item.ID, domain.Feedback{Direction: "more"}); err != nil {
		t.Fatal(err)
	}
	items, err = state.ListMemoryItems(ctx, false, 10)
	if err != nil || len(items) != 1 || items[0].ID == memory.ID {
		t.Fatalf("later More did not recreate a fresh recall stub: items=%+v err=%v", items, err)
	}
}

func TestRoutineLessPreservesFullCopyAndIndependentMemory(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	fullCopyFixture := createRoutineMemoryTimelineFixture(t, state, ctx, "completed", "x:routine-memory:1010")
	if _, err := state.AddFeedback(ctx, fullCopyFixture.Item.ID, domain.Feedback{Direction: "more"}); err != nil {
		t.Fatal(err)
	}
	items, err := state.ListMemoryItems(ctx, false, 10)
	if err != nil || len(items) != 1 {
		t.Fatalf("full-copy fixture memories=%+v err=%v", items, err)
	}
	fullCopyID := items[0].ID
	if _, err := state.KeepMemoryFullCopy(ctx, fullCopyID, domain.MemoryFullCopyInput{Content: "keep this full copy"}); err != nil {
		t.Fatal(err)
	}
	if _, err := state.AddFeedback(ctx, fullCopyFixture.Item.ID, domain.Feedback{Direction: "less", Reason: stringPointer("not_interested")}); err != nil {
		t.Fatal(err)
	}
	kept, err := state.MemoryItem(ctx, fullCopyID)
	if err != nil || kept.LifecycleState != domain.MemoryStateActive || kept.RetentionTier != domain.MemoryTierFullCopy || kept.FullContent == nil || *kept.FullContent != "keep this full copy" {
		t.Fatalf("Less removed full copy=%+v err=%v", kept, err)
	}
	var provenance int
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_provenance WHERE memory_item_id=?`, fullCopyID).Scan(&provenance); err != nil {
		t.Fatal(err)
	}
	if provenance != 0 {
		t.Fatalf("Less left routine More provenance on full copy=%d", provenance)
	}

	independentFixture := createRoutineMemoryTimelineFixture(t, state, ctx, "completed", "x:routine-memory:1011")
	if _, err := state.AddFeedback(ctx, independentFixture.Item.ID, domain.Feedback{Direction: "more"}); err != nil {
		t.Fatal(err)
	}
	items, err = state.ListMemoryItems(ctx, false, 10)
	if err != nil || len(items) != 2 {
		t.Fatalf("independent fixture memories=%+v err=%v", items, err)
	}
	var independentID string
	for _, item := range items {
		if item.CanonicalEvidenceKey == independentFixture.Item.EvidenceKey {
			independentID = item.ID
		}
	}
	if independentID == "" {
		t.Fatalf("independent fixture memory not found: %+v", items)
	}
	if _, err := state.RecordMemoryProvenance(ctx, domain.MemoryProvenance{
		MemoryItemID: independentID, ProvenanceKind: "captured", Source: domain.SourceX,
		CanonicalEvidenceKey: independentFixture.Item.EvidenceKey, SourceURL: independentFixture.Item.Item.SourceURL,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := state.AddFeedback(ctx, independentFixture.Item.ID, domain.Feedback{Direction: "less", Reason: stringPointer("not_interested")}); err != nil {
		t.Fatal(err)
	}
	retained, err := state.MemoryItem(ctx, independentID)
	if err != nil || retained.LifecycleState != domain.MemoryStateActive || retained.RetentionTier != domain.MemoryTierRecall {
		t.Fatalf("Less removed independently retained memory=%+v err=%v", retained, err)
	}
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_provenance WHERE memory_item_id=?`, independentID).Scan(&provenance); err != nil {
		t.Fatal(err)
	}
	if provenance != 1 {
		t.Fatalf("Less removed independent provenance=%d", provenance)
	}
}

func TestRoutineLessRetractionTracksSharedTimelineProvenance(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	first := createRoutineMemoryTimelineFixture(t, state, ctx, "completed", "x:routine-memory:1020")
	second := createRoutineMemoryTimelineFixture(t, state, ctx, "completed", "x:routine-memory:1021")

	// Make two distinct Timeline rows resolve to one canonical memory without
	// changing their Timeline identity. This mirrors a source record that is
	// surfaced in more than one final update.
	itemRaw, err := json.Marshal(first.Item.Item)
	if err != nil {
		t.Fatal(err)
	}
	assessmentRaw, err := json.Marshal(first.Item.Assessment)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.ExecContext(ctx, `
		UPDATE timeline_items
		SET evidence_key=?,item_json=?,assessment_json=?
		WHERE id=?`, first.Item.EvidenceKey, string(itemRaw), string(assessmentRaw), second.Item.ID); err != nil {
		t.Fatal(err)
	}
	second.Item, err = state.TimelineItem(ctx, second.Item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if second.Item.EvidenceKey != first.Item.EvidenceKey {
		t.Fatalf("shared Timeline evidence key=%q want %q", second.Item.EvidenceKey, first.Item.EvidenceKey)
	}

	if _, err := state.AddFeedback(ctx, first.Item.ID, domain.Feedback{Direction: "more"}); err != nil {
		t.Fatal(err)
	}
	items, err := state.ListMemoryItems(ctx, false, 10)
	if err != nil || len(items) != 1 {
		t.Fatalf("first shared memory projection=%+v err=%v", items, err)
	}
	memoryID := items[0].ID
	if _, err := state.AddFeedback(ctx, second.Item.ID, domain.Feedback{Direction: "more"}); err != nil {
		t.Fatal(err)
	}
	items, err = state.ListMemoryItems(ctx, false, 10)
	if err != nil || len(items) != 1 || items[0].ID != memoryID {
		t.Fatalf("shared memory was not reused=%+v err=%v", items, err)
	}
	var provenance int
	if err := state.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM memory_provenance
		WHERE memory_item_id=? AND provenance_kind='explicit_feedback' AND reason='routine_more'`, memoryID).Scan(&provenance); err != nil {
		t.Fatal(err)
	}
	if provenance != 2 {
		t.Fatalf("shared routine More provenance=%d want 2", provenance)
	}

	if _, err := state.AddFeedback(ctx, first.Item.ID, domain.Feedback{Direction: "less", Reason: stringPointer("not_interested")}); err != nil {
		t.Fatal(err)
	}
	if _, err := state.MemoryItem(ctx, memoryID); err != nil {
		t.Fatalf("Less on first Timeline removed shared memory: %v", err)
	}
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_provenance WHERE memory_item_id=?`, memoryID).Scan(&provenance); err != nil {
		t.Fatal(err)
	}
	if provenance != 1 {
		t.Fatalf("first Less removed wrong provenance count=%d want 1", provenance)
	}
	var remainingTimeline string
	if err := state.db.QueryRowContext(ctx, `
		SELECT json_extract(capture_context_json,'$.timelineId')
		FROM memory_provenance WHERE memory_item_id=? LIMIT 1`, memoryID).Scan(&remainingTimeline); err != nil {
		t.Fatal(err)
	}
	if remainingTimeline != second.Item.ID {
		t.Fatalf("remaining provenance belongs to %q want %q", remainingTimeline, second.Item.ID)
	}

	if _, err := state.AddFeedback(ctx, second.Item.ID, domain.Feedback{Direction: "less", Reason: stringPointer("not_interested")}); err != nil {
		t.Fatal(err)
	}
	if _, err := state.MemoryItem(ctx, memoryID); !errors.Is(err, ErrMemoryNotFound) {
		t.Fatalf("Less on second Timeline did not remove final shared stub: %v", err)
	}
	items, err = state.ListMemoryItems(ctx, false, 10)
	if err != nil || len(items) != 0 {
		t.Fatalf("shared memory remains listed after both Less actions=%+v err=%v", items, err)
	}
	var tombstones int
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_tombstone_aliases`).Scan(&tombstones); err != nil {
		t.Fatal(err)
	}
	if tombstones != 0 {
		t.Fatalf("shared routine Less created permanent tombstones=%d", tombstones)
	}

	if _, err := state.AddFeedback(ctx, first.Item.ID, domain.Feedback{Direction: "more"}); err != nil {
		t.Fatal(err)
	}
	items, err = state.ListMemoryItems(ctx, false, 10)
	if err != nil || len(items) != 1 || items[0].ID == memoryID {
		t.Fatalf("later More did not recreate shared recall stub=%+v err=%v", items, err)
	}
}

func TestTimelineKeepFullCopyUsesAuthoritativeEvidenceAndReleaseRestoresRecall(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	fixture := createRoutineMemoryTimelineFixture(t, state, ctx, "completed", "x:routine-memory:1030")

	kept, alreadyKept, err := state.KeepTimelineFullCopy(ctx, fixture.Item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if alreadyKept || kept.RetentionTier != domain.MemoryTierFullCopy || kept.FullContent == nil || *kept.FullContent != fixture.Item.Evidence.Text {
		t.Fatalf("authoritative Keep=%+v already=%v", kept, alreadyKept)
	}
	var feedbacks, keepActions, manualProvenance int
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM feedback_events WHERE timeline_id=?`, fixture.Item.ID).Scan(&feedbacks); err != nil {
		t.Fatal(err)
	}
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_actions WHERE memory_item_id=? AND action='keep_full_copy'`, kept.ID).Scan(&keepActions); err != nil {
		t.Fatal(err)
	}
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_provenance WHERE memory_item_id=? AND provenance_kind='manual' AND reason='timeline_keep_full_copy'`, kept.ID).Scan(&manualProvenance); err != nil {
		t.Fatal(err)
	}
	if feedbacks != 0 || keepActions != 1 || manualProvenance != 1 {
		t.Fatalf("Keep wrote unexpected audit feedback=%d keepActions=%d manual=%d", feedbacks, keepActions, manualProvenance)
	}
	projected, err := state.TimelineItem(ctx, fixture.Item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if projected.PersonalMemory == nil || projected.PersonalMemory.RetentionTier != domain.MemoryTierFullCopy {
		t.Fatalf("Timeline did not restore full-copy projection=%+v", projected.PersonalMemory)
	}

	repeated, alreadyKept, err := state.KeepTimelineFullCopy(ctx, fixture.Item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !alreadyKept || repeated.ID != kept.ID || repeated.FullContent == nil || *repeated.FullContent != *kept.FullContent {
		t.Fatalf("idempotent Keep=%+v already=%v original=%+v", repeated, alreadyKept, kept)
	}
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_actions WHERE memory_item_id=? AND action='keep_full_copy'`, kept.ID).Scan(&keepActions); err != nil {
		t.Fatal(err)
	}
	if keepActions != 1 {
		t.Fatalf("idempotent Keep duplicated action count=%d", keepActions)
	}

	if _, err := state.AddFeedback(ctx, fixture.Item.ID, domain.Feedback{Direction: "less", Reason: stringPointer("not_interested")}); err != nil {
		t.Fatal(err)
	}
	lessKept, err := state.MemoryItem(ctx, kept.ID)
	if err != nil || lessKept.RetentionTier != domain.MemoryTierFullCopy || lessKept.FullContent == nil {
		t.Fatalf("Less downgraded explicitly kept copy=%+v err=%v", lessKept, err)
	}

	released, err := state.ReleaseMemoryFullCopy(ctx, kept.ID)
	if err != nil {
		t.Fatal(err)
	}
	if released.RetentionTier != domain.MemoryTierRecall || released.FullContent != nil || released.ContentBytes != 0 {
		t.Fatalf("Release did not downgrade full copy=%+v", released)
	}
	projected, err = state.TimelineItem(ctx, fixture.Item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if projected.PersonalMemory == nil || projected.PersonalMemory.RetentionTier != domain.MemoryTierRecall {
		t.Fatalf("Timeline did not restore released recall projection=%+v", projected.PersonalMemory)
	}

	if err := state.RemoveMemory(ctx, kept.ID); err != nil {
		t.Fatal(err)
	}
	recreated, alreadyKept, err := state.KeepTimelineFullCopy(ctx, fixture.Item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if alreadyKept || recreated.ID == kept.ID || recreated.RetentionTier != domain.MemoryTierFullCopy {
		t.Fatalf("Keep after ordinary Remove=%+v already=%v previous=%s", recreated, alreadyKept, kept.ID)
	}
}

func TestTimelineKeepFullCopyRejectsNonFinalBlankAndForgottenEvidence(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	nonFinal := createRoutineMemoryTimelineFixture(t, state, ctx, "running", "x:routine-memory:1040")
	if _, _, err := state.KeepTimelineFullCopy(ctx, nonFinal.Item.ID); !errors.Is(err, ErrTimelineMemoryNotEligible) {
		t.Fatalf("non-final Keep err=%v", err)
	}
	if items, err := state.ListMemoryItems(ctx, false, 10); err != nil || len(items) != 0 {
		t.Fatalf("non-final Keep created memory items=%+v err=%v", items, err)
	}
	if err := state.CancelSession(ctx, nonFinal.Session.ID); err != nil {
		t.Fatal(err)
	}

	blank := createRoutineMemoryTimelineFixture(t, state, ctx, "completed", "x:routine-memory:1041")
	var observationRaw string
	if err := state.db.QueryRowContext(ctx, `SELECT observation_json FROM observations WHERE run_id=? LIMIT 1`, blank.Item.RunID).Scan(&observationRaw); err != nil {
		t.Fatal(err)
	}
	var observation domain.Observation
	if err := json.Unmarshal([]byte(observationRaw), &observation); err != nil {
		t.Fatal(err)
	}
	observation.Snapshots[0].Blocks[0].Text = "   "
	updatedObservation, err := json.Marshal(observation)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.ExecContext(ctx, `UPDATE observations SET observation_json=? WHERE run_id=?`, string(updatedObservation), blank.Item.RunID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := state.KeepTimelineFullCopy(ctx, blank.Item.ID); !errors.Is(err, ErrTimelineMemoryTextUnavailable) {
		t.Fatalf("blank-text Keep err=%v", err)
	}

	forgotten := createRoutineMemoryTimelineFixture(t, state, ctx, "completed", "x:routine-memory:1042")
	if _, err := state.AddFeedback(ctx, forgotten.Item.ID, domain.Feedback{Direction: "more"}); err != nil {
		t.Fatal(err)
	}
	items, err := state.ListMemoryItems(ctx, false, 10)
	if err != nil || len(items) != 1 {
		t.Fatalf("forgotten fixture projection=%+v err=%v", items, err)
	}
	forgottenID := items[0].ID
	if _, err := state.ForgetMemory(ctx, forgottenID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := state.KeepTimelineFullCopy(ctx, forgotten.Item.ID); !errors.Is(err, ErrMemoryTombstoned) {
		t.Fatalf("forgotten Keep err=%v", err)
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
	if count != 1 {
		t.Fatalf("Less did not retract the routine More memory count=%d", count)
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
