package store

import (
	"context"
	"database/sql"
	"fmt"
)

// SQLite cannot extend a CHECK constraint in place. Rebuild only this leaf
// table, preserving all rows and explicitly defined indexes/triggers atomically.
func migrateSchema25To26(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exists int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type='table' AND name='capture_surface_events'`).Scan(&exists); err != nil {
		return err
	}
	// Older supported schemas may not have this optional telemetry table yet.
	// Open applies the canonical CREATE IF NOT EXISTS schema after migrations.
	if exists == 0 {
		if _, err = tx.ExecContext(ctx, `UPDATE meta SET value='26' WHERE key='schema_version'`); err != nil {
			return err
		}
		return tx.Commit()
	}
	rows, err := tx.QueryContext(ctx, `SELECT sql FROM sqlite_master WHERE tbl_name='capture_surface_events' AND type IN ('index','trigger') AND sql IS NOT NULL ORDER BY type,name`)
	if err != nil {
		return err
	}
	var definitions []string
	for rows.Next() {
		var definition string
		if err = rows.Scan(&definition); err != nil {
			rows.Close()
			return err
		}
		definitions = append(definitions, definition)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
CREATE TABLE capture_surface_events_v26 (
 id TEXT PRIMARY KEY,
 session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
 run_id TEXT REFERENCES runs(id) ON DELETE CASCADE,
 source TEXT REFERENCES source_definitions(id),
 event TEXT NOT NULL CHECK (event IN ('created','reused','release_requested','released','preserved_user_owned','focus_intervention','reconciled','native_trace')),
 outcome TEXT NOT NULL DEFAULT '',
 detail_json TEXT NOT NULL DEFAULT '{}',
 occurred_at TEXT NOT NULL
);
INSERT INTO capture_surface_events_v26 SELECT id,session_id,run_id,source,event,outcome,detail_json,occurred_at FROM capture_surface_events;
DROP TABLE capture_surface_events;
ALTER TABLE capture_surface_events_v26 RENAME TO capture_surface_events;
`)
	if err != nil {
		return fmt.Errorf("migrate native capture event contract: %w", err)
	}
	for _, definition := range definitions {
		if _, err = tx.ExecContext(ctx, definition); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE meta SET value='26' WHERE key='schema_version'`); err != nil {
		return err
	}
	return tx.Commit()
}
