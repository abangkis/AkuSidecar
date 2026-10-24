package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

const xCapturePauseKey = "source_capture_pause:x"

var ErrSourceCapturePaused = errors.New(domain.XChallengeMessage)

func (s *Store) SourceCapturePause(ctx context.Context) (*domain.SourceCapturePause, error) {
	var raw string
	if err := s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key=?`, xCapturePauseKey).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	var pause domain.SourceCapturePause
	if err := json.Unmarshal([]byte(raw), &pause); err != nil {
		return nil, err
	}
	return &pause, nil
}

func requireSourceCaptureAllowed(ctx context.Context, tx *sql.Tx, source domain.Source) error {
	if source != domain.SourceX {
		return nil
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM meta WHERE key=?`, xCapturePauseKey).Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return ErrSourceCapturePaused
	}
	return nil
}

// The first reason survives duplicates and process restarts. Outstanding work
// is terminated, not recycled: clearing the latch cannot accept stale results.
func (s *Store) pauseXCaptureTx(ctx context.Context, tx *sql.Tx, exceptRun string, category string) error {
	pause := domain.SourceCapturePause{ID: domain.NewID("pause"), Source: domain.SourceX, Code: "challenge_required", Message: domain.XChallengeMessage, PausedAt: s.Now().UTC().Format(time.RFC3339Nano)}
	pause.Category = category
	raw, _ := json.Marshal(pause)
	if _, err := tx.ExecContext(ctx, `INSERT INTO meta(key,value) VALUES(?,?) ON CONFLICT(key) DO NOTHING`, xCapturePauseKey, string(raw)); err != nil {
		return err
	}
	failure, _ := json.Marshal(domain.Failure{Code: "challenge_required", Stage: "capture", Message: domain.XChallengeMessage, Retryable: false})
	if _, err := tx.ExecContext(ctx, `UPDATE bridge_commands SET status='failed',completed_at=?,error_json=? WHERE status IN ('queued','claimed') AND run_id IN (SELECT id FROM runs WHERE source='x')`, pause.PausedAt, string(failure)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE runs SET status='failed',stage='capture',completed_at=?,error_json=? WHERE source='x' AND id!=? AND status IN ('queued','waiting_for_bridge')`, pause.PausedAt, string(failure), exceptRun); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `UPDATE media_recaptures SET status='failed',completed_at=?,error_json=? WHERE source='x' AND status IN ('queued','claimed')`, pause.PausedAt, string(failure))
	return err
}

func (s *Store) PauseXCapture(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := s.pauseXCaptureTx(ctx, tx, "", ""); err != nil {
		return err
	}
	return tx.Commit()
}

// Confirmation permits a new attempt; it does not claim the website is healthy.
// An old UI cannot clear a newer challenge.
func (s *Store) ConfirmXCaptureRecovery(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("a current pause ID is required")
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM meta WHERE key=? AND json_extract(value,'$.id')=?`, xCapturePauseKey, id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return errors.New("capture pause changed; reload before confirming recovery")
	}
	return nil
}
