package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

// syncPreferenceLearningLedger projects compact, normalized learning evidence
// out of run-owned tables before those tables are eligible for retention.
// The ledger intentionally has no foreign key to a session or run.
func (s *Store) syncPreferenceLearningLedger(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	statements := []string{
		`INSERT INTO preference_learning_ledger(
		   event_id,source,evidence_key,direction,reason,origin,created_at,assessment_json,active
		 )
		 SELECT f.id,a.source,f.evidence_key,f.direction,f.reason,'routine',f.created_at,a.assessment_json,1
		 FROM feedback_events f
		 JOIN candidate_assessments a ON a.run_id=f.run_id AND a.evidence_key=f.evidence_key
		 ON CONFLICT(event_id) DO UPDATE SET
		   source=excluded.source,evidence_key=excluded.evidence_key,direction=excluded.direction,
		   reason=excluded.reason,created_at=excluded.created_at,
		   assessment_json=excluded.assessment_json,active=1`,
		`INSERT INTO preference_learning_ledger(
		   event_id,source,evidence_key,direction,reason,origin,created_at,assessment_json,active
		 )
		 SELECT 'calibration:' || c.calibration_session_id || ':' || c.ordinal,
		   a.source,c.evidence_key,
		   CASE c.label WHEN 'more_like_this' THEN 'more' WHEN 'less_like_this' THEN 'less' ELSE 'neutral' END,
		   CASE c.label WHEN 'less_like_this' THEN 'not_interested' ELSE NULL END,
		   'calibration',COALESCE(c.labeled_at,''),a.assessment_json,
		   CASE WHEN c.label IS NULL THEN 0 ELSE 1 END
		 FROM calibration_samples c
		 JOIN candidate_assessments a ON a.run_id=c.run_id AND a.evidence_key=c.evidence_key
		 ON CONFLICT(event_id) DO UPDATE SET
		   source=excluded.source,evidence_key=excluded.evidence_key,direction=excluded.direction,
		   reason=excluded.reason,created_at=excluded.created_at,
		   assessment_json=excluded.assessment_json,active=excluded.active`,
		`INSERT INTO preference_learning_ledger(
		   event_id,source,evidence_key,direction,reason,origin,created_at,assessment_json,active
		 )
		 SELECT c.id,a.source,c.evidence_key,'more',NULL,'selection_correction',
		   c.created_at,a.assessment_json,
		   CASE
		     WHEN c.undone_at IS NULL
		      AND c.created_at > COALESCE((SELECT value FROM meta WHERE key='preference_signal_reset_at'),'')
		     THEN 1 ELSE 0
		   END
		 FROM selection_corrections c
		 JOIN candidate_assessments a ON a.run_id=c.run_id AND a.evidence_key=c.evidence_key
		 ON CONFLICT(event_id) DO UPDATE SET
		   source=excluded.source,evidence_key=excluded.evidence_key,direction=excluded.direction,
		   reason=excluded.reason,created_at=excluded.created_at,
		   assessment_json=excluded.assessment_json,active=excluded.active`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type PreferenceSignal struct {
	EventID     string
	Source      domain.Source
	EvidenceKey string
	CreatedAt   string
	Direction   string
	Reason      *string
	Origin      string
	Assessment  domain.CandidateAssessment
}

func (s *Store) PreferenceSignals(ctx context.Context) ([]PreferenceSignal, error) {
	if err := s.syncPreferenceLearningLedger(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		WITH ranked AS (
		  SELECT ledger.*,ROW_NUMBER() OVER (
		    PARTITION BY source,evidence_key ORDER BY created_at DESC,
		      CASE origin WHEN 'routine' THEN 3 WHEN 'selection_correction' THEN 2 ELSE 1 END DESC,
		      event_id DESC
		  ) AS signal_rank
		  FROM preference_learning_ledger ledger
		  WHERE active=1
		)
		SELECT event_id,source,evidence_key,created_at,direction,reason,origin,assessment_json
		FROM ranked
		WHERE signal_rank=1
		ORDER BY created_at,event_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var signals []PreferenceSignal
	for rows.Next() {
		var signal PreferenceSignal
		var reason sql.NullString
		var raw string
		if err := rows.Scan(
			&signal.EventID, &signal.Source, &signal.EvidenceKey, &signal.CreatedAt,
			&signal.Direction, &reason, &signal.Origin, &raw,
		); err != nil {
			return nil, err
		}
		if reason.Valid {
			signal.Reason = &reason.String
		}
		if err := json.Unmarshal([]byte(raw), &signal.Assessment); err != nil {
			return nil, err
		}
		signals = append(signals, signal)
	}
	return signals, rows.Err()
}

func (s *Store) LoadPreferenceModel(ctx context.Context, value any) (bool, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT model_json FROM preference_model WHERE id=1`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal([]byte(raw), value); err != nil {
		return false, err
	}
	return true, nil
}
