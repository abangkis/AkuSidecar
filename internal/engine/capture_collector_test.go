package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/collection"
	"github.com/abangkis/AkuSidecar/internal/collection/headless"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/AkuSidecar/internal/store"
)

type failingQuietBackend struct{}

func TestUnavailableNativeTargetDoesNotAutomaticallyRetry(t *testing.T) {
	err := &headless.CaptureError{Code: "target_unavailable", Message: "Facebook reports this post is unavailable in the current session"}
	for _, backend := range []string{collection.BackendHeadless, collection.BackendQuiet} {
		failure := internalCollectorFailure(err, backend)
		if failure.Retryable || failure.Stage != "capture" || failure.Message != err.Error() {
			t.Fatalf("target failure lost its scoped non-retryable outcome: %+v", failure)
		}
	}
}

func (failingQuietBackend) Capture(context.Context, domain.Source, map[string]any) (domain.Observation, error) {
	return domain.Observation{}, errors.New("quiet backend unavailable")
}

func TestQuietCollectorPinsFollowupAndRejectsBridgeClaim(t *testing.T) {
	ctx := context.Background()
	e, state := singleSourceEngine(t, reasoning.Deterministic{})
	m := attachTestCapture(t, e)
	c := collection.NewCoordinator(m, nil, func() error { return nil })
	c.SetBrowserCollector(m.Snapshot().Generation, failingQuietBackend{})
	e.AttachCollectionCoordinator(c)
	session, err := e.StartVisibleUpdate(ctx, "quiet collector")
	if err != nil {
		t.Fatal(err)
	}
	run := session.Runs[0]
	if route, err := e.RunCaptureCollector(ctx, run.ID); err != nil || route != collection.BackendQuiet {
		t.Fatalf("route=%s %v", route, err)
	}
	if pending, err := e.PendingBridgeRunID(ctx); err != nil || pending != "" {
		t.Fatalf("quiet exposed to Bridge pending=%s %v", pending, err)
	}
	if claim, err := e.ClaimCommand(ctx, run.ID, "bridge"); err != nil || claim != nil {
		t.Fatalf("Bridge claimed quiet=%+v %v", claim, err)
	}
	queued, err := state.GetRun(ctx, run.ID)
	if err != nil || queued.BridgeCommandStatus != "queued" {
		t.Fatalf("quiet queue mutated=%+v %v", queued, err)
	}
	settings, _ := state.GetSettings(ctx)
	settings.CaptureVisibility = "adaptive_fidelity"
	if err := state.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	c.SetBrowserCollector(m.Snapshot().Generation, nil)
	payload, err := e.ownedCapturePayload(run, session.ID, settings, 2, map[string]any{"anchorKeys": []string{"x:one"}}, "followup")
	if err != nil {
		t.Fatal(err)
	}
	if route, err := store.CaptureCollector(payload); err != nil || route != collection.BackendQuiet {
		t.Fatalf("followup rerouted=%s %v", route, err)
	}
	command, err := e.claimCommandForCollector(ctx, run.ID, "quiet", "browser", collection.BackendQuiet)
	if err != nil || command == nil {
		t.Fatalf("quiet claim=%+v %v", command, err)
	}
	if stamp := command.Payload["captureRuntime"].(map[string]any); stamp["driver"] != "browser" {
		t.Fatal("collector changed owner", stamp)
	}
	if _, err := c.Capture(ctx, run.Source, command.Payload); err == nil {
		t.Fatal("missing backend silently fell back")
	}
	_ = e.CancelSession(ctx, session.ID)
}

func TestQuietCollectorAuthorizationAndOtherBrowserSources(t *testing.T) {
	e, state := testEngine(t)
	m := attachTestCapture(t, e)
	c := collection.NewCoordinator(m, nil, func() error { return nil })
	c.SetBrowserCollector(m.Snapshot().Generation, failingQuietBackend{})
	e.AttachCollectionCoordinator(c)
	settings, _ := state.GetSettings(context.Background())
	for _, visibility := range []string{"quiet", "quiet_multi_window", "adaptive_fidelity"} {
		settings.CaptureVisibility = visibility
		for source, want := range map[domain.Source]string{domain.SourceX: collection.BackendQuiet, domain.SourceFacebook: collection.BackendQuiet, domain.SourceLinkedIn: collection.BackendBridge, domain.SourceInstagram: collection.BackendBridge} {
			if visibility == "adaptive_fidelity" {
				want = collection.BackendBridge
			}
			if got := e.selectCaptureCollector(source, settings, "browser"); got != want {
				t.Fatalf("visibility %s source %s route=%s want=%s", visibility, source, got, want)
			}
		}
	}
	settings.CaptureVisibility = "quiet"
	if got := len(e.grantedActiveSources(settings)); got != len(settings.ActiveSources) {
		t.Fatalf("browser sources disappeared: %d of %d", got, len(settings.ActiveSources))
	}
	heartbeat := ExpectedHeartbeat()
	for i := range heartbeat.SourceAccess.Sources {
		if heartbeat.SourceAccess.Sources[i].Source == "x" {
			heartbeat.SourceAccess.Sources[i].PermissionGranted = false
		}
	}
	e.RecordHeartbeat(heartbeat)
	if route := e.selectCaptureCollector(domain.SourceX, settings, "browser"); route != collection.BackendBridge {
		t.Fatalf("quiet bypassed permission: %s", route)
	}
	settings.CaptureVisibility = "adaptive_fidelity"
	if route := e.selectCaptureCollector(domain.SourceFacebook, settings, "browser"); route != collection.BackendBridge {
		t.Fatalf("Adaptive rerouted: %s", route)
	}
}

func TestAdoptedBridgeCommandDoesNotBecomeQuiet(t *testing.T) {
	ctx := context.Background()
	e, state := singleSourceEngine(t, reasoning.Deterministic{})
	session, err := e.StartVisibleUpdate(ctx, "before managed collector")
	if err != nil {
		t.Fatal(err)
	}
	m := attachTestCapture(t, e)
	c := collection.NewCoordinator(m, nil, func() error { return nil })
	c.SetBrowserCollector(m.Snapshot().Generation, failingQuietBackend{})
	e.AttachCollectionCoordinator(c)
	run := session.Runs[0]
	settings, _ := state.GetSettings(ctx)
	payload, err := e.ownedCapturePayload(run, session.ID, settings, 2, nil, "restored")
	if err != nil {
		t.Fatal(err)
	}
	if route, err := store.CaptureCollector(payload); err != nil || route != collection.BackendBridge {
		t.Fatalf("adopted route=%s %v", route, err)
	}
	if claim, err := e.ClaimCommand(ctx, run.ID, "bridge"); err != nil || claim == nil {
		t.Fatalf("adopted bridge command=%+v %v", claim, err)
	}
	_ = e.CancelSession(ctx, session.ID)
}

func TestQuietCollectorRevocationPreventsRunAndMediaCapture(t *testing.T) {
	ctx := context.Background()
	e, _ := singleSourceEngine(t, reasoning.Deterministic{})
	m := attachTestCapture(t, e)
	c := collection.NewCoordinator(m, nil, func() error { return nil })
	c.SetBrowserCollector(m.Snapshot().Generation, failingQuietBackend{})
	e.AttachCollectionCoordinator(c)
	session, err := e.StartVisibleUpdate(ctx, "revocation fence")
	if err != nil {
		t.Fatal(err)
	}
	command, err := e.claimCommandForCollector(ctx, session.Runs[0].ID, "quiet", "browser", collection.BackendQuiet)
	if err != nil || command == nil {
		t.Fatalf("quiet claim=%+v %v", command, err)
	}
	heartbeat := ExpectedHeartbeat()
	for i := range heartbeat.SourceAccess.Sources {
		if heartbeat.SourceAccess.Sources[i].Source == "x" {
			heartbeat.SourceAccess.Sources[i].PermissionGranted = false
		}
	}
	e.RecordHeartbeat(heartbeat)
	for _, payload := range []map[string]any{command.Payload, {"pageUrl": "https://x.com/example/status/12345", "captureCollector": collectorStamp(collection.BackendQuiet)}} {
		_, err := e.captureForCollector(ctx, domain.SourceX, payload, collection.BackendQuiet)
		var failure *headless.CaptureError
		if !errors.As(err, &failure) || failure.Code != "source_access_revoked" {
			t.Fatalf("backend reached after revocation: %v", err)
		}
		if route, err := store.CaptureCollector(payload); err != nil || route != collection.BackendQuiet {
			t.Fatalf("revocation changed route: %s %v", route, err)
		}
		if result := internalCollectorFailure(err, collection.BackendQuiet); result.Retryable || result.Code != "quiet_source_access_revoked" {
			t.Fatalf("revocation failure=%+v", result)
		}
	}
	_ = e.CancelSession(ctx, session.ID)
}
