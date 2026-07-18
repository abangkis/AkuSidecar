package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

type SelectionCorrectionCandidate struct {
	Run                domain.Run
	Item               domain.TimelineItem
	OriginallySelected bool
}

func candidateRef(runID, evidenceKey string) string {
	sum := sha256.Sum256([]byte(runID + "\x00" + evidenceKey))
	return "candidate_" + hex.EncodeToString(sum[:8])
}

func (s *Store) selectionCorrectionCandidate(ctx context.Context, runID, ref string) (SelectionCorrectionCandidate, error) {
	run, err := s.GetRun(ctx, runID)
	if err != nil {
		return SelectionCorrectionCandidate{}, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT evidence_key,assessment_json,item_json,selected
		FROM candidate_assessments WHERE run_id=?`, runID)
	if err != nil {
		return SelectionCorrectionCandidate{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var evidenceKey, assessmentRaw, itemRaw string
		var selected int
		if err := rows.Scan(&evidenceKey, &assessmentRaw, &itemRaw, &selected); err != nil {
			return SelectionCorrectionCandidate{}, err
		}
		if candidateRef(runID, evidenceKey) != ref {
			continue
		}
		var assessment domain.CandidateAssessment
		var item domain.ReasonedItem
		if err := json.Unmarshal([]byte(assessmentRaw), &assessment); err != nil {
			return SelectionCorrectionCandidate{}, err
		}
		if err := json.Unmarshal([]byte(itemRaw), &item); err != nil || item.EvidenceKey == "" {
			return SelectionCorrectionCandidate{}, errors.New("evaluated candidate is missing its durable reasoned item")
		}
		return SelectionCorrectionCandidate{
			Run: run,
			Item: domain.TimelineItem{
				SessionID: run.SessionID, RunID: run.ID, Source: run.Source,
				EvidenceKey: evidenceKey, Item: item, Assessment: assessment, Coverage: run.Coverage,
			},
			OriginallySelected: selected == 1,
		}, nil
	}
	if err := rows.Err(); err != nil {
		return SelectionCorrectionCandidate{}, err
	}
	return SelectionCorrectionCandidate{}, sql.ErrNoRows
}

func (s *Store) CreateSelectionCorrection(ctx context.Context, runID, ref string) (domain.SelectionCorrection, domain.TimelineItem, error) {
	candidate, err := s.selectionCorrectionCandidate(ctx, runID, ref)
	if err != nil {
		return domain.SelectionCorrection{}, domain.TimelineItem{}, err
	}
	if candidate.OriginallySelected {
		return domain.SelectionCorrection{}, domain.TimelineItem{}, errors.New("candidate was already selected by the original run")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.SelectionCorrection{}, domain.TimelineItem{}, err
	}
	defer tx.Rollback()
	var existing int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM selection_corrections WHERE run_id=? AND evidence_key=? AND undone_at IS NULL`, runID, candidate.Item.EvidenceKey).Scan(&existing); err != nil {
		return domain.SelectionCorrection{}, domain.TimelineItem{}, err
	}
	if existing != 0 {
		return domain.SelectionCorrection{}, domain.TimelineItem{}, errors.New("candidate already has an active selection correction")
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM timeline_items WHERE run_id=? AND evidence_key=?`, runID, candidate.Item.EvidenceKey).Scan(&existing); err != nil {
		return domain.SelectionCorrection{}, domain.TimelineItem{}, err
	}
	if existing != 0 {
		return domain.SelectionCorrection{}, domain.TimelineItem{}, errors.New("candidate is already present in the Timeline")
	}
	var rank int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(rank)+1,0) FROM timeline_items WHERE session_id=?`, candidate.Run.SessionID).Scan(&rank); err != nil {
		return domain.SelectionCorrection{}, domain.TimelineItem{}, err
	}
	now := domain.Now()
	candidate.Item.ID = domain.NewID("timeline")
	candidate.Item.Rank = rank
	candidate.Item.CreatedAt = now
	itemRaw, _ := json.Marshal(candidate.Item.Item)
	assessmentRaw, _ := json.Marshal(candidate.Item.Assessment)
	coverageRaw, _ := json.Marshal(candidate.Item.Coverage)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO timeline_items(id,session_id,run_id,source,evidence_key,rank,item_json,assessment_json,coverage_json,created_at)
		VALUES(?,?,?,?,?,?,?,?,?,?)`, candidate.Item.ID, candidate.Item.SessionID, candidate.Item.RunID, candidate.Item.Source,
		candidate.Item.EvidenceKey, candidate.Item.Rank, string(itemRaw), string(assessmentRaw), string(coverageRaw), now); err != nil {
		return domain.SelectionCorrection{}, domain.TimelineItem{}, err
	}
	correction := domain.SelectionCorrection{
		ID: domain.NewID("selection_correction"), SessionID: candidate.Run.SessionID, RunID: runID,
		EvidenceKey: candidate.Item.EvidenceKey, TimelineID: candidate.Item.ID, Action: "should_select", CreatedAt: now,
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO selection_corrections(id,session_id,run_id,evidence_key,timeline_id,action,created_at)
		VALUES(?,?,?,?,?,?,?)`, correction.ID, correction.SessionID, correction.RunID, correction.EvidenceKey,
		correction.TimelineID, correction.Action, correction.CreatedAt); err != nil {
		return domain.SelectionCorrection{}, domain.TimelineItem{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.SelectionCorrection{}, domain.TimelineItem{}, err
	}
	return correction, candidate.Item, nil
}

func (s *Store) UndoSelectionCorrection(ctx context.Context, id string) (domain.SelectionCorrection, error) {
	var value domain.SelectionCorrection
	var timelineID sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT id,session_id,run_id,evidence_key,timeline_id,action,created_at
		FROM selection_corrections WHERE id=? AND undone_at IS NULL`, id).
		Scan(&value.ID, &value.SessionID, &value.RunID, &value.EvidenceKey, &timelineID, &value.Action, &value.CreatedAt)
	if err != nil {
		return domain.SelectionCorrection{}, err
	}
	value.TimelineID = timelineID.String
	now := domain.Now()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.SelectionCorrection{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE selection_corrections SET undone_at=? WHERE id=? AND undone_at IS NULL`, now, id); err != nil {
		return domain.SelectionCorrection{}, err
	}
	if value.TimelineID != "" {
		if _, err := tx.ExecContext(ctx, `DELETE FROM timeline_items WHERE id=?`, value.TimelineID); err != nil {
			return domain.SelectionCorrection{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM knowledge_events WHERE source=(SELECT source FROM runs WHERE id=?) AND evidence_key=?`, value.RunID, value.EvidenceKey); err != nil {
		return domain.SelectionCorrection{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.SelectionCorrection{}, err
	}
	value.UndoneAt = &now
	if err := s.cleanupOrphanSemanticEvents(ctx); err != nil {
		return domain.SelectionCorrection{}, fmt.Errorf("cleanup semantic event after undo: %w", err)
	}
	return value, nil
}

func (s *Store) SaveSelectionCorrectionKnowledge(ctx context.Context, correctionID string) error {
	var source domain.Source
	var evidenceKey, itemRaw, eventKey, createdAt string
	var duplicate int
	err := s.db.QueryRowContext(ctx, `
		SELECT t.source,t.evidence_key,t.item_json,COALESCE(json_extract(t.item_json,'$.eventKey'),''),t.created_at,
		       CASE WHEN r.relation='duplicate_report' THEN 1 ELSE 0 END
		FROM selection_corrections c
		JOIN timeline_items t ON t.id=c.timeline_id
		LEFT JOIN semantic_event_reports r ON r.timeline_id=t.id
		WHERE c.id=? AND c.undone_at IS NULL`, correctionID).
		Scan(&source, &evidenceKey, &itemRaw, &eventKey, &createdAt, &duplicate)
	if err != nil {
		return err
	}
	if duplicate == 1 || eventKey == "" {
		return nil
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO knowledge_events(id,source,event_key,evidence_key,item_json,first_seen_at,last_seen_at)
		VALUES(?,?,?,?,?,?,?)
		ON CONFLICT(source,event_key) DO UPDATE SET evidence_key=excluded.evidence_key,item_json=excluded.item_json,last_seen_at=excluded.last_seen_at
		WHERE excluded.last_seen_at >= knowledge_events.last_seen_at`, domain.NewID("knowledge"), source, eventKey, evidenceKey, itemRaw, createdAt, createdAt)
	return err
}
