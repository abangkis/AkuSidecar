//go:build auth_session_dev

package engine

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/collection"
	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

func dailyBrowserFixture(t *testing.T, sources []domain.Source) (*Engine, *syntheticDailyOwner, *captureruntime.Manager) {
	t.Helper()
	e, owner := dailyRuntimeFixtureWithConfig(t, func(cfg *config.Config) {
		cfg.Capture.XReaderSource = "main"
		cfg.WindowsCaptureSplit = true
		cfg.Capture.MaxAcquisitionRounds = 1
	})
	manager := attachTestCapture(t, e)
	if err := manager.RetainForProcess(); err != nil {
		t.Fatal(err)
	}
	coordinator := collection.NewCoordinator(manager, func(context.Context, string, uint64) (captureruntime.Process, error) {
		t.Error("ordinary Browser admission must not launch or replace the main owner")
		return nil, errors.New("unexpected replacement")
	}, func() error { return nil })
	e.AttachCollectionCoordinator(coordinator)
	settings, err := e.Settings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	settings.CollectionMode = "browser"
	settings.ActiveSources = sources
	settings.AutoUpdateEnabled = true
	if _, err = e.SaveSettings(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	return e, owner, manager
}

func completeDailyBrowserRun(t *testing.T, e *Engine, manager *captureruntime.Manager, sessionID string, source domain.Source) {
	t.Helper()
	ctx := context.Background()
	var run domain.Run
	waitSession(t, e, sessionID, func(session domain.Session) bool {
		for _, candidate := range session.Runs {
			if candidate.Source == source && candidate.Status == "waiting_for_bridge" {
				run = candidate
				return true
			}
		}
		return false
	})
	command, err := e.ClaimCommand(ctx, run.ID, "synthetic-browser")
	if err != nil || command == nil {
		t.Fatalf("Browser claim for %s: command=%+v err=%v", source, command, err)
	}
	stamp, ok := command.Payload["captureRuntime"].(map[string]any)
	if !ok || stamp["driver"] != "browser" || !captureGenerationMatches(stamp["generation"], manager.Snapshot().Generation) {
		t.Fatalf("wrong main generation/driver for %s: %+v", source, command.Payload["captureRuntime"])
	}
	collector, exists, err := e.store.FirstCommandCollector(ctx, run.ID)
	if err != nil || !exists || collector != collection.BackendBridge {
		t.Fatalf("wrong Browser collector: %q %v %v", collector, exists, err)
	}
	if wrong, err := e.claimCommandForDriver(ctx, run.ID, "wrong-headless", "headless"); wrong != nil || !errors.Is(err, errStaleCaptureRuntime) {
		t.Fatalf("headless crossed the Browser route: %+v %v", wrong, err)
	}
	observation := domain.Observation{Source: source, CapturedAt: domain.Now(), Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{
		EvidenceKey: string(source) + ":" + run.ID, Text: "Synthetic mixed daily Browser evidence for " + string(source),
	}}}}, Coverage: map[string]any{"status": "complete"}}
	if _, err = e.AcceptObservation(ctx, command.ID, run.ID, observation); err != nil {
		t.Fatal(err)
	}
	waitSession(t, e, sessionID, func(session domain.Session) bool {
		for _, candidate := range session.Runs {
			if candidate.ID == run.ID {
				return candidate.Status == "completed"
			}
		}
		return false
	})
}

func TestXDailyBrowserMixedSourcesRepeatedBatches(t *testing.T) {
	for _, order := range [][]domain.Source{
		{domain.SourceInstagram, domain.SourceLinkedIn, domain.SourceX},
		{domain.SourceX, domain.SourceInstagram, domain.SourceLinkedIn},
		{domain.SourceLinkedIn, domain.SourceX, domain.SourceInstagram},
	} {
		t.Run(string(order[0]), func(t *testing.T) {
			ctx := context.Background()
			e, owner, manager := dailyBrowserFixture(t, order)
			before, _ := e.Settings(ctx)
			initial := manager.Snapshot()
			for batch := 0; batch < 2; batch++ {
				session, err := e.StartVisibleUpdate(ctx, "mixed synthetic daily update")
				if err != nil {
					t.Fatalf("normal update admission: %v", err)
				}
				if len(session.Runs) != len(order) {
					t.Fatalf("sources lost: %+v", session.Runs)
				}
				for i, source := range order {
					if session.Runs[i].Source != source {
						t.Fatalf("source order changed: %+v", session.Runs)
					}
					if source != domain.SourceX {
						completeDailyBrowserRun(t, e, manager, session.ID, source)
					}
				}
				awaitDailyBatch(t, e, session.ID)
				waitCaptureLeases(t, manager, 0)
				if status := e.CollectionRuntime(); status.CollectionBorrowSource != "" || !status.PersistentMain || status.Effective != "browser" {
					t.Fatalf("ordinary Browser work used a legacy hybrid hold: %+v", status)
				}
				if snapshot := manager.Snapshot(); snapshot.Generation != initial.Generation || snapshot.Driver != "browser" || snapshot.State != captureruntime.Ready {
					t.Fatalf("main owner changed: %+v", snapshot)
				}
			}
			owner.mu.Lock()
			generations, closes, readerCloses := append([]uint64(nil), owner.generations...), owner.collectorCloses, owner.readerCloses
			owner.mu.Unlock()
			if !reflect.DeepEqual(generations, []uint64{1, 2}) || closes != 2 || readerCloses != 0 {
				t.Fatalf("wrong native lifetimes: generations=%v collectors=%d readers=%d", generations, closes, readerCloses)
			}
			after, _ := e.Settings(ctx)
			if !reflect.DeepEqual(before, after) {
				t.Fatal("mixed updates changed source selection/provider/Auto Update settings")
			}
		})
	}
}

func TestXDailyBrowserAdmissionPreservesPredecessorAndOwnerFences(t *testing.T) {
	ctx := context.Background()
	e, _, manager := dailyBrowserFixture(t, []domain.Source{domain.SourceInstagram, domain.SourceLinkedIn, domain.SourceX})
	session, err := e.StartVisibleUpdate(ctx, "fenced synthetic update")
	if err != nil {
		t.Fatal(err)
	}
	if admitted, err := e.admitHybridRun(ctx, session, session.Runs[1]); admitted || err != nil {
		t.Fatalf("successor crossed predecessor drain: admitted=%v err=%v", admitted, err)
	}
	manager.Terminate()
	if command, err := e.ClaimCommand(ctx, session.Runs[0].ID, "stale-browser"); command != nil || !errors.Is(err, errStaleCaptureRuntime) {
		t.Fatalf("dead owner claimed Browser work: %+v %v", command, err)
	}
}

func TestXDailyBrowserAdmissionDoesNotRelaxHeadlessHybridException(t *testing.T) {
	ctx := context.Background()
	e, _ := dailyRuntimeFixture(t)
	manager := attachTestCapture(t, e)
	coordinator := collection.NewCoordinator(manager, nil, func() error { return nil })
	coordinator.Request("headless")
	e.AttachCollectionCoordinator(coordinator)
	settings, _ := e.Settings(ctx)
	settings.ActiveSources = []domain.Source{domain.SourceInstagram, domain.SourceX}
	session, err := e.store.CreateDailyXUpdateSession(ctx, "legacy hybrid guard", settings, domain.UpdatePolicy{
		Trigger: domain.UpdateTriggerUser, Delivery: domain.UpdateDeliveryVisible, BudgetAuthority: domain.BudgetAuthorityUser,
	})
	if err != nil {
		t.Fatal(err)
	}
	if admitted, err := e.admitHybridRun(ctx, session, session.Runs[0]); admitted || err == nil || !strings.Contains(err.Error(), "only Facebook") {
		t.Fatalf("legacy exception broadened: admitted=%v err=%v", admitted, err)
	}
}

func TestXDailyBrowserQueuedSessionWithoutXAuthorization(t *testing.T) {
	ctx := context.Background()
	e, owner, manager := dailyBrowserFixture(t, []domain.Source{domain.SourceInstagram, domain.SourceLinkedIn, domain.SourceX})
	settings, _ := e.Settings(ctx)
	session, err := e.store.CreateDailyXUpdateSession(ctx, "durable mixed queue", settings, domain.UpdatePolicy{
		Trigger: domain.UpdateTriggerUser, Delivery: domain.UpdateDeliveryVisible, BudgetAuthority: domain.BudgetAuthorityUser,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = e.StopXManual(ctx); err != nil {
		t.Fatal(err)
	}
	if err = e.ReconcileXDailyStartup(ctx); err != nil {
		t.Fatal(err)
	}
	if err = e.ResumeXDailyScheduling(ctx); err != nil {
		t.Fatalf("persisted non-X queue stranded without X consent: %v", err)
	}
	completeDailyBrowserRun(t, e, manager, session.ID, domain.SourceInstagram)
	completeDailyBrowserRun(t, e, manager, session.ID, domain.SourceLinkedIn)
	waitSession(t, e, session.ID, func(s domain.Session) bool { return s.Status == "partial" })
	saved, err := e.store.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Runs[2].Status != "failed" {
		t.Fatalf("X silently fell back without authorization: %+v", saved.Runs[2])
	}
	owner.mu.Lock()
	defer owner.mu.Unlock()
	if len(owner.generations) != 0 {
		t.Fatal("unauthorized X collector started")
	}
	if !e.WaitForIdle(time.Second) {
		t.Fatal("synthetic worker drain did not finish")
	}
}
