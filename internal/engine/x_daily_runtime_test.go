//go:build auth_session_dev

package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
	"github.com/abangkis/AkuSidecar/internal/store"
)

type syntheticDailyOwner struct {
	mu                            sync.Mutex
	e                             *Engine
	generations                   []uint64
	readerCloses, collectorCloses int
	failCleanup                   bool
	failOwnerCleanup              bool
	hold                          chan struct{}
	captureStarted                chan struct{}
	check                         func(context.Context) error
	begin                         func(context.Context, context.Context, uint64, domain.Run) (xDailyNativeBatch, error)
}

func (o *syntheticDailyOwner) CheckReader(ctx context.Context) error {
	if o.check != nil {
		return o.check(ctx)
	}
	return nil
}
func (o *syntheticDailyOwner) PrepareReader(context.Context, readerbroker.Request, string) (readerbroker.NativePostPreparation, error) {
	return readerbroker.NativePostPreparation{}, nil
}
func (o *syntheticDailyOwner) Close(context.Context) error {
	o.mu.Lock()
	o.readerCloses++
	o.mu.Unlock()
	if o.failOwnerCleanup {
		return errors.New("synthetic process cleanup uncertain")
	}
	return nil
}

type syntheticDailyBatch struct {
	o      *syntheticDailyOwner
	run    domain.Run
	start  time.Time
	permit *store.ManualPilotPermit
}

func (o *syntheticDailyOwner) BeginBatch(ctx, parent context.Context, g uint64, r domain.Run) (xDailyNativeBatch, error) {
	if o.begin != nil {
		return o.begin(ctx, parent, g, r)
	}
	o.mu.Lock()
	o.generations = append(o.generations, g)
	o.mu.Unlock()
	start := o.e.store.Now()
	binding := store.DevelopmentManualPilotBinding{Digest: strings.Repeat("a", 64), Attempt: fmt.Sprintf("synthetic:%d", g), OwnerGeneration: g, StartedAt: start, Deadline: start.Add(time.Minute)}
	p, err := o.e.store.NewDevelopmentDailyPilotIssuer(func(context.Context) (store.DevelopmentManualPilotBinding, error) { return binding, nil })(ctx, r.SessionID, r.ID)
	return &syntheticDailyBatch{o: o, run: r, start: start, permit: p}, err
}
func (b *syntheticDailyBatch) StartedAt() time.Time { return b.start }
func (b *syntheticDailyBatch) Deadline() time.Time  { return b.start.Add(time.Minute) }
func (b *syntheticDailyBatch) Capture(ctx context.Context, command string) (domain.Run, error) {
	if b.o.captureStarted != nil {
		close(b.o.captureStarted)
	}
	if b.o.hold != nil {
		select {
		case <-b.o.hold:
		case <-ctx.Done():
			return domain.Run{}, ctx.Err()
		}
	}
	claim, err := b.o.e.store.ClaimManualPilot(ctx, b.permit, command, "synthetic-claim:"+command)
	if err != nil {
		return domain.Run{}, err
	}
	observation := domain.Observation{Source: domain.SourceX, CapturedAt: domain.Now(), Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{EvidenceKey: "x:" + b.run.ID, Text: "Synthetic daily batch evidence"}}}}}
	out, err := b.o.e.store.CommitManualPilotResult(ctx, claim.Handle, "synthetic-result:"+command, observation)
	if err != nil || out != store.Committed {
		return domain.Run{}, fmt.Errorf("synthetic commit %s: %v", out, err)
	}
	return b.o.e.afterAcceptedObservation(ctx, b.run)
}
func (b *syntheticDailyBatch) Close(ctx context.Context) error {
	b.o.mu.Lock()
	b.o.collectorCloses++
	fail := b.o.failCleanup
	b.o.mu.Unlock()
	_, err := b.o.e.store.RevokeManualPilot(ctx, b.permit, "synthetic-revoke:"+b.run.ID)
	if err != nil {
		return err
	}
	if fail {
		return errors.New("synthetic cleanup uncertain")
	}
	return nil
}
func dailyRuntimeFixture(t *testing.T) (*Engine, *syntheticDailyOwner) {
	t.Helper()
	return dailyRuntimeFixtureWithConfig(t, nil)
}

func dailyRuntimeFixtureWithConfig(t *testing.T, configure func(*config.Config)) (*Engine, *syntheticDailyOwner) {
	t.Helper()
	e, state := testEngine(t)
	e.config.Dev = true
	e.config.Database.Path = state.Path()
	e.config.Capture.XMode = config.XCaptureConcurrent
	if configure != nil {
		configure(&e.config)
	}
	s, err := PrepareXDailyDevelopment(e.config, config.Options{Dev: true, AppShell: e.config.Capture.XUsesMainProfile()})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Attach(e); err != nil {
		t.Fatal(err)
	}
	o := &syntheticDailyOwner{e: e}
	if err = e.configureXDailyFactory(context.Background(), func(context.Context, context.Context) (xDailyNativeOwner, error) { return o, nil }); err != nil {
		t.Fatal(err)
	}
	settings, _ := state.GetSettings(context.Background())
	settings.ActiveSources = []domain.Source{domain.SourceX}
	settings.CalibrationEnabled = false
	settings.AIDetectionEnabled = false
	if err = state.SaveSettings(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	if err = e.StartXDaily(context.Background(), XDailyStartRequest{true, true}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
		if !e.WaitForIdle(time.Second) {
			t.Error("daily worker not drained")
		}
	})
	return e, o
}
func awaitDailyBatch(t *testing.T, e *Engine, session string) {
	t.Helper()
	end := time.Now().Add(3 * time.Second)
	for time.Now().Before(end) {
		v, err := e.store.GetSession(context.Background(), session)
		e.xDaily.mu.RLock()
		done := e.xDaily.done
		e.xDaily.mu.RUnlock()
		if err == nil && v.Status == "completed" && done == nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	v, _ := e.store.GetSession(context.Background(), session)
	t.Fatalf("daily batch did not complete %+v status=%+v", v, e.XCaptureStatus())
}
func TestXDailyRepeatedBatchesPersistAndStop(t *testing.T) {
	e, o := dailyRuntimeFixture(t)
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		session, err := e.StartVisibleUpdate(ctx, "daily synthetic")
		if err != nil {
			t.Fatal(err)
		}
		awaitDailyBatch(t, e, session.ID)
	}
	o.mu.Lock()
	if len(o.generations) != 2 || o.generations[0] != 1 || o.generations[1] != 2 || o.readerCloses != 0 || o.collectorCloses != 2 {
		t.Fatalf("lifetimes %+v", o.generations)
	}
	o.mu.Unlock()
	if err := e.StopXManual(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := e.StartVisibleUpdate(ctx, "third must not run"); err == nil {
		t.Fatal("Stop authorized batch three")
	}
	if err := e.StartXDaily(ctx, XDailyStartRequest{true, true}); err == nil {
		t.Fatal("consent replay restored process grant")
	}
}
func TestXDailyStopCancelsBatchPreservesReader(t *testing.T) {
	e, o := dailyRuntimeFixture(t)
	o.hold = make(chan struct{})
	o.captureStarted = make(chan struct{})
	ctx := context.Background()
	if _, err := e.StartVisibleUpdate(ctx, "held"); err != nil {
		t.Fatal(err)
	}
	// Wait for an issued permit and active capture, not merely BeginBatch entry.
	// Startup cancellation has its own regression below.
	select {
	case <-o.captureStarted:
	case <-time.After(time.Second):
		t.Fatal("synthetic capture did not start")
	}
	if err := e.StopXManual(ctx); err != nil {
		t.Fatal(err)
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.readerCloses != 0 || o.collectorCloses != 1 {
		t.Fatal("Stop closed reader or failed collector cleanup")
	}
}

func TestXDailyCleanupFailureLatchesProcess(t *testing.T) {
	e, o := dailyRuntimeFixture(t)
	o.failCleanup = true
	ctx := context.Background()
	if _, err := e.StartVisibleUpdate(ctx, "cleanup failure"); err != nil {
		t.Fatal(err)
	}
	end := time.Now().Add(time.Second)
	for time.Now().Before(end) {
		e.xDaily.mu.RLock()
		uncertain := e.xDaily.cleanupUncertain
		e.xDaily.mu.RUnlock()
		if uncertain {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if e.xDailyReady() {
		t.Fatal("uncertain cleanup retained authorization")
	}
	if _, err := e.StartVisibleUpdate(ctx, "must not derive"); err == nil {
		t.Fatal("cleanup failure allowed a successor")
	}
}

type syntheticFailedStartupBatch struct {
	cleanup func() error
}

func (*syntheticFailedStartupBatch) StartedAt() time.Time { return time.Time{} }
func (*syntheticFailedStartupBatch) Deadline() time.Time  { return time.Time{} }
func (*syntheticFailedStartupBatch) Capture(context.Context, string) (domain.Run, error) {
	return domain.Run{}, errors.New("capture must not run after failed startup")
}
func (b *syntheticFailedStartupBatch) Close(context.Context) error { return b.cleanup() }

func TestXDailyStartupCauseSurvivesStopAndCleanupFailure(t *testing.T) {
	e, o := dailyRuntimeFixture(t)
	var output bytes.Buffer
	e.logger = log.New(&output, "", 0)
	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	releaseCleanup := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseCleanup()
	private := errors.New("private account and cookie error")
	original := xDailyStartupFail(private, "seed_rpc", "rpc")
	o.begin = func(context.Context, context.Context, uint64, domain.Run) (xDailyNativeBatch, error) {
		return &syntheticFailedStartupBatch{cleanup: func() error {
			close(entered)
			<-release
			return errors.New("secondary cleanup error")
		}}, original
	}
	if _, err := e.StartVisibleUpdate(context.Background(), "startup failure"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("startup cleanup did not start")
	}
	// Stop can publish its own state while cleanup is pending. It must not erase
	// the startup predicate already returned by the owner.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := e.StopXManual(ctx); err == nil {
		t.Fatal("pending cleanup reported stopped")
	}
	status := e.XCaptureStatus()
	if status.FailurePhase != "startup" || status.StartupDiagnostic == nil || status.StartupDiagnostic.Stage != "seed_rpc" {
		t.Fatalf("first failure missing during cleanup: %+v", status)
	}
	status.StartupDiagnostic.Stage = "mutated"
	e.xDaily.mu.RLock()
	drain := e.xDaily.drain
	e.xDaily.mu.RUnlock()
	e.rememberXDailyFailure(errors.New("late secondary failure"), "cleanup")
	if e.WaitForIdle(time.Millisecond) {
		t.Fatal("pending cleanup reported idle")
	}
	releaseCleanup()
	select {
	case <-drain:
	case <-time.After(time.Second):
		t.Fatal("cleanup worker did not drain")
	}
	status = e.XCaptureStatus()
	if status.StartupDiagnostic.Stage != "seed_rpc" || status.Cleanup != "uncertain" || status.DerivationAuthorized {
		t.Fatalf("cleanup replaced startup diagnostic or restored authority: %+v", status)
	}
	e.xDaily.mu.RLock()
	retained := e.xDaily.firstFailure
	e.xDaily.mu.RUnlock()
	if !errors.Is(retained, private) {
		t.Fatal("private first cause was discarded")
	}
	if strings.Contains(output.String(), "private account") || !strings.Contains(output.String(), "x_daily_startup stage=seed_rpc condition=rpc cause=unavailable") {
		t.Fatal("terminal log lost safe predicate or exposed private cause")
	}
	// Test teardown may now finish the intentionally uncertain synthetic cleanup.
	e.xDaily.mu.Lock()
	e.xDaily.cleanupUncertain = false
	e.xDaily.mu.Unlock()
}

func TestXDailyStopJoinsStartupAndRejectsLateOwner(t *testing.T) {
	e, state := testEngine(t)
	e.config.Dev = true
	e.config.Database.Path = state.Path()
	e.config.Capture.XMode = config.XCaptureConcurrent
	s, err := PrepareXDailyDevelopment(e.config, config.Options{Dev: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Attach(e); err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	o := &syntheticDailyOwner{e: e}
	if err = e.configureXDailyFactory(context.Background(), func(context.Context, context.Context) (xDailyNativeOwner, error) {
		close(entered)
		<-release
		return o, nil
	}); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { result <- e.StartXDaily(context.Background(), XDailyStartRequest{true, true}) }()
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err = e.StopXManual(ctx); err == nil {
		t.Fatal("unjoined startup reported stopped")
	}
	if e.WaitForIdle(time.Millisecond) {
		t.Fatal("pending startup reported idle")
	}
	close(release)
	if err = <-result; err == nil {
		t.Fatal("late owner inherited stopped grant")
	}
	if e.xDailyReady() {
		t.Fatal("late startup authorized")
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestXDailyStartupFailureRetainsUncertainOwner(t *testing.T) {
	e, state := testEngine(t)
	e.config.Dev = true
	e.config.Database.Path = state.Path()
	e.config.Capture.XMode = config.XCaptureConcurrent
	s, err := PrepareXDailyDevelopment(e.config, config.Options{Dev: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Attach(e); err != nil {
		t.Fatal(err)
	}
	o := &syntheticDailyOwner{e: e, failOwnerCleanup: true}
	if err = e.configureXDailyFactory(context.Background(), func(context.Context, context.Context) (xDailyNativeOwner, error) {
		return o, errors.New("synthetic startup rejected")
	}); err != nil {
		t.Fatal(err)
	}
	if err = e.StartXDaily(context.Background(), XDailyStartRequest{true, true}); err == nil {
		t.Fatal("startup accepted")
	}
	if err = e.StopXManual(context.Background()); err == nil {
		t.Fatal("uncertain owner reported clean stop")
	}
	if e.WaitForIdle(time.Millisecond) {
		t.Fatal("uncertain startup reported idle")
	}
	if err = s.Close(); err == nil {
		t.Fatal("uncertain process owner lost during Close")
	}
	e.xDaily.mu.RLock()
	retained := e.xDaily.owner == o
	e.xDaily.mu.RUnlock()
	if !retained {
		t.Fatal("partial owner was discarded")
	}
	o.failOwnerCleanup = false
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestXDailyStopOwnsWorkerBeforeCommandPersistence(t *testing.T) {
	e, o := dailyRuntimeFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	o.check = func(context.Context) error { close(entered); <-release; return nil }
	session, err := e.StartVisibleUpdate(context.Background(), "stop before SQL")
	if err != nil {
		t.Fatal(err)
	}
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err = e.StopXManual(ctx); err == nil {
		t.Fatal("unjoined SQL admission reported stopped")
	}
	close(release)
	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	if err = e.StopXManual(ctx2); err != nil {
		t.Fatal(err)
	}
	end := time.Now().Add(time.Second)
	for time.Now().Before(end) {
		v, _ := e.store.GetSession(context.Background(), session.ID)
		if v.Status == "failed" {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("unissued X command stranded the active session")
}
