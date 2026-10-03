package collection

import (
	"context"
	"errors"
	"github.com/abangkis/AkuSidecar/internal/appshell"
	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"sync"
	"testing"
)

type testProcess struct {
	driver string
	done   chan error
	once   sync.Once
	closed bool
}

func proc(driver string) *testProcess                             { return &testProcess{driver: driver, done: make(chan error, 1)} }
func (p *testProcess) Driver() string                             { return p.driver }
func (p *testProcess) Done() <-chan error                         { return p.done }
func (p *testProcess) PID() int                                   { return 1 }
func (p *testProcess) Terminate()                                 { p.once.Do(func() { p.closed = true; p.done <- nil }) }
func (p *testProcess) CloseForRetry(context.Context) error        { p.Terminate(); return nil }
func (p *testProcess) OpenExtensionsPage(context.Context) error   { return nil }
func (p *testProcess) ReplacementReadiness(context.Context) error { return nil }
func TestSwitchWaitsForLeaseAndPinsDriver(t *testing.T) {
	p := proc("browser")
	m, _ := captureruntime.New(p)
	defer m.Terminate()
	c := NewCoordinator(m, func(_ context.Context, mode string, _ uint64) (captureruntime.Process, error) { return proc(mode), nil }, func() error { return nil })
	lease, _ := m.Acquire()
	c.Request("headless")
	c.reconcile(context.Background())
	if p.closed || !c.Status().Pending || lease.Driver() != "browser" {
		t.Fatal("live browser owner was replaced")
	}
	lease.Release()
	c.reconcile(context.Background())
	s := c.Status()
	if s.Effective != "headless" || s.Pending || s.Generation != 2 {
		t.Fatalf("status=%+v", s)
	}
	c.Request("browser")
	c.reconcile(context.Background())
	if c.Status().Effective != "browser" {
		t.Fatal("reverse handoff failed")
	}
}
func TestAssetFailurePreservesHealthyBrowser(t *testing.T) {
	p := proc("browser")
	m, _ := captureruntime.New(p)
	defer m.Terminate()
	launches := 0
	c := NewCoordinator(m, func(context.Context, string, uint64) (captureruntime.Process, error) { launches++; return nil, nil }, func() error { return errors.New("missing assets") })
	c.Request("headless")
	c.reconcile(context.Background())
	if launches != 0 || p.closed || c.Status().Effective != "browser" || c.Status().Failure == "" {
		t.Fatal("preflight released a healthy owner")
	}
}
func TestBrowserBorrowReleaseIsIdempotent(t *testing.T) {
	m, _ := captureruntime.New(proc("browser"))
	defer m.Terminate()
	c := NewCoordinator(m, nil, func() error { return nil })
	c.Request("headless")
	lease, release, err := c.BorrowBrowser(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if c.interactive != 1 || m.Snapshot().ActiveLeases != 1 {
		t.Fatal("borrow is unpinned")
	}
	lease.Release()
	release()
	release()
	if c.interactive != 0 {
		t.Fatal("double release corrupted intent")
	}
}
func TestUnsupportedSourcesRejectedBeforeSwitch(t *testing.T) {
	m, _ := captureruntime.New(proc("browser"))
	defer m.Terminate()
	c := NewCoordinator(m, nil, func() error { return nil })
	if c.ValidateSelection("headless", []domain.Source{"x", "unsupported"}) == nil {
		t.Fatal("unsupported source admitted")
	}
	if c.ValidateSelection("headless", []domain.Source{"x", "facebook", "instagram", "linkedin"}) != nil {
		t.Fatal("supported sources refused")
	}
	if got := c.Status().SupportedSources; len(got) != 4 {
		t.Fatalf("Settings runtime capability missing implemented collectors: %v", got)
	}
}

type retiringTestProcess struct {
	*testProcess
	retiring, naturallyExited bool
}

func (p *retiringTestProcess) Retiring() bool { return p.retiring }
func (p *retiringTestProcess) CloseForRetry(context.Context) error {
	p.retiring = true
	if !p.naturallyExited {
		return appshell.ErrCaptureHandoffPending
	}
	p.Terminate()
	return nil
}

func TestRetainedPopupAutomaticallyResumesHeadlessAfterNaturalExit(t *testing.T) {
	p := &retiringTestProcess{testProcess: proc("browser")}
	m, err := captureruntime.New(p)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Terminate()
	launches, readinessChecks := 0, 0
	c := NewCoordinator(m, func(_ context.Context, mode string, _ uint64) (captureruntime.Process, error) {
		launches++
		return proc(mode), nil
	}, func() error { return nil })
	c.SetHeadlessReadiness(func() error {
		readinessChecks++
		if p.retiring {
			return errors.New("retired host heartbeat disappeared")
		}
		return nil
	})
	c.Request("headless")
	c.reconcile(context.Background())
	if s := c.Status(); s.State != captureruntime.Blocked || !s.Pending || s.Generation != 1 || launches != 0 || p.closed {
		t.Fatalf("popup was not retained: %+v launches=%d closed=%t", s, launches, p.closed)
	}
	// An ordinary tick retries retained retirement without new user intent.
	c.reconcile(context.Background())
	if launches != 0 || p.closed || readinessChecks != 1 {
		t.Fatal("pending retirement closed popup or required vanished host heartbeat")
	}
	p.naturallyExited = true
	c.reconcile(context.Background())
	if s := c.Status(); s.Effective != "headless" || s.Pending || s.Generation != 2 || launches != 1 {
		t.Fatalf("automatic return did not complete: %+v launches=%d", s, launches)
	}
}
