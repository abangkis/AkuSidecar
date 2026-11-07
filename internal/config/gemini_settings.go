package config

import "strings"

const geminiFlashLiteProvider = "gemini-flash-lite"

var geminiSettingsModels = [...]string{
	"gemini-3.1-flash-lite", "gemini-3.5-flash-lite", "gemini-3.6-flash", "gemini-3.7-flash",
}

// Reuse the configured Gemini endpoint, credential and workload policies.
// Keep the old provider key so existing selections and profiles still work.
func (r *ReasoningConfig) addGeminiSettingsProviders() {
	base, ok := r.Providers[geminiFlashLiteProvider]
	if !ok || base.HideFromSettings {
		return
	}
	for _, id := range geminiSettingsModels {
		// Saved provider IDs cannot contain periods; Gemini model IDs do.
		name := strings.ReplaceAll(id, ".", "-")
		if _, declared := r.Providers[name]; declared || id == "gemini-3.1-flash-lite" && base.usesOnlyGeminiModel(id) {
			continue
		}
		variant := base
		for _, model := range []*ModelConfig{&variant.Planning, &variant.Evaluation, &variant.SemanticEvent, &variant.AIDetection} {
			model.ModelID, model.Model = id, ""
		}
		r.Providers[name] = variant
	}
}

func (p ProviderConfig) usesOnlyGeminiModel(id string) bool {
	return p.Planning.StableModelID() == id && p.Evaluation.StableModelID() == id &&
		p.SemanticEvent.StableModelID() == id && p.AIDetection.StableModelID() == id
}
