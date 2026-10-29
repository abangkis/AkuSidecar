package engine

import (
	"context"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

var dailySources = [...]domain.Source{domain.SourceX, domain.SourceInstagram}

// State remains separate even when both sources borrow the same reader profile.
func (e *Engine) dailyController(source domain.Source) dailyController {
	switch source {
	case domain.SourceX:
		return dailyController{e, &e.xDaily, source}
	case domain.SourceInstagram:
		return dailyController{e, &e.instagramDaily, source}
	default:
		panic("unsupported daily capture source")
	}
}

func (d dailyController) configured() bool {
	if d.source == domain.SourceX {
		return d.engine.XDailyConfigured()
	}
	return d.engine.config.Capture.InstagramCaptureMode() == config.XCaptureConcurrent
}

func (e *Engine) dailySourceConfigured(source domain.Source) bool {
	for _, supported := range dailySources {
		if source == supported {
			return e.dailyController(source).configured()
		}
	}
	return false
}

func (e *Engine) dailySourceReady(source domain.Source) bool {
	return e.dailySourceConfigured(source) && e.dailyController(source).ready()
}

func (e *Engine) readyDailySources() []domain.Source {
	var sources []domain.Source
	for _, source := range dailySources {
		if e.dailySourceReady(source) {
			sources = append(sources, source)
		}
	}
	return sources
}

// SourceCaptureStatus is a status projection, never proof or permission.
func (e *Engine) SourceCaptureStatus(source domain.Source) XCaptureStatus {
	if source == domain.SourceX {
		return e.XCaptureStatus()
	}
	if !e.dailySourceConfigured(source) {
		return XCaptureStatus{SelectedMode: "browser", EffectiveMode: "browser", State: "selected", Reason: "ordinary-route", RestartRequiredForModeChange: true}
	}
	return e.dailyController(source).status()
}

// StartInstagramDaily uses the existing source/profile permission policy. No
// packaged native Instagram factory is registered until its proof/manifest is qualified.
func (e *Engine) StartInstagramDaily(ctx context.Context, request XDailyStartRequest) error {
	_, err := e.dailyController(domain.SourceInstagram).start(ctx, request, nil)
	return err
}

// Trial consent is explicit for each process and never qualifies the recipe.
func (e *Engine) StartInstagramTrial(ctx context.Context, request XDailyStartRequest, acknowledgeUnqualified bool) error {
	if !e.config.Dev || !e.config.Capture.InstagramTrial || !acknowledgeUnqualified {
		return e.dailyController(domain.SourceInstagram).failure("trial consent required")
	}
	return e.StartInstagramDaily(ctx, request)
}

func (e *Engine) RevokeInstagramDailyPermission(ctx context.Context) error {
	return e.dailyController(domain.SourceInstagram).revoke(ctx)
}

func (e *Engine) closeInstagramDaily(ctx context.Context) error {
	return e.dailyController(domain.SourceInstagram).close(ctx)
}

func (e *Engine) dailyCleanupPending() error {
	for _, source := range e.readyDailySources() {
		d := e.dailyController(source)
		d.state.mu.RLock()
		busy := d.state.done != nil
		d.state.mu.RUnlock()
		if busy {
			return d.failure("collector cleanup is pending")
		}
	}
	return nil
}

func (d dailyController) selectedMode() string {
	if d.source == domain.SourceX {
		return d.engine.config.Capture.XCaptureMode()
	}
	return d.engine.config.Capture.InstagramCaptureMode()
}

func (e *Engine) ordinaryExcludedSources() []domain.Source {
	var sources []domain.Source
	e.xManual.mu.RLock()
	defer e.xManual.mu.RUnlock()
	if e.XDailyConfigured() || e.xManual.starting || e.xManual.route != nil && e.xManual.route.occupied() {
		sources = append(sources, domain.SourceX)
	}
	if e.dailySourceConfigured(domain.SourceInstagram) {
		sources = append(sources, domain.SourceInstagram)
	}
	return sources
}
