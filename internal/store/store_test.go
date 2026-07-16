package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	settings := domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", true)
	value, err := Open(filepath.Join(t.TempDir(), "sidecar.db"), settings)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { value.Close() })
	return value
}

func TestTimelineBoundaryCueModePersists(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.TimelineBoundaryCueMode = "static"
	settings.TimelineBoundaryReturnMS = 650
	if err := state.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	stored, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stored.TimelineBoundaryCueMode != "static" || stored.TimelineBoundaryReturnMS != 650 {
		t.Fatalf("timeline boundary cue settings=%+v", stored)
	}
}

func TestFreshSchemaContainsOnlyNewTables(t *testing.T) {
	state := openTestStore(t)
	rows, err := state.db.Query(`SELECT name FROM sqlite_master WHERE type='table' ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	want := []string{"bridge_commands", "calibration_profile_snapshots", "calibration_samples", "calibration_sessions", "candidate_assessments", "event_resolution_diagnostics", "event_resolution_invocations", "feedback_events", "knowledge_events", "meta", "observations", "preference_model", "reasoning_invocations", "runs", "semantic_event_constraints", "semantic_event_corrections", "semantic_event_reports", "semantic_events", "sessions", "settings", "timeline_items"}
	if len(names) != len(want) {
		t.Fatalf("tables=%v", names)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("tables=%v", names)
		}
	}
}

func TestLatestTimelineCheckUsesLatestTerminalSessionEvenWithZeroAdditions(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	if latest, err := state.LatestTimelineCheck(ctx); err != nil || latest != nil {
		t.Fatalf("fresh latest=%+v err=%v", latest, err)
	}
	settings, _ := state.GetSettings(ctx)
	first, err := state.CreateSession(ctx, "first check", settings)
	if err != nil {
		t.Fatal(err)
	}
	firstRuns, _ := state.listRuns(ctx, first.ID)
	evidence := "x:000000000000000000000401"
	itemRaw, _ := json.Marshal(domain.ReasonedItem{EvidenceKey: evidence, Source: domain.SourceX})
	assessmentRaw, _ := json.Marshal(domain.CandidateAssessment{EvidenceKey: evidence})
	if _, err := state.db.ExecContext(ctx, `INSERT INTO timeline_items(id,session_id,run_id,source,evidence_key,rank,item_json,assessment_json,coverage_json,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, "timeline-first-check", first.ID, firstRuns[0].ID, domain.SourceX, evidence, 0, string(itemRaw), string(assessmentRaw), "{}", "2026-07-16T10:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.ExecContext(ctx, `UPDATE sessions SET status='completed',completed_at='2026-07-16T10:00:00Z' WHERE id=?`, first.ID); err != nil {
		t.Fatal(err)
	}
	second, err := state.CreateSession(ctx, "zero addition check", settings)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.ExecContext(ctx, `UPDATE sessions SET status='partial',completed_at='2026-07-16T11:00:00Z' WHERE id=?`, second.ID); err != nil {
		t.Fatal(err)
	}
	latest, err := state.LatestTimelineCheck(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if latest == nil || latest.SessionID != second.ID || latest.Status != "partial" || latest.CompletedAt != "2026-07-16T11:00:00Z" || latest.AddedItems != 0 {
		t.Fatalf("latest=%+v", latest)
	}
}

func TestCalibrationSessionReflectsSnapshotLiveInfluence(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	session, err := state.CreateSession(ctx, "calibration authority", settings)
	if err != nil {
		t.Fatal(err)
	}
	calibration, err := state.CreateCalibration(ctx, domain.CalibrationSession{
		ID: "calibration-live-influence", UnifiedSessionID: session.ID,
		TriggerKind: "first_run", MaxItems: 2,
		Samples: []domain.CalibrationSample{{
			RunID: session.Runs[0].ID, EvidenceKey: "x:calibration-live", Source: session.Runs[0].Source,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	completed, err := state.CompleteCalibration(ctx, calibration.ID, domain.CalibrationSnapshot{
		Version: 0, Origin: "calibration", CalibrationSessionID: calibration.ID,
		CreatedAt: domain.Now(), Labels: map[string]int{}, LiveInfluence: true,
		ActivationState: "feeds_local_fit",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !completed.LiveInfluence || completed.Snapshot == nil || !completed.Snapshot.LiveInfluence {
		t.Fatalf("completed calibration must expose snapshot influence: %+v", completed)
	}
}

func TestSessionCommandAndObservationLifecycle(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	settings, _ := state.GetSettings(ctx)
	session, err := state.CreateSession(ctx, "What changed?", settings)
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
	claimed, err := state.ClaimCommand(ctx, run.ID, "bridge-test")
	if err != nil || claimed == nil || claimed.ID != command.ID {
		t.Fatalf("claim: %+v %v", claimed, err)
	}
	observation := domain.Observation{Source: run.Source, PageURL: "https://example.test", CapturedAt: domain.Now(), Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{EvidenceKey: "x:000000000000000000000001", Text: "Material update"}}}}, Coverage: map[string]any{"status": "complete"}}
	if err := state.SaveObservation(ctx, command.ID, run.ID, observation); err != nil {
		t.Fatal(err)
	}
	stored, err := state.Observations(ctx, run.ID)
	if err != nil || len(stored) != 1 {
		t.Fatalf("observations: %d %v", len(stored), err)
	}
	updated, err := state.GetRun(ctx, run.ID)
	if err != nil || updated.Status != "reasoning" {
		t.Fatalf("run: %+v %v", updated, err)
	}
}

func TestSessionRemainsActiveUntilCompositionFinalizes(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	settings, _ := state.GetSettings(ctx)
	session, err := state.CreateSession(ctx, "terminal composition boundary", settings)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.ExecContext(ctx, `UPDATE runs SET status='completed',stage='completed',completed_at=? WHERE session_id=?`, domain.Now(), session.ID); err != nil {
		t.Fatal(err)
	}
	next, err := state.AdvanceSession(ctx, session.ID)
	if err != nil || next != nil {
		t.Fatalf("advance after terminal runs: next=%+v err=%v", next, err)
	}
	before, err := state.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if before.Status == "completed" || before.Status == "partial" || before.Status == "failed" {
		t.Fatalf("session became terminal before composition: %+v", before)
	}
	if err := state.ComposeSession(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
	if err := state.FinalizeSession(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
	after, err := state.GetSession(ctx, session.ID)
	if err != nil || after.Status != "completed" {
		t.Fatalf("finalized session=%+v err=%v", after, err)
	}
}

func TestTimelineIncludesCapturedSourceEvidence(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	settings, _ := state.GetSettings(ctx)
	session, err := state.CreateSession(ctx, "What changed?", settings)
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
	claimed, err := state.ClaimCommand(ctx, run.ID, "bridge-test")
	if err != nil || claimed == nil || claimed.ID != command.ID {
		t.Fatalf("claim: %+v %v", claimed, err)
	}
	evidenceKey := "x:000000000000000000000123"
	observation := domain.Observation{
		Source:     run.Source,
		PageURL:    "https://x.com/home",
		CapturedAt: domain.Now(),
		Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{
			EvidenceKey: evidenceKey,
			Author:      "AkuBrowser @akubrowser",
			Text:        "The original source-layout text.",
			Permalink:   "https://x.com/akubrowser/status/123",
		}}}},
		Coverage: map[string]any{"status": "complete"},
	}
	if err := state.SaveObservation(ctx, command.ID, run.ID, observation); err != nil {
		t.Fatal(err)
	}
	itemRaw, _ := json.Marshal(domain.ReasonedItem{ID: evidenceKey, EvidenceKey: evidenceKey, Source: run.Source, WhatChanged: "Changed"})
	assessmentRaw, _ := json.Marshal(domain.CandidateAssessment{EvidenceKey: evidenceKey})
	coverageRaw, _ := json.Marshal(map[string]any{"status": "complete"})
	_, err = state.db.ExecContext(ctx, `INSERT INTO timeline_items(id,session_id,run_id,source,evidence_key,rank,item_json,assessment_json,coverage_json,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, "timeline-test", session.ID, run.ID, run.Source, evidenceKey, 0, string(itemRaw), string(assessmentRaw), string(coverageRaw), domain.Now())
	if err != nil {
		t.Fatal(err)
	}

	items, err := state.ListTimeline(ctx, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Evidence == nil {
		t.Fatalf("timeline evidence=%+v", items)
	}
	if items[0].Evidence.Text != "The original source-layout text." || items[0].Evidence.Author != "AkuBrowser @akubrowser" {
		t.Fatalf("evidence=%+v", items[0].Evidence)
	}
	inbox, total, err := state.ListInboxSessions(ctx, 10, 0)
	if err != nil || total != 1 || len(inbox) != 1 || len(inbox[0].Runs) != 2 {
		t.Fatalf("inbox=%+v total=%d err=%v", inbox, total, err)
	}
	diagnostic := inbox[0].Runs[0]
	if diagnostic.CapturedCandidates != 1 || diagnostic.AcquisitionRounds != 1 || diagnostic.SnapshotCount != 1 || diagnostic.AddedItems != 1 {
		t.Fatalf("inbox diagnostic=%+v", diagnostic)
	}
}

func TestOnboardingAndFullResetStartFromFreshGoState(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	onboarding, err := state.Onboarding(ctx)
	if err != nil || onboarding.Status != "not_started" || onboarding.Profile != nil {
		t.Fatalf("fresh onboarding=%+v err=%v", onboarding, err)
	}
	token, err := state.BridgeToken(ctx)
	if err != nil {
		t.Fatal(err)
	}
	onboarding, err = state.CompleteOnboarding(ctx, []domain.Source{domain.SourceLinkedIn})
	if err != nil || onboarding.Status != "completed" || len(onboarding.Profile.ActiveSources) != 1 {
		t.Fatalf("completed onboarding=%+v err=%v", onboarding, err)
	}
	calibrationStatus, err := state.CalibrationFirstRunStatus(ctx)
	if err != nil || calibrationStatus != "pending" {
		t.Fatalf("calibration status=%q err=%v", calibrationStatus, err)
	}
	settings, _ := state.GetSettings(ctx)
	settings.LoadProfile = "custom"
	settings.MaxScrolls = 1
	settings.MaxItemsPerSource = 3
	settings.MaxItemsTotal = 6
	settings.TimelineCapacity = 7
	settings.DefaultPresentation = "brief"
	settings.StreamWidth = "wide"
	if err := state.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}

	defaults := domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", true)
	reset, err := state.FullReset(ctx, defaults)
	if err != nil {
		t.Fatal(err)
	}
	backupPath := filepath.Join(filepath.Dir(state.Path()), "backups", reset.BackupFile)
	if info, err := os.Stat(backupPath); err != nil || info.Size() == 0 {
		t.Fatalf("backup=%q info=%+v err=%v", backupPath, info, err)
	}
	onboarding, err = state.Onboarding(ctx)
	if err != nil || onboarding.Status != "not_started" || onboarding.Profile != nil {
		t.Fatalf("reset onboarding=%+v err=%v", onboarding, err)
	}
	after, err := state.GetSettings(ctx)
	if err != nil || after.LoadProfile != "expanded" || len(after.ActiveSources) != 2 || after.DefaultPresentation != "source" || after.StreamWidth != "social" {
		t.Fatalf("reset settings=%+v err=%v", after, err)
	}
	afterToken, err := state.BridgeToken(ctx)
	if err != nil || afterToken != token {
		t.Fatalf("bridge token changed: before=%q after=%q err=%v", token, afterToken, err)
	}
	calibrationStatus, err = state.CalibrationFirstRunStatus(ctx)
	if err != nil || calibrationStatus != "not_started" {
		t.Fatalf("reset calibration status=%q err=%v", calibrationStatus, err)
	}
}

func TestBridgeTokenComparison(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	token, err := state.BridgeToken(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !state.MatchesBridgeToken(ctx, token) || state.MatchesBridgeToken(ctx, token+"x") {
		t.Fatal("constant-time token boundary failed")
	}
}

func TestSessionCompositionUsesGlobalScoreWithSourceDiversity(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	settings, _ := state.GetSettings(ctx)
	session, err := state.CreateSession(ctx, "What changed?", settings)
	if err != nil {
		t.Fatal(err)
	}
	runs, err := state.listRuns(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	type fixture struct {
		run      domain.Run
		evidence string
		score    float64
	}
	fixtures := []fixture{
		{runs[0], "x:000000000000000000000101", .9},
		{runs[0], "x:000000000000000000000102", .8},
		{runs[0], "x:000000000000000000000103", .7},
		{runs[1], "linkedin:000000000000000000201", .6},
		{runs[1], "linkedin:000000000000000000202", .5},
	}
	for index, fixture := range fixtures {
		assessment := domain.CandidateAssessment{EvidenceKey: fixture.evidence}
		assessmentRaw, _ := json.Marshal(assessment)
		itemRaw, _ := json.Marshal(domain.ReasonedItem{EvidenceKey: fixture.evidence, Source: fixture.run.Source})
		if _, err := state.db.ExecContext(ctx, `INSERT INTO candidate_assessments(run_id,evidence_key,source,assessment_json,base_score,preference_score,final_score,selected,created_at) VALUES(?,?,?,?,?,?,?,?,?)`, fixture.run.ID, fixture.evidence, fixture.run.Source, string(assessmentRaw), fixture.score, 0, fixture.score, 1, domain.Now()); err != nil {
			t.Fatal(err)
		}
		if _, err := state.db.ExecContext(ctx, `INSERT INTO timeline_items(id,session_id,run_id,source,evidence_key,rank,item_json,assessment_json,coverage_json,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, fmt.Sprintf("timeline-%d", index), session.ID, fixture.run.ID, fixture.run.Source, fixture.evidence, index, string(itemRaw), string(assessmentRaw), "{}", domain.Now()); err != nil {
			t.Fatal(err)
		}
	}
	if err := state.ComposeSession(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
	items, err := state.ListSessionItems(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := []domain.Source{domain.SourceX, domain.SourceX, domain.SourceLinkedIn, domain.SourceX, domain.SourceLinkedIn}
	if len(items) != len(want) {
		t.Fatalf("items=%d", len(items))
	}
	for index, source := range want {
		if items[index].Source != source || items[index].Rank != index {
			t.Fatalf("items[%d]=%+v", index, items[index])
		}
	}
	if _, err := state.db.ExecContext(ctx, `UPDATE sessions SET status='completed',completed_at=? WHERE id=?; UPDATE timeline_items SET rank=0 WHERE session_id=?`, domain.Now(), session.ID, session.ID); err != nil {
		t.Fatal(err)
	}
	if err := state.RecomposeCompletedSessions(ctx); err != nil {
		t.Fatal(err)
	}
	items, err = state.ListSessionItems(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	for index, source := range want {
		if items[index].Source != source || items[index].Rank != index {
			t.Fatalf("recomposed items[%d]=%+v", index, items[index])
		}
	}
}

func TestPreviouslyDeliveredEvidenceIsSourceScoped(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	settings, _ := state.GetSettings(ctx)
	session, err := state.CreateSession(ctx, "What changed?", settings)
	if err != nil {
		t.Fatal(err)
	}
	runs, _ := state.listRuns(ctx, session.ID)
	evidence := "x:000000000000000000000301"
	itemRaw, _ := json.Marshal(domain.ReasonedItem{EvidenceKey: evidence, Source: domain.SourceX})
	assessmentRaw, _ := json.Marshal(domain.CandidateAssessment{EvidenceKey: evidence})
	if _, err := state.db.ExecContext(ctx, `INSERT INTO timeline_items(id,session_id,run_id,source,evidence_key,rank,item_json,assessment_json,coverage_json,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, "timeline-delivered", session.ID, runs[0].ID, domain.SourceX, evidence, 0, string(itemRaw), string(assessmentRaw), "{}", domain.Now()); err != nil {
		t.Fatal(err)
	}
	known, err := state.PreviouslyDeliveredEvidence(ctx, domain.SourceX, []string{evidence, "x:missing"})
	if err != nil || !known[evidence] || known["x:missing"] {
		t.Fatalf("known=%v err=%v", known, err)
	}
	other, err := state.PreviouslyDeliveredEvidence(ctx, domain.SourceLinkedIn, []string{evidence})
	if err != nil || other[evidence] {
		t.Fatalf("other=%v err=%v", other, err)
	}
	if _, err := state.db.ExecContext(ctx, `INSERT INTO knowledge_events(id,source,event_key,evidence_key,item_json,first_seen_at,last_seen_at) VALUES(?,?,?,?,?,?,?)`, "knowledge-test", domain.SourceX, "event-test", evidence, string(itemRaw), domain.Now(), domain.Now()); err != nil {
		t.Fatal(err)
	}
	events, err := state.PreviouslyKnownEvents(ctx, domain.SourceX, []string{"event-test", "event-missing"})
	if err != nil || !events["event-test"] || events["event-missing"] {
		t.Fatalf("events=%v err=%v", events, err)
	}
	otherEvents, err := state.PreviouslyKnownEvents(ctx, domain.SourceLinkedIn, []string{"event-test"})
	if err != nil || otherEvents["event-test"] {
		t.Fatalf("other events=%v err=%v", otherEvents, err)
	}
}

func TestPreferenceSignalsUseLatestCanonicalSourceEvidenceLabel(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	settings, _ := state.GetSettings(ctx)
	evidence := "x:000000000000000000000501"
	reason := "not_interested"
	assessment := domain.CandidateAssessment{EvidenceKey: evidence, TopicFacets: []string{"ai_models"}}
	assessmentRaw, _ := json.Marshal(assessment)

	insertSignal := func(sessionNumber int, direction string, reason *string, created string) {
		session, err := state.CreateSession(ctx, "What changed?", settings)
		if err != nil {
			t.Fatal(err)
		}
		runs, err := state.listRuns(ctx, session.ID)
		if err != nil {
			t.Fatal(err)
		}
		run := runs[0]
		if _, err := state.db.ExecContext(ctx, `INSERT INTO candidate_assessments(run_id,evidence_key,source,assessment_json,base_score,preference_score,final_score,selected,created_at) VALUES(?,?,?,?,?,?,?,?,?)`, run.ID, evidence, run.Source, string(assessmentRaw), .5, 0, .5, 1, created); err != nil {
			t.Fatal(err)
		}
		timelineID := fmt.Sprintf("timeline-signal-%d", sessionNumber)
		if _, err := state.db.ExecContext(ctx, `INSERT INTO timeline_items(id,session_id,run_id,source,evidence_key,rank,item_json,assessment_json,coverage_json,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, timelineID, session.ID, run.ID, run.Source, evidence, 0, "{}", string(assessmentRaw), "{}", created); err != nil {
			t.Fatal(err)
		}
		if _, err := state.db.ExecContext(ctx, `INSERT INTO feedback_events(id,timeline_id,session_id,run_id,evidence_key,direction,reason,created_at) VALUES(?,?,?,?,?,?,?,?)`, fmt.Sprintf("feedback-signal-%d", sessionNumber), timelineID, session.ID, run.ID, evidence, direction, reason, created); err != nil {
			t.Fatal(err)
		}
		if _, err := state.db.ExecContext(ctx, `UPDATE sessions SET status='completed',completed_at=? WHERE id=?`, created, session.ID); err != nil {
			t.Fatal(err)
		}
	}

	insertSignal(1, "more", nil, "2026-07-16T01:00:00Z")
	insertSignal(2, "less", &reason, "2026-07-16T02:00:00Z")
	signals, err := state.PreferenceSignals(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(signals) != 1 || signals[0].Direction != "less" || signals[0].Reason == nil || *signals[0].Reason != reason {
		t.Fatalf("signals=%+v", signals)
	}
}

func TestSchemaMismatchFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sidecar.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE meta(key TEXT PRIMARY KEY,value TEXT NOT NULL); INSERT INTO meta(key,value) VALUES('schema_version','99')`)
	db.Close()
	if err != nil {
		t.Fatal(err)
	}
	_, err = Open(path, domain.DefaultSettings("expanded", "quiet", "rank_only", true))
	if err == nil {
		t.Fatal("schema mismatch must fail")
	}
}
