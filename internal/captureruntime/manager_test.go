package captureruntime

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeProcess struct {
	done          chan error
	terminated    chan struct{}
	terminateOnce sync.Once
	exitOnce      sync.Once
	cleanupGate   <-chan struct{}
	cleanupErr    error
	readinessErr  error
	pid           int
}

func newProcess(pid int) *fakeProcess {
	return &fakeProcess{done: make(chan error, 1), terminated: make(chan struct{}), pid: pid}
}
func (p *fakeProcess) Done() <-chan error                         { return p.done }
func (p *fakeProcess) PID() int                                   { return p.pid }
func (p *fakeProcess) OpenExtensionsPage(context.Context) error   { return nil }
func (p *fakeProcess) ReplacementReadiness(context.Context) error { return p.readinessErr }
func (p *fakeProcess) exit(err error)                             { p.exitOnce.Do(func() { p.done <- err }) }
func (p *fakeProcess) Terminate() {
	p.terminateOnce.Do(func() { close(p.terminated); p.exit(nil) })
}
func (p *fakeProcess) CloseForRetry(ctx context.Context) error {
	p.Terminate() // Root exits even if an owned child is still draining.
	if p.cleanupGate != nil {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-p.cleanupGate:
		}
	}
	return p.cleanupErr
}

func newManager(t *testing.T, process *fakeProcess) *Manager {
	t.Helper()
	m, err := New(process)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Terminate)
	return m
}

func await(t *testing.T, result <-chan error) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal("runtime event timed out")
		return nil
	}
}

func TestReplacementPreservesManagerLifetimeAndFencesGeneration(t *testing.T) {
	m := newManager(t, newProcess(10))
	oldGeneration := m.Snapshot().Generation
	next := newProcess(20)
	err := m.Replace(context.Background(), func(_ context.Context, generation uint64) (Process, error) {
		if generation <= oldGeneration {
			t.Errorf("generation did not advance: %d", generation)
		}
		return next, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if m.PID() != 20 || m.Snapshot().State != Ready {
		t.Fatalf("replacement not ready: %+v", m.Snapshot())
	}
	if m.Accepts(oldGeneration) || !m.Accepts(m.Snapshot().Generation) {
		t.Fatal("generation fence failed")
	}
	select {
	case err := <-m.Done():
		t.Fatalf("intentional replacement ended manager: %v", err)
	default:
	}
	if err := m.OpenExtensionsPage(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestEveryLeaseMustDrainBeforeReplacement(t *testing.T) {
	old := newProcess(10)
	m := newManager(t, old)
	first, err := m.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	second, err := m.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	if first.Generation() != m.Snapshot().Generation {
		t.Fatal("lease did not pin generation")
	}
	launch := func(context.Context, uint64) (Process, error) { return newProcess(20), nil }
	if err := m.Replace(context.Background(), launch); !errors.Is(err, ErrBusy) {
		t.Fatalf("replace with leases: %v", err)
	}
	select {
	case <-old.terminated:
		t.Fatal("busy replacement terminated the owner")
	default:
	}
	first.Release()
	first.Release() // Double release must not free the second lease.
	if m.Snapshot().ActiveLeases != 1 {
		t.Fatal("lease release was not idempotent")
	}
	if err := m.Replace(context.Background(), launch); !errors.Is(err, ErrBusy) {
		t.Fatalf("second lease ignored: %v", err)
	}
	second.Release()
	if err := m.Replace(context.Background(), launch); err != nil {
		t.Fatal(err)
	}
}

func TestRootExitDoesNotReleaseProfileBeforeOwnedTreeCleanup(t *testing.T) {
	gate := make(chan struct{})
	old := newProcess(10)
	old.cleanupGate = gate
	m := newManager(t, old)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // Unblocks cleanup if an assertion fails.
	var launches atomic.Int32
	result := make(chan error, 1)
	go func() {
		result <- m.Replace(ctx, func(context.Context, uint64) (Process, error) {
			launches.Add(1)
			return newProcess(20), nil
		})
	}()
	select {
	case <-old.terminated:
	case <-time.After(2 * time.Second):
		t.Fatal("old owner was not stopped")
	}
	if launches.Load() != 0 {
		t.Fatal("launched while owned tree still existed")
	}
	if _, err := m.Acquire(); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("admitted during transition: %v", err)
	}
	if m.Accepts(1) {
		t.Fatal("accepted stale results during transition")
	}
	close(gate)
	if err := await(t, result); err != nil {
		t.Fatal(err)
	}
}

func TestCleanupFailureBlocksNewOwner(t *testing.T) {
	cleanupErr := errors.New("owned child still alive")
	old := newProcess(10)
	old.cleanupErr = cleanupErr
	m := newManager(t, old)
	err := m.Replace(context.Background(), func(context.Context, uint64) (Process, error) {
		t.Fatal("launcher called without verified cleanup")
		return nil, nil
	})
	if !errors.Is(err, cleanupErr) {
		t.Fatalf("cleanup error lost: %v", err)
	}
	if m.Snapshot().State != Blocked || m.Snapshot().Generation != 1 {
		t.Fatalf("incorrect blocked state: %+v", m.Snapshot())
	}
	if _, err := m.Acquire(); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("blocked runtime admitted work: %v", err)
	}
}

func TestFailedLaunchDoesNotSilentlyFallbackOrRetryUnverifiedOwner(t *testing.T) {
	m := newManager(t, newProcess(10))
	failure := errors.New("Chrome startup failed")
	err := m.Replace(context.Background(), func(context.Context, uint64) (Process, error) { return nil, failure })
	if !errors.Is(err, failure) {
		t.Fatalf("launch error lost: %v", err)
	}
	if m.PID() != 0 || m.Snapshot().State != Blocked {
		t.Fatal("failed launch incorrectly reported ready")
	}
	err = m.Replace(context.Background(), func(context.Context, uint64) (Process, error) {
		t.Fatal("unverified failed launch was retried")
		return nil, nil
	})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("retry not blocked: %v", err)
	}
	select {
	case <-m.Done():
		t.Fatal("failed intentional replacement shut down UI")
	default:
	}
}

func TestUnexpectedExitRetainsExistingShutdownSignal(t *testing.T) {
	process := newProcess(10)
	m := newManager(t, process)
	failure := errors.New("unexpected exit")
	process.exit(failure)
	if err := await(t, m.Done()); !errors.Is(err, failure) {
		t.Fatalf("unexpected exit lost: %v", err)
	}
	if m.Snapshot().State != Failed || m.Accepts(1) {
		t.Fatal("exited runtime still ready")
	}
	if _, err := m.Acquire(); !errors.Is(err, ErrStopped) {
		t.Fatalf("exited runtime admitted work: %v", err)
	}
}

func TestCancelledReplacementDoesNotTerminateCurrentOwner(t *testing.T) {
	process := newProcess(10)
	m := newManager(t, process)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := m.Replace(ctx, func(context.Context, uint64) (Process, error) { return newProcess(20), nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation ignored: %v", err)
	}
	if m.PID() != 10 || m.Snapshot().State != Ready {
		t.Fatal("cancelled request changed owner")
	}
	select {
	case <-process.terminated:
		t.Fatal("cancelled request terminated owner")
	default:
	}
}

func TestCancellationDuringCleanupBlocksLaunchUntilOwnershipIsVerified(t *testing.T) {
	gate := make(chan struct{})
	old := newProcess(10)
	old.cleanupGate = gate
	m := newManager(t, old)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		result <- m.Replace(ctx, func(context.Context, uint64) (Process, error) {
			t.Error("launched during cancelled cleanup")
			return newProcess(20), nil
		})
	}()
	select {
	case <-old.terminated:
	case <-time.After(2 * time.Second):
		t.Fatal("cleanup did not start")
	}
	cancel()
	if err := await(t, result); !errors.Is(err, context.Canceled) {
		t.Fatalf("cleanup cancellation lost: %v", err)
	}
	if m.Snapshot().State != Blocked || m.PID() != 10 {
		t.Fatal("unverified owner was forgotten")
	}
	close(gate)
	if err := m.Replace(context.Background(), func(context.Context, uint64) (Process, error) { return newProcess(20), nil }); err != nil {
		t.Fatal(err)
	}
}

func TestCancellationDuringLaunchNeverPublishesReady(t *testing.T) {
	m := newManager(t, newProcess(10))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	next := newProcess(20)
	err := m.Replace(ctx, func(context.Context, uint64) (Process, error) {
		cancel()
		return next, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("launch cancellation lost: %v", err)
	}
	if m.Snapshot().State != Blocked || m.Accepts(2) {
		t.Fatal("cancelled launch incorrectly published ready")
	}
	m.Terminate()
	select {
	case <-next.terminated:
	default:
		t.Fatal("cancelled launch owner was not retained for shutdown")
	}
}

func TestShutdownPreventsFurtherLaunchesEvenWithLeases(t *testing.T) {
	m := newManager(t, newProcess(10))
	lease, err := m.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	m.Terminate()
	m.Terminate()
	lease.Release()
	if m.Snapshot().State != Stopped || m.Snapshot().ActiveLeases != 0 {
		t.Fatal("shutdown state incorrect")
	}
	if _, err := m.Acquire(); !errors.Is(err, ErrStopped) {
		t.Fatalf("shutdown admitted work: %v", err)
	}
	if err := m.Replace(context.Background(), func(context.Context, uint64) (Process, error) { return newProcess(20), nil }); !errors.Is(err, ErrStopped) {
		t.Fatalf("shutdown allowed launch: %v", err)
	}
}

func TestAbsentProcessIsRejectedAndNilManagerShutdownIsSafe(t *testing.T) {
	if _, err := New(nil); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("nil process accepted: %v", err)
	}
	var m *Manager
	if m.Done() != nil || m.PID() != 0 {
		t.Fatal("nil manager should disable app-shell select")
	}
	m.Terminate()
}

func TestReaderReadinessRefusesReplacementWithoutTerminatingOwner(t *testing.T) {
	old := newProcess(10)
	old.readinessErr = ErrBusy
	m := newManager(t, old)
	if err := m.Replace(context.Background(), func(context.Context, uint64) (Process, error) { t.Fatal("reader owner replaced"); return nil, nil }); !errors.Is(err, ErrBusy) {
		t.Fatalf("reader readiness=%v", err)
	}
	if m.Snapshot().State != Ready || !m.Accepts(1) {
		t.Fatal("reader guard changed healthy owner")
	}
	select {
	case <-old.terminated:
		t.Fatal("reader guard terminated owner")
	default:
	}
}
