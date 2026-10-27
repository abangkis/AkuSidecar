package store

import (
	"context"
	"encoding/json"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

// RejectUnavailableXCapture reconciles ordinary pre-restart capture only. It
// grants no capture authority and never changes a protected pilot session.
// Accepted observations survive: an interrupted follow-up returns to reasoning.
func (s *Store) RejectUnavailableXCapture(ctx context.Context, runID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = rejectManualPilotRun(ctx, tx, runID); err != nil {
		return err
	}
	var source domain.Source
	var status string
	var observations int
	if err = tx.QueryRowContext(ctx, `SELECT source,status,(SELECT COUNT(*) FROM observations WHERE run_id=r.id) FROM runs r WHERE id=?`, runID).Scan(&source, &status, &observations); err != nil {
		return err
	}
	if source != domain.SourceX || status != "queued" && status != "waiting_for_bridge" {
		return nil
	}
	now := s.Now().UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
	raw, _ := json.Marshal(domain.Failure{Code: "x_concurrent_not_ready", Stage: "capture", Message: "X concurrent capture is not ready: batch-adapter-required", Retryable: true})
	if _, err = tx.ExecContext(ctx, `UPDATE bridge_commands SET status='failed',completed_at=?,error_json=? WHERE run_id=? AND status IN ('queued','claimed')`, now, string(raw), runID); err != nil {
		return err
	}
	if observations > 0 {
		_, err = tx.ExecContext(ctx, `UPDATE runs SET status='reasoning',stage='reasoning' WHERE id=?`, runID)
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE runs SET status='failed',stage='capture',completed_at=?,error_json=? WHERE id=?`, now, string(raw), runID)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}
