package engine

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
)

func TestXDailySelectionGates(t *testing.T) {
	cfg := config.Config{Dev: true, Capture: config.CaptureConfig{XMode: config.XCaptureConcurrent}}
	options := config.Options{Dev: true}
	s, err := PrepareXDailyDevelopment(cfg, options)
	if xDailyBuildSupported {
		if err != nil || s == nil {
			t.Fatalf("supported selection: %v", err)
		}
		if s.engine != nil {
			t.Fatal("preparation attached or opened a resource")
		}
		if err = s.Close(); err != nil {
			t.Fatal(err)
		}
	} else if err == nil || s != nil {
		t.Fatal("unsupported build accepted concurrent mode")
	}
	for _, opts := range []config.Options{{}, {Dev: true, DevelopmentSessionReuse: "native-x-manual"}, {Dev: true, DevelopmentQAPilotPrepare: "qa"}, {Dev: true, DevelopmentXManualPlan: "plan"}, {Dev: true, DevelopmentQAPilotPlan: "plan"}} {
		if _, err := PrepareXDailyDevelopment(cfg, opts); err == nil {
			t.Fatalf("invalid selection accepted %+v", opts)
		}
	}
	cfg.Dev = false
	if _, err := PrepareXDailyDevelopment(cfg, options); err == nil {
		t.Fatal("non-dev accepted")
	}
	cfg.Capture.XMode = ""
	if s, err := PrepareXDailyDevelopment(cfg, options); err != nil || s != nil {
		t.Fatal("default must remain exclusive")
	}
}

func TestXDailyOrdinaryDatabaseAndImmutableSelection(t *testing.T) {
	if !xDailyBuildSupported {
		t.Skip("Windows development build required for attachment")
	}
	e, state := testEngine(t)
	e.config.Dev = true
	e.config.Database.Path = state.Path()
	e.config.Capture.XMode = config.XCaptureConcurrent
	before, err := state.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	s, err := PrepareXDailyDevelopment(e.config, config.Options{Dev: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Attach(e); err != nil {
		t.Fatal(err)
	}
	if e.store != state || e.XManualConfigured() {
		t.Fatal("ordinary Store replaced or QA/manual selected")
	}
	if err = s.Attach(e); err == nil {
		t.Fatal("duplicate attachment accepted")
	}
	st := e.XCaptureStatus()
	if st.SelectedMode != "concurrent" || st.EffectiveMode != "unavailable" || st.Reason != "batch-adapter-required" || st.DerivationAuthorized || st.MaxBatchMS != 60000 || st.AuthorizationPolicy != "process" {
		t.Fatalf("status %+v", st)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	if st = e.XCaptureStatus(); st.State != "stopped" || !e.XDailyConfigured() {
		t.Fatalf("close silently reverted route %+v", st)
	}
	if _, ok := e.ordinarySourceRoute(domain.SourceX); ok {
		t.Fatal("closed daily selection fell back")
	}
	after, err := state.GetSettings(context.Background())
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("settings/Auto Update changed")
	}
	// Only constructing a new Engine with exclusive configuration restores it.
	other := New(state, e.provider, config.Config{}, e.logger)
	release, ok := other.ordinarySourceRoute(domain.SourceX)
	release()
	if !ok {
		t.Fatal("restart rollback did not restore exclusive route")
	}
}

func TestXDailyAdmissionAndReaderFailClosed(t *testing.T) {
	e, state := testEngine(t)
	e.config.Capture.XMode = config.XCaptureConcurrent
	ctx := context.Background()
	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.ActiveSources = []domain.Source{domain.SourceX, domain.SourceLinkedIn}
	session, err := state.CreateUpdateSession(ctx, "synthetic queued commands", settings, domain.UpdatePolicy{Trigger: domain.UpdateTriggerUser, Delivery: domain.UpdateDeliveryVisible, BudgetAuthority: domain.BudgetAuthorityUser})
	if err != nil {
		t.Fatal(err)
	}
	for _, run := range session.Runs {
		if _, err := state.StartRun(ctx, run.ID, map[string]any{}); err != nil {
			t.Fatal(err)
		}
		if run.Source == domain.SourceX {
			if cmd, err := e.ClaimCommand(ctx, run.ID, "synthetic"); err != nil || cmd != nil {
				t.Fatalf("X claimed: %+v %v", cmd, err)
			}
		}
	}
	id, err := e.PendingBridgeRunID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	run, err := state.GetRun(ctx, id)
	if err != nil || run.Source != domain.SourceLinkedIn {
		t.Fatalf("non-X queue blocked %q %v", id, err)
	}
	for _, source := range []domain.Source{domain.SourceLinkedIn, domain.SourceFacebook, domain.SourceInstagram} {
		release, ok := e.ordinarySourceRoute(source)
		release()
		if !ok {
			t.Fatalf("non-X blocked: %s", source)
		}
	}
	_, selected, err := e.PrepareXManualReader(ctx, readerbroker.Request{RequestID: "split_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Source: "x", URL: "https://x.com/synthetic/status/123"}, "marker")
	if !selected || err == nil {
		t.Fatal("reader silently fell back to ordinary profile")
	}
	if err := e.StopXManual(ctx); err != nil {
		t.Fatal(err)
	}
	if _, ok := e.ordinarySourceRoute(domain.SourceX); ok {
		t.Fatal("Stop reopened legacy X")
	}
}

func TestXDailyMixedSourcesProgressWithoutSettingsChanges(t *testing.T) {
	e, state := testEngine(t)
	ctx := context.Background()
	e.config.Capture.XMode = config.XCaptureConcurrent
	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.ActiveSources = []domain.Source{domain.SourceX, domain.SourceLinkedIn}
	if err = state.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	before, _ := state.GetSettings(ctx)
	session, err := e.StartVisibleUpdate(ctx, "mixed daily step one")
	if err != nil {
		t.Fatal(err)
	}
	if len(session.Runs) != 1 || session.Runs[0].Source != domain.SourceLinkedIn {
		t.Fatalf("X was queued %+v", session.Runs)
	}
	after, _ := state.GetSettings(ctx)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("saved active sources or Auto Update changed")
	}
	if _, err := e.dailyCaptureSources([]domain.Source{domain.SourceX}); err == nil || !strings.Contains(err.Error(), "batch-adapter-required") {
		t.Fatal("missing explicit X readiness error")
	}
}

func TestXDailyQueuedRunDoesNotStrandFollowingSource(t *testing.T) {
	e, state := testEngine(t)
	ctx := context.Background()
	e.config.Capture.XMode = config.XCaptureConcurrent
	settings, _ := state.GetSettings(ctx)
	settings.ActiveSources = []domain.Source{domain.SourceX, domain.SourceLinkedIn}
	session, err := state.CreateUpdateSession(ctx, "restart queued work", settings, domain.UpdatePolicy{Trigger: domain.UpdateTriggerUser, Delivery: domain.UpdateDeliveryVisible, BudgetAuthority: domain.BudgetAuthorityUser})
	if err != nil {
		t.Fatal(err)
	}
	next, err := e.startNext(ctx, session.ID)
	if err != nil || next == nil || next.Source != domain.SourceLinkedIn {
		t.Fatalf("next %+v err=%v", next, err)
	}
	session, err = state.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, run := range session.Runs {
		if run.Source == domain.SourceX && (run.Status != "failed" || run.Error == nil || run.Error.Code != "x_concurrent_not_ready") {
			t.Fatalf("X not explicitly rejected %+v", run)
		}
	}
}

func TestXDailyRestartReconcilesStartedCommands(t *testing.T) {
	for _, tc := range []struct{ mixed, claimed bool }{{false, false}, {true, false}, {false, true}, {true, true}} {
		mixed := tc.mixed
		t.Run(map[bool]string{false: "X only", true: "mixed"}[mixed]+map[bool]string{false: " queued", true: " claimed"}[tc.claimed], func(t *testing.T) {
			e, state := testEngine(t)
			ctx := context.Background()
			settings, _ := state.GetSettings(ctx)
			settings.ActiveSources = []domain.Source{domain.SourceX}
			if mixed {
				settings.ActiveSources = append(settings.ActiveSources, domain.SourceLinkedIn)
			}
			session, err := state.CreateUpdateSession(ctx, "old process", settings, domain.UpdatePolicy{Trigger: domain.UpdateTriggerUser, Delivery: domain.UpdateDeliveryVisible, BudgetAuthority: domain.BudgetAuthorityUser})
			if err != nil {
				t.Fatal(err)
			}
			for _, run := range session.Runs {
				if run.Source == domain.SourceX {
					if _, err = state.StartRun(ctx, run.ID, map[string]any{}); err != nil {
						t.Fatal(err)
					}
					if tc.claimed {
						if _, err = state.ClaimCommand(ctx, run.ID, "old-synthetic"); err != nil {
							t.Fatal(err)
						}
					}
				}
			}
			cfg := e.config
			cfg.Capture.XMode = config.XCaptureConcurrent
			restarted := New(state, e.provider, cfg, e.logger)
			if err = restarted.ReconcileXDailyStartup(ctx); err != nil {
				t.Fatal(err)
			}
			if err = restarted.ReconcileXDailyStartup(ctx); err != nil {
				t.Fatal("reconciliation must be idempotent", err)
			}
			if err = restarted.ResumeXDailyScheduling(ctx); err != nil {
				t.Fatal(err)
			}
			session, err = state.GetSession(ctx, session.ID)
			if err != nil {
				t.Fatal(err)
			}
			for _, run := range session.Runs {
				if run.Source == domain.SourceX && (run.Status != "failed" || run.BridgeCommandStatus != "failed") {
					t.Fatalf("old X command stranded %+v", run)
				}
				if run.Source == domain.SourceLinkedIn && run.Status != "waiting_for_bridge" {
					t.Fatalf("non-X not resumed %+v", run)
				}
			}
			if !mixed {
				if active, err := state.ActiveSession(ctx); err != nil || active != nil {
					t.Fatalf("X-only session stranded %+v %v", active, err)
				}
			}
		})
	}
}

func TestXDailyRestartPreservesAcceptedObservation(t *testing.T) {
	e, state := testEngine(t)
	ctx := context.Background()
	settings, _ := state.GetSettings(ctx)
	settings.ActiveSources = []domain.Source{domain.SourceX}
	session, err := state.CreateUpdateSession(ctx, "accepted observation", settings, domain.UpdatePolicy{Trigger: domain.UpdateTriggerUser, Delivery: domain.UpdateDeliveryVisible, BudgetAuthority: domain.BudgetAuthorityUser})
	if err != nil {
		t.Fatal(err)
	}
	run := session.Runs[0]
	command, err := state.StartRun(ctx, run.ID, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = state.ClaimCommand(ctx, run.ID, "synthetic"); err != nil {
		t.Fatal(err)
	}
	if err = state.SaveObservation(ctx, command.ID, run.ID, domain.Observation{Source: domain.SourceX, CapturedAt: domain.Now()}); err != nil {
		t.Fatal(err)
	}
	if _, err = state.QueueFollowUp(ctx, run.ID, map[string]any{}); err != nil {
		t.Fatal(err)
	}
	e.config.Capture.XMode = config.XCaptureConcurrent
	if err = e.ReconcileXDailyStartup(ctx); err != nil {
		t.Fatal(err)
	}
	after, err := state.GetRun(ctx, run.ID)
	if err != nil || after.Status != "reasoning" || after.BridgeCommandStatus != "failed" {
		t.Fatalf("accepted run lost %+v %v", after, err)
	}
	observations, err := state.Observations(ctx, run.ID)
	if err != nil || len(observations) != 1 {
		t.Fatal("accepted observation lost", err)
	}
}
