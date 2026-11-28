package captureactivity

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/abangkis/AkuSidecar/internal/readerbroker"
	d "github.com/abangkis/ai4u-diagnostics-go/pkg/diagnostics"
)

// ReaderBroker follows one invocation. Only its authenticated owner may attach
// an action after matching the request ID, source and URL.
type ReaderBroker struct {
	trace             *Trace
	requestID, action string
	queueWait         *int64
}

type helperObservation struct {
	DurationMS    *int64 `json:"duration_ms,omitempty"`
	AfterExpiry   bool   `json:"after_expiry"`
	ContextEnded  bool   `json:"context_ended"`
	Applied       *bool  `json:"applied,omitempty"`
	Readback      *bool  `json:"readback,omitempty"`
	FocusCategory string `json:"focus_category,omitempty"`
	Visible       *bool  `json:"reader_visible,omitempty"`
	Minimized     *bool  `json:"reader_minimized,omitempty"`
}

func durationMS(duration time.Duration) *int64 {
	ms := d.Milliseconds(d.Observed[time.Duration]{Known: duration >= 0, Value: duration}, d.DurationPolicy{})
	if !ms.Known {
		return nil
	}
	return &ms.Value
}

func BeginReaderBroker(logger *log.Logger, requestID string, received time.Time) *ReaderBroker {
	b := &ReaderBroker{trace: newTrace(logger, "reader_broker", received), requestID: opaqueID(requestID, "broker_")}
	b.observe("received", received, record{Outcome: "active"})
	return b
}

// A replay or registration conflict can share an ID with a live conversation.
// Its receipt must never look like the incumbent conversation's terminal result.
func ReaderBrokerNotAdmitted(logger *log.Logger, requestID string, received time.Time, err error) {
	outcome := "rejected"
	if errors.Is(err, context.Canceled) {
		outcome = "cancelled"
	}
	newTrace(logger, "reader_broker_admission", received).emit("not_admitted",
		d.Observed[time.Duration]{Known: true, Value: time.Since(received)}, record{
			Producer: "sidecar", RequestID: opaqueID(requestID, "broker_"), Outcome: outcome,
		})
}

func (b *ReaderBroker) observe(phase d.Phase, at time.Time, event record) {
	event.Producer, event.RequestID, event.Action = "sidecar", b.requestID, b.action
	b.trace.emit(phase, d.Observed[time.Duration]{Known: true, Value: at.Sub(b.trace.start)}, event)
}

func (b *ReaderBroker) Attached(action string, queued, attached time.Time) {
	b.action = opaqueID(action, "split_")
	if !queued.IsZero() {
		b.queueWait = durationMS(attached.Sub(queued))
	}
	b.observe("attached", attached, record{Outcome: "accepted", QueueWaitMS: b.queueWait})
}

func (b *ReaderBroker) Activating() {
	b.observe("activation_started", time.Now(), record{Outcome: "active"})
}

func (b *ReaderBroker) HelperReturned(started, returned, expires time.Time, contextEnded bool, reply readerbroker.Reply, err error) {
	helper := &helperObservation{DurationMS: durationMS(returned.Sub(started)), AfterExpiry: !returned.Before(expires), ContextEnded: contextEnded}
	outcome := "unavailable"
	if err == nil {
		outcome = "observed"
		helper.Applied, helper.Readback = &reply.Applied, &reply.Readback
		helper.FocusCategory = readerbroker.DiagnosticFocusCategory(reply)
		helper.Visible, helper.Minimized = reply.ReaderVisible, reply.ReaderMinimized
	}
	b.observe("helper_reply", returned, record{Outcome: outcome, Reason: readerbroker.ActivationReason(reply, err), Helper: helper})
}

func (b *ReaderBroker) Activation(verified, attempted, manual bool) {
	event := record{Outcome: "unverified"}
	if attempted {
		event.Verified = &verified
		if verified {
			event.Outcome = "accepted"
		} else {
			event.Reason = "post_activation_completion_failed"
		}
	}
	if manual {
		event.Outcome = "manual_required"
	}
	b.observe("activation", time.Now(), event)
}

func (b *ReaderBroker) Finish(err error, stopped, manual bool) {
	outcome := "rejected"
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		outcome = "timed_out"
	case errors.Is(err, context.Canceled):
		outcome = "cancelled"
	case stopped:
		outcome = "stopped"
	case manual:
		outcome = "manual_required"
	case err == nil:
		outcome = "accepted"
	}
	b.observe("terminal", time.Now(), record{Outcome: outcome, QueueWaitMS: b.queueWait})
}

// Reader relay requests have an action ID. Only the pipe owner binds a broker request ID.
func ReaderRelay(logger *log.Logger, action string, phase d.Phase, started time.Time, outcome string) {
	id := opaqueID(action, "split_")
	if id == "" || (phase != "prepare" && phase != "foreground") {
		return
	}
	switch outcome {
	case "accepted", "rejected", "missing_intent":
	default:
		return
	}
	newTrace(logger, "reader_relay", started).emit(phase, d.Observed[time.Duration]{Known: true, Value: time.Since(started)}, record{Producer: "sidecar", Action: id, Outcome: outcome})
}
