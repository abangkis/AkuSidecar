package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/abangkis/AkuSidecar/internal/activationguard"
	"github.com/abangkis/AkuSidecar/internal/aidetector"
	"github.com/abangkis/AkuSidecar/internal/appshell"
	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/codexselection"
	"github.com/abangkis/AkuSidecar/internal/collection"
	"github.com/abangkis/AkuSidecar/internal/collection/headless"
	"github.com/abangkis/AkuSidecar/internal/collection/quiet"
	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/diagnostics/captureactivity"
	"github.com/abangkis/AkuSidecar/internal/diagnostics/observability"
	"github.com/abangkis/AkuSidecar/internal/diagnostics/operational"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/engine"
	semanticengine "github.com/abangkis/AkuSidecar/internal/eventengine"
	"github.com/abangkis/AkuSidecar/internal/httpapi"
	"github.com/abangkis/AkuSidecar/internal/livingtopics"
	"github.com/abangkis/AkuSidecar/internal/mediaprovenance"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/AkuSidecar/internal/store"
	d "github.com/abangkis/ai4u-diagnostics-go/pkg/diagnostics"
	"github.com/abangkis/ai4u-library-go/pkg/codexruntime"
)

func requiresDatabaseDecision(deployment config.DeploymentConfig) bool {
	return deployment.Mode == "production-installed-app"
}

func main() {
	logger := log.New(os.Stdout, "AkuSidecar ", log.LstdFlags|log.LUTC|log.Lmsgprefix)
	options := config.ParseFlags()
	if handled, err := dispatchXApplicationQA(options, os.Stdin, os.Stdout); handled || err != nil {
		fatal(logger, "qa_dispatch", err)
		return
	}
	manual, err := engine.PrepareXManualDevelopment(options)
	fatal(logger, "prepare_manual", err)
	if manual != nil {
		defer func() {
			if err := manual.Close(); err != nil {
				operational.Problem(logger, "app_shutdown", "manual_cleanup", "uncertain", err, "")
			}
		}()
		if options.AppShell {
			options.BridgeExtensionPath = manual.UIExtensionPath()
		}
	}
	if manual == nil {
		if handled, err := dispatchDevelopmentSession(options, os.Stdin, os.Stdout); handled || err != nil {
			fatal(logger, "development_session", err)
			return
		}
	}
	if options.DiscoverCodex {
		os.Exit(discoverCodex(options))
	}
	if options.DiscoverChromium {
		os.Exit(discoverChromium(options))
	}
	cfg, err := config.Load(options)
	fatal(logger, "load_configuration", err)
	if manual != nil {
		fatal(logger, "validate_manual_configuration", manual.ValidateConfig(cfg))
	}
	if options.NativeDerivationHoldProbe {
		fatal(logger, "write_hold_probe", json.NewEncoder(os.Stdout).Encode(map[string]any{"status": "ok", "schema": "aku.native-derivation-hold-capability.v1", "version": 1}))
		return
	}
	if options.RuntimeCandidateProbe {
		probe, probeErr := runtimeCandidateProbe(cfg.Version, options.RuntimeCandidateProbeSchema)
		fatal(logger, "candidate_probe", probeErr)
		fatal(logger, "write_candidate_probe", json.NewEncoder(os.Stdout).Encode(probe))
		return
	}
	if options.DatabaseInspect {
		fatal(logger, "write_database_inspection", json.NewEncoder(os.Stdout).Encode(store.InspectDatabase(cfg.Database.Path)))
		return
	}
	// Diagnostics above validate/read their own inputs without constructing the
	// optional capture composition, opening profiles or requiring process consent.
	daily, err := engine.PrepareXDailyDevelopment(cfg, options)
	fatal(logger, "prepare_daily", err)
	if daily != nil {
		defer daily.Close()
	}
	if _, running := existingInstance(fmt.Sprintf("http://%s/api/health", cfg.Server.HostPort()), time.Second); running {
		operational.Report(logger, "app_startup", "existing_instance", "cancelled", nil)
		os.Exit(5)
	}
	settings := domain.DefaultSettings(cfg.Capture.Profile, cfg.Capture.Visibility, cfg.Preference.Mode, cfg.Capture.OpenMissingSource)
	settings.ReasoningProvider = cfg.Reasoning.ActiveProvider
	if options.DatabaseAction != "" {
		result, prepareErr := store.PrepareDatabaseExpected(cfg.Database.Path, options.DatabaseAction, options.DatabaseConfirm, options.DatabaseExpectedFingerprint, settings)
		fatal(logger, "prepare_database", prepareErr)
		fatal(logger, "write_database_result", json.NewEncoder(os.Stdout).Encode(result))
		return
	}
	if requiresDatabaseDecision(cfg.Deployment) {
		compatibility := store.InspectDatabase(cfg.Database.Path)
		if compatibility.Status != "absent" && compatibility.Status != "current" {
			fatal(logger, "database_recovery_required", fmt.Errorf("database requires an explicit recovery decision (%s): %s; inspect with --database-inspect", compatibility.Status, compatibility.Reason))
		}
	}
	image, imageErr := os.Executable()
	fatal(logger, "locate_executable", imageErr)
	maintenance, holdErr := activationguard.Open(image + activationguard.FileSuffix)
	fatal(logger, "open_activation_guard", holdErr)
	var state *store.Store
	if manual != nil {
		state, err = manual.OpenDatabase(settings)
	} else {
		state, err = store.Open(cfg.Database.Path, settings)
	}
	fatal(logger, "open_database", err)
	defer func() {
		if daily != nil {
			if err := daily.Close(); err != nil {
				operational.Problem(logger, "app_shutdown", "release_daily_database", "deferred", err, "")
				return
			}
		}
		if err := closeDatabaseAfterManualSelection(manual, state); err != nil {
			operational.Problem(logger, "app_shutdown", "release_manual_database", "deferred", err, "")
		}
	}()
	persistedSettings, err := state.GetSettings(context.Background())
	fatal(logger, "read_settings", err)
	selected := cfg.Reasoning.ActiveProvider
	if !cfg.Reasoning.ProviderOverride && persistedSettings.ReasoningProvider != "" {
		selected = persistedSettings.ReasoningProvider
	}
	if err := cfg.Reasoning.Select(selected); err != nil {
		if cfg.Reasoning.ProviderOverride || persistedSettings.ReasoningProvider == "" {
			fatal(logger, "select_provider", err)
		}
		operational.Problem(logger, "reasoning_provider", "select_persisted", "fallback", err, "")
		if err := cfg.Reasoning.Select(cfg.Reasoning.ActiveProvider); err != nil {
			fatal(logger, "fallback_provider", err)
		}
	}
	if persistedSettings.ReasoningProvider != cfg.Reasoning.Provider {
		// Preserve the old active projection before changing the provider key;
		// this also covers config-driven provider switches that bypass the HTTP
		// settings path.
		if persistedSettings.ReasoningProvider != "" {
			persistedSettings.RememberReasoningProfileSet(persistedSettings.ReasoningProvider)
		}
		persistedSettings.ReasoningProvider = cfg.Reasoning.Provider
		fatal(logger, "save_settings", state.SaveSettings(context.Background(), persistedSettings))
	}
	previousExecutableMode := persistedSettings.ReasoningExecutableMode
	persistedSettings.ReasoningExecutableMode = codexselection.Mode(persistedSettings)
	if cfg.Reasoning.Provider == "codex-app-server" {
		cfg.Reasoning.Executable = codexselection.Request(cfg.Root, cfg.Reasoning.Executable, persistedSettings)
	}
	provider, err := reasoning.NewProvider(cfg)
	fatal(logger, "create_provider", err)
	if profileProvider, ok := provider.(reasoning.ProfileProvider); ok {
		restored, profileErr := reasoning.ActivateProviderProfileSet(&persistedSettings, cfg.Reasoning.Provider, profileProvider)
		fatal(logger, "activate_profiles", profileErr)
		migrated := false
		for _, current := range []*string{
			&persistedSettings.ReasoningAcquisitionProfile,
			&persistedSettings.ReasoningEvaluationProfile,
			&persistedSettings.ReasoningSemanticProfile,
			&persistedSettings.ReasoningAIDeepProfile,
		} {
			if replacement := reasoning.EnsureResolvableProfile(provider, *current); replacement != *current {
				*current = replacement
				migrated = true
			}
		}
		remembered := persistedSettings.RememberReasoningProfileSet(cfg.Reasoning.Provider)
		if restored || migrated || remembered {
			if restored {
				operational.Report(logger, "reasoning_provider", "restore_profiles", "completed", operational.Fields{"provider": operational.Provider(provider.Name())})
			}
			if migrated {
				operational.Report(logger, "reasoning_provider", "migrate_profiles", "completed", operational.Fields{"provider": operational.Provider(provider.Name())})
			}
			fatal(logger, "save_profiles", state.SaveSettings(context.Background(), persistedSettings))
		}
	}
	if executableRuntime, ok := provider.(reasoning.ExecutableRuntime); ok {
		resolved := executableRuntime.ExecutablePath()
		if persistedSettings.ReasoningExecutablePath != resolved || previousExecutableMode != persistedSettings.ReasoningExecutableMode {
			persistedSettings.ReasoningExecutablePath = resolved
			fatal(logger, "save_executable", state.SaveSettings(context.Background(), persistedSettings))
		}
	}
	var eventResolver semanticengine.Resolver
	if structured, ok := provider.(reasoning.StructuredInvoker); ok {
		eventResolver, err = semanticengine.NewStructuredResolver(cfg.Root, structured, cfg.Reasoning.SemanticEvent)
		fatal(logger, "semantic_resolver", err)
	}
	eventRuntime := semanticengine.New(state, eventResolver)
	runtime := engine.New(state, provider, cfg, logger, eventRuntime)
	if options.DevGLMPlanningShadow {
		shadow, shadowErr := reasoning.NewGLMPlanningShadow(cfg)
		if shadowErr != nil {
			operational.Problem(logger, "planning_shadow", "setup", "unavailable", shadowErr, "")
		} else {
			runtime.SetGLMPlanningShadow(shadow)
			operational.Report(logger, "planning_shadow", "setup", "enabled", operational.Fields{"model": reasoning.GLMPlanningShadowModel})
		}
	}
	fatal(logger, "bind_derivation_hold", runtime.BindNativeDerivationHold(maintenance))
	if daily != nil {
		fatal(logger, "attach_daily", daily.Attach(runtime))
	}
	fatal(logger, "reconcile_daily", runtime.ReconcileXDailyStartup(context.Background()))
	if manual != nil {
		defer runtime.Shutdown()
		fatal(logger, "attach_manual", manual.Attach(context.Background(), runtime))
	}
	if cfg.Dev {
		// Inspection cannot start an owner or derive a session. A selected
		// process still requires explicit one-use start consent through the API.
		manual := runtime.XManualStatus()
		operational.Report(logger, "source_daily", "manual_provider", "observed", operational.Fields{"available": manual.Available, "state": manual.State})
	}
	mediaInspector := mediaprovenance.NewC2PAToolInspector(cfg.MediaProvenance.C2PAToolPath)
	runtime.SetMediaProvenanceInspector(mediaInspector)
	if mediaInspector.Available() {
		operational.Report(logger, "media_provenance", "setup", "ready", nil)
	} else {
		operational.Report(logger, "media_provenance", "setup", "unavailable", nil)
	}
	if structured, ok := provider.(reasoning.StructuredInvoker); ok {
		aiResolver, err := aidetector.NewStructuredResolver(cfg.Root, structured, cfg.Reasoning.AIDetection)
		fatal(logger, "detection_resolver", err)
		runtime.SetAIDeepResolver(aiResolver)
		topicResolver, err := livingtopics.NewStructuredResolver(cfg.Root, structured, cfg.Reasoning.Evaluation)
		fatal(logger, "topics_resolver", err)
		runtime.SetLivingTopicsResolver(topicResolver)
	}
	if err := runtime.ResumeLivingTopicRouting(context.Background()); err != nil {
		operational.Problem(logger, "living_topics", "resume_routing", "degraded", err, "")
	}
	if err := runtime.ResumeLivingTopicActivation(context.Background()); err != nil {
		operational.Problem(logger, "living_topics", "resume_activation", "degraded", err, "")
	}
	if err := runtime.ResumeLivingTopicUnderstanding(context.Background()); err != nil {
		operational.Problem(logger, "living_topics", "resume_understanding", "degraded", err, "")
	}
	server, err := httpapi.New(cfg, state, runtime, logger)
	fatal(logger, "create_http_server", err)
	// Export remains off: only an explicit adapter config can select an endpoint.
	// No ambient OTEL environment or global provider config is adopted here.
	diagnostics, diagnosticErr := observability.New(context.Background(), observability.Config{})
	if diagnosticErr != nil {
		operational.Problem(logger, "telemetry", "setup", "unavailable", diagnosticErr, "")
	} else {
		server.SetDiagnosticsObserver(diagnostics)
		defer func() {
			shutdown, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := diagnostics.Shutdown(shutdown); err != nil {
				operational.Problem(logger, "telemetry", "shutdown", "incomplete", err, "")
			}
		}()
	}
	resumed, err := runtime.ResumePendingReasoning(context.Background())
	fatal(logger, "resume_runs", err)
	fatal(logger, "resume_provenance", runtime.ResumeMediaProvenance(context.Background()))
	address, err := server.Start()
	fatal(logger, "listen", err)
	operational.Report(logger, "app_startup", "listening", "ready", operational.Fields{"version": domain.ApplicationVersion, "provider": operational.Provider(provider.Name())})
	if resumed > 0 {
		operational.Report(logger, "app_startup", "resume_runs", "completed", operational.Fields{"count": resumed})
	}
	var shell *appshell.Session
	var capture *captureruntime.Manager
	collectionCtx, stopCollection := context.WithCancel(context.Background())
	defer stopCollection()
	var collector *collection.Coordinator
	var initializeCapture func(context.Context) error
	if options.AppShell {
		if resetErr := discardLegacyProfileResetMarker(state, logger); resetErr != nil {
			operational.Problem(logger, "app_startup", "reset_marker_cleanup", "failed", resetErr, "")
		}
		shell, capture, collector, initializeCapture = launchAppShell(logger, options, cfg, address.String(), server, runtime, collectionCtx)
		if capture != nil {
			fatal(logger, "attach_capture", runtime.AttachCaptureRuntime(context.Background(), capture))
			fatal(logger, "configure_split", server.SetSplitCaptureRuntime(capture))
			startupCtx, cancelStartup := context.WithTimeout(collectionCtx, 10*time.Second)
			startupErr := initializeCapture(startupCtx)
			cancelStartup()
			fatal(logger, "capture_readiness", startupErr)
			fatal(logger, "transition_readiness", capture.SetTransitionReadiness(func(ctx context.Context) error {
				if capture.Snapshot().Driver == "headless" {
					return ctx.Err()
				}
				return server.SplitCaptureReplacementReadiness(ctx)
			}))
			runtime.AttachCollectionCoordinator(collector)
			settings, settingsErr := runtime.Settings(collectionCtx)
			fatal(logger, "read_capture_settings", settingsErr)
			collector.Request(settings.CollectionMode)
			collector.Start(collectionCtx)
			runtime.StartHeadlessCollection(collectionCtx)
			server.SetOpenExtensionsAction(capture.OpenExtensionsPage)
			server.SetAppShellPID(capture.PID)
		} else {
			server.SetOpenExtensionsAction(shell.OpenExtensionsPage)
			server.SetAppShellPID(shell.PID)
		}
	}
	if daily != nil {
		ordinaryProfile := browserProfilePath(options, cfg)
		if cfg.WindowsCaptureSplit {
			ordinaryProfile, _, err = appshell.SplitProfilePaths(ordinaryProfile)
			fatal(logger, "split_profile_paths", err)
		}
		fatal(logger, "configure_native_daily", engine.ConfigureXDailyNativeRuntime(collectionCtx, runtime, ordinaryProfile, options.ChromiumPath))
	}
	fatal(logger, "resume_daily_scheduler", runtime.ResumeXDailyScheduling(collectionCtx))
	runtime.StartAutoUpdateScheduler()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	select {
	case <-signals:
	case <-server.ShutdownRequested():
	case <-shell.Done():
		operational.Report(logger, "app_shell", "window", "closed", nil)
	case <-capture.Done():
		operational.Report(logger, "app_shell", "capture_process", "unexpected_exit", nil)
	}
	shutdownStarted := time.Now()
	stopCollection()
	shell.Cancel() // Stop accepting/relaunching windows as soon as shutdown wins.
	operational.Report(logger, "app_shutdown", "request", "accepted", nil)
	runtime.Shutdown()
	diagnosticDrain, cancelDiagnosticDrain := context.WithTimeout(context.Background(), 31*time.Second)
	if err := runtime.DrainInstagramReaderDiagnostic(diagnosticDrain); err != nil {
		operational.Problem(logger, "app_shutdown", "instagram_reader_cleanup", "incomplete", err, "")
	}
	cancelDiagnosticDrain()
	if daily != nil && cfg.Capture.XUsesMainProfile() {
		// Borrowers and disposable collectors finish before the shared browser
		// transport/Job is released. SQLite's existing deferred gate remains.
		if err := daily.Close(); err != nil {
			operational.Problem(logger, "app_shutdown", "borrower_cleanup", "uncertain", err, "")
		}
	}
	shell.Terminate()
	capture.Terminate()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	if err := server.Stop(ctx); err != nil {
		operational.Problem(logger, "app_shutdown", "http", "degraded", err, "")
	}
	cancel()
	if !runtime.WaitForIdle(2 * time.Second) {
		operational.Report(logger, "app_shutdown", "drain_workers", "incomplete", nil)
	}
	if err := runtime.CloseProvider(); err != nil {
		operational.Problem(logger, "reasoning_provider", "shutdown", "failed", err, "")
	}
	operational.Report(logger, "app_shutdown", "complete", "completed", operational.Fields{"duration_ms": operational.Milliseconds(time.Since(shutdownStarted))})
}

// Pending startup may outlast HTTP shutdown's grace period. Join its cleanup
// before closing SQLite; uncertain cleanup retains the database until exit.
func closeDatabaseAfterManualSelection(manual, database io.Closer) error {
	if manual != nil {
		if err := manual.Close(); err != nil {
			return err
		}
	}
	return database.Close()
}

func existingInstance(healthURL string, timeout time.Duration) (string, bool) {
	client := &http.Client{Timeout: timeout}
	response, err := client.Get(healthURL)
	if err != nil {
		return "", false
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", false
	}
	var payload struct {
		Status  string `json:"status"`
		Version string `json:"version"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&payload); err != nil {
		return "", false
	}
	if payload.Status != "ok" {
		return "", false
	}
	return payload.Version, true
}

func runtimeCandidateProbe(configVersion, schemaVersion int) (map[string]any, error) {
	probe := map[string]any{
		"status":                "ok",
		"version":               domain.ApplicationVersion,
		"runtime":               "go",
		"bridgeContractVersion": domain.BridgeContractVersion,
		"configVersion":         configVersion,
	}
	if schemaVersion == 2 {
		probe["databaseSchemaVersion"] = store.SchemaVersion
	} else if schemaVersion != 1 {
		return nil, fmt.Errorf("unsupported runtime candidate probe schema %d", schemaVersion)
	}
	return probe, nil
}

func discoverCodex(options config.Options) int {
	requested := codexselection.Request("", options.CodexPath, domain.Settings{})
	result, err := codexruntime.Discover(context.Background(), requested)
	if encodeErr := json.NewEncoder(os.Stdout).Encode(result); encodeErr != nil {
		return 3
	}
	if err != nil {
		return 2
	}
	return 0
}

func discoverChromium(options config.Options) int {
	result, err := appshell.Discover(context.Background(), options.ChromiumPath)
	if encodeErr := json.NewEncoder(os.Stdout).Encode(result); encodeErr != nil {
		return 3
	}
	if err != nil {
		return 2
	}
	return 0
}

func launchAppShell(logger *log.Logger, options config.Options, cfg config.Config, address string, server *httpapi.Server, runtime *engine.Engine, collectionCtx context.Context) (*appshell.Session, *captureruntime.Manager, *collection.Coordinator, func(context.Context) error) {
	launchTrace := captureactivity.NewLaunch(logger)
	discoveryCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	result, err := appshell.Discover(discoveryCtx, options.ChromiumPath)
	cancel()
	if err != nil {
		var discoveryErr *appshell.DiscoveryError
		if errors.As(err, &discoveryErr) {
			for index, attempt := range discoveryErr.Result.Attempts {
				operational.Report(logger, "app_shell", "chromium_discovery", "rejected", operational.Fields{"attempt": index + 1, "source": operational.DiscoverySource(attempt.Source)})
			}
		}
		fatal(logger, "discover_chromium", err)
	}
	target := address
	if !strings.Contains(target, "://") {
		target = "http://" + target
	}
	if !strings.HasSuffix(target, "/") {
		target += "/"
	}
	identity, err := appShellIdentity(options, cfg)
	fatal(logger, "resolve_identity", err)
	profilePath := browserProfilePath(options, cfg)
	var capture *appshell.Window
	var captureProfileDirectory string
	extensionPath := options.BridgeExtensionPath
	uiResult := result
	var uiArgs []string
	manualUI := options.DevelopmentSessionReuse == "native-x-manual"
	var sidecarExecutable string
	if cfg.WindowsCaptureSplit || manualUI {
		var executableErr error
		sidecarExecutable, executableErr = os.Executable()
		fatal(logger, "locate_executable", executableErr)
		uiDiscoveryCtx, cancelUIDiscovery := context.WithTimeout(context.Background(), 10*time.Second)
		uiResult, err = appshell.DiscoverSplitUI(uiDiscoveryCtx, sidecarExecutable, options.UIChromiumPath)
		cancelUIDiscovery()
		fatal(logger, "discover_ui_browser", err)
	}
	if manualUI {
		helperInfo, helperErr := os.Stat(filepath.Join(filepath.Dir(sidecarExecutable), "aku-reader-broker.exe"))
		if helperErr != nil || !helperInfo.Mode().IsRegular() {
			fatal(logger, "locate_manual_helper", errors.New("manual X UI native helper is not packaged"))
		}
		uiArgs = []string{"--disable-extensions-except=" + extensionPath, "--enable-features=LaunchWindowsNativeHostsDirectly"}
	}
	if cfg.WindowsCaptureSplit {
		fatal(logger, "require_host_retirement", server.RequireHostOnlyRetirement())
		uiBrokerPath := filepath.Join(filepath.Dir(sidecarExecutable), "ui-reader-broker")
		if _, err := os.Stat(filepath.Join(uiBrokerPath, "manifest.json")); err != nil {
			fatal(logger, "locate_reader_broker", fmt.Errorf("Windows split UI reader broker is not packaged: %w", err))
		}
		captureProfile, uiProfile, err := appshell.SplitProfilePaths(profilePath)
		fatal(logger, "split_profile_paths", err)
		captureProfileDirectory, err = appshell.ResolveProfileDirectory(captureProfile)
		fatal(logger, "resolve_profile", err)
		captureURL, err := server.SplitCaptureLaunchURL(target)
		fatal(logger, "capture_url", err)
		mainTransport, err := engine.MainBrowserTransportOptions(cfg)
		fatal(logger, "configure_transport", err)
		if mainTransport != nil {
			mainTransport.ObserveWorkerDiagnostic = func(raw json.RawMessage) {
				if appshell.ValidateMainBrowserWorkerDiagnostic(raw) {
					operational.Report(logger, "main_transport_worker", "snapshot", "observed", raw)
				}
			}
		}
		server.CaptureStartupTrace().Launch()
		capture, err = appshell.Launch(context.Background(), appshell.LaunchOptions{
			Executable: result.Executable, ExtensionPath: extensionPath,
			UserDataDir: captureProfile, URL: captureURL, StartMinimized: true, PrivateCDP: true, ExtraArgs: []string{"--start-minimized", "--profile-directory=" + captureProfileDirectory},
			MainBrowserTransport: mainTransport, MainProfileDirectory: captureProfileDirectory,
		})
		fatal(logger, "launch_capture", err)
		containment, err := capture.StartCaptureContainment(logger)
		fatal(logger, "start_containment", err)
		if containment != nil {
			server.SetSplitReaderPreparation(containment.PrepareReader)
			server.SetSplitReaderBroker(containment.PrepareBrokerReader)
			if source, ok := containment.(interface {
				PrepareSourceWindowLifetime(context.Context, string) error
			}); ok {
				server.SetSplitSourceWindowPreparation(source.PrepareSourceWindowLifetime)
			}
		}
		// Never copy or relocate the signed-in profile. The UI receives a
		// separate directory and no AkuBridge extension.
		profilePath = uiProfile
		extensionPath = uiBrokerPath
		uiArgs = []string{"--disable-extensions-except=" + extensionPath, "--enable-features=LaunchWindowsNativeHostsDirectly"}
		launchTrace.Process("capture_started", capture.PID(), result.Executable, result.Version)
	}
	showStartupStatus, markStartupReady, statusErr := startupStatusPolicy(cfg.Deployment, profilePath)
	if statusErr != nil {
		operational.Problem(logger, "app_shell", "status_policy", "failed", statusErr, "")
	}
	startupLogPath := isolatedChromiumStartupLogPath(profilePath)
	if startupLogPath != "" {
		operational.Report(logger, "app_shell", "startup_diagnostics", "enabled", nil)
	}
	window, err := appshell.LaunchSession(context.Background(), appshell.LaunchOptions{
		Executable:            uiResult.Executable,
		ExtensionPath:         extensionPath,
		IconPath:              appShellIconPath(options.BridgeExtensionPath),
		Identity:              identity,
		UserDataDir:           profilePath,
		URL:                   target,
		StartupLogPath:        startupLogPath,
		ExtraArgs:             uiArgs,
		SuppressStartupWindow: !showStartupStatus,
		OnStartupReady: func() {
			if markStartupReady != nil {
				if err := markStartupReady(); err != nil {
					operational.Problem(logger, "app_shell", "status_marker", "failed", err, "")
				}
			}
		},
	}, server.SetAppShellStartup, func(err error) { operational.Problem(logger, "app_shell", "recovery", "degraded", err, "") })
	if err != nil {
		capture.Terminate()
		fatal(logger, "launch_shell", err)
	}
	launchTrace.Process("ui_started", window.PID(), uiResult.Executable, uiResult.Version)
	if cfg.WindowsCaptureSplit || manualUI {
		go func() {
			if err := window.ServeReaderBroker(context.Background(), server.HandleReaderBroker); err != nil {
				operational.Problem(logger, "reader_broker", "setup", "unavailable", err, "")
			}
		}()
	}
	var captureManager *captureruntime.Manager
	var collector *collection.Coordinator
	var initializeCapture func(context.Context) error
	if capture != nil {
		captureManager, err = captureruntime.New(capture)
		fatal(logger, "create_capture_manager", err)
		if cfg.Capture.XUsesMainProfile() {
			fatal(logger, "retain_capture_manager", captureManager.RetainForProcess())
			fatal(logger, "configure_x_daily", engine.ConfigureXMainDailyRuntime(collectionCtx, runtime, capture, captureManager, captureProfilePath(options, cfg), captureProfileDirectory, result.Executable))
		}
		fatal(logger, "configure_ig_daily", engine.ConfigureInstagramMainDailyRuntime(collectionCtx, runtime, capture, captureManager, captureProfilePath(options, cfg), captureProfileDirectory, result.Executable))
		if runtime.InstagramReaderDiagnosticConfigured() {
			server.SetInstagramReaderDiagnosticAction(instagramReaderDiagnosticAction(capture.CaptureProtocol(), captureManager, capture.PID()))
			server.SetInstagramBootstrapDiagnosticAction(instagramBootstrapDiagnosticAction(capture.CaptureProtocol(), captureManager, capture.PID()))
			server.SetInstagramBootstrapCorroborationDiagnosticAction(instagramBootstrapCorroborationDiagnosticAction(capture.CaptureProtocol(), captureManager, capture.PID()))
		}
		captureManager.SetTimingObserver(func(stage string, elapsed time.Duration, ok bool) {
			captureactivity.Transition(logger, stage, elapsed, &ok)
		})
		captureProfile, _, profileErr := appshell.SplitProfilePaths(browserProfilePath(options, cfg))
		fatal(logger, "split_profile_paths", profileErr)
		executable, exeErr := os.Executable()
		fatal(logger, "locate_executable", exeErr)
		workerRoot := filepath.Join(filepath.Dir(executable), "headless-worker")
		headlessOptions := headless.Options{Node: filepath.Join(workerRoot, "node.exe"), Worker: filepath.Join(workerRoot, "worker.mjs"), Pin: filepath.Join(workerRoot, "node.pin.json"), Chrome: result.Executable, Profile: captureProfile, ProfileDirectory: captureProfileDirectory, BridgePath: options.BridgeExtensionPath}
		headlessOptions.Diagnostics = headless.NewWorkerDiagnostics(collectionCtx, logger)
		if !filepath.IsAbs(headlessOptions.BridgePath) {
			headlessOptions.BridgePath, _ = filepath.Abs(headlessOptions.BridgePath)
		}
		headlessOptions.WindowedIdentitySources = []domain.Source{domain.SourceX}
		// The manager serializes retirement and replacement. Each headed owner
		// supplies its own sample before giving up the signed-in profile.
		var windowedIdentity json.RawMessage
		bindQuiet := func(ctx context.Context, window *appshell.Window, generation uint64, hostURL string) error {
			targets, err := quiet.NewTargets(ctx, window.CaptureProtocol())
			if err != nil {
				if targets != nil {
					cleanupCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
					cleanupErr := targets.Close(cleanupCtx)
					cancel()
					if cleanupErr != nil {
						return fmt.Errorf("%w; Quiet partial session cleanup unverified: %v", err, cleanupErr)
					}
				}
				return err
			}
			worker := quiet.NewWorker(targets, headlessOptions)
			sampled := false
			if err := window.SetCaptureHandoff(func(ctx context.Context) error {
				if err := worker.Retire(ctx); err != nil {
					return err
				}
				if !sampled {
					sampled = true
					identityCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
					windowedIdentity, _ = appshell.ReadWindowedIdentity(identityCtx, window.CaptureProtocol(), hostURL)
					cancel()
				}
				return server.CloseSplitCaptureHost(ctx)
			}); err != nil {
				_ = worker.Retire(ctx)
				return err
			}
			windowedIdentity = nil
			collector.SetBrowserCollector(generation, worker)
			return nil
		}
		collector = collection.NewCoordinator(captureManager, func(ctx context.Context, mode string, generation uint64) (captureruntime.Process, error) {
			if mode != "native_reader" {
				server.SetSplitDirectNativeReader(nil, nil)
				server.SetNativeReaderCloseAction(nil)
			}
			if err := server.RotateSplitCapture(); err != nil {
				return nil, err
			}
			if mode == "headless" {
				options := headlessOptions
				options.WindowedIdentity = windowedIdentity
				return headless.Launch(ctx, options)
			}
			if mode == "native_reader" {
				origin := strings.TrimRight(target, "/")
				idleURL := origin + "/native-reader-idle"
				timing := captureactivity.BeginReaderLaunch(logger, generation)
				process, err := appshell.Launch(ctx, appshell.LaunchOptions{Executable: result.Executable, UserDataDir: captureProfile, URL: idleURL, NormalWindow: true, PrivateCDP: true, ExtraArgs: []string{"--disable-extensions", "--profile-directory=" + captureProfileDirectory}})
				if err != nil {
					timing.Finish(err)
					return process, err
				}
				timing.Next("reader_ready")
				reader, err := appshell.NewNativeReader(ctx, process, idleURL, logger)
				timing.Finish(err)
				if err != nil {
					return reader, err
				}
				server.SetSplitDirectNativeReader(func(ctx context.Context, id, url, marker string) (readerbroker.NativePostPreparation, error) {
					return reader.PrepareNativePost(ctx, id, url, origin+marker)
				}, reader.ReplacementReadiness)
				server.SetNativeReaderCloseAction(reader.CloseOwnedWindow)
				return reader, nil
			}
			url, err := server.SplitCaptureLaunchURL(target)
			if err != nil {
				return nil, err
			}
			process, err := appshell.Launch(ctx, appshell.LaunchOptions{Executable: result.Executable, ExtensionPath: options.BridgeExtensionPath, UserDataDir: captureProfile, URL: url, StartMinimized: true, PrivateCDP: true, ExtraArgs: []string{"--start-minimized", "--profile-directory=" + captureProfileDirectory}})
			if err != nil {
				return process, err
			}
			if err := bindQuiet(ctx, process, generation, url); err != nil {
				return process, err
			}
			containment, err := process.StartCaptureContainment(logger)
			if err != nil {
				return process, err
			}
			server.SetSplitReaderPreparation(containment.PrepareReader)
			server.SetSplitReaderBroker(containment.PrepareBrokerReader)
			if source, ok := containment.(interface {
				PrepareSourceWindowLifetime(context.Context, string) error
			}); ok {
				server.SetSplitSourceWindowPreparation(source.PrepareSourceWindowLifetime)
			}
			if err := appshell.NavigateCaptureHost(ctx, process.CaptureProtocol(), url); err != nil {
				return process, err
			}
			tick := time.NewTicker(100 * time.Millisecond)
			defer tick.Stop()
			for {
				if server.SplitCaptureReplacementReadiness(ctx) == nil {
					return process, nil
				}
				select {
				case <-ctx.Done():
					return process, ctx.Err()
				case <-tick.C:
				}
			}
		}, func() error { return headless.Validate(headlessOptions) })
		collector.SetTimingObserver(func(stage string, elapsed time.Duration) {
			captureactivity.Transition(logger, stage, elapsed, nil)
		})
		initializeCapture = func(ctx context.Context) error {
			url, err := server.SplitCaptureLaunchURL(target)
			if err != nil {
				return err
			}
			if !cfg.Capture.XUsesMainProfile() {
				if err := bindQuiet(ctx, capture, captureManager.Snapshot().Generation, url); err != nil {
					return err
				}
			}
			err = appshell.NavigateCaptureHost(ctx, capture.CaptureProtocol(), url)
			server.CaptureStartupTrace().Navigation(err == nil)
			return err
		}
	}
	return window, captureManager, collector, initializeCapture
}

func captureProfilePath(options config.Options, cfg config.Config) string {
	profile, _, _ := appshell.SplitProfilePaths(browserProfilePath(options, cfg))
	return profile
}

func isolatedChromiumStartupLogPath(profilePath string) string {
	if os.Getenv("AKUBROWSER_CHROMIUM_STARTUP_DIAGNOSTICS") != "1" ||
		!strings.HasPrefix(os.Getenv("AKUBROWSER_ISOLATED_TEST_CREDENTIAL_NAMESPACE"), "AkuBrowserTest-") {
		return ""
	}
	return filepath.Join(profilePath, "chrome-startup.log")
}

func appShellIdentity(options config.Options, cfg config.Config) (appshell.ApplicationIdentity, error) {
	identity := appshell.ApplicationIdentity{
		ID:              strings.TrimSpace(options.AppUserModelID),
		RelaunchCommand: strings.TrimSpace(options.AppRelaunchCommand),
		DisplayName:     strings.TrimSpace(options.AppRelaunchDisplayName),
	}
	if identity.ID != "" || identity.RelaunchCommand != "" || identity.DisplayName != "" || !options.Dev {
		return identity, nil
	}
	workspaceRoot := filepath.Dir(cfg.Root)
	launcherPath := filepath.Join(workspaceRoot, "AkuBrowser", "launcher", "AkuBrowserLauncher.exe")
	launcherArgument, err := quoteWindowsRelaunchArgument(launcherPath)
	if err != nil {
		return appshell.ApplicationIdentity{}, err
	}
	workspaceArgument, err := quoteWindowsRelaunchArgument(workspaceRoot)
	if err != nil {
		return appshell.ApplicationIdentity{}, err
	}
	return appshell.ApplicationIdentity{
		ID:              "AI4U.AkuBrowser.Development",
		RelaunchCommand: launcherArgument + " --development-workspace " + workspaceArgument,
		DisplayName:     "AkuBrowser Development",
	}, nil
}

func quoteWindowsRelaunchArgument(value string) (string, error) {
	if strings.ContainsRune(value, '"') {
		return "", fmt.Errorf("Windows relaunch argument contains a quote")
	}
	return `"` + value + `"`, nil
}

func appShellIconPath(extensionPath string) string {
	if value := strings.TrimSpace(extensionPath); value != "" {
		return filepath.Join(value, "icons", "icon-128.png")
	}
	return ""
}

func browserProfilePath(options config.Options, cfg config.Config) string {
	if options.DevelopmentSessionReuse == "native-x-manual" {
		return filepath.Join(cfg.Root, "profiles", "ui", "profile")
	}
	if profilePath := strings.TrimSpace(options.BrowserProfilePath); profilePath != "" {
		return profilePath
	}
	return filepath.Join(cfg.Root, "runtime", "app-profile")
}

// discardLegacyProfileResetMarker retires the pre-0.8 full-reset behavior.
// Browser profiles contain the product's Bridge installation and must never
// be deleted as part of application-data reset.
func discardLegacyProfileResetMarker(state *store.Store, logger *log.Logger) error {
	ctx := context.Background()
	pending, err := state.PendingAppProfileReset(ctx)
	if err != nil {
		return fmt.Errorf("read legacy profile reset marker: %w", err)
	}
	if !pending {
		return nil
	}
	if err := state.ConsumePendingAppProfileReset(ctx); err != nil {
		return fmt.Errorf("clear legacy profile reset marker: %w", err)
	}
	operational.Report(logger, "app_shell", "reset_marker", "discarded", nil)
	return nil
}

func fatal(logger *log.Logger, phase d.Phase, err error) {
	if err != nil {
		var discoveryErr *codexruntime.DiscoveryError
		if errors.As(err, &discoveryErr) {
			for index, attempt := range discoveryErr.Result.Attempts {
				operational.Report(logger, "app_startup", "codex_discovery", "rejected", operational.Fields{"attempt": index + 1, "source": operational.DiscoverySource(attempt.Source)})
			}
		}
		operational.Problem(logger, "app_startup", phase, "failed", err, "")
		os.Exit(1)
	}
}
