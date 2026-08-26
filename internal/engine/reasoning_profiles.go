package engine

import (
	"context"
	"fmt"
	"os"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/credentials"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
)

func (e *Engine) ReasoningProviders() []config.ProviderSummary {
	summaries := e.config.Reasoning.ProviderSummary()
	store := credentials.ForRoot(e.config.Root)
	for index := range summaries {
		provider := e.config.Reasoning.Providers[summaries[index].Name]
		credentialRef := provider.CredentialRef
		if credentialRef == "" {
			continue
		}
		summaries[index].CredentialName = credentialRef
		if _, err := store.Resolve(credentialRef); err != nil {
			summaries[index].Configured = false
			summaries[index].ConfigurationStatus = "missing_credential"
		}
	}
	return summaries
}

func (e *Engine) reasoningProviderConfiguration(name string) (config.ProviderSummary, bool) {
	for _, provider := range e.ReasoningProviders() {
		if provider.Name == name {
			return provider, true
		}
	}
	return config.ProviderSummary{}, false
}

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

type ReasoningRuntimeProfile struct {
	Provider       string `json:"provider"`
	Label          string `json:"label"`
	ExecutablePath string `json:"executablePath"`
	Editable       bool   `json:"editable"`
	AutoRepaired   bool   `json:"autoRepaired,omitempty"`
}

func (e *Engine) ReasoningRuntime() ReasoningRuntimeProfile {
	result := ReasoningRuntimeProfile{Provider: e.ProviderName(), Label: "Inference executable"}
	if e.ProviderName() == "codex-app-server" {
		result.Label = "Codex executable"
	}
	if runtime, ok := e.provider.(reasoning.ExecutableRuntime); ok {
		result.ExecutablePath = runtime.ExecutablePath()
		result.Editable = true
	}
	return result
}

func (e *Engine) DiscoverReasoningExecutable(ctx context.Context) (ReasoningRuntimeProfile, error) {
	e.operation.Lock()
	defer e.operation.Unlock()
	runtime, ok := e.provider.(reasoning.ExecutableRuntime)
	if !ok {
		return e.ReasoningRuntime(), fmt.Errorf("%s does not expose an editable executable", e.ProviderName())
	}
	path, err := runtime.DiscoverExecutable(ctx, "")
	if err != nil {
		return e.ReasoningRuntime(), err
	}
	result := e.ReasoningRuntime()
	result.ExecutablePath = path
	if executableAvailable(runtime.ExecutablePath()) {
		return result, nil
	}
	settings, err := e.store.GetSettings(ctx)
	if err != nil {
		return e.ReasoningRuntime(), err
	}
	settings.ReasoningExecutablePath = path
	if err := e.store.SaveSettings(ctx, settings); err != nil {
		return e.ReasoningRuntime(), fmt.Errorf("persist rediscovered reasoning executable: %w", err)
	}
	runtime.UseExecutable(path)
	result = e.ReasoningRuntime()
	result.AutoRepaired = true
	return result, nil
}

func executableAvailable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
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
			Model: model.DisplayModel(), Effort: model.DisplayEffort(), Execution: execution,
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
			// Profile selection owns model identity and reasoning effort. Output
			// budgets and structured-output assurance remain workload policy and
			// must not be replaced by a provider-wide profile default.
			model.Assurance = fallback.Assurance
			model.MaxOutputTokens = fallback.MaxOutputTokens
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
		return routed.PlanWithModel(ctx, run, observation, knowledge, string(reasoning.ExecutionProfilePlanning), e.reasoningModel(profileID, e.config.Reasoning.Planning))
	}
	return e.provider.Plan(ctx, run, observation, knowledge)
}

func (e *Engine) analyzeWithProfile(ctx context.Context, run domain.Run, observation domain.Observation, knowledge []domain.ReasonedItem, profileID string) (domain.ReasoningResult, domain.ReasoningTelemetry, error) {
	if routed, ok := e.provider.(reasoning.RoutedProvider); ok {
		return routed.AnalyzeWithModel(ctx, run, observation, knowledge, string(reasoning.ExecutionProfileEvaluation), e.reasoningModel(profileID, e.config.Reasoning.Evaluation))
	}
	return e.provider.Analyze(ctx, run, observation, knowledge)
}
