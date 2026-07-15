package reasoning

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestMain(m *testing.M) {
	if os.Getenv("AKU_FAKE_CODEX_APP_SERVER") == "1" {
		fakeCodexAppServer()
		return
	}
	if os.Getenv("AKU_FAKE_CODEX") == "1" {
		fakeCodex()
		return
	}
	os.Exit(m.Run())
}

func fakeCodex() {
	args := strings.Join(os.Args, " ")
	if strings.Contains(args, "acquisition-plan.schema.json") {
		emit(map[string]any{"decision": "finish", "reason": "enough bounded evidence"})
	} else {
		emit(domain.ReasoningResult{Summary: "fake", Items: []domain.ReasonedItem{{ID: "item-1", WhatChanged: "Changed", WhyItMatters: "Matters", Source: domain.SourceX, SourceURL: "https://x.com/post", SourceURLKind: "native_post", EvidenceKey: "x:000000000000000000000001", EventKey: "event-one", KnowledgeDelta: "new_event", Author: "author", Confidence: .9, EvidenceState: "primary"}}, CandidateAssessments: []domain.CandidateAssessment{{EvidenceKey: "x:000000000000000000000001", TopicTags: []string{"ai"}, TopicFacets: []string{"ai_models"}, ContentType: "release", Novelty: .8, Urgency: .4, Actionability: .6, Materiality: .8, EvidenceStrength: .9, Rationale: "fixture"}}, Limitations: []string{}})
	}
	fmt.Println(`{"type":"turn.completed","usage":{"input_tokens":10,"cached_input_tokens":2,"output_tokens":5,"reasoning_output_tokens":1}}`)
}
func emit(value any) {
	raw, _ := json.Marshal(value)
	event, _ := json.Marshal(map[string]any{"type": "item.completed", "item": map[string]any{"type": "agent_message", "text": string(raw)}})
	fmt.Println(string(event))
}

func TestCodexExecConsumesSDKCompatibleJSONL(t *testing.T) {
	t.Setenv("AKU_FAKE_CODEX", "1")
	root := filepathRoot(t)
	cfg := config.Config{Root: root, Reasoning: config.ReasoningConfig{Executable: os.Args[0], TimeoutMS: 5000, Planning: config.ModelConfig{Model: "fake", Effort: "high"}, Evaluation: config.ModelConfig{Model: "fake", Effort: "high"}}}
	provider, err := NewCodexExec(cfg)
	if err != nil {
		t.Fatal(err)
	}
	run := domain.Run{ID: "run-1", Source: domain.SourceX}
	observation := domain.Observation{Source: domain.SourceX, Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{EvidenceKey: "x:000000000000000000000001", Text: "Changed"}}}}, Coverage: map[string]any{}}
	plan, planTelemetry, err := provider.Plan(context.Background(), run, observation, nil)
	if err != nil || plan.Decision != "finish" || planTelemetry.InputTokens == nil {
		t.Fatalf("plan=%+v telemetry=%+v err=%v", plan, planTelemetry, err)
	}
	result, telemetry, err := provider.Analyze(context.Background(), run, observation, nil)
	if err != nil || len(result.Items) != 1 || telemetry.OutputTokens == nil {
		t.Fatalf("result=%+v telemetry=%+v err=%v", result, telemetry, err)
	}
}

func filepathRoot(t *testing.T) string {
	t.Helper()
	current, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for _, start := range []string{current, filepath.Dir(os.Args[0])} {
		for candidate := start; ; candidate = filepath.Dir(candidate) {
			module, err := os.ReadFile(filepath.Join(candidate, "go.mod"))
			if err == nil && strings.Contains(string(module), "module github.com/abangkis/AkuSidecar") {
				return filepathSlash(candidate)
			}
			parent := filepath.Dir(candidate)
			if parent == candidate {
				break
			}
		}
	}
	t.Fatalf("AkuSidecar go.mod not found from cwd=%s executable=%s", current, os.Args[0])
	return ""
}
func filepathSlash(value string) string { return strings.ReplaceAll(value, "\\", "/") }
