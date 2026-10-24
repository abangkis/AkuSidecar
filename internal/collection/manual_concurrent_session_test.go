//go:build auth_session_dev

package collection

import (
	"context"
	"errors"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

type manualConcurrentProcess struct {
	driver string
	pid    int
	done   chan error
	closed bool
}

func (p *manualConcurrentProcess) Done() <-chan error { return p.done }
func (p *manualConcurrentProcess) Terminate() {
	if !p.closed {
		p.closed = true
		close(p.done)
	}
}
func (p *manualConcurrentProcess) CloseForRetry(context.Context) error      { p.Terminate(); return nil }
func (p *manualConcurrentProcess) PID() int                                 { return p.pid }
func (p *manualConcurrentProcess) Driver() string                           { return p.driver }
func (*manualConcurrentProcess) OpenExtensionsPage(context.Context) error   { return nil }
func (*manualConcurrentProcess) ReplacementReadiness(context.Context) error { return nil }

type manualConcurrentBackend struct{ calls int }

func (b *manualConcurrentBackend) Capture(context.Context, domain.Source, map[string]any) (domain.Observation, error) {
	b.calls++
	return domain.Observation{}, nil
}

func TestManualConcurrentBindingAndFixedOwner(t *testing.T) {
	collector, _ := captureruntime.New(&manualConcurrentProcess{"headless", 1001, make(chan error), false})
	defer collector.Terminate()
	reader, _ := captureruntime.New(&manualConcurrentProcess{"browser", 1002, make(chan error), false})
	defer reader.Terminate()
	launches := 0
	c := NewCoordinator(collector, func(context.Context, string, uint64) (captureruntime.Process, error) {
		launches++
		return nil, errors.New("forbidden")
	}, func() error { return nil })
	backend := &manualConcurrentBackend{}
	gate := func(context.Context) error { return nil }
	if c.BindManualConcurrentSession(reader, 2, backend, gate, gate) == nil || c.BindManualConcurrentSession(collector, 1, backend, gate, gate) == nil {
		t.Fatal("invalid fixed ownership accepted")
	}
	if err := c.BindManualConcurrentSession(reader, 1, backend, gate, gate); err != nil {
		t.Fatal(err)
	}
	if c.BindManualConcurrentSession(reader, 1, backend, gate, gate) == nil {
		t.Fatal("rebinding accepted")
	}
	if c.ValidateSelection("browser", []domain.Source{domain.SourceX}) == nil || c.ValidateSelection("headless", []domain.Source{domain.SourceFacebook}) == nil {
		t.Fatal("exclusive or non-X selection accepted")
	}
	if _, _, err := c.BorrowBrowser(context.Background()); err == nil {
		t.Fatal("exclusive borrow accepted")
	}
	lease, release, err := c.BorrowNativeReader(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	defer lease.Release()
	if lease.Driver() != "browser" || collector.Snapshot().ActiveLeases != 0 || reader.Snapshot().ActiveLeases != 1 {
		t.Fatal("reader lease used collector")
	}
	if _, err := c.Capture(context.Background(), domain.SourceFacebook, nil); err == nil {
		t.Fatal("non-X capture accepted")
	}
	if _, err := c.Capture(context.Background(), domain.SourceX, nil); err != nil || backend.calls != 1 {
		t.Fatal("bound backend unavailable")
	}
	c.Request("browser")
	c.Retry()
	c.reconcile(context.Background())
	if launches != 0 || collector.PID() != 1001 || collector.Snapshot().Generation != 1 {
		t.Fatal("fixed collector replaced")
	}
	collector.Terminate()
	if _, err := c.Capture(context.Background(), domain.SourceX, nil); err == nil || backend.calls != 1 {
		t.Fatal("stopped collector backend accepted")
	}
}

func TestManualConcurrentAdmissionRejection(t *testing.T) {
	collector, _ := captureruntime.New(&manualConcurrentProcess{"headless", 2001, make(chan error), false})
	defer collector.Terminate()
	reader, _ := captureruntime.New(&manualConcurrentProcess{"browser", 2002, make(chan error), false})
	defer reader.Terminate()
	c := NewCoordinator(collector, nil, func() error { return nil })
	backend := &manualConcurrentBackend{}
	deny := func(context.Context) error { return errors.New("manual closed") }
	if err := c.BindManualConcurrentSession(reader, 1, backend, deny, deny); err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.BorrowNativeReader(context.Background()); err == nil || reader.Snapshot().ActiveLeases != 0 {
		t.Fatal("closed reader acquired")
	}
	if _, err := c.Capture(context.Background(), domain.SourceX, nil); err == nil || backend.calls != 0 {
		t.Fatal("closed collector invoked")
	}
}
