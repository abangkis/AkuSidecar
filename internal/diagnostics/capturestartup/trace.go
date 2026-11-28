// Package capturestartup records the first split owner of a development process.
package capturestartup

import (
	"log"
	"sync"
	"time"

	"github.com/abangkis/AkuSidecar/internal/diagnostics/recording"
	d "github.com/abangkis/ai4u-diagnostics-go/pkg/diagnostics"
)

type Document struct {
	Number         int
	Loaded         bool
	ScriptStarted  bool
	WorkerReceived bool
	Reload         bool
}

type Trace struct {
	mu        sync.Mutex
	started   time.Time
	recorder  *recording.Recorder
	log       *log.Logger
	seen      map[event]bool
	documents int
	retired   bool
}

type event struct {
	phase    d.Phase
	document int
}

func New(logger *log.Logger) *Trace {
	return &Trace{started: time.Now(), recorder: recording.New("capture_startup"), log: logger, seen: make(map[event]bool)}
}

// Retire fences late samples and heartbeats when the process replaces its initial owner.
func (t *Trace) Retire() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.retired = true
}

func (t *Trace) Launch() { t.record("capture_launch", nil) }

// Number documents independently of URL and navigation type.
func (t *Trace) HostServed() int {
	if t == nil {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.retired || t.documents >= 8 {
		return 0
	}
	t.documents++
	t.emitLocked("host_document_served", t.documents, nil)
	return t.documents
}
func (t *Trace) Navigation(accepted bool) {
	if accepted {
		t.record("navigation_accepted", nil)
	} else {
		t.record("navigation_failed", nil)
	}
}
func (t *Trace) Bootstrap() { t.record("bootstrap_accepted", nil) }
func (t *Trace) Heartbeat() { t.record("heartbeat_recorded", nil) }
func (t *Trace) Sample(doc Document, final bool) {
	if final {
		t.record("late_sample", &doc)
	} else {
		t.record("early_sample", &doc)
	}
}

func (t *Trace) record(phase d.Phase, doc *Document) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	number := 0
	if doc != nil {
		if doc.Number <= 0 || doc.Number > t.documents {
			return
		}
		number = doc.Number
	}
	t.emitLocked(phase, number, doc)
}

func (t *Trace) emitLocked(phase d.Phase, number int, doc *Document) {
	key := event{phase, number}
	if t.retired || t.seen[key] || t.log == nil {
		return
	}
	t.seen[key] = true
	t.recorder.Advance(phase, d.Observed[time.Duration]{Known: true, Value: time.Since(t.started)})
	s := t.recorder.Snapshot()
	ms := d.Milliseconds(s.Elapsed, d.DurationPolicy{})
	if doc != nil {
		t.log.Printf("%s seq=%d phase=%s elapsed_ms=%d doc=%d loaded=%t reload=%t script_started=%t worker_receipt=%t", s.Location.Operation, s.Sequence, phase, ms.Value, number, doc.Loaded, doc.Reload, doc.ScriptStarted, doc.WorkerReceived)
		return
	}
	t.log.Printf("%s seq=%d phase=%s elapsed_ms=%d doc=%d", s.Location.Operation, s.Sequence, phase, ms.Value, number)
}
