//go:build windows && auth_session_dev

package engine

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/collection"
	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
)

func ordinaryMainXFixture(t *testing.T) (*Engine, *captureruntime.Manager) {
	t.Helper()
	e, _ := testEngine(t)
	e.config.Dev = true
	e.config.WindowsCaptureSplit = true
	e.config.Capture.XMode = config.XCaptureConcurrent
	e.config.Capture.XReaderSource = "main"
	owner := attachTestCapture(t, e)
	if err := owner.RetainForProcess(); err != nil {
		t.Fatal(err)
	}
	c := collection.NewCoordinator(owner, func(context.Context, string, uint64) (captureruntime.Process, error) {
		t.Fatal("ordinary main action replaced the Browser owner")
		return nil, nil
	}, func() error { return nil })
	e.AttachCollectionCoordinator(c)
	settings, err := e.Settings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	settings.AutoUpdateEnabled = true
	if _, err = e.SaveSettings(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	return e, owner
}

func TestXMainBrowserActionsDoNotAuthorizeCapture(t *testing.T) {
	e, owner := ordinaryMainXFixture(t)
	ctx := context.Background()
	before, _ := e.Settings(ctx)
	initial := owner.Snapshot()
	for _, native := range []bool{false, true} {
		lease, done, err := e.BorrowXMainBrowserAction(ctx, native)
		if err != nil || lease == nil {
			t.Fatalf("ordinary action native=%t: %v", native, err)
		}
		if lease.Driver() != "browser" || lease.Generation() != initial.Generation {
			t.Fatal("ordinary action borrowed another driver/generation")
		}
		lease.Release()
		done()
	}
	request := readerbroker.Request{RequestID: "broker_" + strings.Repeat("a", 32), Source: "x", URL: "https://x.com/synthetic/status/1"}
	if _, selected, err := e.PrepareXManualReader(ctx, request, "/split-reader-intent?id=synthetic"); err != nil || selected {
		t.Fatalf("main reader did not delegate to the ordinary broker: selected=%t err=%v", selected, err)
	}
	if status := e.XCaptureStatus(); status.DerivationAuthorized || status.MaxBatchMS != 60000 {
		t.Fatalf("UI action changed batch authority: %+v", status)
	}
	e.xDaily.mu.RLock()
	started := e.xDaily.startUsed || e.xDaily.owner != nil || e.xDaily.generation != 0
	e.xDaily.mu.RUnlock()
	if started {
		t.Fatal("UI interaction initialized a daily owner/collector")
	}
	if release, ok := e.ordinarySourceRoute(domain.SourceX); ok {
		release()
		t.Fatal("main UI action enabled legacy X capture")
	}
	settings := before
	settings.ActiveSources = []domain.Source{domain.SourceX}
	session, err := e.store.CreateUpdateSession(ctx, "synthetic forbidden legacy capture", settings, domain.UpdatePolicy{
		Trigger: domain.UpdateTriggerUser, Delivery: domain.UpdateDeliveryVisible, BudgetAuthority: domain.BudgetAuthorityUser,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.store.StartRun(ctx, session.Runs[0].ID, map[string]any{}); err != nil {
		t.Fatal(err)
	}
	for _, driver := range []string{"browser", "headless"} {
		command, err := e.claimCommandForDriver(ctx, session.Runs[0].ID, "synthetic-ui-worker", driver)
		if command != nil {
			t.Fatalf("UI action enabled %s capture claim: %+v %v", driver, command, err)
		}
	}
	after, _ := e.Settings(ctx)
	if !reflect.DeepEqual(before, after) || owner.Snapshot().ActiveLeases != 0 || owner.Snapshot().Generation != initial.Generation {
		t.Fatal("ordinary actions changed settings, reader generation or retained a lease")
	}
}

func TestXMainBrowserActionsSurviveBatchStopButRejectLostOwner(t *testing.T) {
	e, owner := ordinaryMainXFixture(t)
	ctx := context.Background()
	if err := e.StopXManual(ctx); err != nil {
		t.Fatal(err)
	}
	lease, done, err := e.BorrowXMainBrowserAction(ctx, true)
	if err != nil {
		t.Fatal("batch stop blocked an ordinary reader", err)
	}
	lease.Release()
	done()
	owner.Terminate()
	if lease, _, err = e.BorrowXMainBrowserAction(ctx, true); lease != nil || err == nil {
		t.Fatal("lost owner admitted an ordinary reader")
	}
	request := readerbroker.Request{RequestID: "broker_" + strings.Repeat("b", 32), Source: "x", URL: "https://x.com/synthetic/status/2"}
	if _, selected, err := e.PrepareXManualReader(ctx, request, "synthetic"); !selected || err == nil {
		t.Fatal("lost main owner fell through to an unowned broker")
	}
}

func TestXMainBrowserActionsKeepDedicatedAndConfigurationGates(t *testing.T) {
	for _, missing := range []string{"main-source", "dev", "split", "mode", "coordinator"} {
		t.Run(missing, func(t *testing.T) {
			e, _ := ordinaryMainXFixture(t)
			switch missing {
			case "main-source":
				e.config.Capture.XReaderSource = "dedicated"
			case "dev":
				e.config.Dev = false
			case "split":
				e.config.WindowsCaptureSplit = false
			case "mode":
				e.config.Capture.XMode = config.XCaptureExclusive
			case "coordinator":
				e.collectionRuntime = nil
			}
			if lease, _, err := e.BorrowXMainBrowserAction(context.Background(), true); lease != nil || err == nil {
				t.Fatal("unselected or unowned main interaction was admitted")
			}
		})
	}
}
