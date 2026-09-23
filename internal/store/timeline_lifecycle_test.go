package store

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestDurableTimelineSurvivesOperationalDeletion(t *testing.T) {
	for _, target := range []string{"sessions", "runs"} {
		t.Run(target, func(t *testing.T) {
			ctx := context.Background()
			state := openTestStore(t)
			fixture := createRoutineMemoryTimelineFixture(t, state, ctx, "completed", "x:routine-memory:12345")
			before, err := state.TimelineItem(ctx, fixture.Item.ID)
			if err != nil {
				t.Fatal(err)
			}
			if before.Evidence == nil {
				t.Fatal("fixture missing evidence")
			}
			first, err := state.AddFeedback(ctx, before.ID, domain.Feedback{Direction: "more"})
			if err != nil {
				t.Fatal(err)
			}
			id := fixture.Session.ID
			if target == "runs" {
				id = fixture.Run.ID
			}
			if _, err = state.db.Exec(`DELETE FROM `+target+` WHERE id=?`, id); err != nil {
				t.Fatal(err)
			}
			after, err := state.TimelineItem(ctx, before.ID)
			if err != nil {
				t.Fatal(err)
			}
			if after.Evidence == nil || after.Evidence.Text != before.Evidence.Text || after.Item.WhatChanged != before.Item.WhatChanged {
				t.Fatalf("display changed: %+v", after)
			}
			if after.OriginRunAvailable || (target == "sessions" && after.OriginSessionAvailable) {
				t.Fatalf("origin availability fabricated: %+v", after)
			}
			if after.SessionID != before.SessionID || after.RunID != before.RunID {
				t.Fatal("historical provenance lost")
			}
			for _, direction := range []string{"less", "more"} {
				input := domain.Feedback{Direction: direction}
				if direction == "less" {
					reason := "not_interested"
					input.Reason = &reason
				}
				feedback, err := state.AddFeedback(ctx, before.ID, input)
				if err != nil {
					t.Fatal(err)
				}
				var count int
				if err = state.db.QueryRow(`SELECT count(*) FROM preference_learning_ledger WHERE event_id=?`, feedback.ID).Scan(&count); err != nil || count != 1 {
					t.Fatalf("learning missing: %d %v", count, err)
				}
			}
			var count int
			if err = state.db.QueryRow(`SELECT count(*) FROM feedback_events WHERE id=?`, first.ID).Scan(&count); err != nil || count != 1 {
				t.Fatalf("feedback lost %d %v", count, err)
			}
			items, err := state.ListTimeline(ctx, 20, 0)
			if err != nil || len(items) != 1 {
				t.Fatalf("timeline=%+v %v", items, err)
			}
			if _, err = state.db.Exec(`DELETE FROM content_identity_aliases WHERE NOT EXISTS(SELECT 1 FROM runs WHERE runs.id=last_run_id); DELETE FROM content_continuity WHERE NOT EXISTS(SELECT 1 FROM runs WHERE runs.id=last_run_id)`); err != nil {
				t.Fatal(err)
			}
			health, err := state.DatabaseHealth(ctx)
			if err != nil || health.Status != "healthy" {
				t.Fatalf("health=%+v %v", health, err)
			}
			tx, err := state.db.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			kept, err := timelineItemForRetentionTx(ctx, tx, before.ID, true)
			tx.Rollback()
			if err != nil || kept.Evidence == nil {
				t.Fatalf("retention action after deletion: %+v %v", kept, err)
			}
		})
	}
}

func TestDurableProjectionAndHiddenBatchSurviveSessionDeletion(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	session, id := insertSemanticTimelineFixture(t, state, "new_event")
	for _, q := range []string{
		`INSERT INTO ai_assessments(id,timeline_id,session_id,stage,status,confidence_band,assessed_object,signal_scope,provider,detector_version,created_at) SELECT 'ai',id,session_id,'fast','no_signal_detected','low','social_post','none','local','v1',created_at FROM timeline_items`,
		`INSERT INTO media_provenance_assessments(id,timeline_id,session_id,source,media_index,media_kind,target_url,target_url_hash,status,manifest_state,trust_state,ai_origin,provider,verifier_version,created_at) SELECT 'media',id,session_id,source,0,'image','https://example.com/image','hash','completed','no_manifest','not_applicable','unknown','local','v1',created_at FROM timeline_items`,
		`INSERT INTO ai_feedback_events(id,timeline_id,session_id,source,target_type,target_key,verdict,signal_scope,created_at) SELECT 'user-ai',id,session_id,source,'post',evidence_key,'not_ai','social_post',created_at FROM timeline_items`,
		`INSERT INTO auto_update_batches(session_id,state,created_at) SELECT id,'expired',created_at FROM sessions`,
	} {
		if _, err := state.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := state.db.Exec(`DELETE FROM sessions WHERE id=?`, session.ID); err != nil {
		t.Fatal(err)
	}
	item, err := state.TimelineItem(ctx, id)
	if err != nil || item.SemanticEvent == nil || item.AIDetection == nil {
		t.Fatalf("item=%+v %v", item, err)
	}
	for _, table := range []string{"ai_assessments", "media_provenance_assessments", "ai_feedback_events", "semantic_event_reports"} {
		var count int
		if err := state.db.QueryRow(`SELECT count(*) FROM ` + table).Scan(&count); err != nil || count != 1 {
			t.Fatalf("%s count=%d %v", table, count, err)
		}
	}
	items, err := state.ListTimeline(ctx, 20, 0)
	if err != nil || len(items) != 0 {
		t.Fatalf("expired batch revealed: %+v %v", items, err)
	}
	health, err := state.DatabaseHealth(ctx)
	if err != nil || health.Status != "healthy" {
		t.Fatalf("health=%+v %v", health, err)
	}
}

func lifecycleLegacyDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "legacy.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	legacy := strings.TrimSuffix(schemaSQL, timelineActiveLifecycleSQL+timelineRetentionSQL+semanticEvidenceOwnershipSQL+splitActionAuditSQL)
	legacy = strings.ReplaceAll(legacy, "CREATE INDEX IF NOT EXISTS timeline_presented ON timeline_items(presented_at DESC);\n", "")
	for _, table := range []string{"timeline_items", "ai_assessments", "media_provenance_assessments", "feedback_events", "semantic_event_reports"} {
		start := strings.Index(legacy, "CREATE TABLE IF NOT EXISTS "+table+" (")
		end := start + strings.Index(legacy[start:], "\n);") + 3
		ddl := legacy[start:end]
		ddl = strings.ReplaceAll(ddl, "session_id TEXT NOT NULL,", "session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,")
		ddl = strings.ReplaceAll(ddl, "run_id TEXT NOT NULL,", "run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,")
		for _, column := range []string{"  origin_status TEXT NOT NULL DEFAULT 'unavailable',\n", "  presentation TEXT NOT NULL DEFAULT '',\n", "  presented_at TEXT,\n", "  batch_state TEXT NOT NULL DEFAULT '',\n", "  evidence_snapshot_json TEXT,\n"} {
			ddl = strings.ReplaceAll(ddl, column, "")
		}
		legacy = legacy[:start] + ddl + legacy[end:]
	}
	if _, err = db.Exec(`PRAGMA foreign_keys=ON;` + legacy + `INSERT INTO meta VALUES('schema_version','26'); CREATE INDEX lifecycle_extra_index ON timeline_items(evidence_key); CREATE TRIGGER lifecycle_extra_trigger AFTER INSERT ON feedback_events BEGIN SELECT 1; END;`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`
INSERT INTO source_definitions VALUES('x','X',0,1);
INSERT INTO sessions(id,intent,status,max_items_per_source,max_items_total,created_at,completed_at) VALUES('origin','test','completed',1,1,'2026-01-01','2026-01-01');
INSERT INTO runs(id,session_id,source,ordinal,status,stage,created_at) VALUES('run','origin','x',0,'completed','done','2026-01-01');
INSERT INTO timeline_items(id,session_id,run_id,source,evidence_key,rank,item_json,assessment_json,created_at) VALUES('post','origin','run','x','evidence',0,'{"whatChanged":"preserve"}','{}','2026-01-01');
INSERT INTO feedback_events(id,timeline_id,session_id,run_id,evidence_key,direction,created_at) VALUES('feedback','post','origin','run','evidence','more','2026-01-01');
`); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestTimelineLifecycleMigrationAtomicAndMatchesFresh(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "rollback"}[conflict], func(t *testing.T) {
			db := lifecycleLegacyDB(t)
			ctx := context.Background()
			if conflict {
				if _, err := db.Exec(`CREATE TABLE feedback_events_lifecycle_v27(id TEXT)`); err != nil {
					t.Fatal(err)
				}
			}
			err := migrateSchema26To27(ctx, db)
			if conflict && err == nil {
				t.Fatal("expected atomic failure")
			}
			if !conflict && err != nil {
				t.Fatal(err)
			}
			var version string
			if err := db.QueryRow(`SELECT value FROM meta WHERE key='schema_version'`).Scan(&version); err != nil {
				t.Fatal(err)
			}
			want := "27"
			if conflict {
				want = "26"
			}
			if version != want {
				t.Fatalf("version=%s", version)
			}
			if err := requireForeignKeys(ctx, db); err != nil {
				t.Fatal(err)
			}
			var objects int
			if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name IN ('lifecycle_extra_index','lifecycle_extra_trigger')`).Scan(&objects); err != nil || objects != 2 {
				t.Fatalf("objects %d %v", objects, err)
			}
			if conflict {
				var count int
				if err := db.QueryRow(`SELECT count(*) FROM pragma_foreign_key_list('timeline_items') WHERE "table" IN ('sessions','runs')`).Scan(&count); err != nil || count != 2 {
					t.Fatalf("rollback lost FKs: %d %v", count, err)
				}
				return
			}
			// The migration itself installs lifecycle triggers, before Open's
			// subsequent canonical schema initialization can run.
			if _, err := db.Exec(`DELETE FROM sessions WHERE id='origin'`); err != nil {
				t.Fatal(err)
			}
			var preserved int
			if err := db.QueryRow(`SELECT count(*) FROM timeline_items t JOIN feedback_events f ON f.timeline_id=t.id WHERE t.item_json='{"whatChanged":"preserve"}' AND t.session_id='origin' AND t.origin_status='completed'`).Scan(&preserved); err != nil || preserved != 1 {
				t.Fatalf("migration/delete lost content %d %v", preserved, err)
			}
			var operational int
			if err := db.QueryRow(`SELECT count(*) FROM runs`).Scan(&operational); err != nil || operational != 0 {
				t.Fatalf("run did not cascade %d %v", operational, err)
			}
			fresh := openTestStore(t)
			for _, table := range []string{"timeline_items", "ai_assessments", "media_provenance_assessments", "feedback_events", "semantic_event_reports"} {
				for _, pragma := range []string{"pragma_table_info", "pragma_foreign_key_list"} {
					read := func(db *sql.DB) string {
						rows, err := db.Query(`SELECT * FROM ` + pragma + `('` + table + `')`)
						if err != nil {
							t.Fatal(err)
						}
						defer rows.Close()
						cols, _ := rows.Columns()
						var records []string
						for rows.Next() {
							values := make([]any, len(cols))
							ptrs := make([]any, len(cols))
							for i := range values {
								ptrs[i] = &values[i]
							}
							if err := rows.Scan(ptrs...); err != nil {
								t.Fatal(err)
							} // compare fields independent of column ordering
							if pragma == "pragma_table_info" {
								values[0] = nil
							}
							records = append(records, fmt.Sprint(values))
						}
						sort.Strings(records)
						return strings.Join(records, "\n")
					}
					if got, want := read(db), read(fresh.db); got != want {
						t.Fatalf("%s %s mismatch\ngot %s\nwant %s", table, pragma, got, want)
					}
				}
			}
		})
	}
}

func TestTimelineLifecycleMigratesAlreadyDetachedHistoryWithoutInventingOrigin(t *testing.T) {
	db := lifecycleLegacyDB(t)
	ctx := context.Background()
	if _, err := db.Exec(`PRAGMA foreign_keys=OFF; DELETE FROM sessions WHERE id='origin'; PRAGMA foreign_keys=ON;`); err != nil {
		t.Fatal(err)
	}
	if err := migrateSchema26To27(ctx, db); err != nil {
		t.Fatal(err)
	}
	// The unrelated orphan run still requires normal maintenance; removing it
	// must not remove the migrated product row or its learning evidence.
	if _, err := db.Exec(`DELETE FROM runs WHERE id='run'`); err != nil {
		t.Fatal(err)
	}
	state := &Store{db: db}
	feedback, err := state.AddFeedback(ctx, "post", domain.Feedback{Direction: "more"})
	if err != nil {
		t.Fatal(err)
	}
	var status string
	var parents, learning int
	if err := db.QueryRow(`SELECT origin_status FROM timeline_items WHERE id='post'`).Scan(&status); err != nil || status != "unavailable" {
		t.Fatalf("invented status=%s %v", status, err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM sessions`).Scan(&parents); err != nil || parents != 0 {
		t.Fatalf("invented parents=%d %v", parents, err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM preference_learning_ledger WHERE event_id=?`, feedback.ID).Scan(&learning); err != nil || learning != 1 {
		t.Fatalf("learning=%d %v", learning, err)
	}
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("detached durable history is an FK violation")
	}
}
