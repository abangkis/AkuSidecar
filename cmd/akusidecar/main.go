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

	"github.com/abangkis/AkuSidecar/internal/aidetector"
	"github.com/abangkis/AkuSidecar/internal/appshell"
	"github.com/abangkis/AkuSidecar/internal/codexruntime"
	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/engine"
	semanticengine "github.com/abangkis/AkuSidecar/internal/eventengine"
	"github.com/abangkis/AkuSidecar/internal/httpapi"
	"github.com/abangkis/AkuSidecar/internal/mediaprovenance"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/AkuSidecar/internal/store"
)

func main() {
	logger := log.New(os.Stdout, "AkuSidecar ", log.LstdFlags|log.LUTC|log.Lmsgprefix)
	options := config.ParseFlags()
	if options.DiscoverCodex {
		os.Exit(discoverCodex(options))
	}
	if options.DiscoverChromium {
		os.Exit(discoverChromium(options))
	}
	cfg, err := config.Load(options)
	fatal(logger, err)
	if options.RuntimeCandidateProbe {
		probe, probeErr := runtimeCandidateProbe(cfg.Version, options.RuntimeCandidateProbeSchema)
		fatal(logger, probeErr)
		fatal(logger, json.NewEncoder(os.Stdout).Encode(probe))
		return
	}
	if version, running := existingInstance(fmt.Sprintf("http://%s/api/health", cfg.Server.HostPort()), time.Second); running {
		logger.Printf("another AkuSidecar instance version=%s is already serving this address; start cancelled to avoid a second instance", version)
		os.Exit(5)
	}
	settings := domain.DefaultSettings(cfg.Capture.Profile, cfg.Capture.Visibility, cfg.Preference.Mode, cfg.Capture.OpenMissingSource)
	settings.ReasoningProvider = cfg.Reasoning.ActiveProvider
	state, err := store.Open(cfg.Database.Path, settings)
	fatal(logger, err)
	defer state.Close()
	persistedSettings, err := state.GetSettings(context.Background())
	fatal(logger, err)
	selected := cfg.Reasoning.ActiveProvider
	if !cfg.Reasoning.ProviderOverride && persistedSettings.ReasoningProvider != "" {
		selected = persistedSettings.ReasoningProvider
	}
	if err := cfg.Reasoning.Select(selected); err != nil {
		if cfg.Reasoning.ProviderOverride || persistedSettings.ReasoningProvider == "" {
			fatal(logger, err)
		}
		logger.Printf("persisted reasoning provider %q is not declared; falling back to %q (%v)", selected, cfg.Reasoning.ActiveProvider, err)
		if err := cfg.Reasoning.Select(cfg.Reasoning.ActiveProvider); err != nil {
			fatal(logger, err)
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
		fatal(logger, state.SaveSettings(context.Background(), persistedSettings))
	}
	if options.CodexPath == "" && cfg.Reasoning.Provider == "codex-app-server" && persistedSettings.ReasoningExecutablePath != "" {
		persistedPath := persistedSettings.ReasoningExecutablePath
		if _, statErr := os.Stat(persistedPath); statErr == nil {
			cfg.Reasoning.Executable = persistedPath
			if codexruntime.IsManagedPath(persistedPath) {
				if discovered, discoverErr := codexruntime.Discover(context.Background(), ""); discoverErr != nil {
					logger.Printf("could not refresh managed Codex executable; keeping path=%q error=%v", persistedPath, discoverErr)
				} else if discovered.Executable != "" && !strings.EqualFold(discovered.Executable, persistedPath) {
					logger.Printf("refreshed managed Codex executable old=%q new=%q version=%q", persistedPath, discovered.Executable, discovered.Version)
					cfg.Reasoning.Executable = discovered.Executable
				}
			}
		} else {
			logger.Printf("persisted Codex executable is unavailable; rediscovering path=%q error=%v", persistedPath, statErr)
			persistedSettings.ReasoningExecutablePath = ""
		}
	}
	provider, err := reasoning.NewProvider(cfg)
	fatal(logger, err)
	if profileProvider, ok := provider.(reasoning.ProfileProvider); ok {
		restored, profileErr := reasoning.ActivateProviderProfileSet(&persistedSettings, cfg.Reasoning.Provider, profileProvider)
		fatal(logger, profileErr)
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
				logger.Printf("restored provider-specific reasoning profiles for provider %s", provider.Name())
			}
			if migrated {
				logger.Printf("migrated reasoning profiles for provider %s", provider.Name())
			}
			fatal(logger, state.SaveSettings(context.Background(), persistedSettings))
		}
	}
	if executableRuntime, ok := provider.(reasoning.ExecutableRuntime); ok {
		resolved := executableRuntime.ExecutablePath()
		if persistedSettings.ReasoningExecutablePath != resolved {
			persistedSettings.ReasoningExecutablePath = resolved
			fatal(logger, state.SaveSettings(context.Background(), persistedSettings))
		}
	}
	var eventResolver semanticengine.Resolver
	if structured, ok := provider.(reasoning.StructuredInvoker); ok {
		eventResolver, err = semanticengine.NewStructuredResolver(cfg.Root, structured, cfg.Reasoning.SemanticEvent)
		fatal(logger, err)
	}
	eventRuntime := semanticengine.New(state, eventResolver)
	runtime := engine.New(state, provider, cfg, logger, eventRuntime)
	mediaInspector := mediaprovenance.NewC2PAToolInspector(cfg.MediaProvenance.C2PAToolPath)
	runtime.SetMediaProvenanceInspector(mediaInspector)
	if mediaInspector.Available() {
		logger.Printf("C2PA image provenance ready executable=%s", mediaInspector.Executable())
	} else {
		logger.Printf("C2PA image provenance unavailable; set AKU_C2PATOOL_PATH or place c2patool beside AkuSidecar")
	}
	if structured, ok := provider.(reasoning.StructuredInvoker); ok {
		aiResolver, err := aidetector.NewStructuredResolver(cfg.Root, structured, cfg.Reasoning.AIDetection)
		fatal(logger, err)
		runtime.SetAIDeepResolver(aiResolver)
	}
	server, err := httpapi.New(cfg, state, runtime, logger)
	fatal(logger, err)
	resumed, err := runtime.ResumePendingReasoning(context.Background())
	fatal(logger, err)
	fatal(logger, runtime.ResumeMediaProvenance(context.Background()))
	address, err := server.Start()
	fatal(logger, err)
	runtime.StartAutoUpdateScheduler()
	logger.Printf("version=%s runtime=go address=http://%s provider=%s database=%s", domain.ApplicationVersion, address, provider.Name(), state.Path())
	if resumed > 0 {
		logger.Printf("resumed_reasoning_runs=%d from_durable_capture=true", resumed)
	}
	var shell *appshell.Window
	if options.AppShell {
		if resetErr := resetAppProfileIfPending(state, browserProfilePath(options, cfg), logger); resetErr != nil {
			logger.Printf("staged app profile reset failed; keeping the existing profile: %v", resetErr)
		}
		shell = launchAppShell(logger, options, cfg, address.String())
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	select {
	case <-signals:
	case <-server.ShutdownRequested():
	case <-shell.Done():
		logger.Printf("app shell window closed")
	}
	shutdownStarted := time.Now()
	logger.Printf("shutdown requested")
	runtime.Shutdown()
	shell.Terminate()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	if err := server.Stop(ctx); err != nil {
		logger.Printf("HTTP shutdown degraded: %v", err)
	}
	cancel()
	if !runtime.WaitForIdle(2 * time.Second) {
		logger.Printf("background work did not become idle before provider shutdown")
	}
	if err := runtime.CloseProvider(); err != nil {
		logger.Printf("reasoning provider shutdown failed: %v", err)
	}
	logger.Printf("shutdown completed duration_ms=%d", time.Since(shutdownStarted).Milliseconds())
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
	result, err := codexruntime.Discover(context.Background(), options.CodexPath)
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

func launchAppShell(logger *log.Logger, options config.Options, cfg config.Config, address string) *appshell.Window {
	discoveryCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	result, err := appshell.Discover(discoveryCtx, options.ChromiumPath)
	cancel()
	if err != nil {
		var discoveryErr *appshell.DiscoveryError
		if errors.As(err, &discoveryErr) {
			for index, attempt := range discoveryErr.Result.Attempts {
				logger.Printf("Chromium discovery attempt=%d source=%s path=%q reason=%s", index+1, attempt.Source, attempt.Path, attempt.Reason)
			}
		}
		logger.Fatalf("startup failed: resolve app shell Chromium: %v", err)
	}
	target := address
	if !strings.Contains(target, "://") {
		target = "http://" + target
	}
	if !strings.HasSuffix(target, "/") {
		target += "/"
	}
	identity, err := appShellIdentity(options, cfg)
	fatal(logger, err)
	window, err := appshell.Launch(context.Background(), appshell.LaunchOptions{
		Executable:    result.Executable,
		ExtensionPath: options.BridgeExtensionPath,
		IconPath:      appShellIconPath(options.BridgeExtensionPath),
		Identity:      identity,
		UserDataDir:   browserProfilePath(options, cfg),
		URL:           target,
	})
	fatal(logger, err)
	logger.Printf("app_shell executable=%s version=%s pid=%d url=%s", result.Executable, result.Version, window.PID(), target)
	return window
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
	if profilePath := strings.TrimSpace(options.BrowserProfilePath); profilePath != "" {
		return profilePath
	}
	return filepath.Join(cfg.Root, "runtime", "app-profile")
}

// resetAppProfileIfPending applies the isolated-browser-profile wipe staged by
// a full reset. Chromium must not be running for this to be safe, so it runs
// before the app shell launches; the marker is consumed only after a
// successful removal so a failed wipe is retried on the next start.
func resetAppProfileIfPending(state *store.Store, profilePath string, logger *log.Logger) error {
	ctx := context.Background()
	pending, err := state.PendingAppProfileReset(ctx)
	if err != nil {
		return fmt.Errorf("read staged profile reset: %w", err)
	}
	if !pending {
		return nil
	}
	if err := os.RemoveAll(profilePath); err != nil {
		return fmt.Errorf("remove staged browser profile %s: %w", profilePath, err)
	}
	if err := state.ConsumePendingAppProfileReset(ctx); err != nil {
		return fmt.Errorf("clear staged profile reset marker: %w", err)
	}
	logger.Printf("staged app profile reset applied path=%s", profilePath)
	return nil
}

func fatal(logger *log.Logger, err error) {
	if err != nil {
		var discoveryErr *codexruntime.DiscoveryError
		if errors.As(err, &discoveryErr) {
			for index, attempt := range discoveryErr.Result.Attempts {
				logger.Printf("Codex discovery attempt=%d source=%s path=%q reason=%s", index+1, attempt.Source, attempt.Path, attempt.Reason)
			}
		}
		logger.Fatalf("startup failed: %v", err)
	}
}
