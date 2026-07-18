package eventengine

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
}

func (f *fakeStructuredInvoker) InvokeStructured(_ context.Context, prompt string, _ any, _ config.ModelConfig) (string, domain.ModelUsage, time.Duration, error) {
	f.prompt = prompt
	return `{"decisions":[{"candidateAlias":"candidate_001","relation":"new_event","targetAlias":null,"confidence":0.95,"reason":"Distinct occurrence","event":{"canonicalClaim":"A release happened","actor":"OpenAI","action":"released","object":"Codex","eventKind":"release","aliases":[]}}]}`, domain.ModelUsage{}, time.Millisecond, nil
}

func TestStructuredResolverUsesOpaqueAliasesAndNoTools(t *testing.T) {
	invoker := &fakeStructuredInvoker{}
	resolver := &StructuredResolver{invoker: invoker, model: config.ModelConfig{Model: "test", Effort: "high"}, schema: map[string]any{}}
	candidates := []domain.SemanticCandidate{{Alias: "candidate_001", EvidenceKey: "secret-evidence-key", Text: "OpenAI released Codex"}}
	events := []domain.SemanticEvent{{ID: "secret-event-id", CanonicalClaim: "A prior event"}}
	result, _, _, err := resolver.Resolve(context.Background(), candidates, events)
	if err != nil || len(result.Decisions) != 1 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if strings.Contains(invoker.prompt, "secret-evidence-key") || strings.Contains(invoker.prompt, "secret-event-id") {
		t.Fatalf("stable identity leaked into prompt: %s", invoker.prompt)
	}
	for _, required := range []string{"event_001", "candidate_001", "Do not browse", "duplicate_report"} {
		if !strings.Contains(invoker.prompt, required) {
			t.Fatalf("prompt missing %q", required)
		}
	}
}

func TestStructuredResolverBoundsUntrustedEvidenceExcerpt(t *testing.T) {
	invoker := &fakeStructuredInvoker{}
	resolver := &StructuredResolver{invoker: invoker, model: config.ModelConfig{Model: "test", Effort: "high"}, schema: map[string]any{}}
	longText := strings.Repeat("bounded source evidence ", 80) + "TAIL_SENTINEL"
	_, _, _, err := resolver.Resolve(context.Background(), []domain.SemanticCandidate{{Alias: "candidate_001", Text: longText, WhatChanged: "A bounded event occurred"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(invoker.prompt, "TAIL_SENTINEL") || !strings.Contains(invoker.prompt, "evidenceExcerpt") {
		t.Fatalf("evidence excerpt was not bounded: %s", invoker.prompt)
	}
}
