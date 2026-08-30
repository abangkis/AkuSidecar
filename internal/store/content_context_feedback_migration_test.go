package store

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestSchema14MigratesContentContextFeedbackAtomically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema14.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE meta(key TEXT PRIMARY KEY,value TEXT NOT NULL); INSERT INTO meta(key,value) VALUES('schema_version','14')`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	state, err := Open(path, domain.DefaultSettings("standard", "quiet", "guarded_live", true))
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	var version string
	if err := state.db.QueryRow(`SELECT value FROM meta WHERE key='schema_version'`).Scan(&version); err != nil || version != "20" {
		t.Fatalf("schema version=%q err=%v", version, err)
	}
	var table int
	if err := state.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='content_context_feedback_events'`).Scan(&table); err != nil || table != 1 {
		t.Fatalf("feedback table=%d err=%v", table, err)
	}
}

func TestSchema14FeedbackMigrationFailurePreservesVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema14-conflict.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		CREATE TABLE meta(key TEXT PRIMARY KEY,value TEXT NOT NULL);
		INSERT INTO meta(key,value) VALUES('schema_version','14');
		CREATE TABLE content_context_feedback_events(existing TEXT);
	`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path, domain.DefaultSettings("standard", "quiet", "guarded_live", true)); err == nil {
		t.Fatal("conflicting feedback table must fail migration")
	}
	check, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer check.Close()
	var version string
	if err := check.QueryRow(`SELECT value FROM meta WHERE key='schema_version'`).Scan(&version); err != nil || version != "14" {
		t.Fatalf("failed migration version=%q err=%v", version, err)
	}
}
