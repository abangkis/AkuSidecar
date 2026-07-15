package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

type calibrationAssessmentRow struct {
	runID         string
	source        domain.Source
	evidenceKey   string
	assessmentRaw string
}

func (s *Store) CalibrationCandidates(ctx context.Context, sessionID string) ([]domain.CalibrationCandidate, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT r.id,r.source,a.evidence_key,a.assessment_json
		FROM runs r
		JOIN candidate_assessments a ON a.run_id=r.id
		WHERE r.session_id=? AND r.status='completed'
		ORDER BY r.ordinal,a.created_at,a.evidence_key`, sessionID)
	if err != nil {
		return nil, err
	}
	var assessed []calibrationAssessmentRow
	for rows.Next() {
		var value calibrationAssessmentRow
		if err := rows.Scan(&value.runID, &value.source, &value.evidenceKey, &value.assessmentRaw); err != nil {
			rows.Close()
			return nil, err
		}
		assessed = append(assessed, value)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	evidenceByRun := map[string]map[string]domain.Block{}
	var candidates []domain.CalibrationCandidate
	for _, row := range assessed {
		byKey, loaded := evidenceByRun[row.runID]
		if !loaded {
			byKey = map[string]domain.Block{}
			observations, err := s.Observations(ctx, row.runID)
			if err != nil {
				return nil, err
			}
			for _, observation := range observations {
				for _, snapshot := range observation.Snapshots {
					for _, block := range snapshot.Blocks {
						if block.EvidenceKey != "" {
							byKey[block.EvidenceKey] = block
						}
					}
				}
			}
			evidenceByRun[row.runID] = byKey
		}
		block, exists := byKey[row.evidenceKey]
		if !exists {
			continue
		}
		var assessment domain.CandidateAssessment
		if err := json.Unmarshal([]byte(row.assessmentRaw), &assessment); err != nil {
			return nil, err
		}
		candidates = append(candidates, domain.CalibrationCandidate{
			RunID: row.runID, EvidenceKey: row.evidenceKey, Source: row.source,
			FeedPosition: block.FeedPosition, Author: block.Author, AvatarURL: block.AvatarURL,
			Text: block.Text, SourceURL: block.Permalink, PublishedAt: block.PublishedAt,
			ContentKind: block.ContentKind, QuotedPost: block.QuotedPost,
			Engagement: block.Engagement, Presentation: block.Presentation,
			Media: block.Media, Links: block.Links, Assessment: assessment,
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Source != candidates[j].Source {
			return candidates[i].Source < candidates[j].Source
		}
		if candidates[i].FeedPosition != candidates[j].FeedPosition {
			return candidates[i].FeedPosition < candidates[j].FeedPosition
		}
		return candidates[i].EvidenceKey < candidates[j].EvidenceKey
	})
	return candidates, nil
}

func (s *Store) CreateCalibration(ctx context.Context, value domain.CalibrationSession) (domain.CalibrationSession, error) {
	now := domain.Now()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.CalibrationSession{}, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT INTO calibration_sessions(id,session_id,trigger_kind,status,max_items,sample_count,created_at,updated_at) VALUES(?,?,?,'reviewing',?,?,?,?)`, value.ID, value.UnifiedSessionID, value.TriggerKind, value.MaxItems, len(value.Samples), now, now); err != nil {
		return domain.CalibrationSession{}, err
	}
	for ordinal, sample := range value.Samples {
		raw, err := json.Marshal(sample.Candidate)
		if err != nil {
			return domain.CalibrationSession{}, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO calibration_samples(calibration_session_id,ordinal,run_id,evidence_key,source,candidate_json) VALUES(?,?,?,?,?,?)`, value.ID, ordinal, sample.RunID, sample.EvidenceKey, sample.Source, string(raw)); err != nil {
			return domain.CalibrationSession{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return domain.CalibrationSession{}, err
	}
	return s.Calibration(ctx, value.ID)
}

func (s *Store) Calibration(ctx context.Context, id string) (domain.CalibrationSession, error) {
	var value domain.CalibrationSession
	var completedAt sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT id,session_id,trigger_kind,status,max_items,sample_count,created_at,updated_at,completed_at FROM calibration_sessions WHERE id=?`, id).Scan(
		&value.ID, &value.UnifiedSessionID, &value.TriggerKind, &value.Status, &value.MaxItems,
		&value.SampleCount, &value.CreatedAt, &value.UpdatedAt, &completedAt,
	)
	if err != nil {
		return domain.CalibrationSession{}, err
	}
	if completedAt.Valid {
		value.CompletedAt = &completedAt.String
	}
	rows, err := s.db.QueryContext(ctx, `SELECT ordinal,run_id,evidence_key,source,candidate_json,label,issue_code,labeled_at FROM calibration_samples WHERE calibration_session_id=? ORDER BY ordinal`, id)
	if err != nil {
		return domain.CalibrationSession{}, err
	}
	for rows.Next() {
		var sample domain.CalibrationSample
		var raw string
		var label, issueCode, labeledAt sql.NullString
		if err := rows.Scan(&sample.Ordinal, &sample.RunID, &sample.EvidenceKey, &sample.Source, &raw, &label, &issueCode, &labeledAt); err != nil {
			rows.Close()
			return domain.CalibrationSession{}, err
		}
		if err := json.Unmarshal([]byte(raw), &sample.Candidate); err != nil {
			rows.Close()
			return domain.CalibrationSession{}, err
		}
		if label.Valid {
			sample.Label = &label.String
		}
		if issueCode.Valid {
			sample.IssueCode = &issueCode.String
		}
		if labeledAt.Valid {
			sample.LabeledAt = &labeledAt.String
		}
		if sample.Label != nil || sample.IssueCode != nil {
			value.ResolvedCount++
		} else if value.CurrentOrdinal == nil {
			ordinal := sample.Ordinal
			value.CurrentOrdinal = &ordinal
		}
		value.Samples = append(value.Samples, sample)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return domain.CalibrationSession{}, err
	}
	if err := rows.Close(); err != nil {
		return domain.CalibrationSession{}, err
	}
	var snapshotRaw string
	err = s.db.QueryRowContext(ctx, `SELECT snapshot_json FROM calibration_profile_snapshots WHERE calibration_session_id=?`, id).Scan(&snapshotRaw)
	if err == nil {
		var snapshot domain.CalibrationSnapshot
		if err := json.Unmarshal([]byte(snapshotRaw), &snapshot); err != nil {
			return domain.CalibrationSession{}, err
		}
		value.Snapshot = &snapshot
	} else if !errors.Is(err, sql.ErrNoRows) {
		return domain.CalibrationSession{}, err
	}
	value.LiveInfluence = false
	return value, nil
}

func (s *Store) CalibrationBySession(ctx context.Context, sessionID string) (*domain.CalibrationSession, error) {
	return s.calibrationBy(ctx, `SELECT id FROM calibration_sessions WHERE session_id=?`, sessionID)
}

func (s *Store) CalibrationByTrigger(ctx context.Context, trigger string) (*domain.CalibrationSession, error) {
	return s.calibrationBy(ctx, `SELECT id FROM calibration_sessions WHERE trigger_kind=? ORDER BY created_at LIMIT 1`, trigger)
}

func (s *Store) ActiveCalibration(ctx context.Context) (*domain.CalibrationSession, error) {
	return s.calibrationBy(ctx, `SELECT id FROM calibration_sessions WHERE status='reviewing' ORDER BY created_at LIMIT 1`)
}

func (s *Store) calibrationBy(ctx context.Context, query string, arguments ...any) (*domain.CalibrationSession, error) {
	var id string
	err := s.db.QueryRowContext(ctx, query, arguments...).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	value, err := s.Calibration(ctx, id)
	return &value, err
}

func (s *Store) RecordCalibrationDecision(ctx context.Context, id string, ordinal int, decision domain.CalibrationDecision) (domain.CalibrationSession, error) {
	if err := decision.Validate(); err != nil {
		return domain.CalibrationSession{}, err
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE calibration_samples SET label=?,issue_code=?,labeled_at=?
		WHERE calibration_session_id=? AND ordinal=?
		  AND EXISTS (SELECT 1 FROM calibration_sessions WHERE id=? AND status='reviewing')`,
		decision.Label, decision.IssueCode, domain.Now(), id, ordinal, id)
	if err != nil {
		return domain.CalibrationSession{}, err
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		return domain.CalibrationSession{}, errors.New("calibration sample does not exist")
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE calibration_sessions SET updated_at=? WHERE id=? AND status='reviewing'`, domain.Now(), id); err != nil {
		return domain.CalibrationSession{}, err
	}
	return s.Calibration(ctx, id)
}

func (s *Store) CompleteCalibration(ctx context.Context, id string, snapshot domain.CalibrationSnapshot) (domain.CalibrationSession, error) {
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return domain.CalibrationSession{}, err
	}
	now := domain.Now()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.CalibrationSession{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE calibration_sessions SET status='completed',updated_at=?,completed_at=? WHERE id=? AND status='reviewing'`, now, now, id)
	if err != nil {
		return domain.CalibrationSession{}, err
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		return domain.CalibrationSession{}, errors.New("calibration session is unavailable or already completed")
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO calibration_profile_snapshots(id,calibration_session_id,snapshot_json,created_at) VALUES(?,?,?,?)`, domain.NewID("calibration_snapshot"), id, string(raw), now); err != nil {
		return domain.CalibrationSession{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO meta(key,value) SELECT 'calibration_first_run_status','completed' FROM calibration_sessions WHERE id=? AND trigger_kind='first_run' ON CONFLICT(key) DO UPDATE SET value=excluded.value`, id); err != nil {
		return domain.CalibrationSession{}, err
	}
	if err = tx.Commit(); err != nil {
		return domain.CalibrationSession{}, err
	}
	return s.Calibration(ctx, id)
}

func (s *Store) CalibrationFirstRunStatus(ctx context.Context) (string, error) {
	var status string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key='calibration_first_run_status'`).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return "not_started", nil
	}
	return status, err
}

func (s *Store) SavePreferenceModel(ctx context.Context, model any, feedbackCount int) error {
	raw, err := json.Marshal(model)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO preference_model(id,model_json,feedback_count,updated_at) VALUES(1,?,?,?) ON CONFLICT(id) DO UPDATE SET model_json=excluded.model_json,feedback_count=excluded.feedback_count,updated_at=excluded.updated_at`, string(raw), feedbackCount, domain.Now())
	return err
}
