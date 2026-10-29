//go:build auth_session_dev

package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/activationguard"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

func nativeHoldFixture(t *testing.T) *activationguard.Guard {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sidecar.exe"+activationguard.FileSuffix)
	raw := fmt.Sprintf(`{"schema":%q,"activationId":%q,"sources":["x","instagram"]}`, activationguard.Schema, strings.Repeat("a", 32))
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	g, err := activationguard.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func TestMaintenanceHoldPreservesPermissionButBlocksRestorationAndQueuedAcquisition(t *testing.T) {
	e, state := testEngine(t)
	g := nativeHoldFixture(t)
	if err := e.BindNativeDerivationHold(g); err != nil {
		t.Fatal(err)
	}
	x := &syntheticDailyOwner{}
	scope := strings.Repeat("a", 64)
	attachDailyPermissionRuntime(t, e, state, scope, x)
	ig := attachInstagramOffline(t, e)
	e.config.Capture.InstagramTrial = true
	igChecks := 0
	ig.mutate = func(*instagramSyntheticEvidence, bool) { igChecks++ }
	ctx := context.Background()
	for _, source := range dailySources {
		d := e.dailyController(source)
		if err := state.SaveDailySourcePermission(ctx, source, d.state.profileScope, true); err != nil {
			t.Fatal(err)
		}
	}
	settings, _ := state.GetSettings(ctx)
	settings.AutoUpdateEnabled = true
	settings.ActiveSources = []domain.Source{domain.SourceX, domain.SourceInstagram}
	settings.CalibrationEnabled = false
	if err := state.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	if err := e.ResumeXDailyScheduling(ctx); err != nil {
		t.Fatal(err)
	}
	for _, source := range dailySources {
		d := e.dailyController(source)
		status := d.status()
		if d.ready() || !status.MaintenanceHeld || !status.RememberedPermission || status.Reason != "maintenance-confirmation-required" {
			t.Fatalf("held status %+v", status)
		}
		if d.queue(domain.Run{Source: source, ID: "not-started", SessionID: "not-started"}, nil) == nil {
			t.Fatal("queued acquisition bypassed hold")
		}
	}
	if e.installXDailyOwner(ctx, x) == nil {
		t.Fatal("direct owner install bypassed hold")
	}
	if _, err := e.StartPreparedUpdateNow(ctx); err == nil {
		t.Fatal("automatic path admitted held sources")
	}
	if x.readerChecks != 0 || igChecks != 0 || ig.derivations != 0 {
		t.Fatal("held startup accessed proof or started derivation")
	}
	if err := e.StartXDaily(ctx, XDailyStartRequest{}); err == nil || !g.Held("x") {
		t.Fatal("unacknowledged request released hold")
	}
	if err := e.StartXDaily(ctx, XDailyStartRequest{true, true}); err != nil {
		t.Fatal(err)
	}
	if g.Held("x") || !g.Held("instagram") || !e.xDailyReady() || x.readerChecks != 1 || ig.derivations != 0 {
		t.Fatal("explicit source release lost proof or source separation")
	}
	current, _ := state.GetSettings(ctx)
	if current.AutoUpdateEnabled != settings.AutoUpdateEnabled {
		t.Fatal("maintenance changed Auto Update")
	}
}

func TestMaintenanceReleaseStillRequiresFreshReaderVerification(t *testing.T) {
	e, state := testEngine(t)
	g := nativeHoldFixture(t)
	if err := e.BindNativeDerivationHold(g); err != nil {
		t.Fatal(err)
	}
	x := &syntheticDailyOwner{check: func(context.Context) error { return fmt.Errorf("synthetic identity unavailable") }}
	attachDailyPermissionRuntime(t, e, state, strings.Repeat("c", 64), x)
	if err := e.StartXDaily(context.Background(), XDailyStartRequest{true, true}); err == nil {
		t.Fatal("failed proof accepted")
	}
	if e.xDailyReady() || e.XCaptureStatus().DerivationAuthorized || !e.XCaptureStatus().RememberedPermission {
		t.Fatal("maintenance release substituted for verification")
	}
}

func TestMaintenanceColdOpenDoesNotResumeQueuedNativeRun(t *testing.T) {
	e, state := testEngine(t)
	if err := e.BindNativeDerivationHold(nativeHoldFixture(t)); err != nil {
		t.Fatal(err)
	}
	owner := &syntheticDailyOwner{}
	attachDailyPermissionRuntime(t, e, state, strings.Repeat("d", 64), owner)
	ctx := context.Background()
	if err := state.SaveDailySourcePermission(ctx, domain.SourceX, e.xDaily.profileScope, true); err != nil {
		t.Fatal(err)
	}
	settings, _ := state.GetSettings(ctx)
	settings.ActiveSources = []domain.Source{domain.SourceX}
	settings.CalibrationEnabled = false
	settings.AIDetectionEnabled = false
	session, err := state.CreateUpdateSession(ctx, "synthetic queued cold-open", settings, domain.UpdatePolicy{Trigger: domain.UpdateTriggerScheduler, Delivery: domain.UpdateDeliveryPrepared, BudgetAuthority: domain.BudgetAuthorityAutomatic})
	if err != nil {
		t.Fatal(err)
	}
	if err = e.ReconcileXDailyStartup(ctx); err != nil {
		t.Fatal(err)
	}
	if err = e.ResumeXDailyScheduling(ctx); err != nil {
		t.Fatal(err)
	}
	run, err := state.GetRun(ctx, session.Runs[0].ID)
	if err != nil || run.Status != "failed" || owner.readerChecks != 0 || e.xDailyReady() {
		t.Fatalf("held cold-open escaped reconciliation: run=%+v err=%v", run, err)
	}
}
