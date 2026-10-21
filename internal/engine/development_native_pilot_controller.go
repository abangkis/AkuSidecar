//go:build auth_session_dev

package engine

import (
	"context"
	"sync"
)

// This private controller is not registered with main, a broker or Store. Its
// callbacks are trusted local, synchronous, non-reentrant atomic effects: false
// means no effect occurred. The lock orders those effects against revocation.
type nativePilotManualConfig struct {
	enabled, offlineTest bool
	pilot                nativePilotConfig
}
type nativePilotManualController struct {
	mu                                        sync.Mutex
	parent                                    context.Context
	startupCancel                             context.CancelFunc
	adapter                                   *nativePilotAdapter
	vector                                    *nativePilotVector
	seal                                      *nativePilotSeal
	offlineTest, starting, attempted, stopped bool
	reason                                    string
	work                                      map[string]*nativePilotManualWork
}
type nativePilotManualTicket struct {
	seal      *nativePilotSeal
	work      string
	version   uint64
	collector nativePilotOwner
}
type nativePilotManualWork struct {
	ticket                      *nativePilotManualTicket
	claimed, canceled, complete bool
}

func (*nativePilotManualController) String() string     { return "private-native-manual-controller" }
func (v *nativePilotManualController) GoString() string { return v.String() }
func (nativePilotManualConfig) String() string          { return "private-native-manual-config" }
func (v nativePilotManualConfig) GoString() string      { return v.String() }
func (*nativePilotManualTicket) String() string         { return "private-native-manual-ticket" }
func (v *nativePilotManualTicket) GoString() string     { return v.String() }
func (nativePilotManualWork) String() string            { return "private-native-manual-work" }
func (v nativePilotManualWork) GoString() string        { return v.String() }

func newNativePilotManualController(parent context.Context, cfg nativePilotManualConfig, ports nativePilotPorts) (*nativePilotManualController, nativePilotDiagnostic) {
	if !cfg.enabled {
		return nil, nativePilotDiagnostic{Reason: "admission-disabled"}
	}
	if parent == nil || parent.Err() != nil || ports.source == nil || cfg.offlineTest != (ports.source.provenance() == nativePilotOfflineOnly) {
		return nil, nativePilotDiagnostic{Reason: "invalid-configuration"}
	}
	a, d := newNativePilotAdapter(cfg.pilot, ports)
	if a == nil {
		return nil, d
	}
	c := &nativePilotManualController{parent: parent, adapter: a, seal: &nativePilotSeal{}, offlineTest: cfg.offlineTest, work: make(map[string]*nativePilotManualWork)}
	// Parent cancellation closes admission even when no consumer polls a barrier.
	context.AfterFunc(parent, func() { c.invalidate("parent-exited") })
	return c, nativePilotDiagnostic{Reason: "admission-closed"}
}

func (c *nativePilotManualController) stopLocked(event string) nativePilotDiagnostic {
	if c.stopped {
		return nativePilotDiagnostic{Reason: c.reason}
	}
	d := c.adapter.invalidate(event)
	if c.startupCancel != nil {
		c.startupCancel()
	}
	if source, ok := c.adapter.ports.source.(interface{ discard() }); ok {
		source.discard()
	}
	c.stopped, c.reason, c.vector = true, d.Reason, nil
	return d
}
func (c *nativePilotManualController) invalidate(event string) nativePilotDiagnostic {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stopLocked(event)
}

func (c *nativePilotManualController) start(ctx context.Context) nativePilotDiagnostic {
	c.mu.Lock()
	if c.stopped {
		d := nativePilotDiagnostic{Reason: c.reason}
		c.mu.Unlock()
		return d
	}
	if c.attempted {
		c.mu.Unlock()
		return nativePilotDiagnostic{Reason: "startup-already-attempted"}
	}
	if ctx == nil || ctx.Err() != nil || c.parent.Err() != nil {
		d := c.stopLocked("canceled")
		c.mu.Unlock()
		return d
	}
	startup, cancel := context.WithCancel(ctx)
	c.startupCancel = cancel
	c.attempted, c.starting = true, true
	c.mu.Unlock()
	stop := context.AfterFunc(c.parent, cancel)
	defer cancel()
	defer stop()
	v, d := c.adapter.initialEvidence(startup)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.starting = false
	c.startupCancel = nil
	if c.stopped {
		return nativePilotDiagnostic{Reason: c.reason}
	}
	if c.parent.Err() != nil {
		return c.stopLocked("parent-exited")
	}
	if v == nil || !d.OfflineValidated && !d.ManualEligible {
		c.stopped, c.reason = true, d.Reason
		return d
	}
	if c.offlineTest != (v.provenance == nativePilotOfflineOnly) {
		return c.stopLocked("evidence-invalid")
	}
	c.vector = v
	return c.checkLocked(ctx)
}

func (c *nativePilotManualController) checkLocked(ctx context.Context) nativePilotDiagnostic {
	if c.stopped {
		return nativePilotDiagnostic{Reason: c.reason}
	}
	if c.parent.Err() != nil {
		return c.stopLocked("parent-exited")
	}
	if c.vector == nil {
		return nativePilotDiagnostic{Reason: "admission-closed"}
	}
	d := c.adapter.valid(ctx, c.vector)
	if !d.OfflineValidated && !d.ManualEligible {
		c.stopped, c.reason, c.vector = true, d.Reason, nil
		return d
	}
	if c.offlineTest != (c.vector.provenance == nativePilotOfflineOnly) {
		return c.stopLocked("evidence-invalid")
	}
	return d
}
func (c *nativePilotManualController) check(ctx context.Context) nativePilotDiagnostic {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.checkLocked(ctx)
}

func (c *nativePilotManualController) dispatch(ctx context.Context, work string) (*nativePilotManualTicket, nativePilotDiagnostic) {
	c.mu.Lock()
	defer c.mu.Unlock()
	d := c.checkLocked(ctx)
	if !d.OfflineValidated && !d.ManualEligible {
		return nil, d
	}
	if !nativePilotRef(work, 512) {
		return nil, nativePilotDiagnostic{Reason: "invalid-work"}
	}
	if _, exists := c.work[work]; exists {
		return nil, nativePilotDiagnostic{Reason: "duplicate-work"}
	}
	if len(c.work) >= 32 {
		return nil, nativePilotDiagnostic{Reason: "work-capacity"}
	}
	t := &nativePilotManualTicket{seal: c.seal, work: work, version: c.vector.version, collector: c.vector.binding.collector}
	c.work[work] = &nativePilotManualWork{ticket: t}
	d.Reason = "work-dispatched"
	return t, d
}

func (c *nativePilotManualController) ticketLocked(ctx context.Context, t *nativePilotManualTicket) (*nativePilotManualWork, nativePilotDiagnostic) {
	d := c.checkLocked(ctx)
	if !d.OfflineValidated && !d.ManualEligible {
		return nil, d
	}
	if t == nil || t.seal != c.seal || t.version != c.vector.version || t.collector != c.vector.binding.collector {
		return nil, nativePilotDiagnostic{Reason: "stale-ticket"}
	}
	w := c.work[t.work]
	if w == nil || w.ticket != t {
		return nil, nativePilotDiagnostic{Reason: "stale-ticket"}
	}
	if w.canceled {
		return nil, nativePilotDiagnostic{Reason: "work-canceled"}
	}
	if w.complete {
		return nil, nativePilotDiagnostic{Reason: "result-duplicate"}
	}
	return w, d
}

func (c *nativePilotManualController) claim(ctx context.Context, t *nativePilotManualTicket, effect func() bool) nativePilotDiagnostic {
	c.mu.Lock()
	defer c.mu.Unlock()
	w, d := c.ticketLocked(ctx, t)
	if w == nil {
		return d
	}
	if w.claimed {
		return nativePilotDiagnostic{Reason: "work-already-claimed"}
	}
	if effect == nil || !effect() {
		return nativePilotDiagnostic{Reason: "local-effect-rejected"}
	}
	w.claimed = true
	d.Reason = "work-claimed"
	return d
}
func (c *nativePilotManualController) complete(ctx context.Context, t *nativePilotManualTicket, effect func() bool) nativePilotDiagnostic {
	c.mu.Lock()
	defer c.mu.Unlock()
	w, d := c.ticketLocked(ctx, t)
	if w == nil {
		return d
	}
	if !w.claimed {
		return nativePilotDiagnostic{Reason: "work-not-claimed"}
	}
	if effect == nil || !effect() {
		return nativePilotDiagnostic{Reason: "local-effect-rejected"}
	}
	w.complete = true
	d.Reason = "result-completed"
	return d
}

// Cancel creates a bounded tombstone even before dispatch, so the same command
// cannot be revived with a new ticket. Cancellation itself grants no authority.
func (c *nativePilotManualController) cancel(work string) nativePilotDiagnostic {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !nativePilotRef(work, 512) {
		return nativePilotDiagnostic{Reason: "invalid-work"}
	}
	w := c.work[work]
	if w == nil {
		if len(c.work) >= 32 {
			return nativePilotDiagnostic{Reason: "work-capacity"}
		}
		w = &nativePilotManualWork{}
		c.work[work] = w
	}
	if w.complete {
		return nativePilotDiagnostic{Reason: "result-duplicate"}
	}
	w.canceled = true
	return nativePilotDiagnostic{Reason: "work-canceled"}
}
