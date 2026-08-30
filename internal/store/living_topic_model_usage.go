package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func livingTopicUsageToken(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

// RecordLivingTopicModelInvocation keeps a content-free receipt for provider
// cost accounting. Prompts, model output, and topic evidence never enter this
// ledger.
func (s *Store) RecordLivingTopicModelInvocation(ctx context.Context, category, ownerID, status, provider, model, effort string, duration time.Duration, usage domain.ModelUsage) error {
	category = strings.TrimSpace(category)
	if category != "living_topic_routing" && category != "living_topic_understanding" {
		return errors.New("Living Topic model invocation category is invalid")
	}
	status = strings.TrimSpace(status)
	if status != "completed" && status != "failed" {
		return errors.New("Living Topic model invocation status is invalid")
	}
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return errors.New("Living Topic model invocation provider is required")
	}
	if usage.ProviderModel != "" {
		model = usage.ProviderModel
	}
	if usage.NativeReasoning != "" {
		effort = usage.NativeReasoning
	}
	durationMS := max(duration.Milliseconds(), 0)
	callerLatencyMS := usage.CallerLatencyMS
	if callerLatencyMS == 0 {
		callerLatencyMS = durationMS
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO living_topic_model_invocations(
			id,category,owner_id,status,provider,model,model_descriptor_version,model_maturity,effort,
			duration_ms,caller_latency_ms,queue_wait_ms,provider_execution_ms,response_total_ms,
			input_tokens,cached_input_tokens,output_tokens,reasoning_output_tokens,created_at
		) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		domain.NewID("topic_invocation"), category, strings.TrimSpace(ownerID), status, provider, strings.TrimSpace(model), usage.ModelDescriptorVersion, usage.ModelMaturity, strings.TrimSpace(effort),
		durationMS, max(callerLatencyMS, 0), max(usage.QueueWaitMS, 0), max(usage.ProviderExecutionMS, 0), max(usage.ResponseTotalMS, 0),
		livingTopicUsageToken(usage.Input), livingTopicUsageToken(usage.CachedInput), livingTopicUsageToken(usage.Output), livingTopicUsageToken(usage.ReasoningOutput), memoryNow(s))
	return err
}
