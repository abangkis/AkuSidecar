package engine

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

const xDailyBatchLimitMS = 60000

type xDailyRouting = sourceDailyRouting

type sourceDailyRouting struct {
	mu                         sync.RWMutex
	permissionMu               sync.Mutex
	attached, closed           bool
	owner                      xDailyNativeOwner
	parent                     context.Context
	authorized                 bool
	generation                 uint64
	done                       chan struct{}
	runID                      string
	workers                    int
	drain                      chan struct{}
	cancel                     context.CancelFunc
	reason                     string
	failurePhase               string
	firstFailure               error // Private typed cause; cleanup cannot replace it.
	failureCode                string
	factory                    func(context.Context, context.Context) (xDailyNativeOwner, error)
	profileScope               string
	rememberedPermission       bool
	permissionRevision         uint64
	permissionRestoreAttempted bool
	permissionRestoreDone      chan struct{}
	startUsed                  bool
	cleanupUncertain           bool
	cleanupDiagnostic          *XDailyCleanupDiagnostic
	startDone                  chan struct{}
	cancelStart                context.CancelFunc
	retryCount                 int
	retryAt                    time.Time
	retryWaiting               bool
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
	SelectedMode                 string                   `json:"selectedMode"`
	EffectiveMode                string                   `json:"effectiveMode"`
	State                        string                   `json:"state"`
	Reason                       string                   `json:"reason"`
	RestartRequiredForModeChange bool                     `json:"restartRequiredForModeChange"`
	AuthorizationPolicy          string                   `json:"authorizationPolicy,omitempty"`
	RememberedPermission         bool                     `json:"rememberedPermission"`
	MaxBatchMS                   int                      `json:"maxBatchMs,omitempty"`
	DerivationAuthorized         bool                     `json:"derivationAuthorized"`
	FailurePhase                 string                   `json:"failurePhase,omitempty"`
	FailureCode                  string                   `json:"failureCode,omitempty"`
	RetryCount                   int                      `json:"retryCount,omitempty"`
	RetryLimit                   int                      `json:"retryLimit,omitempty"`
	RetryAt                      string                   `json:"retryAt,omitempty"`
	MaintenanceHeld              bool                     `json:"maintenanceHeld,omitempty"`
	StartupDiagnostic            *XDailyStartupDiagnostic `json:"startupDiagnostic,omitempty"`
	IGDiagnostic                 json.RawMessage          `json:"igDiagnostic,omitempty"`
	CleanupDiagnostic            *XDailyCleanupDiagnostic `json:"cleanupDiagnostic,omitempty"`
	Cleanup                      string                   `json:"cleanup,omitempty"`
	CleanupScope                 string                   `json:"cleanupScope,omitempty"`
	ReaderBootstrapLifetime      string                   `json:"readerBootstrapLifetime,omitempty"`
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
	if cfg.Capture.XUsesMainProfile() && (!cfg.WindowsCaptureSplit || !options.AppShell) {
		return nil, errors.New("main-profile X concurrency requires the ordinary Windows capture/UI split")
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
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return s.engine.closeXDaily(ctx)
	}
	return nil
}

func (e *Engine) XDailyConfigured() bool {
	return e.config.Capture.XCaptureMode() == config.XCaptureConcurrent
}

func (e *Engine) XConcurrentConfigured() bool { return e.XDailyConfigured() || e.XManualConfigured() }

func (e *Engine) XCaptureStatus() XCaptureStatus { return e.dailyController(domain.SourceX).status() }

func (d dailyController) status() XCaptureStatus {
	e := d.engine
	status := XCaptureStatus{SelectedMode: d.selectedMode(), EffectiveMode: config.XCaptureExclusive, State: "selected", Reason: "ordinary-route", RestartRequiredForModeChange: true}
	if !d.configured() {
		return status
	}
	status.EffectiveMode, status.State, status.Reason = "unavailable", "not-ready", "batch-adapter-required"
	status.AuthorizationPolicy, status.MaxBatchMS = "profile", xDailyBatchLimitMS
	status.ReaderBootstrapLifetime = "process"
	if d.source == domain.SourceInstagram || e.config.Capture.XUsesMainProfile() {
		status.ReaderBootstrapLifetime = "batch"
	}
	d.state.mu.RLock()
	defer d.state.mu.RUnlock()
	status.RememberedPermission = d.state.rememberedPermission
	status.MaintenanceHeld = d.maintenanceHeld()
	status.RetryCount, status.RetryLimit = d.state.retryCount, dailyRetryLimit
	if !d.state.retryAt.IsZero() {
		status.RetryAt = d.state.retryAt.UTC().Format(time.RFC3339Nano)
	}
	if d.state.cleanupDiagnostic != nil {
		detail := d.state.cleanupDiagnostic.closed()
		status.CleanupDiagnostic = &detail
	}
	status.FailurePhase = d.state.failurePhase
	status.FailureCode = d.state.failureCode
	status.StartupDiagnostic = xDailyStartupDetail(d.state.firstFailure)
	if d.source == domain.SourceInstagram {
		status.IGDiagnostic = instagramFailureDiagnostic(d.state.firstFailure)
	}
	if d.state.owner != nil {
		status.Cleanup = "verified"
		status.CleanupScope = "batch_collector"
	}
	if d.state.done != nil && !d.state.retryWaiting {
		status.Cleanup = "pending"
	}
	if d.state.startDone != nil {
		select {
		case <-d.state.startDone:
		default:
			status.Cleanup = "pending"
		}
	}
	if d.state.cleanupUncertain {
		status.Cleanup = "uncertain"
	}
	if d.state.owner != nil {
		status.State, status.Reason = "stopped", d.state.reason
		if d.state.authorized && d.state.parent.Err() == nil {
			status.EffectiveMode, status.State, status.Reason, status.DerivationAuthorized = "concurrent", "ready", "", true
			if d.state.done != nil {
				status.State = "capturing"
			}
			if d.state.retryAt.After(e.store.Now()) {
				status.State, status.Reason = "retry-wait", "transient-batch-failure"
			}
		}
	}
	if d.state.owner == nil && d.state.reason != "" {
		status.State, status.Reason = "disabled", d.state.reason
	}
	if d.state.owner == nil && d.state.factory != nil && !d.state.startUsed && d.state.reason == "" {
		status.State, status.Reason = "disabled", "process-consent-required"
		if status.MaintenanceHeld {
			status.Reason = "maintenance-confirmation-required"
		}
	}
	if d.state.closed {
		status.State = "stopped"
		if d.state.reason == "" || d.state.reason == "opted-out" {
			status.Reason = "process-authorization-ended"
		} else {
			status.Reason = d.state.reason
		}
	}
	return status
}

func (e *Engine) closeXDailySelection() {
	e.xDaily.mu.Lock()
	e.xDaily.closed = true
	e.xDaily.mu.Unlock()
}

func (e *Engine) xDailyUnavailable(source domain.Source) bool {
	return e.dailySourceConfigured(source) && !e.dailySourceReady(source)
}

func (e *Engine) dailyCaptureSources(sources []domain.Source) ([]domain.Source, error) {

	allowed := make([]domain.Source, 0, len(sources))
	for _, source := range sources {
		if !e.xDailyUnavailable(source) {
			allowed = append(allowed, source)
		}
	}
	if len(allowed) == 0 && len(sources) > 0 {
		if !e.XDailyConfigured() {
			return nil, errors.New("Instagram concurrent capture is not ready: native-adapter-unqualified")
		}
		return nil, errors.New("X concurrent capture is not ready: batch-adapter-required")
	}
	return allowed, nil
}

// Reconcile before accepted-observation reasoning resumes. No collector or
// source acquisition is performed here, and ordinary unfenced work is untouched.
func (e *Engine) ReconcileXDailyStartup(ctx context.Context) error {
	session, err := e.store.ActiveSession(ctx)
	if err != nil || session == nil {
		return err
	}
	for _, run := range session.Runs {
		fenced, fenceErr := e.store.HasDailySourceFence(ctx, run.Source, run.ID)
		if fenceErr != nil {
			return fenceErr
		}
		if e.dailySourceConfigured(run.Source) || fenced {
			if err = e.store.ReconcileClosedDailySourceCapture(ctx, run.Source, run.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

// Continue only after main's ordinary capture composition is installed. This
// also finalizes an X-only session whose unavailable capture was rejected.
func (e *Engine) ResumeXDailyScheduling(ctx context.Context) error {
	for _, source := range dailySources {
		if e.dailySourceConfigured(source) {
			if err := e.dailyController(source).resume(ctx); err != nil {
				return err
			}
		}
	}
	return nil
}

func (d dailyController) resume(ctx context.Context) error {
	e := d.engine
	if !d.configured() {
		return nil
	}
	if ctx == nil {
		return d.failure("scheduling context required")
	}
	d.state.mu.Lock()
	shouldRestore := d.state.factory != nil && !d.state.permissionRestoreAttempted
	profileScope := d.state.profileScope
	revision := d.state.permissionRevision
	var restoreDone chan struct{}
	if shouldRestore {
		d.state.permissionRestoreAttempted = true
		restoreDone = make(chan struct{})
		d.state.permissionRestoreDone = restoreDone
	} else {
		restoreDone = d.state.permissionRestoreDone
	}
	d.state.mu.Unlock()
	if shouldRestore {
		func() {
			defer func() {
				d.state.mu.Lock()
				if d.state.permissionRestoreDone == restoreDone {
					d.state.permissionRestoreDone = nil
				}
				close(restoreDone)
				d.state.mu.Unlock()
			}()
			remembered, permissionErr := e.store.DailySourcePermission(ctx, d.source, profileScope)
			if !d.publishRestoredPermission(revision, remembered, permissionErr) {
				return
			}
			if permissionErr == nil && remembered {
				d.restoreStart(ctx, revision)
			}
		}()
	} else if restoreDone != nil {
		select {
		case <-restoreDone:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	session, err := e.store.ActiveSession(ctx)
	if err != nil || session == nil {
		return err
	}
	_, err = e.startNext(ctx, session.ID)
	return err
}

func (e *Engine) restoreXDailyStart(ctx context.Context, revision uint64) {
	e.dailyController(domain.SourceX).restoreStart(ctx, revision)
}

func (d dailyController) restoreStart(ctx context.Context, revision uint64) {
	admitted, err := d.start(ctx, XDailyStartRequest{AcknowledgeLimitations: true, AllowProcessSessionDerivation: true}, &revision)
	if !admitted || err == nil {
		return
	}
	d.state.mu.Lock()
	defer d.state.mu.Unlock()
	if d.state.permissionRevision == revision+1 && d.state.reason == "" {
		d.state.startUsed = true
		d.state.reason = "permission-start-failed"
	}
}

func (e *Engine) publishRestoredXDailyPermission(revision uint64, remembered bool, readErr error) bool {
	return e.dailyController(domain.SourceX).publishRestoredPermission(revision, remembered, readErr)
}

func (d dailyController) publishRestoredPermission(revision uint64, remembered bool, readErr error) bool {
	d.state.mu.Lock()
	defer d.state.mu.Unlock()
	if d.state.permissionRevision != revision {
		return false
	}
	d.state.rememberedPermission = remembered && readErr == nil
	if readErr != nil {
		d.state.authorized = false
		d.state.reason = "permission-read-failed"
	}
	return true
}
