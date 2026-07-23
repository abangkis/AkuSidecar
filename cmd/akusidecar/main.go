package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/abangkis/AkuSidecar/internal/aidetector"
	"github.com/abangkis/AkuSidecar/internal/codexruntime"
	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/engine"
	semanticengine "github.com/abangkis/AkuSidecar/internal/eventengine"
	"github.com/abangkis/AkuSidecar/internal/httpapi"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/AkuSidecar/internal/store"
)

func main() {
	logger := log.New(os.Stdout, "AkuSidecar ", log.LstdFlags|log.LUTC|log.Lmsgprefix)
	options := config.ParseFlags()
	if options.DiscoverCodex {
		os.Exit(discoverCodex(options))
	}
	cfg, err := config.Load(options)
	fatal(logger, err)
	settings := domain.DefaultSettings(cfg.Capture.Profile, cfg.Capture.Visibility, cfg.Preference.Mode, cfg.Capture.OpenMissingSource)
	state, err := store.Open(cfg.Database.Path, settings)
	fatal(logger, err)
	defer state.Close()
	persistedSettings, err := state.GetSettings(context.Background())
	fatal(logger, err)
	if options.CodexPath == "" && persistedSettings.ReasoningExecutablePath != "" {
		persistedPath := persistedSettings.ReasoningExecutablePath
		if _, statErr := os.Stat(persistedPath); statErr == nil {
			cfg.Reasoning.Executable = persistedPath
		} else {
			logger.Printf("persisted Codex executable is unavailable; rediscovering path=%q error=%v", persistedPath, statErr)
			persistedSettings.ReasoningExecutablePath = ""
		}
	}
	provider, err := reasoning.NewProvider(cfg)
	fatal(logger, err)
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
	if structured, ok := provider.(reasoning.StructuredInvoker); ok {
		aiResolver, err := aidetector.NewStructuredResolver(cfg.Root, structured, cfg.Reasoning.AIDetection)
		fatal(logger, err)
		runtime.SetAIDeepResolver(aiResolver)
	}
	server, err := httpapi.New(cfg, state, runtime, logger)
	fatal(logger, err)
	resumed, err := runtime.ResumePendingReasoning(context.Background())
	fatal(logger, err)
	address, err := server.Start()
	fatal(logger, err)
	runtime.StartAutoUpdateScheduler()
	logger.Printf("version=%s runtime=go address=http://%s provider=%s database=%s", domain.ApplicationVersion, address, provider.Name(), state.Path())
	if resumed > 0 {
		logger.Printf("resumed_reasoning_runs=%d from_durable_capture=true", resumed)
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	<-signals
	shutdownStarted := time.Now()
	logger.Printf("shutdown requested")
	runtime.Shutdown()
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
