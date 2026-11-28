package captureactivity

import (
	"log"
	"strings"
	"time"

	d "github.com/abangkis/ai4u-diagnostics-go/pkg/diagnostics"
)

// SplitAction belongs to one admitted open_native_post. Its caller serializes
// observations with the queue lock. Logging happens after the lock is released.
type SplitAction struct {
	trace     *Trace
	action    string
	requestID string
	settled   bool
}

func opaqueID(value, prefix string) string {
	digits, ok := strings.CutPrefix(value, prefix)
	if !ok || len(digits) != 32 || strings.Trim(digits, "0123456789abcdef") != "" {
		return ""
	}
	return value
}

func AdmitSplitAction(logger *log.Logger, action, requestID string, queuedAt time.Time, queue Queue) (*SplitAction, Event) {
	id := opaqueID(action, "split_")
	if logger == nil || id == "" {
		return nil, Event{}
	}
	a := &SplitAction{trace: newTrace(logger, "split_action", queuedAt), action: id, requestID: opaqueID(requestID, "broker_")}
	return a, a.project("queued", "accepted", &queue, 0)
}

func (a *SplitAction) project(phase d.Phase, outcome string, queue *Queue, elapsed time.Duration) Event {
	return a.trace.project(phase, d.Observed[time.Duration]{Known: true, Value: elapsed}, record{
		Producer: "sidecar", Action: a.action, RequestID: a.requestID, Outcome: outcome, Queue: queue,
	})
}

// Result records the first committed result, even when the HTTP waiter left.
func (a *SplitAction) Result(ok, verified, manual bool) Event {
	if a == nil || a.settled {
		return Event{}
	}
	a.settled = true
	outcome := "rejected"
	if ok {
		outcome = "accepted"
	}
	event := a.project("result", outcome, nil, time.Since(a.trace.start))
	switch {
	case manual:
		event.record.Foreground = "manual_required"
	case verified:
		event.record.Foreground = "verified"
	default:
		event.record.Foreground = "unverified"
	}
	return event
}

type WaitOutcome string

const (
	ClientCancelled  WaitOutcome = "client_cancelled"
	TimedOut         WaitOutcome = "timed_out"
	TransportStopped WaitOutcome = "transport_stopped"
)

func (a *SplitAction) WaitEnded(outcome WaitOutcome) Event {
	if a == nil {
		return Event{}
	}
	switch outcome {
	case ClientCancelled, TimedOut, TransportStopped:
		return a.project("wait_ended", string(outcome), nil, time.Since(a.trace.start))
	}
	return Event{}
}
