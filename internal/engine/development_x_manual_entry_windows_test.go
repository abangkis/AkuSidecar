//go:build windows && auth_session_dev && auth_session_fixture

package engine

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"golang.org/x/sys/windows"
)

func xDevelopmentOptions(t *testing.T) (config.Options, config.Config) {
	t.Helper()
	node := os.Getenv("AKU_CHROME_FIXTURE_NODE")
	if node == "" {
		t.Skip("canonical development entry helper required")
	}
	file, err := WriteXApplicationQASyntheticPlan(t.TempDir(), node, 28173)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Dir(file)
	if err = os.Mkdir(filepath.Join(root, "config"), 0700); err != nil {
		t.Fatal(err)
	}
	opts := config.Options{Dev: true, DevelopmentSessionReuse: "native-x-manual", DevelopmentXManualPlan: file, ConfigPath: filepath.Join(root, "config", "sidecar.json"), DatabasePath: filepath.Join(root, "state", "pilot.db"), Port: 28173}
	cfg := config.Config{Dev: true, Root: root, Server: config.ServerConfig{Host: "127.0.0.1", Port: 28173}, Database: config.DatabaseConfig{Path: opts.DatabasePath}, Capture: config.CaptureConfig{MaxAcquisitionRounds: 1}}
	raw, _ := json.Marshal(cfg)
	if os.WriteFile(opts.ConfigPath, raw, 0600) != nil {
		t.Fatal("config write failed")
	}
	return opts, cfg
}

func xDevelopmentEngine(t *testing.T) (*xManualDevelopmentEntry, *Engine) {
	t.Helper()
	opts, cfg := xDevelopmentOptions(t)
	selection, err := PrepareXManualDevelopmentSynthetic(opts)
	if err != nil {
		t.Fatal(err)
	}
	v := selection.(*xManualDevelopmentEntry)
	state, err := v.OpenDatabase(domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", false))
	if err != nil {
		t.Fatal(err)
	}
	e := New(state, reasoning.Deterministic{}, cfg, log.New(io.Discard, "", 0))
	if err = v.Attach(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		e.Shutdown()
		if !e.WaitForIdle(3 * time.Second) {
			t.Error("synthetic reasoning did not drain")
		}
		if err := state.Close(); err != nil {
			t.Error("database close failed", err)
		}
		if err := v.Close(); err != nil {
			t.Error(err)
		}
		for _, pin := range v.selection.pins {
			if _, err := pin.Stat(); !errors.Is(err, os.ErrClosed) && !errors.Is(err, windows.ERROR_INVALID_HANDLE) {
				t.Error("selection pin still open", pin.Name(), err)
			}
		}
		if err := os.Rename(state.Path(), state.Path()+".released"); err != nil {
			t.Error("closed synthetic database still locked", err)
		}
	})
	return v, e
}

func TestXDevelopmentEntryDefaultOffAndEarlyRejection(t *testing.T) {
	if v, err := PrepareXManualDevelopment(config.Options{}); err != nil || v != nil {
		t.Fatal("ordinary startup changed", err)
	}
	for _, name := range []string{"nondev", "missing_selector", "missing_plan", "outside_db", "outside_config", "wrong_port", "qa_mix", "browser_override", "ui_unpinned", "missing_config"} {
		t.Run(name, func(t *testing.T) {
			opts, _ := xDevelopmentOptions(t)
			db := opts.DatabasePath
			switch name {
			case "nondev":
				opts.Dev = false
			case "missing_selector":
				opts.DevelopmentSessionReuse = ""
			case "missing_plan":
				opts.DevelopmentXManualPlan = ""
			case "outside_db":
				opts.DatabasePath = filepath.Join(t.TempDir(), "user.db")
			case "outside_config":
				opts.ConfigPath = filepath.Join(t.TempDir(), "user.json")
			case "wrong_port":
				opts.Port++
			case "qa_mix":
				opts.DevelopmentQAPilotPlan = opts.DevelopmentXManualPlan
			case "browser_override":
				opts.BrowserProfilePath = t.TempDir()
			case "ui_unpinned":
				opts.AppShell = true
			case "missing_config":
				if err := os.Remove(opts.ConfigPath); err != nil {
					t.Fatal(err)
				}
			}
			v, err := PrepareXManualDevelopment(opts)
			if err == nil {
				v.Close()
				t.Fatal("invalid selection accepted")
			}
			if _, err = os.Stat(db); !os.IsNotExist(err) {
				t.Fatal("rejected selection created database")
			}
		})
	}
}

func TestXDevelopmentEntryConfigScopeAndUIProfile(t *testing.T) {
	opts, cfg := xDevelopmentOptions(t)
	var plan XApplicationQAPlan
	raw, _ := os.ReadFile(opts.DevelopmentXManualPlan)
	json.Unmarshal(raw, &plan)
	opts.AppShell = true
	opts.ChromiumPath = plan.Chrome.Path
	selection, err := PrepareXManualDevelopment(opts)
	if err != nil {
		t.Fatal(err)
	}
	defer selection.Close()
	v := selection.(*xManualDevelopmentEntry)
	if v.uiLease == nil || v.UIExtensionPath() != filepath.Join(cfg.Root, "ui-reader-broker") {
		t.Fatal("UI profile/broker not isolated")
	}
	if err = v.ValidateConfig(cfg); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"root", "database", "host", "port", "split", "production"} {
		bad := cfg
		switch name {
		case "root":
			bad.Root = filepath.Dir(cfg.Root)
		case "database":
			bad.Database.Path = filepath.Join(t.TempDir(), "user.db")
		case "host":
			bad.Server.Host = "localhost"
		case "port":
			bad.Server.Port++
		case "split":
			bad.WindowsCaptureSplit = true
		case "production":
			bad.Deployment.Mode = "production-installed-app"
		}
		if v.ValidateConfig(bad) == nil {
			t.Fatal("loaded config escaped selection", name)
		}
	}
}

func TestXDevelopmentEntryConsentCaptureReaderAndStop(t *testing.T) {
	v, e := xDevelopmentEngine(t)
	before, _ := e.store.GetSettings(context.Background())
	var calls atomic.Int32
	factory := v.factory
	v.factory = func(e *Engine, s *xQASelection) xManualApplicationFactory { calls.Add(1); return factory(e, s) }
	if s := e.XManualStatus(); s.State != "disabled" || !s.Available || calls.Load() != 0 {
		t.Fatal("attachment started owner", s)
	}
	for _, request := range []XManualStartRequest{{}, {AcknowledgeLimitations: true}, {AllowOneSessionDerivation: true}} {
		if e.StartXManual(context.Background(), request) == nil {
			t.Fatal("missing consent accepted")
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid consent reached factory")
	}
	if err := e.StartXManual(context.Background(), XManualStartRequest{true, true}); err != nil {
		t.Fatal(err)
	}
	if e.StartXManual(context.Background(), XManualStartRequest{true, true}) == nil || calls.Load() != 1 {
		t.Fatal("start replay reached factory")
	}
	e.xManual.mu.RLock()
	r := e.xManual.route.(*xManualApplication)
	e.xManual.mu.RUnlock()
	if release, allowed := e.ordinarySourceRoute(domain.SourceX); allowed {
		release()
		t.Fatal("ordinary X admitted during pilot")
	}
	if release, allowed := e.ordinarySourceRoute(domain.SourceLinkedIn); !allowed {
		t.Fatal("non-X route changed")
	} else {
		release()
	}
	req := readerbroker.Request{RequestID: "broker_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Source: "x", URL: "https://x.com/example/status/123"}
	prepared, selected, err := e.PrepareXManualReader(context.Background(), req, "/split-reader-intent?id=split_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err != nil || !selected {
		t.Fatal("reader did not use pilot", err)
	}
	if err = prepared.VerifyForeground(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err = prepared.NavigatePost(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-r.captureDone:
	case <-time.After(3 * time.Second):
		t.Fatal("capture stalled")
	}
	r.mu.Lock()
	run := r.captureResult.Run.ID
	r.mu.Unlock()
	observations, err := e.store.Observations(context.Background(), run)
	if err != nil || len(observations) != 1 {
		t.Fatal("synthetic observation missing", err)
	}
	// Completed-flow acceptance is separate from the driver cancellation suite.
	// Let deterministic reasoning retire naturally before initiating shutdown.
	if !e.WaitForIdle(3 * time.Second) {
		t.Fatal("completed capture reasoning did not drain")
	}
	if err = e.StopXManual(context.Background()); err != nil {
		t.Fatal(err)
	}
	if s := e.XManualStatus(); s.Cleanup != "verified" || s.State != "stopped" {
		t.Fatal("stop not verified", s)
	}
	after, _ := e.store.GetSettings(context.Background())
	if !reflect.DeepEqual(before, after) {
		t.Fatal("pilot changed saved settings/Auto Update")
	}
}

func TestXDevelopmentEntryStopDuringStartup(t *testing.T) {
	v, e := xDevelopmentEngine(t)
	entered := make(chan struct{})
	var disposed atomic.Int32
	v.factory = func(*Engine, *xQASelection) xManualApplicationFactory {
		return func(ctx, parent context.Context) (xManualApplicationPorts, error) {
			close(entered)
			<-ctx.Done()
			return xManualApplicationPorts{dispose: func(context.Context) error { disposed.Add(1); return nil }}, errors.New("synthetic canceled factory")
		}
	}
	started := make(chan error, 1)
	go func() { started <- e.StartXManual(context.Background(), XManualStartRequest{true, true}) }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("factory not reached")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := e.StopXManual(ctx); err != nil {
		t.Fatal(err)
	}
	if <-started == nil || disposed.Load() != 1 {
		t.Fatal("canceled startup did not dispose once")
	}
	if e.StartXManual(context.Background(), XManualStartRequest{true, true}) == nil {
		t.Fatal("stop allowed restart")
	}
	if release, allowed := e.ordinarySourceRoute(domain.SourceX); !allowed {
		t.Fatal("verified cleanup kept X occupied")
	} else {
		release()
	}
}

func TestXDevelopmentEntryShutdownDrainsStartupBeforeSelectionClose(t *testing.T) {
	v, e := xDevelopmentEngine(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	original := v.factory
	v.factory = func(e *Engine, s *xQASelection) xManualApplicationFactory {
		close(entered)
		<-release
		return original(e, s)
	}
	started := make(chan error, 1)
	go func() { started <- e.StartXManual(context.Background(), XManualStartRequest{true, true}) }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("synthetic startup not reached")
	}
	e.Shutdown()
	if e.WaitForIdle(20 * time.Millisecond) {
		t.Fatal("shutdown reported idle before startup cleanup")
	}
	closed := make(chan error, 1)
	go func() { closed <- v.Close() }()
	deadline := time.Now().Add(3 * time.Second)
	for {
		v.mu.Lock()
		closing := v.closed
		v.mu.Unlock()
		if closing {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("selection close not reached")
		}
		time.Sleep(time.Millisecond)
	}
	select {
	case <-closed:
		t.Fatal("selection released pins before startup cleanup")
	default:
	}
	if _, err := e.store.GetSettings(context.Background()); err != nil {
		t.Fatal("database unavailable during startup cleanup", err)
	}
	unblock()
	select {
	case err := <-started:
		if err == nil {
			t.Fatal("shutdown allowed startup")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("startup cleanup did not finish")
	}
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("selection close did not finish")
	}
	if !e.WaitForIdle(time.Second) {
		t.Fatal("completed startup cleanup still marked busy")
	}
}

func TestXDevelopmentEntryParentLossAndFirstReason(t *testing.T) {
	v, e := xDevelopmentEngine(t)
	parent, cancel := context.WithCancel(context.Background())
	cancel()
	v.mu.Lock()
	v.parent = parent
	v.mu.Unlock()
	var calls atomic.Int32
	v.factory = func(*Engine, *xQASelection) xManualApplicationFactory { calls.Add(1); return nil }
	if e.StartXManual(context.Background(), XManualStartRequest{true, true}) == nil || calls.Load() != 0 {
		t.Fatal("dead parent started owner")
	}
	v.failStart("settings-unavailable")
	v.failStart("later-cleanup")
	if v.status().Reason != "settings-unavailable" {
		t.Fatal("first cause overwritten")
	}
	v.stop()
	if v.status().Reason != "settings-unavailable" {
		t.Fatal("stop overwrote first cause")
	}
}

func TestXDevelopmentEntryUncertainCloseCannotBecomeSuccess(t *testing.T) {
	opts, cfg := xDevelopmentOptions(t)
	selection, err := PrepareXManualDevelopmentSynthetic(opts)
	if err != nil {
		t.Fatal(err)
	}
	v := selection.(*xManualDevelopmentEntry)
	state, err := v.OpenDatabase(domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", false))
	if err != nil {
		t.Fatal(err)
	}
	e := New(state, reasoning.Deterministic{}, cfg, log.New(io.Discard, "", 0))
	if err = v.Attach(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	// All resources here are invented/accountless. Explicit test teardown releases
	// the retained fixture pins after asserting the intentionally uncertain path.
	t.Cleanup(func() { v.fixtureClose(); state.Close(); v.selection.close() })
	original := v.factory
	v.factory = func(e *Engine, s *xQASelection) xManualApplicationFactory {
		build := original(e, s)
		return func(ctx, parent context.Context) (xManualApplicationPorts, error) {
			p, err := build(ctx, parent)
			p.dispose = func(context.Context) error { return errors.New("synthetic failed disposal") }
			return p, err
		}
	}
	if err = e.StartXManual(context.Background(), XManualStartRequest{true, true}); err != nil {
		t.Fatal(err)
	}
	_ = e.StopXManual(context.Background())
	if v.Close() == nil || v.Close() == nil {
		t.Fatal("uncertain cleanup became success on repeated close")
	}
	if s := e.XManualStatus(); s.Cleanup != "uncertain" {
		t.Fatal("uncertainty not retained", s)
	}
	if release, allowed := e.ordinarySourceRoute(domain.SourceX); allowed {
		release()
		t.Fatal("uncertain selection released X admission")
	}
}
