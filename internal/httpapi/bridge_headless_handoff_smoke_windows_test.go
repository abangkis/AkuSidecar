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
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/appshell"
	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/collection"
	"github.com/abangkis/AkuSidecar/internal/collection/headless"
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
	artifactRoot := filepath.Join(sidecarRoot, ".test-artifacts")
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
		URL: launchURL, StartMinimized: true,
		ExtraArgs: []string{"--start-minimized", "--remote-debugging-address=127.0.0.1", "--remote-debugging-port=0"},
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
	if err := window.SetCaptureHandoff(s.CloseSplitCaptureHost); err != nil {
		window.Terminate()
		t.Fatal("bind scoped capture-host retirement")
	}
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
	if err := manager.SetTransitionReadiness(func(checkCtx context.Context) error {
		if manager.Snapshot().Driver == "headless" {
			return checkCtx.Err()
		}
		return s.SplitCaptureReplacementReadiness(checkCtx)
	}); err != nil {
		t.Fatal("bind authenticated transport readiness")
	}
	launch := func(nextCtx context.Context, mode string, _ uint64) (captureruntime.Process, error) {
		if mode != "headless" {
			return nil, errors.New("unexpected smoke runtime")
		}
		if err := s.RotateSplitCapture(); err != nil {
			return nil, err
		}
		return headless.Launch(nextCtx, options)
	}
	coordinator := collection.NewCoordinator(manager, launch, func() error { return headless.Validate(options) })
	e.AttachCollectionCoordinator(coordinator)
	coordinator.Request("headless")

	capabilityCtx, stopCapabilityWait := context.WithTimeout(ctx, 48*time.Second)
	capabilityErr := waitForBridgeHandoffCapability(capabilityCtx, s)
	stopCapabilityWait()
	if capabilityErr != nil {
		diagnosticCtx, stopDiagnostic := context.WithTimeout(ctx, 8*time.Second)
		diagnostic := bridgeHandoffStartupDiagnostic(
			diagnosticCtx, filepath.Join(runtimeRoot, "node.exe"),
			filepath.Join(filepath.Dir(sourceFile), "..", "appshell", "testdata", "host_handoff_cdp.mjs"),
			profile, launchURL, origin,
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
	select {
	case <-window.Done(): // CloseForRetry returns only after natural root and Job drain.
	case <-ctx.Done():
		t.Fatal("headed Chrome owner did not drain naturally")
	}
	status := coordinator.Status()
	if status.Effective != "headless" || status.Pending || status.Generation != 2 || status.State != captureruntime.Ready {
		t.Fatal("headless generation 2 was not ready after verified profile handoff")
	}
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

func bridgeHandoffStartupDiagnostic(
	ctx context.Context, nodePath, helperPath, profile, launchURL, extensionOrigin string,
	hostHits, bootstrapRequests, bootstrapStatus int32, s *Server,
) string {
	hostTarget, hostTitle, hostFragment, bridgeWorker := false, false, false, false
	cdpState := "unavailable"
	if endpoint, ok := bridgeHandoffDevToolsEndpoint(ctx, profile); ok {
		command := exec.CommandContext(ctx, nodePath, helperPath, endpoint, "Target.getTargets", "{}")
		command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		output, err := command.Output()
		if err == nil {
			var result struct {
				TargetInfos []struct {
					Type  string `json:"type"`
					URL   string `json:"url"`
					Title string `json:"title"`
				} `json:"targetInfos"`
			}
			if json.Unmarshal(output, &result) == nil {
				cdpState = "ok"
				baseURL := strings.SplitN(launchURL, "#", 2)[0]
				for _, target := range result.TargetInfos {
					if target.Type == "page" && strings.SplitN(target.URL, "#", 2)[0] == baseURL {
						hostTarget = true
						hostTitle = target.Title == "AkuBrowser capture host"
						hostFragment = target.URL == launchURL
					}
					if target.Type == "service_worker" && strings.HasPrefix(target.URL, extensionOrigin+"/") {
						bridgeWorker = true
					}
				}
			}
		} else if ctx.Err() == nil {
			cdpState = "query_error"
		}
	} else if ctx.Err() != nil {
		cdpState = "deadline"
	}
	s.splitCapture.mu.Lock()
	accepted := s.splitCapture.sourceTrackingSupported && s.splitCapture.hostCloseSupported
	s.splitCapture.mu.Unlock()
	return fmt.Sprintf("host_http_hits=%d host_target=%t host_title=%t host_fragment=%t bridge_worker=%t bootstrap_requests=%d bootstrap_status=%d bootstrap_accepted=%t cdp=%s",
		hostHits, hostTarget, hostTitle, hostFragment, bridgeWorker,
		bootstrapRequests, bootstrapStatus, accepted, cdpState)
}

func bridgeHandoffDevToolsEndpoint(ctx context.Context, profile string) (string, bool) {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		if raw, err := os.ReadFile(filepath.Join(profile, "DevToolsActivePort")); err == nil {
			lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
			if len(lines) == 2 {
				port, portErr := strconv.Atoi(strings.TrimSpace(lines[0]))
				path := strings.TrimSpace(lines[1])
				if portErr == nil && port > 0 && port <= 65535 && strings.HasPrefix(path, "/devtools/browser/") {
					return "ws://127.0.0.1:" + strconv.Itoa(port) + path, true
				}
			}
		}
		select {
		case <-ctx.Done():
			return "", false
		case <-ticker.C:
		}
	}
}
