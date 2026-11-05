package engine

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
)

type acquisitionPlanningShadow struct {
	plan   func(context.Context, reasoning.GLMPlanningShadowInput) (string, domain.ModelUsage, error)
	runID  string
	cancel context.CancelFunc
}

// SetGLMPlanningShadow installs the development-only observer during startup.
func (e *Engine) SetGLMPlanningShadow(planner *reasoning.GLMPlanningShadow) {
	if e.config.Dev && planner != nil {
		e.mu.Lock()
		defer e.mu.Unlock()
		if e.planningShadow == nil {
			e.planningShadow = &acquisitionPlanningShadow{plan: planner.Plan}
		}
	}
}

func (e *Engine) startPlanningShadow(parent context.Context, run domain.Run, observation domain.Observation, provider string, primary reasoning.AcquisitionPlan, primaryErr error) {
	if !e.config.Dev || parent.Err() != nil {
		return
	}
	e.mu.Lock()
	shadow := e.planningShadow
	if shadow == nil || shadow.cancel != nil || e.work.shuttingDown || e.work.cancelled[run.ID] {
		e.mu.Unlock()
		return
	}
	ctx, cancel := context.WithTimeout(parent, reasoning.GLMPlanningShadowTimeout)
	shadow.runID, shadow.cancel = run.ID, cancel
	e.work.workerStarted()
	e.mu.Unlock()

	input := reasoning.NewGLMPlanningShadowInput(run, observation)
	result := planningShadowResult{
		RunID: run.ID, Source: run.Source, Model: reasoning.GLMPlanningShadowModel,
		PrimaryProvider: provider, Agreement: "unavailable",
	}
	if primaryErr == nil && (primary.Decision == "finish" || primary.Decision == "request_follow_up") {
		result.PrimaryDecision = primary.Decision
	}
	go func(sessionID string) {
		defer func() {
			cancel()
			e.mu.Lock()
			shadow.runID, shadow.cancel = "", nil
			e.work.workerFinished()
			e.mu.Unlock()
		}()
		session, err := e.store.GetSession(ctx, sessionID)
		if err != nil || session.Trigger != domain.UpdateTriggerUser ||
			session.Delivery != domain.UpdateDeliveryVisible || session.BudgetAuthority != domain.BudgetAuthorityUser {
			return
		}
		started := time.Now()
		decision, usage, err := shadow.plan(ctx, input)
		result.LatencyMS = time.Since(started).Milliseconds()
		result.InputTokens, result.OutputTokens, result.ReasoningTokens = usage.Input, usage.Output, usage.ReasoningOutput
		switch {
		case errors.Is(ctx.Err(), context.Canceled):
			result.Status = "cancelled"
		case errors.Is(ctx.Err(), context.DeadlineExceeded):
			result.Status = "timeout"
		case err != nil:
			result.Status = "failed"
			if failure, ok := reasoning.ProviderFailureFrom(err); ok {
				result.FailureCode, result.ProviderStatus = failure.Code, failure.ProviderStatus
			}
		default:
			result.Status, result.Decision = "completed", decision
			if result.PrimaryDecision != "" {
				result.Agreement = "different"
				if result.PrimaryDecision == decision {
					result.Agreement = "match"
				}
			}
		}
		raw, _ := json.Marshal(result)
		e.logger.Printf("planning_shadow %s", raw)
	}(run.SessionID)
}

type planningShadowResult struct {
	RunID           string        `json:"runId"`
	Source          domain.Source `json:"source"`
	Model           string        `json:"model"`
	Status          string        `json:"status"`
	PrimaryProvider string        `json:"primaryProvider"`
	PrimaryDecision string        `json:"primaryDecision,omitempty"`
	Decision        string        `json:"shadowDecision,omitempty"`
	Agreement       string        `json:"agreement"`
	LatencyMS       int64         `json:"latencyMs"`
	InputTokens     *int64        `json:"inputTokens,omitempty"`
	OutputTokens    *int64        `json:"outputTokens,omitempty"`
	ReasoningTokens *int64        `json:"reasoningTokens,omitempty"`
	FailureCode     string        `json:"failureCode,omitempty"`
	ProviderStatus  int           `json:"providerStatus,omitempty"`
}
