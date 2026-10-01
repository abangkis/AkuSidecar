// Package bridge adapts acquisition requests to the existing AkuBridge wire
// contract without owning command dispatch or Chrome lifecycle.
package bridge

import "github.com/abangkis/AkuSidecar/internal/collection"

type Builder struct{}

var _ collection.CommandBuilder = Builder{}

// Build preserves the existing collect_visible payload, including explicit nil
// continuation and first-round-only tab mutation. No mode selection is active.
func (Builder) Build(request collection.Request) map[string]any {
	firstRound := request.Round == 1
	pendingPolicy, freshnessPolicy := "detect_only", "preserve_frontier"
	if firstRound {
		pendingPolicy, freshnessPolicy = "reveal_if_present", "wake_and_reveal"
	}
	return map[string]any{
		"mode":                     "catch_up",
		"source":                   request.Source,
		"sourceHydrationTimeoutMs": request.HydrationTimeoutMS,
		"scrolls":                  request.MaxScrolls,
		"scrollFraction":           0.75,
		"scrollSettleMs":           900,
		"captureTimeoutMs":         45000,
		"pendingContentPolicy":     pendingPolicy,
		"sameTabMutationAllowed":   firstRound,
		"pendingContentTimeoutMs":  5000,
		"pendingContentSettleMs":   700,
		"sourceFreshnessPolicy":    freshnessPolicy,
		"captureVisibilityPolicy":  request.Browser.Visibility,
		"captureLeaseId":           request.LeaseID,
		"maxBlocksPerSnapshot":     20,
		"maxBlockCharacters":       4000,
		"qualityReportRequired":    true,
		"qualityRetryBudget":       1,
		"qualityRetrySettleMs":     request.QualityRetrySettleMS,
		"openIfMissing":            firstRound && request.OpenMissingSource,
		"tabLifecycle":             map[string]any{"ownership": "shared", "openedTabDisposition": "preserve"},
		"restoreScroll":            true,
		"browserAdapter":           "aku-bridge",
		"acquisitionRound":         request.Round,
		"maxAcquisitionRounds":     2,
		"continuation":             request.Continuation,
		"followUpReason":           request.FollowUpReason,
	}
}
