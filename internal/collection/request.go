// Package collection defines the acquisition boundary shared by collection
// drivers. Persistence, command claiming and observation acceptance remain with
// the engine/store; a command builder does not execute browser actions.
package collection

import "github.com/abangkis/AkuSidecar/internal/domain"

// Request is the engine's acquisition intent, independent of Settings and Run.
// The engine controls the acquisition round and continuation. Drivers must
// return results through the existing domain.Observation acceptance boundary.
type Request struct {
	Source               domain.Source
	LeaseID              string
	HydrationTimeoutMS   int
	MaxScrolls           int
	QualityRetrySettleMS int
	OpenMissingSource    bool
	Round                int
	Continuation         map[string]any
	FollowUpReason       string
	Browser              BrowserOptions
}

// BrowserOptions contains options specific to the current visible-browser
// driver. Visibility is not a choice of collection mode.
type BrowserOptions struct {
	Visibility string
}

// CommandBuilder translates acquisition intent to a persisted command payload.
// It is a preparation contract, not a synchronous Collect API: the existing
// Bridge transport claims commands and submits observations asynchronously.
type CommandBuilder interface {
	Build(Request) map[string]any
}
