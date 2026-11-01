package engine

import (
	"context"
	"testing"
	"time"
)

func TestWorkLifecyclePendingPresenceAndPlanningCoalescing(t *testing.T) {
	for _, tc := range []struct {
		name         string
		requests     []bool
		wantPlanning bool
	}{
		{"false remains queued", []bool{false, false}, false},
		{"planning remains enabled", []bool{true, false}, true},
		{"planning can be enabled", []bool{false, true}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := &Engine{work: newEngineWorkState()}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			e.mu.Lock()
			if !e.work.admitRun("run", false, false) {
				t.Fatal("fresh run rejected")
			}
			e.work.registerRun("run", cancel)
			if e.work.admitRun("run", true, false) || e.work.snapshot().pending != 0 {
				t.Fatal("duplicate launch queued without a continuation request")
			}
			for _, allowPlanning := range tc.requests {
				if e.work.admitRun("run", allowPlanning, true) {
					t.Fatal("queued continuation acquired duplicate active ownership")
				}
			}
			if e.work.snapshot().pending != 1 {
				t.Fatal("false planning value lost the pending entry")
			}
			planning, relaunch := e.work.retireRun("run", true)
			work := e.work.snapshot()
			e.mu.Unlock()
			if !relaunch || planning != tc.wantPlanning || work.active != 0 || work.pending != 0 || work.workers != 1 {
				t.Fatalf("continuation changed: planning=%v relaunch=%v snapshot=%+v", planning, relaunch, work)
			}
			if ctx.Err() != nil {
				t.Fatal("retiring active bookkeeping canceled the running cleanup")
			}
			if e.WaitForIdle(time.Millisecond) {
				t.Fatal("worker cleanup was omitted from idle accounting")
			}
			e.mu.Lock()
			e.work.workerFinished()
			e.mu.Unlock()
			if !e.WaitForIdle(50 * time.Millisecond) {
				t.Fatal("finished cleanup remained busy")
			}
		})
	}
}

func TestWorkLifecycleCancellationAndShutdownFenceRelaunch(t *testing.T) {
	for _, shutdown := range []bool{false, true} {
		name := "cancellation tombstone"
		if shutdown {
			name = "shutdown"
		}
		t.Run(name, func(t *testing.T) {
			e := &Engine{work: newEngineWorkState()}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			e.mu.Lock()
			e.work.registerRun("run", cancel)
			e.work.admitRun("run", false, true)
			if shutdown {
				e.work.beginShutdown()
				e.work.cancelActive()
			} else {
				e.work.prepareCancellation()
				e.work.cancelRun("run")
			}
			if ctx.Err() == nil || e.work.snapshot().active != 1 || e.work.snapshot().workers != 1 {
				t.Fatal("cancellation removed worker ownership before retirement")
			}
			_, relaunch := e.work.retireRun("run", true)
			e.work.workerFinished()
			if relaunch || e.work.admitRun("run", true, true) {
				t.Fatal("canceled or shutdown work reentered after active retirement")
			}
			e.mu.Unlock()
		})
	}
}
