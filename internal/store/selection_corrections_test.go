package store

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestSelectionCorrectionRestoresEvaluatedCandidateAndIsUndoable(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	settings, _ := state.GetSettings(ctx)
	session, err := state.CreateSession(ctx, "Correction fixture", settings)
	if err != nil {
		t.Fatal(err)
	}
	runs, err := state.listRuns(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	run := runs[0]
	evidence := "x:000000000000000000000801"
	command, err := state.StartRun(ctx, run.ID, map[string]any{"source": run.Source})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.ClaimCommand(ctx, run.ID, "selection-correction-test"); err != nil {
		t.Fatal(err)
	}
	if err := state.SaveObservation(ctx, command.ID, run.ID, domain.Observation{
		Source: run.Source, CapturedAt: domain.Now(),
		Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{
			EvidenceKey: evidence, Author: "Example Author", Text: "A local AI release became available and provides enough durable source evidence for a user selection correction.", Permalink: "https://x.com/example/status/801",
		}}}}, Coverage: map[string]any{"status": "complete"},
	}); err != nil {
		t.Fatal(err)
	}
	assessment := domain.CandidateAssessment{
		EvidenceKey: evidence, TopicTags: []string{"local ai"}, TopicFacets: []string{"ai_models"},
		ContentType: "release", Novelty: .6, Materiality: .5, EvidenceStrength: .8,
		Rationale: "Useful to the user but below the original bounded selection line.",
	}
	item := domain.ReasonedItem{
		ID: evidence, EvidenceKey: evidence, Source: run.Source,
		WhatChanged: "A local AI release became available.", WhyItMatters: "It matches the user's workflow.",
		SourceURL: "https://x.com/example/status/801", SourceURLKind: "native_post",
		EventKey: "local-ai-release", KnowledgeDelta: "new_event", Author: "Example Author",
		Confidence: .8, EvidenceState: "primary",
	}
	assessmentRaw, _ := json.Marshal(assessment)
	itemRaw, _ := json.Marshal(item)
	if _, err := state.db.ExecContext(ctx, `
		INSERT INTO candidate_assessments(run_id,evidence_key,source,assessment_json,item_json,base_score,preference_score,final_score,selected,created_at)
		VALUES(?,?,?,?,?,?,?,?,?,?)`, run.ID, evidence, run.Source, string(assessmentRaw), string(itemRaw), .4, 0, .4, 0, domain.Now()); err != nil {
		t.Fatal(err)
	}

	correction, restored, err := state.CreateSelectionCorrection(ctx, run.ID, candidateRef(run.ID, evidence))
	if err != nil {
		t.Fatal(err)
	}
	if correction.TimelineID == "" || restored.ID != correction.TimelineID || restored.EvidenceKey != evidence {
		t.Fatalf("correction=%+v restored=%+v", correction, restored)
	}
	signals, err := state.PreferenceSignals(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(signals) != 1 || signals[0].Direction != "more" || signals[0].Origin != "selection_correction" {
		t.Fatalf("signals=%+v", signals)
	}
	trace, err := state.InboxRunTrace(ctx, run.ID, "selected", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if trace.Total != 1 || trace.Items[0].Outcome != "user_selected" || trace.Items[0].Correction == nil {
		t.Fatalf("trace=%+v", trace)
	}
	reason := "not_interested"
	if _, err := state.AddFeedback(ctx, restored.ID, domain.Feedback{Direction: "less", Reason: &reason}); err != nil {
		t.Fatal(err)
	}
	signals, err = state.PreferenceSignals(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(signals) != 1 || signals[0].Direction != "less" || signals[0].Origin != "routine" {
		t.Fatalf("later Less did not supersede selection correction: %+v", signals)
	}

	undone, err := state.UndoSelectionCorrection(ctx, correction.ID)
	if err != nil {
		t.Fatal(err)
	}
	if undone.UndoneAt == nil {
		t.Fatalf("undone=%+v", undone)
	}
	var retained int
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM timeline_items WHERE id=?`, restored.ID).Scan(&retained); err != nil {
		t.Fatal(err)
	}
	if retained != 0 {
		t.Fatalf("restored Timeline item survived undo")
	}
	signals, err = state.PreferenceSignals(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(signals) != 1 || signals[0].Direction != "less" || signals[0].Origin != "routine" {
		t.Fatalf("undo erased the later preference correction: %+v", signals)
	}
	trace, err = state.InboxRunTrace(ctx, run.ID, "evaluated", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if trace.Total != 1 || trace.Items[0].Outcome != "not_selected" || trace.Items[0].Correction != nil {
		t.Fatalf("trace after undo=%+v", trace)
	}

	secondCorrection, _, err := state.CreateSelectionCorrection(ctx, run.ID, candidateRef(run.ID, evidence))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.ExecContext(ctx, `
		UPDATE runs SET status='completed',stage='completed',completed_at=? WHERE session_id=?;
		UPDATE sessions SET status='completed',completed_at=? WHERE id=?`, domain.Now(), session.ID, domain.Now(), session.ID); err != nil {
		t.Fatal(err)
	}
	if err := state.ResetLearning(ctx); err != nil {
		t.Fatal(err)
	}
	signals, err = state.PreferenceSignals(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(signals) != 0 {
		t.Fatalf("learning reset retained old selection-correction authority: %+v", signals)
	}
	trace, err = state.InboxRunTrace(ctx, run.ID, "selected", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if trace.Total != 1 || trace.Items[0].Outcome != "user_selected" {
		t.Fatalf("learning reset rewrote the historical correction: %+v", trace)
	}
	if _, err := state.UndoSelectionCorrection(ctx, secondCorrection.ID); err != nil {
		t.Fatal(err)
	}
}
