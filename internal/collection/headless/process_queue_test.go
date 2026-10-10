package headless

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestIdleHoldDeadlineIncludesWaitingForActiveRPC(t *testing.T) {
	p := &Process{}
	p.operationOnce.Do(func() { p.operation = make(chan struct{}, 1) })
	p.operation <- struct{}{} // An existing RPC owns the transport.
	defer func() { <-p.operation }()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- p.SetIdleHold(ctx, true) }()
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal("queued hold bypassed its deadline", err)
		}
	case <-time.After(time.Second):
		t.Fatal("admission remained blocked behind an unrelated active RPC")
	}
	if p.sequence.Load() != 0 {
		t.Fatal("expired queued hold was dispatched")
	}
}
