package reasoning

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

func ollamaTestServer(t *testing.T, wantThink any, content string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Model  string         `json:"model"`
			Think  any            `json:"think"`
			Format map[string]any `json:"format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode ollama payload: %v", err)
		}
		if payload.Model != "nemotron" {
			t.Errorf("model = %v, want nemotron", payload.Model)
		}
		if wantThink != nil && payload.Think != wantThink {
			t.Errorf("think = %v, want %v", payload.Think, wantThink)
		}
		if len(payload.Format) == 0 {
			t.Error("structured format missing from payload")
		}
		raw, _ := json.Marshal(content)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"message":{"content":%s},"done":true,"done_reason":"stop","prompt_eval_count":10,"eval_count":5}`, raw)
	}))
	t.Cleanup(server.Close)
	return server
}

func ollamaTestConfig(t *testing.T, server *httptest.Server) config.Config {
	t.Helper()
	return config.Config{
		Root: filepathRoot(t),
		Reasoning: config.ReasoningConfig{
			Provider:   "ollama",
			Endpoint:   server.URL,
			TimeoutMS:  5000,
			Planning:   config.ModelConfig{Model: "nemotron", Effort: "high"},
			Evaluation: config.ModelConfig{Model: "nemotron", Effort: "high"},
		},
	}
}

func TestOllamaProfileCatalog(t *testing.T) {
	server := ollamaTestServer(t, nil, "")
	provider, err := NewOllama(ollamaTestConfig(t, server))
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	options := provider.ProfileOptions()
	if len(options) != 4 {
		t.Fatalf("options=%+v", options)
	}
	for _, id := range []string{"structured_fast", "short_reasoning", "general_synthesis", "deep_reasoning"} {
		if model, ok := provider.ResolveProfile(id); !ok || model.Model != "nemotron" || model.Effort == "" {
			t.Fatalf("profile %q model=%+v ok=%v", id, model, ok)
		}
	}
	if model, ok := provider.ResolveProfile("deep_reasoning"); !ok || model.Effort != "high" {
		t.Fatalf("deep reasoning profile=%+v ok=%v", model, ok)
	}
	if model, ok := provider.ResolveProfile("luna_high"); ok {
		t.Fatalf("legacy profile unexpectedly resolved: %+v", model)
	}
}

func TestOllamaDefaultsEndpointWhenUnset(t *testing.T) {
	provider, err := NewOllama(config.Config{
		Root: filepathRoot(t),
		Reasoning: config.ReasoningConfig{
			Provider: "ollama", TimeoutMS: 5000,
			Planning:   config.ModelConfig{Model: "nemotron", Effort: "high"},
			Evaluation: config.ModelConfig{Model: "nemotron", Effort: "high"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	if provider.endpoint != ollamaDefaultEndpoint {
		t.Fatalf("endpoint=%q", provider.endpoint)
	}
}

func TestOllamaRequiresPlanningModel(t *testing.T) {
	if _, err := NewOllama(config.Config{
		Root: filepathRoot(t),
		Reasoning: config.ReasoningConfig{
			Provider: "ollama", TimeoutMS: 5000,
			Planning:   config.ModelConfig{Model: "", Effort: "high"},
			Evaluation: config.ModelConfig{Model: "nemotron", Effort: "high"},
		},
	}); err == nil || !strings.Contains(err.Error(), "ollama planning model") {
		t.Fatalf("missing model error=%v", err)
	}
}

func TestOllamaStructuredPlan(t *testing.T) {
	content := `{"decision":"finish","reason":"enough bounded evidence"}`
	server := ollamaTestServer(t, "high", content)
	provider, err := NewOllama(ollamaTestConfig(t, server))
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	run, observation := fakeAppServerInput()
	plan, telemetry, err := provider.Plan(context.Background(), run, observation, nil)
	if err != nil || plan.Decision != "finish" {
		t.Fatalf("plan=%+v err=%v", plan, err)
	}
	if telemetry.Provider != "ollama" || telemetry.Model != "nemotron" || telemetry.Effort != "high" || telemetry.Phase != "acquisition_planning" {
		t.Fatalf("telemetry=%+v", telemetry)
	}
	if telemetry.InputTokens == nil || *telemetry.InputTokens != 10 {
		t.Fatalf("input tokens=%v", telemetry.InputTokens)
	}
	if telemetry.ReasoningOutputTokens != nil {
		t.Fatalf("ollama must not report reasoning tokens: %v", *telemetry.ReasoningOutputTokens)
	}
}

func TestOllamaStructuredAnalyzeBindsEvidenceKeys(t *testing.T) {
	raw, _ := json.Marshal(domain.ReasoningResult{
		Summary: "fake ollama",
		Items:   []domain.ReasonedItem{{ID: "item-1", WhatChanged: "Changed", WhyItMatters: "Matters", Source: domain.SourceX, EvidenceKey: "candidate_001", EventKey: "event-one", KnowledgeDelta: "new_event", Author: "author", Confidence: .9, EvidenceState: "primary"}},
		CandidateAssessments: []domain.CandidateAssessment{{EvidenceKey: "candidate_001", TopicTags: []string{"ai"}, TopicFacets: []string{"ai_models"}, ContentType: "release", Novelty: .8, Urgency: .4, Actionability: .6, Materiality: .8, EvidenceStrength: .9, Rationale: "fixture"}},
		Limitations:          []string{},
	})
	server := ollamaTestServer(t, "high", string(raw))
	provider, err := NewOllama(ollamaTestConfig(t, server))
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	run, observation := fakeAppServerInput()
	result, telemetry, err := provider.Analyze(context.Background(), run, observation, nil)
	if err != nil || len(result.Items) != 1 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if result.Items[0].EvidenceKey != "x:000000000000000000000001" || result.CandidateAssessments[0].EvidenceKey != "x:000000000000000000000001" {
		t.Fatalf("candidate aliases were not restored: %+v", result)
	}
	if telemetry.Provider != "ollama" || telemetry.Effort != "high" || telemetry.OutputTokens == nil || *telemetry.OutputTokens != 5 {
		t.Fatalf("telemetry=%+v", telemetry)
	}
}

func TestEnsureResolvableProfileMigratesLegacyValues(t *testing.T) {
	codex := &CodexAppServer{}
	if got := EnsureResolvableProfile(codex, "luna_high"); got != "luna_high" {
		t.Fatalf("resolvable profile changed to %q", got)
	}
	if got := EnsureResolvableProfile(codex, "unknown"); got != "luna_max" {
		t.Fatalf("unknown profile migrated to %q, want luna_max", got)
	}
	if got := EnsureResolvableProfile(Deterministic{}, "anything"); got != "anything" {
		t.Fatalf("non-catalog provider changed profile to %q", got)
	}
}