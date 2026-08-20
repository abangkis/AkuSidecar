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
	inference.ProfileStructuredFast,
	inference.ProfileShortReasoning,
	inference.ProfileGeneralSynthesis,
	inference.ProfileDeepReasoning,
}

// ollamaBindings maps each built-in execution profile to a native Ollama
// thinking budget. The adapter's own native-to-tier claims make the map
// satisfiable; inference.Resolve verifies the binding at profile resolution.
func ollamaBindings() map[inference.ProfileID]inference.Binding {
	return map[inference.ProfileID]inference.Binding{
		inference.ProfileStructuredFast:   {Adapter: "ollama", Capability: string(ollama.ThinkOff)},
		inference.ProfileShortReasoning:   {Adapter: "ollama", Capability: string(ollama.ThinkLow)},
		inference.ProfileGeneralSynthesis: {Adapter: "ollama", Capability: string(ollama.ThinkMedium)},
		inference.ProfileDeepReasoning:    {Adapter: "ollama", Capability: string(ollama.ThinkHigh)},
	}
}

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
	model         string
	planning      config.ModelConfig
	evaluation    config.ModelConfig
	planSchema    any
	resultSchema  any
	transport     *ollama.Adapter
	caps          inference.ProviderCapabilities
	bindings      map[inference.ProfileID]inference.Binding
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
	model := strings.TrimSpace(cfg.Reasoning.Planning.Model)
	if model == "" {
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
	transport, err := ollama.New(ollama.Config{
		BaseURL:          endpoint,
		Timeout:          time.Duration(cfg.Reasoning.TimeoutMS) * time.Millisecond,
		MaxRetries:       cfg.Reasoning.MaxRetries,
		KeepAliveSeconds: cfg.Reasoning.KeepAliveMinutes * 60,
		NumCtx:           cfg.Reasoning.NumCtx,
	})
	if err != nil {
		return nil, err
	}
	provider := &Ollama{
		name:          name,
		endpoint:      endpoint,
		maxRetries:    cfg.Reasoning.MaxRetries,
		timeout:       time.Duration(cfg.Reasoning.TimeoutMS) * time.Millisecond,
		warmupTimeout: time.Duration(cfg.Reasoning.WarmupTimeoutMS) * time.Millisecond,
		model:         model,
		planning:      cfg.Reasoning.Planning,
		evaluation:    cfg.Reasoning.Evaluation,
		planSchema:    planSchema,
		resultSchema:  resultSchema,
		transport:     transport,
		caps:          transport.Capabilities(),
		bindings:      ollamaBindings(),
		order:         ollamaProfileOrder,
	}
	return provider, nil
}

func (o *Ollama) Name() string { return o.name }

func (o *Ollama) ProfileOptions() []ProfileOption {
	options := make([]ProfileOption, 0, len(o.order))
	for _, id := range o.order {
		binding := o.bindings[id]
		options = append(options, ProfileOption{
			ID:     string(id),
			Label:  ollamaProfileLabel(id),
			Model:  o.model,
			Effort: binding.Capability,
		})
	}
	return options
}

func (o *Ollama) ResolveProfile(id string) (config.ModelConfig, bool) {
	profile := inference.ProfileID(id)
	binding, ok := o.bindings[profile]
	if !ok {
		return config.ModelConfig{}, false
	}
	spec, ok := inference.ProfileSpec(profile)
	if !ok {
		return config.ModelConfig{}, false
	}
	binding.Model = o.model
	resolved, err := inference.Resolve(spec, binding, o.caps)
	if err != nil {
		return config.ModelConfig{}, false
	}
	return config.ModelConfig{Model: resolved.Model, Effort: resolved.Native}, true
}

// InvokeStructured exposes the shared Ollama transport to bounded adapters
// without adding their domain-specific methods to the reasoning Provider.
func (o *Ollama) InvokeStructured(ctx context.Context, prompt string, schema any, model config.ModelConfig) (string, domain.ModelUsage, time.Duration, error) {
	raw, value, duration, err := o.invoke(ctx, prompt, schema, model)
	return raw, domain.ModelUsage{Input: value.Input, CachedInput: value.CachedInput, Output: value.Output, ReasoningOutput: value.ReasoningOutput}, duration, err
}

func (o *Ollama) Plan(ctx context.Context, run domain.Run, observation domain.Observation, knowledge []domain.ReasonedItem) (AcquisitionPlan, domain.ReasoningTelemetry, error) {
	return o.PlanWithModel(ctx, run, observation, knowledge, o.planning)
}

func (o *Ollama) PlanWithModel(ctx context.Context, run domain.Run, observation domain.Observation, knowledge []domain.ReasonedItem, model config.ModelConfig) (AcquisitionPlan, domain.ReasoningTelemetry, error) {
	raw, usage, duration, err := o.invoke(ctx, buildPlanningPrompt(run, observation, knowledge), o.planSchema, model)
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
	return o.AnalyzeWithModel(ctx, run, observation, knowledge, o.evaluation)
}

func (o *Ollama) AnalyzeWithModel(ctx context.Context, run domain.Run, observation domain.Observation, knowledge []domain.ReasonedItem, model config.ModelConfig) (domain.ReasoningResult, domain.ReasoningTelemetry, error) {
	request := buildEvaluationRequest(run, observation, knowledge)
	raw, usage, duration, err := o.invoke(ctx, request.prompt, o.resultSchema, model)
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

func (o *Ollama) invoke(parent context.Context, prompt string, schema any, model config.ModelConfig) (string, ollama.Usage, time.Duration, error) {
	if o.warmupTimeout > 0 {
		if _, err := o.transport.Warm(parent, model.Model, o.warmupTimeout); err != nil {
			return "", ollama.Usage{}, 0, err
		}
	}
	return o.transport.InvokeStructured(parent, prompt, schema, ollama.ModelConfig{
		Model: model.Model,
		Think: ollama.ThinkBudget(model.Effort),
	})
}

func (o *Ollama) Close() error {
	if o.transport == nil {
		return nil
	}
	return o.transport.Close()
}

func ollamaProfileLabel(id inference.ProfileID) string {
	labels := map[inference.ProfileID]string{
		inference.ProfileStructuredFast:   "Structured Fast",
		inference.ProfileShortReasoning:   "Short Reasoning",
		inference.ProfileGeneralSynthesis: "General Synthesis",
		inference.ProfileDeepReasoning:    "Deep Reasoning",
	}
	return labels[id]
}

func (o *Ollama) telemetry(run domain.Run, phase string, model config.ModelConfig, duration time.Duration, value ollama.Usage, runErr error) domain.ReasoningTelemetry {
	status := "completed"
	if runErr != nil {
		status = "failed"
	}
	return domain.ReasoningTelemetry{ID: domain.NewID("reasoning"), RunID: run.ID, Phase: phase, Provider: o.name, Model: model.Model, Effort: model.Effort, DurationMS: duration.Milliseconds(), Status: status, InputTokens: value.Input, CachedInputTokens: value.CachedInput, OutputTokens: value.Output, ReasoningOutputTokens: value.ReasoningOutput, CreatedAt: domain.Now()}
}
