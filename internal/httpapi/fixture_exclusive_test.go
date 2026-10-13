//go:build auth_session_fixture

package httpapi

import (
	"context"
	"io"
	"log"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/collection"
	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/engine"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
)

// Exercise the existing close/resume HTTP API with genuine ownership and
// persistent queue admission. Reader preparation/foreground uses no real broker.
func TestSyntheticExclusiveHTTPReaderResume(t *testing.T) {
	for _, mode := range []string{"user_close", "natural_exit", "account_loss"} {
		t.Run(mode, func(t *testing.T) {
			s, token := splitTestServer(t)
			logger := log.New(io.Discard, "", 0)
			s.engine = engine.New(s.store, reasoning.Deterministic{}, config.Config{Dev: true}, logger)
			process := func(driver string) *collectionUIFixtureProcess {
				return &collectionUIFixtureProcess{splitLeaseProcess: splitLeaseProcess{done: make(chan error, 1)}, mode: driver}
			}
			owner, err := captureruntime.New(process("headless"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(owner.Terminate)
			if err := s.engine.AttachCaptureRuntime(context.Background(), owner); err != nil {
				t.Fatal(err)
			}
			if err := s.SetSplitCaptureRuntime(owner); err != nil {
				t.Fatal(err)
			}
			var reader *nativeResumeFixtureProcess
			var readerMu sync.Mutex
			coordinator := collection.NewCoordinator(owner, func(_ context.Context, driver string, _ uint64) (captureruntime.Process, error) {
				if driver == "native_reader" {
					p := &nativeResumeFixtureProcess{collectionUIFixtureProcess: process("browser")}
					p.readerOpen.Store(true)
					readerMu.Lock()
					reader = p
					readerMu.Unlock()
					return p, nil
				}
				return process(driver), nil
			}, func() error { return nil })
			s.engine.AttachCollectionCoordinator(coordinator)
			// This harness has synthetic evidence and no browser capability host.
			coordinator.SetHeadlessReadiness(func() error { return nil })
			var authMu sync.Mutex
			authenticated := true
			if err := s.engine.BindSyntheticExclusive("fixture-account", func(context.Context) (engine.SyntheticSessionEvidence, error) {
				authMu.Lock()
				defer authMu.Unlock()
				return engine.SyntheticSessionEvidence{Authenticated: authenticated, AccountID: "fixture-account", Revision: "revision-1"}, nil
			}); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			defer cancel()
			coordinator.Start(ctx)
			coordinator.Request("headless")
			// Ready hook opens initial admission; no collectors are started.
			time.Sleep(30 * time.Millisecond)
			settings, err := s.store.GetSettings(ctx)
			if err != nil {
				t.Fatal(err)
			}
			settings.ActiveSources = []domain.Source{domain.SourceX}
			settings.CollectionMode = "headless"
			session, err := s.store.CreateUpdateSession(ctx, "update", settings, domain.UpdatePolicy{Trigger: domain.UpdateTriggerUser, Delivery: domain.UpdateDeliveryVisible, BudgetAuthority: domain.BudgetAuthorityUser})
			if err != nil {
				t.Fatal(err)
			}
			command, err := s.store.StartRun(ctx, session.Runs[0].ID, map[string]any{"captureCollector": map[string]any{"version": 1, "backend": "headless"}, "captureRuntime": map[string]any{"driver": "headless", "epoch": s.engine.Epoch(), "generation": owner.Snapshot().Generation}, "continuation": map[string]any{"cursor": "fixture-cursor"}})
			if err != nil {
				t.Fatal(err)
			}
			lease, release, err := s.engine.BorrowNativeReader(ctx)
			if err != nil {
				t.Fatal(err)
			}
			lease.Release()
			release()
			paused, err := s.store.FixtureCaptureSessionPaused(ctx, session.ID)
			if err != nil || !paused {
				t.Fatalf("not durably paused: %t %v", paused, err)
			}
			if claimed, err := s.store.ClaimCommandForCollector(ctx, command.RunID, "fixture-worker", "headless"); err != nil || claimed != nil {
				t.Fatalf("reader allowed claim: %v %v", claimed, err)
			}
			readerMu.Lock()
			p := reader
			readerMu.Unlock()
			coordinator.Retry()
			time.Sleep(30 * time.Millisecond)
			if !p.readerOpen.Load() || !coordinator.Status().NativeReaderOnly {
				t.Fatal("reader window was not preserved")
			}
			if mode == "account_loss" {
				authMu.Lock()
				authenticated = false
				authMu.Unlock()
			}
			if mode == "natural_exit" || mode == "account_loss" {
				p.readerOpen.Store(false)
				p.Terminate()
				coordinator.Retry()
			} else {
				s.SetNativeReaderCloseAction(func(context.Context) error { p.readerOpen.Store(false); return nil })
				request := httptest.NewRequest("POST", "http://127.0.0.1:11122/api/collection/native-reader/close-and-resume", strings.NewReader("{}")).WithContext(ctx)
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set("X-Aku-Bridge-Contract", domain.BridgeContractVersion)
				request.Header.Set("X-Aku-Bridge-Token", token)
				request.Header.Set("X-Aku-Capture-Instance", s.splitCapture.key)
				request.Header.Set("X-Aku-Split-Epoch", s.engine.Epoch())
				w := httptest.NewRecorder()
				s.api().ServeHTTP(w, request)
				if w.Code != 200 {
					t.Fatalf("close HTTP=%d %s", w.Code, w.Body)
				}
			}
			deadline := time.Now().Add(time.Second)
			for time.Now().Before(deadline) {
				status := coordinator.Status()
				if mode == "account_loss" && status.Failure != "" {
					break
				}
				paused, _ = s.store.FixtureCaptureSessionPaused(ctx, session.ID)
				if mode != "account_loss" && !paused && status.Effective == "headless" {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			paused, err = s.store.FixtureCaptureSessionPaused(ctx, session.ID)
			if err != nil {
				t.Fatal(err)
			}
			claimed, err := s.store.ClaimCommandForCollector(ctx, command.RunID, "fixture-worker", "headless")
			if err != nil {
				t.Fatal(err)
			}
			if mode == "account_loss" {
				if !paused || claimed != nil {
					t.Fatal("lost account resumed queue")
				}
				return
			}
			if paused || claimed == nil || claimed.ID != command.ID || claimed.Payload["continuation"].(map[string]any)["cursor"] != "fixture-cursor" {
				t.Fatalf("queue not resumed unchanged: paused=%t claimed=%v status=%+v", paused, claimed, coordinator.Status())
			}
		})
	}
}
