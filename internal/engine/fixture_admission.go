package engine

import (
	"context"
	"errors"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"sync"
)

// Nil in ordinary builds. Explicit fixture or development bindings construct
// this gate. It tracks physical collector steps through ingestion.
type fixtureAdmissionGate struct {
	mu              sync.Mutex
	paused          bool
	running         int
	pauseEpoch      uint64
	developmentHold bool
	// Set only by the synthetic fixture to observe committed pause before lease release.
	pauseObserver func(context.Context, string) error
}

func (e *Engine) validateDevelopmentSelection(settings domain.Settings) error {
	if e.developmentSources == nil {
		return nil
	}
	if settings.CollectionMode != "headless" {
		return errors.New("development session requires headless collection")
	}
	for _, source := range settings.ActiveSources {
		if !e.developmentSources[source] {
			return errors.New("development session source unsupported")
		}
	}
	return nil
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
