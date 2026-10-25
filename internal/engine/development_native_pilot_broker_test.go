//go:build auth_session_dev

package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// All OS/process facts in these ports are invented, trusted offline test input.
// The tests prove broker ordering and fencing, not Windows ownership attestation.
type testNativeBroker struct {
	mu       sync.Mutex
	f        *testNativePilotFixture
	cfg      nativePilotBrokerConfig
	ports    nativePilotBrokerPorts
	state    nativePilotBrokerState
	actual   map[string]nativePilotOwner
	ready    map[string]nativePilotPreparedRole
	acquired map[string]int
	aborts   map[string]bool
}

func newTestNativeBroker() *testNativeBroker {
	f := newTestNativePilotFixture()
	meta := f.cfg.binding
	x := &testNativeBroker{f: f, actual: map[string]nativePilotOwner{}, ready: map[string]nativePilotPreparedRole{}, acquired: map[string]int{}, aborts: map[string]bool{}}
	for _, o := range []nativePilotOwner{meta.reader, meta.collector} {
		o.context = "paused-context-" + o.role
		o.document = "paused-document-" + o.role
		x.actual[o.role] = o
	}
	x.cfg = nativePilotBrokerConfig{enabled: true, offlineTest: true, pilot: f.cfg}
	x.cfg.pilot.binding.reader = nativePilotOwner{role: "reader", profile: meta.reader.profile, parent: meta.reader.parent}
	x.cfg.pilot.binding.collector = nativePilotOwner{role: "collector", profile: meta.collector.profile, parent: meta.collector.parent}
	x.state = nativePilotBrokerState{account: meta.account, enrollment: meta.enrollment, pair: meta.pair, attempt: meta.attempt, derivation: meta.derivation, policy: meta.policy, consent: meta.consent, readerWindows: 2, completeInventory: true, parentAlive: true, channelHealthy: true, invalidationsReady: true, optedIn: true}
	x.ports = nativePilotBrokerPorts{now: f.ports.now,
		prepare: func(_ context.Context, r nativePilotPrepareRequest) (nativePilotPreparedRole, error) {
			x.mu.Lock()
			defer x.mu.Unlock()
			p := nativePilotPreparedRole{owner: x.actual[r.role], handle: "paused-handle-" + r.role}
			x.ready[r.role] = p
			return p, nil
		},
		inspect: func(_ context.Context, p nativePilotPreparedRole) (nativePilotOwner, error) {
			x.mu.Lock()
			defer x.mu.Unlock()
			return x.actual[p.owner.role], nil
		},
		state: func(context.Context) (nativePilotBrokerState, error) {
			x.mu.Lock()
			defer x.mu.Unlock()
			return x.state, nil
		},
		abort: func(_ context.Context, p nativePilotPreparedRole, preserve bool) error {
			x.mu.Lock()
			defer x.mu.Unlock()
			x.aborts[p.owner.role] = preserve
			return nil
		},
	}
	x.ports.acquire = func(_ context.Context, p nativePilotPreparedRole, c nativePilotCaptureChallenge) ([]byte, error) {
		x.mu.Lock()
		defer x.mu.Unlock()
		if len(x.ready) != 2 || x.ready[p.owner.role] != p || x.actual[p.owner.role] != p.owner || x.acquired[p.owner.role] != 0 {
			return nil, errors.New("offline-gate-mismatch")
		}
		x.acquired[p.owner.role]++
		m := testNativeStartupWire(c)
		o := x.actual[p.owner.role]
		m["ownerId"], m["profileId"], m["contextId"], m["targetId"], m["frameId"], m["documentId"] = o.owner, o.profile, o.context, o.target, o.frame, o.document
		return json.Marshal(m)
	}
	return x
}
func (x *testNativeBroker) broker(t *testing.T, parent context.Context) *nativePilotBroker {
	t.Helper()
	if parent == nil {
		var cancel context.CancelFunc
		parent, cancel = context.WithCancel(context.Background())
		t.Cleanup(cancel)
	}
	b, d := newNativePilotBroker(parent, x.cfg, x.ports)
	assertNativePilotClosed(t, d, "admission-closed")
	if b == nil {
		t.Fatal("offline broker missing")
	}
	return b
}

func TestNativePilotBrokerCombinedPairNoFallback(t *testing.T) {
	for _, fault := range []string{"none", "error", "binding", "policy"} {
		t.Run(fault, func(t *testing.T) {
			x := newTestNativeBroker()
			originalInspect := x.ports.inspect
			legacy, combined := 0, 0
			x.ports.inspect = func(ctx context.Context, role nativePilotPreparedRole) (nativePilotOwner, error) {
				legacy++
				if legacy > 2 {
					return nativePilotOwner{}, errors.New("legacy fallback forbidden")
				}
				return originalInspect(ctx, role)
			}
			x.ports.inspectPair = func(_ context.Context, reader, collector nativePilotPreparedRole) (nativePilotPairInspection, error) {
				combined++
				pair := nativePilotPairInspection{state: x.state, reader: reader.owner, collector: collector.owner}
				switch fault {
				case "error":
					return pair, errors.New("PRIVATE_PAIR_FAILURE")
				case "binding":
					pair.collector.document = "different"
				case "policy":
					pair.state.consent++
				}
				return pair, nil
			}
			b := x.broker(t, nil)
			d := b.start(context.Background())
			if legacy != 2 || combined == 0 || d.CanAuthorize || d.ManualEligible {
				t.Fatal("combined route was bypassed or promoted")
			}
			if fault == "none" {
				if !d.OfflineValidated || combined < 3 {
					t.Fatal("combined checks missed startup acquisition or publication")
				}
			} else if d.OfflineValidated {
				t.Fatal("failed combined check fell back to an accepting route")
			}
		})
	}
}
func startNativeBroker(t *testing.T, b *nativePilotBroker) {
	t.Helper()
	d := b.start(context.Background())
	assertNativePilotClosed(t, d, "manual-attempt-valid")
	if !d.OfflineValidated {
		t.Fatal("offline pair was not admitted")
	}
}

func TestNativePilotBrokerDefaultAndUnboundSelection(t *testing.T) {
	x := newTestNativeBroker()
	x.cfg.enabled = false
	b, d := newNativePilotBroker(nil, x.cfg, nativePilotBrokerPorts{})
	assertNativePilotClosed(t, d, "admission-disabled")
	if b != nil {
		t.Fatal("disabled broker constructed")
	}
	for _, kind := range []string{"old-document", "same-profile", "no-parent", "no-enrollment"} {
		t.Run(kind, func(t *testing.T) {
			x := newTestNativeBroker()
			switch kind {
			case "old-document":
				x.cfg.pilot.binding.reader.document = "old-document"
			case "same-profile":
				x.cfg.pilot.binding.collector.profile = x.cfg.pilot.binding.reader.profile
			case "no-parent":
				x.cfg.pilot.binding.reader.parent = ""
			case "no-enrollment":
				x.cfg.pilot.binding.enrollment = ""
			}
			b, d := newNativePilotBroker(context.Background(), x.cfg, x.ports)
			assertNativePilotClosed(t, d, "invalid-configuration")
			if b != nil {
				t.Fatal("invalid selection accepted")
			}
		})
	}
}

func TestNativePilotBrokerTwoPhaseFlow(t *testing.T) {
	x := newTestNativeBroker()
	entered, release := make(chan string, 2), make(chan struct{})
	original := x.ports.prepare
	x.ports.prepare = func(ctx context.Context, r nativePilotPrepareRequest) (nativePilotPreparedRole, error) {
		entered <- r.role
		select {
		case <-ctx.Done():
			return nativePilotPreparedRole{}, ctx.Err()
		case <-release:
		}
		return original(ctx, r)
	}
	b := x.broker(t, nil)
	assertNativePilotClosed(t, b.openReader(context.Background(), func() bool { t.Fatal("reader admitted before both ready"); return true }), "admission-closed")
	done := make(chan nativePilotDiagnostic, 1)
	go func() { done <- b.start(context.Background()) }()
	for count := 0; count < 2; count++ {
		select {
		case <-entered:
		case <-time.After(time.Second):
			close(release)
			t.Fatal("preparation was not concurrent")
		}
	}
	x.f.advance(3 * time.Second) // Preparation is outside the initial joint vector.
	x.mu.Lock()
	if len(x.acquired) != 0 {
		x.mu.Unlock()
		t.Fatal("identity acquired before both ready")
	}
	x.mu.Unlock()
	close(release)
	assertNativePilotClosed(t, <-done, "manual-attempt-valid")
	collector := b.controller.vector.binding.collector
	if collector.document != "paused-document-collector" || b.controller.vector.expires != x.cfg.pilot.attemptStarted.Add(nativePilotLife) {
		t.Fatal("actual prepared pin or original attempt horizon lost")
	}
	ticket, d := b.dispatch(context.Background(), "capture-1")
	assertNativePilotClosed(t, d, "work-dispatched")
	assertNativePilotClosed(t, b.claim(context.Background(), ticket, func() bool { return true }), "work-claimed")
	assertNativePilotClosed(t, b.openReader(context.Background(), func() bool { x.mu.Lock(); x.state.readerWindows++; x.mu.Unlock(); return true }), "reader-admitted")
	results := 0
	assertNativePilotClosed(t, b.complete(context.Background(), ticket, func() bool { results++; return true }), "result-completed")
	assertNativePilotClosed(t, b.complete(context.Background(), ticket, func() bool { results++; return true }), "result-duplicate")
	if results != 1 || ticket.collector != collector || b.controller.vector.binding.collector != collector {
		t.Fatal("opening reader replaced collector or repeated result")
	}
	assertNativePilotClosed(t, b.start(context.Background()), "startup-already-attempted")
	for _, value := range []any{b, b.cfg, nativePilotPreparedRole{owner: collector, handle: "secret-handle"}, nativePilotPrepareRequest{attempt: "secret-attempt"}, x.state} {
		raw, err := json.Marshal(value)
		if err != nil || string(raw) != "{}" {
			t.Fatal("private broker serialized")
		}
		if strings.Contains(fmt.Sprintf("%#v", value), "secret-") || strings.Contains(fmt.Sprintf("%+v", value), x.state.account) {
			t.Fatal("private broker formatted")
		}
	}
	assertNativePilotClosed(t, b.invalidate("opted-out"), "opted-out")
	x.mu.Lock()
	defer x.mu.Unlock()
	if len(x.aborts) != 2 || !x.aborts["reader"] || x.aborts["collector"] {
		t.Fatal("opt-out cleanup did not preserve reader and retire collector")
	}
}

func TestNativePilotBrokerPinAndHandleFailures(t *testing.T) {
	for _, kind := range []string{"duplicate-handle", "wrong-profile", "changed-document", "changed-process", "policy-drift", "cleanup-failure"} {
		t.Run(kind, func(t *testing.T) {
			x := newTestNativeBroker()
			originalPrepare, originalInspect := x.ports.prepare, x.ports.inspect
			x.ports.prepare = func(ctx context.Context, r nativePilotPrepareRequest) (nativePilotPreparedRole, error) {
				p, err := originalPrepare(ctx, r)
				switch kind {
				case "duplicate-handle":
					p.handle = "replayed-handle"
				case "wrong-profile":
					p.owner.profile = "another-profile"
				}
				return p, err
			}
			x.ports.inspect = func(ctx context.Context, p nativePilotPreparedRole) (nativePilotOwner, error) {
				o, err := originalInspect(ctx, p)
				switch kind {
				case "changed-document":
					o.document = "post-ready-navigation"
				case "changed-process":
					o.creation = "reused-pid"
				case "policy-drift":
					x.mu.Lock()
					x.state.policy++
					x.mu.Unlock()
				}
				return o, err
			}
			if kind == "cleanup-failure" {
				x.ports.prepare = func(ctx context.Context, r nativePilotPrepareRequest) (nativePilotPreparedRole, error) {
					p, _ := originalPrepare(ctx, r)
					return p, errors.New("private-prepare-failure")
				}
				x.ports.abort = func(context.Context, nativePilotPreparedRole, bool) error {
					return errors.New("private-cleanup-failure")
				}
			}
			b := x.broker(t, nil)
			d := b.start(context.Background())
			if d.OfflineValidated || d.ManualEligible || d.CanAuthorize || !b.stopped {
				t.Fatal("invalid prepared pair admitted")
			}
			if kind == "cleanup-failure" && d.Reason != "cleanup-uncertain" {
				t.Fatal("cleanup failure concealed")
			}
			x.mu.Lock()
			defer x.mu.Unlock()
			if len(x.acquired) != 0 {
				t.Fatal("identity acquired before pin failure rejected")
			}
		})
	}
}

func TestNativePilotBrokerPartialAndLatePrepareCleanup(t *testing.T) {
	for _, late := range []bool{false, true} {
		t.Run(fmt.Sprint(late), func(t *testing.T) {
			x := newTestNativeBroker()
			originalPrepare, originalInspect := x.ports.prepare, x.ports.inspect
			readerInspected := make(chan struct{})
			var once sync.Once
			x.ports.inspect = func(ctx context.Context, p nativePilotPreparedRole) (nativePilotOwner, error) {
				o, err := originalInspect(ctx, p)
				if p.owner.role == "reader" {
					once.Do(func() { close(readerInspected) })
				}
				return o, err
			}
			x.ports.prepare = func(ctx context.Context, r nativePilotPrepareRequest) (nativePilotPreparedRole, error) {
				if r.role == "collector" {
					if late {
						<-ctx.Done()
						p, _ := originalPrepare(context.Background(), r)
						return p, nil
					}
					<-readerInspected
					return nativePilotPreparedRole{}, errors.New("private-failure")
				}
				return originalPrepare(ctx, r)
			}
			b := x.broker(t, nil)
			done := make(chan nativePilotDiagnostic, 1)
			go func() { done <- b.start(context.Background()) }()
			<-readerInspected
			if late {
				assertNativePilotClosed(t, b.invalidate("opted-out"), "opted-out")
			}
			d := <-done
			if d.OfflineValidated || d.ManualEligible {
				t.Fatal("partial pair admitted")
			}
			x.mu.Lock()
			defer x.mu.Unlock()
			want := 1
			if late {
				want = 2
			}
			if len(x.aborts) != want {
				t.Fatal("returned before pending owner cleanup acknowledged")
			}
		})
	}
}

func TestNativePilotBrokerUncooperativePrepareCleanupUncertain(t *testing.T) {
	x := newTestNativeBroker()
	entered, release := make(chan struct{}, 2), make(chan struct{})
	original := x.ports.prepare
	x.ports.prepare = func(_ context.Context, r nativePilotPrepareRequest) (nativePilotPreparedRole, error) {
		entered <- struct{}{}
		<-release
		return original(context.Background(), r)
	}
	b := x.broker(t, nil)
	done := make(chan nativePilotDiagnostic, 1)
	go func() { done <- b.start(context.Background()) }()
	<-entered
	<-entered
	d := b.invalidate("opted-out")
	assertNativePilotClosed(t, d, "cleanup-uncertain")
	close(release)
	assertNativePilotClosed(t, <-done, "cleanup-uncertain")
	select {
	case <-b.prepareDone:
	case <-time.After(time.Second):
		t.Fatal("late prepare cleanup did not finish")
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	if len(x.aborts) != 2 {
		t.Fatal("late owner abandoned after cleanup timeout")
	}
}

func TestNativePilotBrokerOriginalClockAndAdmissionFences(t *testing.T) {
	for _, kind := range []string{"attempt-expired", "joint-after-ready", "clock-regressed", "last-reader", "consent", "parent-exit"} {
		t.Run(kind, func(t *testing.T) {
			x := newTestNativeBroker()
			parent, cancel := context.WithCancel(context.Background())
			defer cancel()
			if kind == "attempt-expired" {
				original := x.ports.prepare
				var once sync.Once
				x.ports.prepare = func(ctx context.Context, r nativePilotPrepareRequest) (nativePilotPreparedRole, error) {
					once.Do(func() { x.f.advance(nativePilotLife) })
					return original(ctx, r)
				}
			}
			if kind == "joint-after-ready" {
				original := x.ports.acquire
				var once sync.Once
				x.ports.acquire = func(ctx context.Context, p nativePilotPreparedRole, c nativePilotCaptureChallenge) ([]byte, error) {
					once.Do(func() { x.f.advance(nativePilotStartup) })
					return original(ctx, p, c)
				}
			}
			b := x.broker(t, parent)
			if kind == "attempt-expired" || kind == "joint-after-ready" {
				d := b.start(context.Background())
				if d.OfflineValidated || d.ManualEligible || !b.stopped {
					t.Fatal("expired acquisition admitted")
				}
				return
			}
			startNativeBroker(t, b)
			ticket, d := b.dispatch(context.Background(), "capture-1")
			assertNativePilotClosed(t, d, "work-dispatched")
			b.claim(context.Background(), ticket, func() bool { return true })
			switch kind {
			case "clock-regressed":
				x.f.advance(-time.Nanosecond)
			case "last-reader":
				x.mu.Lock()
				x.state.readerWindows = 0
				x.mu.Unlock()
			case "consent":
				x.mu.Lock()
				x.state.consent++
				x.mu.Unlock()
			case "parent-exit":
				cancel()
			}
			d = b.complete(context.Background(), ticket, func() bool { t.Fatal("invalidated result committed"); return true })
			if d.OfflineValidated || d.ManualEligible {
				t.Fatal("late result accepted")
			}
			if kind == "parent-exit" {
				x.mu.Lock()
				defer x.mu.Unlock()
				if len(x.aborts) != 2 || x.aborts["reader"] {
					t.Fatal("parent exit preserved an owner")
				}
			}
		})
	}
}
