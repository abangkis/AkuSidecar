package reasoning

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/ai4u-inference-sdk-go/inference"
	sdkollama "github.com/abangkis/ai4u-inference-sdk-go/providers/ollama"
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
		if payload.Model != "saluki:8k" {
			t.Errorf("model = %v, want saluki:8k", payload.Model)
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
			Planning:   testOllamaModelConfig(),
			Evaluation: testOllamaModelConfig(),
		},
	}
}

func testOllamaModelConfig() config.ModelConfig {
	return config.ModelConfig{ModelID: string(sdkollama.ModelSaluki8K), Model: string(sdkollama.ModelSaluki8K), MinReasoningTier: "high", ReasoningOptionID: "xhigh", Effort: "high"}
}

// newOllamaTestProvider opts into one SDK experimental fixture explicitly for
// tests. Production NewOllama intentionally accepts only the stable catalog.
func newOllamaTestProvider(t *testing.T, cfg config.Config) *Ollama {
	t.Helper()
	endpoint := strings.TrimSpace(cfg.Reasoning.Endpoint)
	if endpoint == "" {
		endpoint = ollamaDefaultEndpoint
	}
	transport, err := sdkollama.New(sdkollama.Config{
		BaseURL: endpoint, Timeout: time.Duration(cfg.Reasoning.TimeoutMS) * time.Millisecond,
		MaxRetries: cfg.Reasoning.MaxRetries, KeepAliveSeconds: cfg.Reasoning.KeepAliveMinutes * 60,
		NumCtx: cfg.Reasoning.NumCtx, MaxConcurrentInvocations: cfg.Reasoning.OllamaMaxConcurrentInvocations,
		ExperimentalModelIDs: []sdkollama.ModelID{sdkollama.ModelSaluki8K},
	})
	if err != nil {
		t.Fatal(err)
	}
	clients, err := newBoundClientPool(transport)
	if err != nil {
		_ = transport.Close()
		t.Fatal(err)
	}
	planSchema, err := readSchema(filepath.Join(cfg.Root, "schemas", "acquisition-plan.schema.json"))
	if err != nil {
		_ = clients.Close()
		t.Fatal(err)
	}
	resultSchema, err := readSchema(filepath.Join(cfg.Root, "schemas", "reasoning-result.schema.json"))
	if err != nil {
		_ = clients.Close()
		t.Fatal(err)
	}
	name := strings.TrimSpace(cfg.Reasoning.Provider)
	if name == "" {
		name = "ollama"
	}
	return &Ollama{
		name: name, endpoint: endpoint, maxRetries: cfg.Reasoning.MaxRetries,
		timeout:       time.Duration(cfg.Reasoning.TimeoutMS) * time.Millisecond,
		warmupTimeout: time.Duration(cfg.Reasoning.WarmupTimeoutMS) * time.Millisecond,
		planning:      cfg.Reasoning.Planning, evaluation: cfg.Reasoning.Evaluation,
		planSchema: planSchema, resultSchema: resultSchema, transport: transport,
		catalog: transport.ModelCatalog(), clients: clients, order: ollamaProfileOrder,
	}
}

func TestOllamaProfileCatalog(t *testing.T) {
	server := ollamaTestServer(t, nil, "")
	provider := newOllamaTestProvider(t, ollamaTestConfig(t, server))
	defer provider.Close()
	options := provider.ProfileOptions()
	if len(options) != 4 {
		t.Fatalf("options=%+v", options)
	}
	for _, id := range []string{"structured_fast", "short_reasoning", "general_synthesis", "deep_reasoning"} {
		if model, ok := provider.ResolveProfile(id); !ok || model.Model != "saluki:8k" || model.Effort == "" {
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
	provider := newOllamaTestProvider(t, config.Config{
		Root: filepathRoot(t),
		Reasoning: config.ReasoningConfig{
			Provider: "ollama", TimeoutMS: 5000,
			Planning: testOllamaModelConfig(), Evaluation: testOllamaModelConfig(),
		},
	})
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

func TestOllamaStableCatalogRejectionPreservesConfiguredIDAndSkipsEndpoint(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	cfg := ollamaTestConfig(t, server)
	cfg.Reasoning.Planning = config.ModelConfig{Model: "nemotron-3.5-lightning:latest", Effort: "high"}
	cfg.Reasoning.Evaluation = cfg.Reasoning.Planning
	_, err := NewOllama(cfg)
	var unsupported *UnsupportedModelError
	if !errors.As(err, &unsupported) || unsupported.Provider != "ollama" || unsupported.ModelID != "nemotron-3.5-lightning:latest" {
		t.Fatalf("error=%T %v, want typed unsupported-model error", err, err)
	}
	if requests.Load() != 0 {
		t.Fatalf("unsupported model triggered %d endpoint requests", requests.Load())
	}
	if cfg.Reasoning.Planning.Model != "nemotron-3.5-lightning:latest" {
		t.Fatalf("configured model was replaced: %+v", cfg.Reasoning.Planning)
	}
	if err := ValidateOllamaModelSupport("nemotron-3.5-lightning"); err == nil {
		t.Fatal("retired stable ID unexpectedly accepted")
	} else if !errors.As(err, &unsupported) || unsupported.ModelID != "nemotron-3.5-lightning" {
		t.Fatalf("validation error=%T %v", err, err)
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

	provider := newOllamaTestProvider(t, config.Config{
		Root: filepathRoot(t),
		Reasoning: config.ReasoningConfig{
			Provider:        "ollama-nemotron",
			Endpoint:        server.URL,
			TimeoutMS:       30000,
			WarmupTimeoutMS: 30000,
			Planning:        testOllamaModelConfig(),
			Evaluation:      testOllamaModelConfig(),
		},
	})
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
	if !strings.Contains(recorded[0], "(warm preload)") || !strings.HasPrefix(recorded[0], "think=false") {
		t.Errorf("first request was not the warm call: %q", recorded[0])
	}
	if !strings.HasPrefix(recorded[1], "think=true") || !strings.Contains(recorded[1], "content=") {
		t.Errorf("second request was not the real invocation: %q", recorded[1])
	}
}

func TestOllamaStructuredPlan(t *testing.T) {
	content := `{"decision":"finish","reason":"enough bounded evidence"}`
	server := ollamaTestServer(t, true, content)
	provider := newOllamaTestProvider(t, ollamaTestConfig(t, server))
	defer provider.Close()
	run, observation := fakeAppServerInput()
	plan, telemetry, err := provider.Plan(context.Background(), run, observation, nil)
	if err != nil || plan.Decision != "finish" {
		t.Fatalf("plan=%+v err=%v", plan, err)
	}
	if telemetry.Provider != "ollama" || telemetry.Model != "saluki-8k" || telemetry.Effort != "high" || telemetry.Phase != "acquisition_planning" {
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
		Items:                []domain.ReasonedItem{{ID: "item-1", WhatChanged: "Changed", WhyItMatters: "Matters", EvidenceKey: "candidate_001", EventKey: "event-one", KnowledgeDelta: "new_event", Author: "author", Confidence: .9, EvidenceState: "primary"}},
		CandidateAssessments: []domain.CandidateAssessment{{EvidenceKey: "candidate_001", TopicTags: []string{"ai"}, TopicFacets: []string{"ai_models"}, ContentType: "release", Novelty: .8, Urgency: .4, Actionability: .6, Materiality: .8, EvidenceStrength: .9, Rationale: "fixture", KnowledgeRelation: "new_information"}},
		Limitations:          []string{},
	})
	server := ollamaTestServer(t, true, conformReasoningFixture(t, raw))
	provider := newOllamaTestProvider(t, ollamaTestConfig(t, server))
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
	provider := newOllamaTestProvider(t, ollamaTestConfig(t, server))
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

func TestOllamaSalukiXHighUsesNativeFlagAndRejectsUnsupportedMax(t *testing.T) {
	var generations int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/show" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"capabilities":["completion","thinking"]}`)
			return
		}
		var payload struct {
			Model string `json:"model"`
			Think any    `json:"think"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode qwen payload: %v", err)
		}
		generations++
		if payload.Model != "saluki:8k" || payload.Think != true {
			t.Errorf("Saluki payload model=%q think=%v, want saluki:8k/true", payload.Model, payload.Think)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"message":{"content":"{\"decision\":\"finish\",\"reason\":\"ok\"}"},"done":true,"prompt_eval_count":1,"eval_count":1}`)
	}))
	defer server.Close()
	base := ollamaTestConfig(t, server)
	base.Reasoning.Planning = testOllamaModelConfig()
	base.Reasoning.Evaluation = base.Reasoning.Planning
	provider := newOllamaTestProvider(t, base)
	defer provider.Close()
	if _, _, err := provider.Plan(context.Background(), domain.Run{ID: "saluki-xhigh", Source: domain.SourceX}, domain.Observation{}, nil); err != nil {
		t.Fatalf("xhigh requirement should resolve to native enabled thinking: %v", err)
	}
	if generations != 1 {
		t.Fatalf("generation count=%d, want 1", generations)
	}

	base.Reasoning.Planning.MinReasoningTier = "max"
	base.Reasoning.Planning.ReasoningOptionID = "max"
	rejected := newOllamaTestProvider(t, base)
	defer rejected.Close()
	if _, _, err := rejected.Plan(context.Background(), domain.Run{ID: "saluki-max", Source: domain.SourceX}, domain.Observation{}, nil); err == nil {
		t.Fatal("unsupported exact max must fail closed before generation")
	}
	if generations != 1 {
		t.Fatalf("unsupported max reached generation endpoint; count=%d", generations)
	}
}

func TestEnsureResolvableProfileMigratesLegacyValues(t *testing.T) {
	codex := &CodexAppServer{}
	if got := EnsureResolvableProfile(codex, "luna_high"); got != "luna_high" {
		t.Fatalf("resolvable profile changed to %q", got)
	}
	if got := EnsureResolvableProfile(codex, "luna_max"); got != "luna_max" {
		t.Fatalf("explicit max profile changed to %q", got)
	}
	if got := EnsureResolvableProfile(codex, "unknown"); got != "luna_high" {
		t.Fatalf("unknown profile migrated to %q, want luna_high", got)
	}
	if got := EnsureResolvableProfile(Deterministic{}, "anything"); got != "anything" {
		t.Fatalf("non-catalog provider changed profile to %q", got)
	}
}

func conformReasoningFixture(t *testing.T, raw []byte) string {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	items, _ := doc["items"].([]any)
	for _, value := range items {
		item, ok := value.(map[string]any)
		if !ok {
			continue
		}
		delete(item, "source")
	}
	conformed, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return string(conformed)
}

func TestProbeValidationDetail(t *testing.T) {
	var capturedSchema map[string]any
	var capturedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/show" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, "{\"capabilities\":[\"completion\",\"thinking\"]}")
			return
		}
		var payload struct {
			Model  string         `json:"model"`
			Think  any            `json:"think"`
			Format map[string]any `json:"format"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		capturedSchema = payload.Format
		raw, _ := json.Marshal(capturedBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, "{\"message\":{\"content\":%s},\"done\":true,\"done_reason\":\"stop\",\"prompt_eval_count\":10,\"eval_count\":5}", raw)
	}))
	defer server.Close()

	raw, _ := json.Marshal(domain.ReasoningResult{
		Summary:              "fake ollama",
		Items:                []domain.ReasonedItem{{ID: "item-1", WhatChanged: "Changed", WhyItMatters: "Matters", EvidenceKey: "candidate_001", EventKey: "event-one", KnowledgeDelta: "new_event", Author: "author", Confidence: .9, EvidenceState: "primary"}},
		CandidateAssessments: []domain.CandidateAssessment{{EvidenceKey: "candidate_001", TopicTags: []string{"ai"}, TopicFacets: []string{"ai_models"}, ContentType: "release", Novelty: .8, Urgency: .4, Actionability: .6, Materiality: .8, EvidenceStrength: .9, Rationale: "fixture", KnowledgeRelation: "new_information"}},
		Limitations:          []string{},
	})
	capturedBody = conformReasoningFixture(t, raw)

	cfg := ollamaTestConfig(t, server)
	provider := newOllamaTestProvider(t, cfg)
	defer provider.Close()
	run, observation := fakeAppServerInput()
	if _, _, err := provider.Analyze(context.Background(), run, observation, nil); err != nil {
		t.Fatal(err)
	}
	if capturedSchema == nil {
		t.Fatal("provider request did not include a structured schema")
	}

	schemaBytes, _ := json.Marshal(capturedSchema)
	if detail := inference.ValidateJSONSchemaResponse(capturedBody, schemaBytes); detail != nil {
		t.Fatalf("PROBE validation detail: %v", detail)
	}
	t.Log("PROBE: fixture conforms")
}
