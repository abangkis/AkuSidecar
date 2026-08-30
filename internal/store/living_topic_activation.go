package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

const (
	LivingTopicActivationEngineVersion = "living-topics-activation-v1"
	LivingTopicActivationScanLimit     = 100
	LivingTopicActivationShortlist     = 12
)

var (
	ErrLivingTopicCandidateNotFound = errors.New("living topic candidate not found")
	ErrLivingTopicCandidateReview   = errors.New("living topic candidate review cannot be applied")
)

func (s *Store) QueueLivingTopicActivation(ctx context.Context, topicID, trigger string) (bool, error) {
	topic, err := s.LivingTopic(ctx, topicID)
	if err != nil {
		return false, err
	}
	now := memoryNow(s)
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO living_topic_activation_jobs(id,topic_id,criteria_revision,status,trigger,queued_at)
		VALUES(?,?,?,'pending',?,?)
		ON CONFLICT(topic_id,criteria_revision) DO UPDATE SET
		 status=CASE WHEN living_topic_activation_jobs.status='running' THEN 'running' ELSE 'pending' END,
		 trigger=excluded.trigger,result_json='{}',last_error='',
		 queued_at=CASE WHEN living_topic_activation_jobs.status='running' THEN living_topic_activation_jobs.queued_at ELSE excluded.queued_at END,
		 started_at=CASE WHEN living_topic_activation_jobs.status='running' THEN living_topic_activation_jobs.started_at ELSE NULL END,
		 completed_at=CASE WHEN living_topic_activation_jobs.status='running' THEN living_topic_activation_jobs.completed_at ELSE NULL END`,
		domain.NewID("topic_activation"), topic.ID, topic.CriteriaRevision, strings.TrimSpace(trigger), now)
	if err != nil {
		return false, fmt.Errorf("queue Living Topic activation: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE living_topics SET routing_status='pending',routing_last_error='' WHERE id=? AND criteria_revision=?`, topic.ID, topic.CriteriaRevision); err != nil {
		return false, err
	}
	count, _ := result.RowsAffected()
	return count > 0, nil
}

func (s *Store) ResetRunningLivingTopicActivations(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `UPDATE living_topic_activation_jobs SET status='pending',started_at=NULL,last_error='' WHERE status='running'`); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE living_topics SET routing_status='pending' WHERE routing_status='running'`)
	return err
}

func (s *Store) ClaimLivingTopicActivation(ctx context.Context) (*domain.LivingTopicActivationJob, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var job domain.LivingTopicActivationJob
	err = tx.QueryRowContext(ctx, `SELECT id,topic_id,criteria_revision,trigger FROM living_topic_activation_jobs WHERE status='pending' ORDER BY queued_at,id LIMIT 1`).Scan(&job.ID, &job.TopicID, &job.CriteriaRevision, &job.Trigger)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	now := memoryNow(s)
	result, err := tx.ExecContext(ctx, `UPDATE living_topic_activation_jobs SET status='running',started_at=? WHERE id=? AND status='pending'`, now, job.ID)
	if err != nil {
		return nil, err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return nil, nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE living_topics SET routing_status='running',routing_last_error='' WHERE id=? AND criteria_revision=?`, job.TopicID, job.CriteriaRevision); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &job, nil
}

func (s *Store) HasPendingLivingTopicActivation(ctx context.Context) (bool, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM living_topic_activation_jobs WHERE status='pending'`).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *Store) FinishLivingTopicActivation(ctx context.Context, job domain.LivingTopicActivationJob, resultValue map[string]int, activationErr error) error {
	raw, _ := json.Marshal(resultValue)
	status, message := "completed", ""
	if activationErr != nil {
		status, message = "failed", boundedLivingTopicUnderstandingError(activationErr.Error())
	}
	now := memoryNow(s)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE living_topic_activation_jobs SET status=?,result_json=?,last_error=?,completed_at=? WHERE id=?`, status, string(raw), message, now, job.ID); err != nil {
		return err
	}
	topicStatus := "current"
	if activationErr != nil {
		topicStatus = "failed"
	}
	if _, err := tx.ExecContext(ctx, `UPDATE living_topics SET routing_status=?,routing_checked_at=?,routing_last_error=? WHERE id=? AND criteria_revision=?`, topicStatus, now, message, job.TopicID, job.CriteriaRevision); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) LivingTopicActivationItems(ctx context.Context, topicID string, limit int) ([]domain.MemoryItem, error) {
	if _, err := s.LivingTopic(ctx, topicID); err != nil {
		return nil, err
	}
	if limit < 1 || limit > LivingTopicActivationScanLimit {
		limit = LivingTopicActivationScanLimit
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT i.id FROM memory_items i
		WHERE i.lifecycle_state='active'
		  AND NOT EXISTS (SELECT 1 FROM living_topic_memberships m WHERE m.topic_id=? AND m.memory_item_id=i.id)
		ORDER BY i.updated_at DESC,i.id DESC LIMIT ?`, topicID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	items := make([]domain.MemoryItem, 0, len(ids))
	for _, id := range ids {
		item, err := memoryItemByQueryer(ctx, s.db, id)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *Store) SaveLivingTopicCandidateDecision(ctx context.Context, topic domain.LivingTopic, item domain.MemoryItem, decision domain.LivingTopicRoutingDecision) error {
	status := "not_matched"
	if decision.Match && decision.Confidence >= 0.65 {
		status = "suggested"
	}
	now := memoryNow(s)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO living_topic_candidate_evaluations(topic_id,memory_item_id,criteria_revision,status,engine_version,match_mode,confidence,reason,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(topic_id,memory_item_id,criteria_revision) DO UPDATE SET
		 status=CASE WHEN living_topic_candidate_evaluations.status IN ('accepted','rejected') THEN living_topic_candidate_evaluations.status ELSE excluded.status END,
		 engine_version=excluded.engine_version,match_mode=excluded.match_mode,confidence=excluded.confidence,reason=excluded.reason,updated_at=excluded.updated_at`,
		topic.ID, item.ID, topic.CriteriaRevision, status, LivingTopicActivationEngineVersion, decision.Mode, decision.Confidence, strings.TrimSpace(decision.Reason), now, now)
	return err
}

func (s *Store) LivingTopicCandidates(ctx context.Context, topicID string, criteriaRevision, limit int) ([]domain.LivingTopicCandidate, error) {
	if limit < 1 || limit > 20 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.topic_id,c.memory_item_id,c.criteria_revision,c.status,c.match_mode,c.confidence,c.reason,c.created_at,c.updated_at,COALESCE(c.reviewed_at,'')
		FROM living_topic_candidate_evaluations c
		JOIN memory_items i ON i.id=c.memory_item_id AND i.lifecycle_state='active'
		WHERE c.topic_id=? AND c.criteria_revision=? AND c.status IN ('suggested','accepted','rejected')
		ORDER BY CASE c.status WHEN 'suggested' THEN 0 ELSE 1 END,c.confidence DESC,c.updated_at DESC,c.memory_item_id
		LIMIT ?`, topicID, criteriaRevision, limit)
	if err != nil {
		return nil, err
	}
	values := make([]domain.LivingTopicCandidate, 0)
	for rows.Next() {
		var value domain.LivingTopicCandidate
		if err := rows.Scan(&value.TopicID, &value.MemoryItemID, &value.CriteriaRevision, &value.Status, &value.MatchMode, &value.Confidence, &value.Reason, &value.CreatedAt, &value.UpdatedAt, &value.ReviewedAt); err != nil {
			rows.Close()
			return nil, err
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for index := range values {
		item, err := memoryItemByQueryer(ctx, s.db, values[index].MemoryItemID)
		if err != nil {
			return nil, err
		}
		values[index].Item = item
	}
	return values, nil
}

func (s *Store) ReviewLivingTopicCandidate(ctx context.Context, topicID, memoryID, action string) (domain.LivingTopicDetail, error) {
	topic, err := s.LivingTopic(ctx, topicID)
	if err != nil {
		return domain.LivingTopicDetail{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.LivingTopicDetail{}, err
	}
	defer tx.Rollback()
	var status, mode, reason string
	var confidence float64
	err = tx.QueryRowContext(ctx, `SELECT status,match_mode,confidence,reason FROM living_topic_candidate_evaluations WHERE topic_id=? AND memory_item_id=? AND criteria_revision=?`, topic.ID, strings.TrimSpace(memoryID), topic.CriteriaRevision).Scan(&status, &mode, &confidence, &reason)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.LivingTopicDetail{}, ErrLivingTopicCandidateNotFound
	}
	if err != nil {
		return domain.LivingTopicDetail{}, err
	}
	now := memoryNow(s)
	switch action {
	case "accept":
		if status != "suggested" && status != "rejected" {
			return domain.LivingTopicDetail{}, ErrLivingTopicCandidateReview
		}
		var active int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM memory_items WHERE id=? AND lifecycle_state='active'`, memoryID).Scan(&active); err != nil || active != 1 {
			if err != nil {
				return domain.LivingTopicDetail{}, err
			}
			return domain.LivingTopicDetail{}, ErrMemoryNotFound
		}
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM living_topic_memberships WHERE topic_id=?`, topic.ID).Scan(&count); err != nil {
			return domain.LivingTopicDetail{}, err
		}
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM living_topic_memberships WHERE topic_id=? AND memory_item_id=?`, topic.ID, memoryID).Scan(&exists); err != nil {
			return domain.LivingTopicDetail{}, err
		}
		if exists == 0 && count >= LivingTopicMaxMembers {
			return domain.LivingTopicDetail{}, ErrLivingTopicMemberMax
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO living_topic_memberships(topic_id,memory_item_id,added_at,origin,match_mode,confidence,reason) VALUES(?,?,?,'manual','candidate_accept',?,?) ON CONFLICT(topic_id,memory_item_id) DO NOTHING`, topic.ID, memoryID, now, confidence, "Accepted suggestion: "+reason); err != nil {
			return domain.LivingTopicDetail{}, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE living_topic_candidate_evaluations SET status='accepted',reviewed_at=?,updated_at=? WHERE topic_id=? AND memory_item_id=? AND criteria_revision=?`, now, now, topic.ID, memoryID, topic.CriteriaRevision); err != nil {
			return domain.LivingTopicDetail{}, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO living_topic_feedback_events(id,topic_id,memory_item_id,verdict,created_at) VALUES(?,?,?,'include',?)`, domain.NewID("topic_feedback"), topic.ID, memoryID, now); err != nil {
			return domain.LivingTopicDetail{}, err
		}
	case "reject":
		if status != "suggested" && status != "accepted" {
			return domain.LivingTopicDetail{}, ErrLivingTopicCandidateReview
		}
		if status == "accepted" {
			if _, err := tx.ExecContext(ctx, `DELETE FROM living_topic_memberships WHERE topic_id=? AND memory_item_id=? AND match_mode='candidate_accept'`, topic.ID, memoryID); err != nil {
				return domain.LivingTopicDetail{}, err
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE living_topic_candidate_evaluations SET status='rejected',reviewed_at=?,updated_at=? WHERE topic_id=? AND memory_item_id=? AND criteria_revision=?`, now, now, topic.ID, memoryID, topic.CriteriaRevision); err != nil {
			return domain.LivingTopicDetail{}, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO living_topic_feedback_events(id,topic_id,memory_item_id,verdict,created_at) VALUES(?,?,?,'exclude',?)`, domain.NewID("topic_feedback"), topic.ID, memoryID, now); err != nil {
			return domain.LivingTopicDetail{}, err
		}
	case "undo":
		if status != "accepted" && status != "rejected" {
			return domain.LivingTopicDetail{}, ErrLivingTopicCandidateReview
		}
		if status == "accepted" {
			if _, err := tx.ExecContext(ctx, `DELETE FROM living_topic_memberships WHERE topic_id=? AND memory_item_id=? AND match_mode='candidate_accept'`, topic.ID, memoryID); err != nil {
				return domain.LivingTopicDetail{}, err
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE living_topic_candidate_evaluations SET status='suggested',reviewed_at=NULL,updated_at=? WHERE topic_id=? AND memory_item_id=? AND criteria_revision=?`, now, topic.ID, memoryID, topic.CriteriaRevision); err != nil {
			return domain.LivingTopicDetail{}, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO living_topic_feedback_events(id,topic_id,memory_item_id,verdict,created_at) VALUES(?,?,?,'clear',?)`, domain.NewID("topic_feedback"), topic.ID, memoryID, now); err != nil {
			return domain.LivingTopicDetail{}, err
		}
	default:
		return domain.LivingTopicDetail{}, ErrLivingTopicCandidateReview
	}
	if _, err := tx.ExecContext(ctx, `UPDATE living_topics SET updated_at=? WHERE id=?`, now, topic.ID); err != nil {
		return domain.LivingTopicDetail{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.LivingTopicDetail{}, err
	}
	return s.LivingTopicDetail(ctx, topic.ID)
}
