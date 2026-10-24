package store

import (
	"context"
	"database/sql"
	"errors"
)

// Schema 30 is a semantic compatibility barrier: older binaries must not open
// pilot-bound work whose result fence they cannot enforce. No permit is created,
// restored or removed, and existing queue/checkpoint records stay unchanged.
func migrateSchema29To30(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE meta SET value='30' WHERE key='schema_version' AND value='29'`)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil || count != 1 {
		return errors.New("manual pilot migration requires schema 29")
	}
	return tx.Commit()
}
