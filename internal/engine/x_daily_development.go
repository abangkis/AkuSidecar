package engine

import (
	"context"
	"errors"
	"sync"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

const xDailyBatchLimitMS = 60000

type xDailyRouting struct {
	mu               sync.RWMutex
	attached, closed bool
}

// XDailySelection owns only the restart-required routing selection. Step two
// supplies the native batch adapter; selection is never a cookie/Store permit.
type XDailySelection struct {
	mu     sync.Mutex
	cfg    config.Config
	engine *Engine
	closed bool
}

type XCaptureStatus struct {
	SelectedMode                 string `json:"selectedMode"`
	EffectiveMode                string `json:"effectiveMode"`
	State                        string `json:"state"`
	Reason                       string `json:"reason"`
	RestartRequiredForModeChange bool   `json:"restartRequiredForModeChange"`
	AuthorizationPolicy          string `json:"authorizationPolicy,omitempty"`
	MaxBatchMS                   int    `json:"maxBatchMs,omitempty"`
	DerivationAuthorized         bool   `json:"derivationAuthorized"`
}

// Prepare runs after config loading but before the ordinary Store/provider/UI.
// It neither opens resources nor routes through the one-use QA selector.
func PrepareXDailyDevelopment(cfg config.Config, options config.Options) (*XDailySelection, error) {
	if err := cfg.Capture.ValidateXMode(); err != nil {
		return nil, err
	}
	if cfg.Capture.XCaptureMode() == config.XCaptureExclusive {
		return nil, nil
	}
	if !xDailyBuildSupported || !cfg.Dev || !options.Dev || cfg.Deployment.Mode != "" && cfg.Deployment.Mode != "development" {
		return nil, errors.New("concurrent X capture requires the Windows auth_session_dev development build")
	}
	if options.DevelopmentSessionReuse != "" || options.DevelopmentXManualPlan != "" || options.DevelopmentQAPilotPlan != "" || options.DevelopmentQAPilotPrepare != "" {
		return nil, errors.New("daily concurrent X capture cannot mix with manual or QA selectors")
	}
	return &XDailySelection{cfg: cfg}, nil
}

func (s *XDailySelection) Attach(e *Engine) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.engine != nil || e == nil || e.store == nil || !e.config.Dev || e.config.Capture.XCaptureMode() != config.XCaptureConcurrent ||
		e.config.Root != s.cfg.Root || e.config.Server != s.cfg.Server || e.config.Database != s.cfg.Database || e.store.Path() != s.cfg.Database.Path {
		return errors.New("daily X ordinary application scope rejected")
	}
	e.xManual.mu.Lock()
	defer e.xManual.mu.Unlock()
	e.xDaily.mu.Lock()
	defer e.xDaily.mu.Unlock()
	if e.xManual.closed || e.xManual.route != nil || e.xManual.starter != nil || e.xDaily.attached || e.xDaily.closed {
		return errors.New("daily X composition already selected or closed")
	}
	e.xDaily.attached = true
	s.engine = e
	return nil
}

func (s *XDailySelection) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	if s.engine != nil {
		s.engine.closeXDailySelection()
	}
	return nil
}

func (e *Engine) XDailyConfigured() bool {
	return e.config.Capture.XCaptureMode() == config.XCaptureConcurrent
}

func (e *Engine) XConcurrentConfigured() bool { return e.XDailyConfigured() || e.XManualConfigured() }

func (e *Engine) XCaptureStatus() XCaptureStatus {
	status := XCaptureStatus{SelectedMode: e.config.Capture.XCaptureMode(), EffectiveMode: config.XCaptureExclusive, State: "selected", Reason: "ordinary-route", RestartRequiredForModeChange: true}
	if !e.XDailyConfigured() {
		return status
	}
	status.EffectiveMode, status.State, status.Reason = "unavailable", "not-ready", "batch-adapter-required"
	status.AuthorizationPolicy, status.MaxBatchMS = "process", xDailyBatchLimitMS
	e.xDaily.mu.RLock()
	defer e.xDaily.mu.RUnlock()
	if e.xDaily.closed {
		status.State, status.Reason = "stopped", "process-authorization-ended"
	}
	return status
}

func (e *Engine) closeXDailySelection() {
	e.xDaily.mu.Lock()
	e.xDaily.closed = true
	e.xDaily.mu.Unlock()
}

func (e *Engine) xDailyUnavailable(source domain.Source) bool {
	return source == domain.SourceX && e.XDailyConfigured()
}

func (e *Engine) dailyCaptureSources(sources []domain.Source) ([]domain.Source, error) {
	if !e.XDailyConfigured() {
		return sources, nil
	}
	allowed := make([]domain.Source, 0, len(sources))
	for _, source := range sources {
		if !e.xDailyUnavailable(source) {
			allowed = append(allowed, source)
		}
	}
	if len(allowed) == 0 && len(sources) > 0 {
		return nil, errors.New("X concurrent capture is not ready: batch-adapter-required")
	}
	return allowed, nil
}

// Reconcile before accepted-observation reasoning resumes. No collector or
// source acquisition is performed here, and non-X work is untouched.
func (e *Engine) ReconcileXDailyStartup(ctx context.Context) error {
	if !e.XDailyConfigured() {
		return nil
	}
	session, err := e.store.ActiveSession(ctx)
	if err != nil || session == nil {
		return err
	}
	for _, run := range session.Runs {
		if run.Source == domain.SourceX {
			if err = e.store.RejectUnavailableXCapture(ctx, run.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

// Continue only after main's ordinary capture composition is installed. This
// also finalizes an X-only session whose unavailable capture was rejected.
func (e *Engine) ResumeXDailyScheduling(ctx context.Context) error {
	if !e.XDailyConfigured() {
		return nil
	}
	session, err := e.store.ActiveSession(ctx)
	if err != nil || session == nil {
		return err
	}
	_, err = e.startNext(ctx, session.ID)
	return err
}
