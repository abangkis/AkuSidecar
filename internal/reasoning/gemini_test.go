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
	if !ok || model.ModelID != "gemini-3.5-flash-lite" || model.ReasoningOptionID != "high" {
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
