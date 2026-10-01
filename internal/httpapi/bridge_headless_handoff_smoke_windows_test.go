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
	"sync/atomic"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/appshell"
	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/collection"
	"github.com/abangkis/AkuSidecar/internal/collection/headless"
	"github.com/abangkis/AkuSidecar/internal/collection/quiet"
	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/engine"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/AkuSidecar/internal/store"
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
	originalHandler := s.http.Handler
	s.http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	bindQuiet := func(nextCtx context.Context, headed *appshell.Window, generation uint64) error {
		targets, err := quiet.NewTargets(nextCtx, headed.CaptureProtocol())
		if err != nil {
			if targets != nil {
				_ = targets.Close(nextCtx)
			}
			return err
		}
		worker := quiet.NewWorker(targets, options)
		// Keep one actual hidden target alive so retirement must dispose it.
		if _, err := targets.Call(nextCtx, domain.SourceX, "Runtime.evaluate", map[string]any{"expression": "({fixture:true})", "returnByValue": true}); err != nil {
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
		t.Fatal("real Bridge scoped retirement and profile handoff failed")
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
	lease.Release()
	releaseIntent()
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		status := coordinator.Status()
		if status.Effective == "headless" && status.Generation == 4 && !status.Pending && status.State == captureruntime.Ready {
			break
		}
		if status.Failure != "" {
			t.Fatal("auto-return failed", status.Failure)
		}
		select {
		case <-ctx.Done():
			t.Fatal("auto-return did not reach headless generation 4")
		case <-tick.C:
		}
	}
	t.Log("host_only_negotiated=true hidden_retirement=true browser_borrow_generation=3 auto_return_headless_generation=4 source_permission_gate_fixture_only=true")
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
