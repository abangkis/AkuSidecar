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
	model  config.ModelConfig
	schema any
}

func semanticResolverTestSchema() map[string]any {
	return map[string]any{"properties": map[string]any{"decisions": map[string]any{"type": "array", "maxItems": 20}}}
}

func (f *fakeStructuredInvoker) InvokeStructured(_ context.Context, _ string, prompt string, schema any, model config.ModelConfig) (string, domain.ModelUsage, time.Duration, error) {
	f.prompt = prompt
	f.model = model
	f.schema = schema
	return `{"decisions":[{"candidateAlias":"candidate_001","relation":"new_event","targetAlias":null,"confidence":0.95,"reason":"Distinct occurrence","event":{"canonicalClaim":"A release happened","actor":"OpenAI","action":"released","object":"Codex","eventKind":"release","aliases":[]}}]}`, domain.ModelUsage{}, time.Millisecond, nil
}

func TestStructuredResolverRequiresExactDecisionCount(t *testing.T) {
	invoker := &fakeStructuredInvoker{}
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"decisions": map[string]any{"type": "array", "maxItems": 20},
		},
	}
	resolver := &StructuredResolver{invoker: invoker, model: config.ModelConfig{Model: "test", Effort: "high"}, schema: schema}
	candidates := []domain.SemanticCandidate{{Alias: "candidate_001"}, {Alias: "candidate_002"}, {Alias: "candidate_003"}}
	if _, _, _, err := resolver.Resolve(context.Background(), candidates, nil); err != nil {
		t.Fatal(err)
	}
	projected := invoker.schema.(map[string]any)
	decisions := projected["properties"].(map[string]any)["decisions"].(map[string]any)
	if decisions["minItems"] != 3 || decisions["maxItems"] != 3 {
		t.Fatalf("decisions schema=%+v", decisions)
	}
	original := schema["properties"].(map[string]any)["decisions"].(map[string]any)
	if _, exists := original["minItems"]; exists || original["maxItems"] != 20 {
		t.Fatalf("base schema mutated: %+v", original)
	}
}

func TestExactSemanticDecisionCountSchemaRejectsInvalidCounts(t *testing.T) {
	schema := map[string]any{"properties": map[string]any{"decisions": map[string]any{"type": "array", "maxItems": 20}}}
	for _, count := range []int{0, 21} {
		if _, err := exactSemanticDecisionCountSchema(schema, count); err == nil {
			t.Fatalf("count=%d should fail", count)
		}
	}
}

func (f *fakeStructuredInvoker) ResolveProfile(id string) (config.ModelConfig, bool) {
	if id == "terra_xhigh" {
		return config.ModelConfig{Model: "gpt-5.6-terra", Effort: "xhigh"}, true
	}
	return config.ModelConfig{}, false
}

func TestStructuredResolverUsesSelectedBackendProfile(t *testing.T) {
	invoker := &fakeStructuredInvoker{}
	resolver := &StructuredResolver{invoker: invoker, model: config.ModelConfig{Model: "fallback", Effort: "high"}, schema: semanticResolverTestSchema()}
	_, _, _, err := resolver.ResolveWithProfile(context.Background(), []domain.SemanticCandidate{{Alias: "candidate_001", Text: "OpenAI released Codex"}}, nil, "terra_xhigh")
	if err != nil || invoker.model.Model != "gpt-5.6-terra" || invoker.model.Effort != "xhigh" {
		t.Fatalf("model=%+v err=%v", invoker.model, err)
	}
}

func TestStructuredResolverUsesOpaqueAliasesAndNoTools(t *testing.T) {
	invoker := &fakeStructuredInvoker{}
	resolver := &StructuredResolver{invoker: invoker, model: config.ModelConfig{Model: "test", Effort: "high"}, schema: semanticResolverTestSchema()}
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
	resolver := &StructuredResolver{invoker: invoker, model: config.ModelConfig{Model: "test", Effort: "high"}, schema: semanticResolverTestSchema()}
	longText := strings.Repeat("bounded source evidence ", 80) + "TAIL_SENTINEL"
	_, _, _, err := resolver.Resolve(context.Background(), []domain.SemanticCandidate{{Alias: "candidate_001", Text: longText, WhatChanged: "A bounded event occurred"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(invoker.prompt, "TAIL_SENTINEL") || !strings.Contains(invoker.prompt, "evidenceExcerpt") {
		t.Fatalf("evidence excerpt was not bounded: %s", invoker.prompt)
	}
}

func TestStructuredResolverIncludesBoundedKnownDeltasAndSequentialNoveltyContract(t *testing.T) {
	invoker := &fakeStructuredInvoker{}
	resolver := &StructuredResolver{invoker: invoker, model: config.ModelConfig{Model: "test", Effort: "high"}, schema: semanticResolverTestSchema()}
	events := []domain.SemanticEvent{{
		ID:             "event-secret",
		CanonicalClaim: "OpenAI changed Luna pricing",
		KnownDeltas: []domain.SemanticEventDelta{
			{ID: "delta-secret-1", Claim: "Luna is twenty percent cheaper", Kind: "material_update", Source: domain.SourceX, Confidence: .99},
			{ID: "delta-secret-2", Claim: "The change applies to paid usage", Kind: "material_update", Source: domain.SourceLinkedIn, Confidence: .95},
			{ID: "delta-secret-3", Claim: "The new pricing starts today", Kind: "material_update", Source: domain.SourceX, Confidence: .94},
			{ID: "delta-secret-4", Claim: "This fourth delta must not enter the prompt", Kind: "material_update", Source: domain.SourceX, Confidence: .93},
		},
	}}
	candidates := []domain.SemanticCandidate{
		{Alias: "candidate_001", Text: "Luna is twenty percent cheaper"},
		{Alias: "candidate_002", Text: "Another author says Luna is twenty percent cheaper"},
	}
	if _, _, _, err := resolver.Resolve(context.Background(), candidates, events); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"knownDeltas", "twenty percent cheaper", "Process candidates sequentially", "merely repeats that fact is duplicate_report", "Unverified motive"} {
		if !strings.Contains(invoker.prompt, required) {
			t.Fatalf("prompt missing %q: %s", required, invoker.prompt)
		}
	}
	for _, forbidden := range []string{"event-secret", "delta-secret", "fourth delta"} {
		if strings.Contains(invoker.prompt, forbidden) {
			t.Fatalf("bounded semantic identity or delta leaked: %q", forbidden)
		}
	}
}
