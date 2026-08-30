package engine

import (
	"context"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

type fakeLivingTopicResolver struct{ calls int }

func (f *fakeLivingTopicResolver) Name() string { return "topic-fixture" }
func (f *fakeLivingTopicResolver) ModelForProfile(string) config.ModelConfig {
	return config.ModelConfig{Model: "topic-model", Effort: "high"}
}
func (f *fakeLivingTopicResolver) ResolveWithProfile(_ context.Context, _ domain.LivingTopic, evidence []domain.MemoryItem, previous *domain.LivingTopicSnapshot, _ string) (domain.LivingTopicSnapshotResult, domain.ModelUsage, time.Duration, error) {
	f.calls++
	input := int64(len(evidence) * 10)
	deltas := []domain.LivingTopicDelta{}
	if previous == nil {
		deltas = append(deltas, domain.LivingTopicDelta{Kind: "new", Text: "A source was attached.", EvidenceIDs: []string{evidence[0].ID}})
	} else {
		deltas = append(deltas, domain.LivingTopicDelta{Kind: "updated", Text: "The evidence changed.", EvidenceIDs: []string{evidence[0].ID}})
	}
	return domain.LivingTopicSnapshotResult{Status: "ready", Overview: "Evidence-backed understanding.", Claims: []domain.LivingTopicClaim{{Text: "A preview exists.", Assessment: "supported", EvidenceIDs: []string{evidence[0].ID}}}, Deltas: deltas}, domain.ModelUsage{Input: &input}, 9 * time.Millisecond, nil
}

func livingTopicMemoryInput(title string) domain.MemoryItemInput {
	publishedAt := "2026-08-25T00:00:00Z"
	return domain.MemoryItemInput{Identity: domain.MemoryIdentity{Source: domain.SourceX, CanonicalEvidenceKey: "x:living-topic-engine", CanonicalPermalink: "https://x.com/example/status/1", CanonicalPlatformID: "1", ContentFingerprint: "living-topic-engine-fingerprint"}, Title: title, Summary: "A source-backed summary", PublishedAt: &publishedAt}
}

func TestLivingTopicSnapshotsAreExplicitBoundedAndNoChangeAware(t *testing.T) {
	ctx := context.Background()
	runtime, state := testEngine(t)
	resolver := &fakeLivingTopicResolver{}
	runtime.SetLivingTopicsResolver(resolver)
	topic, err := runtime.CreateLivingTopic(ctx, "GPT Astra")
	if err != nil {
		t.Fatal(err)
	}

	empty, err := runtime.CreateLivingTopicSnapshot(ctx, topic.ID)
	if err != nil || empty.Status != "insufficient_evidence" || resolver.calls != 0 {
		t.Fatalf("empty=%+v calls=%d err=%v", empty, resolver.calls, err)
	}
	item, err := state.CreateMemoryRecallStub(ctx, livingTopicMemoryInput("Preview one"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.AddLivingTopicMember(ctx, topic.ID, item.ID); err != nil {
		t.Fatal(err)
	}

	first, err := runtime.CreateLivingTopicSnapshot(ctx, topic.ID)
	if err != nil || first.Status != "ready" || first.Version != 2 || resolver.calls != 1 {
		t.Fatalf("first=%+v calls=%d err=%v", first, resolver.calls, err)
	}
	second, err := runtime.CreateLivingTopicSnapshot(ctx, topic.ID)
	if err != nil || second.Status != "no_change" || second.Version != 3 || resolver.calls != 1 || len(second.Claims) != 1 {
		t.Fatalf("second=%+v calls=%d err=%v", second, resolver.calls, err)
	}

	if _, err := state.UpsertMemoryRecallStub(ctx, livingTopicMemoryInput("Preview two")); err != nil {
		t.Fatal(err)
	}
	third, err := runtime.CreateLivingTopicSnapshot(ctx, topic.ID)
	if err != nil || third.Status != "ready" || third.Version != 4 || resolver.calls != 2 || len(third.Deltas) != 1 || third.Deltas[0].Kind != "updated" {
		t.Fatalf("third=%+v calls=%d err=%v", third, resolver.calls, err)
	}
}
