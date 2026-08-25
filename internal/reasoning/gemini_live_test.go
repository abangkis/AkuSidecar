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
	planning := config.ModelConfig{ModelID: "gemini-3.5-flash-lite", MinReasoningTier: "high", ReasoningOptionID: "high", Assurance: "provider_strict", MaxOutputTokens: 512}
	evaluation := planning
	evaluation.MaxOutputTokens = 4096
	provider, err := NewGemini(config.Config{
		Root: filepathRoot(t),
		Reasoning: config.ReasoningConfig{
			Provider: "gemini-flash-lite", CredentialRef: "env:GEMINI_API_KEY", TimeoutMS: 120000,
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
		plan, telemetry, err := provider.Plan(ctx, run, observation, nil)
		if err != nil || (plan.Decision != "finish" && plan.Decision != "request_follow_up") {
			t.Fatalf("planning gate failed: decision=%q provider=%q status=%q err=%v", plan.Decision, telemetry.Provider, telemetry.Status, err)
		}
		if telemetry.InputTokens == nil || telemetry.OutputTokens == nil {
			t.Fatal("planning gate returned incomplete token telemetry")
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
