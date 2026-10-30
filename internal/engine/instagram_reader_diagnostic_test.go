package engine

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/activationguard"
	"github.com/abangkis/AkuSidecar/internal/collection"
	"github.com/abangkis/AkuSidecar/internal/config"
)

const diagnosticSafeResult = `{"schema":"aku.instagram-reader-diagnostic.v1","primaryCause":"http_rejected","status":400,"mime":"json","bodyClass":"json_object","errorCategory":"unknown","documentBound":true,"wireMatched":true,"cleanup":"verified"}`

func instagramDiagnosticEngine(t *testing.T) *Engine {
	t.Helper()
	e, _ := testEngine(t)
	e.config.Dev = true
	e.config.Deployment.Mode = "development"
	e.config.WindowsCaptureSplit = true
	e.config.Capture.XMode = config.XCaptureConcurrent
	e.config.Capture.XReaderSource = "main"
	e.config.Capture.InstagramTrial = true
	e.config.Capture.InstagramMode = config.XCaptureConcurrent
	owner := attachTestCapture(t, e)
	if err := owner.RetainForProcess(); err != nil {
		t.Fatal(err)
	}
	e.AttachCollectionCoordinator(collection.NewCoordinator(owner, nil, func() error { return nil }))
	return e
}

func holdInstagramDiagnostic(t *testing.T, e *Engine, sources string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "diagnostic-hold.json")
	if err := os.WriteFile(path, []byte(`{"schema":"aku.native-derivation-hold.v1","activationId":"`+strings.Repeat("a", 32)+`","sources":`+sources+`}`), 0600); err != nil {
		t.Fatal(err)
	}
	hold, err := activationguard.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = e.BindNativeDerivationHold(hold); err != nil {
		t.Fatal(err)
	}
}

func requireInstagramDiagnosticBuild(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "windows" || !xDailyBuildSupported {
		t.Skip("sealed diagnostic requires Windows auth_session_dev")
	}
}

func TestInstagramReaderDiagnosticAdmissionIsOneShotAndPreservesState(t *testing.T) {
	requireInstagramDiagnosticBuild(t)
	e := instagramDiagnosticEngine(t)
	holdInstagramDiagnostic(t, e, `["x","instagram"]`)
	before, _ := e.Settings(context.Background())
	calls := 0
	action := func(ctx context.Context) (json.RawMessage, error) {
		calls++
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 30*time.Second || time.Until(deadline) < 25*time.Second {
			t.Fatal("diagnostic budget missing")
		}
		if e.operation.TryLock() {
			e.operation.Unlock()
			t.Fatal("normal work admission was not retained")
		}
		return json.RawMessage(diagnosticSafeResult), nil
	}
	raw, err := e.RunInstagramReaderDiagnostic(context.Background(), action)
	if err != nil || !json.Valid(raw) || calls != 1 {
		t.Fatalf("safe diagnostic result: %v", err)
	}
	if _, err = e.RunInstagramReaderDiagnostic(context.Background(), action); err == nil || calls != 1 {
		t.Fatal("one-shot was replayed")
	}
	after, _ := e.Settings(context.Background())
	if !reflect.DeepEqual(before, after) || !e.nativeDerivationHold.Held("x") || !e.nativeDerivationHold.Held("instagram") {
		t.Fatal("diagnostic changed permission/settings/hold")
	}
	for _, source := range dailySources {
		d := e.dailyController(source)
		if d.state.owner != nil || d.state.startUsed || d.state.generation != 0 {
			t.Fatal("diagnostic started a source")
		}
	}
}

func TestInstagramReaderDiagnosticAdmissionRejectsWithoutDispatch(t *testing.T) {
	requireInstagramDiagnosticBuild(t)
	for _, mode := range []string{"dev_off", "wrong_deployment", "split_off", "dedicated", "trial_off", "no_hold", "one_hold", "operation_busy", "workers", "pending", "active", "daily_cleanup", "daily_start", "daily_uncertain", "missing_coordinator", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			e := instagramDiagnosticEngine(t)
			if mode != "no_hold" {
				sources := `["x","instagram"]`
				if mode == "one_hold" {
					sources = `["instagram"]`
				}
				holdInstagramDiagnostic(t, e, sources)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			restore := func() {}
			switch mode {
			case "dev_off":
				e.config.Dev = false
			case "wrong_deployment":
				e.config.Deployment.Mode = "production-installed-app"
			case "split_off":
				e.config.WindowsCaptureSplit = false
			case "dedicated":
				e.config.Capture.XReaderSource = "dedicated"
			case "trial_off":
				e.config.Capture.InstagramTrial = false
			case "operation_busy":
				e.operation.Lock()
				restore = e.operation.Unlock
			case "workers":
				e.processWorkers = 1
				restore = func() { e.processWorkers = 0 }
			case "pending":
				e.pending["synthetic"] = true
				restore = func() { delete(e.pending, "synthetic") }
			case "active":
				e.active["synthetic"] = func() {}
				restore = func() { delete(e.active, "synthetic") }
			case "daily_cleanup":
				e.instagramDaily.done = make(chan struct{})
				restore = func() { e.instagramDaily.done = nil }
			case "daily_start":
				e.instagramDaily.startDone = make(chan struct{})
				restore = func() { e.instagramDaily.startDone = nil }
			case "daily_uncertain":
				e.instagramDaily.cleanupUncertain = true
				restore = func() { e.instagramDaily.cleanupUncertain = false }
			case "missing_coordinator":
				e.collectionRuntime = nil
			case "canceled":
				cancel()
			}
			calls := 0
			_, err := e.RunInstagramReaderDiagnostic(ctx, func(context.Context) (json.RawMessage, error) { calls++; return nil, nil })
			restore()
			if err == nil || calls != 0 || e.instagramReaderDiagnosticUsed {
				t.Fatalf("%s admission dispatched", mode)
			}
		})
	}
}

func TestInstagramReaderDiagnosticOutputRevalidatedAndAttemptConsumed(t *testing.T) {
	requireInstagramDiagnosticBuild(t)
	for _, outcome := range []string{"error", "private", "uncertain"} {
		t.Run(outcome, func(t *testing.T) {
			e := instagramDiagnosticEngine(t)
			holdInstagramDiagnostic(t, e, `["x","instagram"]`)
			raw, err := e.RunInstagramReaderDiagnostic(context.Background(), func(context.Context) (json.RawMessage, error) {
				if outcome == "error" {
					return nil, errors.New("PRIVATE_TOKEN")
				}
				if outcome == "private" {
					return json.RawMessage(`{"accountId":"PRIVATE_ACCOUNT"}`), nil
				}
				return json.RawMessage(strings.Replace(diagnosticSafeResult, `"verified"`, `"uncertain"`, 1)), nil
			})
			if outcome == "uncertain" {
				if err != nil || !strings.Contains(string(raw), `"cleanup":"uncertain"`) {
					t.Fatal("cleanup uncertainty discarded")
				}
			} else if err == nil || strings.Contains(err.Error(), "PRIVATE") || len(raw) != 0 {
				t.Fatal("private failure escaped")
			}
			if !e.instagramReaderDiagnosticUsed {
				t.Fatal("admitted failed attempt could be replayed")
			}
		})
	}
}

func TestInstagramReaderDiagnosticIsUnavailableWithoutBuildSupport(t *testing.T) {
	if runtime.GOOS == "windows" && xDailyBuildSupported {
		t.Skip("off-build assertion")
	}
	e := instagramDiagnosticEngine(t)
	if e.InstagramReaderDiagnosticConfigured() {
		t.Fatal("unsupported build enabled diagnostic")
	}
	if _, err := e.RunInstagramReaderDiagnostic(context.Background(), func(context.Context) (json.RawMessage, error) {
		t.Fatal("unsupported build dispatched")
		return nil, nil
	}); err == nil {
		t.Fatal("unsupported build accepted")
	}
}

func TestInstagramReaderDiagnosticExcludesDailyStartBeforePermissionMutation(t *testing.T) {
	requireInstagramDiagnosticBuild(t)
	e := instagramDiagnosticEngine(t)
	holdInstagramDiagnostic(t, e, `["x","instagram"]`)
	calls := 0
	for _, source := range dailySources {
		d := e.dailyController(source)
		d.state.attached = true
		d.state.parent = context.Background()
		d.state.profileScope = strings.Repeat("a", 64)
		d.state.factory = func(context.Context, context.Context) (sourceDailyOwner, error) {
			calls++
			return nil, errors.New("unexpected factory")
		}
	}
	entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		_, err := e.RunInstagramReaderDiagnostic(context.Background(), func(context.Context) (json.RawMessage, error) {
			close(entered)
			<-release
			return json.RawMessage(diagnosticSafeResult), nil
		})
		finished <- err
	}()
	<-entered
	for _, source := range dailySources {
		d := e.dailyController(source)
		consumed, err := d.start(context.Background(), XDailyStartRequest{AcknowledgeLimitations: true, AllowProcessSessionDerivation: true}, nil)
		if err == nil || consumed || d.state.startUsed || d.state.permissionRevision != 0 || d.state.rememberedPermission || d.state.startDone != nil || !d.maintenanceHeld() {
			t.Error("daily start mutated permission under diagnostic")
		}
	}
	close(release)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Fatal("daily factory ran under diagnostic")
	}
}

func TestInstagramReaderDiagnosticRejectsWhileDailyStartOwnsAdmission(t *testing.T) {
	requireInstagramDiagnosticBuild(t)
	for _, source := range dailySources {
		t.Run(string(source), func(t *testing.T) {
			e := instagramDiagnosticEngine(t)
			holdInstagramDiagnostic(t, e, `["x","instagram"]`)
			d := e.dailyController(source)
			d.state.attached = true
			d.state.parent = context.Background()
			d.state.profileScope = strings.Repeat("a", 64)
			entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan error, 1)
			d.state.factory = func(context.Context, context.Context) (sourceDailyOwner, error) {
				close(entered)
				<-release
				return nil, errors.New("synthetic startup failure")
			}
			go func() {
				_, err := d.start(context.Background(), XDailyStartRequest{AcknowledgeLimitations: true, AllowProcessSessionDerivation: true}, nil)
				finished <- err
			}()
			select {
			case <-entered:
			case err := <-finished:
				t.Fatalf("synthetic start failed before its factory: %v", err)
			case <-time.After(time.Second):
				t.Fatal("synthetic factory did not start")
			}
			calls := 0
			_, err := e.RunInstagramReaderDiagnostic(context.Background(), func(context.Context) (json.RawMessage, error) { calls++; return nil, nil })
			close(release)
			<-finished
			var rejected *InstagramReaderDiagnosticError
			if !errors.As(err, &rejected) || rejected.Reason != "busy" || calls != 0 || e.instagramReaderDiagnosticUsed {
				t.Fatal("diagnostic raced daily startup admission")
			}
		})
	}
}

func TestInstagramReaderDiagnosticShutdownDrainsBeforeOwnerRetirement(t *testing.T) {
	requireInstagramDiagnosticBuild(t)
	e := instagramDiagnosticEngine(t)
	holdInstagramDiagnostic(t, e, `["x","instagram"]`)
	entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		_, err := e.RunInstagramReaderDiagnostic(context.Background(), func(ctx context.Context) (json.RawMessage, error) {
			close(entered)
			<-release
			if ctx.Err() != nil {
				t.Error("shutdown canceled cleanup observation")
			}
			return json.RawMessage(diagnosticSafeResult), nil
		})
		finished <- err
	}()
	<-entered
	if e.WaitForIdle(time.Millisecond) {
		t.Error("in-flight diagnostic was omitted from worker accounting")
	}
	e.Shutdown()
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if e.DrainInstagramReaderDiagnostic(canceled) == nil {
		t.Error("blocked diagnostic reported drained")
	}
	retired := make(chan struct{})
	go func() {
		if e.DrainInstagramReaderDiagnostic(context.Background()) == nil {
			close(retired)
		}
	}()
	select {
	case <-retired:
		t.Error("owner retired before diagnostic cleanup")
	default:
	}
	close(release)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	select {
	case <-retired:
	case <-time.After(time.Second):
		t.Fatal("diagnostic drain did not finish")
	}
	if !e.WaitForIdle(time.Second) {
		t.Fatal("diagnostic worker did not retire")
	}
}
