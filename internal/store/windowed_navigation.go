package store

import (
	"math"
	"slices"
)

// Keep the ordinary Bridge navigation receipt when a challenge replaces the
// rest of the failure details with a closed, privacy-safe projection.
func safeWindowedNavigation(value any) map[string]any {
	raw, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	state, _ := raw["state"].(string)
	switch state {
	case "matched", "not_observed", "unavailable":
	default:
		return nil
	}
	result := map[string]any{"state": state}
	if opened, ok := raw["tabOpenedForCapture"].(bool); ok {
		result["tabOpenedForCapture"] = opened
	}
	if state != "matched" {
		return result
	}

	result["httpStatusSource"] = "not_observed"
	status, _ := raw["httpStatus"].(float64)
	if source, _ := raw["httpStatusSource"].(string); source == "navigation_timing" &&
		status >= 200 && status <= 599 && math.Trunc(status) == status {
		result["httpStatus"] = status
		result["httpStatusSource"] = source
	}
	if redirects, ok := raw["redirectCount"].(float64); ok &&
		redirects >= 0 && redirects <= 20 && math.Trunc(redirects) == redirects {
		result["redirectCount"] = redirects
	}
	for _, key := range []string{"serviceWorkerInvolved", "userAgentHeadless", "clientHintsHeadless"} {
		if flag, ok := raw[key].(bool); ok {
			result[key] = flag
		}
	}
	for key, allowed := range map[string][]string{
		"protocol":       {"http/1.1", "h2", "h3"},
		"navigationType": {"navigate", "reload", "back_forward", "prerender"},
		"deliveryType":   {"cache", "navigational-prefetch"},
	} {
		if text, ok := raw[key].(string); ok && slices.Contains(allowed, text) {
			result[key] = text
		}
	}
	return result
}
