package reasoning

import (
	"strings"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/ai4u-inference-sdk-go/providers/codexappserver"
)

// Keep application model availability bounded even when the SDK catalog grows.
// Luna comes first so unknown persisted selections retain the high-effort default.
var codexProfileModels = []struct {
	id     codexappserver.ModelID
	prefix string
	label  string
}{
	{codexappserver.ModelGPT6Luna, "gpt6_luna", "GPT-6 Luna"},
	{codexappserver.ModelGPT61Terra, "gpt61_terra", "GPT-6.1 Terra"},
	{codexappserver.ModelGPT61Sol, "gpt61_sol", "GPT-6.1 Sol"},
}

func (c *CodexAppServer) ProfileOptions() []ProfileOption {
	var options []ProfileOption
	for _, model := range codexProfileModels {
		descriptor, ok := codexappserver.KnownModel(model.id)
		if !ok {
			continue
		}
		for _, reasoning := range descriptor.Capabilities.Reasoning {
			switch reasoning.ID {
			case "low", "medium", "high", "xhigh", "max":
				options = append(options, ProfileOption{
					ID: model.prefix + "_" + reasoning.ID, Label: model.label + " " + strings.ToUpper(reasoning.ID[:1]) + reasoning.ID[1:],
					Model: descriptor.ProviderModel, Effort: reasoning.ID,
				})
			}
		}
	}
	return options
}

func (c *CodexAppServer) ResolveProfile(id string) (config.ModelConfig, bool) {
	for _, option := range c.ProfileOptions() {
		if option.ID == id {
			modelID, ok := codexStableModelID(option.Model)
			if !ok {
				return config.ModelConfig{}, false
			}
			return config.ModelConfig{ModelID: modelID, Model: option.Model, MinReasoningTier: option.Effort, ReasoningOptionID: option.Effort, Effort: option.Effort, ProfileID: id}, true
		}
	}
	return config.ModelConfig{}, false
}

// MigrateProfileID translates only previously advertised selections. Model
// aliases are deliberately excluded: the SDK still maps them to GPT-5.6.
func (c *CodexAppServer) MigrateProfileID(id string) (string, bool) {
	switch id {
	case "luna_high", "luna_xhigh", "luna_max":
		return "gpt6_" + id, true
	case "terra_high", "terra_xhigh":
		return "gpt61_" + id, true
	case "sol_medium":
		return "gpt61_" + id, true
	default:
		return "", false
	}
}

func codexStableModelID(value string) (string, bool) {
	switch strings.TrimSpace(value) {
	case string(codexappserver.ModelGPT6Luna), "gpt-6-luna":
		return string(codexappserver.ModelGPT6Luna), true
	case string(codexappserver.ModelGPT61Terra), "gpt-6.1-terra":
		return string(codexappserver.ModelGPT61Terra), true
	case string(codexappserver.ModelGPT61Sol), "gpt-6.1-sol":
		return string(codexappserver.ModelGPT61Sol), true
	default:
		return "", false
	}
}
