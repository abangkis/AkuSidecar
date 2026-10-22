package engine

import (
	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
)

// ollamaContractReadiness runs before any endpoint probe. An installed model
// is not usable when the SDK no longer supports its stable model contract.
func ollamaContractReadiness(summary config.ProviderSummary, provider config.ProviderConfig) (config.ProviderSummary, bool) {
	for _, model := range []config.ModelConfig{provider.Planning, provider.Evaluation, provider.SemanticEvent, provider.AIDetection} {
		if model.StableModelID() == "" {
			continue
		}
		if err := reasoning.ValidateOllamaModelSupport(model.StableModelID()); err != nil {
			summary.AvailabilityChecked = true
			summary.Available = false
			summary.AvailabilityStatus = "model_unsupported"
			summary.AvailabilityMessage = err.Error()
			return summary, true
		}
	}
	return summary, false
}
