package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func semanticOwnershipLegacyDB(t *testing.T) *sql.DB {
	t.Helper()
	ctx := context.Background()
	db := lifecycleLegacyDB(t)
	if err := migrateSchema26To27(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := migrateSchema27To28(ctx, db); err != nil {
		t.Fatal(err)
	}
	// Recreate the exact v28 ownership edges: the legacy helper derives most
	// tables from the current canonical schema, which no longer has these FKs.
	for _, table := range []string{"semantic_event_corrections", "semantic_event_reports"} {
		var ddl string
		if err := db.QueryRow(`SELECT sql FROM sqlite_master WHERE name=?`, table).Scan(&ddl); err != nil {
			t.Fatal(err)
		}
		ddl = strings.ReplaceAll(ddl, "  item_json TEXT,\n", "")
		if table == "semantic_event_reports" {
			ddl = strings.Replace(ddl, "timeline_id TEXT NOT NULL UNIQUE,", "timeline_id TEXT NOT NULL UNIQUE REFERENCES timeline_items(id) ON DELETE CASCADE,", 1)
		} else {
			ddl = strings.Replace(ddl, "timeline_id TEXT NOT NULL,", "timeline_id TEXT NOT NULL REFERENCES timeline_items(id) ON DELETE CASCADE,", 1)
		}
		if _, err := db.Exec(`DROP TABLE ` + table + `;` + ddl); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`
 INSERT INTO semantic_events(id,canonical_claim,first_seen_at,last_seen_at) VALUES('event','preserve','2026-01-01','2026-01-01');
 INSERT INTO semantic_event_reports(id,event_id,timeline_id,session_id,run_id,evidence_key,source,relation,confidence,created_at) VALUES('report','event','post','origin','run','evidence','x','material_update',1,'2026-01-01');
 INSERT INTO semantic_event_deltas(id,event_id,report_id,fingerprint,claim,kind,source,evidence_key,confidence,first_seen_at,last_seen_at) VALUES('delta','event','report','fingerprint','preserve','material_update','x','evidence',1,'2026-01-01','2026-01-01');
 INSERT INTO semantic_event_corrections(id,report_id,timeline_id,action,from_event_id,from_relation,to_event_id,to_relation,created_at) VALUES('correction','report','post','same_event','event','new_event','event','material_update','2026-01-01');
 CREATE INDEX semantic_extra_index ON semantic_event_reports(evidence_key);
 CREATE TRIGGER semantic_extra_trigger AFTER UPDATE ON semantic_event_reports BEGIN SELECT 1; END;
 `); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestSemanticEvidenceOwnershipMigrationPreservesRowsAndSnapshots(t *testing.T) {
	ctx := context.Background()
	db := semanticOwnershipLegacyDB(t)
	if err := migrateSchema28To29(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := requireForeignKeys(ctx, db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM timeline_items WHERE id='post'`); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"semantic_event_reports", "semantic_event_corrections", "semantic_event_deltas"} {
		var count int
		if err := db.QueryRow(`SELECT count(*) FROM ` + table).Scan(&count); err != nil || count != 1 {
			t.Fatalf("%s count=%d err=%v", table, count, err)
		}
	}
	var snapshot, reportID string
	if err := db.QueryRow(`SELECT item_json FROM semantic_event_reports WHERE id='report'`).Scan(&snapshot); err != nil || snapshot != `{"whatChanged":"preserve"}` {
		t.Fatalf("snapshot=%s err=%v", snapshot, err)
	}
	if err := db.QueryRow(`SELECT report_id FROM semantic_event_deltas WHERE id='delta'`).Scan(&reportID); err != nil || reportID != "report" {
		t.Fatalf("report=%s err=%v", reportID, err)
	}
	var objects int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name IN ('semantic_extra_index','semantic_extra_trigger')`).Scan(&objects); err != nil || objects != 2 {
		t.Fatalf("objects=%d err=%v", objects, err)
	}
	issues, err := lifecycleForeignKeyIssues(ctx, mustBeginSemanticTx(t, db))
	if err != nil || len(issues) != 0 {
		t.Fatalf("issues=%v err=%v", issues, err)
	}
}

func mustBeginSemanticTx(t *testing.T, db *sql.DB) *sql.Tx {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tx.Rollback() })
	return tx
}

func TestSemanticEvidenceOwnershipMigrationRollsBackRowsAndFKs(t *testing.T) {
	ctx := context.Background()
	db := semanticOwnershipLegacyDB(t)
	if _, err := db.Exec(`CREATE TABLE timeline_retention_receipts_v28(id TEXT)`); err != nil {
		t.Fatal(err)
	}
	if err := migrateSchema28To29(ctx, db); err == nil {
		t.Fatal("expected late migration failure")
	}
	if err := requireForeignKeys(ctx, db); err != nil {
		t.Fatal(err)
	}
	var version string
	if err := db.QueryRow(`SELECT value FROM meta WHERE key='schema_version'`).Scan(&version); err != nil || version != "28" {
		t.Fatalf("version=%s err=%v", version, err)
	}
	for _, table := range []string{"semantic_event_reports", "semantic_event_corrections"} {
		var count int
		if err := db.QueryRow(`SELECT count(*) FROM pragma_foreign_key_list(?) WHERE "table"='timeline_items'`, table).Scan(&count); err != nil || count != 1 {
			t.Fatalf("%s FKs=%d err=%v", table, count, err)
		}
		if err := db.QueryRow(`SELECT count(*) FROM ` + table).Scan(&count); err != nil || count != 1 {
			t.Fatalf("%s rows=%d err=%v", table, count, err)
		}
	}
}

func TestSemanticEvidenceSurvivesRetentionAndSupportsUndoAndBackfill(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	state := openTimelineRetentionStore(t, now)
	session, id := insertSemanticTimelineFixture(t, state, "material_update")
	if err := state.SaveEventResolutionSummary(ctx, domain.EventResolutionSummary{SessionID: session.ID, Status: "completed", Provider: "test", Model: "test", Effort: "none", CandidateCount: 1, UniqueItems: 1, CreatedAt: domain.Now()}); err != nil {
		t.Fatal(err)
	}
	correction, err := state.CorrectSemanticEvent(ctx, id, "not_same_event", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = state.db.Exec(`UPDATE timeline_items SET presented_at=?,batch_state='visible' WHERE id=?`, now.AddDate(0, 0, -31).Format(time.RFC3339Nano), id); err != nil {
		t.Fatal(err)
	}
	settings, _ := state.GetSettings(ctx)
	for i := 0; i < 2; i++ {
		result, err := state.EnforceRetention(ctx, settings)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 && result.Timeline.RemovedItems != 1 {
			t.Fatalf("%+v", result.Timeline)
		}
	}
	summary, err := state.EventResolutionSummary(ctx, session.ID)
	if err != nil || summary == nil || summary.UserSplitCorrections != 1 {
		t.Fatalf("summary=%+v err=%v", summary, err)
	}
	exact, err := state.ExactSemanticEventIDs(ctx, []string{"x:semantic-fixture"})
	if err != nil || exact["x:semantic-fixture"] != correction.ToEventID {
		t.Fatalf("exact=%v err=%v", exact, err)
	}
	if _, err = state.UndoSemanticCorrection(ctx, correction.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = state.db.Exec(`DELETE FROM meta WHERE key='semantic_delta_backfill_version'`); err != nil {
		t.Fatal(err)
	}
	if err = state.backfillSemanticEventDeltas(ctx); err != nil {
		t.Fatal(err)
	}
	var claim string
	if err = state.db.QueryRow(`SELECT claim FROM semantic_event_deltas WHERE event_id='event-existing'`).Scan(&claim); err != nil || claim != "OpenAI launches Codex App Server" {
		t.Fatalf("claim=%s err=%v", claim, err)
	}
	if _, err = state.CorrectSemanticEvent(ctx, id, "not_same_event", "", ""); err != nil {
		t.Fatalf("correction after eviction: %v", err)
	}
	health, err := state.DatabaseHealth(ctx)
	if err != nil || health.Status != "healthy" {
		t.Fatalf("health=%+v err=%v", health, err)
	}
}

func TestTimelineRetentionWaitsForLivingTopicRouting(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	state := openTimelineRetentionStore(t, now)
	old := now.AddDate(0, 0, -31)
	insertTimelineRetentionItem(t, state, "routing", &old, "visible", 1)
	if _, err := state.db.Exec(`INSERT INTO living_topic_routing_jobs(id,session_id,timeline_id,status,engine_version,queued_at) VALUES('job','session-routing','routing','pending','v1','2026-01-01')`); err != nil {
		t.Fatal(err)
	}
	settings, _ := state.GetSettings(ctx)
	settings.KnowledgeRetentionDays = 1
	result, err := state.EnforceRetention(ctx, settings)
	if err != nil || result.Timeline.RemovedItems != 0 || result.Timeline.ProcessingProtectedItems != 1 {
		t.Fatalf("%+v err=%v", result.Timeline, err)
	}
	if _, err := state.LivingTopicRoutingItem(ctx, "routing"); err != nil {
		t.Fatalf("pending route lost input: %v", err)
	}
	if err := state.FinishLivingTopicRouting(ctx, "job", nil, nil); err != nil {
		t.Fatal(err)
	}
	result, err = state.EnforceRetention(ctx, settings)
	if err != nil || result.Timeline.RemovedItems != 1 {
		t.Fatalf("%+v err=%v", result.Timeline, err)
	}
}

func TestTimelineEvictionPostflightRollsBackProjectionAndReceipt(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	state := openTimelineRetentionStore(t, now)
	old := now.AddDate(0, 0, -31)
	insertTimelineRetentionItem(t, state, "postflight", &old, "visible", 1)
	var receiptsBefore int
	if err := state.db.QueryRow(`SELECT count(*) FROM timeline_retention_receipts`).Scan(&receiptsBefore); err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.Exec(`CREATE TRIGGER timeline_retention_corruption AFTER DELETE ON timeline_items BEGIN INSERT INTO memory_retention_claims(memory_item_id,claim_kind,claimed_at) VALUES('missing','saved','now'); END`); err != nil {
		t.Fatal(err)
	}
	settings, _ := state.GetSettings(ctx)
	if _, err := state.EnforceRetention(ctx, settings); !errors.Is(err, ErrDatabaseMaintenanceRequired) {
		t.Fatalf("postflight err=%v", err)
	}
	var items, receipts int
	if err := state.db.QueryRow(`SELECT count(*) FROM timeline_items`).Scan(&items); err != nil {
		t.Fatal(err)
	}
	if err := state.db.QueryRow(`SELECT count(*) FROM timeline_retention_receipts`).Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if items != 1 || receipts != receiptsBefore {
		t.Fatalf("items=%d receipts=%d", items, receipts)
	}
	health, err := state.DatabaseHealth(ctx)
	if err != nil || health.Status != "healthy" {
		t.Fatalf("health=%+v err=%v", health, err)
	}
}
