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

const LivingTopicRoutingEngineVersion = "living-topics-routing-v1"

type LivingTopicFeedbackExample struct {
	TopicID string
	Verdict string
	Item    domain.MemoryItem
}

func (s *Store) QueueLivingTopicRouting(ctx context.Context, sessionID string) (int, error) {
	now := memoryNow(s)
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO living_topic_routing_jobs(id,session_id,timeline_id,status,engine_version,queued_at)
		SELECT 'topic_route_' || lower(hex(randomblob(16))),t.session_id,t.id,'pending',?,?
		FROM timeline_items t
		JOIN sessions s ON s.id=t.session_id AND s.status IN ('completed','partial')
		LEFT JOIN semantic_event_reports r ON r.timeline_id=t.id
		WHERE t.session_id=? AND COALESCE(r.relation,'')<>'duplicate_report'
		  AND EXISTS (SELECT 1 FROM living_topics)
		ON CONFLICT(timeline_id) DO NOTHING`, LivingTopicRoutingEngineVersion, now, strings.TrimSpace(sessionID))
	if err != nil {
		return 0, fmt.Errorf("queue Living Topics routing: %w", err)
	}
	count, _ := result.RowsAffected()
	return int(count), nil
}

func (s *Store) ResetRunningLivingTopicRouting(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `UPDATE living_topic_routing_jobs SET status='pending',started_at=NULL,last_error='' WHERE status='running'`)
	return err
}

func (s *Store) ClaimLivingTopicRouting(ctx context.Context) (*domain.LivingTopicRoutingJob, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var job domain.LivingTopicRoutingJob
	err = tx.QueryRowContext(ctx, `SELECT id,session_id,timeline_id FROM living_topic_routing_jobs WHERE status='pending' ORDER BY queued_at,id LIMIT 1`).Scan(&job.ID, &job.SessionID, &job.TimelineID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE living_topic_routing_jobs SET status='running',started_at=? WHERE id=? AND status='pending'`, memoryNow(s), job.ID)
	if err != nil {
		return nil, err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return nil, nil
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &job, nil
}

func (s *Store) HasPendingLivingTopicRouting(ctx context.Context) (bool, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM living_topic_routing_jobs WHERE status='pending'`).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *Store) FinishLivingTopicRouting(ctx context.Context, jobID string, decisions []domain.LivingTopicRoutingDecision, routeErr error) error {
	raw, _ := json.Marshal(decisions)
	status, message := "completed", ""
	if routeErr != nil {
		status, message = "failed", routeErr.Error()
	}
	_, err := s.db.ExecContext(ctx, `UPDATE living_topic_routing_jobs SET status=?,result_json=?,last_error=?,completed_at=? WHERE id=?`, status, string(raw), message, memoryNow(s), jobID)
	return err
}

func (s *Store) LivingTopicRoutingItem(ctx context.Context, timelineID string) (domain.TimelineItem, error) {
	var eligible int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM timeline_items t
		JOIN sessions s ON s.id=t.session_id AND s.status IN ('completed','partial')
		LEFT JOIN semantic_event_reports r ON r.timeline_id=t.id
		WHERE t.id=? AND COALESCE(r.relation,'')<>'duplicate_report'`, strings.TrimSpace(timelineID)).Scan(&eligible)
	if err != nil {
		return domain.TimelineItem{}, err
	}
	if eligible != 1 {
		return domain.TimelineItem{}, sql.ErrNoRows
	}
	return s.TimelineItem(ctx, timelineID)
}

func (s *Store) LivingTopicFeedbackExamples(ctx context.Context, limitPerVerdict int) ([]LivingTopicFeedbackExample, error) {
	if limitPerVerdict < 1 || limitPerVerdict > 5 {
		limitPerVerdict = 3
	}
	rows, err := s.db.QueryContext(ctx, `
		WITH latest AS (
		 SELECT topic_id,memory_item_id,verdict,created_at,id,rowid AS sequence,
		 ROW_NUMBER() OVER (PARTITION BY topic_id,memory_item_id ORDER BY created_at DESC,rowid DESC) pair_rank
		 FROM living_topic_feedback_events
		), bounded AS (
		 SELECT topic_id,memory_item_id,verdict,
		 ROW_NUMBER() OVER (PARTITION BY topic_id,verdict ORDER BY created_at DESC,sequence DESC) verdict_rank
		 FROM latest WHERE pair_rank=1 AND verdict IN ('include','exclude')
		)
		SELECT topic_id,memory_item_id,verdict FROM bounded WHERE verdict_rank<=?`, limitPerVerdict)
	if err != nil {
		return nil, err
	}
	type pendingExample struct{ topicID, memoryID, verdict string }
	pending := make([]pendingExample, 0)
	for rows.Next() {
		var topicID, memoryID, verdict string
		if err := rows.Scan(&topicID, &memoryID, &verdict); err != nil {
			rows.Close()
			return nil, err
		}
		pending = append(pending, pendingExample{topicID: topicID, memoryID: memoryID, verdict: verdict})
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	values := make([]LivingTopicFeedbackExample, 0, len(pending))
	for _, candidate := range pending {
		item, err := memoryItemByQueryer(ctx, s.db, candidate.memoryID)
		if errors.Is(err, ErrMemoryNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		values = append(values, LivingTopicFeedbackExample{TopicID: candidate.topicID, Verdict: candidate.verdict, Item: item})
	}
	return values, nil
}

func (s *Store) AddAutomaticLivingTopicMember(ctx context.Context, topicID string, item domain.TimelineItem, decision domain.LivingTopicRoutingDecision) (bool, error) {
	topicID = strings.TrimSpace(topicID)
	input := routineMoreMemoryInput(item)
	input.Reason = "living_topic_automatic"
	input.Provenance = []domain.MemoryProvenance{{
		ProvenanceKind: "captured", Source: item.Source, CanonicalEvidenceKey: item.EvidenceKey,
		SourceURL: item.Item.SourceURL, Reason: "living_topic_automatic",
		CaptureContext: map[string]any{"surface": "timeline", "action": "automatic_topic_routing", "timelineId": item.ID, "sessionId": item.SessionID, "topicId": topicID},
	}}
	normalized, err := normalizeMemoryInput(input)
	if err != nil {
		return false, err
	}
	lookup, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return false, err
	}
	memoryID, resolveErr := resolveMemoryIdentity(ctx, lookup, normalized.Identity)
	_ = lookup.Rollback()
	if resolveErr != nil {
		return false, resolveErr
	}
	if memoryID == "" {
		memory, createErr := s.UpsertMemoryRecallStub(ctx, input)
		if createErr != nil {
			return false, createErr
		}
		memoryID = memory.ID
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM living_topic_memberships WHERE topic_id=? AND memory_item_id=?`, topicID, memoryID).Scan(&exists); err != nil {
		return false, err
	}
	if exists > 0 {
		return false, nil
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM living_topic_memberships WHERE topic_id=?`, topicID).Scan(&count); err != nil {
		return false, err
	}
	if count >= LivingTopicMaxMembers {
		return false, ErrLivingTopicMemberMax
	}
	var latestVerdict string
	err = tx.QueryRowContext(ctx, `SELECT verdict FROM living_topic_feedback_events WHERE topic_id=? AND memory_item_id=? ORDER BY created_at DESC,rowid DESC LIMIT 1`, topicID, memoryID).Scan(&latestVerdict)
	if err == nil && latestVerdict == "exclude" {
		return false, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	now := memoryNow(s)
	result, err := tx.ExecContext(ctx, `
		INSERT INTO living_topic_memberships(topic_id,memory_item_id,added_at,origin,match_mode,confidence,reason,new_evidence,new_evidence_at)
		VALUES(?,?,?,'automatic',?,?,?,1,?)
		ON CONFLICT(topic_id,memory_item_id) DO NOTHING`, topicID, memoryID, now, decision.Mode, decision.Confidence, decision.Reason, now)
	if err != nil {
		return false, err
	}
	added, _ := result.RowsAffected()
	if added == 0 {
		return false, nil
	}
	topicResult, err := tx.ExecContext(ctx, `UPDATE living_topics SET updated_at=? WHERE id=?`, now, topicID)
	if err != nil {
		return false, err
	}
	if count, _ := topicResult.RowsAffected(); count != 1 {
		return false, ErrLivingTopicNotFound
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}
