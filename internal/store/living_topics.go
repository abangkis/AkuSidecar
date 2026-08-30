package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

const (
	LivingTopicMaxNameRunes        = 120
	LivingTopicMaxDescriptionRunes = 1200
	LivingTopicMaxMembers          = 20
	LivingTopicMaxHistory          = 20
)

var (
	ErrLivingTopicNotFound    = errors.New("living topic not found")
	ErrLivingTopicName        = errors.New("living topic name must contain between 1 and 120 characters")
	ErrLivingTopicDescription = errors.New("living topic description cannot exceed 1200 characters")
	ErrLivingTopicMemberMax   = errors.New("living topic already contains 20 Memory items")
)

func normalizeLivingTopicName(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || utf8.RuneCountInString(value) > LivingTopicMaxNameRunes {
		return "", ErrLivingTopicName
	}
	return value, nil
}

func (s *Store) CreateLivingTopic(ctx context.Context, name string) (domain.LivingTopic, error) {
	return s.CreateLivingTopicWithCriteria(ctx, name, "")
}

func (s *Store) CreateLivingTopicWithCriteria(ctx context.Context, name, description string) (domain.LivingTopic, error) {
	name, err := normalizeLivingTopicName(name)
	if err != nil {
		return domain.LivingTopic{}, err
	}
	description = strings.TrimSpace(description)
	if utf8.RuneCountInString(description) > LivingTopicMaxDescriptionRunes {
		return domain.LivingTopic{}, ErrLivingTopicDescription
	}
	now := memoryNow(s)
	id := domain.NewID("topic")
	if _, err := s.db.ExecContext(ctx, `INSERT INTO living_topics(id,name,description,created_at,updated_at) VALUES(?,?,?,?,?)`, id, name, description, now, now); err != nil {
		return domain.LivingTopic{}, fmt.Errorf("create living topic: %w", err)
	}
	return s.LivingTopic(ctx, id)
}

func (s *Store) RenameLivingTopic(ctx context.Context, id, name string) (domain.LivingTopic, error) {
	current, err := s.LivingTopic(ctx, id)
	if err != nil {
		return domain.LivingTopic{}, err
	}
	return s.UpdateLivingTopicCriteria(ctx, id, name, current.Description)
}

func (s *Store) UpdateLivingTopicCriteria(ctx context.Context, id, name, description string) (domain.LivingTopic, error) {
	name, err := normalizeLivingTopicName(name)
	if err != nil {
		return domain.LivingTopic{}, err
	}
	description = strings.TrimSpace(description)
	if utf8.RuneCountInString(description) > LivingTopicMaxDescriptionRunes {
		return domain.LivingTopic{}, ErrLivingTopicDescription
	}
	result, err := s.db.ExecContext(ctx, `UPDATE living_topics SET name=?,description=?,updated_at=? WHERE id=?`, name, description, memoryNow(s), strings.TrimSpace(id))
	if err != nil {
		return domain.LivingTopic{}, fmt.Errorf("rename living topic: %w", err)
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return domain.LivingTopic{}, ErrLivingTopicNotFound
	}
	return s.LivingTopic(ctx, id)
}

func (s *Store) LivingTopic(ctx context.Context, id string) (domain.LivingTopic, error) {
	var topic domain.LivingTopic
	err := s.db.QueryRowContext(ctx, `
		SELECT t.id,t.name,t.description,t.understanding_status,t.understanding_input_digest,
		       COALESCE(t.understanding_checked_at,''),t.understanding_trigger,t.understanding_last_error,
		       t.created_at,t.updated_at,
		       (SELECT COUNT(*) FROM living_topic_memberships m
		        JOIN memory_items i ON i.id=m.memory_item_id AND i.lifecycle_state='active'
		        WHERE m.topic_id=t.id)
		FROM living_topics t WHERE t.id=?`, strings.TrimSpace(id)).Scan(
		&topic.ID, &topic.Name, &topic.Description, &topic.UnderstandingStatus, &topic.UnderstandingInputDigest,
		&topic.UnderstandingCheckedAt, &topic.UnderstandingTrigger, &topic.UnderstandingLastError,
		&topic.CreatedAt, &topic.UpdatedAt, &topic.MemberCount)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.LivingTopic{}, ErrLivingTopicNotFound
	}
	if err != nil {
		return domain.LivingTopic{}, fmt.Errorf("read living topic: %w", err)
	}
	latest, err := s.LatestPublishedLivingTopicSnapshot(ctx, topic.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return domain.LivingTopic{}, err
	}
	if err == nil {
		topic.LatestSnapshot = &latest
	}
	return topic, nil
}

func (s *Store) ListLivingTopics(ctx context.Context) ([]domain.LivingTopic, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM living_topics ORDER BY updated_at DESC,id DESC LIMIT 100`)
	if err != nil {
		return nil, fmt.Errorf("list living topics: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	topics := make([]domain.LivingTopic, 0, len(ids))
	for _, id := range ids {
		topic, err := s.LivingTopic(ctx, id)
		if err != nil {
			return nil, err
		}
		topics = append(topics, topic)
	}
	return topics, nil
}

func (s *Store) AddLivingTopicMember(ctx context.Context, topicID, memoryID string) (domain.LivingTopicDetail, error) {
	topicID, memoryID = strings.TrimSpace(topicID), strings.TrimSpace(memoryID)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.LivingTopicDetail{}, err
	}
	defer tx.Rollback()
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM living_topics WHERE id=?`, topicID).Scan(&exists); err != nil {
		return domain.LivingTopicDetail{}, err
	}
	if exists == 0 {
		return domain.LivingTopicDetail{}, ErrLivingTopicNotFound
	}
	var lifecycle string
	if err := tx.QueryRowContext(ctx, `SELECT lifecycle_state FROM memory_items WHERE id=?`, memoryID).Scan(&lifecycle); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return domain.LivingTopicDetail{}, err
		}
		return domain.LivingTopicDetail{}, ErrMemoryNotFound
	}
	if lifecycle != string(domain.MemoryStateActive) {
		return domain.LivingTopicDetail{}, ErrMemoryNotFound
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM living_topic_memberships WHERE topic_id=? AND memory_item_id=?`, topicID, memoryID).Scan(&exists); err != nil {
		return domain.LivingTopicDetail{}, err
	}
	if exists == 0 {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM living_topic_memberships WHERE topic_id=?`, topicID).Scan(&count); err != nil {
			return domain.LivingTopicDetail{}, err
		}
		if count >= LivingTopicMaxMembers {
			return domain.LivingTopicDetail{}, ErrLivingTopicMemberMax
		}
		now := memoryNow(s)
		if _, err := tx.ExecContext(ctx, `INSERT INTO living_topic_memberships(topic_id,memory_item_id,added_at,origin,match_mode,confidence,reason) VALUES(?,?,?,'manual','manual',1,'Added by user')`, topicID, memoryID, now); err != nil {
			return domain.LivingTopicDetail{}, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE living_topics SET updated_at=? WHERE id=?`, now, topicID); err != nil {
			return domain.LivingTopicDetail{}, err
		}
	} else if _, err := tx.ExecContext(ctx, `UPDATE living_topic_memberships SET origin='manual',match_mode='manual',confidence=1,reason='Added by user' WHERE topic_id=? AND memory_item_id=?`, topicID, memoryID); err != nil {
		return domain.LivingTopicDetail{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO living_topic_feedback_events(id,topic_id,memory_item_id,verdict,created_at) VALUES(?,?,?,'include',?)`, domain.NewID("topic_feedback"), topicID, memoryID, memoryNow(s)); err != nil {
		return domain.LivingTopicDetail{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.LivingTopicDetail{}, err
	}
	return s.LivingTopicDetail(ctx, topicID)
}

func (s *Store) RemoveLivingTopicMember(ctx context.Context, topicID, memoryID string) (domain.LivingTopicDetail, error) {
	if _, err := s.LivingTopic(ctx, topicID); err != nil {
		return domain.LivingTopicDetail{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.LivingTopicDetail{}, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `DELETE FROM living_topic_memberships WHERE topic_id=? AND memory_item_id=?`, strings.TrimSpace(topicID), strings.TrimSpace(memoryID))
	if err != nil {
		return domain.LivingTopicDetail{}, err
	}
	if removed, _ := result.RowsAffected(); removed > 0 {
		now := memoryNow(s)
		if _, err := tx.ExecContext(ctx, `INSERT INTO living_topic_feedback_events(id,topic_id,memory_item_id,verdict,created_at) VALUES(?,?,?,'exclude',?)`, domain.NewID("topic_feedback"), strings.TrimSpace(topicID), strings.TrimSpace(memoryID), now); err != nil {
			return domain.LivingTopicDetail{}, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE living_topics SET updated_at=? WHERE id=?`, now, strings.TrimSpace(topicID)); err != nil {
			return domain.LivingTopicDetail{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return domain.LivingTopicDetail{}, err
	}
	return s.LivingTopicDetail(ctx, topicID)
}

func (s *Store) LivingTopicDetail(ctx context.Context, id string) (domain.LivingTopicDetail, error) {
	topic, err := s.LivingTopic(ctx, id)
	if err != nil {
		return domain.LivingTopicDetail{}, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT m.memory_item_id,m.origin,m.match_mode,m.confidence,m.reason,m.added_at FROM living_topic_memberships m
		JOIN memory_items i ON i.id=m.memory_item_id AND i.lifecycle_state='active'
		WHERE m.topic_id=? ORDER BY m.added_at ASC,m.memory_item_id ASC LIMIT ?`, id, LivingTopicMaxMembers)
	if err != nil {
		return domain.LivingTopicDetail{}, err
	}
	var memberIDs []string
	memberships := make([]domain.LivingTopicMembership, 0)
	for rows.Next() {
		var membership domain.LivingTopicMembership
		if err := rows.Scan(&membership.MemoryItemID, &membership.Origin, &membership.MatchMode, &membership.Confidence, &membership.Reason, &membership.AddedAt); err != nil {
			rows.Close()
			return domain.LivingTopicDetail{}, err
		}
		memberIDs = append(memberIDs, membership.MemoryItemID)
		memberships = append(memberships, membership)
	}
	if err := rows.Close(); err != nil {
		return domain.LivingTopicDetail{}, err
	}
	members := make([]domain.MemoryItem, 0, len(memberIDs))
	for _, memoryID := range memberIDs {
		item, err := memoryItemByQueryer(ctx, s.db, memoryID)
		if err != nil {
			return domain.LivingTopicDetail{}, err
		}
		members = append(members, item)
	}
	snapshots, err := s.LivingTopicSnapshots(ctx, id, LivingTopicMaxHistory)
	if err != nil {
		return domain.LivingTopicDetail{}, err
	}
	return domain.LivingTopicDetail{Topic: topic, Members: members, Memberships: memberships, Snapshots: snapshots}, nil
}

func (s *Store) SaveLivingTopicSnapshot(ctx context.Context, value domain.LivingTopicSnapshot) (domain.LivingTopicSnapshot, error) {
	if err := validateLivingTopicSnapshot(value); err != nil {
		return domain.LivingTopicSnapshot{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.LivingTopicSnapshot{}, err
	}
	defer tx.Rollback()
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM living_topics WHERE id=?`, value.TopicID).Scan(&exists); err != nil {
		return domain.LivingTopicSnapshot{}, err
	}
	if exists == 0 {
		return domain.LivingTopicSnapshot{}, ErrLivingTopicNotFound
	}
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0)+1 FROM living_topic_snapshots WHERE topic_id=?`, value.TopicID).Scan(&value.Version); err != nil {
		return domain.LivingTopicSnapshot{}, err
	}
	if value.ID == "" {
		value.ID = domain.NewID("topic_snapshot")
	}
	if value.CreatedAt == "" {
		value.CreatedAt = memoryNow(s)
	}
	claims, _ := json.Marshal(value.Claims)
	deltas, _ := json.Marshal(value.Deltas)
	evidence, _ := json.Marshal(value.EvidenceIDs)
	usage, _ := json.Marshal(value.Usage)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO living_topic_snapshots(
		 id,topic_id,version,status,overview,claims_json,deltas_json,evidence_ids_json,input_digest,
		 provider,model,effort,duration_ms,usage_json,previous_snapshot_id,created_at
		) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		value.ID, value.TopicID, value.Version, value.Status, value.Overview, string(claims), string(deltas), string(evidence), value.InputDigest,
		value.Provider, value.Model, value.Effort, value.DurationMS, string(usage), nullableText(value.PreviousSnapshotID), value.CreatedAt); err != nil {
		return domain.LivingTopicSnapshot{}, fmt.Errorf("save living topic snapshot: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE living_topics SET updated_at=? WHERE id=?`, value.CreatedAt, value.TopicID); err != nil {
		return domain.LivingTopicSnapshot{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.LivingTopicSnapshot{}, err
	}
	return value, nil
}

func validateLivingTopicSnapshot(value domain.LivingTopicSnapshot) error {
	if strings.TrimSpace(value.TopicID) == "" || strings.TrimSpace(value.Overview) == "" || utf8.RuneCountInString(value.Overview) > 1200 {
		return errors.New("living topic snapshot requires a topic and a bounded overview")
	}
	if value.Status != "ready" && value.Status != "no_change" && value.Status != "insufficient_evidence" {
		return errors.New("living topic snapshot has an unsupported status")
	}
	if len(value.Claims) > 8 || len(value.Deltas) > 8 || len(value.EvidenceIDs) > LivingTopicMaxMembers {
		return errors.New("living topic snapshot exceeds bounded evidence or statement limits")
	}
	if value.Status == "ready" && len(value.Claims) == 0 {
		return errors.New("ready living topic snapshot requires at least one claim")
	}
	return nil
}

func nullableText(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func (s *Store) LatestLivingTopicSnapshot(ctx context.Context, topicID string) (domain.LivingTopicSnapshot, error) {
	return scanLivingTopicSnapshot(s.db.QueryRowContext(ctx, `
		SELECT id,topic_id,version,status,overview,claims_json,deltas_json,evidence_ids_json,input_digest,
		       provider,model,effort,duration_ms,usage_json,previous_snapshot_id,created_at
		FROM living_topic_snapshots WHERE topic_id=? ORDER BY version DESC LIMIT 1`, topicID))
}

// LatestPublishedLivingTopicSnapshot excludes earlier no-change and
// insufficient-evidence receipts. New automatic evaluations publish a version
// only when the source-backed understanding changes materially.
func (s *Store) LatestPublishedLivingTopicSnapshot(ctx context.Context, topicID string) (domain.LivingTopicSnapshot, error) {
	return scanLivingTopicSnapshot(s.db.QueryRowContext(ctx, `
		SELECT id,topic_id,version,status,overview,claims_json,deltas_json,evidence_ids_json,input_digest,
		       provider,model,effort,duration_ms,usage_json,previous_snapshot_id,created_at
		FROM living_topic_snapshots WHERE topic_id=? AND status='ready' ORDER BY version DESC LIMIT 1`, topicID))
}

func (s *Store) LivingTopicSnapshots(ctx context.Context, topicID string, limit int) ([]domain.LivingTopicSnapshot, error) {
	if limit < 1 || limit > LivingTopicMaxHistory {
		limit = LivingTopicMaxHistory
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id,topic_id,version,status,overview,claims_json,deltas_json,evidence_ids_json,input_digest,
		       provider,model,effort,duration_ms,usage_json,previous_snapshot_id,created_at
		FROM living_topic_snapshots WHERE topic_id=? AND status='ready' ORDER BY version DESC LIMIT ?`, topicID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]domain.LivingTopicSnapshot, 0)
	for rows.Next() {
		value, err := scanLivingTopicSnapshot(rows)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

type livingTopicScanner interface{ Scan(...any) error }

func scanLivingTopicSnapshot(row livingTopicScanner) (domain.LivingTopicSnapshot, error) {
	var value domain.LivingTopicSnapshot
	var claims, deltas, evidence, usage string
	var previous sql.NullString
	err := row.Scan(&value.ID, &value.TopicID, &value.Version, &value.Status, &value.Overview, &claims, &deltas, &evidence, &value.InputDigest,
		&value.Provider, &value.Model, &value.Effort, &value.DurationMS, &usage, &previous, &value.CreatedAt)
	if err != nil {
		return domain.LivingTopicSnapshot{}, err
	}
	if err := json.Unmarshal([]byte(claims), &value.Claims); err != nil {
		return domain.LivingTopicSnapshot{}, err
	}
	if err := json.Unmarshal([]byte(deltas), &value.Deltas); err != nil {
		return domain.LivingTopicSnapshot{}, err
	}
	if err := json.Unmarshal([]byte(evidence), &value.EvidenceIDs); err != nil {
		return domain.LivingTopicSnapshot{}, err
	}
	if err := json.Unmarshal([]byte(usage), &value.Usage); err != nil {
		return domain.LivingTopicSnapshot{}, err
	}
	if previous.Valid {
		value.PreviousSnapshotID = previous.String
	}
	if value.Claims == nil {
		value.Claims = []domain.LivingTopicClaim{}
	}
	if value.Deltas == nil {
		value.Deltas = []domain.LivingTopicDelta{}
	}
	if value.EvidenceIDs == nil {
		value.EvidenceIDs = []string{}
	}
	return value, nil
}
