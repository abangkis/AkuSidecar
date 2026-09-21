package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func nativeTraceLegacyDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "legacy.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	_, err = db.Exec(`
PRAGMA foreign_keys=ON;
CREATE TABLE meta(key TEXT PRIMARY KEY,value TEXT);
INSERT INTO meta VALUES('schema_version','25');
CREATE TABLE sessions(id TEXT PRIMARY KEY);
CREATE TABLE runs(id TEXT PRIMARY KEY);
CREATE TABLE source_definitions(id TEXT PRIMARY KEY);
INSERT INTO sessions VALUES('session'); INSERT INTO runs VALUES('run'); INSERT INTO source_definitions VALUES('x');
CREATE TABLE capture_surface_events (
 id TEXT PRIMARY KEY,session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
 run_id TEXT REFERENCES runs(id) ON DELETE CASCADE,source TEXT REFERENCES source_definitions(id),
 event TEXT NOT NULL CHECK(event IN ('created','reused','release_requested','released','preserved_user_owned','focus_intervention','reconciled')),
 outcome TEXT NOT NULL DEFAULT '',detail_json TEXT NOT NULL DEFAULT '{}',occurred_at TEXT NOT NULL
);
CREATE INDEX capture_surface_events_run_occurred ON capture_surface_events(run_id,occurred_at);
CREATE INDEX capture_surface_events_session_source ON capture_surface_events(session_id,source,occurred_at);
CREATE INDEX extra_capture_index ON capture_surface_events(outcome);
CREATE TRIGGER extra_capture_trigger BEFORE INSERT ON capture_surface_events BEGIN SELECT 1; END;
INSERT INTO capture_surface_events VALUES('legacy','session','run','x','created','legacy-outcome','{"preserve":"verbatim"}','2026-09-20T00:00:00Z');
`)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestNativeCaptureMigrationPreservesRowsIndexesTriggersAndForeignKeys(t *testing.T) {
	db := nativeTraceLegacyDB(t)
	if err := migrateSchema25To26(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	var version, detail string
	if err := db.QueryRow(`SELECT value FROM meta WHERE key='schema_version'`).Scan(&version); err != nil || version != "26" {
		t.Fatalf("version=%s err=%v", version, err)
	}
	if err := db.QueryRow(`SELECT detail_json FROM capture_surface_events WHERE id='legacy' AND session_id='session' AND run_id='run' AND source='x' AND event='created' AND outcome='legacy-outcome' AND occurred_at='2026-09-20T00:00:00Z'`).Scan(&detail); err != nil || detail != `{"preserve":"verbatim"}` {
		t.Fatalf("legacy=%s err=%v", detail, err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE tbl_name='capture_surface_events' AND sql IS NOT NULL AND type IN ('index','trigger')`).Scan(&count); err != nil || count != 4 {
		t.Fatalf("definitions=%d err=%v", count, err)
	}
	if _, err := db.Exec(`INSERT INTO capture_surface_events VALUES('native','session','run','x','native_trace','','{}','2026-09-21T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM runs WHERE id='run'`); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM capture_surface_events`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("cascade count=%d err=%v", count, err)
	}
}

func TestNativeCaptureMigrationRollsBackAfterReplacementFailure(t *testing.T) {
	db := nativeTraceLegacyDB(t)
	if _, err := db.Exec(`CREATE TRIGGER reject_schema_update BEFORE UPDATE ON meta BEGIN SELECT RAISE(ABORT,'injected failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := migrateSchema25To26(context.Background(), db); err == nil {
		t.Fatal("expected migration failure")
	}
	var version, detail string
	if err := db.QueryRow(`SELECT value FROM meta WHERE key='schema_version'`).Scan(&version); err != nil || version != "25" {
		t.Fatalf("version=%s err=%v", version, err)
	}
	if err := db.QueryRow(`SELECT detail_json FROM capture_surface_events WHERE id='legacy'`).Scan(&detail); err != nil || detail != `{"preserve":"verbatim"}` {
		t.Fatalf("legacy=%s err=%v", detail, err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name='capture_surface_events_v26'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("temporary table leaked: %d %v", count, err)
	}
	if _, err := db.Exec(`INSERT INTO capture_surface_events VALUES('native','session','run','x','native_trace','','{}','now')`); err == nil {
		t.Fatal("old CHECK contract was not rolled back")
	}
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE tbl_name='capture_surface_events' AND sql IS NOT NULL AND type IN ('index','trigger')`).Scan(&count); err != nil || count != 4 {
		t.Fatalf("definitions lost: %d %v", count, err)
	}
}
