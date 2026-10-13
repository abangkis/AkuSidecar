package engine

import "sync"

// Nil in ordinary builds. Only the explicitly tagged synthetic fixture binder
// constructs this gate. It tracks physical collector steps through ingestion.
type fixtureAdmissionGate struct {
	mu         sync.Mutex
	paused     bool
	running    int
	pauseEpoch uint64
}

func (e *Engine) fixtureAdmissionAllowed() bool {
	if e.fixtureAdmission == nil {
		return true
	}
	e.fixtureAdmission.mu.Lock()
	defer e.fixtureAdmission.mu.Unlock()
	return !e.fixtureAdmission.paused
}

func (e *Engine) beginFixtureCaptureStep() bool {
	if e.fixtureAdmission == nil {
		return true
	}
	e.fixtureAdmission.mu.Lock()
	defer e.fixtureAdmission.mu.Unlock()
	if e.fixtureAdmission.paused {
		return false
	}
	e.fixtureAdmission.running++
	return true
}

func (e *Engine) endFixtureCaptureStep() {
	if e.fixtureAdmission == nil {
		return
	}
	e.fixtureAdmission.mu.Lock()
	e.fixtureAdmission.running--
	e.fixtureAdmission.mu.Unlock()
}
