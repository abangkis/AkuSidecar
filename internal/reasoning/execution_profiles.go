package reasoning

import (
	"context"
	"encoding/json"
	"errors"
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

func modelMaxOutputTokens(model config.ModelConfig) int {
	if model.MaxOutputTokens > 0 {
		return model.MaxOutputTokens
	}
	return defaultProfileMaxOutputTokens
}

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
		Limits: executionprofile.Limits{MaxOutputTokens: modelMaxOutputTokens(model)},
	})
}

type boundClientPool struct {
	registry  *inference.Registry
	adapter   inference.Adapter
	adapterID string
	clients   map[string]inference.Client
	flights   map[string]*bindingFlight
	mu        sync.Mutex
	closeOnce sync.Once
	closed    bool
	closeErr  error
	flightWG  sync.WaitGroup
}

type bindingFlight struct {
	done   chan struct{}
	client inference.Client
	err    error
}

func newBoundClientPool(adapter inference.Adapter) (*boundClientPool, error) {
	registry, err := inference.NewRegistry(adapter)
	if err != nil {
		return nil, err
	}
	return &boundClientPool{registry: registry, adapter: adapter, adapterID: adapter.ID(), clients: map[string]inference.Client{}, flights: map[string]*bindingFlight{}}, nil
}

func (p *boundClientPool) get(ctx context.Context, profileID inference.ProfileID, model config.ModelConfig, modelID string) (inference.Client, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	profile, err := newExecutionProfile(profileID, model)
	if err != nil {
		return nil, err
	}
	optionID := model.ExactReasoningOption()
	assurance := inference.AssurancePolicy(strings.TrimSpace(model.Assurance))
	key := string(profileID) + "|" + modelID + "|" + optionID + "|" + model.MinimumTier() + "|" + string(assurance)
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil, fmt.Errorf("inference adapter %q is closed", p.adapterID)
	}
	if client, ok := p.clients[key]; ok {
		p.mu.Unlock()
		return client, nil
	}
	if flight, ok := p.flights[key]; ok {
		p.mu.Unlock()
		select {
		case <-flight.done:
			return flight.client, flight.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	flight := &bindingFlight{done: make(chan struct{})}
	p.flights[key] = flight
	p.flightWG.Add(1)
	p.mu.Unlock()

	binding := inference.Binding{
		ID:      "akusidecar-" + strings.ReplaceAll(string(profileID), ".", "-") + "-" + modelID,
		Version: "1", AdapterID: p.adapterID, ModelID: modelID,
		ReasoningOptionID: optionID,
		AssurancePolicy:   assurance,
	}
	client, _, err := p.registry.BindBinding(profile, binding)
	if err != nil {
		p.finishFlight(key, flight, nil, err)
		return nil, err
	}
	if err := inference.Preflight(ctx, client); err != nil {
		closeInferenceClient(client)
		wrapped := fmt.Errorf("preflight %s profile %q model %q: %w", p.adapterID, profileID, modelID, err)
		p.finishFlight(key, flight, nil, wrapped)
		return nil, wrapped
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		closeInferenceClient(client)
		p.finishFlight(key, flight, nil, fmt.Errorf("inference adapter %q is closed", p.adapterID))
		return nil, fmt.Errorf("inference adapter %q is closed", p.adapterID)
	}
	p.clients[key] = client
	p.mu.Unlock()
	p.finishFlight(key, flight, client, nil)
	return client, nil
}

func (p *boundClientPool) finishFlight(key string, flight *bindingFlight, client inference.Client, err error) {
	p.mu.Lock()
	flight.client, flight.err = client, err
	delete(p.flights, key)
	close(flight.done)
	p.mu.Unlock()
	p.flightWG.Done()
}

func closeInferenceClient(client inference.Client) error {
	if closer, ok := client.(interface{ Close() error }); ok {
		return closer.Close()
	}
	return nil
}

// Close releases cached bound-client leases exactly once, then closes the
// composition-root adapter. A bound client's Close never owns the transport.
func (p *boundClientPool) Close() error {
	if p == nil {
		return nil
	}
	p.closeOnce.Do(func() {
		p.mu.Lock()
		p.closed = true
		p.mu.Unlock()
		p.flightWG.Wait()
		p.mu.Lock()
		clients := make([]inference.Client, 0, len(p.clients))
		for key, client := range p.clients {
			clients = append(clients, client)
			delete(p.clients, key)
		}
		p.mu.Unlock()
		for _, client := range clients {
			if err := closeInferenceClient(client); err != nil && p.closeErr == nil {
				p.closeErr = err
			}
		}
		if closer, ok := p.adapter.(interface{ Close() error }); ok {
			if err := closer.Close(); err != nil && p.closeErr == nil {
				p.closeErr = err
			}
		}
	})
	return p.closeErr
}

func invokeBound(ctx context.Context, pool *boundClientPool, profileID inference.ProfileID, prompt string, schema any, model config.ModelConfig, modelID string) (string, domain.ModelUsage, time.Duration, error) {
	started := time.Now()
	client, err := pool.get(ctx, profileID, model, modelID)
	if err != nil {
		latency := time.Since(started)
		return "", domain.ModelUsage{CallerLatencyMS: latency.Milliseconds()}, latency, err
	}
	rawSchema, err := schemaJSON(schema)
	if err != nil {
		latency := time.Since(started)
		return "", domain.ModelUsage{CallerLatencyMS: latency.Milliseconds()}, latency, fmt.Errorf("encode %s response schema: %w", profileID, err)
	}
	response, err := client.Generate(ctx, inference.Request{
		ProfileID: profileID, Workload: string(profileID),
		SystemPrompt: "Return only the requested structured JSON result.", UserPrompt: prompt,
		ResponseFormat:  inference.JSONSchema(string(profileID), responseFormatName(profileID), "AkuSidecar structured result", rawSchema, true),
		MaxOutputTokens: modelMaxOutputTokens(model),
	})
	callerLatency := time.Since(started)
	if err != nil {
		return "", modelUsageFromInferenceFailure(err, callerLatency), callerLatency, err
	}
	if response == nil {
		return "", domain.ModelUsage{CallerLatencyMS: callerLatency.Milliseconds()}, callerLatency, fmt.Errorf("%s returned an empty response", profileID)
	}
	usage := modelUsageFromInferenceEvidence(response.Usage, &response.Receipt, response.Timing, response.DurationMillis, callerLatency)
	return response.Text, usage, callerLatency, nil
}

func modelUsageFromInferenceFailure(err error, callerLatency time.Duration) domain.ModelUsage {
	usage := domain.ModelUsage{CallerLatencyMS: callerLatency.Milliseconds()}
	var failure *inference.Error
	if !errors.As(err, &failure) {
		return usage
	}
	evidence := failure.Usage
	timing := failure.Timing
	if failure.Receipt != nil {
		if evidence == (inference.Usage{}) {
			evidence = failure.Receipt.Usage
		}
		if timing == nil {
			timing = failure.Receipt.Timing
		}
	}
	return modelUsageFromInferenceEvidence(evidence, failure.Receipt, timing, 0, callerLatency)
}

func modelUsageFromInferenceEvidence(evidence inference.Usage, receipt *inference.ExecutionReceipt, timing *inference.ResponseTiming, durationMillis int64, callerLatency time.Duration) domain.ModelUsage {
	usage := domain.ModelUsage{}
	if evidence.InputTokens != 0 {
		value := int64(evidence.InputTokens)
		usage.Input = &value
	}
	if evidence.CachedInputTokens != 0 {
		value := int64(evidence.CachedInputTokens)
		usage.CachedInput = &value
	}
	if evidence.OutputTokens != 0 {
		value := int64(evidence.OutputTokens)
		usage.Output = &value
	}
	if evidence.ReasoningTokens != 0 {
		value := int64(evidence.ReasoningTokens)
		usage.ReasoningOutput = &value
	}
	if receipt != nil {
		usage.ProviderModel = receipt.ActualProviderModel
		if usage.ProviderModel == "" {
			usage.ProviderModel = receipt.ProviderModel
		}
		usage.NativeReasoning = receipt.NativeReasoningValue
		usage.ReasoningTier = string(receipt.ReasoningTier)
		usage.ModelDescriptorVersion = receipt.ModelDescriptorVersion
		usage.ModelMaturity = string(receipt.ModelMaturity)
	}
	providerExecution := time.Duration(durationMillis) * time.Millisecond
	responseTotal := providerExecution
	queueWait := time.Duration(0)
	if timing != nil {
		queueWait = timing.QueueWait
		if timing.ProviderExecution > 0 {
			providerExecution = timing.ProviderExecution
		}
		if timing.Total > 0 {
			responseTotal = timing.Total
		}
	}
	if responseTotal <= 0 {
		responseTotal = providerExecution
	}
	usage.CallerLatencyMS = callerLatency.Milliseconds()
	usage.QueueWaitMS = queueWait.Milliseconds()
	usage.ProviderExecutionMS = providerExecution.Milliseconds()
	usage.ResponseTotalMS = responseTotal.Milliseconds()
	return usage
}

// responseFormatName projects the stable workload ID into the conservative
// schema-name vocabulary accepted by provider APIs. SchemaID remains unchanged
// and continues to carry the dotted application identity.
func responseFormatName(profileID inference.ProfileID) string {
	value := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			return r
		default:
			return '-'
		}
	}, string(profileID))
	if len(value) > 64 {
		value = value[:64]
	}
	return value
}

func exactCandidateCountSchema(schema any, candidateCount int) (any, error) {
	if candidateCount < 1 || candidateCount > maxEvaluationCandidates {
		return nil, fmt.Errorf("structured evaluation requires between 1 and %d candidates, got %d", maxEvaluationCandidates, candidateCount)
	}
	raw, err := json.Marshal(schema)
	if err != nil {
		return nil, fmt.Errorf("clone evaluation schema: %w", err)
	}
	var projected map[string]any
	if err := json.Unmarshal(raw, &projected); err != nil {
		return nil, fmt.Errorf("decode evaluation schema: %w", err)
	}
	properties, ok := projected["properties"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("evaluation schema has no properties object")
	}
	for _, field := range []string{"items", "candidateAssessments"} {
		arraySchema, ok := properties[field].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("evaluation schema field %q is not an array schema", field)
		}
		arraySchema["minItems"] = candidateCount
		arraySchema["maxItems"] = candidateCount
	}
	return projected, nil
}

func schemaJSON(schema any) ([]byte, error) {
	switch value := schema.(type) {
	case json.RawMessage:
		return value, nil
	case *json.RawMessage:
		if value == nil {
			return nil, fmt.Errorf("schema is nil")
		}
		return *value, nil
	default:
		return json.Marshal(schema)
	}
}
