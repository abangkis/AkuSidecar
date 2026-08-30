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
	if err := state.SaveLivingTopicCandidateDecision(ctx, topic, first, domain.LivingTopicRoutingDecision{TopicID: topic.ID, Match: true, Confidence: 0.8, Mode: "llm", Reason: "Candidate receipt to scrub"}); err != nil {
		t.Fatal(err)
	}

	if err := state.RemoveMemory(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	detail, err = state.LivingTopicDetail(ctx, topic.ID)
	if err != nil || len(detail.Members) != 1 || detail.Members[0].ID != second.ID || len(detail.Snapshots) != 1 {
		t.Fatalf("detail=%+v err=%v", detail, err)
	}
	if _, err := state.AddLivingTopicMember(ctx, topic.ID, first.ID); !errors.Is(err, ErrMemoryNotFound) {
		t.Fatalf("removed member err=%v", err)
	}
	var candidateRows int
	if err := state.db.QueryRow(`SELECT COUNT(*) FROM living_topic_candidate_evaluations WHERE memory_item_id=?`, first.ID).Scan(&candidateRows); err != nil || candidateRows != 0 {
		t.Fatalf("removed item retained candidate receipts: count=%d err=%v", candidateRows, err)
	}
}

func TestLivingTopicSnapshotValidation(t *testing.T) {
	state := openTestStore(t)
	_, err := state.SaveLivingTopicSnapshot(context.Background(), domain.LivingTopicSnapshot{TopicID: "missing", Status: "invented", Overview: "Invalid"})
	if err == nil {
		t.Fatal("invalid snapshot must be rejected before persistence")
	}
}

func TestLivingTopicUnderstandingQueueCoalescesWithoutLosingRunningChanges(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	topic, err := state.CreateLivingTopic(ctx, "Astra")
	if err != nil {
		t.Fatal(err)
	}
	queued, err := state.QueueLivingTopicUnderstanding(ctx, topic.ID, "evidence_added")
	if err != nil || !queued {
		t.Fatalf("queued=%v err=%v", queued, err)
	}
	queued, err = state.QueueLivingTopicUnderstanding(ctx, topic.ID, "refresh_now")
	if err != nil || queued {
		t.Fatalf("coalesced queued=%v err=%v", queued, err)
	}
	first, err := state.ClaimLivingTopicUnderstanding(ctx)
	if err != nil || first == nil || first.Trigger != "refresh_now" {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	queued, err = state.QueueLivingTopicUnderstanding(ctx, topic.ID, "evidence_removed")
	if err != nil || !queued {
		t.Fatalf("running follow-up queued=%v err=%v", queued, err)
	}
	if err := state.FinishLivingTopicUnderstanding(ctx, *first, "no_change", "digest-one", "", nil); err != nil {
		t.Fatal(err)
	}
	between, err := state.LivingTopic(ctx, topic.ID)
	if err != nil || between.UnderstandingStatus != "pending" || between.UnderstandingInputDigest != "digest-one" {
		t.Fatalf("between=%+v err=%v", between, err)
	}
	second, err := state.ClaimLivingTopicUnderstanding(ctx)
	if err != nil || second == nil || second.Trigger != "evidence_removed" {
		t.Fatalf("second=%+v err=%v", second, err)
	}
	if err := state.FinishLivingTopicUnderstanding(ctx, *second, "insufficient_evidence", "digest-two", "", nil); err != nil {
		t.Fatal(err)
	}
	finished, err := state.LivingTopic(ctx, topic.ID)
	if err != nil || finished.UnderstandingStatus != "insufficient_evidence" || finished.UnderstandingCheckedAt == "" || finished.UnderstandingInputDigest != "digest-two" {
		t.Fatalf("finished=%+v err=%v", finished, err)
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
	if added, err := state.AddAutomaticLivingTopicMember(ctx, topic.ID, item, decision); err != nil || !added {
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

func TestLivingTopicActivationCandidatesSupportAcceptRejectAndUndo(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	item, err := state.CreateMemoryRecallStub(ctx, libraryInput("topic-candidate", domain.SourceX, "Codex usage reset announced", "Tibo shared reset timing", "2026-08-30T00:00:00Z"))
	if err != nil {
		t.Fatal(err)
	}
	topic, err := state.CreateLivingTopicWithRoutingCriteria(ctx, domain.LivingTopicCriteriaInput{Name: "Codex Reset", Description: "Track Codex reset announcements", Aliases: []string{"Codex reset"}, IncludeCriteria: "Reset dates and quota announcements", ExcludeCriteria: "Generic Codex coding tips"})
	if err != nil || topic.CriteriaRevision != 1 || len(topic.Aliases) != 1 {
		t.Fatalf("topic=%+v err=%v", topic, err)
	}
	if err := state.SaveLivingTopicCandidateDecision(ctx, topic, item, domain.LivingTopicRoutingDecision{TopicID: topic.ID, Match: true, Confidence: 0.86, Mode: "llm", Reason: "Central claim is a Codex reset announcement"}); err != nil {
		t.Fatal(err)
	}
	detail, err := state.LivingTopicDetail(ctx, topic.ID)
	if err != nil || len(detail.Candidates) != 1 || detail.Candidates[0].Status != "suggested" {
		t.Fatalf("suggestions=%+v err=%v", detail.Candidates, err)
	}
	detail, err = state.ReviewLivingTopicCandidate(ctx, topic.ID, item.ID, "accept")
	if err != nil || len(detail.Members) != 1 || detail.Candidates[0].Status != "accepted" || detail.Memberships[0].MatchMode != "candidate_accept" {
		t.Fatalf("accepted detail=%+v err=%v", detail, err)
	}
	detail, err = state.ReviewLivingTopicCandidate(ctx, topic.ID, item.ID, "undo")
	if err != nil || len(detail.Members) != 0 || detail.Candidates[0].Status != "suggested" {
		t.Fatalf("undo detail=%+v err=%v", detail, err)
	}
	detail, err = state.ReviewLivingTopicCandidate(ctx, topic.ID, item.ID, "reject")
	if err != nil || detail.Candidates[0].Status != "rejected" {
		t.Fatalf("rejected detail=%+v err=%v", detail, err)
	}
	updated, changed, err := state.UpdateLivingTopicRoutingCriteria(ctx, topic.ID, domain.LivingTopicCriteriaInput{Name: topic.Name, Description: topic.Description, Aliases: topic.Aliases, IncludeCriteria: topic.IncludeCriteria, ExcludeCriteria: "Exclude all reset rumors without a date"})
	if err != nil || !changed || updated.CriteriaRevision != 2 || updated.RoutingStatus != "pending" {
		t.Fatalf("updated=%+v changed=%v err=%v", updated, changed, err)
	}
	detail, err = state.LivingTopicDetail(ctx, topic.ID)
	if err != nil || len(detail.Candidates) != 0 {
		t.Fatalf("new revision candidates=%+v err=%v", detail.Candidates, err)
	}
}

func TestCandidateReviewPreservesIndependentManualMembership(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	item, err := state.CreateMemoryRecallStub(ctx, libraryInput("topic-candidate-manual", domain.SourceX, "Codex reset window", "A confirmed quota reset window", "2026-08-30T00:00:00Z"))
	if err != nil {
		t.Fatal(err)
	}
	topic, err := state.CreateLivingTopic(ctx, "Codex Reset")
	if err != nil {
		t.Fatal(err)
	}
	if err := state.SaveLivingTopicCandidateDecision(ctx, topic, item, domain.LivingTopicRoutingDecision{TopicID: topic.ID, Match: true, Confidence: 0.9, Mode: "llm", Reason: "Reset window matches"}); err != nil {
		t.Fatal(err)
	}
	if _, err := state.AddLivingTopicMember(ctx, topic.ID, item.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := state.ReviewLivingTopicCandidate(ctx, topic.ID, item.ID, "accept"); err != nil {
		t.Fatal(err)
	}
	detail, err := state.ReviewLivingTopicCandidate(ctx, topic.ID, item.ID, "reject")
	if err != nil || len(detail.Members) != 1 || len(detail.Memberships) != 1 || detail.Memberships[0].MatchMode != "manual" {
		t.Fatalf("manual membership was not preserved: detail=%+v err=%v", detail, err)
	}
	if _, err := state.ReviewLivingTopicCandidate(ctx, topic.ID, item.ID, "undo"); err != nil {
		t.Fatal(err)
	}
	examples, err := state.LivingTopicFeedbackExamples(ctx, 3)
	if err != nil || len(examples) != 0 {
		t.Fatalf("clear feedback must neutralize the pair: examples=%+v err=%v", examples, err)
	}
	if _, err := state.ForgetMemory(ctx, item.ID); err != nil {
		t.Fatal(err)
	}
	var candidateRows int
	if err := state.db.QueryRow(`SELECT COUNT(*) FROM living_topic_candidate_evaluations WHERE memory_item_id=?`, item.ID).Scan(&candidateRows); err != nil || candidateRows != 0 {
		t.Fatalf("forgotten item retained candidate receipts: count=%d err=%v", candidateRows, err)
	}
}
