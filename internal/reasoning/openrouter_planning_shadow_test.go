package reasoning

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/ai4u-inference-sdk-go/inference"
)

func TestGLMPlanningShadowUsesBoundedStrictResponsesRequest(t *testing.T) {
	for _, test := range []struct {
		name, output string
		status       int
		code         inference.FailureCode
	}{
		{"valid", `{"decision":"request_follow_up","reason":"More telemetry."}`, 200, ""},
		{"invalid schema", `{"decision":"unknown","reason":"Invalid."}`, 200, inference.FailureCodeValidation},
		{"privacy route unavailable", "", 503, inference.FailureCodeUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			const fixtureKey = "test-only-shadow-key"
			const private = "PRIVATE-SENTINEL"
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.URL.Path != "/responses" || r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer "+fixtureKey {
					t.Errorf("unexpected shadow route: %s %s", r.Method, r.URL.Path)
				}
				var request struct {
					Model     string `json:"model"`
					Input     string `json:"input"`
					Store     *bool  `json:"store"`
					MaxTokens int    `json:"max_output_tokens"`
					Provider  struct {
						ZDR  bool   `json:"zdr"`
						Data string `json:"data_collection"`
					} `json:"provider"`
					Reasoning struct{ Effort string } `json:"reasoning"`
					Text      struct {
						Format struct {
							Type   string          `json:"type"`
							Schema json.RawMessage `json:"schema"`
						} `json:"format"`
					} `json:"text"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				if request.Model != GLMPlanningShadowModel || request.Store == nil || *request.Store ||
					!request.Provider.ZDR || request.Provider.Data != "deny" || request.MaxTokens != 2048 ||
					request.Reasoning.Effort != "high" || request.Text.Format.Type != "json_schema" ||
					!strings.Contains(string(request.Text.Format.Schema), "request_follow_up") {
					t.Error("unexpected shadow model, privacy, budget, or schema")
				}
				if strings.Contains(request.Input, private) || !strings.Contains(request.Input, `"anchorCount":1`) ||
					!strings.Contains(request.Input, `"continuationReady":true`) {
					t.Error("shadow prompt must contain bounded telemetry and exclude private fields")
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(test.status)
				if test.status != http.StatusOK {
					_, _ = w.Write([]byte(`{"error":{"message":"No eligible endpoint for this privacy policy"}}`))
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id": "fixture", "model": GLMPlanningShadowModel, "status": "completed",
					"output": []any{map[string]any{"content": []any{map[string]string{"type": "output_text", "text": test.output}}}},
					"usage":  map[string]int{"input_tokens": 10, "output_tokens": 5},
				})
			}))
			defer server.Close()
			shadow, err := newGLMPlanningShadow(filepathRoot(t), staticCredentialResolver(fixtureKey), server.URL)
			if err != nil {
				t.Fatal(err)
			}
			observation := domain.Observation{
				Source:    domain.SourceX,
				Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{EvidenceKey: private, Text: private, Permalink: "https://example.test/" + private}}}},
				Coverage: map[string]any{
					"scrollStopReason": private, "captureQuality": map[string]any{"verdict": private},
					"frontier": map[string]any{"anchorKeys": []string{private}, "continuationReady": true},
				},
			}
			decision, usage, err := shadow.Plan(context.Background(), NewGLMPlanningShadowInput(domain.Run{ID: private, Source: domain.SourceX}, observation))
			if test.code != "" {
				var failure *inference.Error
				if decision != "" || !errors.As(err, &failure) || failure.Code != test.code {
					t.Fatalf("decision=%q err=%v", decision, err)
				}
			} else if err != nil || decision != "request_follow_up" || usage.Input == nil || *usage.Input != 10 {
				t.Fatalf("decision=%q usage=%+v err=%v", decision, usage, err)
			}
			if calls.Load() != 1 {
				t.Fatalf("shadow requests=%d, want one", calls.Load())
			}
		})
	}
}
