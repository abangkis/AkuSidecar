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

func groqTestServer(t *testing.T, output string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-only-key" {
			t.Errorf("authorization header=%q", got)
		}
		if strings.HasPrefix(r.URL.Path, "/models/") {
			_, _ = fmt.Fprint(w, `{"id":"openai/gpt-oss-120b"}`)
			return
		}
		if r.URL.Path != "/responses" {
			t.Fatalf("path=%q", r.URL.Path)
		}
		var payload struct {
			Model           string `json:"model"`
			MaxOutputTokens int    `json:"max_output_tokens"`
			Reasoning       struct {
				Effort string `json:"effort"`
			} `json:"reasoning"`
			Text struct {
				Format map[string]any `json:"format"`
			} `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if payload.Model != "openai/gpt-oss-120b" || payload.MaxOutputTokens != 512 || payload.Reasoning.Effort != "high" || len(payload.Text.Format) == 0 {
			t.Errorf("payload=%+v", payload)
		}
		if name, _ := payload.Text.Format["name"].(string); strings.Contains(name, ".") || name != "akusidecar-planning" {
			t.Errorf("schema name=%q", name)
		}
		raw, _ := json.Marshal(output)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"id":"resp_test","status":"completed","model":"openai/gpt-oss-120b","output":[{"content":[{"type":"output_text","text":%s}]}],"usage":{"input_tokens":11,"output_tokens":7,"total_tokens":18,"output_tokens_details":{"reasoning_tokens":3}}}`, raw)
	}))
	t.Cleanup(server.Close)
	return server
}

func groqTestConfig(t *testing.T, endpoint string) config.Config {
	t.Helper()
	t.Setenv("GROQ_API_KEY", "test-only-key")
	model := config.ModelConfig{ModelID: "openai/gpt-oss-120b", MinReasoningTier: "high", ReasoningOptionID: "high", Assurance: "provider_strict", MaxOutputTokens: 512}
	return config.Config{Root: filepathRoot(t), Reasoning: config.ReasoningConfig{Provider: "groq", Endpoint: endpoint, CredentialRef: "env:GROQ_API_KEY", TimeoutMS: 5000, Planning: model, Evaluation: model}}
}

func TestGroqProfileCatalog(t *testing.T) {
	server := groqTestServer(t, `{}`)
	provider, err := NewGroq(groqTestConfig(t, server.URL))
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	options := provider.ProfileOptions()
	if len(options) != 3 || options[0].Effort != "low" || options[2].Effort != "high" {
		t.Fatalf("options=%+v", options)
	}
	model, ok := provider.ResolveProfile("groq_high")
	if !ok || model.ModelID != "openai/gpt-oss-120b" || model.ReasoningOptionID != "high" {
		t.Fatalf("model=%+v ok=%v", model, ok)
	}
}

func TestGroqStructuredPlan(t *testing.T) {
	server := groqTestServer(t, `{"decision":"finish","reason":"enough bounded evidence"}`)
	provider, err := NewGroq(groqTestConfig(t, server.URL))
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	run, observation := fakeAppServerInput()
	plan, telemetry, err := provider.Plan(context.Background(), run, observation, nil)
	if err != nil || plan.Decision != "finish" {
		t.Fatalf("plan=%+v err=%v", plan, err)
	}
	if telemetry.Provider != "groq" || telemetry.Model != "openai/gpt-oss-120b" || telemetry.Effort != "high" {
		t.Fatalf("telemetry=%+v", telemetry)
	}
	if telemetry.InputTokens == nil || *telemetry.InputTokens != 11 || telemetry.ReasoningOutputTokens == nil || *telemetry.ReasoningOutputTokens != 3 {
		t.Fatalf("telemetry usage=%+v", telemetry)
	}
}

func TestGroqDoesNotFallBackToImplicitEnvironmentReference(t *testing.T) {
	server := groqTestServer(t, `{}`)
	cfg := groqTestConfig(t, server.URL)
	cfg.Reasoning.CredentialRef = ""
	if _, err := NewGroq(cfg); err == nil || !strings.Contains(err.Error(), "unsupported credential reference") {
		t.Fatalf("error=%v", err)
	}
}

func TestExactCandidateCountSchemaConstrainsBothEvaluationArrays(t *testing.T) {
	provider, err := NewGroq(groqTestConfig(t, groqTestServer(t, `{}`).URL))
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	before, err := schemaJSON(provider.resultSchema)
	if err != nil {
		t.Fatal(err)
	}
	projected, err := exactCandidateCountSchema(provider.resultSchema, 3)
	if err != nil {
		t.Fatal(err)
	}
	root := projected.(map[string]any)
	properties := root["properties"].(map[string]any)
	for _, field := range []string{"items", "candidateAssessments"} {
		arraySchema := properties[field].(map[string]any)
		if arraySchema["minItems"] != 3 || arraySchema["maxItems"] != 3 {
			t.Fatalf("%s constraints=%+v", field, arraySchema)
		}
	}
	after, err := schemaJSON(provider.resultSchema)
	if err != nil || string(after) != string(before) {
		t.Fatalf("base schema mutated: err=%v", err)
	}
	if _, err := exactCandidateCountSchema(provider.resultSchema, 0); err == nil {
		t.Fatal("zero candidates must fail closed")
	}
}
