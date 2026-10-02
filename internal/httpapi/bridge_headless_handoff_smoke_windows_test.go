//go:build windows

package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"github.com/abangkis/AkuSidecar/internal/appshell"
	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/collection"
	"github.com/abangkis/AkuSidecar/internal/collection/headless"
	"github.com/abangkis/AkuSidecar/internal/collection/quiet"
	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/engine"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/AkuSidecar/internal/store"
	"golang.org/x/sys/windows"
)

// Explicit opt-in E2E fixture. It uses a disposable profile and the exact
// product loopback port; it never borrows or stops an existing Sidecar process.
func TestBridgeHeadlessHandoffWindowsSmoke(t *testing.T) {
	runtimeRoot := os.Getenv("AKU_BRIDGE_HANDOFF_SMOKE_RUNTIME")
	chromePath := os.Getenv("AKU_BRIDGE_HANDOFF_SMOKE_CHROME")
	bridgePath := os.Getenv("AKU_BRIDGE_HANDOFF_SMOKE_BRIDGE")
	if runtimeRoot == "" || chromePath == "" || bridgePath == "" {
		t.Skip("explicit staged Node runtime, Chrome, and packaged Bridge paths required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	for _, path := range []*string{&runtimeRoot, &chromePath, &bridgePath} {
		absolute, err := filepath.Abs(*path)
		if err != nil {
			t.Fatal("resolve staged runtime path")
		}
		*path = absolute
	}
	origin, err := bridgeExtensionOrigin(bridgePath)
	if err != nil {
		t.Fatal("read packaged Bridge identity")
	}

	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve project-local smoke directory")
	}
	sidecarRoot := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", ".."))
	artifactRoot := filepath.Join(sidecarRoot, "build", "bridge-handoff-smoke")
	if err := os.MkdirAll(artifactRoot, 0700); err != nil {
		t.Fatal("create project-local smoke directory")
	}
	workDir, err := os.MkdirTemp(artifactRoot, "bridge-headless-handoff-")
	if err != nil {
		t.Fatal("create disposable smoke workspace")
	}
	// Cleanup runs after later-registered manager/server cleanup, so the profile
	// cannot be removed while Chrome still owns it.
	t.Cleanup(func() { _ = os.RemoveAll(workDir) })
	profile := filepath.Join(workDir, "capture-profile")
	if err := os.Mkdir(profile, 0700); err != nil {
		t.Fatal("create disposable Chrome profile")
	}
	options := headless.Options{
		Node: filepath.Join(runtimeRoot, "node.exe"), Worker: filepath.Join(runtimeRoot, "worker.mjs"),
		Pin: filepath.Join(runtimeRoot, "node.pin.json"), Chrome: chromePath,
		Profile: profile, BridgePath: bridgePath,
	}
	if os.Getenv("AKU_BRIDGE_HANDOFF_SMOKE_SOURCE_WORKER") == "1" {
		options.Worker = filepath.Join(sidecarRoot, "internal", "collection", "headless", "worker", "worker.mjs")
	}
	if err := headless.Validate(options); err != nil {
		t.Fatal("staged headless runtime is invalid")
	}

	// Check before constructing or launching Chrome. Server.Start repeats the
	// bind and is also required to succeed before any browser process is created.
	probe, err := net.Listen("tcp", "127.0.0.1:11122")
	if err != nil {
		t.Fatal("127.0.0.1:11122 is occupied; refusing to launch Chrome")
	}
	_ = probe.Close()

	state, err := store.Open(filepath.Join(t.TempDir(), "bridge-handoff.db"), domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", true))
	if err != nil {
		t.Fatal("open isolated smoke database")
	}
	cfg := config.Config{
		Server:              config.ServerConfig{Host: "127.0.0.1", Port: 11122},
		Bridge:              config.BridgeConfig{TrustedExtensionOrigins: []string{origin}},
		WindowsCaptureSplit: true,
	}
	logger := log.New(io.Discard, "", 0)
	e := engine.New(state, reasoning.Deterministic{}, cfg, logger)
	s, err := New(cfg, state, e, logger)
	if err != nil {
		_ = state.Close()
		t.Fatal("create isolated smoke server")
	}
	if err := s.RequireHostOnlyRetirement(); err != nil {
		t.Fatal(err)
	}
	var observed bridgeHandoffHTTPObservation
	const interactiveMarker = "AkuBrowser reader split_fixture_lifetime"
	originalHandler := s.http.Handler
	s.http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/quiet-interactive-fixture" {
			w.Header().Set("Content-Type", "text/html")
			_, _ = io.WriteString(w, "<!doctype html><title>"+interactiveMarker+"</title><p>Disposable interactive lifetime fixture</p>")
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/split-capture-host" {
			observed.hostPageHits.Add(1)
		}
		if r.Method == http.MethodPost && r.URL.Path == "/api/bridge/split-capture/bootstrap" {
			observed.bootstrapRequests.Add(1)
			recorder := &bridgeHandoffStatusWriter{ResponseWriter: w}
			originalHandler.ServeHTTP(recorder, r)
			observed.bootstrapStatus.Store(int32(recorder.code()))
			return
		}
		originalHandler.ServeHTTP(w, r)
	})
	cleanup := func(manager *captureruntime.Manager) {
		if manager != nil {
			manager.Terminate()
		}
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer stopCancel()
		_ = s.Stop(stopCtx)
		_ = state.Close()
	}
	var manager *captureruntime.Manager
	t.Cleanup(func() { cleanup(manager) })
	address, err := s.Start()
	if err != nil {
		t.Fatal("bind 127.0.0.1:11122 before Chrome launch")
	}
	if address.String() != "127.0.0.1:11122" {
		t.Fatal("smoke server did not bind the product loopback address")
	}
	preflight, err := (&http.Client{Timeout: 3 * time.Second}).Get("http://127.0.0.1:11122/split-capture-host")
	if err != nil {
		t.Fatal("capture-host HTTP preflight failed")
	}
	preflightBody, readErr := io.ReadAll(io.LimitReader(preflight.Body, 16*1024))
	_ = preflight.Body.Close()
	if readErr != nil || preflight.StatusCode != http.StatusOK ||
		!strings.Contains(string(preflightBody), "<title>AkuBrowser capture host</title>") {
		t.Fatal("capture-host HTTP preflight did not return the expected page")
	}
	hostHitsAfterPreflight := observed.hostPageHits.Load()
	launchURL, err := s.SplitCaptureLaunchURL("http://127.0.0.1:11122")
	if err != nil {
		t.Fatal("create authenticated capture-host URL")
	}
	window, err := appshell.Launch(ctx, appshell.LaunchOptions{
		Executable: chromePath, ExtensionPath: bridgePath, UserDataDir: profile,
		URL: launchURL, StartMinimized: true, PrivateCDP: true,
		ExtraArgs: []string{"--start-minimized"},
	})
	if err != nil {
		t.Fatal("launch disposable capture Chrome")
	}
	containment, err := window.StartCaptureContainment(logger)
	if err != nil {
		window.Terminate()
		t.Fatal("start capture containment")
	}
	sourceLifetime, ok := containment.(interface {
		PrepareSourceWindowLifetime(context.Context, string) error
	})
	if !ok {
		window.Terminate()
		t.Fatal("capture containment cannot track source windows")
	}
	s.SetSplitSourceWindowPreparation(sourceLifetime.PrepareSourceWindowLifetime)
	manager, err = captureruntime.New(window)
	if err != nil {
		window.Terminate()
		t.Fatal("create headed capture owner")
	}
	if err := e.AttachCaptureRuntime(ctx, manager); err != nil {
		t.Fatal("attach capture owner to engine")
	}
	if err := s.SetSplitCaptureRuntime(manager); err != nil {
		t.Fatal("attach capture owner to split server")
	}
	var coordinator *collection.Coordinator
	var borrowedWindow *appshell.Window
	var borrowedContainment appshell.CaptureContainment
	var borrowedHostURL string
	var borrowedMachineTargets *bridgeFixtureTargetRecorder
	bindQuiet := func(nextCtx context.Context, headed *appshell.Window, generation uint64) error {
		recorder := &bridgeFixtureTargetRecorder{protocol: headed.CaptureProtocol(), created: map[string]bool{}}
		targets, err := quiet.NewTargets(nextCtx, recorder)
		if err != nil {
			if targets != nil {
				_ = targets.Close(nextCtx)
			}
			return err
		}
		worker := quiet.NewWorker(targets, options)
		// Keep one actual hidden target alive so retirement must dispose it.
		if _, err := targets.Call(nextCtx, domain.SourceX, "Page.navigate", map[string]any{"url": "http://127.0.0.1:11122/quiet-hidden-fixture"}); err != nil {
			_ = worker.Retire(nextCtx)
			return err
		}
		if err := headed.SetCaptureHandoff(func(closeCtx context.Context) error {
			if err := worker.Retire(closeCtx); err != nil {
				return err
			}
			return s.CloseSplitCaptureHost(closeCtx)
		}); err != nil {
			_ = worker.Retire(nextCtx)
			return err
		}
		if coordinator != nil {
			coordinator.SetBrowserCollector(generation, worker)
			borrowedMachineTargets = recorder
		}
		return nil
	}
	if err := bindQuiet(ctx, window, manager.Snapshot().Generation); err != nil {
		t.Fatal(err)
	}
	protocol := window.CaptureProtocol()
	if protocol == nil {
		t.Fatal("managed capture private protocol unavailable")
	}
	if err := initializeBridgeSmokeCaptureHost(ctx, protocol, launchURL); err != nil {
		t.Fatal(err)
	}
	if err := manager.SetTransitionReadiness(func(checkCtx context.Context) error {
		if manager.Snapshot().Driver == "headless" {
			return checkCtx.Err()
		}
		return s.SplitCaptureReplacementReadiness(checkCtx)
	}); err != nil {
		t.Fatal("bind authenticated transport readiness")
	}
	launch := func(nextCtx context.Context, mode string, generation uint64) (captureruntime.Process, error) {
		if err := s.RotateSplitCapture(); err != nil {
			return nil, err
		}
		if mode == "headless" {
			return headless.Launch(nextCtx, options)
		}
		if mode != "browser" {
			return nil, errors.New("unexpected smoke runtime")
		}
		url, err := s.SplitCaptureLaunchURL("http://127.0.0.1:11122")
		if err != nil {
			return nil, err
		}
		headed, err := appshell.Launch(nextCtx, appshell.LaunchOptions{Executable: chromePath, ExtensionPath: bridgePath, UserDataDir: profile, URL: url, StartMinimized: true, PrivateCDP: true, ExtraArgs: []string{"--start-minimized"}})
		if err != nil {
			return headed, err
		}
		if err := bindQuiet(nextCtx, headed, generation); err != nil {
			return headed, err
		}
		containment, err := headed.StartCaptureContainment(logger)
		if err != nil {
			return headed, err
		}
		s.SetSplitReaderPreparation(containment.PrepareReader)
		s.SetSplitReaderBroker(containment.PrepareBrokerReader)
		source, ok := containment.(interface {
			PrepareSourceWindowLifetime(context.Context, string) error
		})
		if !ok {
			return headed, errors.New("source-window containment missing")
		}
		s.SetSplitSourceWindowPreparation(source.PrepareSourceWindowLifetime)
		borrowedWindow, borrowedContainment, borrowedHostURL = headed, containment, url
		if err := appshell.NavigateCaptureHost(nextCtx, headed.CaptureProtocol(), url); err != nil {
			return headed, err
		}
		tick := time.NewTicker(50 * time.Millisecond)
		defer tick.Stop()
		for {
			if s.SplitCaptureReplacementReadiness(nextCtx) == nil {
				return headed, nil
			}
			select {
			case <-nextCtx.Done():
				return headed, nextCtx.Err()
			case <-tick.C:
			}
		}
	}
	coordinator = collection.NewCoordinator(manager, launch, func() error { return headless.Validate(options) })
	e.AttachCollectionCoordinator(coordinator)
	coordinator.Request("headless")

	capabilityCtx, stopCapabilityWait := context.WithTimeout(ctx, 48*time.Second)
	capabilityErr := waitForBridgeHandoffCapability(capabilityCtx, s)
	stopCapabilityWait()
	if capabilityErr != nil {
		diagnosticCtx, stopDiagnostic := context.WithTimeout(ctx, 8*time.Second)
		diagnostic := bridgeHandoffOwnedStartupDiagnostic(
			diagnosticCtx, protocol, launchURL, origin,
			observed.hostPageHits.Load()-hostHitsAfterPreflight,
			observed.bootstrapRequests.Load(), observed.bootstrapStatus.Load(),
			s,
		)
		stopDiagnostic()
		t.Fatalf("capture host handoff capability bootstrap timed out (%s)", diagnostic)
	}
	closeAcknowledgedBeforeRotation := false
	t.Log("fixture_window_baseline_before_headless=true")
	baselineTarget, err := createBridgeFixtureWindow(ctx, protocol, origin, t)
	if err != nil {
		t.Fatal("pre-headless single-window baseline", err)
	}
	closedBaseline, err := protocol.Call(ctx, "Target.closeTarget", map[string]any{"targetId": baselineTarget}, "")
	var baselineClosed struct {
		Success bool `json:"success"`
	}
	if err != nil || json.Unmarshal(closedBaseline, &baselineClosed) != nil || !baselineClosed.Success {
		t.Fatal("could not close exact single-tab baseline fixture")
	}
	if err := manager.Replace(ctx, func(nextCtx context.Context, generation uint64) (captureruntime.Process, error) {
		// RotateSplitCapture starts the next Bridge generation and clears the
		// previous generation's action ledger. Snapshot the real ACK first.
		s.splitCapture.mu.Lock()
		for _, action := range s.splitCapture.actions {
			if action.action.Type == "close_capture_host" && action.claimed && action.completed &&
				action.completionResult != nil && action.completionResult.OK {
				closeAcknowledgedBeforeRotation = true
				break
			}
		}
		s.splitCapture.mu.Unlock()
		return launch(nextCtx, "headless", generation)
	}); err != nil {
		t.Fatal("real Bridge scoped retirement and profile handoff failed", err)
	}
	if !closeAcknowledgedBeforeRotation {
		t.Fatal("real Bridge close action was not claimed and acknowledged successfully")
	}
	// Manager already consumes the single Done result. CloseForRetry is the
	// repeatable ownership readback and returns only after verified Job drain.
	if err := window.CloseForRetry(ctx); err != nil {
		t.Fatal("headed Chrome ownership readback failed after replacement")
	}
	t.Log("bridge_bootstrap=true close_ack=true natural_owner_drain=true")
	preferences, prefsErr := os.ReadFile(filepath.Join(profile, "Default", "Preferences"))
	var prefs struct {
		Session struct {
			RestoreOnStartup *int `json:"restore_on_startup"`
		} `json:"session"`
	}
	prefsValid := prefsErr == nil && json.Unmarshal(preferences, &prefs) == nil
	restoreValue := -1
	if prefsValid && prefs.Session.RestoreOnStartup != nil {
		restoreValue = *prefs.Session.RestoreOnStartup
	}
	localState, localErr := os.ReadFile(filepath.Join(profile, "Local State"))
	var local struct {
		WasRestarted bool `json:"was_restarted"`
	}
	localValid := localErr == nil && json.Unmarshal(localState, &local) == nil
	t.Logf("fixture_restore_pref_json_valid=%t restore_on_startup=%d local_state_json_valid=%t was_restarted=%t", prefsValid, restoreValue, localValid, local.WasRestarted)
	status := coordinator.Status()
	if status.Effective != "headless" || status.Pending || status.Generation != 2 || status.State != captureruntime.Ready {
		t.Fatal("headless generation 2 was not ready after verified profile handoff")
	}
	// This static lifecycle fixture has no source permissions or social capture.
	// Permission admission is separately tested; bypass only that gate here.
	coordinator.SetHeadlessReadiness(func() error { return nil })
	coordinator.Start(ctx)
	lease, releaseIntent, err := coordinator.BorrowBrowser(ctx)
	if err != nil {
		t.Fatal("automatic borrow of real browser owner failed", err)
	}
	if lease.Driver() != "browser" || lease.Generation() != 3 || !coordinator.BrowserCollectorAvailable(domain.SourceX) {
		t.Fatal("browser generation 3 collector not ready")
	}
	// An ordinary source window stays outside the machine-target ownership list.
	// Its real HWND lifetime must hold auto-return after the dispatch lease ends.
	beforeRaw, err := borrowedWindow.CaptureProtocol().Call(ctx, "Target.getTargets", nil, "")
	var before struct {
		TargetInfos []struct {
			TargetID string `json:"targetId"`
		} `json:"targetInfos"`
	}
	if err != nil || json.Unmarshal(beforeRaw, &before) != nil {
		t.Fatal("pre-window fixture inventory unavailable")
	}
	beforeIDs := map[string]bool{}
	for _, info := range before.TargetInfos {
		beforeIDs[info.TargetID] = true
	}
	interactiveID, err := createBridgeFixtureWindow(ctx, borrowedWindow.CaptureProtocol(), origin, t)
	if err != nil {
		t.Fatal("create isolated Bridge fixture window", err)
	}
	interactive := struct{ TargetID string }{interactiveID}
	windowInfo, err := borrowedWindow.CaptureProtocol().Call(ctx, "Browser.getWindowForTarget", map[string]any{"targetId": interactive.TargetID}, "")
	if err != nil {
		t.Fatal("static interactive window unavailable", err)
	}
	var nativeWindow struct {
		WindowID int `json:"windowId"`
	}
	if json.Unmarshal(windowInfo, &nativeWindow) != nil {
		t.Fatal("static interactive window response invalid")
	}
	listing, err := borrowedWindow.CaptureProtocol().Call(ctx, "Target.getTargets", nil, "")
	if err != nil {
		t.Fatal("inspect disposable host identity", err)
	}
	var listed struct {
		TargetInfos []struct {
			TargetID string `json:"targetId"`
			URL      string `json:"url"`
			Type     string `json:"type"`
		} `json:"targetInfos"`
	}
	if json.Unmarshal(listing, &listed) != nil {
		t.Fatal("invalid disposable target listing")
	}
	hostMatches := 0
	for _, info := range listed.TargetInfos {
		if info.Type == "page" && info.TargetID != interactive.TargetID {
			otherInfo, otherErr := borrowedWindow.CaptureProtocol().Call(ctx, "Browser.getWindowForTarget", map[string]any{"targetId": info.TargetID}, "")
			var otherWindow struct {
				WindowID int `json:"windowId"`
			}
			if otherErr == nil && json.Unmarshal(otherInfo, &otherWindow) == nil && otherWindow.WindowID == nativeWindow.WindowID {
				t.Fatalf("fixture window contains another page; refusing native window close (other_is_host=%t other_is_blank=%t other_is_fixture=%t other_is_created_hidden=%t other_existed_before_window=%t)", info.URL == borrowedHostURL, info.URL == "about:blank", info.URL == "http://127.0.0.1:11122/quiet-interactive-fixture", borrowedMachineTargets.owns(info.TargetID), beforeIDs[info.TargetID])
			}
		}
		if info.URL != borrowedHostURL {
			continue
		}
		hostMatches++
		hostInfo, err := borrowedWindow.CaptureProtocol().Call(ctx, "Browser.getWindowForTarget", map[string]any{"targetId": info.TargetID}, "")
		var hostWindow struct {
			WindowID int `json:"windowId"`
		}
		if err != nil || json.Unmarshal(hostInfo, &hostWindow) != nil {
			t.Fatal("inspect disposable host window")
		}
		if hostWindow.WindowID == nativeWindow.WindowID {
			t.Fatal("fixture target shares static host window; refusing native window close")
		}
	}
	if hostMatches != 1 {
		t.Fatal("static host window identity is ambiguous")
	}
	if _, err := borrowedWindow.CaptureProtocol().Call(ctx, "Browser.setWindowBounds", map[string]any{"windowId": nativeWindow.WindowID, "bounds": map[string]any{"windowState": "minimized"}}, ""); err != nil {
		t.Fatal(err)
	}
	readerTarget, _, err := borrowedContainment.PrepareBrokerReader(ctx, interactiveMarker)
	if err != nil {
		t.Fatal("track static reader HWND", err)
	}
	lease.Release()
	releaseIntent()
	guardDeadline := time.NewTimer(3 * time.Second)
	guardTick := time.NewTicker(25 * time.Millisecond)
	guardExercised := false
	for !guardExercised {
		guardExercised = strings.Contains(coordinator.Status().Failure, "native interactive window is still open")
		if guardExercised {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-guardDeadline.C:
			t.Fatal("native interactive guard was not exercised")
		case <-guardTick.C:
		}
	}
	guardDeadline.Stop()
	guardTick.Stop()
	if status := coordinator.Status(); status.Effective != "browser" || status.Generation != 3 {
		t.Fatal("auto-return crossed a live interactive HWND", status)
	}
	if err := closeBridgeFixtureReader(ctx, readerTarget); err != nil {
		t.Fatal("close only static fixture window", err)
	}
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		status := coordinator.Status()
		if status.Effective == "headless" && status.Generation == 4 && !status.Pending && status.State == captureruntime.Ready {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("auto-return did not reach headless generation 4", status.Failure)
		case <-tick.C:
		}
	}
	t.Log("host_only_negotiated=true hidden_retirement=true browser_borrow_generation=3 interactive_hwnd_retained=true auto_return_after_window_close=true auto_return_headless_generation=4 source_permission_gate_fixture_only=true")
}

// Record only successful machine-target creation responses, never adopt IDs
// from inventories. This fixture recorder does not change production routing.
type bridgeFixtureTargetRecorder struct {
	protocol appshell.CaptureProtocol
	mu       sync.Mutex
	created  map[string]bool
}

func (r *bridgeFixtureTargetRecorder) Call(ctx context.Context, method string, params any, session string) (json.RawMessage, error) {
	raw, err := r.protocol.Call(ctx, method, params, session)
	if err == nil && method == "Target.createTarget" {
		values, ok := params.(map[string]any)
		var created struct {
			TargetID string `json:"targetId"`
		}
		if ok && values["hidden"] == true && json.Unmarshal(raw, &created) == nil && created.TargetID != "" {
			r.mu.Lock()
			r.created[created.TargetID] = true
			r.mu.Unlock()
		}
	}
	return raw, err
}
func (r *bridgeFixtureTargetRecorder) owns(id string) bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.created[id]
}

// Fixture-only user-close equivalent. The real containment supplies the unique
// owned HWND binding; revalidate PID/property/expiry immediately before WM_CLOSE.
func closeBridgeFixtureReader(ctx context.Context, target readerbroker.Target) error {
	user32 := windows.NewLazySystemDLL("user32.dll")
	property, err := windows.UTF16PtrFromString(target.Property)
	if err != nil || target.HWND == 0 || target.PID == 0 || target.Value == 0 || !strings.HasPrefix(target.Property, "AkuBrowser.ExplicitReader.") {
		return errors.New("invalid fixture reader binding")
	}
	var pid uint32
	user32.NewProc("GetWindowThreadProcessId").Call(uintptr(target.HWND), uintptr(unsafe.Pointer(&pid)))
	value, _, _ := user32.NewProc("GetPropW").Call(uintptr(target.HWND), uintptr(unsafe.Pointer(property)))
	if ctx.Err() != nil || pid != target.PID || value != uintptr(target.Value) || time.Now().After(target.Expires) {
		return errors.New("fixture reader binding expired or changed")
	}
	ok, _, _ := user32.NewProc("PostMessageW").Call(uintptr(target.HWND), 0x0010, 0, 0)
	if ok == 0 {
		return errors.New("fixture window close was rejected")
	}
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(25 * time.Millisecond)
	defer tick.Stop()
	for {
		exists, _, _ := user32.NewProc("IsWindow").Call(uintptr(target.HWND))
		if exists == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return errors.New("fixture HWND did not close")
		case <-tick.C:
		}
	}
}

// Use the same windows API as Bridge source/reader actions. CDP createTarget
// in app mode can leave an additional page in the new window.
func createBridgeFixtureWindow(ctx context.Context, protocol appshell.CaptureProtocol, origin string, t *testing.T) (string, error) {
	const fixtureURL = "http://127.0.0.1:11122/quiet-interactive-fixture"
	list := func() ([]struct {
		TargetID string `json:"targetId"`
		Type     string `json:"type"`
		URL      string `json:"url"`
	}, error) {
		raw, err := protocol.Call(ctx, "Target.getTargets", nil, "")
		var result struct {
			TargetInfos []struct {
				TargetID string `json:"targetId"`
				Type     string `json:"type"`
				URL      string `json:"url"`
			} `json:"targetInfos"`
		}
		if err != nil || json.Unmarshal(raw, &result) != nil {
			return nil, errors.New("fixture target inventory unavailable")
		}
		return result.TargetInfos, nil
	}
	infos, err := list()
	if err != nil {
		return "", err
	}
	worker := ""
	for _, info := range infos {
		if info.Type == "service_worker" && strings.HasPrefix(info.URL, origin+"/") {
			if worker != "" {
				return "", errors.New("Bridge worker identity ambiguous")
			}
			worker = info.TargetID
		}
	}
	if worker == "" {
		return "", errors.New("exact Bridge worker unavailable")
	}
	attached, err := protocol.Call(ctx, "Target.attachToTarget", map[string]any{"targetId": worker, "flatten": true}, "")
	var session struct {
		SessionID string `json:"sessionId"`
	}
	if err != nil || json.Unmarshal(attached, &session) != nil || session.SessionID == "" {
		return "", errors.New("fixture Bridge worker attachment failed")
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_, _ = protocol.Call(cleanup, "Target.detachFromTarget", map[string]any{"sessionId": session.SessionID}, "")
	}()
	raw, err := protocol.Call(ctx, "Runtime.evaluate", map[string]any{"expression": `chrome.windows.create({url:"` + fixtureURL + `",type:"normal",focused:false,state:"minimized"}).then(async w=>{const actual=await chrome.windows.get(w.id,{populate:true});const tabs=actual.tabs??[];const category=t=>{const value=t.url||t.pendingUrl;if(!value)return "url_unavailable";if(value==="` + fixtureURL + `")return "fixture";if(value==="about:blank")return "blank";try{const u=new URL(value);if(u.protocol==="chrome:")return "chrome_internal";if(u.protocol==="chrome-extension:")return "extension";if(u.hostname==="127.0.0.1"||u.hostname==="localhost")return "other_loopback";if(u.hostname==="x.com"||u.hostname==="www.facebook.com")return "source";return "other";}catch{return "invalid";}};return {ok:true,createdTabCount:(w.tabs??[]).length,tabCount:tabs.length,fixtureCount:tabs.filter(t=>t.url==="` + fixtureURL + `"||t.pendingUrl==="` + fixtureURL + `").length,blankCount:tabs.filter(t=>t.url==="about:blank").length,categories:tabs.map(category)};})`, "awaitPromise": true, "returnByValue": true}, session.SessionID)
	var evaluated struct {
		Result struct {
			Value struct {
				OK              bool     `json:"ok"`
				TabCount        int      `json:"tabCount"`
				FixtureCount    int      `json:"fixtureCount"`
				BlankCount      int      `json:"blankCount"`
				CreatedTabCount int      `json:"createdTabCount"`
				Categories      []string `json:"categories"`
			} `json:"value"`
		} `json:"result"`
	}
	if err != nil || json.Unmarshal(raw, &evaluated) != nil || !evaluated.Result.Value.OK {
		return "", errors.New("Bridge fixture window creation rejected")
	}
	t.Logf("fixture_chrome_tabs=%d fixture_url_tabs=%d fixture_blank_tabs=%d", evaluated.Result.Value.TabCount, evaluated.Result.Value.FixtureCount, evaluated.Result.Value.BlankCount)
	t.Logf("fixture_created_tabs=%d fixture_tab_categories=%v", evaluated.Result.Value.CreatedTabCount, evaluated.Result.Value.Categories)
	if evaluated.Result.Value.TabCount != 1 || evaluated.Result.Value.FixtureCount != 1 {
		return "", errors.New("fixture window includes tabs outside its creation request")
	}
	infos, err = list()
	if err != nil {
		return "", err
	}
	target := ""
	for _, info := range infos {
		if info.Type == "page" && info.URL == fixtureURL {
			if target != "" {
				return "", errors.New("fixture target identity ambiguous")
			}
			target = info.TargetID
		}
	}
	if target == "" {
		return "", errors.New("fixture target unavailable after window creation")
	}
	return target, nil
}

func bridgeExtensionOrigin(bridgePath string) (string, error) {
	manifest, err := os.ReadFile(filepath.Join(bridgePath, "manifest.json"))
	if err != nil {
		return "", err
	}
	var value struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(manifest, &value); err != nil || value.Key == "" {
		return "", errors.New("packaged Bridge manifest key is unavailable")
	}
	publicKey, err := base64.StdEncoding.DecodeString(value.Key)
	if err != nil {
		return "", errors.New("packaged Bridge manifest key is invalid")
	}
	hash := sha256.Sum256(publicKey)
	const alphabet = "abcdefghijklmnop"
	id := make([]byte, 32)
	for i, b := range hash[:16] {
		id[2*i], id[2*i+1] = alphabet[b>>4], alphabet[b&0x0f]
	}
	return "chrome-extension://" + string(id), nil
}

type bridgeHandoffHTTPObservation struct {
	hostPageHits      atomic.Int32
	bootstrapRequests atomic.Int32
	bootstrapStatus   atomic.Int32
}

type bridgeHandoffStatusWriter struct {
	http.ResponseWriter
	status int
}

func (w *bridgeHandoffStatusWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *bridgeHandoffStatusWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

func (w *bridgeHandoffStatusWriter) code() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

func waitForBridgeHandoffCapability(ctx context.Context, s *Server) error {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		s.splitCapture.mu.Lock()
		ready := !s.splitCapture.closed && s.splitCapture.sourceTrackingSupported && s.splitCapture.hostCloseSupported
		s.splitCapture.mu.Unlock()
		if ready {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

type bridgeHandoffOwnedProtocol interface {
	Call(context.Context, string, any, string) (json.RawMessage, error)
}

func bridgeHandoffOwnedStartupDiagnostic(ctx context.Context, protocol bridgeHandoffOwnedProtocol, launchURL, extensionOrigin string,
	hostHits, bootstrapRequests, bootstrapStatus int32, s *Server,
) string {
	state := "query_failed"
	expectedTargets := 0
	hostTitle, exactExtensionWorker := false, false
	encoded, err := protocol.Call(ctx, "Target.getTargets", nil, "")
	if err == nil {
		var targets struct {
			TargetInfos []struct {
				Type  string `json:"type"`
				URL   string `json:"url"`
				Title string `json:"title"`
			} `json:"targetInfos"`
		}
		if json.Unmarshal(encoded, &targets) == nil {
			state = "ok"
			for _, target := range targets.TargetInfos {
				if target.Type == "page" && target.URL == launchURL {
					expectedTargets++
					hostTitle = target.Title == "AkuBrowser capture host"
				}
				if target.Type == "service_worker" && strings.HasPrefix(target.URL, extensionOrigin+"/") {
					exactExtensionWorker = true
				}
			}
		}
	}
	s.splitCapture.mu.Lock()
	accepted := s.splitCapture.sourceTrackingSupported && s.splitCapture.hostCloseSupported
	s.splitCapture.mu.Unlock()
	return fmt.Sprintf("host_http_hits=%d private_protocol=%s exact_host_targets=%d host_title=%t exact_bridge_worker=%t bootstrap_requests=%d bootstrap_status=%d bootstrap_accepted=%t",
		hostHits, state, expectedTargets, hostTitle, exactExtensionWorker, bootstrapRequests, bootstrapStatus, accepted)
}

// Navigate only the known static host after its server/runtime bindings exist.
// The owner keeps the root pipe; this function detaches just its temporary page
// session. Protocol acceptance alone does not establish Bridge bootstrap.
func initializeBridgeSmokeCaptureHost(ctx context.Context, protocol bridgeHandoffOwnedProtocol, launchURL string) error {
	return appshell.NavigateCaptureHost(ctx, protocol, launchURL)
}
