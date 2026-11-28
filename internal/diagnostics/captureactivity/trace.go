// Package captureactivity projects local diagnostics for capture process
// launches, owner transitions, worker polls and admitted native-post actions.
package captureactivity

import (
	"encoding/json"
	"log"
	"path/filepath"
	"time"

	"github.com/abangkis/AkuSidecar/internal/diagnostics/recording"
	d "github.com/abangkis/ai4u-diagnostics-go/pkg/diagnostics"
)

// The owner serializes updates to each trace.
type Trace struct {
	logger   *log.Logger
	recorder *recording.Recorder
	start    time.Time
}

func newTrace(logger *log.Logger, op d.Operation, start time.Time) *Trace {
	return &Trace{logger: logger, recorder: recording.New(op), start: start}
}

type record struct {
	Scope      string  `json:"scope,omitempty"`
	DurationMS *int64  `json:"duration_ms,omitempty"`
	Generation *uint64 `json:"generation,omitempty"`
	Foreground string  `json:"foreground,omitempty"`

	Producer    string             `json:"producer,omitempty"`
	Operation   d.Operation        `json:"operation"`
	Phase       d.Phase            `json:"phase"`
	Sequence    uint32             `json:"sequence"`
	ElapsedMS   *int64             `json:"elapsed_ms,omitempty"`
	PID         int                `json:"pid,omitempty"`
	Executable  string             `json:"executable,omitempty"`
	Version     string             `json:"version,omitempty"`
	OK          *bool              `json:"ok,omitempty"`
	Poll        string             `json:"poll,omitempty"`
	Outcome     string             `json:"outcome,omitempty"`
	Queue       *Queue             `json:"queue,omitempty"`
	Action      string             `json:"action,omitempty"`
	RequestID   string             `json:"request_id,omitempty"`
	QueueWaitMS *int64             `json:"queue_wait_ms,omitempty"`
	Reason      string             `json:"reason,omitempty"`
	Verified    *bool              `json:"verified,omitempty"`
	Helper      *helperObservation `json:"helper,omitempty"`
}

type Queue struct {
	Total               int `json:"total"`
	Unclaimed           int `json:"unclaimed"`
	Claimed             int `json:"claimed"`
	NativeUnclaimed     int `json:"native_unclaimed"`
	NativeWaitingBroker int `json:"native_waiting_broker"`
	NativeReady         int `json:"native_ready"`
}

func (t *Trace) emit(phase d.Phase, elapsed d.Observed[time.Duration], event record) {
	t.project(phase, elapsed, event).Write()
}

// Event is a frozen observation. Its zero value writes nothing. Queue owners
// can assign sequence numbers at the transition and write after unlocking.
type Event struct {
	logger *log.Logger
	record record
}

func (t *Trace) project(phase d.Phase, elapsed d.Observed[time.Duration], event record) Event {
	if t.logger == nil {
		return Event{}
	}
	t.recorder.Advance(phase, elapsed)
	s := t.recorder.Snapshot()
	event.Operation, event.Phase, event.Sequence = s.Location.Operation, s.Location.Phase, s.Sequence
	if ms := d.Milliseconds(s.Elapsed, d.DurationPolicy{}); ms.Known {
		event.ElapsedMS = &ms.Value
	}
	return Event{logger: t.logger, record: event}
}

func (e Event) Write() {
	if e.logger == nil {
		return
	}
	if line, err := json.Marshal(e.record); err == nil {
		e.logger.Printf("capture_activity %s", line)
	}
}

func NewLaunch(logger *log.Logger) *Trace { return newTrace(logger, "browser_launch", time.Now()) }

func (t *Trace) Process(phase d.Phase, pid int, executable, version string) {
	t.emit(phase, d.Observed[time.Duration]{Known: true, Value: time.Since(t.start)}, record{
		PID: pid, Executable: filepath.Base(executable), Version: version,
	})
}

func Transition(logger *log.Logger, stage string, elapsed time.Duration, ok *bool) {
	switch stage {
	case "coordinator_wait", "owner_readiness", "owner_release":
		newTrace(logger, "capture_transition", time.Now()).emit(d.Phase(stage), d.Observed[time.Duration]{Known: true, Value: elapsed}, record{OK: ok})
	}
}

type Poll struct {
	trace *Trace
	id    string
}

func BeginPoll(logger *log.Logger, id string) Poll {
	p := Poll{trace: newTrace(logger, "capture_poll", time.Now()), id: id}
	p.trace.emit("start", d.Observed[time.Duration]{}, record{Poll: id, Outcome: "active"})
	return p
}

func (p Poll) End(outcome string, queue Queue, action, requestID string) {
	p.trace.emit("end", d.Observed[time.Duration]{Known: true, Value: time.Since(p.trace.start)}, record{
		Poll: p.id, Outcome: outcome, Queue: &queue, Action: opaqueID(action, "split_"), RequestID: opaqueID(requestID, "broker_"),
	})
}
