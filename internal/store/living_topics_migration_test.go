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
	if err := state.db.QueryRow(`SELECT value FROM meta WHERE key='schema_version'`).Scan(&version); err != nil || version != "20" {
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
	if err := state.db.QueryRow(`SELECT value FROM meta WHERE key='schema_version'`).Scan(&version); err != nil || version != "20" {
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
	if err := state.db.QueryRow(`SELECT value FROM meta WHERE key='schema_version'`).Scan(&version); err != nil || version != "20" {
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

func TestSchema18MigratesTopicActivationAtomically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema18.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE meta(key TEXT PRIMARY KEY,value TEXT NOT NULL); INSERT INTO meta(key,value) VALUES('schema_version','18'); ` + livingTopicsMigrationSQL + livingTopicsRoutingMigrationSQL + livingTopicsUnderstandingMigrationSQL + ` INSERT INTO living_topics(id,name,description,created_at,updated_at) VALUES('topic-existing','Codex','Track Codex','2026-08-30T00:00:00Z','2026-08-30T00:00:00Z');`); err != nil {
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
		t.Fatalf("version=%q err=%v", version, err)
	}
	topic, err := state.LivingTopic(t.Context(), "topic-existing")
	if err != nil || topic.CriteriaRevision != 1 || topic.RoutingStatus != "pending" {
		t.Fatalf("topic=%+v err=%v", topic, err)
	}
	var jobs int
	if err := state.db.QueryRow(`SELECT COUNT(*) FROM living_topic_activation_jobs WHERE topic_id='topic-existing' AND status='pending'`).Scan(&jobs); err != nil || jobs != 1 {
		t.Fatalf("activation jobs=%d err=%v", jobs, err)
	}
	if _, err := state.db.Exec(`INSERT INTO living_topic_feedback_events(id,topic_id,memory_item_id,verdict,created_at) VALUES('clear-feedback','topic-existing','memory-example','clear','2026-08-30T00:01:00Z')`); err != nil {
		t.Fatalf("clear feedback unsupported: %v", err)
	}
}

func TestSchema18ActivationMigrationFailurePreservesVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema18-conflict.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE meta(key TEXT PRIMARY KEY,value TEXT NOT NULL); INSERT INTO meta(key,value) VALUES('schema_version','18'); ` + livingTopicsMigrationSQL + livingTopicsRoutingMigrationSQL + livingTopicsUnderstandingMigrationSQL + ` CREATE TABLE living_topic_activation_jobs(existing TEXT);`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path, domain.DefaultSettings("standard", "quiet", "guarded_live", true)); err == nil {
		t.Fatal("conflicting activation table must fail migration")
	}
	check, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer check.Close()
	var version string
	if err := check.QueryRow(`SELECT value FROM meta WHERE key='schema_version'`).Scan(&version); err != nil || version != "18" {
		t.Fatalf("failed migration version=%q err=%v", version, err)
	}
}

func TestSchema19MigratesLivingTopicNotificationsWithoutInventingUnreadEvidence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema19.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	setup := `CREATE TABLE meta(key TEXT PRIMARY KEY,value TEXT NOT NULL); INSERT INTO meta(key,value) VALUES('schema_version','19'); ` +
		livingTopicsMigrationSQL + livingTopicsRoutingMigrationSQL + livingTopicsUnderstandingMigrationSQL + livingTopicsActivationMigrationSQL +
		` INSERT INTO living_topics(id,name,description,created_at,updated_at) VALUES('topic-existing','Codex','Track Codex','2026-08-30T00:00:00Z','2026-08-30T00:00:00Z');`
	if _, err := db.Exec(setup); err != nil {
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
		t.Fatalf("version=%q err=%v", version, err)
	}
	topic, err := state.LivingTopic(t.Context(), "topic-existing")
	if err != nil || topic.NewEvidenceCount != 0 || topic.NewEvidenceAt != "" || topic.EvidenceSeenAt != "" {
		t.Fatalf("topic=%+v err=%v", topic, err)
	}
}

func TestSchema19NotificationMigrationFailurePreservesVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema19-conflict.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	setup := `CREATE TABLE meta(key TEXT PRIMARY KEY,value TEXT NOT NULL); INSERT INTO meta(key,value) VALUES('schema_version','19'); ` +
		livingTopicsMigrationSQL + livingTopicsRoutingMigrationSQL + livingTopicsUnderstandingMigrationSQL + livingTopicsActivationMigrationSQL +
		` ALTER TABLE living_topic_memberships ADD COLUMN new_evidence INTEGER NOT NULL DEFAULT 0;`
	if _, err := db.Exec(setup); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path, domain.DefaultSettings("standard", "quiet", "guarded_live", true)); err == nil {
		t.Fatal("conflicting notification column must fail migration")
	}
	check, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer check.Close()
	var version string
	if err := check.QueryRow(`SELECT value FROM meta WHERE key='schema_version'`).Scan(&version); err != nil || version != "19" {
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
