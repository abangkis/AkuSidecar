//go:build auth_session_fixture

package engine

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/collection"
	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/AkuSidecar/internal/store"
)

type syntheticProfileProcess struct {
	mode           string
	done           chan error
	once           sync.Once
	windows        atomic.Int64
	closed         atomic.Bool
	cleanupFailure atomic.Bool
	owners         *atomic.Int64
}

func (p *syntheticProfileProcess) Driver() string                           { return p.mode }
func (p *syntheticProfileProcess) Done() <-chan error                       { return p.done }
func (p *syntheticProfileProcess) PID() int                                 { return 1 }
func (p *syntheticProfileProcess) OpenExtensionsPage(context.Context) error { return nil }
func (p *syntheticProfileProcess) ReplacementReadiness(context.Context) error {
	if p.windows.Load() != 0 {
		return errors.New("fixture windows retained")
	}
	return nil
}
func (p *syntheticProfileProcess) Terminate() {
	p.once.Do(func() { p.closed.Store(true); p.owners.Add(-1); p.done <- nil })
}
func (p *syntheticProfileProcess) CloseForRetry(ctx context.Context) error {
	if err := p.ReplacementReadiness(ctx); err != nil {
		return err
	}
	if p.cleanupFailure.Load() {
		return errors.New("fixture owned tree not released")
	}
	p.Terminate()
	return nil
}

type syntheticCaptureBackend struct {
	entered chan struct{}
	release chan struct{}
	calls   atomic.Int64
}

func (b *syntheticCaptureBackend) Capture(ctx context.Context, source domain.Source, _ map[string]any) (domain.Observation, error) {
	if b.calls.Add(1) == 1 && b.entered != nil {
		close(b.entered)
		select {
		case <-b.release:
		case <-ctx.Done():
			return domain.Observation{}, ctx.Err()
		}
	}
	return syntheticObservation(source), nil
}
func syntheticObservation(source domain.Source) domain.Observation {
	return domain.Observation{Source: source, PageURL: "http://127.0.0.1/synthetic", CapturedAt: domain.Now(),
		Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{EvidenceKey: string(source) + ":synthetic", PlatformID: string(source) + ":synthetic", Author: "Fixture", Text: "Synthetic fixture product update for local testing."}}}},
		Coverage:  map[string]any{"quality": "complete"}}
}

type syntheticSidecarHarness struct {
	coordinatorDone <-chan struct{}
	e               *Engine
	state           *store.Store
	c               *collection.Coordinator
	m               *captureruntime.Manager
	ctx             context.Context
	cancel          context.CancelFunc
	path            string
	owners          atomic.Int64
	reader          atomic.Pointer[syntheticProfileProcess]
	initial         *syntheticProfileProcess
	backend         *syntheticCaptureBackend
	probe           func(context.Context) (SyntheticSessionEvidence, error)
}

func newSyntheticSidecarHarness(t *testing.T, path string, backend *syntheticCaptureBackend) *syntheticSidecarHarness {
	t.Helper()
	if path == "" {
		path = filepath.Join(t.TempDir(), "synthetic.db")
	}
	settings := domain.DefaultSettings("expanded", "quiet", "guarded_live", true)
	settings.CollectionMode = "headless"
	settings.ActiveSources = []domain.Source{domain.SourceX}
	settings.CalibrationEnabled = false
	state, err := store.Open(path, settings)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = state.CompleteOnboarding(context.Background(), settings.ActiveSources); err != nil {
		t.Fatal(err)
	}
	h := &syntheticSidecarHarness{state: state, path: path, backend: backend}
	h.ctx, h.cancel = context.WithCancel(context.Background())
	h.e = New(state, reasoning.Deterministic{}, config.Config{Dev: true, Root: filepath.Dir(path), Database: config.DatabaseConfig{Path: path}, Capture: config.CaptureConfig{MaxAcquisitionRounds: 1}}, log.New(io.Discard, "", 0))
	h.e.RecordHeartbeat(ExpectedHeartbeat())
	launch := func(mode string) (*syntheticProfileProcess, error) {
		if h.owners.Add(1) != 1 {
			h.owners.Add(-1)
			return nil, errors.New("fixture profile already owned")
		}
		p := &syntheticProfileProcess{mode: mode, done: make(chan error, 1), owners: &h.owners}
		return p, nil
	}
	h.initial, err = launch("headless")
	if err != nil {
		t.Fatal(err)
	}
	h.m, err = captureruntime.New(h.initial)
	if err != nil {
		t.Fatal(err)
	}
	if err = h.e.AttachCaptureRuntime(h.ctx, h.m); err != nil {
		t.Fatal(err)
	}
	h.c = collection.NewCoordinator(h.m, func(_ context.Context, mode string, generation uint64) (captureruntime.Process, error) {
		if mode == "native_reader" {
			p, err := launch("browser")
			if err == nil {
				p.windows.Store(2)
				h.reader.Store(p)
			}
			return p, err
		}
		p, err := launch(mode)
		if err == nil && mode == "headless" {
			h.c.SetSyntheticHeadlessCollector(generation, backend)
		}
		return p, err
	}, func() error { return nil })
	h.c.SetSyntheticHeadlessCollector(1, backend)
	h.e.AttachCollectionCoordinator(h.c)
	h.c.Request("headless")
	t.Cleanup(func() {
		h.cancel()
		h.e.Shutdown()
		done := h.coordinatorDone
		if done == nil {
			done = h.e.developmentCoordinatorDone
		}
		if done != nil {
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Error("synthetic coordinator did not drain before Store cleanup")
			}
		}
		if !h.e.WaitForIdle(3 * time.Second) {
			t.Error("synthetic reasoning did not drain before Store cleanup")
		}
		h.m.Terminate()
		if err := h.state.Close(); err != nil {
			t.Errorf("synthetic Store close: %v", err)
		}
	})
	return h
}
func (h *syntheticSidecarHarness) bind(t *testing.T, probe func(context.Context) (SyntheticSessionEvidence, error)) {
	t.Helper()
	h.probe = probe
	if err := h.e.BindSyntheticExclusive("alpha", probe); err != nil {
		t.Fatal(err)
	}
	h.coordinatorDone = h.c.StartWithDone(h.ctx)
	syntheticWait(t, func() bool { return !h.c.Status().BatchPaused }, "initial verified admission")
}
func syntheticWait(t *testing.T, check func() bool, label string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !check() {
		if time.Now().After(deadline) {
			t.Fatal("timed out: " + label)
		}
		time.Sleep(5 * time.Millisecond)
	}
}
func syntheticProbe(context.Context) (SyntheticSessionEvidence, error) {
	return SyntheticSessionEvidence{Authenticated: true, AccountID: "alpha", Revision: "invented-revision"}, nil
}
func (h *syntheticSidecarHarness) borrow(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(h.ctx, 2*time.Second)
	defer cancel()
	l, r, err := h.e.BorrowNativeReader(ctx)
	if err != nil {
		t.Fatal(err)
	}
	l.Release()
	r()
}
func (h *syntheticSidecarHarness) closeLast(t *testing.T) {
	t.Helper()
	p := h.reader.Load()
	p.windows.Store(0)
	p.Terminate()
	syntheticWait(t, func() bool { return h.c.Status().Effective == "headless" && !h.c.Status().BatchPaused }, "automatic last-reader return")
}

func TestSyntheticExclusiveQueuedBatchAndProfileOwnership(t *testing.T) {
	h := newSyntheticSidecarHarness(t, "", &syntheticCaptureBackend{})
	h.bind(t, syntheticProbe)
	session, err := h.e.StartVisibleUpdate(h.ctx, "synthetic queued batch")
	if err != nil {
		t.Fatal(err)
	}
	run := session.Runs[0]
	payload := map[string]any{"captureRuntime": map[string]any{"driver": "headless", "epoch": h.e.Epoch(), "generation": int(h.m.Snapshot().Generation)}}
	h.borrow(t)
	if !h.c.Status().BatchPaused || h.owners.Load() != 1 || !h.initial.closed.Load() {
		t.Fatal("batch/profile handoff not exclusive")
	}
	if claimed, err := h.state.ClaimCommandForCollector(h.ctx, run.ID, "fixture", "headless"); err != nil || claimed != nil {
		t.Fatal("paused command was admitted", err)
	}
	p := h.reader.Load()
	p.windows.Store(1)
	h.c.Retry()
	time.Sleep(30 * time.Millisecond)
	if p.closed.Load() || h.c.Status().Effective != "browser" || !h.c.Status().BatchPaused {
		t.Fatal("remaining user window lost ownership")
	}
	h.closeLast(t)
	command, err := h.e.claimCommandForCollector(h.ctx, run.ID, "synthetic", "headless", "headless")
	if err != nil || command == nil {
		t.Fatal("resume failed", err)
	}
	if captureGenerationMatches(command.Payload["captureRuntime"].(map[string]any)["generation"], 1) {
		t.Fatal("old generation reused")
	}
	if err := h.e.validateCaptureOwner(session.ID, payload); err == nil {
		t.Fatal("old owner evidence admitted")
	}
	if _, err = h.e.AcceptObservation(h.ctx, command.ID, run.ID, syntheticObservation(domain.SourceX)); err != nil {
		t.Fatal(err)
	}
	if _, err = h.e.AcceptObservation(h.ctx, command.ID, run.ID, syntheticObservation(domain.SourceX)); err == nil {
		t.Fatal("duplicate publication admitted")
	}
	if !h.e.WaitForIdle(2 * time.Second) {
		t.Fatal("reasoning did not drain")
	}
}

func TestSyntheticExclusiveActualCollectorDrainAndAutomaticResume(t *testing.T) {
	backend := &syntheticCaptureBackend{entered: make(chan struct{}), release: make(chan struct{})}
	h := newSyntheticSidecarHarness(t, "", backend)
	h.bind(t, syntheticProbe)
	settings, _ := h.state.GetSettings(h.ctx)
	settings.ActiveSources = []domain.Source{domain.SourceX, domain.SourceInstagram}
	if err := h.state.SaveSettings(h.ctx, settings); err != nil {
		t.Fatal(err)
	}
	session, err := h.e.StartVisibleUpdate(h.ctx, "synthetic collector drain")
	if err != nil {
		t.Fatal(err)
	}
	h.e.StartHeadlessCollection(h.ctx)
	select {
	case <-backend.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("collector did not enter")
	}
	opened := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(h.ctx, 3*time.Second)
		defer cancel()
		l, r, err := h.e.BorrowNativeReader(ctx)
		if err == nil {
			l.Release()
			r()
		}
		opened <- err
	}()
	syntheticWait(t, func() bool { return h.c.Status().BatchPaused }, "reader fences new batch steps")
	if h.initial.closed.Load() || h.reader.Load() != nil {
		t.Fatal("active physical capture lost owner before drain")
	}
	close(backend.release)
	if err := <-opened; err != nil {
		t.Fatal(err)
	}
	if backend.calls.Load() != 1 || !h.initial.closed.Load() {
		t.Fatal("batch not paused between source runs")
	}
	h.closeLast(t)
	finished := waitSession(t, h.e, session.ID, func(s domain.Session) bool {
		return s.Status == "completed" || s.Status == "partial" || s.Status == "failed"
	})
	if backend.calls.Load() != 2 {
		t.Fatalf("wrong capture count: %d", backend.calls.Load())
	}
	for _, r := range finished.Runs {
		if r.Status != "completed" {
			t.Fatal("resumed run did not complete", r.Status)
		}
	}
}

func TestSyntheticExclusiveDurableRestart(t *testing.T) {
	h := newSyntheticSidecarHarness(t, "", &syntheticCaptureBackend{})
	h.bind(t, syntheticProbe)
	session, err := h.e.StartVisibleUpdate(h.ctx, "persist synthetic pause")
	if err != nil {
		t.Fatal(err)
	}
	h.borrow(t)
	h.cancel()
	h.e.Shutdown()
	h.m.Terminate()
	if err = h.state.Close(); err != nil {
		t.Fatal(err)
	}
	next := newSyntheticSidecarHarness(t, h.path, &syntheticCaptureBackend{})
	next.bind(t, syntheticProbe)
	run := session.Runs[0]
	command, err := next.e.claimCommandForCollector(next.ctx, run.ID, "restart", "headless", "headless")
	if err != nil || command == nil {
		t.Fatal("durable queue not resumed", err)
	}
	stamp := command.Payload["captureRuntime"].(map[string]any)
	if stamp["epoch"] != next.e.Epoch() || stamp["epoch"] == h.e.Epoch() {
		t.Fatal("restart retained old epoch")
	}
}

func TestSyntheticExclusiveSyntheticLoginAndLogout(t *testing.T) {
	var loggedIn atomic.Bool
	var logins atomic.Int64
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" && r.Method == "POST" {
			loggedIn.Store(true)
			logins.Add(1)
			http.SetCookie(w, &http.Cookie{Name: "fixture_session", Value: "invented-local-only", Path: "/", HttpOnly: true})
			return
		}
		if r.URL.Path == "/logout" {
			loggedIn.Store(false)
			return
		}
		cookie, err := r.Cookie("fixture_session")
		if err != nil || cookie.Value != "invented-local-only" || !loggedIn.Load() {
			w.WriteHeader(401)
			return
		}
		_ = json.NewEncoder(w).Encode(SyntheticSessionEvidence{Authenticated: true, AccountID: "alpha", Revision: "invented-local-only-revision"})
	}))
	defer fixture.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: time.Second}
	response, err := client.Post(fixture.URL+"/login", "text/plain", nil)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	probe := func(ctx context.Context) (SyntheticSessionEvidence, error) {
		req, _ := http.NewRequestWithContext(ctx, "GET", fixture.URL+"/probe", nil)
		res, err := client.Do(req)
		if err != nil {
			return SyntheticSessionEvidence{}, err
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			return SyntheticSessionEvidence{}, nil
		}
		var v SyntheticSessionEvidence
		err = json.NewDecoder(res.Body).Decode(&v)
		return v, err
	}
	h := newSyntheticSidecarHarness(t, "", &syntheticCaptureBackend{})
	h.bind(t, probe)
	h.borrow(t)
	response, err = client.Post(fixture.URL+"/logout", "text/plain", nil)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	p := h.reader.Load()
	p.windows.Store(0)
	p.Terminate()
	syntheticWait(t, func() bool { return h.c.Status().Failure != "" }, "logout blocks resume")
	if !h.c.Status().BatchPaused || logins.Load() != 1 {
		t.Fatal("logout silently resumed or logged in again")
	}
}

func TestSyntheticExclusiveFailedCleanupRetainsOwnedProfile(t *testing.T) {
	h := newSyntheticSidecarHarness(t, "", &syntheticCaptureBackend{})
	h.bind(t, syntheticProbe)
	h.initial.cleanupFailure.Store(true)
	ctx, cancel := context.WithTimeout(h.ctx, 150*time.Millisecond)
	defer cancel()
	if _, _, err := h.e.BorrowNativeReader(ctx); err == nil {
		t.Fatal("reader admitted after incomplete collector cleanup")
	}
	if h.initial.closed.Load() || h.reader.Load() != nil || h.owners.Load() != 1 || !h.c.Status().BatchPaused {
		t.Fatal("failed cleanup released profile")
	}
}

func TestSyntheticExclusiveCancellationWhilePausedDoesNotResurrect(t *testing.T) {
	h := newSyntheticSidecarHarness(t, "", &syntheticCaptureBackend{})
	h.bind(t, syntheticProbe)
	session, err := h.e.StartVisibleUpdate(h.ctx, "cancel paused synthetic batch")
	if err != nil {
		t.Fatal(err)
	}
	h.borrow(t)
	if err = h.e.CancelSession(h.ctx, session.ID); err != nil {
		t.Fatal(err)
	}
	h.closeLast(t)
	loaded, err := h.state.GetSession(h.ctx, session.ID)
	if err != nil || loaded.Status != "cancelled" {
		t.Fatal("cancelled session resurrected", err)
	}
	if command, err := h.state.ClaimCommandForCollector(h.ctx, session.Runs[0].ID, "fixture", "headless"); err != nil || command != nil {
		t.Fatal("cancelled work admitted", err)
	}
	paused, err := h.state.FixtureCaptureSessionPaused(h.ctx, session.ID)
	if err != nil || paused {
		t.Fatal("cancelled pause marker retained", err)
	}
}

func TestSyntheticExclusiveAccountMismatchLatchesPause(t *testing.T) {
	var account atomic.Value
	account.Store("alpha")
	probe := func(context.Context) (SyntheticSessionEvidence, error) {
		return SyntheticSessionEvidence{Authenticated: true, AccountID: account.Load().(string), Revision: "invented-revision"}, nil
	}
	h := newSyntheticSidecarHarness(t, "", &syntheticCaptureBackend{})
	h.bind(t, probe)
	h.borrow(t)
	account.Store("beta")
	p := h.reader.Load()
	p.windows.Store(0)
	p.Terminate()
	syntheticWait(t, func() bool { return h.c.Status().Failure != "" }, "different account blocks handoff")
	account.Store("alpha")
	h.c.Retry()
	time.Sleep(40 * time.Millisecond)
	if !h.c.Status().BatchPaused || h.c.Status().Effective == "headless" {
		t.Fatal("account mismatch recovered silently")
	}
}

func TestSyntheticExclusiveUnknownReaderInventoryPreservesWindow(t *testing.T) {
	h := newSyntheticSidecarHarness(t, "", &syntheticCaptureBackend{})
	h.bind(t, syntheticProbe)
	h.borrow(t)
	p := h.reader.Load()
	p.windows.Store(-1)
	h.c.Retry()
	time.Sleep(40 * time.Millisecond)
	if p.closed.Load() || !h.c.Status().BatchPaused || h.owners.Load() != 1 {
		t.Fatal("unknown inventory released reader ownership")
	}
}
