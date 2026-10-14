package store

import (
	"context"
	"database/sql"
)

// fixtureCapturePausePrefix is shared with the opt-in fixture pause APIs. The
// claim guard must also be present in ordinary builds so they honor markers
// written by a fixture-enabled build against the same database.
const fixtureCapturePausePrefix = "fixture_capture_pause:"

func fixtureCaptureClaimPaused(ctx context.Context, tx *sql.Tx, runID string) (bool, error) {
	var count int
	err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM meta m JOIN runs r ON m.key=? || r.session_id WHERE r.id=?`, fixtureCapturePausePrefix, runID).Scan(&count)
	return count != 0, err
}
