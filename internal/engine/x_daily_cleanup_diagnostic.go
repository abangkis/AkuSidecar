package engine

import (
	"errors"
	"fmt"
)

// Resource outcomes are closed values, never worker exceptions or target IDs.
type XDailyCleanupResources struct {
	ReaderControls    string `json:"readerControls"`
	CollectorControls string `json:"collectorControls"`
	Collector         string `json:"collector"`
	Enrollment        string `json:"enrollment"`
	Attachments       string `json:"attachments"`
	Bootstrap         string `json:"bootstrap"`
}

func (r XDailyCleanupResources) valid() bool {
	for _, value := range []string{r.ReaderControls, r.CollectorControls, r.Collector, r.Enrollment, r.Attachments, r.Bootstrap} {
		if value != "verified" && value != "uncertain" && value != "not_needed" {
			return false
		}
	}
	return true
}
func (r XDailyCleanupResources) complete() bool {
	return r.valid() && r.ReaderControls != "uncertain" && r.CollectorControls != "uncertain" && r.Collector != "uncertain" && r.Enrollment != "uncertain" && r.Attachments != "uncertain" && r.Bootstrap != "uncertain"
}

// Cleanup has its own diagnostic channel and cannot replace startup's cause.
type XDailyBootstrapDiagnostic struct {
	Stage     string `json:"stage"`
	Condition string `json:"condition"`
}

func (d XDailyBootstrapDiagnostic) closed() XDailyBootstrapDiagnostic {
	d.Stage = xDailyStartupMember(d.Stage, []string{"inventory_before", "close", "inventory_after", "absence", "release", "state"}, "unknown")
	d.Condition = xDailyStartupMember(d.Condition, []string{"rpc", "invalid_reply", "timeout", "pending", "clock_invalid"}, "unavailable")
	return d
}

func (d XDailyBootstrapDiagnostic) valid() bool { return d == d.closed() }

type XDailyCleanupDiagnostic struct {
	Stage          string                     `json:"stage"`
	Condition      string                     `json:"condition"`
	Broker         string                     `json:"broker"`
	BatchEnd       string                     `json:"batchEnd"`
	CollectorExit  string                     `json:"collectorExit"`
	PinRelease     string                     `json:"pinRelease"`
	StoreReconcile string                     `json:"storeReconcile"`
	Resources      *XDailyCleanupResources    `json:"resources,omitempty"`
	Bootstrap      *XDailyBootstrapDiagnostic `json:"bootstrapDiagnostic,omitempty"`
}

func (d XDailyCleanupDiagnostic) closed() XDailyCleanupDiagnostic {
	if d.Bootstrap != nil {
		bootstrap := d.Bootstrap.closed()
		d.Bootstrap = &bootstrap
	}
	d.Stage = xDailyStartupMember(d.Stage, []string{"broker_stop", "batch_end", "collector_exit", "pin_release", "store_reconcile", "owner_close"}, "unknown")
	d.Condition = xDailyStartupMember(d.Condition, []string{"unverified", "rpc", "receipt", "alive", "release", "unavailable"}, "unavailable")
	outcome := func(s string) string {
		return xDailyStartupMember(s, []string{"verified", "uncertain", "not_needed", "not_attempted"}, "not_attempted")
	}
	d.Broker = outcome(d.Broker)
	d.BatchEnd = outcome(d.BatchEnd)
	d.CollectorExit = outcome(d.CollectorExit)
	d.PinRelease = outcome(d.PinRelease)
	d.StoreReconcile = outcome(d.StoreReconcile)
	if d.Resources != nil {
		r := *d.Resources
		resource := func(s string) string {
			return xDailyStartupMember(s, []string{"verified", "uncertain", "not_needed"}, "uncertain")
		}
		r.ReaderControls = resource(r.ReaderControls)
		r.CollectorControls = resource(r.CollectorControls)
		r.Collector = resource(r.Collector)
		r.Enrollment = resource(r.Enrollment)
		r.Attachments = resource(r.Attachments)
		r.Bootstrap = resource(r.Bootstrap)
		d.Resources = &r
	}
	return d
}

func (d XDailyCleanupDiagnostic) String() string {
	d = d.closed()
	s := fmt.Sprintf("stage=%s condition=%s broker=%s batch_end=%s collector_exit=%s pin_release=%s store_reconcile=%s", d.Stage, d.Condition, d.Broker, d.BatchEnd, d.CollectorExit, d.PinRelease, d.StoreReconcile)
	if r := d.Resources; r != nil {
		s += fmt.Sprintf(" reader_controls=%s collector_controls=%s collector=%s enrollment=%s attachments=%s bootstrap=%s", r.ReaderControls, r.CollectorControls, r.Collector, r.Enrollment, r.Attachments, r.Bootstrap)
	}
	if b := d.Bootstrap; b != nil {
		s += fmt.Sprintf(" bootstrap_stage=%s bootstrap_condition=%s", b.Stage, b.Condition)
	}
	return s
}
func (d XDailyCleanupDiagnostic) GoString() string { return d.String() }

type xDailyCleanupError struct {
	detail XDailyCleanupDiagnostic
	cause  error
}

func (e *xDailyCleanupError) Error() string {
	d := e.detail.closed()
	return "daily_cleanup_" + d.Stage + "_" + d.Condition
}
func (e *xDailyCleanupError) GoString() string { return e.Error() }
func (e *xDailyCleanupError) Unwrap() error    { return e.cause }
func xDailyCleanupFail(err error, stage, condition string) error {
	var known *xDailyCleanupError
	if errors.As(err, &known) {
		return known
	}
	return &xDailyCleanupError{detail: XDailyCleanupDiagnostic{Stage: stage, Condition: condition}.closed(), cause: err}
}
func (e *Engine) rememberXDailyCleanup(err error) {
	if err == nil {
		return
	}
	var known *xDailyCleanupError
	if !errors.As(err, &known) {
		err = xDailyCleanupFail(err, "batch_end", "unverified")
		_ = errors.As(err, &known)
	}
	d := known.detail.closed()
	e.xDaily.mu.Lock()
	if e.xDaily.cleanupDiagnostic == nil {
		e.xDaily.cleanupDiagnostic = &d
	}
	e.xDaily.mu.Unlock()
}
