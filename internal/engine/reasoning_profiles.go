package engine

import (
	"context"
	"fmt"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
)

// ReasoningProcessProfile describes one replaceable inference role without
// exposing transport-specific configuration to the web application.
type ReasoningProcessProfile struct {
	ID          string                    `json:"id"`
	Label       string                    `json:"label"`
	Description string                    `json:"description"`
	Provider    string                    `json:"provider"`
	Model       string                    `json:"model"`
	Effort      string                    `json:"effort"`
	Execution   string                    `json:"execution"`
	ProfileID   string                    `json:"profileId"`
	Options     []reasoning.ProfileOption `json:"options"`
}

func (e *Engine) ReasoningProcesses(settings domain.Settings) []ReasoningProcessProfile {
	provider := e.ProviderName()
	options := []reasoning.ProfileOption{}
	if catalog, ok := e.provider.(reasoning.ProfileProvider); ok {
		options = catalog.ProfileOptions()
	}
	profile := func(id, label, description, execution, profileID string, fallback config.ModelConfig) ReasoningProcessProfile {
		model := e.reasoningModel(profileID, fallback)
		if provider == "deterministic" {
			model = config.ModelConfig{Model: "local-deterministic", Effort: "none"}
		}
		return ReasoningProcessProfile{
			ID: id, Label: label, Description: description, Provider: provider,
			Model: model.Model, Effort: model.Effort, Execution: execution,
			ProfileID: profileID, Options: append([]reasoning.ProfileOption(nil), options...),
		}
	}
	return []ReasoningProcessProfile{
		profile("acquisition_planning", "Acquisition planning", "Decides whether another bounded source observation is useful.", "in-run", settings.ReasoningAcquisitionProfile, e.config.Reasoning.Planning),
		profile("candidate_evaluation", "Candidate evaluation", "Evaluates captured candidates against evidence and personal taste.", "in-run", settings.ReasoningEvaluationProfile, e.config.Reasoning.Evaluation),
		profile("semantic_event_resolution", "Semantic event resolution", "Resolves likely cross-author reports of the same event.", "in-run", settings.ReasoningSemanticProfile, e.config.Reasoning.SemanticEvent),
		profile("ai_deep_detection", "AI Deep Detection", "Reviews AI-origin signals after the Timeline is already usable.", "async", settings.ReasoningAIDeepProfile, e.config.Reasoning.AIDetection),
	}
}

func (e *Engine) reasoningModel(profileID string, fallback config.ModelConfig) config.ModelConfig {
	if catalog, ok := e.provider.(reasoning.ProfileProvider); ok {
		if model, found := catalog.ResolveProfile(profileID); found {
			return model
		}
	}
	return fallback
}

func (e *Engine) validateReasoningProfiles(settings domain.Settings) error {
	catalog, ok := e.provider.(reasoning.ProfileProvider)
	if !ok {
		return nil
	}
	for name, profileID := range map[string]string{
		"acquisition planning":      settings.ReasoningAcquisitionProfile,
		"candidate evaluation":      settings.ReasoningEvaluationProfile,
		"semantic event resolution": settings.ReasoningSemanticProfile,
		"AI Deep Detection":         settings.ReasoningAIDeepProfile,
	} {
		if _, found := catalog.ResolveProfile(profileID); !found {
			return fmt.Errorf("unsupported %s profile %q", name, profileID)
		}
	}
	return nil
}

func (e *Engine) planWithProfile(ctx context.Context, run domain.Run, observation domain.Observation, knowledge []domain.ReasonedItem, profileID string) (reasoning.AcquisitionPlan, domain.ReasoningTelemetry, error) {
	if routed, ok := e.provider.(reasoning.RoutedProvider); ok {
		return routed.PlanWithModel(ctx, run, observation, knowledge, e.reasoningModel(profileID, e.config.Reasoning.Planning))
	}
	return e.provider.Plan(ctx, run, observation, knowledge)
}

func (e *Engine) analyzeWithProfile(ctx context.Context, run domain.Run, observation domain.Observation, knowledge []domain.ReasonedItem, profileID string) (domain.ReasoningResult, domain.ReasoningTelemetry, error) {
	if routed, ok := e.provider.(reasoning.RoutedProvider); ok {
		return routed.AnalyzeWithModel(ctx, run, observation, knowledge, e.reasoningModel(profileID, e.config.Reasoning.Evaluation))
	}
	return e.provider.Analyze(ctx, run, observation, knowledge)
}
