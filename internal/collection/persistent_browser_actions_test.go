package collection

import (
	"context"
	"errors"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
)

func TestPersistentBrowserActionsBindExactOwnerAndKeepIntentLifecycle(t *testing.T) {
	owner, err := captureruntime.New(proc("browser"))
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Terminate()
	other, err := captureruntime.New(proc("browser"))
	if err != nil {
		t.Fatal(err)
	}
	defer other.Terminate()
	for _, manager := range []*captureruntime.Manager{owner, other} {
		if err := manager.RetainForProcess(); err != nil {
			t.Fatal(err)
		}
	}
	c := NewCoordinator(owner, nil, func() error { return nil })
	if owner.Snapshot().Generation != other.Snapshot().Generation {
		t.Fatal("fixture must test indistinguishable generation numbers")
	}
	if lease, _, err := c.BorrowPersistentBrowser(context.Background(), other, true); lease != nil || err == nil {
		t.Fatal("another manager's matching generation was accepted")
	}
	for _, native := range []bool{false, true} {
		lease, release, err := c.BorrowPersistentBrowser(context.Background(), owner, native)
		if err != nil || lease == nil {
			t.Fatalf("native=%t: %v", native, err)
		}
		if owner.Snapshot().ActiveLeases != 1 {
			t.Fatal("ordinary Browser lease was not retained")
		}
		lease.Release()
		release()
		release()
		c.mu.Lock()
		intents := c.interactive + c.nativeReaders
		c.mu.Unlock()
		if owner.Snapshot().ActiveLeases != 0 || intents != 0 {
			t.Fatal("ordinary action leaked or double-released its intent")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if lease, _, err := c.BorrowPersistentBrowser(ctx, owner, true); lease != nil || !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled action acquired an owner")
	}
	owner.Terminate()
	if lease, _, err := c.BorrowPersistentBrowser(context.Background(), owner, true); lease != nil || err == nil {
		t.Fatal("stopped owner accepted an action")
	}
}
