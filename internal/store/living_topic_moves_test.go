package store

import (
	"context"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestLivingTopicMoveAndUndoPreserveMembershipAndLearning(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	from, err := state.CreateLivingTopic(ctx, "Codex Reset")
	if err != nil {
		t.Fatal(err)
	}
	to, err := state.CreateLivingTopic(ctx, "Codex")
	if err != nil {
		t.Fatal(err)
	}
	item, err := state.CreateMemoryRecallStub(ctx, libraryInput("move-topic", domain.SourceX, "Codex usage limits", "Limits drain after updating", "2026-08-30T00:00:00Z"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.AddLivingTopicMember(ctx, from.ID, item.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.ExecContext(ctx, `
		UPDATE living_topic_memberships SET origin='automatic',match_mode='llm',confidence=.95,reason='Reset classifier',new_evidence=1,new_evidence_at='2026-08-30T01:00:00Z'
		WHERE topic_id=? AND memory_item_id=?`, from.ID, item.ID); err != nil {
		t.Fatal(err)
	}

	move, err := state.MoveLivingTopicMember(ctx, from.ID, to.ID, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if move.SourceOrigin != "automatic" || move.SourceMatchMode != "llm" || move.SourceConfidence != .95 || !move.SourceNewEvidence || move.TargetPreexisted {
		t.Fatalf("move receipt=%+v", move)
	}
	var sourceCount, targetCount, targetUnread int
	var targetMoveID string
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM living_topic_memberships WHERE topic_id=? AND memory_item_id=?`, from.ID, item.ID).Scan(&sourceCount); err != nil {
		t.Fatal(err)
	}
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(MAX(new_evidence),0),COALESCE(MAX(move_id),'') FROM living_topic_memberships WHERE topic_id=? AND memory_item_id=?`, to.ID, item.ID).Scan(&targetCount, &targetUnread, &targetMoveID); err != nil {
		t.Fatal(err)
	}
	if sourceCount != 0 || targetCount != 1 || targetUnread != 0 || targetMoveID != move.ID {
		t.Fatalf("after move source=%d target=%d unread=%d moveID=%q", sourceCount, targetCount, targetUnread, targetMoveID)
	}
	if stored, err := state.MemoryItem(ctx, item.ID); err != nil || stored.LifecycleState != domain.MemoryStateActive || stored.RetentionTier != domain.MemoryTierRecall {
		t.Fatalf("recall stub changed by move: item=%+v err=%v", stored, err)
	}
	var includeFeedback, excludeFeedback int
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM living_topic_feedback_events WHERE topic_id=? AND memory_item_id=? AND verdict='include'`, to.ID, item.ID).Scan(&includeFeedback); err != nil {
		t.Fatal(err)
	}
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM living_topic_feedback_events WHERE topic_id=? AND memory_item_id=? AND verdict='exclude'`, from.ID, item.ID).Scan(&excludeFeedback); err != nil {
		t.Fatal(err)
	}
	if includeFeedback != 1 || excludeFeedback != 1 {
		t.Fatalf("contrastive feedback include=%d exclude=%d", includeFeedback, excludeFeedback)
	}

	undone, err := state.UndoLivingTopicMemberMove(ctx, move.ID)
	if err != nil {
		t.Fatal(err)
	}
	if undone.UndoneAt == "" {
		t.Fatalf("undo receipt=%+v", undone)
	}
	var origin, mode, reason, addedAt, unreadAt, restoredMoveID string
	var confidence float64
	var unread int
	if err := state.db.QueryRowContext(ctx, `
		SELECT origin,match_mode,confidence,reason,added_at,new_evidence,COALESCE(new_evidence_at,''),COALESCE(move_id,'')
		FROM living_topic_memberships WHERE topic_id=? AND memory_item_id=?`, from.ID, item.ID).
		Scan(&origin, &mode, &confidence, &reason, &addedAt, &unread, &unreadAt, &restoredMoveID); err != nil {
		t.Fatal(err)
	}
	if origin != "automatic" || mode != "llm" || confidence != .95 || reason != "Reset classifier" || unread != 1 || unreadAt != "2026-08-30T01:00:00Z" || restoredMoveID != "" {
		t.Fatalf("restored membership origin=%q mode=%q confidence=%v reason=%q added=%q unread=%d at=%q move=%q", origin, mode, confidence, reason, addedAt, unread, unreadAt, restoredMoveID)
	}
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM living_topic_memberships WHERE topic_id=? AND memory_item_id=?`, to.ID, item.ID).Scan(&targetCount); err != nil || targetCount != 0 {
		t.Fatalf("target after undo=%d err=%v", targetCount, err)
	}
	var clearCount int
	if err := state.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM living_topic_feedback_events WHERE memory_item_id=? AND verdict='clear'`, item.ID).Scan(&clearCount); err != nil || clearCount != 2 {
		t.Fatalf("clear feedback=%d err=%v", clearCount, err)
	}
}

func TestLivingTopicSnapshotBecomesHistoricalWhenEvidenceLeavesTopic(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	topic, err := state.CreateLivingTopic(ctx, "Quantum Systems")
	if err != nil {
		t.Fatal(err)
	}
	item, err := state.CreateMemoryRecallStub(ctx, libraryInput("topic-current", domain.SourceX, "Quantum system research", "A source-backed finding", "2026-08-30T00:00:00Z"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.AddLivingTopicMember(ctx, topic.ID, item.ID); err != nil {
		t.Fatal(err)
	}
	snapshot, err := state.SaveLivingTopicSnapshot(ctx, domain.LivingTopicSnapshot{
		TopicID: topic.ID, Status: "ready", Overview: "Quantum systems have a current source-backed finding.",
		Claims:      []domain.LivingTopicClaim{{Text: "A quantum finding is supported.", Assessment: "supported", EvidenceIDs: []string{item.ID}}},
		EvidenceIDs: []string{item.ID}, InputDigest: "digest-current",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.ExecContext(ctx, `UPDATE living_topics SET understanding_status='current',understanding_input_digest=? WHERE id=?`, snapshot.InputDigest, topic.ID); err != nil {
		t.Fatal(err)
	}
	detail, err := state.LivingTopicDetail(ctx, topic.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Snapshots) != 1 || !detail.Snapshots[0].IsCurrent || detail.Snapshots[0].EvidenceAvailability != "available" || detail.Snapshots[0].ActiveEvidenceCount != 1 {
		t.Fatalf("current snapshot=%+v", detail.Snapshots)
	}
	if _, err := state.RemoveLivingTopicMember(ctx, topic.ID, item.ID); err != nil {
		t.Fatal(err)
	}
	detail, err = state.LivingTopicDetail(ctx, topic.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Snapshots[0].IsCurrent || detail.Snapshots[0].EvidenceAvailability != "unavailable" || detail.Snapshots[0].ActiveEvidenceCount != 0 {
		t.Fatalf("historical snapshot=%+v", detail.Snapshots[0])
	}
}
