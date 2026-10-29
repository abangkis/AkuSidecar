package store

import (
	"context"
	"encoding/json"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

// HasDailySourceFence is a read-only recovery check. Route changes cannot
// discard a durable permit, and ordinary source runs are not reclassified.
func (s *Store) HasDailySourceFence(ctx context.Context, source domain.Source, runID string) (bool, error) {
	if !supportedDailySource(source) {
		return false, nil
	}
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM runs r JOIN meta m ON m.key=?||r.session_id||? WHERE r.id=? AND r.source=? AND json_extract(m.value,'$.runScoped')=1 AND json_extract(m.value,'$.runId')=r.id`, manualPilotSessionPrefix, manualScopeID("", source), runID, source).Scan(&count)
	return count == 1, err
}

// The receipt must be backed by the private daily issuance record; arbitrary
// observation coverage alone cannot select this acquisition policy.
func (s *Store) IsDailyXCapture(ctx context.Context, runID string) (bool, error) {
	return s.isDailySourceCapture(ctx, domain.SourceX, runID)
}
func (s *Store) IsDailySourceCapture(ctx context.Context, runID string) (bool, error) {
	var source domain.Source
	if err := s.db.QueryRowContext(ctx, `SELECT source FROM runs WHERE id=?`, runID).Scan(&source); err != nil {
		return false, err
	}
	if !supportedDailySource(source) {
		return false, nil
	}
	return s.isDailySourceCapture(ctx, source, runID)
}
func (s *Store) isDailySourceCapture(ctx context.Context, source domain.Source, runID string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM runs r JOIN meta m ON m.key=?||r.session_id||? WHERE r.id=? AND r.source=? AND json_extract(m.value,'$.runScoped')=1 AND json_extract(m.value,'$.runId')=r.id AND json_extract(r.coverage_json,?)='round_one_v1'`, manualPilotSessionPrefix, manualScopeID("", source), runID, source, dailyAcquisitionPath(source)).Scan(&count)
	return count == 1, err
}

// RejectUnavailableXCapture reconciles ordinary pre-restart capture only. It
// grants no capture authority and never changes a protected pilot session.
// Accepted observations survive: an interrupted follow-up returns to reasoning.
func (s *Store) RejectUnavailableXCapture(ctx context.Context, runID string) error {
	return s.RejectUnavailableSourceCapture(ctx, domain.SourceX, runID)
}

// Used after a daily collector closes. A live/protected permit is never bypassed.
func (s *Store) ReconcileClosedDailyXCapture(ctx context.Context, runID string) error {
	return s.ReconcileClosedDailySourceCapture(ctx, domain.SourceX, runID)
}
func (s *Store) RejectUnavailableSourceCapture(ctx context.Context, expected domain.Source, runID string) error {
	return s.reconcileUnavailableSourceCapture(ctx, expected, runID, false)
}
func (s *Store) ReconcileClosedDailySourceCapture(ctx context.Context, expected domain.Source, runID string) error {
	return s.reconcileUnavailableSourceCapture(ctx, expected, runID, true)
}
func (s *Store) reconcileUnavailableSourceCapture(ctx context.Context, expected domain.Source, runID string, closedDaily bool) error {
	if !supportedDailySource(expected) {
		return errManualPilot
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var source domain.Source
	var status string
	var observations int
	if err = tx.QueryRowContext(ctx, `SELECT source,status,(SELECT COUNT(*) FROM observations WHERE run_id=r.id) FROM runs r WHERE id=?`, runID).Scan(&source, &status, &observations); err != nil {
		return err
	}
	if source != expected || status != "queued" && status != "waiting_for_bridge" {
		return nil
	}
	if err = rejectManualPilotRun(ctx, tx, runID); err != nil {
		if !closedDaily {
			return err
		}
		var session string
		if loadErr := tx.QueryRowContext(ctx, `SELECT session_id FROM runs WHERE id=?`, runID).Scan(&session); loadErr != nil {
			return loadErr
		}
		state, _, loadErr := readManualState(ctx, tx, manualScopeID(session, source))
		if loadErr != nil || !state.RunScoped || !state.Closed || state.RunID != runID {
			return err
		}
	}
	now := s.Now().UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
	failure := domain.Failure{Code: "x_concurrent_not_ready", Stage: "capture", Message: "X concurrent capture is not ready: batch-adapter-required", Retryable: true}
	if closedDaily {
		failure.Code = "x_daily_capture_stopped"
		failure.Message = "Daily X capture stopped; inspect runtime status before requesting new work."
	}
	if source != domain.SourceX {
		failure.Code = "instagram_concurrent_not_ready"
		failure.Message = "Instagram concurrent capture is not ready"
		if closedDaily {
			failure.Code = "instagram_daily_capture_stopped"
			failure.Message = "Daily Instagram capture stopped; inspect runtime status before requesting new work."
		}
	}
	raw, _ := json.Marshal(failure)
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
