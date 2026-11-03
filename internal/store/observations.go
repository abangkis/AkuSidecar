package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func (s *Store) SaveObservation(ctx context.Context, commandID, runID string, observation domain.Observation) error {
	if !observation.Source.Valid() {
		return errors.New("observation source is invalid")
	}
	now := domain.Now()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := rejectManualPilotRun(ctx, tx, runID); err != nil {
		return err
	}
	if err := s.saveObservationTx(ctx, tx, commandID, runID, observation, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) saveObservationTx(ctx context.Context, tx *sql.Tx, commandID, runID string, observation domain.Observation, now string) error {
	var captureSource domain.Source
	if err := tx.QueryRowContext(ctx, `SELECT source FROM runs WHERE id=?`, runID).Scan(&captureSource); err != nil {
		return err
	}
	if err := requireSourceCaptureAllowed(ctx, tx, captureSource); err != nil {
		return err
	}
	var err error
	var expectedRun string
	var status string
	var commandClaimedAt sql.NullString
	if err = tx.QueryRowContext(ctx, `SELECT run_id,status,claimed_at FROM bridge_commands WHERE id=?`, commandID).Scan(&expectedRun, &status, &commandClaimedAt); err != nil {
		return err
	}
	if expectedRun != runID || status != "claimed" {
		return errors.New("bridge command is not claimable for this run")
	}
	observedAt := strings.TrimSpace(observation.CapturedAt)
	if _, parseErr := time.Parse(time.RFC3339Nano, observedAt); parseErr != nil {
		observedAt = now
		observation.CapturedAt = observedAt
	}
	identity, err := s.resolveObservationContentIdentity(ctx, tx, runID, &observation, observedAt)
	if err != nil {
		return err
	}
	if observation.Coverage == nil {
		observation.Coverage = map[string]any{}
	}
	observation.Coverage["contentIdentity"] = identity
	raw, err := json.Marshal(observation)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO observations(id,run_id,command_id,source,observation_json,captured_at,created_at) VALUES(?,?,?,?,?,?,?)`, domain.NewID("observation"), runID, commandID, observation.Source, string(raw), observation.CapturedAt, now); err != nil {
		return err
	}
	var coverageRaw string
	if err = tx.QueryRowContext(ctx, `SELECT coverage_json FROM runs WHERE id=?`, runID).Scan(&coverageRaw); err != nil {
		return err
	}
	coverage := durableCaptureCoverage(coverageRaw, observation.Coverage)
	durableCoverageRaw, err := json.Marshal(coverage)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE bridge_commands SET status='completed',completed_at=? WHERE id=?`, now, commandID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE runs SET status='reasoning',stage='reasoning',coverage_json=? WHERE id=?`, string(durableCoverageRaw), runID); err != nil {
		return err
	}
	captureDurationMS := int64(0)
	if claimedAt, parseErr := time.Parse(time.RFC3339Nano, commandClaimedAt.String); commandClaimedAt.Valid && parseErr == nil {
		captureDurationMS = time.Since(claimedAt).Milliseconds()
		if captureDurationMS < 0 {
			captureDurationMS = 0
		}
	}
	if _, err = tx.ExecContext(ctx, `
		INSERT INTO run_stage_timings(run_id,stage,duration_ms,completed_at) VALUES(?,'captured',?,?)
		ON CONFLICT(run_id,stage) DO UPDATE SET duration_ms=run_stage_timings.duration_ms+excluded.duration_ms,completed_at=excluded.completed_at`,
		runID, captureDurationMS, now); err != nil {
		return err
	}
	return nil
}

func durableCaptureCoverage(raw string, next map[string]any) map[string]any {
	var current map[string]any
	decodeJSON(raw, &current)
	if current == nil {
		current = map[string]any{}
	}
	rounds := make([]any, 0, 2)
	if values, ok := current["rounds"].([]any); ok {
		rounds = append(rounds, values...)
	}
	rounds = append(rounds, next)
	current["acquisitionRounds"] = len(rounds)
	current["rounds"] = rounds
	return current
}

func (s *Store) SetRunCoverageField(ctx context.Context, runID, key string, value any) error {
	if strings.TrimSpace(key) == "" {
		return errors.New("run coverage key is required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var raw string
	if err := tx.QueryRowContext(ctx, `SELECT coverage_json FROM runs WHERE id=?`, runID).Scan(&raw); err != nil {
		return err
	}
	coverage := map[string]any{}
	decodeJSON(raw, &coverage)
	coverage[key] = value
	encoded, err := json.Marshal(coverage)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE runs SET coverage_json=? WHERE id=?`, string(encoded), runID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Observations(ctx context.Context, runID string) ([]domain.Observation, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT observation_json FROM observations WHERE run_id=? ORDER BY created_at`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var values []domain.Observation
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var value domain.Observation
		if err := json.Unmarshal([]byte(raw), &value); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func (s *Store) ResumableReasoningRunIDs(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT r.id
		FROM runs r
		WHERE r.status='reasoning'
		  AND EXISTS (SELECT 1 FROM observations o WHERE o.run_id=r.id)
		ORDER BY r.created_at,r.ordinal`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
