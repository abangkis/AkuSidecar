package engine

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/activationguard"
)

func TestSchedulerLifecycleConcurrentStartOnceRemainsConsumedAfterStop(t *testing.T) {
	e := &Engine{scheduler: newAutoUpdateLifecycle()}
	contexts := make(chan context.Context, 16)
	var starts sync.WaitGroup
	for i := 0; i < cap(contexts); i++ {
		starts.Add(1)
		go func() {
			defer starts.Done()
			e.mu.Lock()
			ctx, started := e.scheduler.startOnce()
			if started {
				contexts <- ctx
			}
			e.mu.Unlock()
		}()
	}
	starts.Wait()
	if len(contexts) != 1 {
		t.Fatalf("scheduler started %d times", len(contexts))
	}
	ctx := <-contexts
	e.mu.Lock()
	installed := e.scheduler.hasStarted()
	e.scheduler.stop()
	e.scheduler.stop()
	_, restarted := e.scheduler.startOnce()
	sticky := e.scheduler.hasStarted()
	e.mu.Unlock()
	if !installed || ctx.Err() != context.Canceled || restarted || !sticky {
		t.Fatal("cancellation cleared or renewed scheduler admission")
	}
}

func TestSchedulerLifecycleWakeIsCoalescedBeforeStartAndAfterStop(t *testing.T) {
	e := &Engine{scheduler: newAutoUpdateLifecycle()}
	wake := e.scheduler.wake
	if cap(wake) != 1 {
		t.Fatal("wake capacity changed")
	}
	for i := 0; i < 100; i++ {
		e.scheduler.notify()
	}
	if len(wake) != 1 {
		t.Fatal("pre-start wake was lost or multiplied")
	}
	e.mu.Lock()
	_, started := e.scheduler.startOnce()
	e.scheduler.stop()
	e.mu.Unlock()
	if !started || e.scheduler.wake != wake || len(wake) != 1 {
		t.Fatal("start/stop replaced or drained the wake channel")
	}
	<-wake
	for i := 0; i < 100; i++ {
		e.scheduler.notify()
	}
	if len(wake) != 1 {
		t.Fatal("post-cancel notifications no longer coalesce")
	}
	if _, open := <-wake; !open {
		t.Fatal("shutdown closed the producer channel")
	}
	var zero autoUpdateLifecycle
	zero.notify() // A zero Engine retains its original nil-channel no-op.
}

func TestSchedulerLifecycleEngineKeepsPreinstalledCancellationAndNoDrain(t *testing.T) {
	e := &Engine{scheduler: newAutoUpdateLifecycle()}
	stopped := false
	e.scheduler.cancel = func() { stopped = true }
	e.StartAutoUpdateScheduler()
	e.Shutdown()
	if !stopped {
		t.Fatal("start replaced an installed cancellation handle")
	}
	if !e.WaitForIdle(50 * time.Millisecond) {
		t.Fatal("scheduler was added to worker drain accounting")
	}
}

func TestSchedulerLifecycleMaintenanceGuardRemembersStartAfterStop(t *testing.T) {
	guard, err := activationguard.Open(filepath.Join(t.TempDir(), "sidecar.exe"+activationguard.FileSuffix))
	if err != nil {
		t.Fatal(err)
	}
	fresh := &Engine{scheduler: newAutoUpdateLifecycle()}
	if err := fresh.BindNativeDerivationHold(guard); err != nil {
		t.Fatal("fresh scheduler rejected maintenance binding", err)
	}
	e := &Engine{scheduler: newAutoUpdateLifecycle()}
	e.mu.Lock()
	_, started := e.scheduler.startOnce()
	e.mu.Unlock()
	if !started {
		t.Fatal("fresh scheduler rejected start")
	}
	t.Cleanup(func() { e.mu.Lock(); e.scheduler.stop(); e.mu.Unlock() })
	if err := e.BindNativeDerivationHold(guard); err == nil {
		t.Fatal("started scheduler accepted late maintenance binding")
	}
	e.mu.Lock()
	e.scheduler.stop()
	e.mu.Unlock()
	if err := e.BindNativeDerivationHold(guard); err == nil {
		t.Fatal("stopped scheduler forgot its start-once admission")
	}
}

type schedulerLogFunc func(string, ...any)

func (f schedulerLogFunc) Printf(format string, args ...any) { f(format, args...) }

func TestSchedulerLifecycleStartPublishesHandleBeforeQueuedWake(t *testing.T) {
	e, state := testEngineWithClock(t, &mutableEngineClock{now: time.Now()})
	if err := state.Close(); err != nil {
		t.Fatal(err)
	}
	observed := make(chan bool, 1)
	e.logger = schedulerLogFunc(func(format string, _ ...any) {
		if strings.HasPrefix(format, "recover expired Bridge command:") {
			e.mu.RLock()
			started := e.scheduler.hasStarted()
			e.mu.RUnlock()
			observed <- started
		}
	})
	e.scheduler.notify()
	t.Cleanup(e.Shutdown)
	e.StartAutoUpdateScheduler()
	select {
	case started := <-observed:
		if !started {
			t.Fatal("queued wake observed an unpublished cancellation handle")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("scheduler did not consume its pre-start wake")
	}
}

// Only tests join this goroutine. Production Shutdown keeps its existing
// cancellation-only contract and WaitForIdle keeps its current work predicate.
func runSchedulerLoopForTest(t *testing.T, e *Engine, ticks <-chan time.Time) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); e.autoUpdateLoopOnTicks(ctx, ticks) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("scheduler loop did not stop")
		}
	})
}

func TestSchedulerLifecycleLoopWaitsAndAttemptsUpdateAfterRecoveryError(t *testing.T) {
	clock := &mutableEngineClock{now: time.Now()}
	e, state := testEngineWithClock(t, clock)
	if err := state.Close(); err != nil {
		t.Fatal(err)
	}
	events := make(chan string, 16)
	e.logger = schedulerLogFunc(func(format string, args ...any) { events <- fmt.Sprintf(format, args...) })
	ticks := make(chan time.Time)
	runSchedulerLoopForTest(t, e, ticks)
	select {
	case event := <-events:
		t.Fatal("loop ran without tick/wake", event)
	case <-time.After(25 * time.Millisecond):
	}
	checkStep := func() {
		t.Helper()
		for _, prefix := range []string{"recover expired Bridge command:", "Auto Update deferred:"} {
			select {
			case event := <-events:
				if !strings.HasPrefix(event, prefix) {
					t.Fatalf("step order: expected %q, got %q", prefix, event)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("scheduler step was suppressed")
			}
		}
	}
	e.scheduler.notify()
	checkStep()
	select {
	case ticks <- time.Now():
	case <-time.After(2 * time.Second):
		t.Fatal("tick was not received")
	}
	checkStep()
}

func TestSchedulerLifecycleRecoversExpiredCommandBeforeDisabledUpdate(t *testing.T) {
	clock := &mutableEngineClock{now: time.Now().Add(-10 * time.Minute)}
	e, state := testEngineWithClock(t, clock)
	ctx := context.Background()
	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.AutoUpdateEnabled = false
	if err := state.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	e.RecordHeartbeat(ExpectedHeartbeat()) // Queues a wake before the loop starts.
	session, err := e.StartVisibleUpdate(ctx, "offline expired dispatch")
	if err != nil {
		t.Fatal(err)
	}
	recovered := make(chan bool, 1)
	e.logger = schedulerLogFunc(func(format string, args ...any) {
		if strings.HasPrefix(fmt.Sprintf(format, args...), "Auto Update deferred: Auto Update is disabled") {
			current, readErr := state.GetSession(ctx, session.ID)
			recovered <- readErr == nil && len(current.Runs) > 0 && current.Runs[0].Error != nil && current.Runs[0].Error.Code == "capture_dispatch_timeout"
		}
	})
	runSchedulerLoopForTest(t, e, nil)
	select {
	case ok := <-recovered:
		if !ok {
			t.Fatal("disabled-update decision preceded command recovery")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("disabled Auto Update suppressed the recovery loop")
	}
}
