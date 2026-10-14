//go:build windows && auth_session_dev && auth_session_fixture

package main

import (
	"context"
	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/collection"
	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/engine"
	"github.com/abangkis/AkuSidecar/internal/httpapi"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/AkuSidecar/internal/store"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type developmentStatusProcess struct {
	done chan error
	once sync.Once
}

func (p *developmentStatusProcess) Driver() string                             { return "headless" }
func (p *developmentStatusProcess) Done() <-chan error                         { return p.done }
func (p *developmentStatusProcess) PID() int                                   { return 777 }
func (p *developmentStatusProcess) OpenExtensionsPage(context.Context) error   { return nil }
func (p *developmentStatusProcess) ReplacementReadiness(context.Context) error { return nil }
func (p *developmentStatusProcess) CloseForRetry(context.Context) error        { p.Terminate(); return nil }
func (p *developmentStatusProcess) Terminate()                                 { p.once.Do(func() { p.done <- nil }) }

func TestDevelopmentCollectionRuntimeActualServerEndpointAndStrictPresence(t *testing.T) {
	if os.Getenv("AKU_CHROME_FIXTURE_ROOT") == "" {
		t.Skip("requires project-local preparation helper")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	root := t.TempDir()
	settings := domain.DefaultSettings("expanded", "quiet", "guarded_live", true)
	settings.CollectionMode = "headless"
	settings.ActiveSources = []domain.Source{domain.SourceX}
	state, err := store.Open(filepath.Join(root, "contract.sqlite"), settings)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	cfg := config.Config{Dev: true, Root: root, WindowsCaptureSplit: true, Server: config.ServerConfig{Host: "127.0.0.1", Port: 0}}
	runtime := engine.New(state, reasoning.Deterministic{}, cfg, log.New(io.Discard, "", 0))
	defer runtime.Shutdown()
	owner, err := captureruntime.New(&developmentStatusProcess{done: make(chan error, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Terminate()
	coordinator := collection.NewCoordinator(owner, func(context.Context, string, uint64) (captureruntime.Process, error) {
		t.Error("HTTP status contract must not launch a process")
		return nil, nil
	}, func() error { return nil })
	capability := engine.DevelopmentSessionCapability{
		Provider: "synthetic-fixture", StoreScopeID: "invented-contract-store", ProfileID: "invented-contract-profile",
		ExpectedAccounts: map[domain.Source]string{domain.SourceX: "alpha"}, Sources: []domain.Source{domain.SourceX},
	}
	evidence := func(request engine.DevelopmentSessionProbeRequest) engine.DevelopmentSessionEvidence {
		return engine.DevelopmentSessionEvidence{
			Version: request.Version, Kind: request.Kind, ProbeID: request.ProbeID, ObservedAt: time.Now().UTC(),
			Domain: "synthetic-fixture", Source: request.Source, ProfileID: request.ProfileID,
			AccountID: "alpha", Revision: "invented-contract-revision", AuthState: engine.DevelopmentSessionAuthAuthenticated,
		}
	}
	capability.Authority = func(_ context.Context, request engine.DevelopmentSessionProbeRequest) (engine.DevelopmentSessionEvidence, error) {
		return evidence(request), nil
	}
	capability.Browser = func(_ context.Context, request engine.DevelopmentSessionProbeRequest) (engine.DevelopmentBrowserEvidence, error) {
		s := owner.Snapshot()
		return engine.DevelopmentBrowserEvidence{DevelopmentSessionEvidence: evidence(request), EngineEpoch: runtime.Epoch(), Generation: s.Generation, PID: owner.PID(), Driver: s.Driver}, nil
	}
	if err := runtime.PrepareDevelopmentSessionCheckpoint(ctx, capability, true); err != nil {
		t.Fatal(err)
	}
	if err := runtime.StartDevelopmentSession(ctx, owner, coordinator, capability, nil); err != nil {
		t.Fatal(err)
	}
	readyCtx, stopReady := context.WithTimeout(ctx, time.Second)
	defer stopReady()
	if err := developmentWait(readyCtx, func() bool { return !coordinator.Status().BatchPaused }); err != nil {
		t.Fatal(err)
	}
	server, err := httpapi.New(cfg, state, runtime, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	address, err := server.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cancel()
		stopCtx, stop := context.WithTimeout(context.Background(), time.Second)
		defer stop()
		server.Stop(stopCtx)
	}()
	origin := "http://" + address.String()
	value, err := readDevelopmentCollectionRuntime(ctx, origin)
	if err != nil || value.Generation != owner.Snapshot().Generation || !value.DevelopmentSession {
		t.Fatal("actual collection runtime endpoint rejected", err)
	}
	// Health is a different contract and must not be accepted as false pause.
	health, err := http.Get(origin + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	defer health.Body.Close()
	if _, err := decodeDevelopmentCollectionRuntime(health.StatusCode, health.Body); err == nil {
		t.Fatal("health endpoint missing runtime was accepted")
	}
	for _, body := range []string{`{}`, `{"collectionRuntime":null}`, `{"collectionRuntime":{}}`, `{"collectionRuntime":{"available":true,"state":"ready","requested":"headless","effective":"headless","generation":1,"developmentSession":true,"batchPaused":true}}`, `{"collectionRuntime":{"available":true,"state":"ready","requested":"headless","effective":"headless","generation":1,"developmentSession":true,"pending":true}}`} {
		if _, err := decodeDevelopmentCollectionRuntime(200, strings.NewReader(body)); err == nil {
			t.Fatal("missing/paused/pending runtime accepted")
		}
	}
	t.Log("actualServerCollectionRuntimeGET=true typedRuntimeStatus=true requiredRuntimePresent=true readyHeadlessVerified=true healthMissingRuntimeRejected=true pausedPendingUnknownRejected=true noUIOrBrowserLaunched=true")
}
