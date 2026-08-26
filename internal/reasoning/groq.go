package reasoning

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/credentials"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/ai4u-inference-sdk-go/endpoints/groq"
	"github.com/abangkis/ai4u-inference-sdk-go/inference"
)

const groqDefaultEndpoint = "https://api.groq.com/openai/v1"

var groqProfileOrder = []inference.ProfileID{"groq_low", "groq_medium", "groq_high"}

// Groq composes the SDK Groq Responses adapter into AkuSidecar's four
// structured workloads. The credential is resolved once at this boundary and
// is held only by the in-memory SDK adapter.
type Groq struct {
	endpoint     string
	planning     config.ModelConfig
	evaluation   config.ModelConfig
	planSchema   any
	resultSchema any
	transport    *groq.Adapter
	clients      *boundClientPool
}

func NewGroq(cfg config.Config) (*Groq, error) {
	return newGroq(cfg, credentials.ForRoot(cfg.Root))
}

func newGroq(cfg config.Config, resolver credentials.Resolver) (*Groq, error) {
	if resolver == nil {
		return nil, fmt.Errorf("Groq credential resolver is required")
	}
	apiKey, err := resolver.Resolve(cfg.Reasoning.CredentialRef)
	if err != nil {
		return nil, fmt.Errorf("resolve Groq credential: %w", err)
	}
	endpoint := strings.TrimSpace(cfg.Reasoning.Endpoint)
	if endpoint == "" {
		endpoint = groqDefaultEndpoint
	}
	if cfg.Reasoning.Planning.StableModelID() == "" {
		return nil, fmt.Errorf("Groq planning model is required")
	}
	planSchema, err := readSchema(filepath.Join(cfg.Root, "schemas", "acquisition-plan.schema.json"))
	if err != nil {
		return nil, err
	}
	resultSchema, err := readSchema(filepath.Join(cfg.Root, "schemas", "reasoning-result.schema.json"))
	if err != nil {
		return nil, err
	}
	transport, err := groq.New(groq.Config{
		APIKey: apiKey, BaseURL: endpoint,
		Timeout: time.Duration(cfg.Reasoning.TimeoutMS) * time.Millisecond,
		// The curated conditional-free route intentionally defaults to no retry.
		ConditionalFreeRetryPolicy: groq.RetryPolicy{MaxRetries: cfg.Reasoning.MaxRetries},
	})
	if err != nil {
		return nil, err
	}
	clients, err := newBoundClientPool(transport)
	if err != nil {
		return nil, err
	}
	return &Groq{endpoint: endpoint, planning: cfg.Reasoning.Planning, evaluation: cfg.Reasoning.Evaluation, planSchema: planSchema, resultSchema: resultSchema, transport: transport, clients: clients}, nil
}

func (g *Groq) Name() string { return "groq" }

func (g *Groq) ProfileOptions() []ProfileOption {
	model := string(groq.ModelGPTOSS120B)
	options := make([]ProfileOption, 0, len(groqProfileOrder))
	for _, id := range groqProfileOrder {
		effort := strings.TrimPrefix(string(id), "groq_")
		options = append(options, ProfileOption{ID: string(id), Label: "Groq " + strings.ToUpper(effort[:1]) + effort[1:], Model: model, Effort: effort})
	}
	return options
}

func (g *Groq) ResolveProfile(id string) (config.ModelConfig, bool) {
	for _, option := range g.ProfileOptions() {
		if option.ID == id {
			return config.ModelConfig{ModelID: option.Model, Model: option.Model, MinReasoningTier: option.Effort, ReasoningOptionID: option.Effort, Effort: option.Effort, ProfileID: id}, true
		}
	}
	return config.ModelConfig{}, false
}

func (g *Groq) InvokeStructured(ctx context.Context, profileID string, prompt string, schema any, model config.ModelConfig) (string, domain.ModelUsage, time.Duration, error) {
	return g.invoke(ctx, inference.ProfileID(profileID), prompt, schema, model)
}

func (g *Groq) Plan(ctx context.Context, run domain.Run, observation domain.Observation, knowledge []domain.ReasonedItem) (AcquisitionPlan, domain.ReasoningTelemetry, error) {
	return g.PlanWithModel(ctx, run, observation, knowledge, string(ExecutionProfilePlanning), g.planning)
}

func (g *Groq) PlanWithModel(ctx context.Context, run domain.Run, observation domain.Observation, knowledge []domain.ReasonedItem, profileID string, model config.ModelConfig) (AcquisitionPlan, domain.ReasoningTelemetry, error) {
	raw, usage, duration, err := g.invoke(ctx, inference.ProfileID(profileID), buildPlanningPrompt(run, observation, knowledge), g.planSchema, model)
	telemetry := g.telemetry(run, "acquisition_planning", model, duration, usage, err)
	if err != nil {
		return AcquisitionPlan{}, telemetry, err
	}
	var plan AcquisitionPlan
	if err := json.Unmarshal([]byte(raw), &plan); err != nil {
		return AcquisitionPlan{}, telemetry, fmt.Errorf("decode Groq acquisition plan: %w", err)
	}
	if plan.Decision != "finish" && plan.Decision != "request_follow_up" {
		return AcquisitionPlan{}, telemetry, fmt.Errorf("invalid acquisition decision %q", plan.Decision)
	}
	return plan, telemetry, nil
}

func (g *Groq) Analyze(ctx context.Context, run domain.Run, observation domain.Observation, knowledge []domain.ReasonedItem) (domain.ReasoningResult, domain.ReasoningTelemetry, error) {
	return g.AnalyzeWithModel(ctx, run, observation, knowledge, string(ExecutionProfileEvaluation), g.evaluation)
}

func (g *Groq) AnalyzeWithModel(ctx context.Context, run domain.Run, observation domain.Observation, knowledge []domain.ReasonedItem, profileID string, model config.ModelConfig) (domain.ReasoningResult, domain.ReasoningTelemetry, error) {
	request := buildEvaluationRequest(run, observation, knowledge)
	schema, err := exactCandidateCountSchema(g.resultSchema, len(request.evidenceKeys))
	if err != nil {
		return domain.ReasoningResult{}, g.telemetry(run, "candidate_evaluation", model, 0, domain.ModelUsage{}, err), err
	}
	raw, usage, duration, err := g.invoke(ctx, inference.ProfileID(profileID), request.prompt, schema, model)
	telemetry := g.telemetry(run, "candidate_evaluation", model, duration, usage, err)
	if err != nil {
		return domain.ReasoningResult{}, telemetry, err
	}
	var result domain.ReasoningResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return domain.ReasoningResult{}, telemetry, fmt.Errorf("decode Groq reasoning result: %w", err)
	}
	if err := bindEvidenceKeysByPosition(&result, request.evidenceKeys); err != nil {
		return domain.ReasoningResult{}, telemetry, err
	}
	return result, telemetry, nil
}

func (g *Groq) invoke(ctx context.Context, profileID inference.ProfileID, prompt string, schema any, model config.ModelConfig) (string, domain.ModelUsage, time.Duration, error) {
	modelID := strings.TrimSpace(model.StableModelID())
	if _, ok := g.transport.ModelCatalog().Lookup(modelID); !ok {
		return "", domain.ModelUsage{}, 0, fmt.Errorf("unknown Groq model %q", modelID)
	}
	return invokeBound(ctx, g.clients, profileID, prompt, schema, model, modelID)
}

func (g *Groq) Close() error {
	if g.clients == nil {
		return nil
	}
	err := g.clients.Close()
	g.clients = nil
	g.transport = nil
	return err
}

func (g *Groq) telemetry(run domain.Run, phase string, model config.ModelConfig, duration time.Duration, usage domain.ModelUsage, runErr error) domain.ReasoningTelemetry {
	status := "completed"
	if runErr != nil {
		status = "failed"
	}
	modelName := usage.ProviderModel
	if modelName == "" {
		modelName = model.StableModelID()
	}
	effort := model.MinimumTier()
	if usage.NativeReasoning != "" {
		effort = usage.NativeReasoning
	}
	callerLatency := usage.CallerLatencyMS
	if callerLatency == 0 {
		callerLatency = duration.Milliseconds()
	}
	return domain.ReasoningTelemetry{ID: domain.NewID("reasoning"), RunID: run.ID, Phase: phase, Provider: "groq", Model: modelName, Effort: effort, ModelDescriptorVersion: usage.ModelDescriptorVersion, ModelMaturity: usage.ModelMaturity, DurationMS: duration.Milliseconds(), CallerLatencyMS: callerLatency, QueueWaitMS: usage.QueueWaitMS, ProviderExecutionMS: usage.ProviderExecutionMS, ResponseTotalMS: usage.ResponseTotalMS, Status: status, InputTokens: usage.Input, CachedInputTokens: usage.CachedInput, OutputTokens: usage.Output, ReasoningOutputTokens: usage.ReasoningOutput, CreatedAt: domain.Now()}
}

var _ Provider = (*Groq)(nil)
var _ StructuredInvoker = (*Groq)(nil)
var _ ProfileProvider = (*Groq)(nil)
