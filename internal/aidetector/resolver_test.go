package aidetector

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

type fakeStructuredInvoker struct {
	prompt string
	raw    string
}

func (f *fakeStructuredInvoker) InvokeStructured(_ context.Context, prompt string, _ any, _ config.ModelConfig) (string, domain.ModelUsage, time.Duration, error) {
	f.prompt = prompt
	input, output := int64(100), int64(20)
	return f.raw, domain.ModelUsage{Input: &input, Output: &output}, 15 * time.Millisecond, nil
}

func TestDeepResolverUsesBoundedUntrustedEvidenceAndNoTools(t *testing.T) {
	invoker := &fakeStructuredInvoker{raw: `{"assessments":[{"status":"insufficient_evidence","confidenceBand":"low","evidenceCodes":["insufficient_content"],"rationale":"The available authored content is inadequate."}]}`}
	resolver := &AppServerResolver{invoker: invoker, model: config.ModelConfig{Model: "test", Effort: "high"}, schema: map[string]any{}}
	item := domain.TimelineItem{
		ID: "private-timeline-id", SessionID: "private-session-id", Source: domain.SourceX,
		Item: domain.ReasonedItem{Author: "Author", WhatChanged: "Fallback text"},
		Evidence: &domain.Block{
			Text: strings.Repeat("bounded evidence ", 300) + "TAIL_SENTINEL",
			QuotedPost: map[string]any{
				"author": "Quoted author", "text": "Bounded quoted evidence", "permalink": "private-quoted-permalink", "platformId": "private-platform-id",
			},
		},
		AIDetection:   &domain.TimelineAIDetection{AssessmentID: "private-assessment-id", CorrectionID: "private-ai-correction-id", Stage: "fast", Status: "strong_signals", ConfidenceBand: "medium"},
		SemanticEvent: &domain.TimelineSemanticEvent{EventID: "private-event-id", CorrectionID: "private-event-correction-id", CanonicalClaim: "A bounded prior event", Relation: "new_event"},
	}
	result, usage, _, err := resolver.Resolve(context.Background(), []domain.TimelineItem{item})
	if err != nil || len(result.Assessments) != 1 || usage.Input == nil || *usage.Input != 100 {
		t.Fatalf("result=%+v usage=%+v err=%v", result, usage, err)
	}
	for _, forbidden := range []string{"private-timeline-id", "private-session-id", "TAIL_SENTINEL", "private-quoted-permalink", "private-platform-id", "private-assessment-id", "private-ai-correction-id", "private-event-id", "private-event-correction-id"} {
		if strings.Contains(invoker.prompt, forbidden) {
			t.Fatalf("prompt leaked %q", forbidden)
		}
	}
	if strings.Contains(invoker.prompt, "A bounded prior event") {
		t.Fatal("AI Detector does not need the semantic canonical claim in its prompt")
	}
	if len(invoker.prompt) > 10000 {
		t.Fatalf("bounded AI Detector prompt unexpectedly grew to %d bytes", len(invoker.prompt))
	}
	for _, required := range []string{"untrusted social-media evidence", "Do not browse", "AI origin signals", "post_001"} {
		if !strings.Contains(invoker.prompt, required) {
			t.Fatalf("prompt missing %q", required)
		}
	}
}

func TestDeepCandidatesSpendModelEffortOnlyWhereReviewCanHelp(t *testing.T) {
	items := []domain.TimelineItem{
		{ID: "ordinary", AIDetection: &domain.TimelineAIDetection{Status: "no_signal_detected"}},
		{ID: "preliminary", AIDetection: &domain.TimelineAIDetection{Status: "strong_signals", EvidenceCodes: []string{"author_declared_ai"}}},
		{ID: "short", AIDetection: &domain.TimelineAIDetection{Status: "insufficient_evidence"}},
		{ID: "platform", AIDetection: &domain.TimelineAIDetection{Status: "strong_signals", EvidenceCodes: []string{"platform_ai_label"}}},
		{ID: "corrected", AIDetection: &domain.TimelineAIDetection{Status: "user_marked_not_ai", UserOverride: true}},
	}
	result := DeepCandidates(items)
	if len(result) != 2 || result[0].ID != "ordinary" || result[1].ID != "preliminary" {
		t.Fatalf("deep candidates=%+v", result)
	}
}

func TestDeepResolverRejectsIncompleteAssessmentBatch(t *testing.T) {
	invoker := &fakeStructuredInvoker{raw: `{"assessments":[]}`}
	resolver := &AppServerResolver{invoker: invoker, model: config.ModelConfig{Model: "test"}, schema: map[string]any{}}
	_, _, _, err := resolver.Resolve(context.Background(), []domain.TimelineItem{{ID: "timeline", SessionID: "session"}})
	if err == nil || !strings.Contains(err.Error(), "returned 0 assessments for 1 candidates") {
		t.Fatalf("err=%v", err)
	}
}

func TestDeepResolverRejectsUserAuthorityStatus(t *testing.T) {
	invoker := &fakeStructuredInvoker{raw: `{"assessments":[{"status":"user_marked_ai","confidenceBand":"high","evidenceCodes":[],"rationale":"invalid authority"}]}`}
	resolver := &AppServerResolver{invoker: invoker, model: config.ModelConfig{Model: "test"}, schema: map[string]any{}}
	_, _, _, err := resolver.Resolve(context.Background(), []domain.TimelineItem{{ID: "timeline", SessionID: "session"}})
	if err == nil || !strings.Contains(err.Error(), "authority do not match") {
		t.Fatalf("err=%v", err)
	}
}
