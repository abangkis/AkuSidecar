// Package diagnostictrace records observations. It grants no execution authority.
package diagnostictrace

import (
	d "github.com/abangkis/ai4u-diagnostics-go/pkg/diagnostics"
	"sync"
	"time"
)

type Failure struct {
	Location  d.Location
	Condition string
}
type Settlement struct{ State, Condition string }
type Snapshot struct {
	Location          d.Location
	Completed         d.Phase
	Sequence          uint32
	Elapsed           d.Observed[time.Duration]
	Cause             *Failure
	Cleanup, Delivery Settlement
	Disposition       string
	Complete          bool
}
type Recorder struct {
	mu    sync.Mutex
	value Snapshot
}

func New(operation d.Operation) *Recorder {
	return &Recorder{value: Snapshot{Location: d.Location{Operation: operation, Phase: "none"}, Completed: "none", Disposition: "not_started", Cleanup: Settlement{State: "not_started", Condition: "none"}, Delivery: Settlement{State: "pending", Condition: "none"}, Complete: true}}
}
func (r *Recorder) Advance(phase d.Phase, elapsed d.Observed[time.Duration]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.value.Location.Phase != phase {
		r.value.Location.Phase = phase
	}
	r.value.Sequence++
	r.value.Elapsed = elapsed
}
func (r *Recorder) Completed(phase d.Phase) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.value.Completed = phase
}
func (r *Recorder) Fail(location d.Location, condition string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.value.Cause == nil {
		r.value.Cause = &Failure{location, condition}
	}
}
func (r *Recorder) Cleanup(state, condition string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.value.Cleanup = Settlement{state, condition}
}
func (r *Recorder) Delivery(state, condition string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.value.Delivery = Settlement{state, condition}
}
func (r *Recorder) Disposition(state string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.value.Disposition = state
}
func (r *Recorder) Incomplete() { r.mu.Lock(); defer r.mu.Unlock(); r.value.Complete = false }
func (r *Recorder) Snapshot() Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.value
	if s.Cause != nil {
		c := *s.Cause
		s.Cause = &c
	}
	return s
}
