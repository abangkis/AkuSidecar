package reasoning

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/config"
)

// TestGroqLiveSidecarWorkloads is an explicit, non-authoritative development
// gate. It does not mutate Sidecar state and never logs prompts, outputs, or
// credentials. Enable it with AKU_GROQ_LIVE=1 and GROQ_API_KEY.
func TestGroqLiveSidecarWorkloads(t *testing.T) {
	if os.Getenv("AKU_GROQ_LIVE") != "1" {
		t.Skip("set AKU_GROQ_LIVE=1 to run the live Groq Sidecar gate")
	}
	planning := config.ModelConfig{ModelID: "openai/gpt-oss-120b", MinReasoningTier: "high", ReasoningOptionID: "high", Assurance: "provider_strict", MaxOutputTokens: 512}
	evaluation := planning
	evaluation.MaxOutputTokens = 4096
	provider, err := NewGroq(config.Config{
		Root: filepathRoot(t),
		Reasoning: config.ReasoningConfig{
			Provider: "groq", CredentialRef: "env:GROQ_API_KEY", TimeoutMS: 120000,
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
		if telemetry.InputTokens == nil {
			t.Fatal("planning gate returned no token telemetry")
		}
	})
	t.Run("evaluation", func(t *testing.T) {
		result, telemetry, err := provider.Analyze(ctx, run, observation, nil)
		if err != nil || len(result.Items) == 0 || len(result.CandidateAssessments) == 0 {
			t.Fatalf("evaluation gate failed: items=%d assessments=%d provider=%q status=%q err=%v", len(result.Items), len(result.CandidateAssessments), telemetry.Provider, telemetry.Status, err)
		}
		if telemetry.InputTokens == nil {
			t.Fatal("evaluation gate returned no token telemetry")
		}
	})
}
