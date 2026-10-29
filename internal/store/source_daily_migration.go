package store

import (
	"context"
	"database/sql"
	"errors"
)

// Schema 31 is a semantic barrier: schema 30 knows only X run-scoped fences.
// No permission, account, or queued work is created or rewritten by migration.
func migrateSchema30To31(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	r, err := tx.ExecContext(ctx, `UPDATE meta SET value='31' WHERE key='schema_version' AND value='30'`)
	if err != nil {
		return err
	}
	n, err := r.RowsAffected()
	if err != nil || n != 1 {
		return errors.New("source daily migration requires schema 30")
	}
	return tx.Commit()
}
