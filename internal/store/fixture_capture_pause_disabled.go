//go:build !auth_session_fixture

package store

import (
	"context"
	"database/sql"
)

func fixtureCaptureClaimPaused(context.Context, *sql.Tx, string) (bool, error) {
	return false, nil
}
