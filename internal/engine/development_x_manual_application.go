//go:build auth_session_dev

package engine

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
)

// Only trusted Go construction supplies these ports. A packaged driver, status
// flag, or JSON report cannot install a route or issue a Store permit.
type xManualApplicationPorts struct {
	runtime *nativePilotRuntime
	broker  *nativePilotBroker
	// Native capture pins exist only after broker startup. Synthetic callers
	// may supply an already constructed runtime instead of this private phase.
	buildRuntime func(context.Context) (*nativePilotRuntime, error)
	admission    nativePilotStoreAdmission
	prepare      func(context.Context, readerbroker.Request, string) (readerbroker.NativePostPreparation, error)
	dispose      func(context.Context) error
}

func (p xManualApplicationPorts) startupBroker() *nativePilotBroker {
	if p.broker != nil {
		return p.broker
	}
	if p.runtime != nil {
		return p.runtime.broker
	}
	return nil
}

type xManualApplicationFactory func(context.Context, context.Context) (xManualApplicationPorts, error)

type xManualApplication struct {
	mu                              sync.Mutex
	e                               *Engine
	ports                           xManualApplicationPorts
	state, reason, cleanup          string
	command                         string
	deadline                        time.Time
	ready                           chan struct{}
	work                            context.Context
	cancel, cancelStartup           context.CancelFunc
	stepOnce, stopOnce, disposeOnce sync.Once
	stopped                         chan struct{}
	disposeErr                      error
	published                       bool
	captureStarted                  bool
	captureDone                     chan struct{}
	startupFailure                  error
	captureResult                   nativePilotStoreResult
	captureResultReady              bool
}

// Reserve before any owner acquisition. Existing Store source occupancy is
// checked again transactionally by the unchanged manual claim path.
func (e *Engine) startXManualApplication(ctx, parent context.Context, session, run, command string, factory xManualApplicationFactory) (*xManualApplication, error) {
	if e == nil || !e.config.Dev || e.store == nil || ctx == nil || parent == nil || factory == nil || ctx.Err() != nil || parent.Err() != nil || command == "" {
		return nil, errors.New("manual application unavailable")
	}
	v, err := e.store.GetRun(ctx, run)
	if err != nil || v.Source != domain.SourceX || v.SessionID != session {
		return nil, errors.New("manual run scope rejected")
	}
	work, cancel := context.WithCancel(parent)
	// Owner/home preparation is preflight; paired account sampling retains its
	// own unchanged 18-second envelope and the broker's original attempt clock.
	startupBase, cancelStartup := context.WithCancel(work)
	preflight, cancelPreflight := context.WithTimeout(startupBase, time.Duration(nativePilotTimingPolicy["collectorHomeReadyMS"])*time.Millisecond)
	defer cancelPreflight()
	r := &xManualApplication{e: e, state: "starting", command: command, ready: make(chan struct{}), stopped: make(chan struct{}), captureDone: make(chan struct{}), work: work, cancel: cancel, cancelStartup: cancelStartup}
	e.xManual.mu.Lock()
	if e.xManual.closed || e.xManual.route != nil {
		e.xManual.mu.Unlock()
		cancelStartup()
		cancel()
		return nil, errors.New("manual application already selected")
	}
	busy, busyErr := e.store.SourceCaptureBusy(ctx, domain.SourceX)
	if busyErr != nil || busy {
		e.xManual.mu.Unlock()
		cancelStartup()
		cancel()
		return nil, errors.New("manual X source busy")
	}
	e.xManual.route = r
	e.xManual.mu.Unlock()
	requestStop := context.AfterFunc(ctx, cancelStartup)
	p, err := factory(preflight, parent)
	cancelPreflight()
	r.mu.Lock()
	r.ports = p
	r.mu.Unlock()
	b := p.startupBroker()
	if err == nil && (b == nil || p.runtime == nil && p.buildRuntime == nil || p.admission == nil || p.prepare == nil || p.dispose == nil || p.runtime != nil && p.runtime.broker != b) {
		err = errors.New("manual ports incomplete")
	}
	startup, cancelPair := context.WithTimeout(startupBase, nativePilotStartup)
	defer cancelPair()
	if err == nil {
		d := b.start(startup)
		offline, isOffline := p.admission.(interface{ syntheticStartupAllowed() bool })
		if !d.ManualEligible && !(d.OfflineValidated && isOffline && offline.syntheticStartupAllowed()) {
			err = errors.New("manual startup rejected")
		}
	}
	if err == nil && p.runtime == nil {
		p.runtime, err = p.buildRuntime(startup)
		if err == nil && (p.runtime == nil || p.runtime.broker != b) {
			err = errors.New("manual runtime startup mismatch")
		}
		r.mu.Lock()
		r.ports = p
		r.mu.Unlock()
	}
	if err == nil {
		err = e.bindNativePilotStoreAdmission(startup, p.runtime, session, run, p.admission)
	}
	if err == nil {
		var end time.Time
		binding, checkErr := p.admission.validate(startup)
		if checkErr != nil {
			err = errors.New("manual startup invalidated")
		} else {
			end = binding.Deadline
		}
		r.mu.Lock()
		if r.state != "starting" || startup.Err() != nil {
			err = errors.New("manual startup canceled")
		}
		if err == nil {
			r.state, r.deadline = "active", end
			r.published = true
		}
		r.mu.Unlock()
	}
	requestStop()
	cancelStartup()
	close(r.ready)
	if err != nil {
		r.mu.Lock()
		r.startupFailure = err
		r.mu.Unlock()
		_ = r.stop(context.Background(), "startup-failed")
		return r, errors.New("manual application startup failed")
	}
	context.AfterFunc(parent, func() {
		call, cancel := context.WithTimeout(context.Background(), nativePilotCleanup)
		defer cancel()
		_ = r.stop(call, "parent-exited")
	})
	return r, nil
}

func (r *xManualApplication) status() XManualStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	if b := r.ports.startupBroker(); (r.state == "active" || r.state == "stopped" && r.cleanup == "pending") && b != nil {
		b.mu.Lock()
		if b.stopped {
			r.state = "stopped"
			if r.reason == "" {
				r.reason = b.firstStopReason
			}
			r.cleanup = "pending"
			select {
			case <-b.cleanupDone:
				if b.cleanupFailed {
					r.state, r.cleanup = "failed", "uncertain"
				} else {
					r.cleanup = "verified"
					if r.captureStarted {
						select {
						case <-r.captureDone:
						default:
							r.cleanup = "pending"
						}
					}
				}
			default:
			}
		}
		b.mu.Unlock()
	}
	remaining := int64(0)
	if r.state == "active" {
		remaining = max(r.deadline.Sub(r.e.store.Now()).Milliseconds(), 0)
	}
	return XManualStatus{Source: "x", State: r.state, Reason: r.reason, Available: r.state == "active", RemainingMS: remaining, Cleanup: r.cleanup}
}
func (r *xManualApplication) occupied() bool {
	r.status()
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cleanup != "verified"
}

func (r *xManualApplication) stop(ctx context.Context, reason string) error {
	if ctx == nil {
		return errors.New("manual stop context required")
	}
	r.stopOnce.Do(func() {
		r.mu.Lock()
		// A broker invalidation can precede this API call without a status read.
		// Keep its originating reason even if cleanup changed the public reason.
		if b := r.ports.startupBroker(); r.reason == "" && b != nil {
			b.mu.Lock()
			if b.stopped {
				r.reason = b.firstStopReason
			}
			b.mu.Unlock()
		}
		if r.reason == "" {
			r.reason = reason
		}
		r.state = "stopping"
		r.cleanup = "pending"
		r.cancelStartup()
		r.cancel()
		r.mu.Unlock()
		go func() {
			<-r.ready
			r.mu.Lock()
			p := r.ports
			first := r.reason
			startupFailed := !r.published
			captureStarted := r.captureStarted
			r.mu.Unlock()
			clean := true
			if b := p.startupBroker(); b != nil {
				b.stop(first)
				b.mu.Lock()
				clean = !b.cleanupFailed
				b.mu.Unlock()
				if startupFailed {
					call, cancel := context.WithTimeout(context.Background(), nativePilotCleanup)
					clean = r.dispose(call) == nil && clean
					cancel()
				}
			} else if p.dispose != nil {
				call, cancel := context.WithTimeout(context.Background(), nativePilotCleanup)
				clean = r.dispose(call) == nil
				cancel()
			} else {
				clean = false
			}
			if captureStarted {
				select {
				case <-r.captureDone:
				case <-time.After(nativePilotCleanup):
					clean = false
				}
			}
			r.mu.Lock()
			if clean {
				r.state, r.cleanup = "stopped", "verified"
			} else {
				r.state, r.cleanup = "failed", "uncertain"
			}
			r.mu.Unlock()
			close(r.stopped)
		}()
	})
	select {
	case <-r.stopped:
	case <-ctx.Done():
		r.mu.Lock()
		if r.cleanup != "verified" {
			r.cleanup = "uncertain"
		}
		r.mu.Unlock()
		return errors.New("manual stop deadline")
	}
	if reason == "parent-exited" {
		if r.dispose(ctx) != nil {
			r.mu.Lock()
			r.state, r.cleanup = "failed", "uncertain"
			r.mu.Unlock()
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cleanup != "verified" {
		return errors.New("manual cleanup uncertain")
	}
	return nil
}

func (r *xManualApplication) shutdown() {
	call, cancel := context.WithTimeout(context.Background(), nativePilotCleanup)
	defer cancel()
	_ = r.stop(call, "parent-exited")
}
func (r *xManualApplication) dispose(ctx context.Context) error {
	r.disposeOnce.Do(func() {
		r.mu.Lock()
		dispose := r.ports.dispose
		r.mu.Unlock()
		if dispose == nil {
			r.disposeErr = errors.New("manual disposal unavailable")
		} else {
			r.disposeErr = dispose(ctx)
		}
	})
	return r.disposeErr
}
func (r *xManualApplication) step(ctx context.Context) {
	if r.status().State != "active" {
		return
	}
	r.stepOnce.Do(func() {
		r.mu.Lock()
		if r.state != "active" {
			r.mu.Unlock()
			return
		}
		r.captureStarted = true
		runtime := r.ports.runtime
		r.mu.Unlock()
		go func() {
			stop := context.AfterFunc(ctx, r.cancel)
			defer stop()
			result := r.e.captureNativePilot(r.work, runtime, r.command)
			r.mu.Lock()
			r.captureResult, r.captureResultReady = result, true
			r.mu.Unlock()
			// Publish reconciliation before stop can wait for this work. A failing
			// capture must never wait for its own goroutine to finish.
			close(r.captureDone)
			if result.PersistenceError != nil || result.DeliveryError != nil {
				_ = r.stop(context.Background(), "capture-failed")
			}
		}()
	})
}
func (r *xManualApplication) prepareReader(ctx context.Context, req readerbroker.Request, marker string) (readerbroker.NativePostPreparation, error) {
	if r.status().State != "active" {
		return readerbroker.NativePostPreparation{}, errors.New("manual reader admission closed")
	}
	r.mu.Lock()
	p := r.ports
	remaining := r.deadline.Sub(r.e.store.Now())
	r.mu.Unlock()
	if ctx == nil || remaining <= 0 || remaining > nativePilotLife {
		return readerbroker.NativePostPreparation{}, errors.New("manual reader deadline unavailable")
	}
	call, cancel := context.WithTimeout(ctx, remaining)
	defer cancel()
	stop := context.AfterFunc(r.work, cancel)
	defer stop()
	var prepared readerbroker.NativePostPreparation
	err := p.runtime.withReader(call, req, func(call context.Context) error {
		var err error
		prepared, err = p.prepare(call, req, marker)
		return err
	})
	return prepared, err
}
