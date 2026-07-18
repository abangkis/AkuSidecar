package engine

import "github.com/abangkis/AkuSidecar/internal/config"

// ReasoningProcessProfile describes one replaceable inference role without
// exposing transport-specific configuration to the web application.
type ReasoningProcessProfile struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Provider    string `json:"provider"`
	Model       string `json:"model"`
	Effort      string `json:"effort"`
	Execution   string `json:"execution"`
}

func (e *Engine) ReasoningProcesses() []ReasoningProcessProfile {
	provider := e.ProviderName()
	profile := func(id, label, description, execution string, model config.ModelConfig) ReasoningProcessProfile {
		if provider == "deterministic" {
			model = config.ModelConfig{Model: "local-deterministic", Effort: "none"}
		}
		return ReasoningProcessProfile{
			ID: id, Label: label, Description: description, Provider: provider,
			Model: model.Model, Effort: model.Effort, Execution: execution,
		}
	}
	return []ReasoningProcessProfile{
		profile("acquisition_planning", "Acquisition planning", "Decides whether another bounded source observation is useful.", "in-run", e.config.Reasoning.Planning),
		profile("candidate_evaluation", "Candidate evaluation", "Evaluates captured candidates against evidence and personal taste.", "in-run", e.config.Reasoning.Evaluation),
		profile("semantic_event_resolution", "Semantic event resolution", "Resolves likely cross-author reports of the same event.", "in-run", e.config.Reasoning.SemanticEvent),
		profile("ai_deep_detection", "AI Deep Detection", "Reviews AI-origin signals after the Timeline is already usable.", "async", e.config.Reasoning.AIDetection),
	}
}
