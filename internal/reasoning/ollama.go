package reasoning

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/ai4u-inference-sdk-go/inference"
	"github.com/abangkis/ai4u-inference-sdk-go/providers/ollama"
)

// ollamaDefaultEndpoint is used when the reasoning endpoint is left unset.
const ollamaDefaultEndpoint = "http://127.0.0.1:11434"

// ollamaProfileOrder keeps the published profile catalog deterministic.
var ollamaProfileOrder = []inference.ProfileID{
	"structured_fast", "short_reasoning", "general_synthesis", "deep_reasoning",
}

// ollamaBindings maps each built-in execution profile to a native Ollama
// thinking budget. The adapter's own native-to-tier claims make the map
// satisfiable; inference.Resolve verifies the binding at profile resolution.
// Ollama exposes the Ollama Chat transport as an AkuSidecar reasoning
// provider. Profile selection is validated through the execution profile
// model: the built-in profiles resolve to native thinking budgets via a
// capability binding map, and inference.Resolve enforces satisfiability.
type Ollama struct {
	name          string
	endpoint      string
	maxRetries    int
	timeout       time.Duration
	warmupTimeout time.Duration
	planning      config.ModelConfig
	evaluation    config.ModelConfig
	planSchema    any
	resultSchema  any
	transport     *ollama.Adapter
	clients       *boundClientPool
	order         []inference.ProfileID
}

func NewOllama(cfg config.Config) (*Ollama, error) {
	name := strings.TrimSpace(cfg.Reasoning.Provider)
	if name == "" {
		name = "ollama"
	}
	endpoint := strings.TrimSpace(cfg.Reasoning.Endpoint)
	if endpoint == "" {
		endpoint = ollamaDefaultEndpoint
	}
	if cfg.Reasoning.Planning.StableModelID() == "" {
		return nil, fmt.Errorf("ollama planning model is required")
	}
	planSchema, err := readSchema(filepath.Join(cfg.Root, "schemas", "acquisition-plan.schema.json"))
	if err != nil {
		return nil, err
	}
	resultSchema, err := readSchema(filepath.Join(cfg.Root, "schemas", "reasoning-result.schema.json"))
	if err != nil {
		return nil, err
	}
	experimentalModelIDs, err := ollamaExperimentalModelIDs(cfg.Reasoning.ExperimentalModelIDs)
	if err != nil {
		return nil, err
	}
	transport, err := ollama.New(ollama.Config{
		BaseURL:                  endpoint,
		Timeout:                  time.Duration(cfg.Reasoning.TimeoutMS) * time.Millisecond,
		MaxRetries:               cfg.Reasoning.MaxRetries,
		KeepAliveSeconds:         cfg.Reasoning.KeepAliveMinutes * 60,
		NumCtx:                   cfg.Reasoning.NumCtx,
		MaxConcurrentInvocations: cfg.Reasoning.OllamaMaxConcurrentInvocations,
		ExperimentalModelIDs:     experimentalModelIDs,
	})
	if err != nil {
		return nil, err
	}
	clients, err := newBoundClientPool(transport)
	if err != nil {
		_ = transport.Close()
		return nil, err
	}
	provider := &Ollama{
		name:          name,
		endpoint:      endpoint,
		maxRetries:    cfg.Reasoning.MaxRetries,
		timeout:       time.Duration(cfg.Reasoning.TimeoutMS) * time.Millisecond,
		warmupTimeout: time.Duration(cfg.Reasoning.WarmupTimeoutMS) * time.Millisecond,
		planning:      cfg.Reasoning.Planning,
		evaluation:    cfg.Reasoning.Evaluation,
		planSchema:    planSchema,
		resultSchema:  resultSchema,
		transport:     transport,
		clients:       clients,
		order:         ollamaProfileOrder,
	}
	return provider, nil
}

func (o *Ollama) Name() string { return o.name }

func (o *Ollama) ProfileOptions() []ProfileOption {
	options := make([]ProfileOption, 0, len(o.order))
	model := o.planning.Model
	modelID, modelErr := ollamaModelID(o.transport.ModelCatalog(), o.planning)
	if modelErr == nil {
		if wire, wireErr := ollamaWireModel(o.transport.ModelCatalog(), modelID); wireErr == nil {
			model = wire
		}
	}
	order := o.order
	if modelErr == nil {
		if descriptor, ok := o.transport.ModelCatalog().Lookup(modelID); ok && descriptor.Maturity == inference.ModelMaturityExperimental {
			// Experimental descriptors own their complete reasoning surface. The
			// SDK currently exposes only xhigh/native think for this model, so do
			// not advertise unsupported lower tiers from the stable UI catalog.
			order = []inference.ProfileID{"deep_reasoning"}
		}
	}
	for _, id := range order {
		effort := map[inference.ProfileID]string{
			"structured_fast": "off", "short_reasoning": "low",
			"general_synthesis": "medium", "deep_reasoning": "high",
		}[id]
		if modelErr == nil {
			if descriptor, ok := o.transport.ModelCatalog().Lookup(modelID); ok && descriptor.Maturity == inference.ModelMaturityExperimental {
				effort = "xhigh"
			}
		}
		options = append(options, ProfileOption{
			ID:     string(id),
			Label:  ollamaProfileLabel(id),
			Model:  model,
			Effort: effort,
		})
	}
	return options
}

func (o *Ollama) ResolveProfile(id string) (config.ModelConfig, bool) {
	for _, option := range o.ProfileOptions() {
		if option.ID == id {
			modelID, err := ollamaModelID(o.transport.ModelCatalog(), o.planning)
			if err != nil {
				return config.ModelConfig{}, false
			}
			return config.ModelConfig{ModelID: modelID, Model: option.Model, MinReasoningTier: option.Effort, ReasoningOptionID: option.Effort, Effort: option.Effort, ProfileID: id}, true
		}
	}
	return config.ModelConfig{}, false
}

// InvokeStructured exposes the shared Ollama transport to bounded adapters
// without adding their domain-specific methods to the reasoning Provider.
func (o *Ollama) InvokeStructured(ctx context.Context, profileID string, prompt string, schema any, model config.ModelConfig) (string, domain.ModelUsage, time.Duration, error) {
	return o.invoke(ctx, inference.ProfileID(profileID), prompt, schema, model)
}

func (o *Ollama) Plan(ctx context.Context, run domain.Run, observation domain.Observation, knowledge []domain.ReasonedItem) (AcquisitionPlan, domain.ReasoningTelemetry, error) {
	return o.PlanWithModel(ctx, run, observation, knowledge, string(ExecutionProfilePlanning), o.planning)
}

func (o *Ollama) PlanWithModel(ctx context.Context, run domain.Run, observation domain.Observation, knowledge []domain.ReasonedItem, profileID string, model config.ModelConfig) (AcquisitionPlan, domain.ReasoningTelemetry, error) {
	raw, usage, duration, err := o.invoke(ctx, inference.ProfileID(profileID), buildPlanningPrompt(run, observation, knowledge), o.planSchema, model)
	telemetry := o.telemetry(run, "acquisition_planning", model, duration, usage, err)
	if err != nil {
		return AcquisitionPlan{}, telemetry, err
	}
	var plan AcquisitionPlan
	if err := json.Unmarshal([]byte(raw), &plan); err != nil {
		return AcquisitionPlan{}, telemetry, fmt.Errorf("decode Ollama acquisition plan: %w", err)
	}
	if plan.Decision != "finish" && plan.Decision != "request_follow_up" {
		return AcquisitionPlan{}, telemetry, fmt.Errorf("invalid acquisition decision %q", plan.Decision)
	}
	return plan, telemetry, nil
}

func (o *Ollama) Analyze(ctx context.Context, run domain.Run, observation domain.Observation, knowledge []domain.ReasonedItem) (domain.ReasoningResult, domain.ReasoningTelemetry, error) {
	return o.AnalyzeWithModel(ctx, run, observation, knowledge, string(ExecutionProfileEvaluation), o.evaluation)
}

func (o *Ollama) AnalyzeWithModel(ctx context.Context, run domain.Run, observation domain.Observation, knowledge []domain.ReasonedItem, profileID string, model config.ModelConfig) (domain.ReasoningResult, domain.ReasoningTelemetry, error) {
	request := buildEvaluationRequest(run, observation, knowledge)
	raw, usage, duration, err := o.invoke(ctx, inference.ProfileID(profileID), request.prompt, o.resultSchema, model)
	telemetry := o.telemetry(run, "candidate_evaluation", model, duration, usage, err)
	if err != nil {
		return domain.ReasoningResult{}, telemetry, err
	}
	var result domain.ReasoningResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return domain.ReasoningResult{}, telemetry, fmt.Errorf("decode Ollama reasoning result: %w", err)
	}
	if err := bindEvidenceKeysByPosition(&result, request.evidenceKeys); err != nil {
		return domain.ReasoningResult{}, telemetry, err
	}
	return result, telemetry, nil
}

func (o *Ollama) invoke(parent context.Context, profileID inference.ProfileID, prompt string, schema any, model config.ModelConfig) (string, domain.ModelUsage, time.Duration, error) {
	modelID, err := ollamaModelID(o.transport.ModelCatalog(), model)
	if err != nil {
		return "", domain.ModelUsage{}, 0, err
	}
	if o.warmupTimeout > 0 {
		wireModel, err := ollamaWireModel(o.transport.ModelCatalog(), modelID)
		if err != nil {
			return "", domain.ModelUsage{}, 0, err
		}
		if _, err := o.transport.Warm(parent, wireModel, o.warmupTimeout); err != nil {
			return "", domain.ModelUsage{}, 0, err
		}
	}
	return invokeBound(parent, o.clients, profileID, prompt, schema, model, modelID)
}

func (o *Ollama) Close() error {
	if o.clients != nil {
		err := o.clients.Close()
		o.clients = nil
		o.transport = nil
		return err
	}
	if o.transport == nil {
		return nil
	}
	return o.transport.Close()
}

func ollamaProfileLabel(id inference.ProfileID) string {
	labels := map[inference.ProfileID]string{
		"structured_fast": "Structured Fast", "short_reasoning": "Short Reasoning",
		"general_synthesis": "General Synthesis", "deep_reasoning": "Deep Reasoning",
	}
	return labels[id]
}

func (o *Ollama) telemetry(run domain.Run, phase string, model config.ModelConfig, duration time.Duration, value domain.ModelUsage, runErr error) domain.ReasoningTelemetry {
	status := "completed"
	if runErr != nil {
		status = "failed"
	}
	modelName := model.Model
	if modelName == "" {
		modelName = value.ProviderModel
	}
	if modelName == "" {
		if stableID, err := ollamaModelID(o.transport.ModelCatalog(), model); err == nil {
			modelName, _ = ollamaWireModel(o.transport.ModelCatalog(), stableID)
		}
	}
	effort := model.Effort
	if effort == "" {
		effort = value.NativeReasoning
	}
	callerLatency := value.CallerLatencyMS
	if callerLatency == 0 {
		callerLatency = duration.Milliseconds()
	}
	return domain.ReasoningTelemetry{ID: domain.NewID("reasoning"), RunID: run.ID, Phase: phase, Provider: o.name, Model: modelName, Effort: effort, ModelDescriptorVersion: value.ModelDescriptorVersion, ModelMaturity: value.ModelMaturity, DurationMS: duration.Milliseconds(), CallerLatencyMS: callerLatency, QueueWaitMS: value.QueueWaitMS, ProviderExecutionMS: value.ProviderExecutionMS, ResponseTotalMS: value.ResponseTotalMS, Status: status, InputTokens: value.Input, CachedInputTokens: value.CachedInput, OutputTokens: value.Output, ReasoningOutputTokens: value.ReasoningOutput, CreatedAt: domain.Now()}
}

func ollamaModelID(catalog inference.ModelCatalog, model config.ModelConfig) (string, error) {
	requested := strings.TrimSpace(model.ModelID)
	if requested == "" {
		requested = strings.TrimSpace(model.Model)
	}
	for _, descriptor := range catalog.List() {
		if requested == descriptor.ModelID || requested == descriptor.ProviderModel {
			return descriptor.ModelID, nil
		}
	}
	return "", fmt.Errorf("unknown Ollama model %q; use a provider-owned configured model ID", requested)
}

func ollamaWireModel(catalog inference.ModelCatalog, modelID string) (string, error) {
	descriptor, ok := catalog.Lookup(modelID)
	if !ok {
		return "", fmt.Errorf("unknown Ollama configured model ID %q", modelID)
	}
	return descriptor.ProviderModel, nil
}

func ollamaExperimentalModelIDs(values []string) ([]ollama.ModelID, error) {
	result := make([]ollama.ModelID, 0, len(values))
	for _, raw := range values {
		requested := strings.TrimSpace(raw)
		matched := false
		for _, id := range ollama.ExperimentalModels() {
			descriptor, ok := ollama.ExperimentalModel(id)
			if ok && (requested == descriptor.ModelID || requested == descriptor.ProviderModel) {
				result = append(result, id)
				matched = true
				break
			}
		}
		if !matched {
			return nil, fmt.Errorf("unknown Ollama experimental model %q; use an SDK-owned experimental model ID", requested)
		}
	}
	return result, nil
}
