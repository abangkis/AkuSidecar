//go:build auth_session_dev && auth_session_fixture

package engine

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/collection"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
	"github.com/abangkis/AkuSidecar/internal/store"
)

func startApplicationFixture(t *testing.T, x *nativeStoreFixture, parent context.Context, prepare func(context.Context, readerbroker.Request, string) (readerbroker.NativePostPreparation, error), dispose func(context.Context) error) *xManualApplication {
	t.Helper()
	r, err := x.e.startXManualApplication(context.Background(), parent, x.session.ID, x.command.RunID, x.command.ID, func(context.Context, context.Context) (xManualApplicationPorts, error) {
		return xManualApplicationPorts{runtime: x.f.runtime, admission: nativePilotNativeStoreAdmission{broker: x.f.broker, store: x.s}, prepare: prepare, dispose: dispose}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	x.f.runtime.storeLink.afterCommit = func(_ context.Context, v domain.Run) (domain.Run, error) { x.deliveries.Add(1); return v, nil }
	t.Cleanup(func() { _ = r.stop(context.Background(), "parent-exited") })
	return r
}
func applicationReaderRequest() readerbroker.Request {
	return readerbroker.Request{RequestID: "broker_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Source: "x", URL: "https://x.com/example/status/123"}
}
func applicationPrepared(context.Context, readerbroker.Request, string) (readerbroker.NativePostPreparation, error) {
	return readerbroker.NativePostPreparation{Target: readerbroker.Target{PID: 123, HWND: 42}, VerifyForeground: func(context.Context) error { return nil }, NavigatePost: func(context.Context) error { return nil }}, nil
}

func waitXApplicationCapture(t *testing.T, r *xManualApplication) {
	t.Helper()
	select {
	case <-r.captureDone:
	case <-time.After(time.Second):
		t.Fatal("private capture did not settle")
	}
}

func TestXApplicationRouteDisabledAndNoFactoryWithoutDev(t *testing.T) {
	x := newNativeStoreFixture(t, true, nil)
	if v := x.e.XManualStatus(); v.State != "disabled" || v.Available {
		t.Fatalf("status %+v", v)
	}
	if err := x.e.StopXManual(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, selected, err := x.e.PrepareXManualReader(context.Background(), applicationReaderRequest(), "marker")
	if err != nil || selected {
		t.Fatal("disabled selected a reader")
	}
	x.e.config.Dev = false
	called := false
	_, err = x.e.startXManualApplication(context.Background(), context.Background(), x.session.ID, x.command.RunID, x.command.ID, func(context.Context, context.Context) (xManualApplicationPorts, error) {
		called = true
		return xManualApplicationPorts{}, nil
	})
	if err == nil || called {
		t.Fatal("disabled invoked factory")
	}
}

func TestXApplicationRouteStoreReaderAndReplay(t *testing.T) {
	x := newNativeStoreFixture(t, true, nil)
	var disposed atomic.Int32
	r := startApplicationFixture(t, x, context.Background(), applicationPrepared, func(context.Context) error { disposed.Add(1); return nil })
	if v := x.e.XManualStatus(); v.State != "active" || v.RemainingMS <= 0 {
		t.Fatalf("status %+v", v)
	}
	if cmd, err := x.e.ClaimCommand(context.Background(), x.command.RunID, "ordinary"); err != nil || cmd != nil {
		t.Fatal("ordinary X claim escaped routing gate")
	}
	release, ok := x.e.ordinarySourceRoute(domain.SourceLinkedIn)
	release()
	if !ok {
		t.Fatal("non-X route blocked")
	}
	prepared, selected, err := x.e.PrepareXManualReader(context.Background(), applicationReaderRequest(), "marker")
	if err != nil || !selected || prepared.Target.HWND != 42 {
		t.Fatalf("reader selected=%t err=%v", selected, err)
	}
	close(x.f.backend.release)
	x.e.stepXManual(context.Background())
	x.e.stepXManual(context.Background())
	waitXApplicationCapture(t, r)
	x.count(t, 1)
	if x.b.calls.Load() != 1 || x.deliveries.Load() != 1 {
		t.Fatal("duplicate native dispatch")
	}
	if err = x.e.StopXManual(context.Background()); err != nil {
		t.Fatal(err)
	}
	if v := r.status(); v.State != "stopped" || v.Reason != "opted-out" || v.Cleanup != "verified" {
		t.Fatalf("stop %+v", v)
	}
	if disposed.Load() != 0 {
		t.Fatal("opt-out disposed preserved reader")
	}
	if got := x.capture(); got.Outcome != store.AlreadyCommitted {
		t.Fatalf("replay %+v", got)
	}
	if _, err = x.s.ClaimCommand(context.Background(), x.command.RunID, "bypass"); err == nil {
		t.Fatal("stopped manual work lost Store fence")
	}
	if err = r.stop(context.Background(), "parent-exited"); err != nil || disposed.Load() != 1 {
		t.Fatal("parent disposal missing")
	}
	raw, _ := json.Marshal(r.status())
	for _, secret := range []string{x.session.ID, x.command.ID, "accountId", "profile", "digest"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("private status leak")
		}
	}
}

func TestXApplicationRouteStopHeldCaptureAndFirstCause(t *testing.T) {
	x := newNativeStoreFixture(t, true, nil)
	r := startApplicationFixture(t, x, context.Background(), applicationPrepared, func(context.Context) error { return nil })
	done := make(chan struct{})
	go func() { x.e.stepXManual(context.Background()); close(done) }()
	select {
	case <-x.f.backend.entered:
	case <-time.After(time.Second):
		t.Fatal("capture not entered")
	}
	if _, selected, err := x.e.PrepareXManualReader(context.Background(), applicationReaderRequest(), "marker"); err != nil || !selected {
		t.Fatal("reader blocked behind capture")
	}
	if err := x.e.StopXManual(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("capture did not cancel")
	}
	waitXApplicationCapture(t, r)
	x.count(t, 0)
	if got := r.status(); got.Reason != "opted-out" {
		t.Fatalf("cleanup or capture replaced first cause %+v", got)
	}
	if got := x.capture(); got.Outcome == store.Committed || x.b.calls.Load() != 1 {
		t.Fatal("canceled capture resurrected")
	}
}

func TestXApplicationRouteStopDuringStartup(t *testing.T) {
	x := newNativeStoreFixture(t, true, nil)
	entered := make(chan struct{})
	finished := make(chan error, 1)
	var disposed atomic.Int32
	go func() {
		_, err := x.e.startXManualApplication(context.Background(), context.Background(), x.session.ID, x.command.RunID, x.command.ID, func(ctx, parent context.Context) (xManualApplicationPorts, error) {
			close(entered)
			<-ctx.Done()
			return xManualApplicationPorts{dispose: func(context.Context) error { disposed.Add(1); return nil }}, errors.New("private startup error")
		})
		finished <- err
	}()
	<-entered
	if err := x.e.StopXManual(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := <-finished; err == nil {
		t.Fatal("canceled startup accepted")
	}
	if v := x.e.XManualStatus(); v.State != "stopped" || v.Reason != "opted-out" || disposed.Load() != 1 {
		t.Fatalf("startup stop %+v disposals=%d", v, disposed.Load())
	}
}

func TestXApplicationRouteUncertainCleanupStaysFenced(t *testing.T) {
	x := newNativeStoreFixture(t, true, func(f *manualRuntimeFixture) {
		f.broker.ports.abort = func(context.Context, nativePilotPreparedRole, bool) error { return errors.New("private cleanup cause") }
	})
	r := startApplicationFixture(t, x, context.Background(), applicationPrepared, func(context.Context) error { return nil })
	if err := x.e.StopXManual(context.Background()); err == nil {
		t.Fatal("uncertain cleanup accepted")
	}
	if v := r.status(); v.State != "failed" || v.Reason != "opted-out" || v.Cleanup != "uncertain" {
		t.Fatalf("cleanup %+v", v)
	}
	release, allowed := x.e.ordinarySourceRoute(domain.SourceX)
	release()
	if allowed {
		t.Fatal("uncertain source unfenced")
	}
}

func TestXApplicationRouteBusyBeforeFactory(t *testing.T) {
	x := newNativeStoreFixture(t, true, nil)
	if cmd, err := x.s.ClaimCommandForCollector(context.Background(), x.command.RunID, "ordinary", collection.BackendHeadless); err != nil || cmd == nil {
		t.Fatal(err)
	}
	called := false
	_, err := x.e.startXManualApplication(context.Background(), context.Background(), x.session.ID, x.command.RunID, x.command.ID, func(context.Context, context.Context) (xManualApplicationPorts, error) {
		called = true
		return xManualApplicationPorts{}, nil
	})
	if err == nil || called {
		t.Fatal("busy owner acquired a profile")
	}
}

func TestXApplicationRouteExpiryAndBrokerCause(t *testing.T) {
	x := newNativeStoreFixture(t, true, nil)
	r := startApplicationFixture(t, x, context.Background(), applicationPrepared, func(context.Context) error { return nil })
	x.f.mu.Lock()
	x.f.now = x.f.now.Add(nativePilotLife)
	x.f.mu.Unlock()
	x.f.broker.check(context.Background())
	if v := r.status(); v.State != "stopped" || v.Reason != "attempt-expired" || v.RemainingMS != 0 {
		t.Fatalf("expiry %+v", v)
	}
}

func TestXApplicationRoutePartialFactoryCleanup(t *testing.T) {
	x := newNativeStoreFixture(t, true, nil)
	var disposed atomic.Int32
	r, err := x.e.startXManualApplication(context.Background(), context.Background(), x.session.ID, x.command.RunID, x.command.ID, func(context.Context, context.Context) (xManualApplicationPorts, error) {
		return xManualApplicationPorts{runtime: &nativePilotRuntime{}, dispose: func(context.Context) error { disposed.Add(1); return nil }}, nil
	})
	if err == nil || r == nil || disposed.Load() != 1 || r.status().Reason != "startup-failed" {
		t.Fatal("partial factory not safely retired")
	}
}

func TestXApplicationRoutePendingSelectionPreservesOtherSources(t *testing.T) {
	x := newNativeStoreFixture(t, true, nil, domain.SourceX, domain.SourceLinkedIn)
	var runID string
	for _, run := range x.session.Runs {
		if run.Source == domain.SourceLinkedIn {
			runID = run.ID
		}
	}
	if runID == "" {
		t.Fatal("synthetic LinkedIn run missing")
	}
	cmd, err := x.s.StartRun(context.Background(), runID, map[string]any{"captureRuntime": map[string]any{"driver": "headless"}})
	if err != nil {
		t.Fatal(err)
	}
	startApplicationFixture(t, x, context.Background(), applicationPrepared, func(context.Context) error { return nil })
	id, err := x.e.ordinaryPendingRun(context.Background(), collection.BackendHeadless)
	if err != nil || id != cmd.RunID {
		t.Fatalf("non-X candidate=%q error=%v", id, err)
	}
}

func TestXApplicationRouteStopKeepsEarlierBrokerCauseWithoutStatusRead(t *testing.T) {
	for _, uncertain := range []bool{false, true} {
		t.Run(map[bool]string{false: "verified", true: "uncertain"}[uncertain], func(t *testing.T) {
			x := newNativeStoreFixture(t, true, func(f *manualRuntimeFixture) {
				if uncertain {
					f.broker.ports.abort = func(context.Context, nativePilotPreparedRole, bool) error { return errors.New("private cleanup cause") }
				}
			})
			r := startApplicationFixture(t, x, context.Background(), applicationPrepared, func(context.Context) error { return nil })
			x.f.broker.stop("owner-or-policy-changed")
			_ = x.e.StopXManual(context.Background())
			v := r.status()
			if v.Reason != "owner-or-policy-changed" {
				t.Fatalf("later API stop replaced broker cause %+v", v)
			}
			if uncertain && v.Cleanup != "uncertain" {
				t.Fatal("cleanup uncertainty lost")
			}
		})
	}
}

func TestXApplicationRouteShutdownClosesEngineBeforeOwnerCleanup(t *testing.T) {
	x := newNativeStoreFixture(t, true, nil)
	before, err := x.s.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var schedulerCanceled atomic.Bool
	x.e.autoCancel = func() { schedulerCanceled.Store(true) }
	var orderFailure atomic.Bool
	r := startApplicationFixture(t, x, context.Background(), applicationPrepared, func(ctx context.Context) error {
		if !x.e.mu.TryRLock() {
			orderFailure.Store(true)
			return errors.New("Engine lock held during external disposal")
		}
		closed := x.e.shuttingDown
		x.e.mu.RUnlock()
		if !closed || !schedulerCanceled.Load() {
			orderFailure.Store(true)
		}
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > nativePilotCleanup {
			orderFailure.Store(true)
		}
		return nil
	})
	x.e.Shutdown()
	if orderFailure.Load() || r.status().State != "stopped" {
		t.Fatal("shutdown cleanup order or bound rejected")
	}
	after, err := x.s.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if before.AutoUpdateEnabled != after.AutoUpdateEnabled {
		t.Fatal("shutdown changed saved Auto Update preference")
	}
}

func TestXApplicationRouteStopCancelsReaderPreparation(t *testing.T) {
	x := newNativeStoreFixture(t, true, nil)
	entered := make(chan struct{})
	result := make(chan error, 1)
	request, cancelRequest := context.WithCancel(context.Background())
	defer cancelRequest()
	r := startApplicationFixture(t, x, context.Background(), func(ctx context.Context, _ readerbroker.Request, _ string) (readerbroker.NativePostPreparation, error) {
		close(entered)
		<-ctx.Done()
		return readerbroker.NativePostPreparation{}, ctx.Err()
	}, func(context.Context) error { return nil })
	go func() {
		_, err := r.prepareReader(request, applicationReaderRequest(), "/synthetic-marker")
		result <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("reader preparation did not start")
	}
	if err := r.stop(context.Background(), "opted-out"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("stopped reader preparation accepted")
		}
	case <-time.After(time.Second):
		t.Fatal("Stop left human preparation waiting")
	}
	if request.Err() != nil {
		t.Fatal("internal stop canceled the caller's context")
	}
}

func TestXApplicationRouteStartupStopTimeoutRetainsSourceUntilCleanup(t *testing.T) {
	x := newNativeStoreFixture(t, true, nil)
	entered, release := make(chan struct{}), make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		_, err := x.e.startXManualApplication(context.Background(), context.Background(), x.session.ID, x.command.RunID, x.command.ID, func(context.Context, context.Context) (xManualApplicationPorts, error) {
			close(entered)
			<-release
			return xManualApplicationPorts{dispose: func(context.Context) error { return nil }}, errors.New("private late startup failure")
		})
		finished <- err
	}()
	<-entered
	call, cancel := context.WithTimeout(context.Background(), 15*time.Millisecond)
	defer cancel()
	if err := x.e.StopXManual(call); err == nil {
		t.Fatal("unsettled startup cleanup accepted")
	}
	if v := x.e.XManualStatus(); v.State != "stopping" || v.Reason != "opted-out" || v.Cleanup != "uncertain" {
		t.Fatalf("timeout %+v", v)
	}
	unlock, allowed := x.e.ordinarySourceRoute(domain.SourceX)
	unlock()
	if allowed {
		t.Fatal("unknown cleanup released X")
	}
	close(release)
	select {
	case err := <-finished:
		if err == nil {
			t.Fatal("late startup accepted")
		}
	case <-time.After(time.Second):
		t.Fatal("late resources not retired")
	}
	if v := x.e.XManualStatus(); v.State != "stopped" || v.Cleanup != "verified" {
		t.Fatalf("late cleanup %+v", v)
	}
}

func TestXApplicationRouteBuildsRuntimeAfterStartupBeforeStore(t *testing.T) {
	for _, scenario := range []string{"success", "construction-failure", "wrong-broker"} {
		t.Run(scenario, func(t *testing.T) {
			x := newNativeStoreFixture(t, true, nil)
			var constructed bool
			r, err := x.e.startXManualApplication(context.Background(), context.Background(), x.session.ID, x.command.RunID, x.command.ID, func(context.Context, context.Context) (xManualApplicationPorts, error) {
				return xManualApplicationPorts{broker: x.f.broker, admission: nativePilotNativeStoreAdmission{broker: x.f.broker, store: x.s}, prepare: applicationPrepared, dispose: func(context.Context) error { return nil }, buildRuntime: func(ctx context.Context) (*nativePilotRuntime, error) {
					if !x.f.broker.check(ctx).ManualEligible || x.f.runtime.storeLink != nil {
						t.Error("runtime phase bypassed startup/Store order")
					}
					constructed = true
					if scenario == "construction-failure" {
						return nil, errors.New("private construction failure")
					}
					if scenario == "wrong-broker" {
						return &nativePilotRuntime{broker: &nativePilotBroker{}}, nil
					}
					return newNativePilotRuntime(x.f.broker, x.f.reader, x.f.collector, x.b, func(context.Context, readerbroker.Request) error { return nil })
				}}, nil
			})
			if !constructed || r == nil {
				t.Fatal("runtime construction phase not reached")
			}
			t.Cleanup(func() { _ = r.stop(context.Background(), "parent-exited") })
			if scenario != "success" {
				if err == nil || r.status().State != "stopped" {
					t.Fatal("invalid runtime published")
				}
				x.count(t, 0)
				return
			}
			if err != nil || r.status().State != "active" {
				t.Fatal("phased startup rejected")
			}
			r.ports.runtime.storeLink.afterCommit = func(_ context.Context, v domain.Run) (domain.Run, error) { return v, nil }
			close(x.f.backend.release)
			r.step(context.Background())
			waitXApplicationCapture(t, r)
			x.count(t, 1)
		})
	}
}

func TestXApplicationRouteFailedBrokerStartupDisposesDespiteFirstCause(t *testing.T) {
	x := newNativeStoreFixture(t, true, func(f *manualRuntimeFixture) {
		f.broker.ports.prepare = func(context.Context, nativePilotPrepareRequest) (nativePilotPreparedRole, error) {
			return nativePilotPreparedRole{}, errors.New("private prepare failure")
		}
	})
	var disposed atomic.Int32
	r, err := x.e.startXManualApplication(context.Background(), context.Background(), x.session.ID, x.command.RunID, x.command.ID, func(context.Context, context.Context) (xManualApplicationPorts, error) {
		return xManualApplicationPorts{runtime: x.f.runtime, admission: nativePilotNativeStoreAdmission{broker: x.f.broker, store: x.s}, prepare: applicationPrepared, dispose: func(context.Context) error { disposed.Add(1); return nil }}, nil
	})
	if err == nil || r == nil || disposed.Load() != 1 {
		t.Fatal("rejected startup leaked full owner disposal")
	}
	if v := r.status(); v.Reason != "prepare-failed" || v.Cleanup != "verified" {
		t.Fatalf("startup cause/disposal %+v", v)
	}
}

func TestXApplicationRouteHeldCaptureDoesNotBlockOrdinaryPump(t *testing.T) {
	x := newNativeStoreFixture(t, true, nil)
	r := startApplicationFixture(t, x, context.Background(), applicationPrepared, func(context.Context) error { return nil })
	returned := make(chan struct{})
	go func() { x.e.stepXManual(context.Background()); close(returned) }()
	select {
	case <-x.f.backend.entered:
	case <-time.After(time.Second):
		t.Fatal("private capture did not start")
	}
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("private X capture blocked the ordinary pump")
	}
	unlock, allowed := x.e.ordinarySourceRoute(domain.SourceLinkedIn)
	unlock()
	if !allowed {
		t.Fatal("other source dispatch blocked")
	}
	if err := x.e.StopXManual(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitXApplicationCapture(t, r)
}

func TestXApplicationRouteAutonomousStopWaitsForResultReconciliation(t *testing.T) {
	x := newNativeStoreFixture(t, true, nil)
	r := startApplicationFixture(t, x, context.Background(), applicationPrepared, func(context.Context) error { return nil })
	entered, release := make(chan struct{}), make(chan struct{})
	r.ports.runtime.storeLink.afterCommit = func(_ context.Context, run domain.Run) (domain.Run, error) {
		close(entered)
		<-release
		return run, nil
	}
	close(x.f.backend.release)
	r.step(context.Background())
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("result reconciliation not entered")
	}
	x.f.broker.stop("attempt-expired")
	if v := r.status(); v.Cleanup != "pending" || v.Reason != "attempt-expired" {
		t.Fatalf("autonomous cleanup %+v", v)
	}
	unlock, allowed := x.e.ordinarySourceRoute(domain.SourceX)
	unlock()
	if allowed {
		t.Fatal("source released before reconciliation")
	}
	close(release)
	waitXApplicationCapture(t, r)
	if v := r.status(); v.Cleanup != "verified" {
		t.Fatalf("settled cleanup %+v", v)
	}
	x.count(t, 1)
}
