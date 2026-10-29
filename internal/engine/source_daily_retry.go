package engine

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/abangkis/AkuSidecar/internal/collection/headless"
)

const dailyRetryLimit = 2
const dailyRetryDelay = 30 * time.Second

type dailyTransientFailure struct{ class string }

func (e *dailyTransientFailure) dailyFailureCode() string { return e.Error() }

func (e *dailyTransientFailure) Error() string           { return "daily_transient_" + e.class }
func (e *dailyTransientFailure) dailyRetryClass() string { return e.class }

type dailyRetryCleanup interface{ dailyRetryReady() bool }

// Operational error brands, never diagnostic JSON or arbitrary error text,
// classify a transient failure. Unknown failures remain terminal.
func dailyTransientClass(cause error) string {
	if cause == nil || errors.Is(cause, context.Canceled) {
		return ""
	}
	var capture *headless.CaptureError
	if errors.As(cause, &capture) {
		return ""
	}
	var typed interface{ dailyRetryClass() string }
	if errors.As(cause, &typed) {
		switch class := typed.dailyRetryClass(); class {
		case "timeout", "network":
			return class
		}
		return ""
	}
	if errors.Is(cause, context.DeadlineExceeded) {
		return "timeout"
	}
	var network *net.OpError
	if errors.As(cause, &network) {
		return "network"
	}
	return ""
}

// The caller holds state.mu after batch Close and Store reconciliation.
// Retries occur only on a subsequent regular update, never by replaying a run.
func (d dailyController) finishRetryPolicy(cause error, phase string, cleanupFailed, retryReady bool) {
	if cause == nil && !cleanupFailed {
		d.state.retryCount = 0
		d.state.retryAt = time.Time{}
		return
	}
	allowedPhase := phase == "startup" || phase == "capture"
	if !cleanupFailed && retryReady && allowedPhase && dailyTransientClass(cause) != "" &&
		d.state.authorized && !d.state.closed && d.state.reason == "" &&
		d.state.parent != nil && d.state.parent.Err() == nil && d.state.retryCount < dailyRetryLimit {
		d.state.retryCount++
		d.state.retryAt = d.engine.store.Now().Add(time.Duration(d.state.retryCount) * dailyRetryDelay)
		return
	}
	d.state.retryAt = time.Time{}
	d.state.authorized = false
	if d.state.reason == "" {
		d.state.reason = "batch-failed"
	}
}
