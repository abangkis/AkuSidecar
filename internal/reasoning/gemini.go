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
	"github.com/abangkis/ai4u-inference-sdk-go/endpoints/gemini"
	"github.com/abangkis/ai4u-inference-sdk-go/inference"
)

const geminiDefaultEndpoint = "https://generativelanguage.googleapis.com/v1"

// Gemini Interactions structured output currently rejects candidate-evaluation
// requests with seven effective candidates, while the same shape succeeds for
// six. Keep this adapter boundary local to Gemini; other providers retain the
// full bounded candidate set and their existing request contract.
const geminiCandidateEvaluationChunkSize = 6

// Gemini composes the SDK's native stateless Interactions adapter. Provider
// schema projection is always followed by validation against the complete
// application schema, so unsupported wire constraints are never silently lost.
type Gemini struct {
	name         string
	endpoint     string
	planning     config.ModelConfig
	evaluation   config.ModelConfig
	planSchema   any
	resultSchema any
	transport    *gemini.Adapter
	clients      *boundClientPool
}

func NewGemini(cfg config.Config) (*Gemini, error) {
	return newGemini(cfg, credentials.ForRuntime(cfg.Root, cfg.Dev))
}

func newGemini(cfg config.Config, resolver credentials.Resolver) (*Gemini, error) {
	if resolver == nil {
		return nil, fmt.Errorf("Gemini credential resolver is required")
	}
	apiKey, err := resolver.Resolve(cfg.Reasoning.CredentialRef)
	if err != nil {
		return nil, fmt.Errorf("resolve Gemini credential: %w", err)
	}
	name := strings.TrimSpace(cfg.Reasoning.Provider)
	if name == "" {
		name = "gemini"
	}
	endpoint := strings.TrimSpace(cfg.Reasoning.Endpoint)
	if endpoint == "" {
		endpoint = geminiDefaultEndpoint
	}
	if cfg.Reasoning.Planning.StableModelID() == "" {
		return nil, fmt.Errorf("Gemini planning model is required")
	}
	planSchema, err := readSchema(filepath.Join(cfg.Root, "schemas", "acquisition-plan.schema.json"))
	if err != nil {
		return nil, err
	}
	resultSchema, err := readSchema(filepath.Join(cfg.Root, "schemas", "reasoning-result.schema.json"))
	if err != nil {
		return nil, err
	}
	transport, err := gemini.New(gemini.Config{
		APIKey: apiKey, BaseURL: endpoint,
		Timeout:                    time.Duration(cfg.Reasoning.TimeoutMS) * time.Millisecond,
		ConditionalFreeRetryPolicy: gemini.RetryPolicy{MaxRetries: cfg.Reasoning.MaxRetries},
	})
	if err != nil {
		return nil, err
	}
	clients, err := newBoundClientPool(transport)
	if err != nil {
		return nil, err
	}
	return &Gemini{name: name, endpoint: endpoint, planning: cfg.Reasoning.Planning, evaluation: cfg.Reasoning.Evaluation, planSchema: planSchema, resultSchema: resultSchema, transport: transport, clients: clients}, nil
}

func (g *Gemini) Name() string { return g.name }

func (g *Gemini) ProfileOptions() []ProfileOption {
	modelID := g.planning.StableModelID()
	descriptor, ok := g.transport.ModelCatalog().Lookup(modelID)
	if !ok {
		return nil
	}
	options := make([]ProfileOption, 0, len(descriptor.Capabilities.Reasoning))
	for _, reasoning := range descriptor.Capabilities.Reasoning {
		effort := reasoning.ID
		label := "Gemini " + strings.ToUpper(effort[:1]) + effort[1:]
		options = append(options, ProfileOption{ID: "gemini_" + effort, Label: label, Model: modelID, Effort: effort})
	}
	return options
}

func (g *Gemini) ResolveProfile(id string) (config.ModelConfig, bool) {
	for _, option := range g.ProfileOptions() {
		if option.ID == id {
			return config.ModelConfig{ModelID: option.Model, Model: option.Model, MinReasoningTier: option.Effort, ReasoningOptionID: option.Effort, Effort: option.Effort, ProfileID: id}, true
		}
	}
	return config.ModelConfig{}, false
}

func (g *Gemini) InvokeStructured(ctx context.Context, profileID string, prompt string, schema any, model config.ModelConfig) (string, domain.ModelUsage, time.Duration, error) {
	return g.invoke(ctx, inference.ProfileID(profileID), prompt, schema, model)
}

func (g *Gemini) Plan(ctx context.Context, run domain.Run, observation domain.Observation, knowledge []domain.ReasonedItem) (AcquisitionPlan, domain.ReasoningTelemetry, error) {
	return g.PlanWithModel(ctx, run, observation, knowledge, string(ExecutionProfilePlanning), g.planning)
}

func (g *Gemini) PlanWithModel(ctx context.Context, run domain.Run, observation domain.Observation, knowledge []domain.ReasonedItem, profileID string, model config.ModelConfig) (AcquisitionPlan, domain.ReasoningTelemetry, error) {
	raw, usage, duration, err := g.invoke(ctx, inference.ProfileID(profileID), buildPlanningPrompt(run, observation, knowledge), g.planSchema, model)
	telemetry := g.telemetry(run, "acquisition_planning", model, duration, usage, err)
	if err != nil {
		return AcquisitionPlan{}, telemetry, err
	}
	var plan AcquisitionPlan
	if err := json.Unmarshal([]byte(raw), &plan); err != nil {
		return AcquisitionPlan{}, telemetry, fmt.Errorf("decode Gemini acquisition plan: %w", err)
	}
	if plan.Decision != "finish" && plan.Decision != "request_follow_up" {
		return AcquisitionPlan{}, telemetry, fmt.Errorf("invalid acquisition decision %q", plan.Decision)
	}
	return plan, telemetry, nil
}

func (g *Gemini) Analyze(ctx context.Context, run domain.Run, observation domain.Observation, knowledge []domain.ReasonedItem) (domain.ReasoningResult, domain.ReasoningTelemetry, error) {
	return g.AnalyzeWithModel(ctx, run, observation, knowledge, string(ExecutionProfileEvaluation), g.evaluation)
}

func (g *Gemini) AnalyzeWithModel(ctx context.Context, run domain.Run, observation domain.Observation, knowledge []domain.ReasonedItem, profileID string, model config.ModelConfig) (domain.ReasoningResult, domain.ReasoningTelemetry, error) {
	compact, evidenceKeys := evaluationPromptObservation(observation)
	if len(evidenceKeys) == 0 {
		request := buildEvaluationRequestFromCompact(run, observation, compact, evidenceKeys, knowledge)
		err := fmt.Errorf("structured evaluation requires between 1 and %d candidates, got %d", maxEvaluationCandidates, len(request.evidenceKeys))
		return domain.ReasoningResult{}, g.telemetry(run, "candidate_evaluation", model, 0, domain.ModelUsage{}, err), err
	}

	var merged domain.ReasoningResult
	aggregateUsage := domain.ModelUsage{}
	var aggregateDuration time.Duration
	chunkCount := (len(evidenceKeys) + geminiCandidateEvaluationChunkSize - 1) / geminiCandidateEvaluationChunkSize
	for chunkIndex, start := 0, 0; start < len(evidenceKeys); chunkIndex, start = chunkIndex+1, start+geminiCandidateEvaluationChunkSize {
		end := start + geminiCandidateEvaluationChunkSize
		if end > len(evidenceKeys) {
			end = len(evidenceKeys)
		}
		chunkObservation := compact
		chunkObservation.Candidates = append([]evaluationCandidate(nil), compact.Candidates[start:end]...)
		request := buildEvaluationRequestFromCompactForProvider(promptProviderGemini, run, observation, chunkObservation, evidenceKeys[start:end], knowledge)
		schema, err := exactCandidateCountSchema(g.resultSchema, len(request.evidenceKeys))
		if err != nil {
			wrapped := fmt.Errorf("Gemini candidate evaluation chunk %d/%d: %w", chunkIndex+1, chunkCount, err)
			return domain.ReasoningResult{}, g.telemetry(run, "candidate_evaluation", model, aggregateDuration, aggregateUsage, wrapped), wrapped
		}
		raw, usage, duration, err := g.invoke(ctx, inference.ProfileID(profileID), request.prompt, schema, model)
		aggregateModelUsage(&aggregateUsage, usage)
		aggregateDuration += duration
		if err != nil {
			wrapped := fmt.Errorf("Gemini candidate evaluation chunk %d/%d: %w", chunkIndex+1, chunkCount, err)
			return domain.ReasoningResult{}, g.telemetry(run, "candidate_evaluation", model, aggregateDuration, aggregateUsage, wrapped), wrapped
		}
		var chunkResult domain.ReasoningResult
		if err := json.Unmarshal([]byte(raw), &chunkResult); err != nil {
			wrapped := fmt.Errorf("decode Gemini reasoning result for chunk %d/%d: %w", chunkIndex+1, chunkCount, err)
			return domain.ReasoningResult{}, g.telemetry(run, "candidate_evaluation", model, aggregateDuration, aggregateUsage, wrapped), wrapped
		}
		if err := bindEvidenceKeysByPosition(&chunkResult, request.evidenceKeys); err != nil {
			wrapped := fmt.Errorf("bind Gemini reasoning result for chunk %d/%d: %w", chunkIndex+1, chunkCount, err)
			return domain.ReasoningResult{}, g.telemetry(run, "candidate_evaluation", model, aggregateDuration, aggregateUsage, wrapped), wrapped
		}
		mergeGeminiEvaluationResult(&merged, chunkResult)
	}
	return merged, g.telemetry(run, "candidate_evaluation", model, aggregateDuration, aggregateUsage, nil), nil
}

func mergeGeminiEvaluationResult(dst *domain.ReasoningResult, src domain.ReasoningResult) {
	dst.Items = append(dst.Items, src.Items...)
	dst.CandidateAssessments = append(dst.CandidateAssessments, src.CandidateAssessments...)
	dst.Summary = mergeGeminiText(dst.Summary, src.Summary)
	dst.Limitations = appendUniqueGeminiStrings(dst.Limitations, src.Limitations...)
	dst.RepeatedClaimsCollapsed += src.RepeatedClaimsCollapsed
	dst.DeferredByBudget += src.DeferredByBudget
}

func mergeGeminiText(existing, next string) string {
	existing, next = strings.TrimSpace(existing), strings.TrimSpace(next)
	switch {
	case existing == "":
		return next
	case next == "" || next == existing:
		return existing
	default:
		return existing + "\n\n" + next
	}
}

func appendUniqueGeminiStrings(values []string, additions ...string) []string {
	seen := make(map[string]struct{}, len(values)+len(additions))
	for _, value := range values {
		seen[value] = struct{}{}
	}
	for _, value := range additions {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		values = append(values, value)
	}
	return values
}

func aggregateModelUsage(total *domain.ModelUsage, next domain.ModelUsage) {
	total.Input = sumModelUsageCounter(total.Input, next.Input)
	total.CachedInput = sumModelUsageCounter(total.CachedInput, next.CachedInput)
	total.Output = sumModelUsageCounter(total.Output, next.Output)
	total.ReasoningOutput = sumModelUsageCounter(total.ReasoningOutput, next.ReasoningOutput)
	total.CallerLatencyMS += next.CallerLatencyMS
	total.QueueWaitMS += next.QueueWaitMS
	total.ProviderExecutionMS += next.ProviderExecutionMS
	total.ResponseTotalMS += next.ResponseTotalMS
	if total.ProviderModel == "" {
		total.ProviderModel = next.ProviderModel
	}
	if total.NativeReasoning == "" {
		total.NativeReasoning = next.NativeReasoning
	}
	if total.ReasoningTier == "" {
		total.ReasoningTier = next.ReasoningTier
	}
	if total.ModelDescriptorVersion == "" {
		total.ModelDescriptorVersion = next.ModelDescriptorVersion
	}
	if total.ModelMaturity == "" {
		total.ModelMaturity = next.ModelMaturity
	}
}

func sumModelUsageCounter(existing, next *int64) *int64 {
	if existing == nil && next == nil {
		return nil
	}
	var total int64
	if existing != nil {
		total += *existing
	}
	if next != nil {
		total += *next
	}
	return &total
}

func (g *Gemini) invoke(ctx context.Context, profileID inference.ProfileID, prompt string, schema any, model config.ModelConfig) (string, domain.ModelUsage, time.Duration, error) {
	modelID := strings.TrimSpace(model.StableModelID())
	if _, ok := g.transport.ModelCatalog().Lookup(modelID); !ok {
		return "", domain.ModelUsage{}, 0, fmt.Errorf("unknown Gemini model %q", modelID)
	}
	projected, err := projectGeminiSchema(schema)
	if err != nil {
		return "", domain.ModelUsage{}, 0, err
	}
	raw, usage, duration, err := invokeBound(ctx, g.clients, profileID, prompt, projected, model, modelID)
	if err != nil {
		return "", usage, duration, err
	}
	fullSchema, err := schemaJSON(schema)
	if err != nil {
		return "", usage, duration, fmt.Errorf("encode complete Sidecar schema: %w", err)
	}
	if err := inference.ValidateJSONSchemaResponse(raw, fullSchema); err != nil {
		return "", usage, duration, fmt.Errorf("validate Gemini response against complete Sidecar schema: %w", err)
	}
	return raw, usage, duration, nil
}

var geminiUnsupportedWireKeywords = map[string]bool{
	"$schema": true, "pattern": true, "minLength": true, "maxLength": true,
	// Gemini documents maxItems support, but its Interactions v1 endpoint
	// rejects the Semantic Event schema when decisions.maxItems is 20. The
	// complete Sidecar schema still enforces the exact candidate count locally.
	"maxItems": true,
}

func projectGeminiSchema(schema any) (any, error) {
	raw, err := schemaJSON(schema)
	if err != nil {
		return nil, fmt.Errorf("encode Gemini schema: %w", err)
	}
	var projected any
	if err := json.Unmarshal(raw, &projected); err != nil {
		return nil, fmt.Errorf("decode Gemini schema: %w", err)
	}
	projectGeminiSchemaNode(projected)
	return projected, nil
}

func projectGeminiSchemaNode(value any) {
	switch node := value.(type) {
	case map[string]any:
		for key, child := range node {
			if geminiUnsupportedWireKeywords[key] {
				delete(node, key)
				continue
			}
			projectGeminiSchemaNode(child)
		}
	case []any:
		for _, child := range node {
			projectGeminiSchemaNode(child)
		}
	}
}

func (g *Gemini) Close() error {
	if g.clients == nil {
		return nil
	}
	err := g.clients.Close()
	g.clients = nil
	g.transport = nil
	return err
}

func (g *Gemini) telemetry(run domain.Run, phase string, model config.ModelConfig, duration time.Duration, usage domain.ModelUsage, runErr error) domain.ReasoningTelemetry {
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
	return domain.ReasoningTelemetry{ID: domain.NewID("reasoning"), RunID: run.ID, Phase: phase, Provider: g.name, Model: modelName, Effort: effort, ModelDescriptorVersion: usage.ModelDescriptorVersion, ModelMaturity: usage.ModelMaturity, DurationMS: duration.Milliseconds(), CallerLatencyMS: callerLatency, QueueWaitMS: usage.QueueWaitMS, ProviderExecutionMS: usage.ProviderExecutionMS, ResponseTotalMS: usage.ResponseTotalMS, Status: status, InputTokens: usage.Input, CachedInputTokens: usage.CachedInput, OutputTokens: usage.Output, ReasoningOutputTokens: usage.ReasoningOutput, CreatedAt: domain.Now()}
}

var _ Provider = (*Gemini)(nil)
var _ StructuredInvoker = (*Gemini)(nil)
var _ ProfileProvider = (*Gemini)(nil)
