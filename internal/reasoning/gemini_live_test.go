package reasoning

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/config"
)

// TestGeminiLiveSidecarWorkloads is an explicit, non-authoritative acceptance
// gate. It never mutates Sidecar state or logs prompts, outputs, or credentials.
func TestGeminiLiveSidecarWorkloads(t *testing.T) {
	if os.Getenv("AKU_GEMINI_LIVE") != "1" {
		t.Skip("set AKU_GEMINI_LIVE=1 to run the live Gemini Sidecar gate")
	}
	planning := config.ModelConfig{ModelID: "gemini-3.5-flash-lite", MinReasoningTier: "high", ReasoningOptionID: "high", Assurance: "provider_strict", MaxOutputTokens: 2048}
	evaluation := planning
	evaluation.MaxOutputTokens = 8192
	provider, err := NewGemini(config.Config{
		Root: filepathRoot(t),
		Reasoning: config.ReasoningConfig{
			Provider: "gemini-flash-lite", CredentialRef: "gemini.primary", TimeoutMS: 120000,
			Planning: planning, Evaluation: evaluation,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	run, observation := fakeAppServerInput()
	t.Run("planning", func(t *testing.T) {
		for _, known := range []bool{true, false} {
			capture := observation
			capture.Coverage = map[string]any{"captureMode": "headless_worker", "performedScrolls": 2, "scrollStopReason": "budget_exhausted", "frontier": map[string]any{"newCandidateCount": 2, "hasMoreCandidateSignal": true, "anchorKeys": []string{"x:status:10001"}, "continuationReady": true}}
			if !known {
				capture.Coverage = map[string]any{"frontier": map[string]any{"newCandidateCount": nil, "hasMoreCandidateSignal": nil, "continuationReady": nil}}
			}
			plan, telemetry, err := provider.Plan(ctx, run, capture, nil)
			if err != nil || (plan.Decision != "finish" && plan.Decision != "request_follow_up") {
				t.Fatalf("planning gate failed: known=%t decision=%q provider=%q status=%q err=%v", known, plan.Decision, telemetry.Provider, telemetry.Status, err)
			}
			if telemetry.InputTokens == nil || telemetry.OutputTokens == nil {
				t.Fatal("planning gate returned incomplete token telemetry")
			}
			var thinking any = "unknown"
			if telemetry.ReasoningOutputTokens != nil {
				thinking = *telemetry.ReasoningOutputTokens
			}
			t.Logf("known_frontier=%t decision=%s input=%d output=%d reasoning=%v", known, plan.Decision, *telemetry.InputTokens, *telemetry.OutputTokens, thinking)
		}
	})
	t.Run("evaluation", func(t *testing.T) {
		result, telemetry, err := provider.Analyze(ctx, run, observation, nil)
		if err != nil || len(result.Items) != 1 || len(result.CandidateAssessments) != 1 {
			t.Fatalf("evaluation gate failed: items=%d assessments=%d provider=%q status=%q err=%v", len(result.Items), len(result.CandidateAssessments), telemetry.Provider, telemetry.Status, err)
		}
		if telemetry.InputTokens == nil || telemetry.OutputTokens == nil {
			t.Fatal("evaluation gate returned incomplete token telemetry")
		}
	})
}
