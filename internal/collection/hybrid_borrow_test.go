package collection

import (
	"context"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestFacebookCollectionBorrowDoesNotReleaseInteractiveOwner(t *testing.T) {
	m, _ := captureruntime.New(proc("browser"))
	defer m.Terminate()
	c := NewCoordinator(m, func(_ context.Context, mode string, _ uint64) (captureruntime.Process, error) { return proc(mode), nil }, func() error { return nil })
	c.Request("headless")
	reader, releaseReader, err := c.BorrowBrowser(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	batch, releaseBatch, err := c.BorrowBrowserCollection(context.Background(), domain.SourceFacebook)
	if err != nil {
		t.Fatal(err)
	}
	if c.Status().CollectionBorrowSource != domain.SourceFacebook || c.interactive != 1 {
		t.Fatal("collection and interaction were not distinguished")
	}
	batch.Release()
	releaseBatch()
	releaseBatch()
	c.reconcile(context.Background())
	if c.Status().Effective != "browser" || c.interactive != 1 || c.browserCollection != 0 {
		t.Fatal("batch release retired live reader")
	}
	reader.Release()
	releaseReader()
	c.reconcile(context.Background())
	if c.Status().Effective != "headless" || c.Status().CollectionBorrowSource != "" {
		t.Fatal("drained borrows did not restore requested mode")
	}
}

func TestBrowserCollectionBorrowRejectsOtherSourcesWithoutTakingOwnership(t *testing.T) {
	m, _ := captureruntime.New(proc("browser"))
	defer m.Terminate()
	c := NewCoordinator(m, nil, func() error { return nil })
	if _, _, err := c.BorrowBrowserCollection(context.Background(), domain.SourceX); err == nil {
		t.Fatal("non-exception source borrowed Browser collection")
	}
	if m.Snapshot().ActiveLeases != 0 || c.browserCollection != 0 || c.interactive != 0 {
		t.Fatal("rejected borrow leaked ownership")
	}
}

func TestHybridPreferredDriverKeepsFacebookOnBrowser(t *testing.T) {
	for _, source := range []domain.Source{domain.SourceX, domain.SourceInstagram, domain.SourceLinkedIn, domain.SourceFacebook} {
		want := "headless"
		if source == domain.SourceFacebook {
			want = "browser"
		}
		got, err := PreferredDriver("headless", source)
		if err != nil || got != want {
			t.Fatalf("source=%s driver=%s err=%v", source, got, err)
		}
		got, err = PreferredDriver("browser", source)
		if err != nil || got != "browser" {
			t.Fatal("Browser rollback did not cover every source")
		}
	}
	if _, err := PreferredDriver("headless", "unknown"); err == nil {
		t.Fatal("unknown source admitted")
	}
}

func TestFacebookCollectionIntentWaitsForPriorLeaseDrain(t *testing.T) {
	m, _ := captureruntime.New(proc("headless"))
	defer m.Terminate()
	c := NewCoordinator(m, func(_ context.Context, mode string, _ uint64) (captureruntime.Process, error) { return proc(mode), nil }, func() error { return nil })
	c.Request("headless")
	old, _ := m.Acquire()
	release, err := c.BeginBrowserCollection(domain.SourceFacebook)
	if err != nil {
		t.Fatal(err)
	}
	c.reconcile(context.Background())
	if c.Status().Effective != "headless" {
		t.Fatal("intent retired live headless work")
	}
	old.Release()
	c.reconcile(context.Background())
	if c.Status().Effective != "browser" {
		t.Fatal("drained batch did not borrow Browser")
	}
	release()
	release()
	c.reconcile(context.Background())
	if c.Status().Effective != "headless" {
		t.Fatal("released collection intent did not return headless")
	}
}
