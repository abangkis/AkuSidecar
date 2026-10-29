//go:build auth_session_dev

package engine

import (
	"errors"
	"github.com/abangkis/AkuSidecar/internal/appshell"
	"strings"
)

// Closed diagnostic projections retain private causes only through Unwrap.
type sourceNativeFailure struct {
	stage, reason string
	cause         error
}

func (e *sourceNativeFailure) Error() string    { return e.dailyFailureCode() }
func (e *sourceNativeFailure) GoString() string { return e.Error() }
func (e *sourceNativeFailure) Unwrap() error    { return e.cause }
func (e *sourceNativeFailure) dailyFailureCode() string {
	return "native_source_" + e.stage + "_" + e.reason
}

var sourceNativeStages = map[string]bool{"admission": true, "lifetime": true, "target": true, "collector": true, "session": true, "evidence": true, "navigation": true, "capture": true, "cleanup": true, "operation": true, "qualification": true, "startup": true, "ownership": true, "wire": true, "store": true}
var sourceNativeReasons = map[string]bool{"scope": true, "clock": true, "closed": true, "deadline": true, "canceled": true, "unavailable": true, "inventory": true, "target_present": true, "identity": true, "nonce": true, "projection": true, "required": true, "document": true, "document_changed": true, "account_mismatch": true, "stale": true, "pair": true, "fields": true, "profile": true, "owner": true, "changed": true, "home": true, "url": true, "expression": true, "method": true, "unverified": true, "binding": true, "busy": true, "replay": true, "batch": true, "collector": true, "state": true, "source": true, "observation": true, "operation": true, "generation": true, "proof": true, "decode": true}

func sourceNativeFail(cause error, stage, reason string) error {
	var previous *sourceNativeFailure
	if errors.As(cause, &previous) {
		return cause
	}
	if !sourceNativeStages[stage] || !sourceNativeReasons[reason] {
		stage, reason = "operation", "unavailable"
	}
	return &sourceNativeFailure{stage: stage, reason: reason, cause: cause}
}
func sourceNativeWireFailure(cause error) error {
	var wire *appshell.MainBrowserError
	if errors.As(cause, &wire) {
		for stage := range sourceNativeStages {
			prefix := "native_source_" + stage + "_"
			if strings.HasPrefix(wire.Code, prefix) {
				reason := strings.TrimPrefix(wire.Code, prefix)
				if sourceNativeReasons[reason] {
					return sourceNativeFail(cause, stage, reason)
				}
			}
		}
	}
	return sourceNativeFail(cause, "wire", "unavailable")
}
