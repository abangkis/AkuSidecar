package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestSchema21AddsLivingTopicModelUsageLedger(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema21.db")
	state, err := Open(path, domain.DefaultSettings("standard", "quiet", "guarded_live", true))
	if err != nil {
		t.Fatal(err)
	}
	if err := state.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		DROP TABLE living_topic_model_invocations;
		UPDATE meta SET value='21' WHERE key='schema_version';
		INSERT INTO living_topics(id,name,created_at,updated_at) VALUES('topic-cost','Codex','2026-08-30T00:00:00Z','2026-08-30T00:00:00Z');
		INSERT INTO living_topic_snapshots(id,topic_id,version,status,input_digest,provider,model,effort,duration_ms,usage_json,created_at)
		VALUES('snapshot-cost','topic-cost',1,'ready','digest','structured-inference','gemini-test','high',500,'{"inputTokens":120,"cachedInputTokens":20,"outputTokens":30,"reasoningOutputTokens":40,"modelDescriptorVersion":"catalog-1","modelMaturity":"stable"}','2026-08-30T00:00:00Z');
	`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	state, err = Open(path, domain.DefaultSettings("standard", "quiet", "guarded_live", true))
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	var version string
	var tables int
	if err := state.db.QueryRow(`SELECT value FROM meta WHERE key='schema_version'`).Scan(&version); err != nil || version != "23" {
		t.Fatalf("version=%q err=%v", version, err)
	}
	if err := state.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='living_topic_model_invocations'`).Scan(&tables); err != nil || tables != 1 {
		t.Fatalf("usage ledger=%d err=%v", tables, err)
	}
	var input, output int64
	if err := state.db.QueryRow(`SELECT input_tokens,output_tokens FROM living_topic_model_invocations WHERE id='topic_snapshot_usage:snapshot-cost'`).Scan(&input, &output); err != nil || input != 120 || output != 30 {
		t.Fatalf("backfilled usage input=%d output=%d err=%v", input, output, err)
	}
	var contract, understandingStatus string
	if err := state.db.QueryRow(`SELECT contract_version FROM living_topic_snapshots WHERE id='snapshot-cost'`).Scan(&contract); err != nil || contract != "legacy-v1" {
		t.Fatalf("legacy contract=%q err=%v", contract, err)
	}
	if err := state.db.QueryRow(`SELECT understanding_status FROM living_topics WHERE id='topic-cost'`).Scan(&understandingStatus); err != nil || understandingStatus != "pending" {
		t.Fatalf("rebaseline status=%q err=%v", understandingStatus, err)
	}
	var pending int
	if err := state.db.QueryRow(`SELECT COUNT(*) FROM living_topic_understanding_jobs WHERE topic_id='topic-cost' AND status='pending' AND trigger='migration_rebaseline'`).Scan(&pending); err != nil || pending != 1 {
		t.Fatalf("rebaseline jobs=%d err=%v", pending, err)
	}
}

func TestSchema21LivingTopicModelUsageConflictPreservesVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema21-conflict.db")
	state, err := Open(path, domain.DefaultSettings("standard", "quiet", "guarded_live", true))
	if err != nil {
		t.Fatal(err)
	}
	if err := state.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP TABLE living_topic_model_invocations; CREATE TABLE living_topic_model_invocations(existing TEXT); UPDATE meta SET value='21' WHERE key='schema_version'`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path, domain.DefaultSettings("standard", "quiet", "guarded_live", true)); err == nil {
		t.Fatal("conflicting model usage ledger must fail migration")
	}
	check, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer check.Close()
	var version string
	if err := check.QueryRowContext(context.Background(), `SELECT value FROM meta WHERE key='schema_version'`).Scan(&version); err != nil || version != "21" {
		t.Fatalf("failed migration version=%q err=%v", version, err)
	}
}

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
	if err := state.db.QueryRow(`SELECT value FROM meta WHERE key='schema_version'`).Scan(&version); err != nil || version != "23" {
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
	if err := state.db.QueryRow(`SELECT value FROM meta WHERE key='schema_version'`).Scan(&version); err != nil || version != "23" {
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
	if err := state.db.QueryRow(`SELECT value FROM meta WHERE key='schema_version'`).Scan(&version); err != nil || version != "23" {
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
	if err := state.db.QueryRow(`SELECT value FROM meta WHERE key='schema_version'`).Scan(&version); err != nil || version != "23" {
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
	if err := state.db.QueryRow(`SELECT value FROM meta WHERE key='schema_version'`).Scan(&version); err != nil || version != "23" {
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

func TestSchema20MigratesReversibleLivingTopicMovesAtomically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema20.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	setup := `CREATE TABLE meta(key TEXT PRIMARY KEY,value TEXT NOT NULL); INSERT INTO meta(key,value) VALUES('schema_version','20'); ` +
		livingTopicsMigrationSQL + livingTopicsRoutingMigrationSQL + livingTopicsUnderstandingMigrationSQL + livingTopicsActivationMigrationSQL + livingTopicsNewEvidenceMigrationSQL
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
	if err := state.db.QueryRow(`SELECT value FROM meta WHERE key='schema_version'`).Scan(&version); err != nil || version != "23" {
		t.Fatalf("version=%q err=%v", version, err)
	}
	var tableCount, columnCount int
	if err := state.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='living_topic_membership_moves'`).Scan(&tableCount); err != nil || tableCount != 1 {
		t.Fatalf("move table count=%d err=%v", tableCount, err)
	}
	if err := state.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('living_topic_memberships') WHERE name='move_id'`).Scan(&columnCount); err != nil || columnCount != 1 {
		t.Fatalf("move_id column count=%d err=%v", columnCount, err)
	}
}

func TestSchema20MoveMigrationFailurePreservesVersionAndMembershipShape(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schema20-conflict.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	setup := `CREATE TABLE meta(key TEXT PRIMARY KEY,value TEXT NOT NULL); INSERT INTO meta(key,value) VALUES('schema_version','20'); ` +
		livingTopicsMigrationSQL + livingTopicsRoutingMigrationSQL + livingTopicsUnderstandingMigrationSQL + livingTopicsActivationMigrationSQL + livingTopicsNewEvidenceMigrationSQL +
		` CREATE TABLE living_topic_membership_moves(existing TEXT);`
	if _, err := db.Exec(setup); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path, domain.DefaultSettings("standard", "quiet", "guarded_live", true)); err == nil {
		t.Fatal("conflicting move table must fail migration")
	}
	check, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer check.Close()
	var version string
	if err := check.QueryRow(`SELECT value FROM meta WHERE key='schema_version'`).Scan(&version); err != nil || version != "20" {
		t.Fatalf("failed migration version=%q err=%v", version, err)
	}
	var columnCount int
	if err := check.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('living_topic_memberships') WHERE name='move_id'`).Scan(&columnCount); err != nil || columnCount != 0 {
		t.Fatalf("rolled-back move_id column count=%d err=%v", columnCount, err)
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
