package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"time"

	"github.com/abangkis/AkuSidecar/internal/batchbehavior"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

// captureBehaviorAllowed runs in the claim transaction: durable claimed rows
// are the reservation. No timer, process-local slot, or result callback can
// release a different operation. Terminal timestamps survive restart.
func (s *Store) captureBehaviorAllowed(ctx context.Context, tx *sql.Tx, source domain.Source, policy batchbehavior.Policy) (bool, error) {
	decision, err := s.captureBehaviorDecision(ctx, tx, source, policy)
	return decision.Allowed, err
}

func (s *Store) CaptureBehaviorDecision(ctx context.Context, source domain.Source, policy batchbehavior.Policy) (batchbehavior.Decision, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return batchbehavior.Decision{}, err
	}
	defer tx.Rollback()
	if err := requireSourceCaptureAllowed(ctx, tx, source); err != nil {
		if err == ErrSourceCapturePaused {
			return batchbehavior.Decision{Reason: "challenge_required"}, nil
		}
		return batchbehavior.Decision{}, err
	}
	return s.captureBehaviorDecision(ctx, tx, source, policy)
}

func (s *Store) captureBehaviorDecision(ctx context.Context, tx *sql.Tx, source domain.Source, policy batchbehavior.Policy) (batchbehavior.Decision, error) {
	if !policy.Enabled {
		// A manual pilot is exclusive with ordinary capture for this source,
		// independently of the optional spacing configuration.
		var active int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM bridge_commands c JOIN runs r ON r.id=c.run_id JOIN meta m ON m.key=?||r.session_id WHERE r.source=? AND c.status='claimed'`, manualPilotSessionPrefix, source).Scan(&active); err != nil {
			return batchbehavior.Decision{}, err
		}
		if active > 0 {
			return batchbehavior.Decision{Reason: "capture_active"}, nil
		}
		return batchbehavior.Decision{Allowed: true}, nil
	}
	active, err := activeSourceCaptureCount(ctx, tx, source)
	if err != nil {
		return batchbehavior.Decision{}, err
	}
	if active > 0 {
		return batchbehavior.Decision{Reason: "capture_active"}, nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT id,completed_at,error_json FROM (
 SELECT c.id,c.completed_at,c.error_json FROM bridge_commands c JOIN runs r ON r.id=c.run_id WHERE r.source=? AND c.claimed_at IS NOT NULL AND c.completed_at IS NOT NULL AND c.status!='claimed'
 UNION ALL SELECT id,completed_at,error_json FROM media_recaptures WHERE source=? AND claimed_at IS NOT NULL AND completed_at IS NOT NULL AND status!='claimed'
 ) ORDER BY julianday(completed_at) DESC,id DESC LIMIT 16`, source, source)
	if err != nil {
		return batchbehavior.Decision{}, err
	}
	defer rows.Close()
	var recent []batchbehavior.Completion
	for rows.Next() {
		var item batchbehavior.Completion
		var at string
		var raw sql.NullString
		if err := rows.Scan(&item.ID, &at, &raw); err != nil {
			return batchbehavior.Decision{}, err
		}
		item.At, err = time.Parse(time.RFC3339Nano, at)
		if err != nil {
			return batchbehavior.Decision{}, err
		}
		if raw.Valid && raw.String != "" && raw.String != "null" {
			var failure domain.Failure
			if err := json.Unmarshal([]byte(raw.String), &failure); err != nil {
				return batchbehavior.Decision{}, err
			}
			item.Code = failure.Code
			item.Retryable = failure.Retryable
		}
		recent = append(recent, item)
	}
	if err := rows.Err(); err != nil {
		return batchbehavior.Decision{}, err
	}
	// Order the bounded history at full precision after SQLite's time-based
	// selection; two RFC3339Nano timestamps may share the same Julian millisecond.
	sort.SliceStable(recent, func(i, j int) bool { return recent[i].At.After(recent[j].At) })
	return policy.Decide(s.Now(), false, recent, nil), nil
}

func activeSourceCaptureCount(ctx context.Context, tx *sql.Tx, source domain.Source) (int, error) {
	var active int
	err := tx.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM bridge_commands c JOIN runs r ON r.id=c.run_id WHERE r.source=? AND c.status='claimed')+(SELECT COUNT(*) FROM media_recaptures WHERE source=? AND status='claimed')`, source, source).Scan(&active)
	return active, err
}

func (s *Store) ClaimCommandWithBehavior(ctx context.Context, runID, bridgeID, collector string, policy batchbehavior.Policy) (*domain.BridgeCommand, error) {
	return s.claimCommandWithBehavior(ctx, runID, bridgeID, collector, policy)
}

func (s *Store) ClaimMediaRecaptureWithBehavior(ctx context.Context, id, bridgeID, collector string, policy batchbehavior.Policy) (domain.MediaRecapture, error) {
	return s.claimMediaRecaptureWithBehavior(ctx, id, bridgeID, collector, policy)
}
