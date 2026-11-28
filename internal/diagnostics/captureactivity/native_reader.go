package captureactivity

import (
	"context"
	"errors"
	"log"
	"time"

	d "github.com/abangkis/ai4u-diagnostics-go/pkg/diagnostics"
)

// NativeReader measures consecutive phases owned by one synchronous caller.
type NativeReader struct {
	trace *Trace
	base  record
	phase d.Phase
	since time.Time
}

func BeginNativeReader(logger *log.Logger, scope, action, phase string) *NativeReader {
	now := time.Now()
	return &NativeReader{trace: newTrace(logger, "native_reader", now), base: record{
		Producer: "sidecar", Scope: scope, Action: opaqueID(action, "split_"),
	}, phase: d.Phase(phase), since: now}
}

func BeginReaderLaunch(logger *log.Logger, generation uint64) *NativeReader {
	n := BeginNativeReader(logger, "launch", "", "chrome_launch")
	n.base.Generation = &generation
	return n
}

func (n *NativeReader) observe(outcome string) {
	now := time.Now()
	event := n.base
	event.DurationMS, event.Outcome = durationMS(now.Sub(n.since)), outcome
	n.trace.emit(n.phase, d.Observed[time.Duration]{Known: true, Value: now.Sub(n.trace.start)}, event)
}

func (n *NativeReader) Next(phase d.Phase) {
	n.Finish(nil)
	n.phase, n.since = phase, time.Now()
}

func (n *NativeReader) Finish(err error) {
	outcome := "accepted"
	switch {
	case errors.Is(err, context.Canceled):
		outcome = "cancelled"
	case errors.Is(err, context.DeadlineExceeded):
		outcome = "timed_out"
	case err != nil:
		outcome = "rejected"
	}
	n.observe(outcome)
}

// Total records only the HTTP request lifetime, including cancellation.
func (n *NativeReader) Total() { n.observe("observed") }
