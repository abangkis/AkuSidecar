//go:build windows

package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/collection"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/engine"
	"github.com/abangkis/AkuSidecar/internal/store"
)

// The shared operator fixture owns profile stop/test/restore prerequisites.
// This branch uses real acquisition with deterministic local reasoning; it
// certifies routing and ownership, not AI selection quality or media parity.
func runAuthenticatedHybridUpdate(t *testing.T, ctx context.Context, state *store.Store, e *engine.Engine, coordinator *collection.Coordinator, manager *captureruntime.Manager) {
	t.Helper()
	e.StartHeadlessCollection(ctx)
	session, err := e.StartVisibleUpdate(ctx, "bounded authenticated hybrid routing qualification")
	if err != nil || len(session.Runs) != 4 {
		t.Fatal("mixed Update did not admit all four previously authorized sources")
	}
	if session.Runs[3].Source != domain.SourceFacebook {
		t.Fatal("Facebook was not placed after the headless batch")
	}
	browserConfigured := false
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		owner := manager.Snapshot()
		if !browserConfigured && owner.State == captureruntime.Ready && owner.Driver == "browser" {
			if owner.Generation != 3 {
				t.Fatal("Facebook Browser owner did not follow the initial headless generation")
			}
			configureAuthenticatedHybridBridge(t, ctx, state, e)
			browserConfigured = true
		}
		session, err = state.GetSession(ctx, session.ID)
		if err != nil {
			t.Fatal("mixed session state is unavailable")
		}
		if session.Status == "completed" || session.Status == "partial" || session.Status == "failed" || session.Status == "cancelled" {
			break
		}
		select {
		case <-ctx.Done():
			for _, run := range session.Runs {
				t.Logf("source=%s status=%s stage=%s", run.Source, run.Status, run.Stage)
			}
			t.Fatal("mixed Update did not finish within its acquisition bound")
		case <-ticker.C:
		}
	}
	if session.Status != "completed" || !browserConfigured {
		t.Fatal("mixed Update did not complete through the real Facebook Browser collector")
	}
	for _, run := range session.Runs {
		wantDriver, wantCollector := "headless", collection.BackendHeadless
		if run.Source == domain.SourceFacebook {
			wantDriver, wantCollector = "browser", collection.BackendBridge
		}
		driver, driverExists, driverErr := state.FirstCommandCaptureDriver(ctx, run.ID)
		collector, collectorExists, collectorErr := state.FirstCommandCollector(ctx, run.ID)
		if run.Status != "completed" || !driverExists || !collectorExists || driverErr != nil || collectorErr != nil || driver != wantDriver || collector != wantCollector {
			t.Fatalf("source %s failed its persisted route/status qualification", run.Source)
		}
		observations, observationErr := state.Observations(ctx, run.ID)
		blocks := 0
		for _, observation := range observations {
			if observation.Source != run.Source {
				t.Fatal("observation source differs from its owned run")
			}
			for _, snapshot := range observation.Snapshots {
				blocks += len(snapshot.Blocks)
			}
		}
		if observationErr != nil || blocks == 0 {
			t.Fatalf("source %s has no real captured blocks; empty acquisition is not success", run.Source)
		}
		t.Logf("source=%s driver=%s collector=%s observations=%d blocks=%d", run.Source, driver, collector, len(observations), blocks)
	}
	if err := waitForAuthJourneyRuntime(ctx, coordinator, "headless", 4); err != nil {
		t.Fatal("terminal Facebook collection did not clean up and naturally return to headless")
	}
	drainDeadline := time.Now().Add(3 * time.Second)
	for manager.Snapshot().ActiveLeases != 0 || coordinator.Status().CollectionBorrowSource != "" {
		if time.Now().After(drainDeadline) {
			t.Fatal("terminal mixed session retained collection ownership")
		}
		select {
		case <-ctx.Done():
			t.Fatal("terminal ownership readback timed out")
		case <-ticker.C:
		}
	}
	settings, err := state.GetSettings(ctx)
	if err != nil || len(settings.ActiveSources) != 4 || settings.ActiveSources[0] != domain.SourceFacebook || settings.CollectionMode != "headless" {
		t.Fatal("execution ordering changed the isolated user's configured source order or selected mode")
	}
	t.Log("mixed_update_completed=true headless_generation=2 facebook_browser_generation=3 auto_return_generation=4 configured_source_order_preserved=true")
}

func configureAuthenticatedHybridBridge(t *testing.T, ctx context.Context, state *store.Store, e *engine.Engine) {
	t.Helper()
	token, err := state.BridgeToken(ctx)
	if err != nil {
		t.Fatal("isolated Bridge token is unavailable")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://127.0.0.1:11122/api/split-capture/actions", strings.NewReader(`{"type":"configure_background"}`))
	if err != nil {
		t.Fatal("Bridge dispatch configuration request could not be prepared")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Aku-Bridge-Token", token)
	req.Header.Set("X-Aku-Bridge-Contract", domain.BridgeContractVersion)
	req.Header.Set("X-Aku-Split-Epoch", e.Epoch())
	response, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		t.Fatal("real Bridge dispatch configuration did not complete")
	}
	defer response.Body.Close()
	var result splitActionResult
	if response.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(response.Body, 128*1024)).Decode(&result) != nil || !result.OK {
		t.Fatal("real Bridge rejected its isolated mixed-session dispatch configuration")
	}
}
