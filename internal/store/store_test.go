package store

import (
	"context"
	"database/sql"
	"encoding/json"
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
	want := []string{"bridge_commands", "calibration_profile_snapshots", "calibration_samples", "calibration_sessions", "candidate_assessments", "feedback_events", "knowledge_events", "meta", "observations", "preference_model", "reasoning_invocations", "runs", "sessions", "settings", "timeline_items"}
	if len(names) != len(want) {
		t.Fatalf("tables=%v", names)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("tables=%v", names)
		}
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
