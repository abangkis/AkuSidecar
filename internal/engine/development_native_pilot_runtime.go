//go:build auth_session_dev

package engine

import (
	"context"
	"errors"
	"sync"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/collection"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
)

// Private wiring composes the manual V2 broker with fixed process ownership.
// There is deliberately no native backend factory or public authority input.
type nativePilotRuntimeBackend interface {
	collection.CaptureBackend
	// The trusted factory binds the complete collector identity, not JSON flags.
	nativePilotCollectorOwner() nativePilotOwner
}

type nativePilotRuntime struct {
	broker            *nativePilotBroker
	reader, collector *captureruntime.Manager
	coordinator       *collection.Coordinator
	backend           nativePilotRuntimeBackend
	open              func(context.Context, readerbroker.Request) error
	mu                sync.Mutex
	requests          map[string]bool
	results           map[string]domain.Observation
	storeLink         *nativePilotStoreRuntime
	storeBinding      bool
}

func (*nativePilotRuntime) String() string     { return "private-native-manual-runtime" }
func (r *nativePilotRuntime) GoString() string { return r.String() }

func nativePilotRuntimeAllowed(d nativePilotDiagnostic) bool {
	return (d.OfflineValidated || d.ManualEligible) && !d.CanAuthorize && !d.NativeQualified && !d.StorePermit
}

func newNativePilotRuntime(b *nativePilotBroker, reader, collector *captureruntime.Manager, backend nativePilotRuntimeBackend, open func(context.Context, readerbroker.Request) error) (*nativePilotRuntime, error) {
	if b == nil || reader == nil || collector == nil || reader == collector || backend == nil || open == nil {
		return nil, errors.New("manual runtime unavailable")
	}
	bound := backend.nativePilotCollectorOwner()
	if !validNativePilotOwner(bound, "collector") || bound.pid != collector.PID() || bound.generation != collector.Snapshot().Generation {
		return nil, errors.New("manual backend owner mismatch")
	}
	r := &nativePilotRuntime{broker: b, reader: reader, collector: collector, backend: backend, open: open, requests: map[string]bool{}, results: map[string]domain.Observation{}}
	r.coordinator = collection.NewCoordinator(collector, func(context.Context, string, uint64) (captureruntime.Process, error) {
		return nil, errors.New("manual runtime replacement forbidden")
	}, func() error { return nil })
	check := func(ctx context.Context) error {
		if !nativePilotRuntimeAllowed(b.check(ctx)) {
			return errors.New("manual admission closed")
		}
		return nil
	}
	if err := r.coordinator.BindManualConcurrentSession(reader, collector.Snapshot().Generation, backend, check, check); err != nil {
		return nil, err
	}
	return r, nil
}

func nativePilotRuntimeOwner(manager *captureruntime.Manager, lease *captureruntime.Lease, owner nativePilotOwner, driver string) bool {
	s := manager.Snapshot()
	return lease != nil && s.State == captureruntime.Ready && s.Driver == driver && s.Generation == owner.generation && lease.Generation() == owner.generation && lease.Driver() == driver && manager.PID() == owner.pid
}

func (r *nativePilotRuntime) openReader(ctx context.Context, request readerbroker.Request) error {
	if request.Validate() != nil || request.Source != "x" {
		return errors.New("manual reader request rejected")
	}
	// Only bounded in-memory reservations execute under broker/controller locks.
	d := r.broker.openReader(ctx, func() bool {
		r.mu.Lock()
		defer r.mu.Unlock()
		if len(r.requests) >= 32 {
			return false
		}
		if _, exists := r.requests[request.RequestID]; exists {
			return false
		}
		r.requests[request.RequestID] = false
		return true
	})
	if !nativePilotRuntimeAllowed(d) {
		return errors.New("manual reader admission rejected")
	}
	lease, release, err := r.coordinator.BorrowNativeReader(ctx)
	if err != nil {
		return err
	}
	defer release()
	defer lease.Release()
	r.broker.mu.Lock()
	owner := r.broker.cfg.pilot.binding.reader
	if r.broker.controller != nil {
		owner = r.broker.controller.adapter.cfg.binding.reader
	}
	r.broker.mu.Unlock()
	if !nativePilotRuntimeOwner(r.reader, lease, owner, "browser") {
		r.broker.invalidate("owner-or-policy-changed")
		return errors.New("manual reader owner changed")
	}
	// External reader work always runs outside broker/controller locks.
	if err = r.open(ctx, request); err != nil {
		return err
	}
	if !nativePilotRuntimeOwner(r.reader, lease, owner, "browser") {
		r.broker.invalidate("owner-or-policy-changed")
		return errors.New("manual reader owner changed")
	}
	d = r.broker.openReader(ctx, func() bool { r.mu.Lock(); defer r.mu.Unlock(); r.requests[request.RequestID] = true; return true })
	if !nativePilotRuntimeAllowed(d) {
		return errors.New("manual reader completion rejected")
	}
	return nil
}

func (r *nativePilotRuntime) capture(ctx context.Context, work string, source domain.Source) (*nativePilotManualTicket, error) {
	r.mu.Lock()
	storeBound := r.storeBinding
	r.mu.Unlock()
	if storeBound {
		return nil, errors.New("Store runtime requires Engine capture entry")
	}
	if source != domain.SourceX {
		return nil, errors.New("manual capture requires X")
	}
	ticket, d := r.broker.dispatch(ctx, work)
	if ticket == nil || !nativePilotRuntimeAllowed(d) {
		return nil, errors.New("manual dispatch rejected")
	}
	if !nativePilotRuntimeAllowed(r.broker.claim(ctx, ticket, func() bool { return true })) {
		return ticket, errors.New("manual claim rejected")
	}
	lease, err := r.collector.Acquire()
	if err != nil {
		return ticket, err
	}
	defer lease.Release()
	if !nativePilotRuntimeOwner(r.collector, lease, ticket.collector, "headless") || r.backend.nativePilotCollectorOwner() != ticket.collector {
		r.broker.invalidate("owner-or-policy-changed")
		return ticket, errors.New("manual collector owner changed")
	}
	observation, err := r.coordinator.Capture(ctx, source, map[string]any{"workId": work})
	if err != nil {
		return ticket, err
	}
	if !nativePilotRuntimeOwner(r.collector, lease, ticket.collector, "headless") || r.backend.nativePilotCollectorOwner() != ticket.collector {
		r.broker.invalidate("owner-or-policy-changed")
		return ticket, errors.New("manual collector owner changed")
	}
	return ticket, r.complete(ctx, ticket, observation)
}

func (r *nativePilotRuntime) complete(ctx context.Context, ticket *nativePilotManualTicket, observation domain.Observation) error {
	r.mu.Lock()
	storeBound := r.storeBinding
	r.mu.Unlock()
	if storeBound {
		return errors.New("Store runtime requires guarded result commit")
	}
	if observation.Source != domain.SourceX {
		return errors.New("manual result source rejected")
	}
	if ticket == nil {
		return errors.New("manual result ticket rejected")
	}
	s := r.collector.Snapshot()
	if s.State != captureruntime.Ready || s.Driver != "headless" || s.Generation != ticket.collector.generation || r.collector.PID() != ticket.collector.pid || r.backend.nativePilotCollectorOwner() != ticket.collector {
		return errors.New("manual result owner rejected")
	}
	d := r.broker.complete(ctx, ticket, func() bool {
		r.mu.Lock()
		defer r.mu.Unlock()
		if len(r.results) >= 32 {
			return false
		}
		if _, exists := r.results[ticket.work]; exists {
			return false
		}
		r.results[ticket.work] = observation
		return true
	})
	if !nativePilotRuntimeAllowed(d) {
		return errors.New("manual result rejected")
	}
	return nil
}
