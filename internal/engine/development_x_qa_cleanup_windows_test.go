//go:build windows && auth_session_dev && auth_session_fixture

package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/readerbroker"
)

type xQACleanupWatch struct {
	mu                   sync.Mutex
	aborts               int
	disposals            int
	abortSawParentDone   bool
	disposeSawParentDone bool
	failAbort            bool
	failDispose          bool
	originalDispose      func(context.Context) error
}

func xQAWatchCleanup(q *XApplicationQAPilot, failAbort, failDispose bool) *xQACleanupWatch {
	watch := &xQACleanupWatch{failAbort: failAbort, failDispose: failDispose}
	originalFactory := q.kit.factory
	q.kit.factory = func(e *Engine, selection *xQASelection) xManualApplicationFactory {
		inner := originalFactory(e, selection)
		return func(ctx, parent context.Context) (xManualApplicationPorts, error) {
			ports, err := inner(ctx, parent)
			watch.mu.Lock()
			watch.originalDispose = ports.dispose
			watch.mu.Unlock()
			if broker := ports.startupBroker(); broker != nil {
				abort := broker.ports.abort
				broker.ports.abort = func(call context.Context, prepared nativePilotPreparedRole, preserve bool) error {
					watch.mu.Lock()
					watch.aborts++
					watch.abortSawParentDone = watch.abortSawParentDone || q.parent.Err() != nil
					fail := watch.failAbort
					watch.mu.Unlock()
					abortErr := abort(call, prepared, preserve)
					if fail {
						return errors.New("synthetic broker abort failure")
					}
					return abortErr
				}
			}
			dispose := ports.dispose
			ports.dispose = func(call context.Context) error {
				watch.mu.Lock()
				watch.disposals++
				watch.disposeSawParentDone = watch.disposeSawParentDone || q.parent.Err() != nil
				fail := watch.failDispose
				watch.mu.Unlock()
				if fail {
					return errors.New("synthetic owner disposal failure")
				}
				return dispose(call)
			}
			return ports, err
		}
	}
	return watch
}

func xQASetReaderFailure(q *XApplicationQAPilot) error {
	q.kit.foreground = func(context.Context, readerbroker.Target) error {
		return errors.New("synthetic foreground failure")
	}
	return q.OpenPost(context.Background(), readerbroker.Request{RequestID: "broker_0123456789abcdef0123456789abcdef", Source: "x", URL: "https://x.com/aku_fixture/status/1"})
}

func xQAOpenRetainedCleanupPilot(t *testing.T, parent context.Context, port int) (*XApplicationQAPilot, string, string) {
	t.Helper()
	node := os.Getenv("AKU_CHROME_FIXTURE_NODE")
	if node == "" {
		t.Skip("canonical QA helper required")
	}
	path, err := WriteXApplicationQASyntheticRetainedPlan(t.TempDir(), node, port)
	if err != nil {
		t.Fatal(err)
	}
	q, err := OpenXApplicationQAPilot(parent, XApplicationQAOptions{Dev: true, PlanPath: path, Synthetic: true})
	if err != nil {
		t.Fatal(err)
	}
	return q, path, filepath.Join(q.selection.plan.Root, "retained-fixture")
}

func xQACleanupWatchSnapshot(watch *xQACleanupWatch) (aborts, disposals int, abortParentDone, disposeParentDone bool, originalDispose func(context.Context) error) {
	watch.mu.Lock()
	defer watch.mu.Unlock()
	return watch.aborts, watch.disposals, watch.abortSawParentDone, watch.disposeSawParentDone, watch.originalDispose
}

func xQACleanupUncertainTestTeardown(t *testing.T, q *XApplicationQAPilot, originalDispose func(context.Context) error) {
	t.Helper()
	const drainTimeout = 5 * time.Second
	if q.e != nil {
		q.e.Shutdown()
	}
	drained := true
	if q.runtime != nil {
		q.runtime.mu.Lock()
		captureStarted, captureDone := q.runtime.captureStarted, q.runtime.captureDone
		q.runtime.mu.Unlock()
		if captureStarted {
			select {
			case <-captureDone:
			case <-time.After(drainTimeout):
				t.Errorf("synthetic capture did not drain within %s", drainTimeout)
				drained = false
			}
		}
	}
	if q.e != nil && !q.e.WaitForIdle(drainTimeout) {
		t.Errorf("engine work did not become idle within %s", drainTimeout)
		drained = false
	}
	if !drained {
		return
	}
	if originalDispose != nil {
		if err := originalDispose(context.Background()); err != nil {
			t.Errorf("synthetic owner teardown failed: %v", err)
		}
	}
	if q.kit != nil && q.kit.close != nil {
		q.kit.close()
	}
	if q.state != nil {
		if err := q.state.Close(); err != nil {
			t.Errorf("synthetic QA state close failed: %v", err)
		}
	}
	if q.selection != nil {
		q.selection.close()
	}
}

func TestXQACloseDrainsBeforeCancelAndKeepsFirstReaderFailure(t *testing.T) {
	path := xQAPlanTest(t)
	q, err := OpenXApplicationQAPilot(context.Background(), XApplicationQAOptions{Dev: true, PlanPath: path, Synthetic: true})
	if err != nil {
		t.Fatal(err)
	}
	defer q.Close()
	watch := xQAWatchCleanup(q, false, false)
	if err = q.Start(context.Background(), true, false); err != nil {
		t.Fatal(err)
	}
	if err = xQASetReaderFailure(q); err == nil {
		t.Fatal("synthetic foreground failure was accepted")
	}
	first := q.Report().Evidence.FirstFailure
	if first == nil || first.Stage != "reader_foreground" {
		t.Fatal("reader failure was not recorded before shutdown")
	}
	if err = q.Close(); err != nil {
		t.Fatal(err)
	}
	aborts, disposals, abortParentDone, disposeParentDone, _ := xQACleanupWatchSnapshot(watch)
	if aborts == 0 || disposals != 1 || abortParentDone || disposeParentDone {
		t.Fatalf("shutdown callback order aborts=%d disposals=%d abortParentDone=%t disposeParentDone=%t", aborts, disposals, abortParentDone, disposeParentDone)
	}
	if q.parent.Err() == nil {
		t.Fatal("Close returned before canceling the pilot parent")
	}
	final := q.Report().Evidence
	if final.FirstFailure == nil || *final.FirstFailure != *first || final.Cleanup != "verified" {
		t.Fatalf("shutdown replaced reader failure or lost cleanup proof: %+v", final)
	}
}

func TestXQACloseAbortFailureKeepsSelectorAndFirstFailure(t *testing.T) {
	path := xQAPlanTest(t)
	q, err := OpenXApplicationQAPilot(context.Background(), XApplicationQAOptions{Dev: true, PlanPath: path, Synthetic: true})
	if err != nil {
		t.Fatal(err)
	}
	watch := xQAWatchCleanup(q, true, false)
	defer func() {
		_, _, _, _, dispose := xQACleanupWatchSnapshot(watch)
		xQACleanupUncertainTestTeardown(t, q, dispose)
	}()
	if err = q.Start(context.Background(), true, false); err != nil {
		t.Fatal(err)
	}
	if err = xQASetReaderFailure(q); err == nil {
		t.Fatal("synthetic foreground failure was accepted")
	}
	first := q.Report().Evidence.FirstFailure
	if first == nil {
		t.Fatal("first reader failure missing")
	}
	if err = q.Close(); err == nil {
		t.Fatal("broker cleanup failure was reported as verified")
	}
	aborts, disposals, abortParentDone, disposeParentDone, _ := xQACleanupWatchSnapshot(watch)
	if aborts == 0 || disposals != 1 || abortParentDone || disposeParentDone {
		t.Fatalf("uncertain shutdown callbacks aborts=%d disposals=%d abortParentDone=%t disposeParentDone=%t", aborts, disposals, abortParentDone, disposeParentDone)
	}
	report := q.Report()
	if report.Status.Cleanup != "uncertain" || report.Evidence.Cleanup != "uncertain" || report.Evidence.FirstFailure == nil || *report.Evidence.FirstFailure != *first {
		t.Fatalf("broker failure replaced cause or unlocked cleanup state: %+v", report)
	}
	if other, openErr := OpenXApplicationQAPilot(context.Background(), XApplicationQAOptions{Dev: true, PlanPath: path, Synthetic: true}); openErr == nil {
		_ = other.Close()
		t.Fatal("uncertain cleanup released the isolated-instance selector")
	}
}

func TestXQAExternalParentCancellationDoesNotFabricateRetainedRelease(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	defer cancelParent()
	q, path, retainedParent := xQAOpenRetainedCleanupPilot(t, parent, 28176)
	watch := xQAWatchCleanup(q, false, true)
	defer func() {
		_, _, _, _, dispose := xQACleanupWatchSnapshot(watch)
		xQACleanupUncertainTestTeardown(t, q, dispose)
	}()
	if err := q.Start(context.Background(), true, false); err != nil {
		t.Fatal(err)
	}
	if err := xQASetReaderFailure(q); err == nil {
		t.Fatal("synthetic foreground failure was accepted")
	}
	first := q.Report().Evidence.FirstFailure
	if first == nil || first.Stage != "reader_foreground" {
		t.Fatal("reader failure was not recorded before external cancellation")
	}
	cancelParent()
	deadline := time.Now().Add(3 * time.Second)
	stoppedByExternalCancellation := false
	for time.Now().Before(deadline) {
		status := q.e.XManualStatus()
		if status.State == "failed" && status.Cleanup == "uncertain" {
			stoppedByExternalCancellation = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if q.parent.Err() == nil {
		t.Fatal("external cancellation did not reach the pilot parent")
	}
	if !stoppedByExternalCancellation {
		t.Fatal("external cancellation did not complete the uncertain stop path")
	}
	if err := q.Close(); err == nil {
		t.Fatal("external cancellation plus disposal failure was reported clean")
	}
	aborts, disposals, abortParentDone, disposeParentDone, _ := xQACleanupWatchSnapshot(watch)
	if aborts == 0 || disposals != 1 || !abortParentDone || !disposeParentDone {
		t.Fatalf("external cancellation callbacks aborts=%d disposals=%d abortParentDone=%t disposeParentDone=%t", aborts, disposals, abortParentDone, disposeParentDone)
	}
	status := q.e.XManualStatus()
	if status.State != "failed" || status.Cleanup != "uncertain" {
		t.Fatalf("external cancellation did not fail closed: %+v", status)
	}
	manifest, err := xRetainedReadManifest(filepath.Join(retainedParent, xRetainedQAProfileManifestName))
	if err != nil || manifest.State != "active" {
		t.Fatalf("failed physical disposal fabricated a clean retained lease: manifest=%+v err=%v", manifest, err)
	}
	if competing, leaseErr := acquireXRetainedProfileAtParent(retainedParent, false); leaseErr == nil {
		_ = competing.Release(false)
		t.Fatal("failed physical disposal released the retained reader lease")
	}
	if other, openErr := OpenXApplicationQAPilot(context.Background(), XApplicationQAOptions{Dev: true, PlanPath: path, Synthetic: true}); openErr == nil {
		_ = other.Close()
		t.Fatal("external cancellation released the isolated-instance selector")
	}
	final := q.Report().Evidence
	if final.Cleanup != "uncertain" || final.FirstFailure == nil || *final.FirstFailure != *first {
		t.Fatalf("external cancellation fabricated cleanup proof or replaced the first cause: %+v", final)
	}
}
