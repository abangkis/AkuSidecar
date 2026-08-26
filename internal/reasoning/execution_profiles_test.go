package reasoning

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/ai4u-inference-sdk-go/inference"
)

type poolTestAdapter struct {
	bindCalls      atomic.Int32
	preflightCalls atomic.Int32
	closeCalls     atomic.Int32
	failPreflight  atomic.Int32
	preflightStart chan struct{}
	release        chan struct{}
	client         *poolTestClient
	catalog        inference.ModelCatalog
	responseTiming *inference.ResponseTiming
	durationMillis int64
}

type poolTestClient struct {
	adapter     *poolTestAdapter
	closeOnce   sync.Once
	closeCalls  atomic.Int32
	generateErr error
	usage       inference.Usage
	receipt     inference.ExecutionReceipt
}

func newPoolTestAdapter() *poolTestAdapter {
	catalog, err := inference.NewModelCatalog(inference.ModelDescriptor{
		ModelID: "test-model", ProviderModel: "test-model", Version: "1",
		Source: inference.CapabilitySourceDeclared,
		Capabilities: inference.ModelCapabilities{
			StructuredOutput: true,
			OutputFormats:    []inference.ResponseFormatType{inference.ResponseFormatJSONSchema},
			Assurance:        []inference.AssuranceMode{inference.AssuranceModeNative},
			Reasoning:        []inference.ReasoningOption{{ID: "high", NativeValue: "high", Tier: inference.ReasoningEffortHigh, Source: inference.TierSourceAdapter, Provenance: string(inference.TierSourceAdapter)}},
		},
	})
	if err != nil {
		panic(err)
	}
	adapter := &poolTestAdapter{preflightStart: make(chan struct{}), release: make(chan struct{}), catalog: catalog}
	adapter.client = &poolTestClient{adapter: adapter}
	return adapter
}

func (a *poolTestAdapter) ID() string                           { return "pool-test" }
func (a *poolTestAdapter) Version() string                      { return "1" }
func (a *poolTestAdapter) ModelCatalog() inference.ModelCatalog { return a.catalog }
func (a *poolTestAdapter) BindResolved(inference.ResolvedBinding) (inference.Client, error) {
	a.bindCalls.Add(1)
	return a.client, nil
}
func (a *poolTestAdapter) Close() error { a.closeCalls.Add(1); return nil }

func (a *poolTestAdapter) preflight(ctx context.Context) error {
	a.preflightCalls.Add(1)
	select {
	case <-a.preflightStart:
	default:
		close(a.preflightStart)
	}
	if a.failPreflight.Swap(0) != 0 {
		return errors.New("temporary preflight failure")
	}
	select {
	case <-a.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *poolTestClient) Preflight(ctx context.Context) error { return c.adapter.preflight(ctx) }
func (c *poolTestClient) Generate(context.Context, inference.Request) (*inference.Response, error) {
	if c.generateErr != nil {
		return nil, c.generateErr
	}
	response := &inference.Response{Text: "{}", DurationMillis: c.adapter.durationMillis, Timing: c.adapter.responseTiming, Usage: c.usage, Receipt: c.receipt}
	return response, nil
}
func (c *poolTestClient) Close() error {
	c.closeOnce.Do(func() { c.closeCalls.Add(1) })
	return nil
}

func poolTestModel() config.ModelConfig {
	return config.ModelConfig{ModelID: "test-model", MinReasoningTier: "high", ReasoningOptionID: "high"}
}

func TestBoundClientPoolSingleflightWaiterCancellationAndRetry(t *testing.T) {
	adapter := newPoolTestAdapter()
	pool, err := newBoundClientPool(adapter)
	if err != nil {
		t.Fatal(err)
	}
	profile := ExecutionProfileEvaluation
	model := poolTestModel()
	leaderDone := make(chan error, 1)
	go func() {
		_, err := pool.get(context.Background(), profile, model, "test-model")
		leaderDone <- err
	}()
	select {
	case <-adapter.preflightStart:
	case <-time.After(time.Second):
		t.Fatal("leader did not reach preflight")
	}
	waiterCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := pool.get(waiterCtx, profile, model, "test-model"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waiter error=%v, want cancellation", err)
	}
	close(adapter.release)
	if err := <-leaderDone; err != nil {
		t.Fatal(err)
	}
	if adapter.bindCalls.Load() != 1 || adapter.preflightCalls.Load() != 1 {
		t.Fatalf("bind=%d preflight=%d, want one each", adapter.bindCalls.Load(), adapter.preflightCalls.Load())
	}
	if _, err := pool.get(context.Background(), profile, model, "test-model"); err != nil {
		t.Fatal(err)
	}
	if adapter.bindCalls.Load() != 1 {
		t.Fatalf("cache miss duplicated after success: binds=%d", adapter.bindCalls.Load())
	}
	if err := pool.Close(); err != nil {
		t.Fatal(err)
	}
	if err := pool.Close(); err != nil {
		t.Fatal(err)
	}
	if adapter.client.closeCalls.Load() != 1 || adapter.closeCalls.Load() != 1 {
		t.Fatalf("client closes=%d adapter closes=%d, want one each", adapter.client.closeCalls.Load(), adapter.closeCalls.Load())
	}
}

func TestBoundClientPoolFailureIsRetryable(t *testing.T) {
	adapter := newPoolTestAdapter()
	adapter.failPreflight.Store(1)
	close(adapter.release)
	pool, err := newBoundClientPool(adapter)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.get(context.Background(), ExecutionProfileEvaluation, poolTestModel(), "test-model"); err == nil {
		t.Fatal("first preflight failure unexpectedly succeeded")
	}
	if _, err := pool.get(context.Background(), ExecutionProfileEvaluation, poolTestModel(), "test-model"); err != nil {
		t.Fatalf("retry after transient failure failed: %v", err)
	}
	if adapter.bindCalls.Load() != 2 || adapter.preflightCalls.Load() != 2 {
		t.Fatalf("bind=%d preflight=%d, want two attempts", adapter.bindCalls.Load(), adapter.preflightCalls.Load())
	}
	_ = pool.Close()
}

func TestInvokeBoundPreservesCallerAndResponseTiming(t *testing.T) {
	adapter := newPoolTestAdapter()
	close(adapter.release)
	adapter.durationMillis = 17
	adapter.responseTiming = &inference.ResponseTiming{QueueWait: 5 * time.Millisecond, ProviderExecution: 9 * time.Millisecond, Total: 14 * time.Millisecond}
	pool, err := newBoundClientPool(adapter)
	if err != nil {
		t.Fatal(err)
	}
	_, usage, callerLatency, err := invokeBound(context.Background(), pool, ExecutionProfileEvaluation, "prompt", []byte(`{"type":"object"}`), poolTestModel(), "test-model")
	if err != nil {
		t.Fatal(err)
	}
	if usage.CallerLatencyMS != callerLatency.Milliseconds() || usage.QueueWaitMS != 5 || usage.ProviderExecutionMS != 9 || usage.ResponseTotalMS != 14 {
		t.Fatalf("caller=%s usage=%+v", callerLatency, usage)
	}
	if usage.ProviderExecutionMS == usage.CallerLatencyMS {
		t.Fatal("caller latency was overwritten by provider execution")
	}
	_ = pool.Close()
}

func TestSchemaJSONUsesPreparedRawBytes(t *testing.T) {
	raw := []byte(`{"type":"object","properties":{"answer":{"type":"string"}}}`)
	encoded, err := schemaJSON(json.RawMessage(raw))
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != string(raw) {
		t.Fatalf("schema bytes changed: %s", encoded)
	}
}

func TestInvokeBoundNilTimingFallsBackToLegacyDuration(t *testing.T) {
	adapter := newPoolTestAdapter()
	close(adapter.release)
	adapter.durationMillis = 17
	adapter.responseTiming = nil
	pool, err := newBoundClientPool(adapter)
	if err != nil {
		t.Fatal(err)
	}
	_, usage, _, err := invokeBound(context.Background(), pool, ExecutionProfileEvaluation, "prompt", json.RawMessage(`{"type":"object"}`), poolTestModel(), "test-model")
	if err != nil {
		t.Fatal(err)
	}
	if usage.ProviderExecutionMS != 17 || usage.ResponseTotalMS != 17 || usage.QueueWaitMS != 0 {
		t.Fatalf("legacy timing fallback=%+v", usage)
	}
	_ = pool.Close()
}

func TestInvokeBoundPreservesSafeUsageAndTimingFromFailedInferenceInvocation(t *testing.T) {
	adapter := newPoolTestAdapter()
	close(adapter.release)
	adapter.client.generateErr = inference.WithMetadata(nil, inference.ErrorMetadata{
		Code: inference.FailureCodeIncomplete, Stage: inference.FailureStageProvider,
		Usage:  inference.Usage{InputTokens: 101, CachedInputTokens: 40, OutputTokens: 17, ReasoningTokens: 9, TotalTokens: 118},
		Timing: &inference.ResponseTiming{QueueWait: 4 * time.Millisecond, ProviderExecution: 13 * time.Millisecond, Total: 19 * time.Millisecond},
		Receipt: &inference.ExecutionReceipt{
			ProviderModel: "gemini-3.7-flash", ActualProviderModel: "gemini-3.7-flash",
			NativeReasoningValue: "low", ReasoningTier: inference.ReasoningEffortLow,
			ModelDescriptorVersion: "gemini-catalog-2026-08-25", ModelMaturity: inference.ModelMaturityStable,
		},
	})
	pool, err := newBoundClientPool(adapter)
	if err != nil {
		t.Fatal(err)
	}
	_, usage, callerLatency, err := invokeBound(context.Background(), pool, ExecutionProfileEvaluation, "prompt", json.RawMessage(`{"type":"object"}`), poolTestModel(), "test-model")
	if err == nil {
		t.Fatal("failed inference invocation unexpectedly succeeded")
	}
	if usage.Input == nil || *usage.Input != 101 || usage.CachedInput == nil || *usage.CachedInput != 40 || usage.Output == nil || *usage.Output != 17 || usage.ReasoningOutput == nil || *usage.ReasoningOutput != 9 {
		t.Fatalf("failed invocation usage=%+v", usage)
	}
	if usage.ProviderModel != "gemini-3.7-flash" || usage.NativeReasoning != "low" || usage.ReasoningTier != "low" || usage.ModelDescriptorVersion != "gemini-catalog-2026-08-25" || usage.ModelMaturity != "stable" {
		t.Fatalf("failed invocation receipt=%+v", usage)
	}
	if usage.CallerLatencyMS != callerLatency.Milliseconds() || usage.QueueWaitMS != 4 || usage.ProviderExecutionMS != 13 || usage.ResponseTotalMS != 19 {
		t.Fatalf("failed invocation timing caller=%s usage=%+v", callerLatency, usage)
	}
	_ = pool.Close()
}

func TestResponseFormatNamePreservesStableSchemaIDVocabulary(t *testing.T) {
	if got := responseFormatName(ExecutionProfilePlanning); got != "akusidecar-planning" {
		t.Fatalf("planning response format name=%q", got)
	}
	if got := responseFormatName("custom.profile/value"); got != "custom-profile-value" {
		t.Fatalf("sanitized response format name=%q", got)
	}
}

func TestModelMaxOutputTokensKeepsDefaultAndAllowsProviderOverride(t *testing.T) {
	if got := modelMaxOutputTokens(config.ModelConfig{}); got != defaultProfileMaxOutputTokens {
		t.Fatalf("default max output tokens=%d", got)
	}
	if got := modelMaxOutputTokens(config.ModelConfig{MaxOutputTokens: 4096}); got != 4096 {
		t.Fatalf("provider max output tokens=%d", got)
	}
}
