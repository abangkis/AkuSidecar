package engine

import "context"

// engineWorkState tracks in-memory work admission, reruns and drain accounting.
// Engine.mu protects every access. These methods never acquire locks, launch
// goroutines, release leases or touch durable state; callers retain their
// existing atomic sections and cleanup ordering.
type engineWorkState struct {
	active         map[string]context.CancelFunc
	pending        map[string]bool // Presence queues a rerun; the value is allowPlanning.
	cancelled      map[string]bool
	processWorkers int // Includes workers still performing deferred cleanup.
	shuttingDown   bool
}

func newEngineWorkState() engineWorkState {
	return engineWorkState{
		active:    map[string]context.CancelFunc{},
		pending:   map[string]bool{},
		cancelled: map[string]bool{},
	}
}

// admitRun preserves the run-specific admission policy. Other background tasks
// keep their own existing admission checks before registering active work.
func (w *engineWorkState) admitRun(runID string, allowPlanning, queueIfActive bool) bool {
	if w.shuttingDown || w.cancelled[runID] {
		return false
	}
	if _, exists := w.active[runID]; exists {
		if queueIfActive {
			queuedAllowPlanning, queued := w.pending[runID]
			w.pending[runID] = allowPlanning || (queued && queuedAllowPlanning)
		}
		return false
	}
	return true
}

func (w *engineWorkState) registerActive(key string, cancel context.CancelFunc) {
	w.active[key] = cancel
}

func (w *engineWorkState) retireActive(key string) {
	delete(w.active, key)
}

func (w *engineWorkState) registerRun(runID string, cancel context.CancelFunc) {
	w.registerActive(runID, cancel)
	w.workerStarted()
}

// retireRun releases active admission while keeping the worker counted until
// its caller has completed all deferred session/capture cleanup.
func (w *engineWorkState) retireRun(runID string, succeeded bool) (allowPlanning, relaunch bool) {
	w.retireActive(runID)
	allowPlanning, pending := w.pending[runID]
	delete(w.pending, runID)
	return allowPlanning, succeeded && pending && !w.cancelled[runID] && !w.shuttingDown
}

func (w *engineWorkState) prepareCancellation() {
	if w.cancelled == nil {
		w.cancelled = map[string]bool{}
	}
}

func (w *engineWorkState) cancelRun(runID string) {
	w.cancelled[runID] = true
	delete(w.pending, runID)
	if cancel, active := w.active[runID]; active {
		cancel()
	}
}

func (w *engineWorkState) workerStarted()  { w.processWorkers++ }
func (w *engineWorkState) workerFinished() { w.processWorkers-- }

func (w *engineWorkState) beginShutdown() { w.shuttingDown = true }

func (w *engineWorkState) cancelActive() {
	for _, cancel := range w.active {
		cancel()
	}
}

type engineWorkSnapshot struct {
	active, pending, workers int
	shuttingDown             bool
}

// Counts are separate observations, not a number of distinct jobs. Consumers
// retain their existing readiness predicates and additional source/owner gates.
func (w *engineWorkState) snapshot() engineWorkSnapshot {
	return engineWorkSnapshot{len(w.active), len(w.pending), w.processWorkers, w.shuttingDown}
}
