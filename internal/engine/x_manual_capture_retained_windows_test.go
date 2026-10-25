//go:build windows && auth_session_dev && auth_session_fixture

package engine

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/AkuSidecar/internal/store"
	"golang.org/x/sys/windows"
)

// Borrowed role adapter. The existing Job/lease owns cleanup; replacement is
// forbidden. Each manager observes its actual Chrome handle independently.
type retainedCaptureProcess struct {
	role xManualPilotWindowsRole
	done chan error
}

func (p *retainedCaptureProcess) Driver() string {
	if p.role.role == "reader" {
		return "browser"
	}
	return "headless"
}
func (p *retainedCaptureProcess) PID() int           { return int(p.role.chrome.PID) }
func (p *retainedCaptureProcess) Done() <-chan error { return p.done }
func (p *retainedCaptureProcess) Terminate()         { p.role.bridge.terminateOwnedTree() }
func (*retainedCaptureProcess) CloseForRetry(context.Context) error {
	return errors.New("pilot replacement forbidden")
}
func (*retainedCaptureProcess) OpenExtensionsPage(context.Context) error {
	return errors.New("pilot extension navigation forbidden")
}
func (*retainedCaptureProcess) ReplacementReadiness(context.Context) error {
	return errors.New("pilot replacement forbidden")
}
func retainedCaptureManager(t *testing.T, role xManualPilotWindowsRole) *captureruntime.Manager {
	t.Helper()
	p := &retainedCaptureProcess{role: role, done: make(chan error, 1)}
	stop := make(chan struct{})
	ended := make(chan struct{})
	go func() {
		defer close(ended)
		defer close(p.done)
		ticker := time.NewTicker(25 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				state, err := windows.WaitForSingleObject(role.chromeHandle, 0)
				if err != nil || state != uint32(windows.WAIT_TIMEOUT) {
					return
				}
			}
		}
	}()
	t.Cleanup(func() { close(stop); <-ended })
	manager, err := captureruntime.New(p)
	if err != nil {
		t.Fatal("capture QA manager unavailable")
	}
	return manager
}

func TestXManualCaptureRetainedQA(t *testing.T) {
	if os.Getenv("AKU_X_NATIVE_MANUAL_CAPTURE_ALLOWED") != "1" {
		t.Skip("capture QA requires explicit prepared capture authorization")
	}
	runXManualRetainedQA(t, func(ctx context.Context, ready xManualRetainedReady) {
		path := filepath.Join(ready.root, "capture-qa.db")
		// Never reopen an existing or user database, even if the helper is misused.
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal("fresh capture QA database unavailable")
		}
		file.Close()
		settings := domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", false)
		settings.CollectionMode = "headless"
		settings.ActiveSources = []domain.Source{domain.SourceX}
		s, err := store.Open(path, settings)
		if err != nil {
			t.Fatal("capture QA database unavailable")
		}
		defer s.Close()
		session, err := s.CreateUpdateSession(ctx, "Retained QA native capture", settings, domain.UpdatePolicy{Trigger: domain.UpdateTriggerUser, Delivery: domain.UpdateDeliveryVisible, BudgetAuthority: domain.BudgetAuthorityUser})
		if err != nil {
			t.Fatal("capture QA session rejected")
		}
		command, err := s.StartRun(ctx, session.Runs[0].ID, map[string]any{"captureRuntime": map[string]any{"driver": "headless"}, "scrolls": 1, "maxBlocksPerSnapshot": 5, "scrollFraction": .5, "scrollSettleMs": 1400})
		if err != nil {
			t.Fatal("capture QA command rejected")
		}
		e := New(s, reasoning.Deterministic{}, config.Config{Dev: true, Capture: config.CaptureConfig{MaxAcquisitionRounds: 1, Behavior: config.CaptureBehaviorConfig{Enabled: true}}}, log.New(io.Discard, "", 0))
		defer func() { e.Shutdown(); e.WaitForIdle(3 * time.Second) }()
		reader, collector := retainedCaptureManager(t, ready.reader), retainedCaptureManager(t, ready.collector)
		runtime, err := e.bindXNativePilotRuntime(ctx, ready.broker, ready.ports, reader, collector, func(call context.Context, _ readerbroker.Request) error {
			var raw json.RawMessage
			if err := ready.reader.bridge.rpc(call, "manual_open_reader", map[string]any{"owner": ready.reader.protocolOwner}, &raw); err != nil {
				return errors.New("capture QA reader failed")
			}
			defer clear(raw)
			var reply struct {
				Opened         bool `json:"opened"`
				WindowCount    int  `json:"windowCount"`
				BootstrapCount int  `json:"bootstrapCount"`
			}
			if xManualPilotDecode(raw, &reply) != nil || !reply.Opened || reply.WindowCount < 2 || reply.BootstrapCount != 1 {
				return errors.New("capture QA reader inventory failed")
			}
			return nil
		}, session.ID, command.RunID)
		if err != nil {
			t.Fatal("capture QA startup binding rejected")
		}
		request := readerbroker.Request{RequestID: "broker_0123456789abcdef0123456789abcdef", Source: "x", URL: "https://x.com/home"}
		if err = runtime.openReader(ctx, request); err != nil {
			t.Fatal("capture QA reader open rejected")
		}
		result := e.captureNativePilot(ctx, runtime, command.ID)
		if result.Outcome != store.Committed || result.PersistenceError != nil || result.DeliveryError != nil {
			t.Fatalf("capture QA rejected phase=%s outcome=%s; no retry", result.Phase, result.Outcome)
		}
		observations, err := s.Observations(ctx, command.RunID)
		if err != nil || len(observations) != 1 {
			t.Fatal("capture QA observation count rejected")
		}
		blocks := 0
		for _, snapshot := range observations[0].Snapshots {
			blocks += len(snapshot.Blocks)
		}
		if blocks == 0 {
			t.Fatal("capture QA returned no posts")
		}
		if len(processFixtureWindows(ready.reader.chrome.PID)) < 2 || collector.PID() != int(ready.collector.chrome.PID) {
			t.Fatal("capture QA concurrent ownership lost")
		}
		if again := e.captureNativePilot(ctx, runtime, command.ID); again.Outcome != store.AlreadyCommitted {
			t.Fatal("capture QA replay fence failed")
		}
		if !e.WaitForIdle(3 * time.Second) {
			t.Fatal("capture QA reasoning did not settle")
		}
		t.Log("native X capture persisted once in disposable QA Store; reader windows survived; replay did not recapture")
	})
}
