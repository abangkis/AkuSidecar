package collection

import (
	"context"
	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"testing"
)

func TestPersistentMainReportsHeadlessConflictAndPreservesReader(t *testing.T) {
	p := proc("browser")
	m, _ := captureruntime.New(p)
	defer m.Terminate()
	if err := m.RetainForProcess(); err != nil {
		t.Fatal(err)
	}
	launches := 0
	c := NewCoordinator(m, func(context.Context, string, uint64) (captureruntime.Process, error) {
		launches++
		return proc("headless"), nil
	}, func() error { return nil })
	if c.ValidateSelection("headless", []domain.Source{domain.SourceFacebook}) == nil {
		t.Fatal("headless selection accepted")
	}
	if c.ValidateSelection("headless", []domain.Source{domain.SourceX}) != nil {
		t.Fatal("independent X collector confused with main replacement")
	}
	c.Request("headless")
	c.reconcile(context.Background())
	s := c.Status()
	if launches != 0 || p.closed || !s.PersistentMain || s.Effective != "browser" || s.Failure == "" {
		t.Fatalf("unexpected persistent state: %+v", s)
	}
	c.Request("browser")
	lease, done, err := c.BorrowNativeReader(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	lease.Release()
	done()
	c.reconcile(context.Background())
	if p.closed || m.Snapshot().Generation != 1 {
		t.Fatal("reader intent replaced main owner")
	}
}
