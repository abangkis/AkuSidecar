package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func (s *Store) FailCommand(ctx context.Context, commandID, runID string, failure domain.Failure) (bool, error) {
	return s.failCommand(ctx, commandID, runID, failure, nil)
}

func (s *Store) ExpireQueuedBridgeCommand(ctx context.Context, commandID, runID string, failure domain.Failure, at time.Time) (bool, error) {
	return s.failCommand(ctx, commandID, runID, failure, &at)
}

func (s *Store) failCommand(ctx context.Context, commandID, runID string, failure domain.Failure, queuedAt *time.Time) (bool, error) {
	now := s.Now().UTC().Format(time.RFC3339Nano)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	if err := rejectManualPilotRun(ctx, tx, runID); err != nil {
		return false, err
	}
	expectedStatus := "claimed"
	if queuedAt != nil {
		var command domain.BridgeCommand
		var payload string
		if err := tx.QueryRowContext(ctx, `SELECT status,created_at,payload_json FROM bridge_commands WHERE id=? AND run_id=?`, commandID, runID).Scan(&command.Status, &command.CreatedAt, &payload); err != nil {
			return false, err
		}
		command.Payload, err = decodeCapturePayload(payload)
		if err != nil {
			return false, err
		}
		if !expiredQueuedBridgeCommand(command, *queuedAt) {
			return false, errors.New("bridge command is not active")
		}
		expectedStatus = "queued"
	}
	var runStage string
	var observationCount int
	if err = tx.QueryRowContext(ctx, `SELECT r.stage,(SELECT COUNT(*) FROM observations o WHERE o.run_id=r.id) FROM runs r JOIN bridge_commands c ON c.run_id=r.id WHERE r.id=? AND c.id=?`, runID, commandID).Scan(&runStage, &observationCount); err != nil {
		return false, err
	}
	var source domain.Source
	if err := tx.QueryRowContext(ctx, `SELECT source FROM runs WHERE id=?`, runID).Scan(&source); err != nil {
		return false, err
	}
	if source == domain.SourceX && domain.IsAccessChallenge(failure.Code) {
		failure = domain.SafeXChallenge(failure)
	}
	raw, _ := json.Marshal(failure)
	result, err := tx.ExecContext(ctx, `UPDATE bridge_commands SET status='failed',completed_at=?,error_json=? WHERE id=? AND run_id=? AND status=?`, now, string(raw), commandID, runID, expectedStatus)
	if err != nil {
		return false, err
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return false, errors.New("bridge command is not active")
	}
	if domain.IsAccessChallenge(failure.Code) {
		if source == domain.SourceX {
			if err := s.pauseXCaptureTx(ctx, tx, runID, domain.XChallengeCategory(failure)); err != nil {
				return false, err
			}
		}
	}
	if runStage == "follow_up_capture" && observationCount > 0 {
		if _, err = tx.ExecContext(ctx, `UPDATE runs SET status='reasoning',stage='reasoning' WHERE id=?`, runID); err != nil {
			return false, err
		}
		if err = tx.Commit(); err != nil {
			return false, err
		}
		return true, nil
	}
	if _, err = tx.ExecContext(ctx, `UPDATE runs SET status='failed',stage=?,completed_at=?,error_json=? WHERE id=?`, failure.Stage, now, string(raw), runID); err != nil {
		return false, err
	}
	return false, tx.Commit()
}

func (s *Store) FailRun(ctx context.Context, runID string, failure domain.Failure) error {
	raw, _ := json.Marshal(failure)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = rejectManualPilotRun(ctx, tx, runID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE runs SET status='failed',stage=?,completed_at=?,error_json=? WHERE id=? AND status NOT IN ('completed','failed','cancelled')`, failure.Stage, domain.Now(), string(raw), runID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) FailQueuedRuns(ctx context.Context, sessionID string, failure domain.Failure) error {
	raw, err := json.Marshal(failure)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var protected int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM meta WHERE key IN (?,?)`, manualPilotSessionPrefix+sessionID, manualPilotSessionPrefix+manualScopeID(sessionID, domain.SourceInstagram)).Scan(&protected); err != nil {
		return err
	}
	if protected != 0 {
		return errManualPilot
	}
	if _, err = tx.ExecContext(ctx, `UPDATE runs SET status='failed',stage=?,completed_at=?,error_json=? WHERE session_id=? AND status='queued'`, failure.Stage, s.Now().UTC().Format(time.RFC3339Nano), string(raw), sessionID); err != nil {
		return err
	}
	return tx.Commit()
}
