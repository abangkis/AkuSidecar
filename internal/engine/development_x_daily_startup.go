//go:build auth_session_dev

package engine

import "errors"

// Keep the domain's existing closed diagnostics; the operational error stays
// privately unwrap-able. No arbitrary Error() or child exception is exported.
func xDailyNativeStartupFail(cause error, stage, condition string, broker *nativePilotBroker, diagnostic nativePilotDiagnostic) error {
	var first *xDailyStartupError
	if errors.As(cause, &first) && first != nil {
		return first
	}
	result := xDailyStartupFail(cause, stage, condition).(*xDailyStartupError)
	var detail nativePilotInitialDetail
	var hasDetail bool
	var initial *nativePilotInitialError
	if errors.As(cause, &initial) && initial != nil {
		detail, hasDetail = initial.detail.closed(), true
	} else {
		var carrier interface{ dailyFailureCode() string }
		if errors.As(cause, &carrier) {
			detail, hasDetail = nativePilotInitialCode(carrier.dailyFailureCode())
		}
	}
	reason := diagnostic.Reason
	if broker != nil {
		if !hasDetail {
			detail, hasDetail = broker.initialDiagnostic(), true
		}
		broker.mu.Lock()
		if broker.firstStopReason != "" {
			reason = broker.firstStopReason
		}
		broker.mu.Unlock()
	}
	if hasDetail {
		result.origin = detail.closed().String()
	}
	result.brokerReason = xDailyStartupMember(reason, []string{
		"admission-disabled", "invalid-configuration", "admission-closed", "startup-already-attempted",
		"owner-unavailable", "parent-exited", "opted-out", "evidence-channel-lost", "reader-lost",
		"owner-or-policy-changed", "prepare-failed", "prepare-pins-invalid", "prepare-handle-replayed",
		"source-unavailable", "local-effect-rejected", "canceled", "deadline", "expired",
		"identity-mismatch", "authentication-rejected", "cleanup-uncertain", "evidence-invalid",
		"admitted", "valid", "initial-evidence-rejected", "clock-regressed",
	}, "")
	return result
}
