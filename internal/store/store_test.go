package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestSettingsPersistSingleImageFit(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if settings.SingleImageFit != "cover" {
		t.Fatalf("default single image fit=%q", settings.SingleImageFit)
	}
	settings.SingleImageFit = "contain"
	if err := state.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	persisted, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.SingleImageFit != "contain" {
		t.Fatalf("persisted single image fit=%q", persisted.SingleImageFit)
	}
}

func TestSettingsPersistReasoningProvider(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.ReasoningProvider = "ollama"
	if err := state.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	persisted, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.ReasoningProvider != "ollama" {
		t.Fatalf("persisted reasoning provider=%q", persisted.ReasoningProvider)
	}
}

func TestSettingsPersistPostFreshnessStyle(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if settings.PostFreshnessStyle != "header_shade" {
		t.Fatalf("default post freshness style=%q", settings.PostFreshnessStyle)
	}
	settings.PostFreshnessStyle = "border_shade"
	if err := state.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	persisted, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.PostFreshnessStyle != "border_shade" {
		t.Fatalf("persisted post freshness style=%q", persisted.PostFreshnessStyle)
	}
}

func TestExistingDefaultSourceProfileAdoptsInstagramOnlyOnce(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "sidecar.db")
	defaults := domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", true)
	state, err := Open(path, defaults)
	if err != nil {
		t.Fatal(err)
	}
	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.ActiveSources = []domain.Source{domain.SourceX, domain.SourceLinkedIn, domain.SourceFacebook}
	if err := state.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.ExecContext(ctx, `DELETE FROM meta WHERE key='source_default_instagram_v1'`); err != nil {
		t.Fatal(err)
	}
	if err := state.Close(); err != nil {
		t.Fatal(err)
	}

	state, err = Open(path, defaults)
	if err != nil {
		t.Fatal(err)
	}
	settings, err = state.GetSettings(ctx)
	if err != nil || len(settings.ActiveSources) != 4 || settings.ActiveSources[3] != domain.SourceInstagram {
		t.Fatalf("migrated active sources=%v err=%v", settings.ActiveSources, err)
	}
	settings.ActiveSources = []domain.Source{domain.SourceX, domain.SourceLinkedIn, domain.SourceFacebook}
	if err := state.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	if err := state.Close(); err != nil {
		t.Fatal(err)
	}

	state, err = Open(path, defaults)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { state.Close() })
	settings, err = state.GetSettings(ctx)
	if err != nil || len(settings.ActiveSources) != 3 {
		t.Fatalf("explicit post-migration source choice was overwritten: %v err=%v", settings.ActiveSources, err)
	}
}

func visibleUpdatePolicy() domain.UpdatePolicy {
	return domain.UpdatePolicy{
		Trigger: domain.UpdateTriggerUser, Delivery: domain.UpdateDeliveryVisible, BudgetAuthority: domain.BudgetAuthorityUser,
	}
}

func preparedUpdatePolicy() domain.UpdatePolicy {
	return domain.UpdatePolicy{
		Trigger: domain.UpdateTriggerScheduler, Delivery: domain.UpdateDeliveryPrepared, BudgetAuthority: domain.BudgetAuthorityAutomatic,
	}
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

func TestLearningPanelPreferencePersists(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.ShowLearningPanel = false
	if err := state.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	stored, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ShowLearningPanel {
		t.Fatal("disabled learning panel preference was not persisted")
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
	want := []string{"ai_assessments", "ai_detection_jobs", "ai_feedback_events", "auto_update_batches", "auto_update_state", "bridge_commands", "calibration_profile_snapshots", "calibration_samples", "calibration_sessions", "candidate_assessments", "capture_surface_events", "content_continuity", "content_continuity_occurrences", "content_identity_aliases", "event_resolution_diagnostics", "event_resolution_invocations", "feedback_events", "knowledge_events", "media_provenance_assessments", "media_recaptures", "memory_actions", "memory_content_versions", "memory_identity_aliases", "memory_items", "memory_provenance", "memory_tombstone_aliases", "meta", "observations", "preference_learning_ledger", "preference_model", "reasoning_invocations", "run_stage_timings", "runs", "selection_corrections", "semantic_event_constraints", "semantic_event_corrections", "semantic_event_deltas", "semantic_event_reports", "semantic_events", "semantic_novelty_constraints", "sessions", "settings", "source_definitions", "timeline_evidence_overrides", "timeline_items", "vision_evaluation_jobs"}
	if len(names) != len(want) {
		t.Fatalf("tables=%v", names)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("tables=%v", names)
		}
	}
}

func TestCaptureSurfaceTelemetryPersistsUntilInboxReceipt(t *testing.T) {
	state := openTestStore(t)
	ctx := context.Background()
	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	session, err := createVisibleUpdateSession(state, ctx, "capture lifecycle", settings)
	if err != nil {
		t.Fatal(err)
	}
	runs, err := state.listRuns(ctx, session.ID)
	if err != nil || len(runs) == 0 {
		t.Fatalf("runs=%v err=%v", runs, err)
	}
	recorded, err := state.RecordCaptureSurfaceEvent(ctx, domain.CaptureSurfaceEvent{
		ID:        "capture-event-1",
		SessionID: session.ID,
		RunID:     runs[0].ID,
		Source:    runs[0].Source,
		Event:     "release_requested",
		Outcome:   "source_acquisition_closed",
		Detail:    map[string]any{"isolation": "shared"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if recorded.OccurredAt == "" {
		t.Fatal("capture surface receipt has no timestamp")
	}
	sessions, _, err := state.ListInboxSessions(ctx, 5, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || len(sessions[0].Runs) == 0 {
		t.Fatalf("inbox sessions=%+v", sessions)
	}
	got := sessions[0].Runs[0].CaptureSurface
	if len(got) != 1 || got[0].Event != "release_requested" || got[0].Outcome != "source_acquisition_closed" {
		t.Fatalf("capture surface telemetry=%+v", got)
	}
}

func TestAutomaticSessionRemainsHiddenUntilPreparedBatchIsRevealed(t *testing.T) {
	state := openTestStore(t)
	ctx := context.Background()
	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.CompleteOnboarding(ctx, settings.ActiveSources); err != nil {
		t.Fatal(err)
	}
	session, err := createPreparedUpdateSession(state, ctx, "automatic catch up", settings)
	if err != nil {
		t.Fatal(err)
	}
	if session.Trigger != domain.UpdateTriggerScheduler || session.Delivery != domain.UpdateDeliveryPrepared || session.BudgetAuthority != domain.BudgetAuthorityAutomatic || session.DeliveryState != "preparing" {
		t.Fatalf("prepared update policy=%+v", session)
	}
	runs, err := state.listRuns(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	now := domain.Now()
	for _, run := range runs {
		if _, err := state.db.ExecContext(ctx, `UPDATE runs SET status='completed',stage='completed',started_at=?,completed_at=? WHERE id=?`, now, now, run.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := state.db.ExecContext(ctx, `INSERT INTO timeline_items(id,session_id,run_id,source,evidence_key,rank,item_json,assessment_json,created_at) VALUES(?,?,?,?,?,0,'{}','{"urgency":0.5}',?)`, "timeline-auto", session.ID, runs[0].ID, runs[0].Source, "auto-evidence", now); err != nil {
		t.Fatal(err)
	}
	if err := state.FinalizeSession(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
	items, err := state.ListTimeline(ctx, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("prepared automatic items leaked into Timeline: %d", len(items))
	}
	batches, err := state.PreparedBatches(ctx, settings.PreparedBatchMaxAgeHours)
	if err != nil {
		t.Fatal(err)
	}
	if len(batches) != 1 || batches[0].ItemCount != 1 {
		t.Fatalf("prepared batches=%+v", batches)
	}
	if _, err := state.RevealPreparedBatch(ctx, session.ID, "prepend"); err != nil {
		t.Fatal(err)
	}
	schedule, err := state.AutoUpdateScheduleState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if schedule.LastQueueVacancyAt == "" {
		t.Fatal("revealing a prepared batch did not record its queue vacancy")
	}
	items, err = state.ListTimeline(ctx, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "timeline-auto" {
		t.Fatalf("revealed items=%+v", items)
	}
	batchSummaries, err := state.TimelineBatchSummaries(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(batchSummaries) != 1 || batchSummaries[0].SessionID != session.ID || batchSummaries[0].Trigger != domain.UpdateTriggerScheduler || batchSummaries[0].Delivery != domain.UpdateDeliveryPrepared || batchSummaries[0].RevealedAt == "" || batchSummaries[0].Presentation != "prepend" {
		t.Fatalf("timeline batch summaries=%+v", batchSummaries)
	}
}

func TestPreparedBatchPresentationOrderPersists(t *testing.T) {
	for _, test := range []struct {
		name         string
		presentation string
		want         []string
	}{
		{name: "header newest first", presentation: "prepend", want: []string{"batch-2", "batch-1"}},
		{name: "finish line reading order", presentation: "append", want: []string{"batch-1", "batch-2"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := openTestStore(t)
			ctx := context.Background()
			settings, err := state.GetSettings(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := state.CompleteOnboarding(ctx, settings.ActiveSources); err != nil {
				t.Fatal(err)
			}
			sessionIDs := make([]string, 0, 2)
			for ordinal := 1; ordinal <= 2; ordinal++ {
				session, err := createPreparedUpdateSession(state, ctx, fmt.Sprintf("prepared batch %d", ordinal), settings)
				if err != nil {
					t.Fatal(err)
				}
				runs, err := state.listRuns(ctx, session.ID)
				if err != nil {
					t.Fatal(err)
				}
				now := domain.Now()
				for _, run := range runs {
					if _, err := state.db.ExecContext(ctx, `UPDATE runs SET status='completed',stage='completed',started_at=?,completed_at=? WHERE id=?`, now, now, run.ID); err != nil {
						t.Fatal(err)
					}
				}
				timelineID := fmt.Sprintf("batch-%d", ordinal)
				if _, err := state.db.ExecContext(ctx, `INSERT INTO timeline_items(id,session_id,run_id,source,evidence_key,rank,item_json,assessment_json,created_at) VALUES(?,?,?,?,?,0,'{}','{"urgency":0.5}',?)`, timelineID, session.ID, runs[0].ID, runs[0].Source, timelineID, now); err != nil {
					t.Fatal(err)
				}
				if err := state.FinalizeSession(ctx, session.ID); err != nil {
					t.Fatal(err)
				}
				if _, err := state.RevealPreparedBatch(ctx, session.ID, test.presentation); err != nil {
					t.Fatal(err)
				}
				revealedAt := fmt.Sprintf("2026-07-24T00:00:0%dZ", ordinal)
				if _, err := state.db.ExecContext(ctx, `UPDATE auto_update_batches SET revealed_at=? WHERE session_id=?`, revealedAt, session.ID); err != nil {
					t.Fatal(err)
				}
				sessionIDs = append(sessionIDs, session.ID)
			}
			items, err := state.ListTimeline(ctx, 10, 0)
			if err != nil {
				t.Fatal(err)
			}
			got := make([]string, 0, len(items))
			for _, item := range items {
				got = append(got, item.ID)
			}
			if fmt.Sprint(got) != fmt.Sprint(test.want) {
				t.Fatalf("timeline order=%v want=%v sessions=%v", got, test.want, sessionIDs)
			}
			batches, err := state.TimelineBatchSummaries(ctx)
			if err != nil {
				t.Fatal(err)
			}
			firstSession := sessionIDs[0]
			if test.presentation == "prepend" {
				firstSession = sessionIDs[1]
			}
			if len(batches) != 2 || batches[0].SessionID != firstSession {
				t.Fatalf("batch summaries=%+v sessions=%v", batches, sessionIDs)
			}
			for _, batch := range batches {
				if batch.Presentation != test.presentation {
					t.Fatalf("batch presentation=%+v", batch)
				}
			}
		})
	}
}

func TestAutoUpdateDailyQuotaResetPreservesUsageHistory(t *testing.T) {
	state := openTestStore(t)
	ctx := context.Background()
	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.CompleteOnboarding(ctx, settings.ActiveSources); err != nil {
		t.Fatal(err)
	}
	session, err := createPreparedUpdateSession(state, ctx, "automatic quota fixture", settings)
	if err != nil {
		t.Fatal(err)
	}
	runs, err := state.listRuns(ctx, session.ID)
	if err != nil || len(runs) == 0 {
		t.Fatalf("runs=%+v err=%v", runs, err)
	}
	now := domain.Now()
	if _, err := state.db.ExecContext(ctx, `UPDATE sessions SET status='completed',started_at=?,completed_at=? WHERE id=?`, now, now, session.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.ExecContext(ctx, `INSERT INTO reasoning_invocations(id,run_id,phase,provider,model,effort,duration_ms,status,input_tokens,output_tokens,reasoning_output_tokens,created_at) VALUES('quota-before',?,'candidate_evaluation','fixture','fixture','high',1,'completed',100,50,25,?)`, runs[0].ID, now); err != nil {
		t.Fatal(err)
	}
	before, err := state.AutoUpdateBudgetUsage(ctx)
	if err != nil || before.ActualTotal != 175 || before.QuotaTotal != 175 || before.QuotaAutomatic != 175 {
		t.Fatalf("before reset=%+v err=%v", before, err)
	}
	reset, err := state.ResetAutoUpdateDailyQuota(ctx)
	if err != nil || reset.ActualTotal != 175 || reset.QuotaTotal != 0 || reset.LastManualResetAt == "" {
		t.Fatalf("reset=%+v err=%v", reset, err)
	}
	after, err := state.AutoUpdateBudgetUsage(ctx)
	if err != nil || after.ActualTotal != 175 || after.QuotaTotal != 0 || after.QuotaAutomatic != 0 {
		t.Fatalf("after reset=%+v err=%v", after, err)
	}
	if _, err := state.db.ExecContext(ctx, `INSERT INTO reasoning_invocations(id,run_id,phase,provider,model,effort,duration_ms,status,input_tokens,output_tokens,reasoning_output_tokens,created_at) VALUES('quota-after',?,'candidate_evaluation','fixture','fixture','high',1,'completed',20,5,0,?)`, runs[0].ID, now); err != nil {
		t.Fatal(err)
	}
	afterNewUsage, err := state.AutoUpdateBudgetUsage(ctx)
	if err != nil || afterNewUsage.ActualTotal != 200 || afterNewUsage.QuotaTotal != 25 || afterNewUsage.QuotaAutomatic != 25 {
		t.Fatalf("new usage=%+v err=%v", afterNewUsage, err)
	}
}

func TestFreshSchemaSourcesMatchDomainRegistry(t *testing.T) {
	state := openTestStore(t)
	rows, err := state.db.Query(`SELECT id,display_name,ordinal,enabled FROM source_definitions ORDER BY ordinal`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	descriptors := domain.Sources()
	index := 0
	for rows.Next() {
		if index >= len(descriptors) {
			t.Fatal("database contains a source outside the domain registry")
		}
		var id domain.Source
		var name string
		var ordinal, enabled int
		if err := rows.Scan(&id, &name, &ordinal, &enabled); err != nil {
			t.Fatal(err)
		}
		want := descriptors[index]
		if id != want.ID || name != want.DisplayName || ordinal != index || enabled != 1 {
			t.Fatalf("source[%d]=%s %q %d %d; want %+v", index, id, name, ordinal, enabled, want)
		}
		index++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if index != len(descriptors) {
		t.Fatalf("database source count=%d want=%d", index, len(descriptors))
	}
}

func TestRetiredSchemasFailWithoutMutation(t *testing.T) {
	for _, version := range []string{"2", "3", "4", "5", "6", "99"} {
		t.Run("schema_"+version, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "retired.db")
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`CREATE TABLE meta (key TEXT PRIMARY KEY,value TEXT NOT NULL); INSERT INTO meta(key,value) VALUES('schema_version',?)`, version); err != nil {
				db.Close()
				t.Fatal(err)
			}
			db.Close()

			if _, err := Open(path, domain.DefaultSettings("standard", "quiet", "guarded_live", true)); err == nil {
				t.Fatal("retired schema must fail closed")
			}
			db, err = sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			var tables int
			if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table'`).Scan(&tables); err != nil {
				t.Fatal(err)
			}
			if tables != 1 {
				t.Fatalf("incompatible database was mutated: tables=%d", tables)
			}
		})
	}
}

func TestSchema7MigratesTimingColumnsAtomically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema7.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		CREATE TABLE meta (key TEXT PRIMARY KEY,value TEXT NOT NULL);
		INSERT INTO meta(key,value) VALUES('schema_version','7');
		CREATE TABLE reasoning_invocations (id TEXT PRIMARY KEY,run_id TEXT,created_at TEXT);
		CREATE TABLE event_resolution_invocations (session_id TEXT PRIMARY KEY);
		CREATE TABLE ai_detection_jobs (id TEXT PRIMARY KEY,status TEXT,created_at TEXT);
	`)
	if closeErr := db.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}

	state, err := Open(path, domain.DefaultSettings("standard", "quiet", "guarded_live", true))
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	var version string
	if err := state.db.QueryRow(`SELECT value FROM meta WHERE key='schema_version'`).Scan(&version); err != nil || version != "12" {
		t.Fatalf("schema version=%q err=%v", version, err)
	}
	var receiptColumn int
	if err := state.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('event_resolution_diagnostics') WHERE name='receipt_json'`).Scan(&receiptColumn); err != nil || receiptColumn != 1 {
		t.Fatalf("receipt_json migration column=%d err=%v", receiptColumn, err)
	}
	for _, table := range []string{"reasoning_invocations", "event_resolution_invocations", "ai_detection_jobs"} {
		rows, err := state.db.Query(`PRAGMA table_info(` + table + `)`)
		if err != nil {
			t.Fatal(err)
		}
		columns := map[string]bool{}
		for rows.Next() {
			var ordinal, notNull, primaryKey int
			var name, kind string
			var defaultValue any
			if err := rows.Scan(&ordinal, &name, &kind, &notNull, &defaultValue, &primaryKey); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			columns[name] = true
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
		for _, column := range []string{"caller_latency_ms", "queue_wait_ms", "provider_execution_ms", "response_total_ms", "model_descriptor_version", "model_maturity"} {
			if !columns[column] {
				t.Fatalf("%s missing migrated column %s", table, column)
			}
		}
	}
}

func TestSchema9MigratesSemanticSignalReceiptColumn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema9.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		CREATE TABLE meta (key TEXT PRIMARY KEY,value TEXT NOT NULL);
		INSERT INTO meta(key,value) VALUES('schema_version','9');
		CREATE TABLE event_resolution_diagnostics (
			session_id TEXT PRIMARY KEY,
			historical_event_count INTEGER NOT NULL,
			resolver_invoked INTEGER NOT NULL,
			trigger_reason TEXT NOT NULL,
			strongest_overlap INTEGER NOT NULL,
			trigger_tokens_json TEXT NOT NULL DEFAULT '[]'
		);
	`)
	if closeErr := db.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	state, err := Open(path, domain.DefaultSettings("standard", "quiet", "guarded_live", true))
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	var version string
	if err := state.db.QueryRow(`SELECT value FROM meta WHERE key='schema_version'`).Scan(&version); err != nil || version != "12" {
		t.Fatalf("schema version=%q err=%v", version, err)
	}
	var receiptColumn int
	if err := state.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('event_resolution_diagnostics') WHERE name='receipt_json'`).Scan(&receiptColumn); err != nil || receiptColumn != 1 {
		t.Fatalf("receipt_json column=%d err=%v", receiptColumn, err)
	}
	var legacyDefault string
	if err := state.db.QueryRow(`SELECT dflt_value FROM pragma_table_info('event_resolution_diagnostics') WHERE name='receipt_json'`).Scan(&legacyDefault); err != nil || legacyDefault != "'{}'" {
		t.Fatalf("receipt_json default=%q err=%v", legacyDefault, err)
	}
}

func TestLatestTimelineCheckUsesLatestTerminalSessionEvenWithZeroAdditions(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	if latest, err := state.LatestTimelineCheck(ctx); err != nil || latest != nil {
		t.Fatalf("fresh latest=%+v err=%v", latest, err)
	}
	settings, _ := state.GetSettings(ctx)
	first, err := createVisibleUpdateSession(state, ctx, "first check", settings)
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
	second, err := createVisibleUpdateSession(state, ctx, "zero addition check", settings)
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
	session, err := createVisibleUpdateSession(state, ctx, "calibration authority", settings)
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
	session, err := createVisibleUpdateSession(state, ctx, "What changed?", settings)
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
	pendingRunID, err := state.PendingBridgeRunID(ctx)
	if err != nil || pendingRunID != run.ID {
		t.Fatalf("pending bridge run=%q err=%v", pendingRunID, err)
	}
	queuedRun, err := state.GetRun(ctx, run.ID)
	if err != nil || queuedRun.BridgeCommandStatus != "queued" {
		t.Fatalf("queued bridge status=%q err=%v", queuedRun.BridgeCommandStatus, err)
	}
	claimed, err := state.ClaimCommand(ctx, run.ID, "bridge-test")
	if err != nil || claimed == nil || claimed.ID != command.ID {
		t.Fatalf("claim: %+v %v", claimed, err)
	}
	pendingRunID, err = state.PendingBridgeRunID(ctx)
	if err != nil || pendingRunID != "" {
		t.Fatalf("claimed command remained pending: run=%q err=%v", pendingRunID, err)
	}
	claimedRun, err := state.GetRun(ctx, run.ID)
	if err != nil || claimedRun.BridgeCommandStatus != "claimed" {
		t.Fatalf("claimed bridge status=%q err=%v", claimedRun.BridgeCommandStatus, err)
	}
	expired, err := state.ExpiredBridgeCommands(ctx, time.Now().Add(4*time.Minute))
	if err != nil || len(expired) != 1 || expired[0].ID != command.ID {
		t.Fatalf("expired commands=%+v err=%v", expired, err)
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
	if updated.BridgeCommandStatus != "completed" {
		t.Fatalf("completed bridge status=%q", updated.BridgeCommandStatus)
	}
	if updated.Coverage["acquisitionRounds"] != float64(1) {
		t.Fatalf("durable acquisition rounds=%v coverage=%+v", updated.Coverage["acquisitionRounds"], updated.Coverage)
	}
	rounds, ok := updated.Coverage["rounds"].([]any)
	if !ok || len(rounds) != 1 {
		t.Fatalf("durable coverage rounds=%+v", updated.Coverage["rounds"])
	}
}

func TestSessionSnapshotsSourceWaitModeAndSerializesBrowserClaims(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	settings, _ := state.GetSettings(ctx)
	settings.SourceWaitMode = "progressive_wait"
	session, err := createVisibleUpdateSession(state, ctx, "progressive capture lane", settings)
	if err != nil {
		t.Fatal(err)
	}
	if session.Coverage["sourceWaitMode"] != "progressive_wait" {
		t.Fatalf("session coverage=%+v", session.Coverage)
	}
	if len(session.Runs) < 2 {
		t.Fatal("test requires at least two sources")
	}
	first, second := session.Runs[0], session.Runs[1]
	firstCommand, err := state.StartRun(ctx, first.ID, map[string]any{"source": first.Source})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.StartRun(ctx, second.ID, map[string]any{"source": second.Source}); err != nil {
		t.Fatal(err)
	}
	claimedFirst, err := state.ClaimCommand(ctx, first.ID, "bridge-one")
	if err != nil || claimedFirst == nil || claimedFirst.ID != firstCommand.ID {
		t.Fatalf("first claim=%+v err=%v", claimedFirst, err)
	}
	active, err := state.GetSession(ctx, session.ID)
	if err != nil || active.ActiveSource == nil || *active.ActiveSource != first.Source {
		t.Fatalf("active source must follow the claimed capture blocker: session=%+v err=%v", active, err)
	}
	claimedSecond, err := state.ClaimCommand(ctx, second.ID, "bridge-two")
	if err != nil || claimedSecond != nil {
		t.Fatalf("second capture lane must wait: claim=%+v err=%v", claimedSecond, err)
	}
	observation := domain.Observation{Source: first.Source, CapturedAt: domain.Now(), Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{EvidenceKey: "x:progressive-lane", Text: "Captured first"}}}}, Coverage: map[string]any{"status": "complete"}}
	if err := state.SaveObservation(ctx, firstCommand.ID, first.ID, observation); err != nil {
		t.Fatal(err)
	}
	active, err = state.GetSession(ctx, session.ID)
	if err != nil || active.ActiveSource == nil || *active.ActiveSource != first.Source {
		t.Fatalf("active source must follow the reasoning run after capture: session=%+v err=%v", active, err)
	}
	claimedSecond, err = state.ClaimCommand(ctx, second.ID, "bridge-two")
	if err != nil || claimedSecond == nil {
		t.Fatalf("second capture did not enter released lane: claim=%+v err=%v", claimedSecond, err)
	}
}

func TestPipelineStagesAreDurableAndValidated(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	settings, _ := state.GetSettings(ctx)
	session, err := createVisibleUpdateSession(state, ctx, "observable pipeline", settings)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.SetSessionPipelineStage(ctx, session.ID, "ai_fast_detection"); err != nil {
		t.Fatal(err)
	}
	storedSession, err := state.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if storedSession.Coverage["pipelineStage"] != "ai_fast_detection" || storedSession.Coverage["pipelineStageUpdatedAt"] == nil {
		t.Fatalf("session pipeline coverage=%+v", storedSession.Coverage)
	}
	if err := state.SetSessionPipelineStage(ctx, session.ID, "unknown"); err == nil {
		t.Fatal("expected invalid session pipeline stage to fail")
	}

	run, err := state.AdvanceSession(ctx, session.ID)
	if err != nil || run == nil {
		t.Fatalf("next run: %+v %v", run, err)
	}
	if _, err := state.db.ExecContext(ctx, `UPDATE runs SET status='reasoning',stage='reasoning' WHERE id=?`, run.ID); err != nil {
		t.Fatal(err)
	}
	if err := state.SetRunPipelineStage(ctx, run.ID, "candidate_evaluation"); err != nil {
		t.Fatal(err)
	}
	storedRun, err := state.GetRun(ctx, run.ID)
	if err != nil || storedRun.Stage != "candidate_evaluation" {
		t.Fatalf("run pipeline stage=%+v err=%v", storedRun, err)
	}
	if err := state.SetRunPipelineStage(ctx, run.ID, "unknown"); err == nil {
		t.Fatal("expected invalid run pipeline stage to fail")
	}
}

func TestAcquisitionPlanningReceiptSurvivesFollowUpCapture(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	settings, _ := state.GetSettings(ctx)
	settings.ActiveSources = []domain.Source{domain.SourceLinkedIn}
	if err := state.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	session, err := createVisibleUpdateSession(state, ctx, "durable acquisition receipt", settings)
	if err != nil {
		t.Fatal(err)
	}
	run := session.Runs[0]
	command, err := state.StartRun(ctx, run.ID, map[string]any{"source": run.Source})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.ClaimCommand(ctx, run.ID, "receipt-test"); err != nil {
		t.Fatal(err)
	}
	receipt := map[string]any{
		"mode": "model", "decision": "request_follow_up", "reason": "bounded overlap",
		"followUpQueued": true, "followUpNewCandidates": 0,
	}
	if err := state.SetRunCoverageField(ctx, run.ID, "acquisitionPlanning", receipt); err != nil {
		t.Fatal(err)
	}
	first := domain.Observation{
		Source: run.Source, CapturedAt: domain.Now(),
		Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{
			EvidenceKey: "linkedin:overlap", Author: "Author",
			Text: strings.Repeat("Durable LinkedIn overlap evidence. ", 4),
		}}}},
		Coverage: map[string]any{"status": "partial"},
	}
	if err := state.SaveObservation(ctx, command.ID, run.ID, first); err != nil {
		t.Fatal(err)
	}
	followUp, err := state.QueueFollowUp(ctx, run.ID, map[string]any{"source": run.Source, "round": 2})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.ClaimCommand(ctx, run.ID, "receipt-test"); err != nil {
		t.Fatal(err)
	}
	second := first
	second.CapturedAt = domain.Now()
	if err := state.SaveObservation(ctx, followUp.ID, run.ID, second); err != nil {
		t.Fatal(err)
	}
	stored, err := state.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	planning, ok := stored.Coverage["acquisitionPlanning"].(map[string]any)
	if !ok || planning["decision"] != "request_follow_up" || planning["followUpQueued"] != true {
		t.Fatalf("planning receipt=%+v", stored.Coverage)
	}
	rounds, ok := stored.Coverage["rounds"].([]any)
	if !ok || len(rounds) != 2 {
		t.Fatalf("capture rounds=%+v", stored.Coverage)
	}
}

func TestSessionRemainsActiveUntilCompositionFinalizes(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	settings, _ := state.GetSettings(ctx)
	session, err := createVisibleUpdateSession(state, ctx, "terminal composition boundary", settings)
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
	session, err := createVisibleUpdateSession(state, ctx, "What changed?", settings)
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
	if err != nil || total != 1 || len(inbox) != 1 || len(inbox[0].Runs) != 4 {
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
	if _, err := state.db.ExecContext(ctx, `INSERT INTO content_continuity(source,evidence_key,content_fingerprint,context_fingerprint,engagement_score,first_seen_at,last_seen_at,last_run_id,seen_count) VALUES(?,?,?,?,?,?,?,?,?)`, domain.SourceLinkedIn, "linkedin:pre-reset", "content-before-reset", "", 0, domain.Now(), domain.Now(), "run-before-reset", 2); err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.ExecContext(ctx, `INSERT INTO content_identity_aliases(source,identity_fingerprint,canonical_evidence_key,canonical_platform_id,canonical_permalink,canonical_content_kind,canonical_published_at,ambiguous,first_seen_at,last_seen_at,last_run_id,seen_count) VALUES(?,?,?,?,?,?,?,0,?,?,?,?)`, domain.SourceLinkedIn, "identity-before-reset", "linkedin:pre-reset", "", "", "post", "", domain.Now(), domain.Now(), "run-before-reset", 2); err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.ExecContext(ctx, `INSERT INTO meta(key,value) VALUES('pending_app_profile_reset',?)`, domain.Now()); err != nil {
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
	if err != nil || after.LoadProfile != "expanded" || len(after.ActiveSources) != 4 || after.ActiveSources[3] != domain.SourceInstagram || after.DefaultPresentation != "source" || after.StreamWidth != "social" {
		t.Fatalf("reset settings=%+v err=%v", after, err)
	}
	afterToken, err := state.BridgeToken(ctx)
	if err != nil || afterToken != token {
		t.Fatalf("bridge token changed: before=%q after=%q err=%v", token, afterToken, err)
	}
	pendingProfileWipe, err := state.PendingAppProfileReset(ctx)
	if err != nil || pendingProfileWipe {
		t.Fatalf("full reset retained legacy profile-wipe marker: pending=%v err=%v", pendingProfileWipe, err)
	}
	calibrationStatus, err = state.CalibrationFirstRunStatus(ctx)
	if err != nil || calibrationStatus != "not_started" {
		t.Fatalf("reset calibration status=%q err=%v", calibrationStatus, err)
	}
	var continuityCount int
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM content_continuity`).Scan(&continuityCount); err != nil || continuityCount != 0 {
		t.Fatalf("full reset retained native content continuity: count=%d err=%v", continuityCount, err)
	}
	var identityCount int
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM content_identity_aliases`).Scan(&identityCount); err != nil || identityCount != 0 {
		t.Fatalf("full reset retained content identity aliases: count=%d err=%v", identityCount, err)
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
	session, err := createVisibleUpdateSession(state, ctx, "What changed?", settings)
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
}

func TestSessionCompositionCountsOnlyUniqueInformationAgainstCapacity(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	settings, _ := state.GetSettings(ctx)
	session, err := createVisibleUpdateSession(state, ctx, "What changed?", settings)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.ExecContext(ctx, `UPDATE sessions SET max_items_total=2 WHERE id=?`, session.ID); err != nil {
		t.Fatal(err)
	}
	runs, err := state.listRuns(ctx, session.ID)
	if err != nil || len(runs) == 0 {
		t.Fatalf("runs=%+v err=%v", runs, err)
	}
	now := domain.Now()
	if _, err := state.db.ExecContext(ctx, `INSERT INTO semantic_events(id,canonical_claim,actor,event_kind,aliases_json,first_seen_at,last_seen_at) VALUES(?,?,?,?,?,?,?)`, "event-capacity", "OpenAI changed Luna pricing", "OpenAI", "pricing", "[]", now, now); err != nil {
		t.Fatal(err)
	}
	relations := []string{"duplicate_report", "new_event", "duplicate_report", "new_event", "duplicate_report"}
	for index, relation := range relations {
		evidence := fmt.Sprintf("x:capacity-%d", index)
		timelineID := fmt.Sprintf("timeline-capacity-%d", index)
		assessmentRaw, _ := json.Marshal(domain.CandidateAssessment{EvidenceKey: evidence})
		itemRaw, _ := json.Marshal(domain.ReasonedItem{EvidenceKey: evidence, Source: runs[0].Source, WhatChanged: fmt.Sprintf("Luna pricing report %d", index)})
		score := 1 - float64(index)*.1
		if _, err := state.db.ExecContext(ctx, `INSERT INTO candidate_assessments(run_id,evidence_key,source,assessment_json,base_score,preference_score,final_score,selected,created_at) VALUES(?,?,?,?,?,?,?,?,?)`, runs[0].ID, evidence, runs[0].Source, string(assessmentRaw), score, 0, score, 1, now); err != nil {
			t.Fatal(err)
		}
		if _, err := state.db.ExecContext(ctx, `INSERT INTO timeline_items(id,session_id,run_id,source,evidence_key,rank,item_json,assessment_json,coverage_json,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, timelineID, session.ID, runs[0].ID, runs[0].Source, evidence, index, string(itemRaw), string(assessmentRaw), "{}", now); err != nil {
			t.Fatal(err)
		}
		if _, err := state.db.ExecContext(ctx, `INSERT INTO semantic_event_reports(id,event_id,timeline_id,session_id,run_id,evidence_key,source,relation,confidence,reason,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, fmt.Sprintf("report-capacity-%d", index), "event-capacity", timelineID, session.ID, runs[0].ID, evidence, runs[0].Source, relation, .99, "fixture", now); err != nil {
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
	unique, duplicates := 0, 0
	for index, item := range items {
		if item.Rank != index {
			t.Fatalf("non-compact rank at %d: %+v", index, item)
		}
		if item.SemanticEvent != nil && item.SemanticEvent.Relation == "duplicate_report" {
			duplicates++
		} else {
			unique++
		}
	}
	if unique != 2 || duplicates != 2 || len(items) != 4 {
		t.Fatalf("unique=%d duplicates=%d items=%+v", unique, duplicates, items)
	}
}

func TestPreviouslyDeliveredEvidenceIsSourceScoped(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	settings, _ := state.GetSettings(ctx)
	session, err := createVisibleUpdateSession(state, ctx, "What changed?", settings)
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
		session, err := createVisibleUpdateSession(state, ctx, "What changed?", settings)
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

func TestPreferenceLearningSurvivesBulkyRunRetention(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	settings, _ := state.GetSettings(ctx)
	settings.KnowledgeRetentionDays = 30
	evidence := "x:000000000000000000000551"
	assessment := domain.CandidateAssessment{
		EvidenceKey: evidence, TopicTags: []string{"durable taste"}, TopicFacets: []string{"product"},
	}
	assessmentRaw, _ := json.Marshal(assessment)
	session, err := createVisibleUpdateSession(state, ctx, "Old but useful preference evidence", settings)
	if err != nil {
		t.Fatal(err)
	}
	runs, err := state.listRuns(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	run := runs[0]
	old := "2025-01-01T00:00:00Z"
	if _, err := state.db.ExecContext(ctx, `INSERT INTO candidate_assessments(run_id,evidence_key,source,assessment_json,base_score,preference_score,final_score,selected,created_at) VALUES(?,?,?,?,?,?,?,?,?)`, run.ID, evidence, run.Source, string(assessmentRaw), .5, 0, .5, 1, old); err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.ExecContext(ctx, `INSERT INTO timeline_items(id,session_id,run_id,source,evidence_key,rank,item_json,assessment_json,coverage_json,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, "timeline-retained-taste", session.ID, run.ID, run.Source, evidence, 0, "{}", string(assessmentRaw), "{}", old); err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.ExecContext(ctx, `INSERT INTO feedback_events(id,timeline_id,session_id,run_id,evidence_key,direction,created_at) VALUES(?,?,?,?,?,?,?)`, "feedback-retained-taste", "timeline-retained-taste", session.ID, run.ID, evidence, "more", old); err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.ExecContext(ctx, `UPDATE sessions SET status='completed',completed_at=? WHERE id=?`, old, session.ID); err != nil {
		t.Fatal(err)
	}

	result, err := state.EnforceRetention(ctx, settings)
	if err != nil {
		t.Fatal(err)
	}
	if result.RemovedSessions != 1 {
		t.Fatalf("removed sessions=%d", result.RemovedSessions)
	}
	var remainingRuns int
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions WHERE id=?`, session.ID).Scan(&remainingRuns); err != nil {
		t.Fatal(err)
	}
	if remainingRuns != 0 {
		t.Fatal("bulky session was not trimmed")
	}
	signals, err := state.PreferenceSignals(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(signals) != 1 || signals[0].EventID != "feedback-retained-taste" || signals[0].Direction != "more" {
		t.Fatalf("retained preference signals=%+v", signals)
	}
}

func TestInboxLetsLatestMoreOrLessDecisionReplaceAnEarlierChoice(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	settings, _ := state.GetSettings(ctx)
	session, err := createVisibleUpdateSession(state, ctx, "What changed?", settings)
	if err != nil {
		t.Fatal(err)
	}
	runs, err := state.listRuns(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	run := runs[0]
	evidence := "x:000000000000000000000601"
	item := domain.ReasonedItem{
		EvidenceKey: evidence,
		Source:      run.Source,
		Author:      "Example Author",
		WhatChanged: "A useful update that was rated by mistake.",
		SourceURL:   "https://x.com/example/status/601",
	}
	assessment := domain.CandidateAssessment{EvidenceKey: evidence, TopicFacets: []string{"useful_update"}}
	itemRaw, _ := json.Marshal(item)
	assessmentRaw, _ := json.Marshal(assessment)
	if _, err := state.db.ExecContext(ctx, `INSERT INTO candidate_assessments(run_id,evidence_key,source,assessment_json,base_score,preference_score,final_score,selected,created_at) VALUES(?,?,?,?,?,?,?,?,?)`, run.ID, evidence, run.Source, string(assessmentRaw), .5, 0, .5, 1, "2026-07-17T01:00:00Z"); err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.ExecContext(ctx, `INSERT INTO timeline_items(id,session_id,run_id,source,evidence_key,rank,item_json,assessment_json,coverage_json,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, "timeline-inbox-feedback", session.ID, run.ID, run.Source, evidence, 0, string(itemRaw), string(assessmentRaw), "{}", "2026-07-17T01:00:00Z"); err != nil {
		t.Fatal(err)
	}
	reason := "not_interested"
	less, err := state.AddFeedback(ctx, "timeline-inbox-feedback", domain.Feedback{Direction: "less", Reason: &reason})
	if err != nil {
		t.Fatal(err)
	}
	more, err := state.AddFeedback(ctx, "timeline-inbox-feedback", domain.Feedback{Direction: "more"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.ExecContext(ctx, `UPDATE feedback_events SET created_at=CASE id WHEN ? THEN '2026-07-17T01:01:00Z' WHEN ? THEN '2026-07-17T01:02:00Z' END WHERE id IN (?,?)`, less.ID, more.ID, less.ID, more.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.ExecContext(ctx, `UPDATE sessions SET status='completed',completed_at='2026-07-17T01:03:00Z' WHERE id=?`, session.ID); err != nil {
		t.Fatal(err)
	}

	inbox, total, err := state.ListInboxSessions(ctx, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(inbox) != 1 || len(inbox[0].PreferenceDecisions) != 1 {
		t.Fatalf("inbox=%+v total=%d", inbox, total)
	}
	decision := inbox[0].PreferenceDecisions[0]
	if decision.TimelineID != "timeline-inbox-feedback" || decision.Direction != "more" || decision.Author != item.Author || decision.Summary != item.WhatChanged || decision.SourceURL != item.SourceURL {
		t.Fatalf("decision=%+v", decision)
	}
	signals, err := state.PreferenceSignals(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(signals) != 1 || signals[0].Direction != "more" {
		t.Fatalf("signals=%+v", signals)
	}
	timeline, err := state.ListTimeline(ctx, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(timeline) != 1 || timeline[0].Feedback == nil || timeline[0].Feedback.Direction != "more" || timeline[0].Feedback.ID != more.ID {
		t.Fatalf("timeline feedback=%+v", timeline)
	}
}

func TestCompletedOnboardingSurvivesStoreRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "sidecar.db")
	defaults := domain.DefaultSettings("standard", "quiet", "guarded_live", true)

	state, err := Open(path, defaults)
	if err != nil {
		t.Fatal(err)
	}
	completed, err := state.CompleteOnboarding(ctx, []domain.Source{domain.SourceX, domain.SourceLinkedIn})
	if err != nil || completed.Status != "completed" || completed.Profile == nil {
		t.Fatalf("completed onboarding=%+v err=%v", completed, err)
	}
	completedAt := completed.Profile.CompletedAt
	if completedAt == "" {
		t.Fatal("completed onboarding must persist a completion timestamp")
	}
	if err := state.Close(); err != nil {
		t.Fatal(err)
	}

	restarted, err := Open(path, defaults)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	restored, err := restarted.Onboarding(ctx)
	if err != nil || restored.Status != "completed" || restored.Profile == nil {
		t.Fatalf("restored onboarding=%+v err=%v", restored, err)
	}
	if restored.Profile.CompletedAt != completedAt {
		t.Fatalf("completion timestamp changed across restart: before=%q after=%q", completedAt, restored.Profile.CompletedAt)
	}
	if len(restored.Profile.ActiveSources) != 2 || restored.Profile.ActiveSources[0] != domain.SourceX || restored.Profile.ActiveSources[1] != domain.SourceLinkedIn {
		t.Fatalf("restored active sources=%+v", restored.Profile.ActiveSources)
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
