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

func TestLivingTopicCriteriaAndMembershipFeedbackAreAuditable(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	topic, err := state.CreateLivingTopicWithCriteria(ctx, "Astra", "Track Astra capabilities; exclude generic AI news.")
	if err != nil || topic.Description == "" {
		t.Fatalf("topic=%+v err=%v", topic, err)
	}
	item, err := state.CreateMemoryRecallStub(ctx, libraryInput("topic-feedback", domain.SourceX, "Astra testing", "New agent capability", "2026-08-25T00:00:00Z"))
	if err != nil {
		t.Fatal(err)
	}
	detail, err := state.AddLivingTopicMember(ctx, topic.ID, item.ID)
	if err != nil || len(detail.Memberships) != 1 || detail.Memberships[0].Origin != "manual" {
		t.Fatalf("detail=%+v err=%v", detail, err)
	}
	if _, err := state.RemoveLivingTopicMember(ctx, topic.ID, item.ID); err != nil {
		t.Fatal(err)
	}
	examples, err := state.LivingTopicFeedbackExamples(ctx, 3)
	if err != nil || len(examples) != 1 || examples[0].Verdict != "exclude" {
		t.Fatalf("examples=%+v err=%v", examples, err)
	}
}

func TestAutomaticLivingTopicMembershipCreatesOnlyRecallEvidence(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	topic, err := state.CreateLivingTopicWithCriteria(ctx, "Astra", "Track Astra releases")
	if err != nil {
		t.Fatal(err)
	}
	item := domain.TimelineItem{ID: "timeline-route", SessionID: "session-route", RunID: "run-route", Source: domain.SourceX, EvidenceKey: "x:route-astra", Item: domain.ReasonedItem{Source: domain.SourceX, EvidenceKey: "x:route-astra", WhatChanged: "GPT Astra preview expanded", WhyItMatters: "New agent capability", SourceURL: "https://x.com/example/status/991", Author: "Example"}, Assessment: domain.CandidateAssessment{TopicTags: []string{"Astra"}}}
	decision := domain.LivingTopicRoutingDecision{TopicID: topic.ID, Match: true, Confidence: 0.88, Mode: "deterministic", Reason: "Criteria matched"}
	if err := state.AddAutomaticLivingTopicMember(ctx, topic.ID, item, decision); err != nil {
		t.Fatal(err)
	}
	detail, err := state.LivingTopicDetail(ctx, topic.ID)
	if err != nil || len(detail.Members) != 1 || len(detail.Memberships) != 1 {
		t.Fatalf("detail=%+v err=%v", detail, err)
	}
	if detail.Members[0].RetentionTier != domain.MemoryTierRecall || detail.Members[0].Saved || detail.Members[0].PermanentKeep {
		t.Fatalf("automatic route changed retention ownership: %+v", detail.Members[0])
	}
	if detail.Memberships[0].Origin != "automatic" || detail.Memberships[0].MatchMode != "deterministic" {
		t.Fatalf("membership=%+v", detail.Memberships[0])
	}
}
