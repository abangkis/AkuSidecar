package reasoning

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	executionprofile "github.com/abangkis/ai4u-common-execution-profile-go"
	"github.com/abangkis/ai4u-inference-sdk-go/inference"
)

// These are client-owned workload identities. Provider profile IDs such as
// luna_high remain UI/binding projections and are never capability authority.
const (
	ExecutionProfilePlanning    inference.ProfileID = "akusidecar.planning"
	ExecutionProfileEvaluation  inference.ProfileID = "akusidecar.candidate_evaluation"
	ExecutionProfileSemantic    inference.ProfileID = "akusidecar.semantic_event_resolution"
	ExecutionProfileAIDetection inference.ProfileID = "akusidecar.ai_detection"
	ExecutionProfileVersion                         = "1"
)

const defaultProfileMaxOutputTokens = 16384

func newExecutionProfile(id inference.ProfileID, model config.ModelConfig) (inference.ExecutionProfile, error) {
	tier := strings.TrimSpace(model.MinimumTier())
	if tier == "" {
		return inference.ExecutionProfile{}, fmt.Errorf("minimum reasoning tier is required for execution profile %q", id)
	}
	return executionprofile.New(executionprofile.Spec{
		ID: id, Version: ExecutionProfileVersion,
		Requirements: executionprofile.Requirements{
			MinReasoningTier: executionprofile.ReasoningTier(tier),
			OutputFormat:     executionprofile.OutputJSONSchema,
		},
		Limits: executionprofile.Limits{MaxOutputTokens: defaultProfileMaxOutputTokens},
	})
}

type boundClientPool struct {
	registry  *inference.Registry
	adapterID string
	clients   map[string]inference.Client
	mu        sync.Mutex
}

func newBoundClientPool(adapter inference.Adapter) (*boundClientPool, error) {
	registry, err := inference.NewRegistry(adapter)
	if err != nil {
		return nil, err
	}
	return &boundClientPool{registry: registry, adapterID: adapter.ID(), clients: map[string]inference.Client{}}, nil
}

func (p *boundClientPool) get(ctx context.Context, profileID inference.ProfileID, model config.ModelConfig, modelID string) (inference.Client, error) {
	profile, err := newExecutionProfile(profileID, model)
	if err != nil {
		return nil, err
	}
	optionID := model.ExactReasoningOption()
	key := string(profileID) + "|" + modelID + "|" + optionID + "|" + model.MinimumTier()
	p.mu.Lock()
	if client, ok := p.clients[key]; ok {
		p.mu.Unlock()
		return client, nil
	}
	p.mu.Unlock()

	binding := inference.Binding{
		ID:      "akusidecar-" + strings.ReplaceAll(string(profileID), ".", "-") + "-" + modelID,
		Version: "1", AdapterID: p.adapterID, ModelID: modelID,
		ReasoningOptionID: optionID,
	}
	client, _, err := p.registry.BindBinding(profile, binding)
	if err != nil {
		return nil, err
	}
	if err := inference.Preflight(ctx, client); err != nil {
		if closer, ok := client.(interface{ Close() error }); ok {
			_ = closer.Close()
		}
		return nil, fmt.Errorf("preflight %s profile %q model %q: %w", p.adapterID, profileID, modelID, err)
	}
	p.mu.Lock()
	if existing, ok := p.clients[key]; ok {
		p.mu.Unlock()
		if closer, ok := client.(interface{ Close() error }); ok {
			_ = closer.Close()
		}
		return existing, nil
	}
	p.clients[key] = client
	p.mu.Unlock()
	return client, nil
}

func invokeBound(ctx context.Context, pool *boundClientPool, profileID inference.ProfileID, prompt string, schema any, model config.ModelConfig, modelID string) (string, domain.ModelUsage, time.Duration, error) {
	client, err := pool.get(ctx, profileID, model, modelID)
	if err != nil {
		return "", domain.ModelUsage{}, 0, err
	}
	rawSchema, err := json.Marshal(schema)
	if err != nil {
		return "", domain.ModelUsage{}, 0, fmt.Errorf("encode %s response schema: %w", profileID, err)
	}
	started := time.Now()
	response, err := client.Generate(ctx, inference.Request{
		ProfileID: profileID, Workload: string(profileID),
		SystemPrompt: "Return only the requested structured JSON result.", UserPrompt: prompt,
		ResponseFormat:  inference.JSONSchema(string(profileID), string(profileID), "AkuSidecar structured result", rawSchema, true),
		MaxOutputTokens: defaultProfileMaxOutputTokens,
	})
	if err != nil {
		return "", domain.ModelUsage{}, time.Since(started), err
	}
	usage := domain.ModelUsage{}
	if response.Usage.InputTokens != 0 {
		value := int64(response.Usage.InputTokens)
		usage.Input = &value
	}
	if response.Usage.CachedInputTokens != 0 {
		value := int64(response.Usage.CachedInputTokens)
		usage.CachedInput = &value
	}
	if response.Usage.OutputTokens != 0 {
		value := int64(response.Usage.OutputTokens)
		usage.Output = &value
	}
	if response.Usage.ReasoningTokens != 0 {
		value := int64(response.Usage.ReasoningTokens)
		usage.ReasoningOutput = &value
	}
	usage.ProviderModel = response.Receipt.ActualProviderModel
	if usage.ProviderModel == "" {
		usage.ProviderModel = response.Receipt.ProviderModel
	}
	usage.NativeReasoning = response.Receipt.NativeReasoningValue
	usage.ReasoningTier = string(response.Receipt.ReasoningTier)
	duration := time.Duration(response.DurationMillis) * time.Millisecond
	if duration <= 0 {
		duration = time.Since(started)
	}
	return response.Text, usage, duration, nil
}
