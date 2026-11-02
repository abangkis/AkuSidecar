package engine

import (
	"context"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
)

func boundedFailureMessage(err error) string {
	return boundedText(err.Error(), 500)
}

func boundedText(value string, limit int) string {
	message := strings.TrimSpace(value)
	if len(message) > limit {
		return message[:limit]
	}
	return message
}

func providerFailureDetails(failure reasoning.ProviderFailure) map[string]any {
	details := map[string]any{"sdkCode": failure.Code, "sdkStage": failure.Stage, "sdkRetry": failure.Retry}
	if failure.Category != "" {
		details["sdkCategory"] = failure.Category
	}
	if failure.Reason != "" {
		details["sdkReason"] = failure.Reason
	}
	if failure.ProviderResponseStatus != "" {
		details["sdkProviderResponseStatus"] = failure.ProviderResponseStatus
	}
	if failure.PartialOutputSeen {
		details["sdkPartialOutputSeen"] = true
	}
	if len(failure.DiagnosticCodes) > 0 {
		details["sdkDiagnosticCodes"] = append([]string(nil), failure.DiagnosticCodes...)
	}
	if failure.ProviderStatus != 0 {
		details["sdkProviderStatus"] = failure.ProviderStatus
	}
	if failure.RequestID != "" {
		details["sdkRequestID"] = failure.RequestID
	}
	if failure.Operation != "" {
		details["sdkOperation"] = failure.Operation
	}
	if failure.RPCCode != 0 {
		details["sdkRPCCode"] = failure.RPCCode
	}
	if failure.ProcessExitCode != 0 {
		details["sdkProcessExitCode"] = failure.ProcessExitCode
	}
	return details
}

func (e *Engine) failRunAfterProcessError(ctx context.Context, runID string, processErr error) error {
	usageLimit := reasoning.IsUsageLimitError(processErr)
	failure := domain.Failure{Code: "reasoning_failed", Stage: "reasoning", Message: boundedFailureMessage(processErr), Retryable: true}
	if providerFailure, ok := reasoning.ProviderFailureFrom(processErr); ok {
		failure.Details = providerFailureDetails(providerFailure)
		if !providerFailure.RetryTransient {
			failure.Retryable = false
		}
		if providerFailure.Message != "" {
			failure.Message = boundedText(providerFailure.Message+" ("+processErr.Error()+")", 500)
		}
	}
	if usageLimit {
		failure.Code = "codex_usage_limit"
		failure.Retryable = false
	}
	run, err := e.store.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	// Capture authority ends when the native collector is retired. Recording
	// a reasoning failure uses the durable accepted-capture receipt instead;
	// it must not reopen capture or pass through the generic mutation fence.
	if run.Status == "reasoning" {
		err = e.store.FailCapturedRunReasoning(ctx, run, failure)
	} else {
		err = e.store.FailRun(ctx, runID, failure)
	}
	if err != nil {
		return err
	}
	if usageLimit {
		e.pauseAutoUpdateForUsageLimit(ctx, run.SessionID, processErr)
		failedRun, loadErr := e.store.GetRun(ctx, runID)
		if loadErr != nil {
			return loadErr
		}
		if err := e.store.FailQueuedAfterReasoningFailure(ctx, failedRun, failure); err != nil {
			return err
		}
	}
	_, err = e.startNext(ctx, run.SessionID)
	return err
}

func (e *Engine) pauseAutoUpdateForUsageLimit(ctx context.Context, sessionID string, err error) bool {
	if !reasoning.IsUsageLimitError(err) {
		return false
	}
	message := strings.TrimSpace(err.Error())
	if len(message) > 500 {
		message = message[:500]
	}
	pause := domain.AutoUpdateUsageLimitPause{Message: message, SessionID: sessionID}
	if pauseErr := e.store.PauseAutoUpdateForUsageLimit(ctx, pause); pauseErr != nil {
		e.logger.Printf("pause Auto Update after Codex usage limit: %v", pauseErr)
		return false
	}
	e.scheduler.notify()
	return true
}

func (e *Engine) clearAutoUpdateUsageLimitAfterSuccessfulUserSession(ctx context.Context, session domain.Session) {
	if session.Status != "completed" || session.Trigger != domain.UpdateTriggerUser || session.Delivery != domain.UpdateDeliveryVisible || session.BudgetAuthority != domain.BudgetAuthorityUser {
		return
	}
	pause, err := e.store.AutoUpdateUsageLimitPause(ctx)
	if err != nil {
		e.logger.Printf("read Auto Update usage-limit pause after successful user session %s: %v", session.ID, err)
		return
	}
	if pause == nil {
		return
	}
	report, err := e.store.SessionModelUsage(ctx, session.ID)
	if err != nil {
		e.logger.Printf("read model usage before restoring Auto Update after user session %s: %v", session.ID, err)
		return
	}
	if !modelUsageProvesCodexRestored(report) {
		return
	}
	if err := e.store.ConfirmAutoUpdateUsageRestored(ctx); err != nil {
		e.logger.Printf("restore Auto Update after successful user session %s: %v", session.ID, err)
		return
	}
	e.logger.Printf("restored Auto Update after model-backed user session %s completed successfully", session.ID)
	e.scheduler.notify()
}

func modelUsageProvesCodexRestored(report domain.ModelUsageReport) bool {
	completedModelInvocation := false
	for _, category := range report.Categories {
		for _, entry := range category.Entries {
			switch entry.Status {
			case "failed":
				return false
			case "completed":
				if strings.TrimSpace(entry.Provider) != "" && strings.TrimSpace(entry.Model) != "" && !strings.EqualFold(strings.TrimSpace(entry.Model), "none") {
					completedModelInvocation = true
				}
			}
		}
	}
	return completedModelInvocation
}
