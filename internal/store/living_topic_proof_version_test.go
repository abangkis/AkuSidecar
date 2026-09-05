package store

import (
	"context"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestLivingTopicProofContractDoesNotPromoteOldSnapshots(t *testing.T) {
	topic := domain.LivingTopic{UnderstandingStatus: "current", UnderstandingInputDigest: "same-input"}
	for _, version := range []string{"current-projection-v4", "current-projection-v5"} {
		snapshots := annotateLivingTopicSnapshots(topic, []domain.LivingTopicSnapshot{{
			ContractVersion: version, InputDigest: "same-input", EvidenceIDs: []string{"evidence"},
		}}, map[string]struct{}{"evidence": {}})
		if snapshots[0].IsCurrent != (version == "current-projection-v5") {
			t.Fatalf("contract %s current=%v", version, snapshots[0].IsCurrent)
		}
		if snapshots[0].ActiveEvidenceCount != 1 || snapshots[0].EvidenceAvailability != "available" {
			t.Fatalf("contract change must preserve historical evidence availability: %+v", snapshots[0])
		}
	}
}

func TestLivingTopicOldProofContractNeedsRefreshWithoutRewritingHistory(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	item, err := state.CreateMemoryRecallStub(ctx, libraryInput("proof-version", domain.SourceX, "Release", "A release was announced", "2026-09-01T00:00:00Z"))
	if err != nil {
		t.Fatal(err)
	}
	topic, err := state.CreateLivingTopic(ctx, "Release")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.AddLivingTopicMember(ctx, topic.ID, item.ID); err != nil {
		t.Fatal(err)
	}
	old, err := state.SaveLivingTopicSnapshot(ctx, domain.LivingTopicSnapshot{
		TopicID: topic.ID, Status: "ready", ContractVersion: "current-projection-v4", InputDigest: "old-digest",
		Overview: "Prior projection", EvidenceIDs: []string{item.ID},
		Claims: []domain.LivingTopicClaim{{Text: "Prior projection", Assessment: "supported", EvidenceIDs: []string{item.ID}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.ExecContext(ctx, "UPDATE living_topics SET understanding_status='current', understanding_input_digest=? WHERE id=?", old.InputDigest, topic.ID); err != nil {
		t.Fatal(err)
	}
	detail, err := state.LivingTopicDetail(ctx, topic.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Topic.UnderstandingStatus != "needs_refresh" || detail.Topic.LatestSnapshot != nil || len(detail.Snapshots) != 1 || detail.Snapshots[0].IsCurrent || detail.Snapshots[0].Overview != old.Overview {
		t.Fatalf("old projection must remain unchanged history: %+v", detail)
	}
	var persistedStatus string
	if err := state.db.QueryRowContext(ctx, "SELECT understanding_status FROM living_topics WHERE id=?", topic.ID).Scan(&persistedStatus); err != nil {
		t.Fatal(err)
	}
	if persistedStatus != "current" {
		t.Fatalf("readback mutated status: %s", persistedStatus)
	}
}
