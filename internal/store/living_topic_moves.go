package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

var (
	ErrLivingTopicMoveSameTopic      = errors.New("living topic evidence already belongs to the selected destination")
	ErrLivingTopicMoveNotFound       = errors.New("living topic move was not found")
	ErrLivingTopicMoveNotCurrent     = errors.New("only the latest active living topic move can be undone")
	ErrLivingTopicMoveSourceConflict = errors.New("living topic move cannot be undone because the source already contains this evidence")
)

func (s *Store) MoveLivingTopicMember(ctx context.Context, fromTopicID, toTopicID, memoryID string) (domain.LivingTopicMembershipMove, error) {
	fromTopicID = strings.TrimSpace(fromTopicID)
	toTopicID = strings.TrimSpace(toTopicID)
	memoryID = strings.TrimSpace(memoryID)
	if fromTopicID == toTopicID {
		return domain.LivingTopicMembershipMove{}, ErrLivingTopicMoveSameTopic
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.LivingTopicMembershipMove{}, err
	}
	defer tx.Rollback()
	for _, topicID := range []string{fromTopicID, toTopicID} {
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM living_topics WHERE id=?`, topicID).Scan(&exists); err != nil {
			return domain.LivingTopicMembershipMove{}, err
		}
		if exists == 0 {
			return domain.LivingTopicMembershipMove{}, ErrLivingTopicNotFound
		}
	}
	var lifecycle string
	if err := tx.QueryRowContext(ctx, `SELECT lifecycle_state FROM memory_items WHERE id=?`, memoryID).Scan(&lifecycle); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.LivingTopicMembershipMove{}, ErrMemoryNotFound
		}
		return domain.LivingTopicMembershipMove{}, err
	}
	if lifecycle != string(domain.MemoryStateActive) {
		return domain.LivingTopicMembershipMove{}, ErrMemoryNotFound
	}

	move := domain.LivingTopicMembershipMove{
		ID:           domain.NewID("topic_move"),
		MemoryItemID: memoryID,
		FromTopicID:  fromTopicID,
		ToTopicID:    toTopicID,
		CreatedAt:    memoryNow(s),
	}
	var sourceNewEvidence int
	var sourceNewEvidenceAt, sourceMoveID sql.NullString
	if err := tx.QueryRowContext(ctx, `
		SELECT origin,match_mode,confidence,reason,added_at,new_evidence,new_evidence_at,move_id
		FROM living_topic_memberships WHERE topic_id=? AND memory_item_id=?`, fromTopicID, memoryID).
		Scan(&move.SourceOrigin, &move.SourceMatchMode, &move.SourceConfidence, &move.SourceReason,
			&move.SourceAddedAt, &sourceNewEvidence, &sourceNewEvidenceAt, &sourceMoveID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.LivingTopicMembershipMove{}, ErrLivingTopicMoveNotFound
		}
		return domain.LivingTopicMembershipMove{}, err
	}
	move.SourceNewEvidence = sourceNewEvidence == 1
	move.SourceNewEvidenceAt = sourceNewEvidenceAt.String
	move.SourceMoveID = sourceMoveID.String

	var targetExists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM living_topic_memberships WHERE topic_id=? AND memory_item_id=?`, toTopicID, memoryID).Scan(&targetExists); err != nil {
		return domain.LivingTopicMembershipMove{}, err
	}
	move.TargetPreexisted = targetExists > 0
	if !move.TargetPreexisted {
		var targetCount int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM living_topic_memberships WHERE topic_id=?`, toTopicID).Scan(&targetCount); err != nil {
			return domain.LivingTopicMembershipMove{}, err
		}
		if targetCount >= LivingTopicMaxMembers {
			return domain.LivingTopicMembershipMove{}, ErrLivingTopicMemberMax
		}
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO living_topic_membership_moves(
		 id,memory_item_id,from_topic_id,to_topic_id,source_origin,source_match_mode,source_confidence,
		 source_reason,source_added_at,source_new_evidence,source_new_evidence_at,source_move_id,target_preexisted,created_at
		) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		move.ID, move.MemoryItemID, move.FromTopicID, move.ToTopicID, move.SourceOrigin, move.SourceMatchMode,
		move.SourceConfidence, move.SourceReason, move.SourceAddedAt, sourceNewEvidence,
		nullableText(move.SourceNewEvidenceAt), nullableText(move.SourceMoveID), boolInt(move.TargetPreexisted), move.CreatedAt); err != nil {
		return domain.LivingTopicMembershipMove{}, fmt.Errorf("record Living Topic move: %w", err)
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM living_topic_memberships WHERE topic_id=? AND memory_item_id=?`, fromTopicID, memoryID)
	if err != nil {
		return domain.LivingTopicMembershipMove{}, err
	}
	if count, _ := result.RowsAffected(); count != 1 {
		return domain.LivingTopicMembershipMove{}, ErrLivingTopicMoveNotFound
	}
	if !move.TargetPreexisted {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO living_topic_memberships(
			 topic_id,memory_item_id,added_at,origin,match_mode,confidence,reason,new_evidence,new_evidence_at,move_id
			) VALUES(?,?,?,'manual','move',1,'Moved by user',0,NULL,?)`, toTopicID, memoryID, move.CreatedAt, move.ID); err != nil {
			return domain.LivingTopicMembershipMove{}, err
		}
	}
	if err := appendLivingTopicMoveFeedback(ctx, tx, fromTopicID, toTopicID, memoryID, "exclude", "include", move.CreatedAt); err != nil {
		return domain.LivingTopicMembershipMove{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE living_topics SET updated_at=? WHERE id IN (?,?)`, move.CreatedAt, fromTopicID, toTopicID); err != nil {
		return domain.LivingTopicMembershipMove{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.LivingTopicMembershipMove{}, err
	}
	return move, nil
}

func (s *Store) UndoLivingTopicMemberMove(ctx context.Context, moveID string) (domain.LivingTopicMembershipMove, error) {
	moveID = strings.TrimSpace(moveID)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.LivingTopicMembershipMove{}, err
	}
	defer tx.Rollback()
	move, err := livingTopicMoveByID(ctx, tx, moveID)
	if err != nil {
		return domain.LivingTopicMembershipMove{}, err
	}
	if move.UndoneAt != "" {
		return domain.LivingTopicMembershipMove{}, ErrLivingTopicMoveNotCurrent
	}
	var latestMoveID string
	if err := tx.QueryRowContext(ctx, `
		SELECT id FROM living_topic_membership_moves
		WHERE memory_item_id=? AND undone_at IS NULL ORDER BY rowid DESC LIMIT 1`, move.MemoryItemID).Scan(&latestMoveID); err != nil {
		return domain.LivingTopicMembershipMove{}, err
	}
	if latestMoveID != move.ID {
		return domain.LivingTopicMembershipMove{}, ErrLivingTopicMoveNotCurrent
	}
	var lifecycle string
	if err := tx.QueryRowContext(ctx, `SELECT lifecycle_state FROM memory_items WHERE id=?`, move.MemoryItemID).Scan(&lifecycle); err != nil || lifecycle != string(domain.MemoryStateActive) {
		if errors.Is(err, sql.ErrNoRows) || lifecycle != string(domain.MemoryStateActive) {
			return domain.LivingTopicMembershipMove{}, ErrMemoryNotFound
		}
		return domain.LivingTopicMembershipMove{}, err
	}
	var sourceExists int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM living_topic_memberships WHERE topic_id=? AND memory_item_id=?`, move.FromTopicID, move.MemoryItemID).Scan(&sourceExists); err != nil {
		return domain.LivingTopicMembershipMove{}, err
	}
	if sourceExists > 0 {
		return domain.LivingTopicMembershipMove{}, ErrLivingTopicMoveSourceConflict
	}
	var sourceCount int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM living_topic_memberships WHERE topic_id=?`, move.FromTopicID).Scan(&sourceCount); err != nil {
		return domain.LivingTopicMembershipMove{}, err
	}
	if sourceCount >= LivingTopicMaxMembers {
		return domain.LivingTopicMembershipMove{}, ErrLivingTopicMemberMax
	}

	if !move.TargetPreexisted {
		if _, err := tx.ExecContext(ctx, `DELETE FROM living_topic_memberships WHERE topic_id=? AND memory_item_id=? AND move_id=?`, move.ToTopicID, move.MemoryItemID, move.ID); err != nil {
			return domain.LivingTopicMembershipMove{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO living_topic_memberships(
		 topic_id,memory_item_id,added_at,origin,match_mode,confidence,reason,new_evidence,new_evidence_at,move_id
		) VALUES(?,?,?,?,?,?,?,?,?,?)`, move.FromTopicID, move.MemoryItemID, move.SourceAddedAt, move.SourceOrigin,
		move.SourceMatchMode, move.SourceConfidence, move.SourceReason, boolInt(move.SourceNewEvidence),
		nullableText(move.SourceNewEvidenceAt), nullableText(move.SourceMoveID)); err != nil {
		return domain.LivingTopicMembershipMove{}, err
	}
	now := memoryNow(s)
	if err := appendLivingTopicMoveFeedback(ctx, tx, move.FromTopicID, move.ToTopicID, move.MemoryItemID, "clear", "clear", now); err != nil {
		return domain.LivingTopicMembershipMove{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE living_topic_membership_moves SET undone_at=? WHERE id=? AND undone_at IS NULL`, now, move.ID); err != nil {
		return domain.LivingTopicMembershipMove{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE living_topics SET updated_at=? WHERE id IN (?,?)`, now, move.FromTopicID, move.ToTopicID); err != nil {
		return domain.LivingTopicMembershipMove{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.LivingTopicMembershipMove{}, err
	}
	move.UndoneAt = now
	return move, nil
}

type livingTopicMoveScanner interface{ Scan(...any) error }

func livingTopicMoveByID(ctx context.Context, queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id string) (domain.LivingTopicMembershipMove, error) {
	return scanLivingTopicMove(queryer.QueryRowContext(ctx, `
		SELECT id,memory_item_id,from_topic_id,to_topic_id,source_origin,source_match_mode,source_confidence,
		 source_reason,source_added_at,source_new_evidence,source_new_evidence_at,source_move_id,target_preexisted,created_at,undone_at
		FROM living_topic_membership_moves WHERE id=?`, strings.TrimSpace(id)))
}

func scanLivingTopicMove(row livingTopicMoveScanner) (domain.LivingTopicMembershipMove, error) {
	var value domain.LivingTopicMembershipMove
	var sourceNewEvidence, targetPreexisted int
	var sourceNewEvidenceAt, sourceMoveID, undoneAt sql.NullString
	if err := row.Scan(&value.ID, &value.MemoryItemID, &value.FromTopicID, &value.ToTopicID, &value.SourceOrigin,
		&value.SourceMatchMode, &value.SourceConfidence, &value.SourceReason, &value.SourceAddedAt, &sourceNewEvidence,
		&sourceNewEvidenceAt, &sourceMoveID, &targetPreexisted, &value.CreatedAt, &undoneAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.LivingTopicMembershipMove{}, ErrLivingTopicMoveNotFound
		}
		return domain.LivingTopicMembershipMove{}, err
	}
	value.SourceNewEvidence = sourceNewEvidence == 1
	value.SourceNewEvidenceAt = sourceNewEvidenceAt.String
	value.SourceMoveID = sourceMoveID.String
	value.TargetPreexisted = targetPreexisted == 1
	value.UndoneAt = undoneAt.String
	return value, nil
}

func appendLivingTopicMoveFeedback(ctx context.Context, tx *sql.Tx, fromTopicID, toTopicID, memoryID, fromVerdict, toVerdict, createdAt string) error {
	for _, event := range []struct {
		topicID string
		verdict string
	}{{fromTopicID, fromVerdict}, {toTopicID, toVerdict}} {
		if _, err := tx.ExecContext(ctx, `INSERT INTO living_topic_feedback_events(id,topic_id,memory_item_id,verdict,created_at) VALUES(?,?,?,?,?)`,
			domain.NewID("topic_feedback"), event.topicID, memoryID, event.verdict, createdAt); err != nil {
			return err
		}
	}
	return nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
