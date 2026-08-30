package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

// QueueLivingTopicUnderstanding coalesces pending work for one topic. A new
// pending row is still allowed while an older row is running, so evidence that
// changes during synthesis is evaluated afterwards rather than lost.
func (s *Store) QueueLivingTopicUnderstanding(ctx context.Context, topicID, trigger string) (bool, error) {
	topicID, trigger = strings.TrimSpace(topicID), strings.TrimSpace(trigger)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM living_topics WHERE id=?`, topicID).Scan(&exists); err != nil {
		return false, err
	}
	if exists == 0 {
		return false, ErrLivingTopicNotFound
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM living_topic_understanding_jobs WHERE topic_id=? AND status='pending'`, topicID).Scan(&exists); err != nil {
		return false, err
	}
	now := memoryNow(s)
	queued := exists == 0
	if queued {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO living_topic_understanding_jobs(id,topic_id,status,trigger,queued_at)
			VALUES(?,?,'pending',?,?)`, domain.NewID("topic_understanding"), topicID, trigger, now); err != nil {
			return false, fmt.Errorf("queue Living Topic understanding: %w", err)
		}
	} else if _, err := tx.ExecContext(ctx, `
		UPDATE living_topic_understanding_jobs SET trigger=?,queued_at=?
		WHERE id=(SELECT id FROM living_topic_understanding_jobs WHERE topic_id=? AND status='pending' ORDER BY queued_at DESC,id DESC LIMIT 1)`, trigger, now, topicID); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE living_topics SET understanding_status='pending',understanding_trigger=?,understanding_last_error='',updated_at=? WHERE id=?`, trigger, now, topicID); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return queued, nil
}

func (s *Store) ResetRunningLivingTopicUnderstanding(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE living_topic_understanding_jobs SET status='pending',started_at=NULL,last_error='' WHERE status='running'`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE living_topics SET understanding_status='pending'
		WHERE id IN (SELECT topic_id FROM living_topic_understanding_jobs WHERE status='pending')`); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ClaimLivingTopicUnderstanding(ctx context.Context) (*domain.LivingTopicUnderstandingJob, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var job domain.LivingTopicUnderstandingJob
	err = tx.QueryRowContext(ctx, `
		SELECT id,topic_id,trigger FROM living_topic_understanding_jobs
		WHERE status='pending' ORDER BY queued_at,id LIMIT 1`).Scan(&job.ID, &job.TopicID, &job.Trigger)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	now := memoryNow(s)
	result, err := tx.ExecContext(ctx, `UPDATE living_topic_understanding_jobs SET status='running',started_at=? WHERE id=? AND status='pending'`, now, job.ID)
	if err != nil {
		return nil, err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return nil, nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE living_topics SET understanding_status='running',understanding_trigger=?,understanding_last_error='' WHERE id=?`, job.Trigger, job.TopicID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &job, nil
}

func (s *Store) HasPendingLivingTopicUnderstanding(ctx context.Context) (bool, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM living_topic_understanding_jobs WHERE status='pending'`).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *Store) FinishLivingTopicUnderstanding(ctx context.Context, job domain.LivingTopicUnderstandingJob, outcome, digest, snapshotID string, jobErr error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := memoryNow(s)
	jobStatus, message := "completed", ""
	if jobErr != nil {
		jobStatus, message = "failed", boundedLivingTopicUnderstandingError(jobErr.Error())
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE living_topic_understanding_jobs
		SET status=?,outcome=?,input_digest=?,snapshot_id=?,last_error=?,completed_at=? WHERE id=?`,
		jobStatus, outcome, digest, nullableText(snapshotID), message, now, job.ID); err != nil {
		return err
	}
	var pending int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM living_topic_understanding_jobs WHERE topic_id=? AND status='pending'`, job.TopicID).Scan(&pending); err != nil {
		return err
	}
	status := "current"
	if jobErr != nil {
		status = "failed"
	} else if outcome == "insufficient_evidence" {
		status = "insufficient_evidence"
	}
	if pending > 0 {
		status = "pending"
	}
	if jobErr == nil {
		_, err = tx.ExecContext(ctx, `
			UPDATE living_topics SET understanding_status=?,understanding_input_digest=?,understanding_checked_at=?,
			understanding_trigger=?,understanding_last_error='',updated_at=? WHERE id=?`,
			status, digest, now, job.Trigger, now, job.TopicID)
	} else {
		_, err = tx.ExecContext(ctx, `
			UPDATE living_topics SET understanding_status=?,understanding_trigger=?,understanding_last_error=?,updated_at=? WHERE id=?`,
			status, job.Trigger, message, now, job.TopicID)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

func boundedLivingTopicUnderstandingError(value string) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) > 500 {
		runes = runes[:500]
	}
	return string(runes)
}

func (s *Store) LivingTopicIDsForMemory(ctx context.Context, memoryID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT topic_id FROM living_topic_memberships WHERE memory_item_id=? ORDER BY topic_id`, strings.TrimSpace(memoryID))
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
	return ids, rows.Err()
}
