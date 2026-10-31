//go:build auth_session_dev

package engine

import (
	"errors"
	"github.com/abangkis/AkuSidecar/internal/appshell"
	"strconv"
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

func (e *sourceNativeFailure) dailyRetryClass() string {
	if e.stage == "evidence" {
		for _, role := range []string{"reader", "collector"} {
			switch e.reason {
			case role + "_identity_fetch_timeout":
				return "timeout"
			}
		}
	}
	if e.stage == "capture" || e.stage == "navigation" || e.stage == "evidence" || e.stage == "startup" {
		if e.reason == "deadline" {
			return "timeout"
		}
		if e.reason == "unavailable" {
			return dailyTransientClass(e.cause)
		}
	}
	return ""
}

var sourceNativeStages = map[string]bool{"admission": true, "lifetime": true, "target": true, "collector": true, "session": true, "evidence": true, "navigation": true, "capture": true, "cleanup": true, "operation": true, "qualification": true, "startup": true, "ownership": true, "wire": true, "store": true}
var sourceNativeReasons = map[string]bool{"scope": true, "clock": true, "closed": true, "deadline": true, "canceled": true, "unavailable": true, "inventory": true, "target_present": true, "identity": true, "nonce": true, "projection": true, "required": true, "document": true, "document_changed": true, "account_mismatch": true, "stale": true, "pair": true, "fields": true, "profile": true, "owner": true, "changed": true, "home": true, "url": true, "expression": true, "method": true, "unverified": true, "binding": true, "busy": true, "replay": true, "batch": true, "collector": true, "state": true, "source": true, "observation": true, "operation": true, "generation": true, "proof": true, "decode": true}

// Keep the browser projection's closed rejection categories intact across RPC.
// No arbitrary reason or native exception text becomes a public diagnostic.
func init() {
	for _, role := range []string{"reader", "collector", "unknown"} {
		// Same closed categories as INSTAGRAM_CORROBORATION_REASONS, excluding
		// success. These describe private acquisition, never an account value.
		for _, reason := range []string{
			"invalid_request", "scope_unavailable", "setup_failed", "navigation_failed", "wire_unmatched",
			"http_rejected", "delivery_unproven", "body_bounds", "body_read", "body_encoding", "body_deadline",
			"body_invalid", "context_unavailable", "document_changed", "stale_delivery", "evaluation_failed",
			"projection_invalid", "page_scope", "login_visible", "challenge", "bootstrap_rejected", "id_mismatch",
			"cookie_unsupported", "cookie_missing", "cookie_ambiguous", "cookie_invalid", "cookie_mismatch",
			"cookie_failed", "cleanup_failed",
		} {
			sourceNativeReasons[role+"_bootstrap_"+reason] = true
		}
		// Same finite contract as INSTAGRAM_HTTP_REJECTIONS: no arbitrary text
		// or unbounded numeric suffix is admitted across the wire boundary.
		for status := 100; status <= 599; status++ {
			if status != 200 && status != 401 && status != 403 && status != 429 {
				sourceNativeReasons[role+"_identity_http_status_"+strconv.Itoa(status)] = true
			}
		}
		for _, reason := range []string{
			"page_scope", "login_visible", "http_unauthorized", "http_forbidden", "http_rate_limited",
			"http_server", "http_status", "response_origin", "response_mime", "body_size", "body_missing",
			"body_read", "body_encoding", "body_json", "response_shape", "account_missing", "account_anonymous",
			"account_type", "account_format", "page_changed", "fetch_timeout", "fetch_failed", "projection",
		} {
			sourceNativeReasons[role+"_identity_"+reason] = true
		}
	}
}

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
