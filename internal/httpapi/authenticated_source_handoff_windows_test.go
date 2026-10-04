//go:build windows

package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	neturl "net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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

// TestAuthenticatedSourceHeadlessHandoffWindowsSmoke is an explicit operator
// fixture. The wrapper must first stop and drain the registered browser owner;
// this test then borrows the acknowledged existing profile only for one real
// source-open action and bounded headless capture. It never removes the profile.
func TestAuthenticatedSourceHeadlessHandoffWindowsSmoke(t *testing.T) {
	if os.Getenv("AKU_AUTH_JOURNEY_ACK_MIXED_UPDATE") == "1" || os.Getenv("AKU_AUTH_JOURNEY_ACK_FACEBOOK_RECAPTURE") == "1" {
		t.Skip("mixed Update authorization does not include a separate source-window cycle")
	}
	runAuthenticatedSourceFixture(t, false, false)
}

func TestAuthenticatedHybridUpdateWindowsSmoke(t *testing.T) {
	if os.Getenv("AKU_AUTH_JOURNEY_ACK_MIXED_UPDATE") != "1" {
		t.Skip("one mixed Update requires its own explicit operator acknowledgement")
	}
	runAuthenticatedSourceFixture(t, true, false)
}

func runAuthenticatedSourceFixture(t *testing.T, mixedUpdate, facebookRecapture bool) {
	if strings.TrimSpace(os.Getenv("AKU_AUTH_JOURNEY_RUNTIME")) == "" {
		for _, key := range []string{"AKU_AUTH_JOURNEY_CHROME", "AKU_AUTH_JOURNEY_BRIDGE", "AKU_AUTH_JOURNEY_PROFILE", "AKU_AUTH_JOURNEY_PROFILE_DIRECTORY", "AKU_AUTH_JOURNEY_SOURCE", "AKU_AUTH_JOURNEY_TARGET_URL", "AKU_AUTH_JOURNEY_ACK_PROFILE", "AKU_AUTH_JOURNEY_ACK_FOREGROUND", "AKU_AUTH_JOURNEY_ACK_BRIDGE_RELOAD", "AKU_AUTH_JOURNEY_ACK_MIXED_UPDATE", "AKU_AUTH_JOURNEY_ACK_FACEBOOK_RECAPTURE"} {
			if os.Getenv(key) != "" {
				t.Fatalf("%s requires AKU_AUTH_JOURNEY_RUNTIME", key)
			}
		}
		t.Skip("set AKU_AUTH_JOURNEY_RUNTIME and all authenticated journey inputs to run")
	}
	if os.Getenv("AKU_AUTH_JOURNEY_ACK_PROFILE") != "1" || os.Getenv("AKU_AUTH_JOURNEY_ACK_FOREGROUND") != "1" {
		t.Fatal("authenticated profile and possible source-window foreground require both explicit acknowledgements")
	}

	runtimeRoot, err := authJourneyPath("AKU_AUTH_JOURNEY_RUNTIME")
	if err != nil {
		t.Fatal("invalid staged runtime path")
	}
	chromePath, err := authJourneyPath("AKU_AUTH_JOURNEY_CHROME")
	if err != nil {
		t.Fatal("invalid registered Chrome path")
	}
	bridgePath, err := authJourneyPath("AKU_AUTH_JOURNEY_BRIDGE")
	if err != nil {
		t.Fatal("invalid registered Bridge path")
	}
	profile, err := authJourneyPath("AKU_AUTH_JOURNEY_PROFILE")
	if err != nil {
		t.Fatal("invalid existing profile path")
	}
	profileDirectory := strings.TrimSpace(os.Getenv("AKU_AUTH_JOURNEY_PROFILE_DIRECTORY"))
	if !appshell.ValidProfileDirectory(profileDirectory) {
		t.Fatal("registered Chrome profile directory is invalid")
	}
	profileInfo, err := os.Stat(profile)
	if err != nil || !profileInfo.IsDir() {
		t.Fatal("existing authenticated profile directory is unavailable")
	}
	selectedDirectory, err := appshell.ResolveProfileDirectory(profile)
	if err != nil || selectedDirectory != profileDirectory {
		t.Fatal("registered Local State selection does not match the supplied profile directory")
	}
	selectedInfo, err := os.Stat(filepath.Join(profile, profileDirectory))
	if err != nil || !selectedInfo.IsDir() {
		t.Fatal("selected authenticated subprofile is unavailable")
	}
	source := domain.Source(strings.ToLower(strings.TrimSpace(os.Getenv("AKU_AUTH_JOURNEY_SOURCE"))))
	if facebookRecapture {
		if source != domain.SourceFacebook || os.Getenv("AKU_AUTH_JOURNEY_ACK_FACEBOOK_RECAPTURE") != "1" || strings.TrimSpace(os.Getenv("AKU_AUTH_JOURNEY_TARGET_URL")) != "" {
			t.Fatal("Facebook Recapture requires its explicit acknowledgement and a fresh target")
		}
	} else if source != domain.SourceInstagram && source != domain.SourceLinkedIn {
		t.Fatal("source must be instagram or linkedin")
	}
	targetURL, ok := domain.CanonicalSourceURL(source, os.Getenv("AKU_AUTH_JOURNEY_TARGET_URL"))
	if !mixedUpdate && !facebookRecapture && (!ok || targetURL != strings.TrimSpace(os.Getenv("AKU_AUTH_JOURNEY_TARGET_URL"))) {
		t.Fatal("target URL must be a canonical native post URL for the selected source")
	}

	origin, err := bridgeExtensionOrigin(bridgePath)
	if err != nil {
		t.Fatal("registered Bridge identity is unavailable")
	}
	options := headless.Options{
		Node: filepath.Join(runtimeRoot, "node.exe"), Worker: filepath.Join(runtimeRoot, "worker.mjs"),
		Pin: filepath.Join(runtimeRoot, "node.pin.json"), Chrome: chromePath,
		Profile: profile, ProfileDirectory: profileDirectory, BridgePath: bridgePath,
	}
	if err := headless.Validate(options); err != nil {
		t.Fatal("staged headless runtime validation failed")
	}

	timeout := 150 * time.Second
	if mixedUpdate {
		timeout = 360 * time.Second
	}
	if facebookRecapture {
		timeout = 480 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	probe, err := net.Listen("tcp", "127.0.0.1:11122")
	if err != nil {
		t.Fatal("127.0.0.1:11122 is occupied; refusing to launch Chrome")
	}
	_ = probe.Close()

	_, sourceFile, _, found := runtime.Caller(0)
	if !found {
		t.Fatal("project-local artifact root is unavailable")
	}
	projectRoot := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", ".."))
	artifactRoot := filepath.Join(projectRoot, "build", "authenticated-source-handoff")
	if err := os.MkdirAll(artifactRoot, 0700); err != nil {
		t.Fatal("project-local fixture directory is unavailable")
	}
	dbDir, err := os.MkdirTemp(artifactRoot, "isolated-db-")
	if err != nil {
		t.Fatal("isolated database directory could not be created")
	}
	t.Cleanup(func() { _ = os.RemoveAll(dbDir) })
	state, err := store.Open(filepath.Join(dbDir, "fixture.db"), domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", true))
	if err != nil {
		t.Fatal("isolated fixture database could not be opened")
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
		t.Fatal("isolated fixture server could not be created")
	}
	if err := s.RequireHostOnlyRetirement(); err != nil {
		_ = state.Close()
		t.Fatal("host-only capture retirement could not be required")
	}
	var manager *captureruntime.Manager
	t.Cleanup(func() {
		if manager != nil {
			manager.Terminate()
		}
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer stopCancel()
		_ = s.Stop(stopCtx)
		_ = state.Close()
	})
	address, err := s.Start()
	if err != nil || address.String() != "127.0.0.1:11122" {
		t.Fatal("fixture server did not bind the product loopback address")
	}

	launchURL, err := s.SplitCaptureLaunchURL("http://127.0.0.1:11122")
	if err != nil {
		t.Fatal("capture-host capability URL could not be created")
	}
	window, err := appshell.Launch(ctx, appshell.LaunchOptions{
		Executable: chromePath, ExtensionPath: bridgePath, UserDataDir: profile,
		URL: launchURL, StartMinimized: true, PrivateCDP: true,
		ExtraArgs: []string{"--start-minimized", "--profile-directory=" + profileDirectory},
	})
	if err != nil {
		t.Fatal("registered Chrome capture owner could not be launched")
	}
	containment, err := window.StartCaptureContainment(logger)
	if err != nil {
		window.Terminate()
		t.Fatal("capture window ownership could not be established")
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
		t.Fatal("capture process manager could not be created")
	}
	if err := e.AttachCaptureRuntime(ctx, manager); err != nil {
		t.Fatal("engine could not attach the capture owner")
	}
	if err := s.SetSplitCaptureRuntime(manager); err != nil {
		t.Fatal("split server could not attach the capture owner")
	}
	if err := window.SetCaptureHandoff(s.CloseSplitCaptureHost); err != nil {
		t.Fatal("scoped capture-host retirement could not be attached")
	}
	if err := initializeBridgeSmokeCaptureHost(ctx, window.CaptureProtocol(), launchURL); err != nil {
		t.Fatal("capture host could not be initialized")
	}
	if err := manager.SetTransitionReadiness(func(checkCtx context.Context) error {
		if manager.Snapshot().Driver == "headless" {
			return checkCtx.Err()
		}
		return s.SplitCaptureReplacementReadiness(checkCtx)
	}); err != nil {
		t.Fatal("authenticated capture readiness guard could not be attached")
	}

	var coordinator *collection.Coordinator
	var borrowedWindow *appshell.Window
	var borrowedContainment appshell.CaptureContainment
	var borrowedHostURL string
	var baselineTargetIDs map[string]bool
	var preparedSourceMu sync.Mutex
	var preparedSourceID string
	var preparedSourceTargetID string
	launch := func(nextCtx context.Context, mode string, generation uint64) (captureruntime.Process, error) {
		if err := s.RotateSplitCapture(); err != nil {
			return nil, err
		}
		if mode == "headless" {
			return headless.Launch(nextCtx, options)
		}
		if mode != "browser" {
			return nil, errors.New("unsupported fixture capture mode")
		}
		url, err := s.SplitCaptureLaunchURL("http://127.0.0.1:11122")
		if err != nil {
			return nil, err
		}
		headed, err := appshell.Launch(nextCtx, appshell.LaunchOptions{
			Executable: chromePath, ExtensionPath: bridgePath, UserDataDir: profile,
			URL: url, StartMinimized: true, PrivateCDP: true,
			ExtraArgs: []string{"--start-minimized", "--profile-directory=" + profileDirectory},
		})
		if err != nil {
			return headed, err
		}
		if err := headed.SetCaptureHandoff(s.CloseSplitCaptureHost); err != nil {
			return headed, err
		}
		owner, err := headed.StartCaptureContainment(logger)
		if err != nil {
			return headed, err
		}
		tracked, ok := owner.(interface {
			PrepareSourceWindowLifetime(context.Context, string) error
		})
		if !ok {
			return headed, errors.New("source window lifetime tracking unavailable")
		}
		s.SetSplitSourceWindowPreparation(func(prepareCtx context.Context, marker string) error {
			if err := tracked.PrepareSourceWindowLifetime(prepareCtx, marker); err != nil {
				return err
			}
			const prefix = "AkuBrowser source split_"
			if !strings.HasPrefix(marker, prefix) {
				return errors.New("source marker identity invalid")
			}
			actionID := strings.TrimPrefix(marker, "AkuBrowser source ")
			// Bind the exact action's intent page before Bridge navigates it.
			// tabs.update acknowledges a navigation request, not its commit.
			infos, err := authJourneyTargets(prepareCtx, headed.CaptureProtocol())
			if err != nil {
				return err
			}
			intentURL := "http://127.0.0.1:11122/split-source-intent?id=" + neturl.QueryEscape(actionID)
			targetID := ""
			for _, info := range infos {
				if info.Type == "page" && info.URL == intentURL && !baselineTargetIDs[info.ID] {
					if targetID != "" {
						return errors.New("source intent target ambiguous")
					}
					targetID = info.ID
				}
			}
			if targetID == "" {
				return errors.New("exact source intent target unavailable")
			}
			preparedSourceMu.Lock()
			preparedSourceID = actionID
			preparedSourceTargetID = targetID
			preparedSourceMu.Unlock()
			return nil
		})
		borrowedWindow, borrowedContainment, borrowedHostURL = headed, owner, url
		if err := appshell.NavigateCaptureHost(nextCtx, headed.CaptureProtocol(), url); err != nil {
			return headed, err
		}
		tick := time.NewTicker(50 * time.Millisecond)
		defer tick.Stop()
		for {
			if s.SplitCaptureReplacementReadiness(nextCtx) == nil {
				infos, inventoryErr := authJourneyTargets(nextCtx, headed.CaptureProtocol())
				if inventoryErr != nil {
					return headed, errors.New("pre-action target inventory unavailable")
				}
				baselineTargetIDs = make(map[string]bool, len(infos))
				for _, info := range infos {
					baselineTargetIDs[info.ID] = true
				}
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
	coordinator.Start(ctx)

	if err := waitForAuthJourneyBridge(ctx, e, source); err != nil {
		t.Fatal("real Bridge source permission/readiness was not established")
	}
	if mixedUpdate {
		for _, required := range []domain.Source{domain.SourceX, domain.SourceFacebook, domain.SourceInstagram, domain.SourceLinkedIn} {
			if err := waitForAuthJourneyBridge(ctx, e, required); err != nil {
				t.Fatal("all four existing source permissions must be ready before the mixed Update")
			}
		}
	}
	// A permission heartbeat may come from an already registered background
	// worker. It does not prove that this new capture host negotiated retirement.
	capabilityCtx, capabilityCancel := context.WithTimeout(ctx, 10*time.Second)
	capabilityErr := waitForAuthJourneyHostCapabilities(capabilityCtx, s)
	capabilityCancel()
	if capabilityErr != nil && os.Getenv("AKU_AUTH_JOURNEY_ACK_BRIDGE_RELOAD") == "1" {
		// Use the product's authenticated Bridge control route exactly once.
		// No extension API is injected through CDP and no permission is granted.
		if err := reloadAuthJourneyBridge(ctx, state, e); err != nil {
			t.Fatalf("one explicitly authorized Bridge reload did not complete (%v)", err)
		}
		t.Log("authorized_bridge_reload_completed=true")
		reloadCtx, reloadCancel := context.WithTimeout(ctx, 20*time.Second)
		capabilityErr = waitForAuthJourneyHostCapabilities(reloadCtx, s)
		reloadCancel()
	}
	if capabilityErr != nil {
		s.splitCapture.mu.Lock()
		tracking, closeHost, hostOnly := s.splitCapture.sourceTrackingSupported, s.splitCapture.hostCloseSupported, s.splitCapture.hostOnlySupported
		s.splitCapture.mu.Unlock()
		t.Logf("bridge_heartbeat_ready=true source_tracking=%t capture_host_close=%t host_only_retirement=%t", tracking, closeHost, hostOnly)
		t.Fatal("capture-host handoff capabilities were not negotiated; no source window was opened")
	}
	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal("isolated fixture settings could not be read")
	}
	settings.CollectionMode = "headless"
	settings.ActiveSources = []domain.Source{source}
	if mixedUpdate {
		// Deliberately put Facebook first: the frozen execution plan must reorder
		// it behind the headless batch without changing configured source order.
		settings.ActiveSources = []domain.Source{domain.SourceFacebook, domain.SourceX, domain.SourceInstagram, domain.SourceLinkedIn}
		settings.CaptureVisibility = "adaptive_fidelity"
		settings.MaxScrolls = 0
		settings.MaxItemsPerSource = 5
		settings.MaxItemsTotal = 20
		settings.AIDetectionEnabled = false
		settings.CalibrationEnabled = false
	}
	if facebookRecapture {
		settings.CaptureVisibility = "adaptive_fidelity"
		settings.MaxScrolls = 0
		settings.MaxItemsPerSource = 5
		settings.MaxItemsTotal = 5
		settings.AIDetectionEnabled = false
		settings.CalibrationEnabled = false
	}
	settingsPayload, err := json.Marshal(map[string]any{"settings": settings})
	if err != nil {
		t.Fatal("isolated headless settings could not be encoded")
	}
	settingsReq, err := http.NewRequestWithContext(ctx, http.MethodPut, "http://127.0.0.1:11122/api/settings", bytes.NewReader(settingsPayload))
	if err != nil {
		t.Fatal("isolated settings request could not be created")
	}
	settingsReq.Header.Set("Content-Type", "application/json")
	settingsResp, err := (&http.Client{Timeout: 8 * time.Second}).Do(settingsReq)
	if err != nil {
		t.Fatal("isolated headless settings could not be applied")
	}
	var settingsResult struct {
		Settings domain.Settings `json:"settings"`
	}
	decodeErr := json.NewDecoder(io.LimitReader(settingsResp.Body, 128*1024)).Decode(&settingsResult)
	_ = settingsResp.Body.Close()
	if settingsResp.StatusCode != http.StatusOK || decodeErr != nil || settingsResult.Settings.CollectionMode != "headless" || len(settingsResult.Settings.ActiveSources) != len(settings.ActiveSources) || settingsResult.Settings.ActiveSources[0] != settings.ActiveSources[0] {
		t.Fatal("isolated Settings API did not persist the requested source and headless mode")
	}
	if err := waitForAuthJourneyRuntime(ctx, coordinator, "headless", 2); err != nil {
		t.Fatal("initial headless owner did not become ready")
	}
	if facebookRecapture {
		runAuthenticatedFacebookRecapture(t, ctx, state, e, coordinator, manager)
		return
	}
	if mixedUpdate {
		runAuthenticatedHybridUpdate(t, ctx, state, e, coordinator, manager)
		return
	}
	preHandoffID, err := captureAuthJourneyTarget(ctx, coordinator, source, targetURL)
	if err != nil {
		assertAuthJourneyCaptureError(t, err)
	}
	wantID := domain.NativeIdentityFromPermalink(source, targetURL)
	if wantID == "" || preHandoffID != wantID {
		t.Fatal("initial headless capture did not match the exact native post identity")
	}
	t.Log("initial_headless_generation=2 initial_native_identity=true")

	bridgeToken, err := state.BridgeToken(ctx)
	if err != nil {
		t.Fatal("fixture Bridge token is unavailable")
	}
	openSourceReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://127.0.0.1:11122/api/split-capture/actions", strings.NewReader(`{"type":"open_source","source":"`+string(source)+`"}`))
	if err != nil {
		t.Fatal("source-open action request could not be created")
	}
	openSourceReq.Header.Set("Content-Type", "application/json")
	openSourceReq.Header.Set("X-Aku-Bridge-Token", bridgeToken)
	openSourceReq.Header.Set("X-Aku-Bridge-Contract", domain.BridgeContractVersion)
	openSourceReq.Header.Set("X-Aku-Split-Epoch", e.Epoch())
	openSourceResp, err := (&http.Client{Timeout: 125 * time.Second}).Do(openSourceReq)
	if err != nil {
		t.Fatal("real Bridge source-open action did not complete")
	}
	var openSourceResult splitActionResult
	decodeErr = json.NewDecoder(io.LimitReader(openSourceResp.Body, 128*1024)).Decode(&openSourceResult)
	_ = openSourceResp.Body.Close()
	if openSourceResp.StatusCode != http.StatusOK || decodeErr != nil || !openSourceResult.OK {
		t.Fatal("real Bridge rejected the source-open action")
	}
	preparedSourceMu.Lock()
	actionID := preparedSourceID
	preparedTargetID := preparedSourceTargetID
	preparedSourceMu.Unlock()
	if actionID == "" {
		t.Fatal("source action did not complete its actual HWND lifetime preparation")
	}
	if borrowedWindow == nil || borrowedContainment == nil || manager.Snapshot().Driver != "browser" || manager.Snapshot().Generation != 3 {
		t.Fatal("source action did not borrow the next real browser generation")
	}
	if err := waitForAuthJourneyBrowserBorrow(ctx, coordinator, 3); err != nil {
		t.Fatal("interactive browser owner did not become ready")
	}
	t.Log("interactive_browser_generation=3 source_window_prepared=true")

	navigationCtx, navigationCancel := context.WithTimeout(ctx, 8*time.Second)
	pageTarget, sourceWindowID, err := waitForAuthJourneySourceWindow(navigationCtx, borrowedWindow.CaptureProtocol(), borrowedHostURL, source, actionID, preparedTargetID, baselineTargetIDs)
	navigationCancel()
	if err != nil {
		t.Fatalf("new source window could not be proven safe to close (%v)", err)
	}
	if err := waitForAuthJourneyLifetimeBlock(ctx, borrowedContainment); err != nil {
		t.Fatal("tracked live source HWND did not block automatic headless return")
	}
	if err := waitForAuthJourneyBlockedAutoReturn(ctx, coordinator); err != nil {
		t.Fatal("coordinator did not retain the browser owner after source action leases drained")
	}
	if err := closeAuthJourneySourceTarget(ctx, borrowedWindow.CaptureProtocol(), pageTarget, sourceWindowID, borrowedHostURL, source); err != nil {
		t.Fatal("exact source target could not be closed after ownership revalidation")
	}
	if err := waitForAuthJourneyRuntime(ctx, coordinator, "headless", 4); err != nil {
		t.Fatal("headless auto-return did not complete after natural source-window close")
	}

	second, secondErr := captureAuthJourneyTarget(ctx, coordinator, source, targetURL)
	if secondErr != nil {
		assertAuthJourneyCaptureError(t, secondErr)
	}
	if wantID == "" || second != wantID {
		t.Fatal("headless captures did not preserve the exact native post identity across handoff")
	}
	t.Logf("authenticated_source_handoff=true source=%s browser_generation=3 headless_generation=4 source_window_prepared=true live_window_blocked_return=true pre_and_post_native_identity=true capture_count=2", source)
}

func authJourneyPath(name string) (string, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return "", errors.New("missing")
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(absolute)
	if err != nil || !info.IsDir() && name != "AKU_AUTH_JOURNEY_CHROME" {
		return "", errors.New("unavailable")
	}
	if name == "AKU_AUTH_JOURNEY_CHROME" && info.IsDir() {
		return "", errors.New("not a file")
	}
	return absolute, nil
}

func waitForAuthJourneyBridge(ctx context.Context, e *engine.Engine, source domain.Source) error {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		status := e.BridgeStatus()
		if status.Compatible && status.Actual != nil {
			for _, access := range status.Actual.SourceAccess.Sources {
				if access.Source == string(source) && access.PermissionGranted && access.ScriptRegistered && access.Ready {
					return nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func waitForAuthJourneyHostCapabilities(ctx context.Context, s *Server) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if s.SplitCaptureReplacementReadiness(ctx) == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func reloadAuthJourneyBridge(ctx context.Context, state *store.Store, e *engine.Engine) error {
	token, err := state.BridgeToken(ctx)
	if err != nil {
		return errors.New("fixture token unavailable")
	}
	return reloadAuthJourneyControl(ctx, "http://127.0.0.1:11122", token, e.Epoch())
}

// Follow the existing UI maintenance protocol: create, claim, relay, verify.
// The real Bridge accepts the delivered action; this helper never accepts it.
func reloadAuthJourneyControl(ctx context.Context, endpoint, token, epoch string) error {
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	request := func(method, path string, body any, expected int, value any) error {
		stage := "verify"
		switch path {
		case "/api/operations/bridge/actions/reload-self":
			stage = "create"
		case "/api/operations/bridge/actions/next":
			stage = "claim"
		case "/api/split-capture/actions":
			stage = "relay"
		}
		var payload []byte
		if body != nil {
			var err error
			payload, err = json.Marshal(body)
			if err != nil {
				return err
			}
		}
		req, err := http.NewRequestWithContext(ctx, method, endpoint+path, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Aku-Bridge-Token", token)
		req.Header.Set("X-Aku-Bridge-Contract", domain.BridgeContractVersion)
		req.Header.Set("X-Aku-Split-Epoch", epoch)
		resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
		if err != nil {
			return errors.New("reload control request failed")
		}
		defer resp.Body.Close()
		if resp.StatusCode != expected {
			return fmt.Errorf("reload %s rejected: HTTP %d", stage, resp.StatusCode)
		}
		if err := json.NewDecoder(io.LimitReader(resp.Body, 128*1024)).Decode(value); err != nil {
			return errors.New("reload control response invalid")
		}
		return nil
	}
	var created struct {
		Action engine.ReloadAction `json:"action"`
	}
	if err := request(http.MethodPost, "/api/operations/bridge/actions/reload-self", map[string]any{
		"requestId": domain.NewID("source_handoff_reload"),
		"actor":     map[string]string{"actorType": "user", "actorId": "authorized-source-handoff-fixture"},
		"reason":    "explicitly authorized single Bridge reload for authenticated source handoff qualification",
	}, http.StatusAccepted, &created); err != nil {
		return err
	}
	if created.Action.ID == "" || created.Action.Status != "pending" {
		return errors.New("reload action was not created")
	}
	var delivered struct {
		Action engine.ReloadAction `json:"action"`
	}
	if err := request(http.MethodGet, "/api/operations/bridge/actions/next", nil, http.StatusOK, &delivered); err != nil {
		return err
	}
	if delivered.Action.ID != created.Action.ID || delivered.Action.Status != "delivered" {
		return errors.New("reload action claim did not match")
	}
	var result splitActionResult
	if err := request(http.MethodPost, "/api/split-capture/actions", map[string]string{"type": "reload_self", "actionId": created.Action.ID}, http.StatusOK, &result); err != nil {
		return err
	}
	if !result.OK {
		return errors.New("reload control request rejected")
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		var observed struct {
			Action engine.ReloadAction `json:"action"`
		}
		if err := request(http.MethodGet, "/api/operations/bridge/actions/"+neturl.PathEscape(created.Action.ID), nil, http.StatusOK, &observed); err != nil {
			return err
		}
		if observed.Action.ID != created.Action.ID {
			return errors.New("reload verification identity changed")
		}
		if observed.Action.Status == "failed" {
			return errors.New("reload verification failed")
		}
		if observed.Action.Status == "completed" {
			if observed.Action.HeartbeatObservedAt == nil || observed.Action.ObservedBuildID != engine.ExpectedBridgeBuildID {
				return errors.New("reload expected heartbeat missing")
			}
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func waitForAuthJourneyRuntime(ctx context.Context, c *collection.Coordinator, driver string, generation uint64) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		status := c.Status()
		if status.Effective == driver && status.Generation == generation && !status.Pending && status.State == captureruntime.Ready {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func waitForAuthJourneyBrowserBorrow(ctx context.Context, c *collection.Coordinator, generation uint64) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		status := c.Status()
		if status.Effective == "browser" && status.Requested == "headless" && status.Generation == generation && status.State == captureruntime.Ready {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

type authJourneyTarget struct {
	ID     string `json:"targetId"`
	Type   string `json:"type"`
	URL    string `json:"url"`
	Opener string `json:"openerId"`
}

func authJourneyTargets(ctx context.Context, protocol appshell.CaptureProtocol) ([]authJourneyTarget, error) {
	raw, err := protocol.Call(ctx, "Target.getTargets", nil, "")
	var result struct {
		TargetInfos []authJourneyTarget `json:"targetInfos"`
	}
	if err != nil || json.Unmarshal(raw, &result) != nil {
		return nil, errors.New("target inventory unavailable")
	}
	return result.TargetInfos, nil
}

func authJourneyWindowID(ctx context.Context, protocol appshell.CaptureProtocol, targetID string) (int, error) {
	raw, err := protocol.Call(ctx, "Browser.getWindowForTarget", map[string]any{"targetId": targetID}, "")
	var result struct {
		WindowID int `json:"windowId"`
	}
	if err != nil || json.Unmarshal(raw, &result) != nil || result.WindowID == 0 {
		return 0, errors.New("native window identity unavailable")
	}
	return result.WindowID, nil
}

func authJourneySourceHost(source domain.Source, raw string) bool {
	u, err := neturl.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return false
	}
	switch source {
	case domain.SourceInstagram:
		return u.Hostname() == "www.instagram.com" || u.Hostname() == "instagram.com"
	case domain.SourceLinkedIn:
		return u.Hostname() == "www.linkedin.com" || u.Hostname() == "linkedin.com"
	default:
		return false
	}
}

func findNewAuthJourneySourceWindow(ctx context.Context, protocol appshell.CaptureProtocol, hostURL string, source domain.Source, actionID string, baseline map[string]bool) (string, int, error) {
	// The action ID identifies an exact successfully prepared action in the local
	// ledger. The caller snapshots target IDs before dispatch and passes the
	// source action's browser generation; no unrelated page can be adopted.
	if actionID == "" {
		return "", 0, errors.New("source action identity unavailable")
	}
	infos, err := authJourneyTargets(ctx, protocol)
	if err != nil {
		return "", 0, err
	}
	var candidate authJourneyTarget
	for _, info := range infos {
		if info.Type != "page" || baseline[info.ID] || info.URL == hostURL || !authJourneySourceHost(source, info.URL) {
			continue
		}
		// Only a newly-created page whose URL is one of the requested source's
		// native host family is eligible. The caller-side baseline check below
		// rejects old pages as well as ambiguous candidates.
		if candidate.ID != "" {
			return "", 0, errors.New("multiple new source page candidates")
		}
		candidate = info
	}
	if candidate.ID == "" {
		return "", 0, errors.New("new source page was not found")
	}
	windowID, err := authJourneyWindowID(ctx, protocol, candidate.ID)
	if err != nil {
		return "", 0, err
	}
	hostMatches := 0
	pageCount := 0
	for _, info := range infos {
		if info.Type != "page" {
			continue
		}
		id, idErr := authJourneyWindowID(ctx, protocol, info.ID)
		if idErr != nil {
			return "", 0, errors.New("page window inventory is ambiguous")
		}
		if id == windowID {
			pageCount++
			if info.URL == hostURL {
				hostMatches++
			}
		}
	}
	if pageCount != 1 || hostMatches != 0 {
		return "", 0, errors.New("source target does not occupy a unique single-page window")
	}
	return candidate.ID, windowID, nil
}

func waitForAuthJourneySourceWindow(ctx context.Context, protocol appshell.CaptureProtocol, hostURL string, source domain.Source, actionID, preparedTargetID string, baseline map[string]bool) (string, int, error) {
	if preparedTargetID == "" || baseline[preparedTargetID] {
		return "", 0, errors.New("prepared source target identity missing or preexisting")
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		id, window, err := findNewAuthJourneySourceWindow(ctx, protocol, hostURL, source, actionID, baseline)
		if err == nil {
			if id != preparedTargetID {
				return "", 0, errors.New("source target did not match prepared action")
			}
			return id, window, nil
		}
		// Only the not-yet-committed navigation is retryable. Ambiguous or
		// changed ownership remains a terminal rejection.
		if err.Error() != "new source page was not found" {
			return "", 0, err
		}
		select {
		case <-ctx.Done():
			return "", 0, errors.New("prepared source navigation did not commit within bound")
		case <-ticker.C:
		}
	}
}

func waitForAuthJourneyLifetimeBlock(ctx context.Context, containment appshell.CaptureContainment) error {
	checker, ok := containment.(interface{ ReplacementReadiness(context.Context) error })
	if !ok {
		return errors.New("native lifetime checker unavailable")
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		err := checker.ReplacementReadiness(ctx)
		if err != nil && strings.Contains(err.Error(), "native interactive window is still open") {
			return nil
		}
		if err != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func waitForAuthJourneyBlockedAutoReturn(ctx context.Context, coordinator *collection.Coordinator) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		status := coordinator.Status()
		if status.Effective == "browser" && status.Requested == "headless" && status.Generation == 3 &&
			status.ActiveLeases == 0 && strings.Contains(status.Failure, "native interactive window is still open") {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func closeAuthJourneySourceTarget(ctx context.Context, protocol appshell.CaptureProtocol, targetID string, windowID int, hostURL string, source domain.Source) error {
	infos, err := authJourneyTargets(ctx, protocol)
	if err != nil {
		return err
	}
	found := 0
	targetWindow := 0
	for _, info := range infos {
		if info.ID == targetID && info.Type == "page" && authJourneySourceHost(source, info.URL) {
			found++
			targetWindow, err = authJourneyWindowID(ctx, protocol, info.ID)
			if err != nil {
				return errors.New("exact source window identity changed")
			}
		}
		id, idErr := authJourneyWindowID(ctx, protocol, info.ID)
		if idErr != nil && info.Type == "page" {
			return errors.New("page window identity changed")
		}
		if id == windowID && (info.ID != targetID || info.URL == hostURL) {
			return errors.New("source window acquired another page; refusing close")
		}
	}
	if found != 1 || targetWindow != windowID {
		return errors.New("exact source target identity changed")
	}
	closed, err := protocol.Call(ctx, "Target.closeTarget", map[string]any{"targetId": targetID}, "")
	var result struct {
		Success bool `json:"success"`
	}
	if err != nil || json.Unmarshal(closed, &result) != nil || !result.Success {
		return errors.New("exact source target close was rejected")
	}
	return nil
}

func captureAuthJourneyTarget(ctx context.Context, coordinator *collection.Coordinator, source domain.Source, targetURL string) (string, error) {
	observation, err := coordinator.Capture(ctx, source, map[string]any{
		"pageUrl": targetURL, "scrolls": 0, "maxScrolls": 0, "maxPosts": 1,
		"maxBlocksPerSnapshot": 1, "sourceHydrationTimeoutMs": 8000, "captureTimeoutMs": 30000,
	})
	if err != nil {
		return "", err
	}
	want := domain.NativeIdentityFromPermalink(source, targetURL)
	for _, snapshot := range observation.Snapshots {
		for _, block := range snapshot.Blocks {
			canonical, ok := domain.CanonicalSourceURL(source, block.Permalink)
			if ok && canonical == targetURL && domain.NormalizeNativeIdentity(source, block.PlatformID) == want {
				return domain.NormalizeNativeIdentity(source, block.PlatformID), nil
			}
		}
	}
	return "", errors.New("exact native identity was not observed")
}

func assertAuthJourneyCaptureError(t *testing.T, err error) {
	t.Helper()
	var typed *headless.CaptureError
	if errors.As(err, &typed) && typed.Code == "login_required" {
		t.Fatal("headless capture reported login_required; authenticated source context did not survive handoff")
	}
	t.Fatal("headless native target capture failed")
}
