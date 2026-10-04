package collection

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
)

type nativeReaderProcessFixture struct {
	*testProcess
	readerOpen bool
}

func (p *nativeReaderProcessFixture) ReplacementReadiness(context.Context) error {
	if p.readerOpen {
		return errors.New("native reader remains open")
	}
	return nil
}

func TestHeadlessNativeBorrowUsesReaderRoleAndReturnsAfterNaturalExit(t *testing.T) {
	m, _ := captureruntime.New(proc("headless"))
	defer m.Terminate()
	var modes []string
	var reader *nativeReaderProcessFixture
	c := NewCoordinator(m, func(_ context.Context, mode string, _ uint64) (captureruntime.Process, error) {
		modes = append(modes, mode)
		if mode == "native_reader" {
			reader = &nativeReaderProcessFixture{testProcess: proc("browser"), readerOpen: true}
			return reader, nil
		}
		return proc(mode), nil
	}, func() error { return nil })
	c.Request("headless")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	type result struct {
		lease   *captureruntime.Lease
		release func()
		err     error
	}
	done := make(chan result, 1)
	go func() { l, r, e := c.BorrowNativeReader(ctx); done <- result{l, r, e} }()
	for {
		c.mu.Lock()
		waiting := c.interactive == 1 && c.nativeReaders == 1
		c.mu.Unlock()
		if waiting {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("borrow not started")
		case <-time.After(time.Millisecond):
		}
	}
	c.reconcile(ctx)
	borrow := <-done
	if borrow.err != nil {
		t.Fatal(borrow.err)
	}
	if len(modes) != 1 || modes[0] != "native_reader" || !c.Status().NativeReaderOnly || borrow.lease.Driver() != "browser" {
		t.Fatal("wrong role", modes, c.Status())
	}
	borrow.lease.Release()
	borrow.release()
	borrow.release()
	c.reconcile(ctx)
	if reader.closed || c.Status().Effective != "browser" || len(modes) != 1 {
		t.Fatal("open reader lost profile ownership")
	}
	// A source/FB borrow cannot dispatch Bridge work into a hostless process.
	if _, _, err := c.BorrowBrowser(ctx); err == nil {
		t.Fatal("source borrow accepted reader-only owner")
	}
	reader.readerOpen = false
	reader.Terminate()
	for m.Snapshot().State != captureruntime.Failed {
		select {
		case <-ctx.Done():
			t.Fatal("natural exit not observed")
		case <-time.After(time.Millisecond):
		}
	}
	c.reconcile(ctx)
	if c.Status().Effective != "headless" || c.Status().NativeReaderOnly || c.Status().Pending || len(modes) != 2 || modes[1] != "headless" {
		t.Fatal("auto-return failed", modes, c.Status())
	}
}

func TestBrowserNativeBorrowKeepsExistingForegroundOwner(t *testing.T) {
	m, _ := captureruntime.New(proc("browser"))
	defer m.Terminate()
	launches := 0
	c := NewCoordinator(m, func(context.Context, string, uint64) (captureruntime.Process, error) {
		launches++
		return proc("browser"), nil
	}, func() error { return nil })
	l, r, err := c.BorrowNativeReader(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	c.reconcile(context.Background())
	l.Release()
	r()
	if launches != 0 || c.Status().NativeReaderOnly {
		t.Fatal("foreground path replaced")
	}
}

func TestNativeReaderIntentWakesCoordinatorBeforeFallbackTick(t *testing.T) {
	m, _ := captureruntime.New(proc("headless"))
	defer m.Terminate()
	launched := make(chan string, 1)
	c := NewCoordinator(m, func(_ context.Context, mode string, _ uint64) (captureruntime.Process, error) {
		launched <- mode
		return proc("browser"), nil
	}, func() error { return nil })
	c.Request("headless")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.Start(ctx)
	done := make(chan error, 1)
	go func() {
		lease, release, err := c.BorrowNativeReader(ctx)
		if err == nil {
			lease.Release()
			release()
		}
		done <- err
	}()
	select {
	case mode := <-launched:
		if mode != "native_reader" {
			t.Fatal(mode)
		}
	case <-time.After(750 * time.Millisecond):
		t.Fatal("reader intent waited for the one-second fallback timer")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("borrow did not finish")
	}
	cancel()
}
