//go:build auth_session_dev

package engine

import (
	"context"
	"errors"
	"sync"
	"time"
)

// PREPARE holds each real startup document before identity acquisition. OS and
// physical directory facts come from trusted in-process ownership ports, never
// JSON assertions. ACQUIRE must use that same held handle without navigation.
type nativePilotPrepareRequest struct{ role, profile, parent, attempt string }
type nativePilotPreparedRole struct {
	owner  nativePilotOwner
	handle string
}
type nativePilotBrokerState struct {
	account, enrollment, pair, attempt, derivation                              string
	policy, consent                                                             uint64
	readerWindows                                                               int
	completeInventory, parentAlive, channelHealthy, invalidationsReady, optedIn bool
}
type nativePilotPairInspection struct {
	state             nativePilotBrokerState
	reader, collector nativePilotOwner
	reason            string
}

func (nativePilotPairInspection) String() string     { return "private-native-pair-inspection" }
func (v nativePilotPairInspection) GoString() string { return v.String() }

type nativePilotBrokerPorts struct {
	now     func() time.Time
	prepare func(context.Context, nativePilotPrepareRequest) (nativePilotPreparedRole, error)
	inspect func(context.Context, nativePilotPreparedRole) (nativePilotOwner, error)
	// Optional trusted composition; failure never falls back to the legacy path.
	inspectPair func(context.Context, nativePilotPreparedRole, nativePilotPreparedRole) (nativePilotPairInspection, error)
	acquire     func(context.Context, nativePilotPreparedRole, nativePilotCaptureChallenge) ([]byte, error)
	state       func(context.Context) (nativePilotBrokerState, error)
	// Abort must honor its bounded context and report uncertain cleanup as error.
	// preserveReader is true only for the reader, except after parent exit.
	abort func(context.Context, nativePilotPreparedRole, bool) error
}
type nativePilotBrokerConfig struct {
	enabled, offlineTest bool
	pilot                nativePilotConfig
}
type nativePilotBroker struct {
	mu                       sync.Mutex
	detailMu                 sync.Mutex
	initial                  nativePilotInitialDetail
	clockMu                  sync.Mutex
	parent                   context.Context
	cfg                      nativePilotBrokerConfig
	ports                    nativePilotBrokerPorts
	last                     time.Time
	clockRegressed           bool
	cleanupFailed            bool
	started, stopped         bool
	reason                   string
	cancelPrepare            context.CancelFunc
	prepareDone, cleanupDone chan struct{}
	prepared                 []nativePilotPreparedRole
	controller               *nativePilotManualController
	// Installed only by private development Store composition. Called unlocked.
	onStop func()
}

func (b *nativePilotBroker) noteInitial(err error, role, phase string) {
	b.detailMu.Lock()
	defer b.detailMu.Unlock()
	if b.initial.role == "" {
		b.initial = nativePilotInitialOf(err, role, phase)
	}
}
func (b *nativePilotBroker) startupTrace() string {
	b.mu.Lock()
	c := b.controller
	b.mu.Unlock()
	if c == nil {
		return "unavailable"
	}
	c.adapter.mu.Lock()
	snapshot := c.adapter.startupTiming
	c.adapter.mu.Unlock()
	if snapshot == nil {
		return "unavailable"
	}
	return snapshot.String()
}
func (b *nativePilotBroker) initialDiagnostic() nativePilotInitialDetail {
	b.detailMu.Lock()
	detail := b.initial
	b.detailMu.Unlock()
	b.mu.Lock()
	c := b.controller
	b.mu.Unlock()
	if c != nil {
		c.adapter.mu.Lock()
		fromSource := c.adapter.initial
		c.adapter.mu.Unlock()
		if fromSource.role != "" {
			if detail.role != "" {
				detail.timing = fromSource.timing
			}
			// A joint cancellation/deadline can follow a recorded owner failure.
			// Keep its originating predicate, while a known branded source cause
			// remains authoritative over subsequent inspection during teardown.
			jointCancellation := fromSource.role == "pair" && fromSource.phase == "joint" && (fromSource.reason == "canceled" || fromSource.reason == "deadline")
			if detail.role == "" || !jointCancellation {
				return fromSource.closed()
			}
		}
	}
	if detail.role == "" {
		return nativePilotInitialDetail{role: "unknown", phase: "joint", reason: "unavailable"}
	}
	return detail.closed()
}

func (nativePilotPrepareRequest) String() string     { return "private-native-prepare-request" }
func (v nativePilotPrepareRequest) GoString() string { return v.String() }
func (nativePilotPreparedRole) String() string       { return "private-native-prepared-role" }
func (v nativePilotPreparedRole) GoString() string   { return v.String() }
func (nativePilotBrokerState) String() string        { return "private-native-broker-state" }
func (v nativePilotBrokerState) GoString() string    { return v.String() }
func (nativePilotBrokerConfig) String() string       { return "private-native-broker-config" }
func (v nativePilotBrokerConfig) GoString() string   { return v.String() }
func (*nativePilotBroker) String() string            { return "private-native-broker" }
func (v *nativePilotBroker) GoString() string        { return v.String() }

func newNativePilotBroker(parent context.Context, cfg nativePilotBrokerConfig, ports nativePilotBrokerPorts) (*nativePilotBroker, nativePilotDiagnostic) {
	if !cfg.enabled {
		return nil, nativePilotDiagnostic{Reason: "admission-disabled"}
	}
	p, meta := cfg.pilot, cfg.pilot.binding
	r, c := meta.reader, meta.collector
	// Initial document/process pins must be unbound, not silently overwritten.
	selected := func(o nativePilotOwner, role string) bool {
		return o == (nativePilotOwner{role: role, profile: o.profile, parent: o.parent}) && nativePilotRef(o.profile, 512) && nativePilotRef(o.parent, 512)
	}
	valid := parent != nil && parent.Err() == nil && p.enabled && p.contract == nativePilotContract && p.limitationAcknowledged && !p.attemptStarted.IsZero() && nativePilotAccount(meta.account) && meta.policy > 0 && meta.consent > 0 && selected(r, "reader") && selected(c, "collector") && r.profile != c.profile && r.parent == c.parent
	for _, ref := range []string{meta.enrollment, meta.pair, meta.attempt, meta.derivation} {
		valid = valid && nativePilotRef(ref, 512)
	}
	if !valid || ports.now == nil || ports.prepare == nil || ports.inspect == nil || ports.acquire == nil || ports.state == nil || ports.abort == nil || !p.attemptValidUntil.IsZero() && !p.attemptValidUntil.After(p.attemptStarted) {
		return nil, nativePilotDiagnostic{Reason: "invalid-configuration"}
	}
	b := &nativePilotBroker{parent: parent, cfg: cfg, ports: ports, last: p.attemptStarted}
	context.AfterFunc(parent, func() { b.invalidate("parent-exited") })
	return b, nativePilotDiagnostic{Reason: "admission-closed"}
}
func (b *nativePilotBroker) deadline() time.Time {
	end := b.cfg.pilot.attemptStarted.Add(nativePilotLife)
	if limit := b.cfg.pilot.attemptValidUntil; !limit.IsZero() && limit.Before(end) {
		end = limit
	}
	return end
}
func (b *nativePilotBroker) clockLocked(ctx context.Context) (time.Time, string) {
	if b.stopped {
		return time.Time{}, b.reason
	}
	if b.parent.Err() != nil {
		return time.Time{}, "parent-exited"
	}
	if ctx == nil || ctx.Err() != nil {
		return time.Time{}, "canceled"
	}
	now := b.readTime()
	if now.IsZero() {
		return now, "clock-regressed"
	}
	if !now.Before(b.deadline()) {
		return now, "attempt-expired"
	}
	return now, ""
}
func (b *nativePilotBroker) readTime() time.Time {
	b.clockMu.Lock()
	defer b.clockMu.Unlock()
	now := b.ports.now()
	if now.Before(b.last) {
		b.clockRegressed = true
	}
	b.last = now
	if b.clockRegressed {
		return time.Time{}
	}
	return now
}
func (b *nativePilotBroker) stateReason(ctx context.Context) (nativePilotBrokerState, string) {
	s, err := b.ports.state(ctx)
	if err != nil {
		return s, "owner-unavailable"
	}
	return s, nativePilotBrokerStateReason(s, b.cfg.pilot.binding, true)
}

// Policy checks are identical for ordinary and combined reads. The preflight of
// a combined read defers only the window count until the reader is inspected.
func nativePilotBrokerStateReason(s nativePilotBrokerState, m nativePilotBinding, checkWindows bool) string {
	if !s.parentAlive {
		return "parent-exited"
	}
	if !s.optedIn {
		return "opted-out"
	}
	if !s.channelHealthy || !s.invalidationsReady {
		return "evidence-channel-lost"
	}
	if !s.completeInventory || checkWindows && s.readerWindows < 1 {
		return "reader-lost"
	}
	if s.account != m.account || s.enrollment != m.enrollment || s.pair != m.pair || s.attempt != m.attempt || s.derivation != m.derivation || s.policy != m.policy || s.consent != m.consent {
		return "owner-or-policy-changed"
	}
	return ""
}

func (b *nativePilotBroker) abortPrepared(p nativePilotPreparedRole, preserve bool) bool {
	if p.handle == "" {
		return true
	}
	ctx, cancel := context.WithTimeout(context.Background(), nativePilotCleanup)
	defer cancel()
	if b.ports.abort(ctx, p, preserve && p.owner.role == "reader") != nil {
		b.mu.Lock()
		b.cleanupFailed = true
		b.reason = "cleanup-uncertain"
		b.mu.Unlock()
		return false
	}
	return true
}
func (b *nativePilotBroker) stop(reason string) nativePilotDiagnostic {
	b.mu.Lock()
	if b.stopped {
		done := b.cleanupDone
		b.mu.Unlock()
		if done != nil {
			select {
			case <-done:
			case <-time.After(nativePilotCleanup):
				b.mu.Lock()
				b.cleanupFailed = true
				b.reason = "cleanup-uncertain"
				b.mu.Unlock()
			}
		}
		b.mu.Lock()
		defer b.mu.Unlock()
		return nativePilotDiagnostic{Reason: b.reason}
	}
	b.stopped, b.reason = true, reason
	if b.cleanupFailed {
		b.reason = "cleanup-uncertain"
	}
	b.cleanupDone = make(chan struct{})
	cleanupDone, prepareDone := b.cleanupDone, b.prepareDone
	if b.cancelPrepare != nil {
		b.cancelPrepare()
	}
	if b.controller != nil {
		b.controller.invalidate(reason)
	}
	prepared := b.prepared
	b.prepared = nil
	onStop := b.onStop
	b.mu.Unlock()
	if onStop != nil {
		onStop()
	}
	cleanupCtx, cancel := context.WithTimeout(context.Background(), nativePilotCleanup)
	defer cancel()
	var cleanup sync.WaitGroup
	for _, p := range prepared {
		cleanup.Add(1)
		go func(p nativePilotPreparedRole) {
			defer cleanup.Done()
			if b.ports.abort(cleanupCtx, p, reason != "parent-exited" && p.owner.role == "reader") != nil {
				b.mu.Lock()
				b.cleanupFailed = true
				b.reason = "cleanup-uncertain"
				b.mu.Unlock()
			}
		}(p)
	}
	abortsDone := make(chan struct{})
	go func() { cleanup.Wait(); close(abortsDone) }()
	for _, done := range []<-chan struct{}{abortsDone, prepareDone} {
		if done == nil {
			continue
		}
		select {
		case <-done:
		case <-cleanupCtx.Done():
			b.mu.Lock()
			b.cleanupFailed = true
			b.reason = "cleanup-uncertain"
			b.mu.Unlock()
		}
	}
	close(cleanupDone)
	b.mu.Lock()
	defer b.mu.Unlock()
	return nativePilotDiagnostic{Reason: b.reason}
}
func (b *nativePilotBroker) invalidate(event string) nativePilotDiagnostic {
	switch event {
	case "opted-out", "reader-lost", "parent-exited", "owner-or-policy-changed", "evidence-channel-lost", "candidate-cookie-write", "canceled", "identity-mismatch", "authentication-rejected", "cleanup-uncertain":
	default:
		event = "evidence-invalid"
	}
	return b.stop(event)
}

func (b *nativePilotBroker) start(ctx context.Context) nativePilotDiagnostic {
	b.mu.Lock()
	now, reason := b.clockLocked(ctx)
	if reason != "" {
		b.mu.Unlock()
		return b.stop(reason)
	}
	if b.started {
		b.mu.Unlock()
		return nativePilotDiagnostic{Reason: "startup-already-attempted"}
	}
	b.started = true
	prepareCtx, cancel := context.WithTimeout(ctx, b.deadline().Sub(now))
	b.cancelPrepare = cancel
	b.mu.Unlock()
	defer cancel()
	if _, reason = b.stateReason(prepareCtx); reason != "" {
		return b.stop(reason)
	}
	type result struct {
		prepared nativePilotPreparedRole
		role     string
		err      error
		ack      chan struct{}
	}
	results := make(chan result)
	var preparing sync.WaitGroup
	b.mu.Lock()
	if b.stopped {
		reason = b.reason
		b.mu.Unlock()
		return b.stop(reason)
	}
	b.prepareDone = make(chan struct{})
	preparing.Add(2)
	for _, selected := range []nativePilotOwner{b.cfg.pilot.binding.reader, b.cfg.pilot.binding.collector} {
		go func(o nativePilotOwner) {
			defer preparing.Done()
			p, err := b.ports.prepare(prepareCtx, nativePilotPrepareRequest{role: o.role, profile: o.profile, parent: o.parent, attempt: b.cfg.pilot.binding.attempt})
			if err != nil {
				b.noteInitial(err, o.role, "prepare")
			}
			ack := make(chan struct{})
			select {
			case results <- result{prepared: p, role: o.role, err: err, ack: ack}:
				<-ack
			case <-prepareCtx.Done():
				b.abortPrepared(p, b.parent.Err() == nil)
			}
		}(selected)
	}
	go func() { preparing.Wait(); close(b.prepareDone) }()
	b.mu.Unlock()
	roles := make(map[string]nativePilotPreparedRole, 2)
	for count := 0; count < 2; count++ {
		var out result
		select {
		case <-prepareCtx.Done():
			reason = "canceled"
			if b.parent.Err() != nil {
				reason = "parent-exited"
			}
			return b.stop(reason)
		case out = <-results:
		}
		p := out.prepared
		b.mu.Lock()
		if b.stopped {
			reason = b.reason
			b.mu.Unlock()
			b.abortPrepared(p, reason != "parent-exited")
			close(out.ack)
			return b.stop(reason)
		}
		if p.handle != "" {
			b.prepared = append(b.prepared, p)
		}
		_, reason = b.clockLocked(prepareCtx)
		b.mu.Unlock()
		close(out.ack)
		if reason != "" {
			return b.stop(reason)
		}
		if out.err != nil {
			return b.stop("prepare-failed")
		}
		selected := b.cfg.pilot.binding.reader
		if out.role == "collector" {
			selected = b.cfg.pilot.binding.collector
		}
		if !nativePilotRef(p.handle, 512) || !validNativePilotOwner(p.owner, out.role) || p.owner.profile != selected.profile || p.owner.parent != selected.parent {
			return b.stop("prepare-pins-invalid")
		}
		for _, other := range roles {
			if other.handle == p.handle {
				return b.stop("prepare-handle-replayed")
			}
		}
		actual, err := b.ports.inspect(prepareCtx, p)
		if err != nil || actual != p.owner {
			b.noteInitial(err, out.role, "inspect_before")
			return b.stop("owner-or-policy-changed")
		}
		roles[out.role] = p
	}
	frozen := b.cfg.pilot.binding
	frozen.reader, frozen.collector = roles["reader"].owner, roles["collector"].owner
	if !validNativePilotBinding(frozen) {
		return b.stop("prepare-pins-invalid")
	}
	owners := func(call context.Context) (nativePilotState, error) {
		var s nativePilotBrokerState
		var why string
		var actual [2]nativePilotOwner
		if b.ports.inspectPair != nil {
			pair, err := b.ports.inspectPair(call, roles["reader"], roles["collector"])
			s, why = pair.state, pair.reason
			if err != nil {
				b.noteInitial(err, "pair", "inspect_before")
				if why == "" {
					why = "owner-or-policy-changed"
				}
			} else if why == "" {
				why = nativePilotBrokerStateReason(s, frozen, true)
			}
			actual = [2]nativePilotOwner{pair.reader, pair.collector}
		} else {
			s, why = b.stateReason(call)
			if why == "" {
				for i, p := range []nativePilotPreparedRole{roles["reader"], roles["collector"]} {
					owner, err := b.ports.inspect(call, p)
					if err != nil || owner != p.owner {
						b.noteInitial(err, p.owner.role, "inspect_before")
						why = "owner-or-policy-changed"
						break
					}
					actual[i] = owner
				}
			}
		}
		if why == "" && (call.Err() != nil || actual[0] != frozen.reader || actual[1] != frozen.collector) {
			why = "owner-or-policy-changed"
		}
		if why == "owner-unavailable" {
			return nativePilotState{}, errors.New("owner-unavailable")
		}
		if why != "" {
			return nativePilotState{binding: frozen, readerWindows: s.readerWindows, completeInventory: s.completeInventory, parentAlive: s.parentAlive, channelHealthy: s.channelHealthy, invalidationsReady: s.invalidationsReady, optedIn: s.optedIn}, nil
		}
		return nativePilotState{binding: frozen, readerWindows: s.readerWindows, completeInventory: true, parentAlive: true, ownersAlive: true, channelHealthy: true, invalidationsReady: true, optedIn: true}, nil
	}
	// Recheck the whole pair after BOTH are ready, before constructing a source.
	if _, reason = b.stateReason(prepareCtx); reason != "" {
		return b.stop(reason)
	}
	s, err := owners(prepareCtx)
	if err != nil || !s.ownersAlive {
		return b.stop("owner-or-policy-changed")
	}
	b.mu.Lock()
	_, reason = b.clockLocked(prepareCtx)
	b.mu.Unlock()
	if reason != "" {
		return b.stop(reason)
	}
	source, err := newNativePilotStartupSource(nativePilotStartupConfig{enabled: true, offlineOnly: b.cfg.offlineTest, binding: frozen, capture: func(call context.Context, ch nativePilotCaptureChallenge) ([]byte, error) {
		return b.ports.acquire(call, roles[ch.challenge.role], ch)
	}})
	if err != nil {
		return b.stop("source-unavailable")
	}
	pilot := b.cfg.pilot
	pilot.binding = frozen
	c, d := newNativePilotManualController(b.parent, nativePilotManualConfig{enabled: true, offlineTest: b.cfg.offlineTest, pilot: pilot}, nativePilotPorts{now: b.readTime, source: source, owners: owners})
	if c == nil {
		return b.stop(d.Reason)
	}
	b.mu.Lock()
	if b.stopped {
		reason = b.reason
		b.mu.Unlock()
		c.invalidate(reason)
		return nativePilotDiagnostic{Reason: reason}
	}
	b.controller = c
	b.mu.Unlock()
	d = c.start(prepareCtx)
	if !d.OfflineValidated && !d.ManualEligible {
		return b.stop(d.Reason)
	}
	return d
}

// Controller callbacks remain synchronous, trusted and non-reentrant. The
// broker lock orders reader requests and work effects against broker revocation.
func (b *nativePilotBroker) use(ctx context.Context, op func(*nativePilotManualController) nativePilotDiagnostic) nativePilotDiagnostic {
	b.mu.Lock()
	if b.stopped {
		d := nativePilotDiagnostic{Reason: b.reason}
		b.mu.Unlock()
		return d
	}
	if b.controller == nil {
		b.mu.Unlock()
		return nativePilotDiagnostic{Reason: "admission-closed"}
	}
	d := op(b.controller)
	b.controller.mu.Lock()
	stopped := b.controller.stopped
	b.controller.mu.Unlock()
	b.mu.Unlock()
	if stopped {
		return b.stop(d.Reason)
	}
	return d
}
func (b *nativePilotBroker) check(ctx context.Context) nativePilotDiagnostic {
	return b.use(ctx, func(c *nativePilotManualController) nativePilotDiagnostic { return c.check(ctx) })
}
func (b *nativePilotBroker) openReader(ctx context.Context, effect func() bool) nativePilotDiagnostic {
	return b.use(ctx, func(c *nativePilotManualController) nativePilotDiagnostic {
		c.mu.Lock()
		defer c.mu.Unlock()
		d := c.checkLocked(ctx)
		if !d.OfflineValidated && !d.ManualEligible {
			return d
		}
		if effect == nil || !effect() {
			return nativePilotDiagnostic{Reason: "local-effect-rejected"}
		}
		d.Reason = "reader-admitted"
		return d
	})
}
func (b *nativePilotBroker) dispatch(ctx context.Context, work string) (*nativePilotManualTicket, nativePilotDiagnostic) {
	var ticket *nativePilotManualTicket
	d := b.use(ctx, func(c *nativePilotManualController) nativePilotDiagnostic {
		var d nativePilotDiagnostic
		ticket, d = c.dispatch(ctx, work)
		return d
	})
	return ticket, d
}
func (b *nativePilotBroker) claim(ctx context.Context, t *nativePilotManualTicket, effect func() bool) nativePilotDiagnostic {
	return b.use(ctx, func(c *nativePilotManualController) nativePilotDiagnostic { return c.claim(ctx, t, effect) })
}
func (b *nativePilotBroker) complete(ctx context.Context, t *nativePilotManualTicket, effect func() bool) nativePilotDiagnostic {
	return b.use(ctx, func(c *nativePilotManualController) nativePilotDiagnostic { return c.complete(ctx, t, effect) })
}
func (b *nativePilotBroker) cancel(work string) nativePilotDiagnostic {
	d := b.use(context.Background(), func(c *nativePilotManualController) nativePilotDiagnostic { return c.cancel(work) })
	b.mu.Lock()
	onStop := b.onStop
	b.mu.Unlock()
	if onStop != nil && d.Reason == "work-canceled" {
		onStop()
	}
	return d
}
