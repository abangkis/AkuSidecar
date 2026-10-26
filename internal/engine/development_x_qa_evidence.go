//go:build auth_session_dev

package engine

import (
	"context"
	"errors"
	"sync"
	"time"
)

// QA evidence describes execution, never account/session or Store authority.
type XApplicationQAFailure struct {
	Stage  string `json:"stage"`
	Reason string `json:"reason"`
	Detail string `json:"detail,omitempty"`
}
type XApplicationQAStage struct {
	Stage     string `json:"stage"`
	ElapsedMS int64  `json:"elapsedMS"`
}
type XApplicationQAEvidence struct {
	Stages       []XApplicationQAStage  `json:"stages"`
	Overlap      string                 `json:"overlap"`
	OverlapBasis string                 `json:"overlapBasis"`
	Dropped      bool                   `json:"dropped"`
	FirstFailure *XApplicationQAFailure `json:"firstFailure,omitempty"`
	Cleanup      string                 `json:"cleanup"`
	AttemptClock string                 `json:"attemptClock"`
}
type xQAOperationError struct{ failure XApplicationQAFailure }

func (e *xQAOperationError) Error() string {
	return "QA operation failed: " + e.failure.Stage + "/" + e.failure.Reason
}

// Only privately constructed errors can supply diagnostics. Arbitrary error
// text, SQL, URLs, paths, account fields and peer values never reach the report.
func XApplicationQAFailureOf(err error) XApplicationQAFailure {
	var own *xQAOperationError
	if errors.As(err, &own) {
		return own.failure
	}
	return XApplicationQAFailure{Stage: "control", Reason: "unavailable"}
}
func xQAFailure(stage, reason string, err error) XApplicationQAFailure {
	validStage := false
	for _, v := range []string{"consent", "session", "command", "owner_startup", "reader_prepare", "reader_foreground", "reader_verify", "reader_navigate", "capture_wait", "capture", "reasoning", "stop", "control", "cleanup"} {
		if stage == v {
			validStage = true
			break
		}
	}
	if !validStage {
		stage = "control"
	}
	validReason := false
	for _, v := range []string{"rejected", "unavailable", "deadline", "canceled", "failed", "not_started", "replay", "invalid_command", "capacity", "input", "output", "listener", "uncertain"} {
		if reason == v {
			validReason = true
			break
		}
	}
	if !validReason {
		reason = "unavailable"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		reason = "deadline"
	} else if errors.Is(err, context.Canceled) {
		reason = "canceled"
	}
	return XApplicationQAFailure{Stage: stage, Reason: reason}
}

type xQAAttemptEvidence struct {
	mu                                                      sync.Mutex
	now                                                     func() time.Time
	origin                                                  time.Time
	stages                                                  []XApplicationQAStage
	first                                                   *XApplicationQAFailure
	failed                                                  chan struct{}
	active, entered, returned, readerSeen, overlap, dropped bool
	cleanup                                                 string
}

func newXQAAttemptEvidence(now func() time.Time) *xQAAttemptEvidence {
	return &xQAAttemptEvidence{now: now, origin: now(), failed: make(chan struct{}), cleanup: "not_run"}
}
func (e *xQAAttemptEvidence) mark(stage string) {
	if e == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	valid := false
	for _, v := range []string{"start_requested", "startup_accepted", "capture_entered", "capture_returned", "reader_requested", "reader_marker_ready", "reader_foreground_verified", "reader_navigation_requested", "reader_navigation_completed", "capture_settled", "stop_requested", "parent_cleanup_started"} {
		if stage == v {
			valid = true
			break
		}
	}
	if !valid {
		return
	}
	if len(e.stages) < 96 {
		e.stages = append(e.stages, XApplicationQAStage{stage, max(e.now().Sub(e.origin).Milliseconds(), 0)})
	} else {
		e.dropped = true
	}
	switch stage {
	case "capture_entered":
		e.active = true
		e.entered = true
	case "capture_returned":
		e.active = false
		e.returned = true
	case "reader_marker_ready", "reader_foreground_verified", "reader_navigation_requested", "reader_navigation_completed":
		e.readerSeen = true
		if e.active {
			e.overlap = true
		}
	}
}
func (e *xQAAttemptEvidence) fail(f XApplicationQAFailure) error {
	if e != nil {
		e.mu.Lock()
		if e.first == nil {
			copy := f
			e.first = &copy
			close(e.failed)
		}
		e.mu.Unlock()
	}
	return &xQAOperationError{failure: f}
}
func (e *xQAAttemptEvidence) snapshot() XApplicationQAEvidence {
	out := XApplicationQAEvidence{Overlap: "unknown", OverlapBasis: "capture_backend_rpc_interval", Cleanup: "not_run", AttemptClock: "seed_authorization_start"}
	if e == nil {
		return out
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	out.Stages = append([]XApplicationQAStage{}, e.stages...)
	out.Dropped = e.dropped
	out.Cleanup = e.cleanup
	if e.first != nil {
		f := *e.first
		out.FirstFailure = &f
	}
	if e.overlap {
		out.Overlap = "observed"
	} else if e.entered && e.returned && e.readerSeen && !e.dropped {
		out.Overlap = "not_observed"
	}
	return out
}
func (e *xQAAttemptEvidence) cleaned(ok bool) {
	if e == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cleanup = "uncertain"
	if ok {
		e.cleanup = "verified"
	}
}

type xQAEvidenceContextKey struct{}

func withXQAEvidence(ctx context.Context, e *xQAAttemptEvidence) context.Context {
	if e == nil {
		return ctx
	}
	return context.WithValue(ctx, xQAEvidenceContextKey{}, e)
}

// Begin only at the backend call, after claim/admission/payload checks. A queued
// goroutine, active pilot or reasoning work cannot count as executing capture.
func xQACaptureExecuting(ctx context.Context) func() {
	if ctx == nil {
		return func() {}
	}
	e, _ := ctx.Value(xQAEvidenceContextKey{}).(*xQAAttemptEvidence)
	if e == nil {
		return func() {}
	}
	e.mark("capture_entered")
	var once sync.Once
	return func() { once.Do(func() { e.mark("capture_returned") }) }
}
