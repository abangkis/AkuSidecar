package captureruntime

import (
	"context"
	"errors"
	"testing"
)

func TestPersistentOwnerRejectsReplacementWithoutLeakingWorkLease(t *testing.T) {
	p := newProcess(100)
	m, err := New(p)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Terminate()
	if err = m.RetainForProcess(); err != nil {
		t.Fatal(err)
	}
	called := false
	factory := func(context.Context, uint64) (Process, error) { called = true; return newProcess(101), nil }
	if !errors.Is(m.Replace(context.Background(), factory), ErrPersistent) || !errors.Is(m.Recover(context.Background(), factory), ErrPersistent) {
		t.Fatal("persistent replacement accepted")
	}
	if called || m.Snapshot().ActiveLeases != 0 || m.Snapshot().Generation != 1 {
		t.Fatal("policy used a fake lease or changed owner")
	}
	lease, err := m.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	lease.Release()
	if m.Snapshot().ActiveLeases != 0 {
		t.Fatal("work lease did not release")
	}
}
