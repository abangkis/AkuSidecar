package store

import (
	"context"
	"database/sql"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func (s *Store) SaveTelemetry(ctx context.Context, value domain.ReasoningTelemetry) error {
	callerLatency := value.CallerLatencyMS
	if callerLatency == 0 {
		callerLatency = value.DurationMS
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO reasoning_invocations(id,run_id,phase,provider,model,model_descriptor_version,model_maturity,effort,duration_ms,caller_latency_ms,queue_wait_ms,provider_execution_ms,response_total_ms,status,input_tokens,cached_input_tokens,output_tokens,reasoning_output_tokens,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, value.ID, value.RunID, value.Phase, value.Provider, value.Model, value.ModelDescriptorVersion, value.ModelMaturity, value.Effort, value.DurationMS, callerLatency, value.QueueWaitMS, value.ProviderExecutionMS, value.ResponseTotalMS, value.Status, value.InputTokens, value.CachedInputTokens, value.OutputTokens, value.ReasoningOutputTokens, value.CreatedAt)
	return err
}

// ReasoningInvocationsBySession lists all provider invocations belonging to a
// session's runs, ordered chronologically. It is used to enrich diagnostics
// exports with per-run provider, model, effort, and elapsed timing.
func (s *Store) ReasoningInvocationsBySession(ctx context.Context, sessionID string) ([]domain.ReasoningTelemetry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT i.id,i.run_id,i.phase,i.provider,i.model,i.model_descriptor_version,i.model_maturity,i.effort,i.duration_ms,i.caller_latency_ms,i.queue_wait_ms,i.provider_execution_ms,i.response_total_ms,i.status,
		       i.input_tokens,i.cached_input_tokens,i.output_tokens,i.reasoning_output_tokens,i.created_at
		FROM reasoning_invocations i JOIN runs r ON r.id=i.run_id
		WHERE r.session_id=? ORDER BY i.created_at,i.id`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.ReasoningTelemetry
	for rows.Next() {
		var t domain.ReasoningTelemetry
		var input, cached, output, reasoning sql.NullInt64
		if err := rows.Scan(&t.ID, &t.RunID, &t.Phase, &t.Provider, &t.Model, &t.ModelDescriptorVersion, &t.ModelMaturity, &t.Effort, &t.DurationMS, &t.CallerLatencyMS, &t.QueueWaitMS, &t.ProviderExecutionMS, &t.ResponseTotalMS, &t.Status, &input, &cached, &output, &reasoning, &t.CreatedAt); err != nil {
			return nil, err
		}
		if t.CallerLatencyMS == 0 {
			t.CallerLatencyMS = t.DurationMS
		}
		if input.Valid {
			t.InputTokens = &input.Int64
		}
		if cached.Valid {
			t.CachedInputTokens = &cached.Int64
		}
		if output.Valid {
			t.OutputTokens = &output.Int64
		}
		if reasoning.Valid {
			t.ReasoningOutputTokens = &reasoning.Int64
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
