package reasoning

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/ai4u-inference-sdk-go/inference"
)

type staticCredentialResolver string

func (resolver staticCredentialResolver) Resolve(reference string) (string, error) {
	if strings.TrimSpace(reference) == "" {
		return "", fmt.Errorf("credential ref is required")
	}
	return string(resolver), nil
}

func geminiTestServer(t *testing.T, output string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-goog-api-key"); got != "gemini-test-key" {
			t.Errorf("API key header=%q", got)
		}
		if strings.HasPrefix(r.URL.Path, "/models/") {
			_, _ = fmt.Fprint(w, `{"name":"models/gemini-3.5-flash-lite"}`)
			return
		}
		if r.URL.Path != "/interactions" {
			t.Fatalf("path=%q", r.URL.Path)
		}
		var payload struct {
			Model            string `json:"model"`
			Store            bool   `json:"store"`
			GenerationConfig struct {
				MaxOutputTokens int    `json:"max_output_tokens"`
				ThinkingLevel   string `json:"thinking_level"`
			} `json:"generation_config"`
			ResponseFormat struct {
				Schema map[string]any `json:"schema"`
			} `json:"response_format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if payload.Model != "gemini-3.5-flash-lite" || payload.Store || payload.GenerationConfig.MaxOutputTokens != 512 || payload.GenerationConfig.ThinkingLevel != "high" {
			t.Errorf("payload=%+v", payload)
		}
		if _, exists := payload.ResponseFormat.Schema["$schema"]; exists {
			t.Error("unsupported $schema reached Gemini")
		}
		reason := payload.ResponseFormat.Schema["properties"].(map[string]any)["reason"].(map[string]any)
		if _, exists := reason["minLength"]; exists {
			t.Error("unsupported minLength reached Gemini")
		}
		raw, _ := json.Marshal(output)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"id":"interaction-test","model":"gemini-3.5-flash-lite","status":"completed","output_text":%s,"usage":{"total_input_tokens":13,"total_output_tokens":8,"total_thought_tokens":5,"total_tokens":26}}`, raw)
	}))
	t.Cleanup(server.Close)
	return server
}

func geminiTestConfig(t *testing.T, endpoint string) config.Config {
	t.Helper()
	model := config.ModelConfig{ModelID: "gemini-3.5-flash-lite", MinReasoningTier: "high", ReasoningOptionID: "high", Assurance: "provider_strict", MaxOutputTokens: 512}
	return config.Config{Root: filepathRoot(t), Reasoning: config.ReasoningConfig{Provider: "gemini-flash-lite", Endpoint: endpoint, CredentialRef: "gemini.primary", TimeoutMS: 5000, Planning: model, Evaluation: model}}
}

func TestGeminiProfileCatalog(t *testing.T) {
	server := geminiTestServer(t, `{}`)
	provider, err := newGemini(geminiTestConfig(t, server.URL), staticCredentialResolver("gemini-test-key"))
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	options := provider.ProfileOptions()
	if len(options) != 4 || options[0].Effort != "minimal" || options[3].Effort != "high" {
		t.Fatalf("options=%+v", options)
	}
	model, ok := provider.ResolveProfile("gemini_high")
	if !ok || model.ModelID != "gemini-3.5-flash-lite" || model.ReasoningOptionID != "high" || model.MaxOutputTokens != 0 || model.Assurance != "" {
		t.Fatalf("model=%+v ok=%v", model, ok)
	}
}

func TestGeminiStructuredPlanUsesProjectedWireSchemaAndFullLocalValidation(t *testing.T) {
	server := geminiTestServer(t, `{"decision":"finish","reason":"enough bounded evidence"}`)
	provider, err := newGemini(geminiTestConfig(t, server.URL), staticCredentialResolver("gemini-test-key"))
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	run, observation := fakeAppServerInput()
	plan, telemetry, err := provider.Plan(context.Background(), run, observation, nil)
	if err != nil || plan.Decision != "finish" {
		t.Fatalf("plan=%+v err=%v", plan, err)
	}
	if telemetry.Provider != "gemini-flash-lite" || telemetry.InputTokens == nil || *telemetry.InputTokens != 13 || telemetry.ReasoningOutputTokens == nil || *telemetry.ReasoningOutputTokens != 5 {
		t.Fatalf("telemetry=%+v", telemetry)
	}
}

func TestGeminiRejectsResponseThatViolatesCompleteSidecarSchema(t *testing.T) {
	server := geminiTestServer(t, `{"decision":"finish","reason":""}`)
	provider, err := newGemini(geminiTestConfig(t, server.URL), staticCredentialResolver("gemini-test-key"))
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	run, observation := fakeAppServerInput()
	if _, _, err := provider.Plan(context.Background(), run, observation, nil); err == nil || !strings.Contains(err.Error(), "complete Sidecar schema") {
		t.Fatalf("full schema validation error=%v", err)
	}
}

func TestGeminiDoesNotUseImplicitCredentialReference(t *testing.T) {
	server := geminiTestServer(t, `{}`)
	cfg := geminiTestConfig(t, server.URL)
	cfg.Reasoning.CredentialRef = ""
	if _, err := newGemini(cfg, staticCredentialResolver("gemini-test-key")); err == nil || !strings.Contains(err.Error(), "credential ref is required") {
		t.Fatalf("error=%v", err)
	}
}

func TestProjectGeminiSchemaLeavesFullSchemaUnchanged(t *testing.T) {
	root := filepathRoot(t)
	schema, err := readSchema(root + `/schemas/reasoning-result.schema.json`)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := schemaJSON(schema)
	projected, err := projectGeminiSchema(schema)
	if err != nil {
		t.Fatal(err)
	}
	projectedRaw, _ := schemaJSON(projected)
	if strings.Contains(string(projectedRaw), `"pattern"`) || strings.Contains(string(projectedRaw), `"maxLength"`) || strings.Contains(string(projectedRaw), `"maxItems"`) {
		t.Fatalf("unsupported keywords remained: %s", projectedRaw)
	}
	after, _ := schemaJSON(schema)
	if string(after) != string(before) {
		t.Fatal("complete Sidecar schema was mutated")
	}
}

func TestProjectGeminiSchemaRemovesLivingTopicUniqueItemsOnlyOnWire(t *testing.T) {
	root := filepathRoot(t)
	schema, err := readSchema(root + `/schemas/living-topic-snapshot.schema.json`)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := schemaJSON(schema)
	projected, err := projectGeminiSchema(schema)
	if err != nil {
		t.Fatal(err)
	}
	projectedRaw, _ := schemaJSON(projected)
	if strings.Contains(string(projectedRaw), `"uniqueItems"`) {
		t.Fatalf("Gemini wire schema retained uniqueItems: %s", projectedRaw)
	}
	after, _ := schemaJSON(schema)
	if string(after) != string(before) || !strings.Contains(string(after), `"uniqueItems"`) {
		t.Fatal("complete Living Topics schema was mutated")
	}
}

func TestGeminiCandidateEvaluationChunksSevenCandidatesAndAggregatesTelemetry(t *testing.T) {
	server, calls := geminiEvaluationChunkServer(t, 0)
	provider, err := newGemini(geminiTestConfig(t, server.URL), staticCredentialResolver("gemini-test-key"))
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	run, observation := sevenCandidateGeminiInput()
	result, telemetry, err := provider.Analyze(context.Background(), run, observation, nil)
	if err != nil {
		t.Fatalf("analysis error: %v", err)
	}
	if got := calls.sizes; fmt.Sprint(got) != "[6 1]" {
		t.Fatalf("candidate chunk sizes=%v, want [6 1]", got)
	}
	for index, prompt := range calls.prompts {
		if got := strings.Count(prompt, "Gemini Candidate Evaluation compatibility guidance:"); got != 1 {
			t.Fatalf("chunk %d overlay count=%d, want 1", index+1, got)
		}
	}
	if len(result.Items) != 7 || len(result.CandidateAssessments) != 7 {
		t.Fatalf("merged result cardinality items=%d assessments=%d", len(result.Items), len(result.CandidateAssessments))
	}
	for index := range result.Items {
		want := fmt.Sprintf("x:candidate-%d", index+1)
		if result.Items[index].EvidenceKey != want || result.CandidateAssessments[index].EvidenceKey != want {
			t.Fatalf("position %d binding: item=%q assessment=%q want=%q", index, result.Items[index].EvidenceKey, result.CandidateAssessments[index].EvidenceKey, want)
		}
	}
	if result.Summary != "chunk 1\n\nchunk 2" || len(result.Limitations) != 2 || result.RepeatedClaimsCollapsed != 3 || result.DeferredByBudget != 3 {
		t.Fatalf("merged metadata=%+v", result)
	}
	if telemetry.Status != "completed" || telemetry.InputTokens == nil || telemetry.OutputTokens == nil || telemetry.ReasoningOutputTokens == nil {
		t.Fatalf("aggregated telemetry=%+v", telemetry)
	}
	if *telemetry.InputTokens != 30 || *telemetry.OutputTokens != 9 || *telemetry.ReasoningOutputTokens != 6 {
		t.Fatalf("aggregated token telemetry input=%d output=%d reasoning=%d", *telemetry.InputTokens, *telemetry.OutputTokens, *telemetry.ReasoningOutputTokens)
	}
}

func TestGeminiCandidateEvaluationChunkFailureDoesNotReturnPartialSuccess(t *testing.T) {
	server, calls := geminiEvaluationChunkServer(t, 2)
	provider, err := newGemini(geminiTestConfig(t, server.URL), staticCredentialResolver("gemini-test-key"))
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	run, observation := sevenCandidateGeminiInput()
	result, telemetry, err := provider.Analyze(context.Background(), run, observation, nil)
	if err == nil || !strings.Contains(err.Error(), "chunk 2/2") {
		t.Fatalf("error=%v, want second-chunk failure", err)
	}
	if len(result.Items) != 0 || len(result.CandidateAssessments) != 0 {
		t.Fatalf("partial result returned after chunk failure: %+v", result)
	}
	if telemetry.Status != "failed" || telemetry.InputTokens == nil || *telemetry.InputTokens != 10 {
		t.Fatalf("failure telemetry=%+v", telemetry)
	}
	if got := fmt.Sprint(calls.sizes); got != "[6 1]" {
		t.Fatalf("candidate chunk sizes=%s, want [6 1]", got)
	}
}

type geminiChunkCapture struct {
	mu      sync.Mutex
	sizes   []int
	prompts []string
}

func geminiEvaluationChunkServer(t *testing.T, failCall int) (*httptest.Server, *geminiChunkCapture) {
	t.Helper()
	calls := &geminiChunkCapture{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-goog-api-key"); got != "gemini-test-key" {
			t.Errorf("API key header=%q", got)
		}
		if strings.HasPrefix(r.URL.Path, "/models/") {
			_, _ = fmt.Fprint(w, `{"name":"models/gemini-3.5-flash-lite"}`)
			return
		}
		if r.URL.Path != "/interactions" {
			t.Fatalf("path=%q", r.URL.Path)
		}
		var payload struct {
			Input string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		size := strings.Count(payload.Input, `"alias"`)
		calls.mu.Lock()
		calls.sizes = append(calls.sizes, size)
		calls.prompts = append(calls.prompts, payload.Input)
		callNumber := len(calls.sizes)
		calls.mu.Unlock()
		if failCall == callNumber {
			http.Error(w, "provider rejected candidate batch", http.StatusBadRequest)
			return
		}
		output := geminiEvaluationChunkOutput(callNumber, size)
		raw, _ := json.Marshal(output)
		fullSchema, schemaErr := readSchema(filepathRoot(t) + "/schemas/reasoning-result.schema.json")
		if schemaErr != nil {
			t.Errorf("read fixture schema: %v", schemaErr)
		} else if exactSchema, exactErr := exactCandidateCountSchema(fullSchema, size); exactErr != nil {
			t.Errorf("prepare fixture schema: %v", exactErr)
		} else if projected, projectErr := projectGeminiSchema(exactSchema); projectErr != nil {
			t.Errorf("project fixture schema: %v", projectErr)
		} else if projectedRaw, rawErr := schemaJSON(projected); rawErr != nil {
			t.Errorf("encode fixture schema: %v", rawErr)
		} else if validationErr := inference.ValidateJSONSchemaResponse(string(raw), projectedRaw); validationErr != nil {
			t.Errorf("fixture response validation: %v", validationErr)
		}
		encoded, _ := json.Marshal(string(raw))
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"id":"interaction-%d","model":"gemini-3.5-flash-lite","status":"completed","output_text":%s,"usage":{"total_input_tokens":%d,"total_output_tokens":%d,"total_thought_tokens":%d}}`, callNumber, encoded, 10*callNumber, 3*callNumber, 2*callNumber)
	}))
	t.Cleanup(server.Close)
	return server, calls
}

func geminiEvaluationChunkOutput(callNumber, size int) map[string]any {
	base := 1
	if callNumber > 1 {
		base = 7
	}
	items := make([]map[string]any, 0, size)
	assessments := make([]map[string]any, 0, size)
	for index := 0; index < size; index++ {
		alias := fmt.Sprintf("candidate_%03d", base+index)
		items = append(items, map[string]any{
			"id": alias, "whatChanged": alias, "whyItMatters": "fixture", "evidenceKey": alias,
			"eventKey": "fixture-event", "knowledgeDelta": "new_event", "author": "fixture", "publishedAt": nil,
			"confidence": .8, "evidenceState": "primary",
		})
		assessments = append(assessments, map[string]any{
			"evidenceKey": alias, "topicTags": []string{"fixture"}, "topicFacets": []string{"other"},
			"contentType": "other", "novelty": .8, "urgency": .2, "actionability": .3, "materiality": .7,
			"evidenceStrength": .9, "knowledgeRelation": "new_information", "rationale": "fixture",
		})
	}
	return map[string]any{
		"summary": fmt.Sprintf("chunk %d", callNumber), "items": items, "candidateAssessments": assessments,
		"repeatedClaimsCollapsed": callNumber, "deferredByBudget": callNumber, "limitations": []string{fmt.Sprintf("limitation %d", callNumber)},
	}
}

func sevenCandidateGeminiInput() (domain.Run, domain.Observation) {
	blocks := make([]domain.Block, 0, 7)
	for index := 0; index < 7; index++ {
		blocks = append(blocks, domain.Block{EvidenceKey: fmt.Sprintf("x:candidate-%d", index+1), Author: "fixture", Text: "bounded evidence"})
	}
	return domain.Run{ID: "run-gemini-chunking", Source: domain.SourceX}, domain.Observation{Source: domain.SourceX, Snapshots: []domain.Snapshot{{Blocks: blocks}}, Coverage: map[string]any{}}
}
