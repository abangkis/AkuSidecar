package engine

import (
	"context"
	"errors"
)

// XDailyStartupDiagnostic contains only application-owned catalog values. It
// cannot contain account IDs, cookies, paths, worker replies or exception text.
type XDailyStartupDiagnostic struct {
	Stage        string `json:"stage"`
	Condition    string `json:"condition"`
	Cause        string `json:"cause"`
	Origin       string `json:"origin,omitempty"`
	BrokerReason string `json:"brokerReason,omitempty"`
}

var xDailyStartupStages = []string{
	"admission", "reader_check", "nonce", "collector_root", "collector_profile",
	"root_identity", "profile_identity", "collector_lease", "batch_begin_rpc",
	"batch_begin_reply", "reader_bind", "collector_bind", "seed_rpc", "seed_receipt",
	"broker_create", "broker_start", "managers", "runtime", "store_admission",
}
var xDailyStartupConditions = []string{
	"unavailable", "owner_failed", "batch_active", "generation", "source", "run_id",
	"session_id", "context", "parent", "ownership", "random", "create", "identity",
	"lease", "rpc", "decode", "owner", "reader_pid", "collector_pid", "account",
	"seeded", "home_ready", "reader_profile", "collector_profile", "account_changed",
	"configuration", "eligibility", "manager", "runtime", "authorization",
}

func xDailyStartupMember(value string, catalog []string, fallback string) string {
	for _, allowed := range catalog {
		if value == allowed {
			return value
		}
	}
	return fallback
}

func (d XDailyStartupDiagnostic) closed() XDailyStartupDiagnostic {
	// Only the tagged native adapter can supply validated origin metadata.
	d.Origin, d.BrokerReason = "", ""
	d.Stage = xDailyStartupMember(d.Stage, xDailyStartupStages, "admission")
	d.Condition = xDailyStartupMember(d.Condition, xDailyStartupConditions, "unavailable")
	d.Cause = xDailyStartupMember(d.Cause, []string{"unavailable", "canceled", "deadline", "rejected"}, "unavailable")
	return d
}

type xDailyStartupError struct {
	detail       XDailyStartupDiagnostic
	cause        error  // Private operational cause; never formatted or serialized.
	origin       string // Produced only from the existing closed native catalog.
	brokerReason string
}

func (e *xDailyStartupError) Error() string {
	d := e.detail.closed()
	return "daily_startup_" + d.Stage + "_" + d.Condition + "_" + d.Cause
}
func (e *xDailyStartupError) GoString() string         { return e.Error() }
func (e *xDailyStartupError) Unwrap() error            { return e.cause }
func (e *xDailyStartupError) dailyFailureCode() string { return e.Error() }

// Startup diagnostics do not grant retry authority. In particular, a peer
// deadline can originate from missing identity rather than network failure.
func (e *xDailyStartupError) dailyRetryClass() string { return "" }

func xDailyStartupFail(cause error, stage, condition string) error {
	var first *xDailyStartupError
	if errors.As(cause, &first) && first != nil {
		return first
	}
	category := "rejected"
	if cause != nil {
		category = "unavailable"
	}
	if errors.Is(cause, context.DeadlineExceeded) {
		category = "deadline"
	} else if errors.Is(cause, context.Canceled) {
		category = "canceled"
	}
	return &xDailyStartupError{detail: (XDailyStartupDiagnostic{Stage: stage, Condition: condition, Cause: category}).closed(), cause: cause}
}

func (e *Engine) rememberXDailyFailure(cause error, phase string) {
	e.dailyController("x").rememberFailure(cause, phase)
}

func (d dailyController) rememberFailure(cause error, phase string) {
	if cause == nil {
		return
	}
	d.state.mu.Lock()
	defer d.state.mu.Unlock()
	if d.state.firstFailure == nil {
		d.state.firstFailure = cause
		d.state.failurePhase = phase
		d.state.failureCode = xDailyClosedFailureCode(cause)
	}
}

func xDailyStartupDetail(err error) *XDailyStartupDiagnostic {
	var known *xDailyStartupError
	if !errors.As(err, &known) || known == nil {
		return nil
	}
	d := known.detail.closed()
	d.Origin, d.BrokerReason = known.origin, known.brokerReason
	return &d
}
