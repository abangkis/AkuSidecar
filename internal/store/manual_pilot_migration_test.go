package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestManualPilotSchemaMigrationPreservesExistingRecords(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "synthetic.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, `CREATE TABLE meta(key TEXT PRIMARY KEY,value TEXT NOT NULL); INSERT INTO meta VALUES('schema_version','29'),('synthetic_account_checkpoint_required','existing-protection'),('fixture_capture_pause_session','1'); CREATE TABLE queue_sentinel(id TEXT PRIMARY KEY,status TEXT); INSERT INTO queue_sentinel VALUES('queued','queued'),('claimed','claimed');`); err != nil {
		t.Fatal(err)
	}
	if err := migrateSchema29To30(ctx, db); err != nil {
		t.Fatal(err)
	}
	var version, checkpoint, pause string
	if err := db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key='schema_version'`).Scan(&version); err != nil || version != "30" {
		t.Fatalf("schema barrier missing: %s %v", version, err)
	}
	if err := db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key='synthetic_account_checkpoint_required'`).Scan(&checkpoint); err != nil || checkpoint != "existing-protection" {
		t.Fatal("checkpoint protection changed")
	}
	if err := db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key='fixture_capture_pause_session'`).Scan(&pause); err != nil || pause != "1" {
		t.Fatal("pause marker changed")
	}
	var records int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM queue_sentinel WHERE (id='queued' AND status='queued') OR (id='claimed' AND status='claimed')`).Scan(&records); err != nil || records != 2 {
		t.Fatal("migration changed queued/claimed work")
	}
	if migrateSchema29To30(ctx, db) == nil {
		t.Fatal("migration replay should not recreate or bypass the schema barrier")
	}
}
