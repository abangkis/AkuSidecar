package engine

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/collection"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
)

type runtimeTestProcess struct {
	done chan error
	once sync.Once
}
type headlessTestProcess struct{ *runtimeTestProcess }

func (p *headlessTestProcess) Driver() string { return "headless" }

func TestFreshBrowserHeartbeatRevokesRetainedHeadlessAuthority(t *testing.T) {
	e, _ := testEngine(t)
	if len(e.headlessAccess) != 2 {
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
	if len(e.headlessAccess) != 1 || e.headlessAccess[0] != "facebook" {
		t.Fatal("revoked X remained authorized", e.headlessAccess)
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
	e.ResetCaptureHeartbeat()
	if err := m.Replace(ctx, func(context.Context, uint64) (captureruntime.Process, error) {
		return &headlessTestProcess{&runtimeTestProcess{done: make(chan error, 1)}}, nil
	}); err != nil {
		t.Fatal(err)
	}
	c.Request("headless")
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
	settings, _ := e.Settings(ctx)
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
