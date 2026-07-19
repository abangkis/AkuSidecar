package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestInboxRunTraceDeduplicatesSnapshotsAndExplainsFinalOutcomes(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	settings, _ := state.GetSettings(ctx)
	session, err := state.CreateSession(ctx, "Inspect this flow", settings)
	if err != nil {
		t.Fatal(err)
	}
	run, err := state.AdvanceSession(ctx, session.ID)
	if err != nil || run == nil {
		t.Fatalf("next run: %+v %v", run, err)
	}
	command, err := state.StartRun(ctx, run.ID, map[string]any{"source": run.Source})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.ClaimCommand(ctx, run.ID, "trace-test"); err != nil {
		t.Fatal(err)
	}
	keys := []string{
		"x:000000000000000000000701",
		"x:000000000000000000000702",
		"x:000000000000000000000703",
		"x:000000000000000000000704",
	}
	blocks := make([]domain.Block, 0, len(keys))
	for index, key := range keys {
		blocks = append(blocks, domain.Block{
			EvidenceKey: key,
			Author:      fmt.Sprintf("Author %d", index+1),
			Text:        fmt.Sprintf("Captured candidate %d", index+1),
			Permalink:   fmt.Sprintf("https://x.com/example/status/70%d", index+1),
		})
	}
	observation := domain.Observation{
		Source:     run.Source,
		CapturedAt: domain.Now(),
		Snapshots: []domain.Snapshot{
			{Blocks: blocks},
			{Blocks: []domain.Block{{EvidenceKey: keys[3], Author: "Author 4", Text: "Captured candidate 4, repeated snapshot"}}},
		},
		Coverage: map[string]any{"status": "complete"},
	}
	if err := state.SaveObservation(ctx, command.ID, run.ID, observation); err != nil {
		t.Fatal(err)
	}

	for index, key := range keys[1:] {
		assessment := domain.CandidateAssessment{EvidenceKey: key, Rationale: fmt.Sprintf("Assessment rationale %d", index+2)}
		assessmentRaw, _ := json.Marshal(assessment)
		selected := 0
		if index > 0 {
			selected = 1
		}
		if _, err := state.db.ExecContext(ctx, `INSERT INTO candidate_assessments(run_id,evidence_key,source,assessment_json,base_score,preference_score,final_score,selected,created_at) VALUES(?,?,?,?,?,?,?,?,?)`, run.ID, key, run.Source, string(assessmentRaw), .5, 0, .5, selected, domain.Now()); err != nil {
			t.Fatal(err)
		}
	}
	for index, key := range keys[2:] {
		item := domain.ReasonedItem{EvidenceKey: key, Source: run.Source, Author: fmt.Sprintf("Author %d", index+3), WhatChanged: fmt.Sprintf("Reasoned item %d", index+3), SourceURL: fmt.Sprintf("https://x.com/example/status/70%d", index+3)}
		itemRaw, _ := json.Marshal(item)
		assessmentRaw, _ := json.Marshal(domain.CandidateAssessment{EvidenceKey: key, Rationale: fmt.Sprintf("Assessment rationale %d", index+3)})
		if _, err := state.db.ExecContext(ctx, `INSERT INTO timeline_items(id,session_id,run_id,source,evidence_key,rank,item_json,assessment_json,coverage_json,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, fmt.Sprintf("timeline-trace-%d", index), session.ID, run.ID, run.Source, key, index, string(itemRaw), string(assessmentRaw), "{}", domain.Now()); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := state.db.ExecContext(ctx, `INSERT INTO semantic_events(id,canonical_claim,first_seen_at,last_seen_at) VALUES('event-trace','A specific event',?,?)`, domain.Now(), domain.Now()); err != nil {
		t.Fatal(err)
	}
	for index, relation := range []string{"new_event", "duplicate_report"} {
		reason := ""
		if relation == "duplicate_report" {
			reason = "Same specific event as an earlier report."
		}
		if _, err := state.db.ExecContext(ctx, `INSERT INTO semantic_event_reports(id,event_id,timeline_id,session_id,run_id,evidence_key,source,relation,confidence,reason,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, fmt.Sprintf("report-trace-%d", index), "event-trace", fmt.Sprintf("timeline-trace-%d", index), session.ID, run.ID, keys[index+2], run.Source, relation, .95, reason, domain.Now()); err != nil {
			t.Fatal(err)
		}
	}

	trace, err := state.InboxRunTrace(ctx, run.ID, "captured", 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	if trace.Counts != (domain.InboxFlowCounts{Captured: 4, Evaluated: 3, Selected: 2, Added: 1}) {
		t.Fatalf("counts=%+v", trace.Counts)
	}
	if trace.Total != 4 || len(trace.Items) != 2 || trace.Items[0].Outcome != "captured_only" || trace.Items[1].Outcome != "not_selected" {
		t.Fatalf("captured trace=%+v", trace)
	}
	publicTrace, _ := json.Marshal(trace)
	if strings.Contains(string(publicTrace), "evidenceKey") || strings.Contains(string(publicTrace), keys[0]) {
		t.Fatalf("trace API leaked internal evidence identity: %s", publicTrace)
	}
	selected, err := state.InboxRunTrace(ctx, run.ID, "selected", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if selected.Total != 2 || selected.Items[0].Outcome != "added" || selected.Items[1].Outcome != "collapsed_duplicate" || selected.Items[1].Reason != "Same specific event as an earlier report." {
		t.Fatalf("selected trace=%+v", selected)
	}
	if _, err := state.db.ExecContext(ctx, `UPDATE semantic_event_reports SET reason='Exact native source post was already captured.' WHERE timeline_id='timeline-trace-1'`); err != nil {
		t.Fatal(err)
	}
	selected, err = state.InboxRunTrace(ctx, run.ID, "selected", 10, 0)
	if err != nil || selected.Items[1].Outcome != "exact_replay" {
		t.Fatalf("exact replay trace=%+v err=%v", selected, err)
	}
	added, err := state.InboxRunTrace(ctx, run.ID, "added", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if added.Total != 1 || len(added.Items) != 1 || added.Items[0].EvidenceKey != keys[2] {
		t.Fatalf("added trace=%+v", added)
	}

	inbox, _, err := state.ListInboxSessions(ctx, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if inbox[0].Runs[0].AddedItems != 1 {
		t.Fatalf("run additions must exclude semantic duplicate reports: %+v", inbox[0].Runs[0])
	}
}

func TestInboxRunTraceUsesTheSameCanonicalIdentityAsReasoning(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	settings, _ := state.GetSettings(ctx)
	session, err := state.CreateSession(ctx, "Canonical trace", settings)
	if err != nil {
		t.Fatal(err)
	}
	run, _ := state.AdvanceSession(ctx, session.ID)
	command, err := state.StartRun(ctx, run.ID, map[string]any{"source": run.Source})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.ClaimCommand(ctx, run.ID, "canonical-trace-test"); err != nil {
		t.Fatal(err)
	}
	text := "This sufficiently long LinkedIn-style post appears first without a stable permalink and later with its canonical native activity identity."
	stable := "x:000000000000000000000901"
	observation := domain.Observation{
		Source: run.Source, CapturedAt: domain.Now(), Coverage: map[string]any{"status": "complete"},
		Snapshots: []domain.Snapshot{
			{Blocks: []domain.Block{{EvidenceKey: "x:000000000000000000000900", Author: "Canonical Author", Text: text}}},
			{Blocks: []domain.Block{{EvidenceKey: stable, PlatformID: "901", Permalink: "https://x.com/example/status/901", Author: "Canonical Author", Text: text}}},
		},
	}
	if err := state.SaveObservation(ctx, command.ID, run.ID, observation); err != nil {
		t.Fatal(err)
	}
	assessment := domain.CandidateAssessment{EvidenceKey: stable, Rationale: "Canonical candidate was evaluated."}
	assessmentRaw, _ := json.Marshal(assessment)
	if _, err := state.db.ExecContext(ctx, `INSERT INTO candidate_assessments(run_id,evidence_key,source,assessment_json,base_score,preference_score,final_score,selected,created_at) VALUES(?,?,?,?,?,?,?,?,?)`, run.ID, stable, run.Source, string(assessmentRaw), .4, 0, .4, 0, domain.Now()); err != nil {
		t.Fatal(err)
	}
	trace, err := state.InboxRunTrace(ctx, run.ID, "captured", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if trace.Counts.Captured != 1 || trace.Counts.Evaluated != 1 || trace.Total != 1 || trace.Items[0].Outcome != "not_selected" {
		t.Fatalf("canonical trace=%+v", trace)
	}
	inbox, _, err := state.ListInboxSessions(ctx, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(inbox) != 1 {
		t.Fatalf("Inbox headline and canonical trace disagree: inbox=%+v trace=%+v", inbox, trace)
	}
	var inboxRun *domain.InboxRun
	for index := range inbox[0].Runs {
		if inbox[0].Runs[index].ID == run.ID {
			inboxRun = &inbox[0].Runs[index]
			break
		}
	}
	if inboxRun == nil || inboxRun.CapturedCandidates != trace.Counts.Captured {
		t.Fatalf("Inbox headline and canonical trace disagree: inbox=%+v trace=%+v", inbox, trace)
	}
}
