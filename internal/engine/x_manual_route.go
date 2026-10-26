package engine

import (
	"context"
	"errors"
	"sync"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
)

// XManualStatus is an operational projection. It cannot grant capture authority.
type XManualStatus struct {
	Source      string `json:"source"`
	State       string `json:"state"`
	Reason      string `json:"reason"`
	Available   bool   `json:"available"`
	RemainingMS int64  `json:"remainingMs"`
	Cleanup     string `json:"cleanup"`
}

// The route is installed only by private, build-gated composition. There is no
// public factory setter, serialized authority input, or implicit native start.
type xManualRoute interface {
	occupied() bool
	status() XManualStatus
	stop(context.Context, string) error
	shutdown()
	step(context.Context)
	prepareReader(context.Context, readerbroker.Request, string) (readerbroker.NativePostPreparation, error)
}
type xManualRouting struct {
	mu     sync.RWMutex
	route  xManualRoute
	closed bool
}

func (e *Engine) XManualStatus() XManualStatus {
	e.xManual.mu.RLock()
	r := e.xManual.route
	e.xManual.mu.RUnlock()
	if r != nil {
		return r.status()
	}
	return XManualStatus{Source: "x", State: "disabled", Reason: "explicit-start-required"}
}

func (e *Engine) StopXManual(ctx context.Context) error {
	if ctx == nil {
		return errors.New("manual stop context required")
	}
	e.xManual.mu.RLock()
	r := e.xManual.route
	e.xManual.mu.RUnlock()
	if r == nil {
		return nil
	}
	return r.stop(ctx, "opted-out")
}

// PrepareXManualReader is called only from the existing validated split-action
// conversation. A selected X route fails closed instead of borrowing the
// ordinary capture profile. The existing OS broker still proves foreground.
func (e *Engine) PrepareXManualReader(ctx context.Context, req readerbroker.Request, marker string) (readerbroker.NativePostPreparation, bool, error) {
	if err := req.Validate(); err != nil {
		return readerbroker.NativePostPreparation{}, true, errors.New("manual reader request rejected")
	}
	e.xManual.mu.RLock()
	r := e.xManual.route
	e.xManual.mu.RUnlock()
	if req.Source != "x" || r == nil {
		return readerbroker.NativePostPreparation{}, false, nil
	}
	p, err := r.prepareReader(ctx, req, marker)
	return p, true, err
}

// Hold the read lock through the ordinary durable claim. Startup reserves X
// under the write lock, so a preliminary availability check cannot race it.
func (e *Engine) ordinarySourceRoute(source domain.Source) (func(), bool) {
	if source != domain.SourceX {
		return func() {}, true
	}
	e.xManual.mu.RLock()
	if e.xManual.route != nil && e.xManual.route.occupied() {
		e.xManual.mu.RUnlock()
		return func() {}, false
	}
	return e.xManual.mu.RUnlock, true
}

func (e *Engine) stepXManual(ctx context.Context) {
	e.xManual.mu.RLock()
	r := e.xManual.route
	e.xManual.mu.RUnlock()
	if r != nil {
		r.step(ctx)
	}
}

func (e *Engine) ordinaryPendingRun(ctx context.Context, collector string) (string, error) {
	e.xManual.mu.RLock()
	r := e.xManual.route
	blocked := r != nil && r.occupied()
	e.xManual.mu.RUnlock()
	if blocked {
		return e.store.PendingRunIDForCollectorExcept(ctx, collector, domain.SourceX)
	}
	return e.store.PendingRunIDForCollector(ctx, collector)
}

func (e *Engine) shutdownXManual() {
	e.xManual.mu.Lock()
	e.xManual.closed = true
	r := e.xManual.route
	e.xManual.mu.Unlock()
	if r != nil {
		r.shutdown()
	}
}
