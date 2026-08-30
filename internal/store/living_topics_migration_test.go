package store

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestSchema15MigratesLivingTopicsAtomically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema15.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE meta(key TEXT PRIMARY KEY,value TEXT NOT NULL); INSERT INTO meta(key,value) VALUES('schema_version','15')`); err != nil {
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
	if err := state.db.QueryRow(`SELECT value FROM meta WHERE key='schema_version'`).Scan(&version); err != nil || version != "18" {
		t.Fatalf("schema version=%q err=%v", version, err)
	}
	var count int
	if err := state.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name IN ('living_topics','living_topic_memberships','living_topic_snapshots')`).Scan(&count); err != nil || count != 3 {
		t.Fatalf("Living Topics tables=%d err=%v", count, err)
	}
}

func TestSchema15LivingTopicsMigrationFailurePreservesVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema15-conflict.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		CREATE TABLE meta(key TEXT PRIMARY KEY,value TEXT NOT NULL);
		INSERT INTO meta(key,value) VALUES('schema_version','15');
		CREATE TABLE living_topics(existing TEXT);
	`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path, domain.DefaultSettings("standard", "quiet", "guarded_live", true)); err == nil {
		t.Fatal("conflicting Living Topics table must fail migration")
	}
	check, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer check.Close()
	var version string
	if err := check.QueryRow(`SELECT value FROM meta WHERE key='schema_version'`).Scan(&version); err != nil || version != "15" {
		t.Fatalf("failed migration version=%q err=%v", version, err)
	}
}

func TestSchema16MigratesLivingTopicRoutingAtomically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema16.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE meta(key TEXT PRIMARY KEY,value TEXT NOT NULL); INSERT INTO meta(key,value) VALUES('schema_version','16'); ` + livingTopicsMigrationSQL); err != nil {
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
	if err := state.db.QueryRow(`SELECT value FROM meta WHERE key='schema_version'`).Scan(&version); err != nil || version != "18" {
		t.Fatalf("version=%q err=%v", version, err)
	}
	var count int
	if err := state.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name IN ('living_topic_feedback_events','living_topic_routing_jobs')`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("routing tables=%d err=%v", count, err)
	}
}

func TestSchema17MigratesAutomaticUnderstandingAtomically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema17.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE meta(key TEXT PRIMARY KEY,value TEXT NOT NULL); INSERT INTO meta(key,value) VALUES('schema_version','17'); ` + livingTopicsMigrationSQL + livingTopicsRoutingMigrationSQL); err != nil {
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
	if err := state.db.QueryRow(`SELECT value FROM meta WHERE key='schema_version'`).Scan(&version); err != nil || version != "18" {
		t.Fatalf("version=%q err=%v", version, err)
	}
	var count int
	if err := state.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='living_topic_understanding_jobs'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("understanding tables=%d err=%v", count, err)
	}
}

func TestSchema17UnderstandingMigrationFailurePreservesVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema17-conflict.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE meta(key TEXT PRIMARY KEY,value TEXT NOT NULL); INSERT INTO meta(key,value) VALUES('schema_version','17'); ` + livingTopicsMigrationSQL + livingTopicsRoutingMigrationSQL + ` CREATE TABLE living_topic_understanding_jobs(existing TEXT);`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path, domain.DefaultSettings("standard", "quiet", "guarded_live", true)); err == nil {
		t.Fatal("conflicting understanding table must fail migration")
	}
	check, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer check.Close()
	var version string
	if err := check.QueryRow(`SELECT value FROM meta WHERE key='schema_version'`).Scan(&version); err != nil || version != "17" {
		t.Fatalf("failed migration version=%q err=%v", version, err)
	}
}

func TestSchema16RoutingMigrationFailurePreservesVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema16-conflict.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE meta(key TEXT PRIMARY KEY,value TEXT NOT NULL); INSERT INTO meta(key,value) VALUES('schema_version','16'); ` + livingTopicsMigrationSQL + ` CREATE TABLE living_topic_feedback_events(existing TEXT);`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path, domain.DefaultSettings("standard", "quiet", "guarded_live", true)); err == nil {
		t.Fatal("conflicting routing table must fail migration")
	}
	check, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer check.Close()
	var version string
	if err := check.QueryRow(`SELECT value FROM meta WHERE key='schema_version'`).Scan(&version); err != nil || version != "16" {
		t.Fatalf("failed migration version=%q err=%v", version, err)
	}
}
