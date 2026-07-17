package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/abangkis/AkuSidecar/internal/aidetector"
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
	cfg, err := config.Load(options)
	fatal(logger, err)
	settings := domain.DefaultSettings(cfg.Capture.Profile, cfg.Capture.Visibility, cfg.Preference.Mode, cfg.Capture.OpenMissingSource)
	state, err := store.Open(cfg.Database.Path, settings)
	fatal(logger, err)
	defer state.Close()
	provider, err := reasoning.NewProvider(cfg)
	fatal(logger, err)
	var eventResolver semanticengine.Resolver
	if appServer, ok := provider.(*reasoning.CodexAppServer); ok {
		eventResolver, err = semanticengine.NewAppServerResolver(cfg.Root, appServer, cfg.Reasoning.Evaluation)
		fatal(logger, err)
	}
	eventRuntime := semanticengine.New(state, eventResolver)
	runtime := engine.New(state, provider, cfg, logger, eventRuntime)
	if appServer, ok := provider.(*reasoning.CodexAppServer); ok {
		aiResolver, err := aidetector.NewAppServerResolver(cfg.Root, appServer, cfg.Reasoning.AIDetection)
		fatal(logger, err)
		runtime.SetAIDeepResolver(aiResolver)
	}
	server, err := httpapi.New(cfg, state, runtime, logger)
	fatal(logger, err)
	address, err := server.Start()
	fatal(logger, err)
	logger.Printf("version=%s runtime=go address=http://%s provider=%s database=%s", domain.ApplicationVersion, address, provider.Name(), state.Path())
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

func fatal(logger *log.Logger, err error) {
	if err != nil {
		logger.Fatalf("startup failed: %v", err)
	}
}
