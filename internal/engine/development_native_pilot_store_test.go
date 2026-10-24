//go:build auth_session_dev && auth_session_fixture

package engine

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/AkuSidecar/internal/store"
)

type nativeStoreClock struct {
	f             *manualRuntimeFixture
	probe         atomic.Bool
	lockViolation atomic.Bool
	hook          func()
}

func (c *nativeStoreClock) Now() time.Time {
	if c.hook != nil {
		c.hook()
	}
	if c.probe.Load() {
		if !c.f.broker.mu.TryLock() {
			c.lockViolation.Store(true)
		} else {
			c.f.broker.mu.Unlock()
		}
		if !c.f.broker.controller.mu.TryLock() {
			c.lockViolation.Store(true)
		} else {
			c.f.broker.controller.mu.Unlock()
		}
	}
	c.f.mu.Lock()
	defer c.f.mu.Unlock()
	return c.f.now
}

type nativeStoreBackend struct {
	*manualRuntimeFixtureProcess
	calls atomic.Int32
}

func (b *nativeStoreBackend) Capture(ctx context.Context, source domain.Source, payload map[string]any) (domain.Observation, error) {
	b.calls.Add(1)
	o, err := b.manualRuntimeFixtureProcess.Capture(ctx, source, payload)
	if err != nil {
		return o, err
	}
	o.Coverage = map[string]any{"synthetic": true}
	o.Snapshots = []domain.Snapshot{{Blocks: []domain.Block{{Text: "Synthetic evidence for a bounded Store test"}}}}
	return o, nil
}

type nativeStoreFixture struct {
	f          *manualRuntimeFixture
	s          *store.Store
	e          *Engine
	b          *nativeStoreBackend
	command    domain.BridgeCommand
	session    domain.Session
	path       string
	deliveries atomic.Int32
	clock      *nativeStoreClock
}

func newNativeStoreFixture(t *testing.T, trusted bool, beforeStart func(*manualRuntimeFixture)) *nativeStoreFixture {
	t.Helper()
	f, err := newManualRuntimeFixture(false)
	if err != nil {
		t.Fatal(err)
	}
	// Select the recipe BEFORE any startup vector exists. All ports remain
	// invented/in-memory; this never relabels an offline validated vector.
	f.broker.cfg.offlineTest = !trusted
	if beforeStart != nil {
		beforeStart(f)
	}
	b := &nativeStoreBackend{manualRuntimeFixtureProcess: f.backend}
	r, err := newNativePilotRuntime(f.broker, f.reader, f.collector, b, func(context.Context, readerbroker.Request) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	f.runtime = r
	path := filepath.Join(t.TempDir(), "synthetic.db")
	clock := &nativeStoreClock{f: f}
	s, err := store.OpenWithClock(path, domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", false), clock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.close(); s.Close() })
	settings, err := s.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	settings.CollectionMode = "headless"
	settings.ActiveSources = []domain.Source{domain.SourceX}
	session, err := s.CreateUpdateSession(context.Background(), "synthetic Store integration", settings, domain.UpdatePolicy{Trigger: domain.UpdateTriggerUser, Delivery: domain.UpdateDeliveryVisible, BudgetAuthority: domain.BudgetAuthorityUser})
	if err != nil {
		t.Fatal(err)
	}
	command, err := s.StartRun(context.Background(), session.Runs[0].ID, map[string]any{"captureRuntime": map[string]any{"driver": "headless"}})
	if err != nil {
		t.Fatal(err)
	}
	x := &nativeStoreFixture{f: f, s: s, e: &Engine{store: s, config: config.Config{Dev: true}}, b: b, command: command, session: session, path: path, clock: clock}
	return x
}
func (x *nativeStoreFixture) bind(t *testing.T) {
	t.Helper()
	d := x.f.broker.start(context.Background())
	if !d.ManualEligible || d.CanAuthorize || d.NativeQualified || d.StorePermit {
		t.Fatalf("startup %+v", d)
	}
	if err := x.e.bindNativePilotStore(context.Background(), x.f.runtime, x.session.ID, x.command.RunID); err != nil {
		t.Fatal(err)
	}
	x.f.runtime.storeLink.afterCommit = func(_ context.Context, r domain.Run) (domain.Run, error) { x.deliveries.Add(1); return r, nil }
}
func (x *nativeStoreFixture) capture() nativePilotStoreResult {
	return x.e.captureNativePilot(context.Background(), x.f.runtime, x.command.ID)
}
func (x *nativeStoreFixture) count(t *testing.T, want int) {
	t.Helper()
	obs, err := x.s.Observations(context.Background(), x.command.RunID)
	if err != nil || len(obs) != want {
		t.Fatalf("observations=%d err=%v", len(obs), err)
	}
}

func TestNativePilotStoreSuccessReplayAndGenericFence(t *testing.T) {
	x := newNativeStoreFixture(t, true, nil)
	x.bind(t)
	x.clock.probe.Store(true)
	// Backend and continuation can acquire broker/controller locks: neither
	// executes from an atomic bool effect callback.
	x.f.backend.probe = func() error {
		if !x.f.broker.mu.TryLock() {
			return errors.New("broker lock held")
		}
		x.f.broker.mu.Unlock()
		if !x.f.broker.controller.mu.TryLock() {
			return errors.New("controller lock held")
		}
		x.f.broker.controller.mu.Unlock()
		return nil
	}
	close(x.f.backend.release)
	got := x.capture()
	if got.Outcome != store.Committed || got.PersistenceError != nil || got.DeliveryError != nil {
		t.Fatalf("capture %+v", got)
	}
	x.count(t, 1)
	if x.clock.lockViolation.Load() {
		t.Fatal("Store I/O occurred under broker/controller lock")
	}
	if err := x.s.SaveObservation(context.Background(), x.command.ID, x.command.RunID, domain.Observation{Source: domain.SourceX}); err == nil {
		t.Fatal("generic bypass")
	}
	x.f.broker.invalidate("opted-out")
	got = x.capture()
	if got.Outcome != store.AlreadyCommitted || got.PersistenceError != nil || x.b.calls.Load() != 1 || x.deliveries.Load() != 1 {
		t.Fatalf("replay %+v capture=%d delivery=%d", got, x.b.calls.Load(), x.deliveries.Load())
	}
	x.count(t, 1)
}

func TestNativePilotStoreAdmissionFailures(t *testing.T) {
	for _, scenario := range []string{"offline", "before-start", "disabled", "engine-disabled", "bad-document", "pair-drift", "owner-drift"} {
		t.Run(scenario, func(t *testing.T) {
			x := newNativeStoreFixture(t, scenario != "offline", func(f *manualRuntimeFixture) {
				if scenario == "disabled" {
					f.broker.cfg.pilot.enabled = false
				}
				if scenario == "bad-document" {
					original := f.broker.ports.acquire
					f.broker.ports.acquire = func(ctx context.Context, p nativePilotPreparedRole, c nativePilotCaptureChallenge) ([]byte, error) {
						raw, err := original(ctx, p, c)
						if err != nil {
							return nil, err
						}
						var v map[string]any
						_ = json.Unmarshal(raw, &v)
						v["documentId"] = "wrong-document"
						return json.Marshal(v)
					}
				}
			})
			if scenario != "before-start" {
				x.f.broker.start(context.Background())
			}
			if scenario == "engine-disabled" {
				x.e.config.Dev = false
			}
			if scenario == "pair-drift" {
				x.f.mu.Lock()
				x.f.state.pair = "different-pair"
				x.f.mu.Unlock()
			}
			if scenario == "owner-drift" {
				x.f.mu.Lock()
				o := x.f.actual["collector"]
				o.document = "changed-document"
				x.f.actual["collector"] = o
				x.f.mu.Unlock()
			}
			if err := x.e.bindNativePilotStore(context.Background(), x.f.runtime, x.session.ID, x.command.RunID); err == nil {
				t.Fatal("invalid startup issued Store authority")
			}
			if got := x.capture(); got.Outcome != store.RejectedNoMutation || x.b.calls.Load() != 0 {
				t.Fatalf("unexpected capture %+v", got)
			}
			x.count(t, 0)
		})
	}
}

func TestNativePilotStoreExpiryCancelAndInvalidationDuringCapture(t *testing.T) {
	for _, scenario := range []string{"expiry", "short-limit", "cancel-work", "cancel-context", "revoke", "wrong-source", "owner-drift"} {
		t.Run(scenario, func(t *testing.T) {
			x := newNativeStoreFixture(t, true, func(f *manualRuntimeFixture) {
				if scenario == "expiry" {
					f.broker.cfg.pilot.attemptStarted = f.now.Add(-50 * time.Second)
				}
				if scenario == "short-limit" {
					f.broker.cfg.pilot.attemptValidUntil = f.now.Add(3 * time.Second)
				}
			})
			x.bind(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			result := make(chan nativePilotStoreResult, 1)
			go func() { result <- x.e.captureNativePilot(ctx, x.f.runtime, x.command.ID) }()
			select {
			case <-x.f.backend.entered:
			case <-time.After(3 * time.Second):
				t.Fatal("capture did not begin")
			}
			switch scenario {
			case "expiry":
				x.f.mu.Lock()
				x.f.now = x.f.now.Add(10 * time.Second)
				x.f.mu.Unlock()
			case "short-limit":
				x.f.mu.Lock()
				x.f.now = x.f.now.Add(3 * time.Second)
				x.f.mu.Unlock()
			case "cancel-work":
				x.f.broker.cancel(x.command.ID)
			case "cancel-context":
				cancel()
			case "revoke":
				x.f.broker.invalidate("opted-out")
			case "wrong-source":
				x.f.backend.resultSource = domain.SourceLinkedIn
			case "owner-drift":
				x.f.backend.owner.document = "changed-document"
			}
			close(x.f.backend.release)
			var got nativePilotStoreResult
			select {
			case got = <-result:
			case <-time.After(4 * time.Second):
				t.Fatal("capture did not close")
			}
			if got.Outcome == store.Committed || got.Outcome == store.AlreadyCommitted {
				t.Fatalf("invalid result committed %+v", got)
			}
			x.count(t, 0)
			x.capture()
			if x.b.calls.Load() != 1 || x.deliveries.Load() != 0 {
				t.Fatal("failed operation recaptured or delivered")
			}
		})
	}
}

func TestNativePilotStoreCommittedDeliveryFailureAndRestart(t *testing.T) {
	x := newNativeStoreFixture(t, true, nil)
	x.bind(t)
	x.f.runtime.storeLink.afterCommit = func(context.Context, domain.Run) (domain.Run, error) {
		x.deliveries.Add(1)
		return domain.Run{}, errors.New("synthetic scheduling failure")
	}
	close(x.f.backend.release)
	got := x.capture()
	if got.Outcome != store.Committed || got.PersistenceError != nil || got.DeliveryError == nil {
		t.Fatalf("lost committed result %+v", got)
	}
	x.count(t, 1)
	got = x.capture()
	if got.Outcome != store.AlreadyCommitted || x.b.calls.Load() != 1 || x.deliveries.Load() != 1 {
		t.Fatalf("replay %+v", got)
	}
	opID := "result:" + x.f.runtime.storeLink.revokeID + ":" + x.command.ID
	x.f.close()
	x.s.Close()
	reopened, err := store.OpenWithClock(x.path, domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", false), &nativeStoreClock{f: x.f})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	receipt, err := reopened.LookupManualPilotOp(context.Background(), opID)
	if err != nil || receipt.Outcome != store.Committed {
		t.Fatalf("receipt %+v %v", receipt, err)
	}
	if result, err := reopened.ClaimManualPilot(context.Background(), x.f.runtime.storeLink.permit, x.command.ID, "restart-claim"); err == nil || result.Handle != nil {
		t.Fatal("restart restored authority")
	}
	if err := reopened.SaveObservation(context.Background(), x.command.ID, x.command.RunID, domain.Observation{Source: domain.SourceX}); err == nil {
		t.Fatal("restart removed generic fence")
	}
}

func TestNativePilotStoreFollowupFreshClaimAndOriginalDeadline(t *testing.T) {
	x := newNativeStoreFixture(t, true, func(f *manualRuntimeFixture) { f.broker.cfg.pilot.attemptStarted = f.now.Add(-50 * time.Second) })
	x.bind(t)
	close(x.f.backend.release)
	if got := x.capture(); got.Outcome != store.Committed {
		t.Fatalf("first %+v", got)
	}
	ctx := context.Background()
	next, err := x.s.QueueFollowUp(ctx, x.command.RunID, map[string]any{"captureRuntime": map[string]any{"driver": "headless"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := x.e.captureNativePilot(ctx, x.f.runtime, next.ID); got.Outcome != store.Committed || got.ClaimOutcome != store.Committed {
		t.Fatalf("followup %+v", got)
	}
	x.count(t, 2)
	last, err := x.s.QueueFollowUp(ctx, x.command.RunID, map[string]any{"captureRuntime": map[string]any{"driver": "headless"}})
	if err != nil {
		t.Fatal(err)
	}
	x.f.mu.Lock()
	x.f.now = x.f.now.Add(10 * time.Second)
	x.f.mu.Unlock()
	if got := x.e.captureNativePilot(ctx, x.f.runtime, last.ID); got.Outcome != store.RejectedNoMutation {
		t.Fatalf("deadline renewed %+v", got)
	}
	if x.b.calls.Load() != 2 {
		t.Fatal("expired followup captured")
	}
}

func TestNativePilotStoreIssuanceInvalidationRace(t *testing.T) {
	x := newNativeStoreFixture(t, true, nil)
	if d := x.f.broker.start(context.Background()); !d.ManualEligible {
		t.Fatalf("startup %+v", d)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	x.clock.hook = func() { once.Do(func() { close(entered); <-release }) }
	result := make(chan error, 1)
	go func() {
		result <- x.e.bindNativePilotStore(context.Background(), x.f.runtime, x.session.ID, x.command.RunID)
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("issuance did not reach Store")
	}
	x.f.broker.invalidate("opted-out")
	close(release)
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("invalidation race published permit")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("issuance did not close")
	}
	if got := x.capture(); got.Outcome != store.RejectedNoMutation || x.b.calls.Load() != 0 {
		t.Fatalf("capture %+v", got)
	}
	if _, err := x.f.runtime.capture(context.Background(), "legacy", domain.SourceX); err == nil {
		t.Fatal("legacy capture bypassed binding")
	}
	if err := x.f.runtime.complete(context.Background(), nil, domain.Observation{Source: domain.SourceX}); err == nil {
		t.Fatal("legacy result bypassed binding")
	}
}

func TestNativePilotStoreActualEngineContinuation(t *testing.T) {
	x := newNativeStoreFixture(t, true, nil)
	x.e = New(x.s, reasoning.Deterministic{}, config.Config{Dev: true, Capture: config.CaptureConfig{MaxAcquisitionRounds: 1}}, log.New(io.Discard, "", 0))
	t.Cleanup(func() { x.e.Shutdown(); x.e.WaitForIdle(3 * time.Second) })
	if d := x.f.broker.start(context.Background()); !d.ManualEligible {
		t.Fatalf("startup %+v", d)
	}
	if err := x.e.bindNativePilotStore(context.Background(), x.f.runtime, x.session.ID, x.command.RunID); err != nil {
		t.Fatal(err)
	}
	close(x.f.backend.release)
	got := x.capture()
	if got.Outcome != store.Committed || got.PersistenceError != nil || got.DeliveryError != nil {
		t.Fatalf("capture %+v", got)
	}
	if !x.e.WaitForIdle(3 * time.Second) {
		t.Fatal("Engine continuation did not finish")
	}
	run, err := x.s.GetRun(context.Background(), x.command.RunID)
	if err != nil || run.Status != "completed" {
		t.Fatalf("actual continuation run=%+v err=%v", run, err)
	}
	x.count(t, 1)
}
