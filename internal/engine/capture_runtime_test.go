package engine

import (
	"context"
	"errors"
	"io"
	"log"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/collection"
	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
)

type runtimeTestProcess struct {
	done chan error
	once sync.Once
}
type headlessTestProcess struct{ *runtimeTestProcess }

func (p *headlessTestProcess) Driver() string { return "headless" }

func TestAdditionalHeadlessSourcesRequireRetainedPermissionAndScript(t *testing.T) {
	for _, wanted := range []domain.Source{domain.SourceInstagram, domain.SourceLinkedIn} {
		for _, missing := range []string{"none", "permission", "script", "readiness"} {
			heartbeat := ExpectedHeartbeat()
			for i := range heartbeat.SourceAccess.Sources {
				access := &heartbeat.SourceAccess.Sources[i]
				access.Ready = domain.Source(access.Source) == wanted
				if domain.Source(access.Source) == wanted {
					if missing == "permission" {
						access.PermissionGranted = false
					}
					if missing == "script" {
						access.ScriptRegistered = false
					}
					if missing == "readiness" {
						access.Ready = false
					}
				}
			}
			got := headlessGrantedSources(BridgeStatus{Compatible: true, Actual: &heartbeat})
			if missing == "none" {
				if !reflect.DeepEqual(got, []domain.Source{wanted}) {
					t.Fatalf("authorized added source not retained: %v", got)
				}
			} else if len(got) != 0 {
				t.Fatalf("%s source admitted without %s", wanted, missing)
			}
		}
	}
}

func TestFreshBrowserHeartbeatRevokesRetainedHeadlessAuthority(t *testing.T) {
	e, _ := testEngine(t)
	if len(e.headlessAccess) != 4 {
		t.Fatalf("initial retained access=%v", e.headlessAccess)
	}
	heartbeat := ExpectedHeartbeat()
	heartbeat.SourceAccess.GrantedSources = []string{"facebook", "linkedin", "instagram"}
	for i := range heartbeat.SourceAccess.Sources {
		if heartbeat.SourceAccess.Sources[i].Source == "x" {
			heartbeat.SourceAccess.Sources[i].PermissionGranted = false
			heartbeat.SourceAccess.Sources[i].ScriptRegistered = false
			heartbeat.SourceAccess.Sources[i].Ready = false
			heartbeat.SourceAccess.Sources[i].Reason = "permission_not_granted"
		}
	}
	if status := e.RecordHeartbeat(heartbeat); !status.Compatible {
		t.Fatal(status.Reasons)
	}
	if len(e.headlessAccess) != 3 {
		t.Fatal("revoked X remained authorized", e.headlessAccess)
	}
	for _, source := range e.headlessAccess {
		if source == domain.SourceX {
			t.Fatal("revoked X remained authorized")
		}
	}
	heartbeat.ContractVersion = "incompatible"
	e.RecordHeartbeat(heartbeat)
	if len(e.headlessAccess) != 0 {
		t.Fatal("unknown access remained authorized")
	}
}
func TestHeadlessClaimsCannotCrossDriverAndRetainGrantedAccess(t *testing.T) {
	ctx := context.Background()
	e, _ := singleSourceEngine(t, nil)
	m := attachTestCapture(t, e)
	c := collection.NewCoordinator(m, nil, func() error { return nil })
	e.AttachCollectionCoordinator(c)
	settings, err := e.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.CollectionMode = "headless"
	if _, err := e.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	e.ResetCaptureHeartbeat()
	if err := m.Replace(ctx, func(context.Context, uint64) (captureruntime.Process, error) {
		return &headlessTestProcess{&runtimeTestProcess{done: make(chan error, 1)}}, nil
	}); err != nil {
		t.Fatal(err)
	}
	session, err := e.StartVisibleUpdate(ctx, "headless owner")
	if err != nil {
		t.Fatal(err)
	}
	id := session.Runs[0].ID
	if command, err := e.ClaimCommand(ctx, id, "old-browser"); command != nil || !errors.Is(err, errStaleCaptureRuntime) {
		t.Fatalf("cross-driver=%+v %v", command, err)
	}
	command, err := e.claimCommandForDriver(ctx, id, "headless", "headless")
	if err != nil || command == nil {
		t.Fatalf("headless=%+v %v", command, err)
	}
	if stamp := command.Payload["captureRuntime"].(map[string]any); stamp["driver"] != "headless" {
		t.Fatal(stamp)
	}
	if len(e.CollectionRuntime().AuthorizedSources) == 0 {
		t.Fatal("confirmed grants lost")
	}
	e.mu.Lock()
	e.headlessAccess = nil
	e.mu.Unlock()
	settings, _ = e.Settings(ctx)
	if len(e.grantedActiveSources(settings)) != 0 {
		t.Fatal("headless bypassed source consent")
	}
	_ = e.CancelSession(ctx, session.ID)
}

func (p *runtimeTestProcess) Done() <-chan error                         { return p.done }
func (p *runtimeTestProcess) PID() int                                   { return 123 }
func (p *runtimeTestProcess) Terminate()                                 { p.once.Do(func() { p.done <- nil }) }
func (p *runtimeTestProcess) CloseForRetry(context.Context) error        { p.Terminate(); return nil }
func (p *runtimeTestProcess) OpenExtensionsPage(context.Context) error   { return nil }
func (p *runtimeTestProcess) ReplacementReadiness(context.Context) error { return nil }

func attachTestCapture(t *testing.T, engine *Engine) *captureruntime.Manager {
	t.Helper()
	manager, err := captureruntime.New(&runtimeTestProcess{done: make(chan error, 1)})
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.AttachCaptureRuntime(context.Background(), manager); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { engine.Shutdown(); engine.WaitForIdle(time.Second); manager.Terminate() })
	return manager
}

func waitCaptureLeases(t *testing.T, manager *captureruntime.Manager, count int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if manager.Snapshot().ActiveLeases == count {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("leases=%d want=%d", manager.Snapshot().ActiveLeases, count)
}

func TestSessionLeasePinsQueuedCaptureAndCancelledWorkerDrain(t *testing.T) {
	ctx := context.Background()
	engine, _ := testEngine(t)
	manager := attachTestCapture(t, engine)
	session, err := engine.StartVisibleUpdate(ctx, "lease boundary")
	if err != nil {
		t.Fatal(err)
	}
	waitCaptureLeases(t, manager, 1)
	if err := manager.Replace(ctx, func(context.Context, uint64) (captureruntime.Process, error) {
		t.Fatal("active session replaced")
		return nil, nil
	}); !errors.Is(err, captureruntime.ErrBusy) {
		t.Fatalf("session not pinned: %v", err)
	}
	_, cancel := context.WithCancel(ctx)
	engine.mu.Lock()
	engine.active[session.Runs[0].ID] = cancel
	engine.mu.Unlock()
	if err := engine.CancelSession(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
	waitCaptureLeases(t, manager, 1)
	engine.mu.Lock()
	delete(engine.active, session.Runs[0].ID)
	engine.mu.Unlock()
	engine.releaseTerminalCaptureSessions(ctx)
	waitCaptureLeases(t, manager, 0)
	// Polling a terminal run must not leak a newly acquired lease.
	if _, err := engine.ClaimCommand(ctx, session.Runs[0].ID, "bridge-test"); err != nil {
		t.Fatal(err)
	}
	waitCaptureLeases(t, manager, 0)
}

func TestUnavailableRuntimeDoesNotCreatePartialSession(t *testing.T) {
	ctx := context.Background()
	engine, state := testEngine(t)
	manager := attachTestCapture(t, engine)
	if err := manager.Replace(ctx, func(context.Context, uint64) (captureruntime.Process, error) { return nil, errors.New("failed launch") }); err == nil {
		t.Fatal("expected launch failure")
	}
	if _, err := engine.StartVisibleUpdate(ctx, "blocked"); !errors.Is(err, captureruntime.ErrUnavailable) {
		t.Fatalf("admission error=%v", err)
	}
	active, err := state.ActiveSession(ctx)
	if err != nil || active != nil {
		t.Fatalf("partial session created: %+v %v", active, err)
	}
}

func TestPersistedCaptureOwnerRejectsPriorEpochBeforeObservationWrites(t *testing.T) {
	ctx := context.Background()
	engine, state := singleSourceEngine(t, nil)
	// No reasoning is launched: ownership validation precedes evidence validation.
	manager := attachTestCapture(t, engine)
	session, err := engine.StartVisibleUpdate(ctx, "owner fence")
	if err != nil {
		t.Fatal(err)
	}
	command, err := engine.ClaimCommand(ctx, session.Runs[0].ID, "bridge-test")
	if err != nil || command == nil {
		t.Fatalf("command=%+v %v", command, err)
	}
	stamp, ok := command.Payload["captureRuntime"].(map[string]any)
	if !ok || stamp["epoch"] != engine.epoch || stamp["driver"] != "browser" {
		t.Fatalf("missing persisted owner: %+v", command.Payload)
	}
	engine.epoch = "different-restart-epoch"
	if _, err := engine.AcceptObservation(ctx, command.ID, command.RunID, domain.Observation{}); !errors.Is(err, errStaleCaptureRuntime) {
		t.Fatalf("stale result accepted: %v", err)
	}
	observations, err := state.Observations(ctx, command.RunID)
	if err != nil || len(observations) != 0 {
		t.Fatalf("stale evidence persisted: %d %v", len(observations), err)
	}
	if manager.Snapshot().ActiveLeases != 1 {
		t.Fatal("rejected callback released active session")
	}
	if err := engine.CancelSession(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
}

func TestClaimDoesNotDispatchPriorEpochCommand(t *testing.T) {
	ctx := context.Background()
	engine, _ := singleSourceEngine(t, nil)
	manager := attachTestCapture(t, engine)
	session, err := engine.StartVisibleUpdate(ctx, "stale dispatch")
	if err != nil {
		t.Fatal(err)
	}
	engine.epoch = "replacement-epoch"
	command, err := engine.ClaimCommand(ctx, session.Runs[0].ID, "bridge-test")
	if command != nil || !errors.Is(err, errStaleCaptureRuntime) {
		t.Fatalf("stale command dispatched: %+v %v", command, err)
	}
	waitCaptureLeases(t, manager, 0)
}

func assertSameCaptureStamp(t *testing.T, first, second map[string]any) {
	t.Helper()
	if !reflect.DeepEqual(first["captureRuntime"], second["captureRuntime"]) {
		t.Fatalf("owner changed between rounds: %+v %+v", first["captureRuntime"], second["captureRuntime"])
	}
}

func TestMediaRecaptureLeaseAndOwnerFence(t *testing.T) {
	ctx := context.Background()
	engine, _ := singleSourceEngine(t, reasoning.Deterministic{})
	manager := attachTestCapture(t, engine)
	session, err := engine.StartVisibleUpdate(ctx, "media owner")
	if err != nil {
		t.Fatal(err)
	}
	command, err := engine.ClaimCommand(ctx, session.Runs[0].ID, "bridge-test")
	if err != nil || command == nil {
		t.Fatalf("claim=%+v %v", command, err)
	}
	observation := domain.Observation{Source: domain.SourceX, CapturedAt: domain.Now(), Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{EvidenceKey: "x:status:123456789012345", PlatformID: "x:status:123456789012345", Permalink: "https://x.com/author/status/123456789012345", Author: "Author", Text: "Captured evidence with unavailable media", MediaRecovery: map[string]any{"outcome": "unavailable"}}}}}, Coverage: map[string]any{"quality": "complete"}}
	if _, err := engine.AcceptObservation(ctx, command.ID, command.RunID, observation); err != nil {
		t.Fatal(err)
	}
	completed := waitSession(t, engine, session.ID, func(value domain.Session) bool { return value.Status == "completed" })
	waitCaptureLeases(t, manager, 0)
	if len(completed.Items) != 1 {
		t.Fatalf("items=%d", len(completed.Items))
	}
	job, err := engine.QueueMediaRecapture(ctx, completed.Items[0].ID, domain.MediaRecaptureBackground)
	if err != nil {
		t.Fatal(err)
	}
	waitCaptureLeases(t, manager, 1)
	if _, err := engine.QueueMediaRecapture(ctx, completed.Items[0].ID, domain.MediaRecaptureBackground); err == nil {
		t.Fatal("duplicate recapture admitted")
	}
	waitCaptureLeases(t, manager, 1)
	if err := manager.Replace(ctx, func(context.Context, uint64) (captureruntime.Process, error) {
		t.Fatal("active recapture replaced")
		return nil, nil
	}); !errors.Is(err, captureruntime.ErrBusy) {
		t.Fatalf("recapture not pinned: %v", err)
	}
	claimed, err := engine.ClaimMediaRecapture(ctx, job.ID, "bridge-test")
	if err != nil || claimed.Payload["captureRuntime"] == nil {
		t.Fatalf("owner not persisted: %+v %v", claimed, err)
	}
	if _, err := engine.FailMediaRecapture(ctx, job.ID, domain.Failure{Code: "test_capture_failed", Stage: "capture", Message: "controlled failure"}); err != nil {
		t.Fatal(err)
	}
	waitCaptureLeases(t, manager, 0)
	job, err = engine.QueueMediaRecapture(ctx, completed.Items[0].ID, domain.MediaRecaptureBackground)
	if err != nil {
		t.Fatal(err)
	}
	engine.epoch = "new-recapture-epoch"
	if _, err := engine.ClaimMediaRecapture(ctx, job.ID, "bridge-test"); !errors.Is(err, errStaleCaptureRuntime) {
		t.Fatalf("stale media dispatched: %v", err)
	}
	waitCaptureLeases(t, manager, 0)
	if _, err := engine.ClaimMediaRecapture(ctx, "missing-recapture", "bridge-test"); err == nil {
		t.Fatal("missing recapture accepted")
	}
	waitCaptureLeases(t, manager, 0)
}

func TestHeadlessPhotoMediaRecapturePreservesSavedFacebookItem(t *testing.T) {
	ctx := context.Background()
	engine, state := testEngine(t)
	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.ActiveSources = []domain.Source{domain.SourceFacebook}
	if err := state.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	manager := attachTestCapture(t, engine)
	coordinator := collection.NewCoordinator(manager, nil, func() error { return nil })
	engine.AttachCollectionCoordinator(coordinator)
	session, err := engine.StartVisibleUpdate(ctx, "save Facebook photo")
	if err != nil {
		t.Fatal(err)
	}
	var facebookRun string
	for _, run := range session.Runs {
		if run.Source == domain.SourceFacebook {
			facebookRun = run.ID
		}
	}
	if facebookRun == "" {
		t.Fatalf("Facebook run missing: %+v", session.Runs)
	}
	command, err := engine.ClaimCommand(ctx, facebookRun, "bridge-test")
	if err != nil || command == nil {
		t.Fatalf("seed claim=%+v err=%v", command, err)
	}
	photoURL := "https://www.facebook.com/photo?fbid=123"
	original := domain.Block{
		EvidenceKey:   "facebook:post:000000000000000000000123",
		PlatformID:    "facebook:post:000000000000000000000123",
		Permalink:     photoURL,
		Author:        "Saved photo author",
		Text:          "A saved caption that must survive media recovery.",
		MediaRecovery: map[string]any{"outcome": "unavailable"},
	}
	seed := domain.Observation{Source: domain.SourceFacebook, PageURL: photoURL, CapturedAt: domain.Now(),
		Snapshots: []domain.Snapshot{{Blocks: []domain.Block{original}}}, Coverage: map[string]any{"quality": "complete"}}
	if _, err := engine.AcceptObservation(ctx, command.ID, facebookRun, seed); err != nil {
		t.Fatal(err)
	}
	completed := waitSession(t, engine, session.ID, func(value domain.Session) bool { return value.Status == "completed" })
	if len(completed.Items) != 1 {
		t.Fatalf("seed timeline items=%d", len(completed.Items))
	}
	waitCaptureLeases(t, manager, 0)
	settings.CollectionMode = "headless"
	if _, err := engine.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.QueueMediaRecapture(ctx, completed.Items[0].ID, domain.MediaRecaptureBackground); !errors.Is(err, ErrFacebookRecaptureUnavailable) {
		t.Fatalf("saved headless Facebook recapture did not fail closed: %v", err)
	}
	settings.CollectionMode = "browser"
	if _, err := engine.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	engine.ResetCaptureHeartbeat()
	if err := manager.Replace(ctx, func(context.Context, uint64) (captureruntime.Process, error) {
		return &headlessTestProcess{&runtimeTestProcess{done: make(chan error, 1)}}, nil
	}); err != nil {
		t.Fatal(err)
	}
	coordinator.Request("headless")
	job, err := engine.QueueMediaRecapture(ctx, completed.Items[0].ID, domain.MediaRecaptureBackground)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.ClaimMediaRecapture(ctx, job.ID, "bridge-test"); !errors.Is(err, errStaleCaptureRuntime) {
		t.Fatalf("Bridge claimed headless work: %v", err)
	}
	claimed, err := engine.claimMediaRecaptureForDriver(ctx, job.ID, "headless-test", "headless")
	if err != nil || claimed.Status != "claimed" {
		t.Fatalf("headless claim=%+v err=%v", claimed, err)
	}
	stamp, ok := claimed.Payload["captureRuntime"].(map[string]any)
	if !ok || stamp["driver"] != "headless" {
		t.Fatalf("headless owner stamp=%+v", claimed.Payload["captureRuntime"])
	}
	result := domain.Observation{Source: domain.SourceFacebook, PageURL: photoURL, CapturedAt: domain.Now(),
		Coverage: map[string]any{"photoMediaRecapture": map[string]any{
			"status": "verified", "photoId": "123", "ownerId": "456", "provenance": "exact_photo_metadata_and_visible_image",
		}},
		Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{PlatformID: "facebook:photo:123", Permalink: photoURL,
			Media: []map[string]any{{"kind": "image", "url": "https://media.fbcdn.net/photo.jpg"}},
		}}}},
	}
	done, err := engine.acceptMediaRecapture(ctx, job.ID, result, true)
	if err != nil || done.Outcome != "recovered" {
		t.Fatalf("headless completion=%+v err=%v", done, err)
	}
	items, err := engine.Timeline(ctx, 10, 0)
	if err != nil || len(items) != 1 {
		t.Fatalf("timeline rows=%d err=%v", len(items), err)
	}
	saved := items[0].Evidence
	if saved == nil || saved.EvidenceKey != original.EvidenceKey || saved.PlatformID != original.PlatformID ||
		saved.Permalink != original.Permalink || saved.Author != original.Author || saved.Text != original.Text || len(saved.Media) != 1 {
		t.Fatalf("saved photo identity/content changed: %+v", saved)
	}
}

func TestCaptureGenerationFenceRejectsMalformedValues(t *testing.T) {
	for _, value := range []any{nil, "1", float64(1.5), float64(0), -1, float64(2)} {
		if captureGenerationMatches(value, 1) {
			t.Fatalf("invalid generation accepted: %v", value)
		}
	}
	if !captureGenerationMatches(1, 1) || !captureGenerationMatches(float64(1), 1) {
		t.Fatal("valid generation rejected")
	}
}

type hybridGateProvider struct {
	reasoning.Deterministic
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (p *hybridGateProvider) Analyze(ctx context.Context, run domain.Run, observation domain.Observation, knowledge []domain.ReasonedItem) (domain.ReasoningResult, domain.ReasoningTelemetry, error) {
	p.once.Do(func() { close(p.started) })
	select {
	case <-p.release:
		return p.Deterministic.Analyze(ctx, run, observation, knowledge)
	case <-ctx.Done():
		return domain.ReasoningResult{}, domain.ReasoningTelemetry{}, ctx.Err()
	}
}

func newHybridCollectionEngine(t *testing.T, provider reasoning.Provider) (*Engine, *captureruntime.Manager, *collection.Coordinator, context.CancelFunc) {
	t.Helper()
	engine, state := testEngine(t)
	if provider != nil {
		engine.provider = provider
	}
	initial := &headlessTestProcess{&runtimeTestProcess{done: make(chan error, 1)}}
	manager, err := captureruntime.New(initial)
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.AttachCaptureRuntime(context.Background(), manager); err != nil {
		t.Fatal(err)
	}
	coordinator := collection.NewCoordinator(manager, func(_ context.Context, mode string, _ uint64) (captureruntime.Process, error) {
		process := &runtimeTestProcess{done: make(chan error, 1)}
		if mode == "headless" {
			return &headlessTestProcess{process}, nil
		}
		return process, nil
	}, func() error { return nil })
	coordinator.Request("headless")
	engine.AttachCollectionCoordinator(coordinator)
	settings, err := state.GetSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	settings.CollectionMode = "headless"
	settings.ActiveSources = []domain.Source{domain.SourceFacebook, domain.SourceX}
	if _, err := engine.SaveSettings(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	coordinatorCtx, cancelCoordinator := context.WithCancel(context.Background())
	coordinator.Start(coordinatorCtx)
	t.Cleanup(func() {
		cancelCoordinator()
		engine.Shutdown()
		engine.WaitForIdle(time.Second)
		manager.Terminate()
	})
	return engine, manager, coordinator, cancelCoordinator
}

func TestHybridHeadlessDrainsReasoningBeforeFacebookBridgeAndCleanup(t *testing.T) {
	ctx := context.Background()
	provider := &hybridGateProvider{started: make(chan struct{}), release: make(chan struct{})}
	engine, manager, coordinator, _ := newHybridCollectionEngine(t, provider)
	session, err := engine.StartVisibleUpdate(ctx, "hybrid sources")
	if err != nil {
		t.Fatal(err)
	}
	if len(session.Runs) != 2 || session.Runs[0].Source != domain.SourceX || session.Runs[1].Source != domain.SourceFacebook {
		t.Fatalf("frozen execution order=%v", []domain.Source{session.Runs[0].Source, session.Runs[1].Source})
	}
	first := session.Runs[0]
	command, err := engine.claimCommandForDriver(ctx, first.ID, "headless-test", "headless")
	if err != nil || command == nil {
		t.Fatalf("headless first claim=%+v err=%v", command, err)
	}
	if route, _, routeErr := engine.store.FirstCommandCollector(ctx, first.ID); routeErr != nil || route != collection.BackendHeadless {
		t.Fatalf("headless route=%q err=%v", route, routeErr)
	}
	observation := domain.Observation{
		Source: domain.SourceX, CapturedAt: domain.Now(),
		Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{EvidenceKey: "x:hybrid-one", Text: "A bounded X update"}}}},
		Coverage:  map[string]any{"status": "complete"},
	}
	if _, err := engine.AcceptObservation(ctx, command.ID, first.ID, observation); err != nil {
		t.Fatal(err)
	}
	select {
	case <-provider.started:
	case <-time.After(time.Second):
		t.Fatal("reasoning did not enter the controlled gate")
	}
	waitHybridCondition(t, "Facebook collection intent while predecessor reasoning is active", func() bool {
		active, loadErr := engine.Session(ctx, session.ID)
		return loadErr == nil && active.Runs[0].Status == "reasoning" && active.Runs[1].Status == "queued" &&
			coordinator.Status().CollectionBorrowSource == domain.SourceFacebook
	})
	if snapshot := manager.Snapshot(); snapshot.Driver != "headless" || snapshot.ActiveLeases != 1 {
		t.Fatalf("headless owner released before reasoning drained: %+v", snapshot)
	}
	if err := manager.Replace(ctx, func(context.Context, uint64) (captureruntime.Process, error) { return nil, nil }); !errors.Is(err, captureruntime.ErrBusy) {
		t.Fatalf("reasoning did not pin predecessor owner: %v", err)
	}

	// A fresh Browser heartbeat is required after the handoff. Revoked access
	// must leave the queued Facebook run unstamped until a ready heartbeat arrives.
	denied := ExpectedHeartbeat()
	for i := range denied.SourceAccess.Sources {
		if denied.SourceAccess.Sources[i].Source == string(domain.SourceFacebook) {
			denied.SourceAccess.Sources[i].Ready = false
			denied.SourceAccess.Sources[i].PermissionGranted = false
			denied.SourceAccess.Sources[i].ScriptRegistered = false
			denied.SourceAccess.Sources[i].Reason = "permission_not_granted"
		}
	}
	engine.RecordHeartbeat(denied)
	close(provider.release)
	waitSession(t, engine, session.ID, func(value domain.Session) bool { return value.Runs[0].Status == "completed" })
	waitHybridCondition(t, "Browser owner replacement after drain", func() bool {
		snapshot := manager.Snapshot()
		return snapshot.State == captureruntime.Ready && snapshot.Driver == "browser" && snapshot.Generation > 1
	})
	if _, err := engine.startNext(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
	active, err := engine.Session(ctx, session.ID)
	if err != nil || active.Runs[1].Status != "queued" {
		t.Fatalf("Facebook run started without fresh permission: %+v err=%v", active, err)
	}

	engine.RecordHeartbeat(ExpectedHeartbeat())
	if _, err := engine.startNext(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
	active, err = engine.Session(ctx, session.ID)
	if err != nil || active.Runs[1].Status != "waiting_for_bridge" {
		t.Fatalf("Facebook run did not start on Bridge: %+v err=%v", active, err)
	}
	facebook := active.Runs[1]
	if route, exists, routeErr := engine.store.FirstCommandCollector(ctx, facebook.ID); routeErr != nil || !exists || route != collection.BackendBridge {
		t.Fatalf("Facebook initial route=%q exists=%v err=%v", route, exists, routeErr)
	}

	settings, err := engine.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.CaptureVisibility = "quiet"
	if err := engine.store.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	followUp, err := engine.ownedCapturePayload(facebook, session.ID, settings, 2, map[string]any{"anchorKeys": []string{"facebook:post:123"}}, "follow-up")
	if err != nil {
		t.Fatal(err)
	}
	if followUp["captureCollector"].(map[string]any)["backend"] != collection.BackendBridge {
		t.Fatalf("quiet settings rewrote Facebook follow-up route: %+v", followUp["captureCollector"])
	}
	if wrong, err := engine.claimCommandForCollector(ctx, facebook.ID, "quiet-worker", "browser", collection.BackendQuiet); err != nil || wrong != nil {
		t.Fatalf("Quiet claimed Bridge work: command=%+v err=%v", wrong, err)
	}
	if wrong, err := engine.claimCommandForDriver(ctx, facebook.ID, "headless-worker", "headless"); wrong != nil || !errors.Is(err, errStaleCaptureRuntime) {
		t.Fatalf("headless crossed frozen Facebook route: command=%+v err=%v", wrong, err)
	}
	facebookCommand, err := engine.claimCommandForDriver(ctx, facebook.ID, "bridge-test", "browser")
	if err != nil || facebookCommand == nil {
		t.Fatalf("Bridge Facebook claim=%+v err=%v", facebookCommand, err)
	}
	stamp, ok := facebookCommand.Payload["captureRuntime"].(map[string]any)
	if !ok || stamp["driver"] != "browser" || !captureGenerationMatches(stamp["generation"], manager.Snapshot().Generation) {
		t.Fatalf("Facebook runtime stamp=%+v generation=%d", facebookCommand.Payload["captureRuntime"], manager.Snapshot().Generation)
	}

	cleanupStarted := make(chan struct{}, 1)
	cleanupRelease := make(chan struct{})
	cleanupCalls := make(chan struct {
		sessionID  string
		generation uint64
	}, 1)
	engine.SetBrowserCollectionCleanup(func(cleanupCtx context.Context, sessionID string, generation uint64) error {
		cleanupCalls <- struct {
			sessionID  string
			generation uint64
		}{sessionID: sessionID, generation: generation}
		cleanupStarted <- struct{}{}
		select {
		case <-cleanupRelease:
			return nil
		case <-cleanupCtx.Done():
			return cleanupCtx.Err()
		}
	})
	if err := engine.CancelSession(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-cleanupStarted:
	case <-time.After(time.Second):
		t.Fatal("terminal Browser surfaces were not scheduled for explicit cleanup")
	}
	cleanupCall := <-cleanupCalls
	if cleanupCall.sessionID != session.ID || cleanupCall.generation != manager.Snapshot().Generation {
		t.Fatalf("cleanup identity=%+v owner=%+v", cleanupCall, manager.Snapshot())
	}
	time.Sleep(30 * time.Millisecond)
	if manager.Snapshot().ActiveLeases != 1 || coordinator.Status().CollectionBorrowSource != domain.SourceFacebook {
		t.Fatalf("ownership released before cleanup acknowledgement: owner=%+v collection=%+v", manager.Snapshot(), coordinator.Status())
	}
	close(cleanupRelease)
	waitCaptureLeases(t, manager, 0)
	waitHybridCondition(t, "Facebook collection intent release", func() bool {
		return coordinator.Status().CollectionBorrowSource == ""
	})
}

func TestHybridRecoveryDoesNotAdoptMismatchedDurableCaptureDriver(t *testing.T) {
	ctx := context.Background()
	engine, manager, _, _ := newHybridCollectionEngine(t, nil)
	session, err := engine.StartVisibleUpdate(ctx, "durable hybrid recovery")
	if err != nil {
		t.Fatal(err)
	}
	run := session.Runs[0]
	driver, stamped, err := engine.store.FirstCommandCaptureDriver(ctx, run.ID)
	if err != nil || !stamped || driver != "headless" {
		t.Fatalf("durable command driver=%q stamped=%v err=%v", driver, stamped, err)
	}
	engine.releaseSessionCaptureLease(session.ID)
	restarted := New(engine.store, reasoning.Deterministic{}, config.Config{}, log.New(io.Discard, "", 0))
	browserManager, err := captureruntime.New(&runtimeTestProcess{done: make(chan error, 1)})
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.AttachCaptureRuntime(ctx, browserManager); err != nil {
		t.Fatal(err)
	}
	restartCoordinator := collection.NewCoordinator(browserManager, func(_ context.Context, mode string, _ uint64) (captureruntime.Process, error) {
		process := &runtimeTestProcess{done: make(chan error, 1)}
		if mode == "headless" {
			return &headlessTestProcess{process}, nil
		}
		return process, nil
	}, func() error { return nil })
	restartCoordinator.Request("headless")
	restarted.AttachCollectionCoordinator(restartCoordinator)
	if _, err := restarted.startNext(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
	active, err := restarted.Session(ctx, session.ID)
	if err != nil || active.Runs[1].Status != "queued" || browserManager.Snapshot().ActiveLeases != 0 {
		t.Fatalf("queued Facebook bypassed old headless command drain: session=%+v owner=%+v err=%v", active, browserManager.Snapshot(), err)
	}
	if browserManager.Snapshot().ActiveLeases != 0 {
		t.Fatalf("restart adopted a mismatched Browser owner: %+v", browserManager.Snapshot())
	}
	if command, err := restarted.claimCommandForDriver(ctx, run.ID, "browser", "browser"); command != nil || !errors.Is(err, errStaleCaptureRuntime) {
		t.Fatalf("Browser claimed immutable headless command: %+v %v", command, err)
	}
	if command, err := restarted.claimCommandForDriver(ctx, run.ID, "headless", "headless"); command != nil || !errors.Is(err, errStaleCaptureRuntime) {
		t.Fatalf("headless command dispatched without matching lease: %+v %v", command, err)
	}
	if driver, stamped, err := engine.store.FirstCommandCaptureDriver(ctx, run.ID); err != nil || !stamped || driver != "headless" {
		t.Fatalf("durable command was rewritten: driver=%q stamped=%v err=%v", driver, stamped, err)
	}
	t.Cleanup(func() {
		restarted.captureMu.Lock()
		releaseIntent := restarted.collectionIntents[session.ID]
		delete(restarted.collectionIntents, session.ID)
		restarted.captureMu.Unlock()
		if releaseIntent != nil {
			releaseIntent()
		}
		restarted.Shutdown()
		browserManager.Terminate()
		manager.Terminate()
	})
}

func TestHybridRecoveryRestoresBrowserIntentForTerminalFacebookRun(t *testing.T) {
	ctx := context.Background()
	engine, manager, _, stopOldCoordinator := newHybridCollectionEngine(t, nil)
	settings, err := engine.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.ActiveSources = []domain.Source{domain.SourceFacebook}
	if _, err := engine.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	session, err := engine.StartVisibleUpdate(ctx, "terminal Facebook recovery")
	if err != nil {
		t.Fatal(err)
	}
	waitHybridCondition(t, "Browser owner for Facebook-only session", func() bool {
		snapshot := manager.Snapshot()
		return snapshot.State == captureruntime.Ready && snapshot.Driver == "browser" && snapshot.Generation > 1
	})
	if _, err := engine.startNext(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
	active, err := engine.Session(ctx, session.ID)
	if err != nil || active.Runs[0].Status != "waiting_for_bridge" {
		t.Fatalf("Facebook command not started: session=%+v err=%v", active, err)
	}
	command, err := engine.claimCommandForDriver(ctx, active.Runs[0].ID, "bridge-test", "browser")
	if err != nil || command == nil {
		t.Fatalf("Facebook claim=%+v err=%v", command, err)
	}
	if _, err := engine.store.FailCommand(ctx, command.ID, command.RunID, domain.Failure{
		Code: "restart_window_fixture", Stage: "capture", Message: "controlled terminal state", Retryable: false,
	}); err != nil {
		t.Fatal(err)
	}
	active, err = engine.Session(ctx, session.ID)
	if err != nil || active.Status != "running" || active.Runs[0].Status != "failed" {
		t.Fatalf("expected terminal run before session finalization: session=%+v err=%v", active, err)
	}

	stopOldCoordinator()
	engine.releaseSessionCaptureLease(session.ID)
	engine.captureMu.Lock()
	oldIntent := engine.collectionIntents[session.ID]
	delete(engine.collectionIntents, session.ID)
	engine.captureMu.Unlock()
	if oldIntent != nil {
		oldIntent()
	}

	restarted := New(engine.store, reasoning.Deterministic{}, config.Config{}, log.New(io.Discard, "", 0))
	if err := restarted.AttachCaptureRuntime(ctx, manager); err != nil {
		t.Fatal(err)
	}
	coordinator := collection.NewCoordinator(manager, func(_ context.Context, mode string, _ uint64) (captureruntime.Process, error) {
		process := &runtimeTestProcess{done: make(chan error, 1)}
		if mode == "headless" {
			return &headlessTestProcess{process}, nil
		}
		return process, nil
	}, func() error { return nil })
	coordinator.Request("headless")
	restarted.AttachCollectionCoordinator(coordinator)
	if coordinator.Status().CollectionBorrowSource != domain.SourceFacebook || manager.Snapshot().ActiveLeases != 1 {
		t.Fatalf("restart skipped terminal Facebook cleanup hold: coordinator=%+v owner=%+v", coordinator.Status(), manager.Snapshot())
	}
	t.Cleanup(func() {
		restarted.captureMu.Lock()
		releaseIntent := restarted.collectionIntents[session.ID]
		delete(restarted.collectionIntents, session.ID)
		restarted.captureMu.Unlock()
		if releaseIntent != nil {
			releaseIntent()
		}
		restarted.releaseSessionCaptureLease(session.ID)
		restarted.Shutdown()
	})
}

func TestHybridRecoveryRestoresIntentForActiveFacebookCommand(t *testing.T) {
	ctx := context.Background()
	engine, manager, _, stopOldCoordinator := newHybridCollectionEngine(t, nil)
	settings, err := engine.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.ActiveSources = []domain.Source{domain.SourceFacebook}
	if _, err := engine.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	session, err := engine.StartVisibleUpdate(ctx, "active Facebook recovery")
	if err != nil {
		t.Fatal(err)
	}
	waitHybridCondition(t, "Browser owner for Facebook command", func() bool {
		snapshot := manager.Snapshot()
		return snapshot.State == captureruntime.Ready && snapshot.Driver == "browser" && snapshot.Generation > 1
	})
	if _, err := engine.startNext(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
	active, err := engine.Session(ctx, session.ID)
	if err != nil || active.Runs[0].Status != "waiting_for_bridge" {
		t.Fatalf("Facebook command not durable before restart: session=%+v err=%v", active, err)
	}
	stopOldCoordinator()
	engine.releaseSessionCaptureLease(session.ID)
	engine.captureMu.Lock()
	oldIntent := engine.collectionIntents[session.ID]
	delete(engine.collectionIntents, session.ID)
	engine.captureMu.Unlock()
	if oldIntent != nil {
		oldIntent()
	}

	restarted := New(engine.store, reasoning.Deterministic{}, config.Config{}, log.New(io.Discard, "", 0))
	if err := restarted.AttachCaptureRuntime(ctx, manager); err != nil {
		t.Fatal(err)
	}
	coordinator := collection.NewCoordinator(manager, func(_ context.Context, mode string, _ uint64) (captureruntime.Process, error) {
		process := &runtimeTestProcess{done: make(chan error, 1)}
		if mode == "headless" {
			return &headlessTestProcess{process}, nil
		}
		return process, nil
	}, func() error { return nil })
	coordinator.Request("headless")
	restarted.AttachCollectionCoordinator(coordinator)
	if coordinator.Status().CollectionBorrowSource != domain.SourceFacebook || manager.Snapshot().ActiveLeases != 1 {
		t.Fatalf("restart skipped active Facebook collection hold: coordinator=%+v owner=%+v", coordinator.Status(), manager.Snapshot())
	}
	t.Cleanup(func() {
		restarted.captureMu.Lock()
		releaseIntent := restarted.collectionIntents[session.ID]
		delete(restarted.collectionIntents, session.ID)
		restarted.captureMu.Unlock()
		if releaseIntent != nil {
			releaseIntent()
		}
		restarted.releaseSessionCaptureLease(session.ID)
		restarted.Shutdown()
	})
}

func TestHybridPartialSessionKeepsBrowserLeaseUntilCleanupAcknowledges(t *testing.T) {
	ctx := context.Background()
	engine, manager, coordinator, _ := newHybridCollectionEngine(t, nil)
	session, err := engine.StartVisibleUpdate(ctx, "hybrid partial cleanup")
	if err != nil {
		t.Fatal(err)
	}
	first := session.Runs[0]
	command, err := engine.claimCommandForDriver(ctx, first.ID, "headless-test", "headless")
	if err != nil || command == nil {
		t.Fatalf("headless claim=%+v err=%v", command, err)
	}
	observation := domain.Observation{Source: domain.SourceX, CapturedAt: domain.Now(),
		Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{EvidenceKey: "x:partial-hybrid", Text: "A completed X update"}}}},
		Coverage:  map[string]any{"status": "complete"}}
	if _, err := engine.AcceptObservation(ctx, command.ID, first.ID, observation); err != nil {
		t.Fatal(err)
	}
	waitHybridCondition(t, "Browser owner replacement after completed X reasoning", func() bool {
		snapshot := manager.Snapshot()
		return snapshot.State == captureruntime.Ready && snapshot.Driver == "browser" && snapshot.Generation > 1
	})
	if _, err := engine.startNext(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
	waitHybridCondition(t, "Facebook Browser run after completed X reasoning", func() bool {
		active, loadErr := engine.Session(ctx, session.ID)
		return loadErr == nil && active.Runs[0].Status == "completed" && active.Runs[1].Status == "waiting_for_bridge"
	})
	active, err := engine.Session(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	facebook := active.Runs[1]
	facebookCommand, err := engine.claimCommandForDriver(ctx, facebook.ID, "bridge-test", "browser")
	if err != nil || facebookCommand == nil {
		t.Fatalf("Bridge claim=%+v err=%v", facebookCommand, err)
	}

	cleanupStarted := make(chan struct{}, 1)
	cleanupRelease := make(chan struct{})
	engine.SetBrowserCollectionCleanup(func(cleanupCtx context.Context, _ string, _ uint64) error {
		cleanupStarted <- struct{}{}
		select {
		case <-cleanupRelease:
			return nil
		case <-cleanupCtx.Done():
			return cleanupCtx.Err()
		}
	})
	if _, err := engine.FailCommand(ctx, facebookCommand.ID, facebook.ID, domain.Failure{
		Code: "facebook_capture_fixture_failure", Stage: "capture", Message: "controlled failure", Retryable: false,
	}); err != nil {
		t.Fatal(err)
	}
	partial, err := engine.Session(ctx, session.ID)
	if err != nil || partial.Status != "partial" {
		t.Fatalf("session status=%q err=%v", partial.Status, err)
	}
	select {
	case <-cleanupStarted:
	case <-time.After(time.Second):
		t.Fatal("partial terminal session skipped Browser surface cleanup")
	}
	if manager.Snapshot().ActiveLeases != 1 || coordinator.Status().CollectionBorrowSource != domain.SourceFacebook {
		t.Fatalf("partial session released ownership early: owner=%+v collection=%+v", manager.Snapshot(), coordinator.Status())
	}
	close(cleanupRelease)
	waitCaptureLeases(t, manager, 0)
	waitHybridCondition(t, "partial Facebook collection intent release", func() bool {
		return coordinator.Status().CollectionBorrowSource == ""
	})
}

func waitHybridCondition(t *testing.T, description string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", description)
}
