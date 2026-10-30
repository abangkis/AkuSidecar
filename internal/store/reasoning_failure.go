package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

var errReasoningFailureTransition = errors.New("captured reasoning failure transition rejected")

// FailCapturedRunReasoning records failure after capture without granting capture
// authority. Native daily runs require their durable accepted-result receipt;
// the capture permit may already be expired or closed. Generic FailRun remains
// fenced. Ordinary reasoning runs retain their existing failure behavior.
func (s *Store) FailCapturedRunReasoning(ctx context.Context, expected domain.Run, failure domain.Failure) error {
	if expected.ID == "" || expected.SessionID == "" || expected.Status != "reasoning" || failure.Stage != "reasoning" {
		return errReasoningFailureTransition
	}
	raw, err := json.Marshal(failure)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var session, source, status, stage, sessionStatus, prior string
	if err = tx.QueryRowContext(ctx, `SELECT r.session_id,r.source,r.status,r.stage,s.status,COALESCE(r.error_json,'') FROM runs r JOIN sessions s ON s.id=r.session_id WHERE r.id=?`, expected.ID).Scan(&session, &source, &status, &stage, &sessionStatus, &prior); err != nil {
		return err
	}
	if session != expected.SessionID || source != string(expected.Source) {
		return errReasoningFailureTransition
	}
	if status == "failed" && prior == string(raw) {
		return nil
	}
	if status != "reasoning" || stage != expected.Stage || sessionStatus != "running" {
		return errReasoningFailureTransition
	}
	if fenceErr := rejectManualPilotRun(ctx, tx, expected.ID); fenceErr != nil {
		if !errors.Is(fenceErr, errManualPilot) {
			return fenceErr
		}
		if !supportedDailySource(expected.Source) || stage != "reasoning" && stage != "candidate_evaluation" && stage != "acquisition_planning" {
			return errReasoningFailureTransition
		}
		if err = requireAcceptedDailyResult(ctx, tx, expected); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE runs SET status='failed',stage=?,completed_at=?,error_json=? WHERE id=? AND status='reasoning'`, failure.Stage, s.Now().UTC().Format(time.RFC3339Nano), string(raw), expected.ID); err != nil {
		return err
	}
	return tx.Commit()
}

func requireAcceptedDailyResult(ctx context.Context, tx *sql.Tx, run domain.Run) error {
	state, _, err := readManualState(ctx, tx, manualScopeID(run.SessionID, run.Source))
	if err != nil || !supportedDailySource(run.Source) || !state.RunScoped || state.SessionID != run.SessionID || state.Source != run.Source || state.RunID != run.ID || state.Attempt == "" {
		return errReasoningFailureTransition
	}
	var active, accepted int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM bridge_commands WHERE run_id=? AND status IN ('queued','claimed')`, run.ID).Scan(&active); err != nil {
		return err
	}
	if active != 0 {
		return errReasoningFailureTransition
	}
	// The observation and committed result receipt identify the same completed
	// command and original permit attempt. A live permit is not required.
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*)
	 FROM observations o JOIN bridge_commands c ON c.id=o.command_id AND c.run_id=o.run_id
	 JOIN meta m ON substr(m.key,1,?)=?
	 WHERE o.run_id=? AND o.source=? AND c.status='completed'
	 AND CASE WHEN json_valid(m.value) THEN
	   json_extract(m.value,'$.kind')='result' AND json_extract(m.value,'$.outcome')='committed'
	   AND json_extract(m.value,'$.runId')=o.run_id AND json_extract(m.value,'$.sessionId')=?
	   AND json_extract(m.value,'$.commandId')=c.id AND json_extract(m.value,'$.binding')=?
	 ELSE 0 END`, len(manualPilotOpPrefix), manualPilotOpPrefix, run.ID, run.Source, run.SessionID, state.Attempt).Scan(&accepted); err != nil {
		return err
	}
	if accepted == 0 {
		return errReasoningFailureTransition
	}
	return nil
}

// FailQueuedAfterReasoningFailure preserves quota-stop policy after a durable
// reasoning failure. It can only fail untouched queued peers, never a capture
// command, accepted observation, protected peer, or existing terminal result.
func (s *Store) FailQueuedAfterReasoningFailure(ctx context.Context, expected domain.Run, failure domain.Failure) error {
	if expected.ID == "" || expected.SessionID == "" || expected.Status != "failed" || failure.Code != "codex_usage_limit" || failure.Stage != "reasoning" {
		return errReasoningFailureTransition
	}
	raw, err := json.Marshal(failure)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var session, source, status, stage, prior, sessionStatus string
	if err = tx.QueryRowContext(ctx, `SELECT r.session_id,r.source,r.status,r.stage,COALESCE(r.error_json,''),s.status FROM runs r JOIN sessions s ON s.id=r.session_id WHERE r.id=?`, expected.ID).Scan(&session, &source, &status, &stage, &prior, &sessionStatus); err != nil {
		return err
	}
	if session != expected.SessionID || source != string(expected.Source) || status != "failed" || stage != "reasoning" || prior != string(raw) || sessionStatus != "running" {
		return errReasoningFailureTransition
	}
	if fenceErr := rejectManualPilotRun(ctx, tx, expected.ID); fenceErr != nil {
		if !errors.Is(fenceErr, errManualPilot) {
			return fenceErr
		}
		if err = requireAcceptedDailyResult(ctx, tx, expected); err != nil {
			return err
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT r.id FROM runs r WHERE r.session_id=? AND r.status='queued'`, session)
	if err != nil {
		return err
	}
	var queued []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		queued = append(queued, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range queued {
		// Preserve each run's fence even though the already failed anchor's
		// retained fence no longer prevents stopping unprotected peer work.
		if err = rejectManualPilotRun(ctx, tx, id); err != nil {
			return err
		}
		var touched int
		if err = tx.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM bridge_commands WHERE run_id=?)+(SELECT COUNT(*) FROM observations WHERE run_id=?)+(SELECT COUNT(*) FROM runs WHERE id=? AND started_at IS NOT NULL)`, id, id, id).Scan(&touched); err != nil {
			return err
		}
		if touched != 0 {
			return errReasoningFailureTransition
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE runs SET status='failed',stage=?,completed_at=?,error_json=? WHERE session_id=? AND status='queued'`, failure.Stage, s.Now().UTC().Format(time.RFC3339Nano), string(raw), session); err != nil {
		return err
	}
	return tx.Commit()
}
