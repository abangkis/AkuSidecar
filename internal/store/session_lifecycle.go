package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func (s *Store) SetSessionPipelineStage(ctx context.Context, sessionID, stage string) error {
	switch stage {
	case "semantic_event_resolution", "timeline_composition", "ai_fast_detection", "finalizing":
	default:
		return fmt.Errorf("invalid session pipeline stage %q", stage)
	}
	coverage := map[string]any{}
	var raw string
	if err := s.db.QueryRowContext(ctx, `SELECT coverage_json FROM sessions WHERE id=? AND status IN ('queued','running')`, sessionID).Scan(&raw); err != nil {
		return err
	}
	decodeJSON(raw, &coverage)
	coverage["pipelineStage"] = stage
	coverage["pipelineStageUpdatedAt"] = domain.Now()
	encoded, err := json.Marshal(coverage)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE sessions SET coverage_json=? WHERE id=? AND status IN ('queued','running')`, string(encoded), sessionID)
	return err
}

func (s *Store) SetRunPipelineStage(ctx context.Context, runID, stage string) error {
	switch stage {
	case "acquisition_planning", "candidate_evaluation":
	default:
		return fmt.Errorf("invalid run pipeline stage %q", stage)
	}
	_, err := s.db.ExecContext(ctx, `UPDATE runs SET stage=? WHERE id=? AND status='reasoning'`, stage, runID)
	return err
}

type ScoredAssessment struct {
	Assessment                             domain.CandidateAssessment
	BaseScore, PreferenceScore, FinalScore float64
	Selected                               bool
}

func (s *Store) CompleteRun(ctx context.Context, run domain.Run, result domain.ReasoningResult, scored []ScoredAssessment, items []domain.TimelineItem, coverage map[string]any) error {
	now := s.Now().UTC().Format(time.RFC3339Nano)
	coverageRaw, _ := json.Marshal(coverage)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var delivery string
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(json_extract(coverage_json,'$.delivery'),'visible') FROM sessions WHERE id=?`, run.SessionID).Scan(&delivery); err != nil {
		return err
	}
	presentation, presentedAt, batchState := "", any(nil), "preparing"
	if delivery != string(domain.UpdateDeliveryPrepared) {
		presentation, presentedAt, batchState = "prepend", now, "visible"
	}
	byKey := map[string]ScoredAssessment{}
	itemsByKey := map[string]domain.ReasonedItem{}
	for _, item := range result.Items {
		itemsByKey[item.EvidenceKey] = item
	}
	for _, value := range scored {
		byKey[value.Assessment.EvidenceKey] = value
		raw, _ := json.Marshal(value.Assessment)
		itemRaw, _ := json.Marshal(itemsByKey[value.Assessment.EvidenceKey])
		selected := 0
		if value.Selected {
			selected = 1
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO candidate_assessments(run_id,evidence_key,source,assessment_json,item_json,base_score,preference_score,final_score,selected,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, run.ID, value.Assessment.EvidenceKey, run.Source, string(raw), string(itemRaw), value.BaseScore, value.PreferenceScore, value.FinalScore, selected, now); err != nil {
			return err
		}
	}
	for index, item := range items {
		item.Rank = index
		item.CreatedAt = now
		itemRaw, _ := json.Marshal(item.Item)
		assessment := byKey[item.EvidenceKey].Assessment
		assessmentRaw, _ := json.Marshal(assessment)
		itemCoverage, _ := json.Marshal(item.Coverage)
		if _, err = tx.ExecContext(ctx, `INSERT INTO timeline_items(id,session_id,run_id,source,evidence_key,rank,item_json,assessment_json,coverage_json,origin_status,presentation,presented_at,batch_state,created_at) VALUES(?,?,?,?,?,?,?,?,?,'running',?,?,?,?)`, item.ID, item.SessionID, item.RunID, item.Source, item.EvidenceKey, item.Rank, string(itemRaw), string(assessmentRaw), string(itemCoverage), presentation, presentedAt, batchState, now); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE runs SET status='completed',stage='completed',completed_at=?,summary=?,coverage_json=? WHERE id=?`, now, result.Summary, string(coverageRaw), run.ID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) AdvanceSession(ctx context.Context, sessionID string) (*domain.Run, error) {
	runs, err := s.listRuns(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	for _, run := range runs {
		if run.Status == "queued" {
			return &run, nil
		}
	}
	return nil, nil
}

// FinalizeSession publishes a terminal session only after Timeline composition
// and knowledge continuity have committed. Until this method succeeds, a new
// update remains blocked by the still-active session.
func (s *Store) FinalizeSession(ctx context.Context, sessionID string) error {
	runs, err := s.listRuns(ctx, sessionID)
	if err != nil {
		return err
	}
	now := s.Now().UTC().Format(time.RFC3339Nano)
	completed := 0
	failed := 0
	for _, run := range runs {
		switch run.Status {
		case "completed":
			completed++
		case "failed", "cancelled":
			failed++
		default:
			return fmt.Errorf("session %s cannot finalize while run %s is %s", sessionID, run.ID, run.Status)
		}
	}
	status := "failed"
	if completed == len(runs) {
		status = "completed"
	} else if completed > 0 {
		status = "partial"
	}
	if _, err = s.db.ExecContext(ctx, `UPDATE sessions SET status=?,active_source=NULL,completed_at=? WHERE id=?`, status, now, sessionID); err != nil {
		return err
	}
	settings, settingsErr := s.GetSettings(ctx)
	if settingsErr != nil {
		return settingsErr
	}
	freshHours := settings.PreparedBatchMaxAgeHours
	var highestUrgency float64
	if urgencyErr := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(CAST(json_extract(assessment_json,'$.urgency') AS REAL)),0) FROM timeline_items WHERE session_id=?`, sessionID).Scan(&highestUrgency); urgencyErr != nil {
		return urgencyErr
	}
	switch {
	case highestUrgency >= 0.85:
		freshHours = 2
	case highestUrgency >= 0.75 && freshHours > 4:
		freshHours = 4
	case highestUrgency >= 0.50 && freshHours > 12:
		freshHours = 12
	}
	expiresAt := s.Now().UTC().Add(time.Duration(freshHours) * time.Hour).Format(time.RFC3339Nano)
	if _, err = s.db.ExecContext(ctx, `UPDATE auto_update_batches SET state=CASE WHEN EXISTS (SELECT 1 FROM timeline_items t WHERE t.session_id=?) THEN 'prepared' ELSE 'expired' END,prepared_at=?,expires_at=? WHERE session_id=? AND state='preparing'`, sessionID, now, expiresAt, sessionID); err != nil {
		return err
	}
	if _, err = s.db.ExecContext(ctx, `
		UPDATE timeline_items
		SET origin_status=?,presentation=COALESCE(NULLIF(presentation,''),'prepend'),presented_at=COALESCE(NULLIF(presented_at,''),?),batch_state='visible'
		WHERE session_id=? AND NOT EXISTS (
		  SELECT 1 FROM auto_update_batches b WHERE b.session_id=? AND b.state IN ('preparing','prepared','expired')
		)`, status, now, sessionID, sessionID); err != nil {
		return err
	}
	return nil
}

func (s *Store) CancelSession(ctx context.Context, id string) error {
	for _, source := range []domain.Source{domain.SourceX, domain.SourceInstagram} {
		if value, ok := s.manualPilotClosures.Load(manualScopeID(id, source)); ok {
			value.(*ManualPilotPermit).revoked.Store(true)
		}
	}
	s.manualPilotMu.Lock()
	defer s.manualPilotMu.Unlock()
	for _, source := range []domain.Source{domain.SourceX, domain.SourceInstagram} {
		if p := s.manualPilots[manualScopeID(id, source)]; p != nil {
			p.revoked.Store(true)
		}
	}
	now := domain.Now()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, source := range []domain.Source{domain.SourceX, domain.SourceInstagram} {
		if err := revokeManualPilotSessionTx(ctx, tx, manualScopeID(id, source)); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE sessions SET status='cancelled',active_source=NULL,completed_at=? WHERE id=? AND status IN ('queued','running')`, now, id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE runs SET status='cancelled',stage='cancelled',completed_at=? WHERE session_id=? AND status IN ('queued','waiting_for_bridge','reasoning')`, now, id); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE bridge_commands SET status='cancelled',completed_at=? WHERE run_id IN (SELECT id FROM runs WHERE session_id=?) AND status IN ('queued','claimed')`, now, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) RetryFailedRun(ctx context.Context, runID string) (domain.Run, error) {
	run, err := s.GetRun(ctx, runID)
	if err != nil {
		return domain.Run{}, err
	}
	if run.Status != "failed" {
		return domain.Run{}, errors.New("only a failed run can be re-evaluated")
	}
	var observations, evaluations, timeline int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM observations WHERE run_id=?`, runID).Scan(&observations); err != nil {
		return domain.Run{}, err
	}
	if observations == 0 {
		return domain.Run{}, errors.New("failed run has no durable captured evidence to re-evaluate")
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM candidate_assessments WHERE run_id=?`, runID).Scan(&evaluations); err != nil {
		return domain.Run{}, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM timeline_items WHERE run_id=?`, runID).Scan(&timeline); err != nil {
		return domain.Run{}, err
	}
	if evaluations != 0 || timeline != 0 {
		return domain.Run{}, errors.New("failed run already contains evaluated state and cannot be replayed safely")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Run{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE sessions SET status='running',active_source=?,completed_at=NULL,error_json=NULL WHERE id=?`, run.Source, run.SessionID); err != nil {
		return domain.Run{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE runs SET status='reasoning',stage='reasoning',completed_at=NULL,summary='',error_json=NULL WHERE id=?`, runID); err != nil {
		return domain.Run{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.Run{}, err
	}
	return s.GetRun(ctx, runID)
}
