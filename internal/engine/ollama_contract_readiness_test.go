package engine

import (
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/config"
)

func TestOllamaContractReadinessRejectsRetiredModelsWithoutSubstitution(t *testing.T) {
	for _, id := range []string{"qwen3.8-27b", "nemotron-3.5-lightning"} {
		for _, phase := range []string{"planning", "evaluation", "semanticEvent", "aiDetection"} {
			t.Run(id+"/"+phase, func(t *testing.T) {
				provider := config.ProviderConfig{}
				model := config.ModelConfig{ModelID: id}
				switch phase {
				case "planning":
					provider.Planning = model
				case "evaluation":
					provider.Evaluation = model
				case "semanticEvent":
					provider.SemanticEvent = model
				case "aiDetection":
					provider.AIDetection = model
				}
				original := provider
				summary, rejected := ollamaContractReadiness(config.ProviderSummary{Available: true}, provider)
				if !rejected || summary.Available || !summary.AvailabilityChecked || summary.AvailabilityStatus != "model_unsupported" || !strings.Contains(summary.AvailabilityMessage, id) {
					t.Fatalf("rejected=%t summary=%+v", rejected, summary)
				}
				if provider != original {
					t.Fatal("configured models changed")
				}
			})
		}
	}
}
