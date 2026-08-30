package store

import (
	"context"
	"errors"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestLivingTopicManualMembershipAndAppendOnlySnapshots(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	first, err := state.CreateMemoryRecallStub(ctx, libraryInput("topic-first", domain.SourceX, "Astra preview", "A first source", "2026-08-20T00:00:00Z"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := state.CreateMemoryRecallStub(ctx, libraryInput("topic-second", domain.SourceLinkedIn, "Astra follow-up", "A second source", "2026-08-21T00:00:00Z"))
	if err != nil {
		t.Fatal(err)
	}
	topic, err := state.CreateLivingTopic(ctx, "  GPT Astra  ")
	if err != nil || topic.Name != "GPT Astra" {
		t.Fatalf("topic=%+v err=%v", topic, err)
	}
	if _, err := state.AddLivingTopicMember(ctx, topic.ID, first.ID); err != nil {
		t.Fatal(err)
	}
	detail, err := state.AddLivingTopicMember(ctx, topic.ID, first.ID)
	if err != nil || len(detail.Members) != 1 {
		t.Fatalf("idempotent members=%d err=%v", len(detail.Members), err)
	}
	detail, err = state.AddLivingTopicMember(ctx, topic.ID, second.ID)
	if err != nil || len(detail.Members) != 2 {
		t.Fatalf("members=%d err=%v", len(detail.Members), err)
	}

	ready, err := state.SaveLivingTopicSnapshot(ctx, domain.LivingTopicSnapshot{TopicID: topic.ID, Status: "ready", Overview: "Two sources support a preview.", Claims: []domain.LivingTopicClaim{{Text: "A preview exists.", Assessment: "supported", EvidenceIDs: []string{first.ID}}}, EvidenceIDs: []string{first.ID, second.ID}, Provider: "fixture", Model: "fixture", Effort: "none", InputDigest: "digest-one"})
	if err != nil || ready.Version != 1 {
		t.Fatalf("snapshot=%+v err=%v", ready, err)
	}
	noChange, err := state.SaveLivingTopicSnapshot(ctx, domain.LivingTopicSnapshot{TopicID: topic.ID, Status: "no_change", Overview: "No evidence changed.", Claims: ready.Claims, EvidenceIDs: ready.EvidenceIDs, Provider: "local-deterministic", Model: "none", Effort: "none", InputDigest: "digest-one", PreviousSnapshotID: ready.ID})
	if err != nil || noChange.Version != 2 || noChange.PreviousSnapshotID != ready.ID {
		t.Fatalf("snapshot=%+v err=%v", noChange, err)
	}

	if err := state.RemoveMemory(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	detail, err = state.LivingTopicDetail(ctx, topic.ID)
	if err != nil || len(detail.Members) != 1 || detail.Members[0].ID != second.ID || len(detail.Snapshots) != 2 {
		t.Fatalf("detail=%+v err=%v", detail, err)
	}
	if _, err := state.AddLivingTopicMember(ctx, topic.ID, first.ID); !errors.Is(err, ErrMemoryNotFound) {
		t.Fatalf("removed member err=%v", err)
	}
}

func TestLivingTopicSnapshotValidation(t *testing.T) {
	state := openTestStore(t)
	_, err := state.SaveLivingTopicSnapshot(context.Background(), domain.LivingTopicSnapshot{TopicID: "missing", Status: "invented", Overview: "Invalid"})
	if err == nil {
		t.Fatal("invalid snapshot must be rejected before persistence")
	}
}
