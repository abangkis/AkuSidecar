package engine

import (
	"context"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/store"
)

type fakeLivingTopicResolver struct{ calls, routeCalls int }

func (f *fakeLivingTopicResolver) Name() string { return "topic-fixture" }
func (f *fakeLivingTopicResolver) ModelForProfile(string) config.ModelConfig {
	return config.ModelConfig{Model: "topic-model", Effort: "high"}
}

func (f *fakeLivingTopicResolver) RouteWithProfile(_ context.Context, _ domain.TimelineItem, topics []domain.LivingTopic, _ []domain.LivingTopicRoutingExample, _ string) ([]domain.LivingTopicRoutingDecision, domain.ModelUsage, time.Duration, error) {
	f.routeCalls++
	decisions := make([]domain.LivingTopicRoutingDecision, 0, len(topics))
	for _, topic := range topics {
		decisions = append(decisions, domain.LivingTopicRoutingDecision{TopicID: topic.ID, Match: true, Confidence: 0.9, Mode: "llm", Reason: "Semantic criteria match"})
	}
	return decisions, domain.ModelUsage{}, time.Millisecond, nil
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

func TestDeterministicLivingTopicScoreLearnsFromNegativeExamples(t *testing.T) {
	topic := domain.LivingTopic{ID: "topic", Name: "OpenAI GPT Astra", Description: "Track Astra capabilities, releases, and testing."}
	item := domain.TimelineItem{Item: domain.ReasonedItem{WhatChanged: "OpenAI expands GPT Astra testing", WhyItMatters: "Astra may ship new agent capabilities"}, Assessment: domain.CandidateAssessment{TopicTags: []string{"OpenAI", "Astra"}}}
	base, _ := deterministicLivingTopicScore(item, topic, nil)
	examples := []domain.LivingTopicRoutingExample{{TopicID: topic.ID, Verdict: "exclude", Item: domain.MemoryItem{Title: "OpenAI expands GPT Astra testing", Summary: "Astra may ship new agent capabilities", Tags: []string{"OpenAI", "Astra"}}}}
	corrected, _ := deterministicLivingTopicScore(item, topic, examples)
	if base < 0.70 {
		t.Fatalf("expected obvious criteria match, score=%f", base)
	}
	if corrected >= base || corrected >= 0.70 {
		t.Fatalf("negative correction did not lower automatic confidence: base=%f corrected=%f", base, corrected)
	}
}

func TestLivingTopicSemanticRouterProjectsFinalTimelineItemAsRecallOnly(t *testing.T) {
	ctx := context.Background()
	runtime, state := testEngine(t)
	resolver := &fakeLivingTopicResolver{}
	runtime.SetLivingTopicsResolver(resolver)
	topic, err := runtime.CreateLivingTopicWithCriteria(ctx, "Agent runtimes", "Track new autonomous runtime capabilities")
	if err != nil {
		t.Fatal(err)
	}
	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.ActiveSources = []domain.Source{domain.SourceX}
	if err := state.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	session, err := state.CreateUpdateSession(ctx, "topic routing", settings, domain.UpdatePolicy{Trigger: domain.UpdateTriggerUser, Delivery: domain.UpdateDeliveryVisible, BudgetAuthority: domain.BudgetAuthorityUser})
	if err != nil {
		t.Fatal(err)
	}
	run := session.Runs[0]
	evidence := "x:semantic-topic-route"
	reasoned := domain.ReasonedItem{Source: domain.SourceX, EvidenceKey: evidence, WhatChanged: "A new system coordinates long-running agents", WhyItMatters: "It changes autonomous task execution", SourceURL: "https://x.com/example/status/992"}
	assessment := domain.CandidateAssessment{EvidenceKey: evidence, TopicTags: []string{"agents"}}
	item := domain.TimelineItem{ID: "timeline-semantic-topic-route", SessionID: session.ID, RunID: run.ID, Source: domain.SourceX, EvidenceKey: evidence, Item: reasoned, Assessment: assessment}
	if err := state.CompleteRun(ctx, run, domain.ReasoningResult{Items: []domain.ReasonedItem{reasoned}, CandidateAssessments: []domain.CandidateAssessment{assessment}}, []store.ScoredAssessment{{Assessment: assessment, Selected: true}}, []domain.TimelineItem{item}, map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if err := state.ComposeSession(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
	if err := state.FinalizeSession(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
	decisions, err := runtime.routeLivingTopicItem(ctx, item.ID)
	if err != nil || len(decisions) != 1 || resolver.routeCalls != 1 {
		t.Fatalf("decisions=%+v calls=%d err=%v", decisions, resolver.routeCalls, err)
	}
	detail, err := state.LivingTopicDetail(ctx, topic.ID)
	if err != nil || len(detail.Members) != 1 || detail.Members[0].RetentionTier != domain.MemoryTierRecall || detail.Members[0].Saved || detail.Members[0].PermanentKeep {
		t.Fatalf("detail=%+v err=%v", detail, err)
	}
	if detail.Memberships[0].Origin != "automatic" || detail.Memberships[0].MatchMode != "llm" {
		t.Fatalf("membership=%+v", detail.Memberships[0])
	}
}
