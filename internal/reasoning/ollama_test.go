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
	"github.com/abangkis/ai4u-inference-sdk-go/inference"
)

func ollamaTestServer(t *testing.T, wantThink any, content string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/show" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"capabilities":["completion","thinking"]}`)
			return
		}
		var payload struct {
			Model  string         `json:"model"`
			Think  any            `json:"think"`
			Format map[string]any `json:"format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode ollama payload: %v", err)
		}
		if payload.Model != "nemotron-3.5-lightning:latest" {
			t.Errorf("model = %v, want nemotron-3.5-lightning:latest", payload.Model)
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
			Planning:   config.ModelConfig{Model: "nemotron-3.5-lightning:latest", Effort: "high"},
			Evaluation: config.ModelConfig{Model: "nemotron-3.5-lightning:latest", Effort: "high"},
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
		if model, ok := provider.ResolveProfile(id); !ok || model.Model != "nemotron-3.5-lightning:latest" || model.Effort == "" {
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
			Planning:   config.ModelConfig{Model: "nemotron-3.5-lightning:latest", Effort: "high"},
			Evaluation: config.ModelConfig{Model: "nemotron-3.5-lightning:latest", Effort: "high"},
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

func TestOllamaWarmsBeforeInvokeWhenWarmupConfigured(t *testing.T) {
	recorded := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/show" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"capabilities":["completion","thinking"]}`)
			return
		}
		var payload struct {
			Model    string           `json:"model"`
			Think    any              `json:"think"`
			Format   any              `json:"format"`
			Messages []map[string]any `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode ollama payload: %v", err)
		}
		var content string
		if len(payload.Messages) == 0 {
			content = "(warm preload)"
		} else {
			content = payload.Messages[0]["content"].(string)
		}
		recorded = append(recorded, fmt.Sprintf("think=%v content=%s", payload.Think, content))
		raw, _ := json.Marshal(`{"decision":"finish","reason":"enough"}`)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"message":{"content":%s},"done":true,"done_reason":"stop","prompt_eval_count":10,"eval_count":5,"load_duration":2000000000}`, raw)
	}))
	defer server.Close()

	provider, err := NewOllama(config.Config{
		Root: filepathRoot(t),
		Reasoning: config.ReasoningConfig{
			Provider:        "ollama-nemotron",
			Endpoint:        server.URL,
			TimeoutMS:       30000,
			WarmupTimeoutMS: 30000,
			Planning:        config.ModelConfig{Model: "nemotron-3.5-lightning:latest", Effort: "high"},
			Evaluation:      config.ModelConfig{Model: "nemotron-3.5-lightning:latest", Effort: "high"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()

	observation := domain.Observation{
		Source: domain.SourceX,
		Snapshots: []domain.Snapshot{{
			Blocks: []domain.Block{{EvidenceKey: "x:000000000000000000000001", Text: "ok"}},
		}},
	}
	if _, _, err := provider.Plan(context.Background(), domain.Run{ID: "run-1", Source: domain.SourceX}, observation, nil); err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
	if len(recorded) < 2 {
		t.Fatalf("expected warm then invoke requests, got %d: %+v", len(recorded), recorded)
	}
	if !strings.Contains(recorded[0], "(warm preload)") {
		t.Errorf("first request was not the warm call: %q", recorded[0])
	}
	if strings.HasPrefix(recorded[1], "think=false") || !strings.Contains(recorded[1], "content=") {
		t.Errorf("second request was not the real invocation: %q", recorded[1])
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
	if telemetry.Provider != "ollama" || telemetry.Model != "nemotron-3.5-lightning:latest" || telemetry.Effort != "high" || telemetry.Phase != "acquisition_planning" {
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
		Summary:              "fake ollama",
		Items:                []domain.ReasonedItem{{ID: "item-1", WhatChanged: "Changed", WhyItMatters: "Matters", Source: domain.SourceX, EvidenceKey: "candidate_001", EventKey: "event-one", KnowledgeDelta: "new_event", Author: "author", Confidence: .9, EvidenceState: "primary"}},
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

// TestOllamaCatalogSatisfiesBuiltinProfiles proves the consumer shares the
// SDK's single profile model: every built-in execution profile resolves to a
// native thinking budget whose neutral tier meets the profile's need through
// the shared resolver.
func TestOllamaCatalogSatisfiesBuiltinProfiles(t *testing.T) {
	server := ollamaTestServer(t, nil, "")
	provider, err := NewOllama(ollamaTestConfig(t, server))
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	tier := map[string]inference.ReasoningEffort{
		"off":    inference.ReasoningEffortNone,
		"low":    inference.ReasoningEffortLow,
		"medium": inference.ReasoningEffortMedium,
		"high":   inference.ReasoningEffortHigh,
		"max":    inference.ReasoningEffortMax,
	}
	for _, id := range []string{"structured_fast", "short_reasoning", "general_synthesis", "deep_reasoning"} {
		model, ok := provider.ResolveProfile(id)
		if !ok {
			t.Fatalf("catalog cannot resolve built-in profile %q", id)
		}
		effort, ok := tier[model.Effort]
		if !ok {
			t.Fatalf("profile %q resolved to unknown effort %q", id, model.Effort)
		}
		if !inference.TierAtLeast(effort, inference.ReasoningEffort(model.MinimumTier())) {
			t.Fatalf("profile %q effort %q (tier %q) below need %q", id, model.Effort, effort, model.MinimumTier())
		}
	}
}

func TestOllamaQwenXHighUsesNativeMaxAndRejectsExactXHigh(t *testing.T) {
	var generations int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/show" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"capabilities":["completion","thinking"]}`)
			return
		}
		var payload struct {
			Model string `json:"model"`
			Think string `json:"think"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode qwen payload: %v", err)
		}
		generations++
		if payload.Model != "qwen3.8:27b" || payload.Think != "max" {
			t.Errorf("qwen payload model=%q think=%q, want qwen3.8:27b/max", payload.Model, payload.Think)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"message":{"content":"{\"decision\":\"finish\",\"reason\":\"ok\"}"},"done":true,"prompt_eval_count":1,"eval_count":1}`)
	}))
	defer server.Close()
	base := ollamaTestConfig(t, server)
	base.Reasoning.Planning = config.ModelConfig{ModelID: "qwen3.8-27b", MinReasoningTier: "xhigh"}
	base.Reasoning.Evaluation = base.Reasoning.Planning
	provider, err := NewOllama(base)
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	if _, _, err := provider.Plan(context.Background(), domain.Run{ID: "qwen-max", Source: domain.SourceX}, domain.Observation{}, nil); err != nil {
		t.Fatalf("xhigh requirement should resolve to native max: %v", err)
	}
	if generations != 1 {
		t.Fatalf("generation count=%d, want 1", generations)
	}

	base.Reasoning.Planning.ReasoningOptionID = "xhigh"
	rejected, err := NewOllama(base)
	if err != nil {
		t.Fatal(err)
	}
	defer rejected.Close()
	if _, _, err := rejected.Plan(context.Background(), domain.Run{ID: "qwen-exact", Source: domain.SourceX}, domain.Observation{}, nil); err == nil {
		t.Fatal("exact Qwen xhigh must fail closed before generation")
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
