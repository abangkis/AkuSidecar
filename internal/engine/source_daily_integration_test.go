//go:build auth_session_dev

package engine

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/store"
)

type instagramOfflineTransport struct {
	mu                            sync.Mutex
	engine                        *Engine
	account, revision             string
	derivations, captures, closes int
	mutate                        func(*instagramSyntheticEvidence, bool)
	cleanupFailure                bool
	wrongObservation              bool
	hold                          <-chan struct{}
	started                       chan struct{}
}

func TestSourceDailySwitchBackReconcilesOnlyClosedDurableFence(t *testing.T) {
	for _, mode := range []string{"closed", "live", "ordinary"} {
		t.Run(mode, func(t *testing.T) {
			e, _ := testEngine(t)
			ctx := context.Background()
			settings, _ := e.store.GetSettings(ctx)
			settings.ActiveSources = []domain.Source{domain.SourceInstagram}
			settings.CollectionMode = "headless"
			session, err := e.store.CreateUpdateSession(ctx, "switch back fixture", settings, domain.UpdatePolicy{Trigger: domain.UpdateTriggerUser, Delivery: domain.UpdateDeliveryVisible, BudgetAuthority: domain.BudgetAuthorityUser})
			if err != nil {
				t.Fatal(err)
			}
			run := session.Runs[0]
			command, err := e.store.StartRun(ctx, run.ID, map[string]any{"captureRuntime": map[string]any{"driver": "headless"}})
			if err != nil {
				t.Fatal(err)
			}
			if mode != "ordinary" {
				now := e.store.Now()
				binding := store.DevelopmentManualPilotBinding{Digest: strings.Repeat("a", 64), Attempt: "synthetic:restart", OwnerGeneration: 1, StartedAt: now, Deadline: now.Add(time.Minute)}
				permit, err := e.store.NewDevelopmentSourceDailyPilotIssuer(domain.SourceInstagram, func(context.Context) (store.DevelopmentManualPilotBinding, error) { return binding, nil })(ctx, session.ID, run.ID)
				if err != nil {
					t.Fatal(err)
				}
				if mode == "closed" {
					if _, err = e.store.RevokeManualPilot(ctx, permit, "close-before-switch"); err != nil {
						t.Fatal(err)
					}
				}
			}
			e.config.Capture.InstagramMode = "browser"
			err = e.ReconcileXDailyStartup(ctx)
			if mode == "live" {
				if err == nil {
					t.Fatal("unclosed protected work silently resumed")
				}
				if claim, err := e.store.ClaimCommandForCollector(ctx, command.RunID, "ordinary", "headless"); claim != nil || err == nil {
					t.Fatal("route switch bypassed durable IG fence")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got, err := e.store.GetRun(ctx, run.ID)
			if err != nil {
				t.Fatal(err)
			}
			want := "failed"
			if mode == "ordinary" {
				want = "waiting_for_bridge"
			}
			if got.Status != want {
				t.Fatalf("switch recovery status=%s want=%s", got.Status, want)
			}
		})
	}
}

func TestSourceDailyPredecessorWaitsForOwnedCollectorCleanup(t *testing.T) {
	for _, source := range dailySources {
		t.Run(string(source), func(t *testing.T) {
			e, _ := testEngine(t)
			e.config.Capture.XMode = config.XCaptureConcurrent
			e.config.Capture.InstagramMode = config.XCaptureConcurrent
			ctx := context.Background()
			other := domain.SourceInstagram
			if source == other {
				other = domain.SourceX
			}
			settings, _ := e.store.GetSettings(ctx)
			settings.ActiveSources = []domain.Source{source, other}
			session, err := e.store.CreateDailySourceUpdateSession(ctx, "cleanup predecessor fixture", settings, domain.UpdatePolicy{Trigger: domain.UpdateTriggerUser, Delivery: domain.UpdateDeliveryVisible, BudgetAuthority: domain.BudgetAuthorityUser}, []domain.Source{source, other})
			if err != nil {
				t.Fatal(err)
			}
			if err = e.store.FailRun(ctx, session.Runs[0].ID, domain.Failure{Code: "synthetic", Stage: "capture"}); err != nil {
				t.Fatal(err)
			}
			session, _ = e.store.GetSession(ctx, session.ID)
			d := e.dailyController(source)
			d.state.runID = session.Runs[0].ID
			d.state.done = make(chan struct{})
			if drained, err := e.hybridPredecessorsDrained(ctx, session, session.Runs[1]); err != nil || drained {
				t.Fatal("collector ownership did not fence successor")
			}
			close(d.state.done)
			d.state.done = nil
			d.state.cleanupUncertain = true
			if drained, err := e.hybridPredecessorsDrained(ctx, session, session.Runs[1]); err != nil || drained {
				t.Fatal("cleanup uncertainty admitted successor")
			}
			d.state.cleanupUncertain = false
			if drained, err := e.hybridPredecessorsDrained(ctx, session, session.Runs[1]); err != nil || !drained {
				t.Fatal("verified retirement did not release successor")
			}
		})
	}
}

func (f *instagramOfflineTransport) evidence(c instagramSyntheticChallenge, collector bool) instagramSyntheticEvidence {
	f.mu.Lock()
	defer f.mu.Unlock()
	v := instagramSyntheticEvidence{Domain: "synthetic-fixture", Nonce: c.Nonce, ProfileScope: c.ProfileScope, Source: domain.SourceInstagram, Generation: c.Generation, Account: f.account, Revision: f.revision, Authenticated: true, ObservedAt: f.engine.store.Now()}
	v.Role, v.OwnerProfileID = "reader", c.ReaderProfileID
	if collector {
		v.Role, v.OwnerProfileID = "collector", c.CollectorProfileID
	}
	if f.mutate != nil {
		f.mutate(&v, collector)
	}
	return v
}
func (f *instagramOfflineTransport) ReaderEvidence(ctx context.Context, c instagramSyntheticChallenge) (instagramSyntheticEvidence, error) {
	return f.evidence(c, false), ctx.Err()
}
func (f *instagramOfflineTransport) Derive(ctx context.Context, c instagramSyntheticChallenge, v instagramSyntheticEvidence, end time.Time) (instagramSyntheticCollector, error) {
	if v.Domain != "synthetic-fixture" || !end.After(f.engine.store.Now()) || end.After(f.engine.store.Now().Add(time.Minute)) {
		return nil, errors.New("invalid fixture derivation")
	}
	f.mu.Lock()
	f.derivations++
	f.mu.Unlock()
	return &instagramOfflineCollector{f: f}, ctx.Err()
}

type instagramOfflineCollector struct{ f *instagramOfflineTransport }

func (c *instagramOfflineCollector) Evidence(ctx context.Context, q instagramSyntheticChallenge) (instagramSyntheticEvidence, error) {
	return c.f.evidence(q, true), ctx.Err()
}
func (c *instagramOfflineCollector) Capture(ctx context.Context, payload map[string]any) (domain.Observation, error) {
	f := c.f
	f.mu.Lock()
	f.captures++
	wrong, hold, started := f.wrongObservation, f.hold, f.started
	f.mu.Unlock()
	if started != nil {
		close(started)
	}
	if hold != nil {
		select {
		case <-hold:
		case <-ctx.Done():
			return domain.Observation{}, ctx.Err()
		}
	}
	source := domain.SourceInstagram
	if wrong {
		source = domain.SourceX
	}
	return domain.Observation{Source: source, CapturedAt: domain.Now(), Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{EvidenceKey: "instagram:synthetic", Text: "Synthetic Instagram shared-core evidence"}}}}}, nil
}
func (c *instagramOfflineCollector) Close(context.Context) error {
	c.f.mu.Lock()
	defer c.f.mu.Unlock()
	c.f.closes++
	if c.f.cleanupFailure {
		return errors.New("synthetic target absence unverified")
	}
	return nil
}

func attachInstagramOffline(t *testing.T, e *Engine) *instagramOfflineTransport {
	t.Helper()
	e.config.Dev = true
	e.config.Capture.InstagramMode = config.XCaptureConcurrent
	f := &instagramOfflineTransport{engine: e, account: "synthetic:alpha", revision: "synthetic:revision-1"}
	if err := e.attachInstagramSynthetic(context.Background(), strings.Repeat("b", 64), f); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := e.closeInstagramDaily(ctx); err != nil && !f.cleanupFailure {
			t.Error(err)
		}
		if !f.cleanupFailure && !e.WaitForIdle(time.Second) {
			t.Error("source workers not drained")
		}
	})
	return f
}

func TestInstagramOnlyRoutePreservesOrdinaryX(t *testing.T) {
	e, _ := testEngine(t)
	e.config.Capture.InstagramMode = config.XCaptureConcurrent
	ctx := context.Background()
	release, allowed := e.ordinarySourceRoute(domain.SourceX)
	release()
	if !allowed {
		t.Fatal("Instagram selection reserved ordinary X")
	}
	settings, _ := e.store.GetSettings(ctx)
	settings.ActiveSources = []domain.Source{domain.SourceX, domain.SourceInstagram}
	settings.CollectionMode = "headless"
	session, err := e.store.CreateUpdateSession(ctx, "source selection fixture", settings, domain.UpdatePolicy{Trigger: domain.UpdateTriggerUser, Delivery: domain.UpdateDeliveryVisible, BudgetAuthority: domain.BudgetAuthorityUser})
	if err != nil {
		t.Fatal(err)
	}
	for _, run := range session.Runs {
		if _, err = e.store.StartRun(ctx, run.ID, map[string]any{"captureRuntime": map[string]any{"driver": "headless"}}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := e.ordinaryPendingRun(ctx, "headless")
	if err != nil || got != session.Runs[0].ID {
		t.Fatalf("ordinary X excluded: %s %v", got, err)
	}
	if _, allowed = e.ordinarySourceRoute(domain.SourceInstagram); allowed {
		t.Fatal("unqualified concurrent IG fell through to ordinary route")
	}
}

func TestInstagramDailyCleanupFailureRemainsUnverified(t *testing.T) {
	e, _ := testEngine(t)
	f := attachInstagramOffline(t, e)
	f.cleanupFailure = true
	ctx := context.Background()
	settings, _ := e.store.GetSettings(ctx)
	settings.ActiveSources = []domain.Source{domain.SourceInstagram}
	settings.CalibrationEnabled = false
	settings.AIDetectionEnabled = false
	if err := e.store.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	if err := e.StartInstagramDaily(ctx, XDailyStartRequest{true, true}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.StartVisibleUpdate(ctx, "cleanup uncertainty fixture"); err != nil {
		t.Fatal(err)
	}
	end := time.Now().Add(2 * time.Second)
	for time.Now().Before(end) {
		if e.SourceCaptureStatus(domain.SourceInstagram).Cleanup == "uncertain" {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	status := e.SourceCaptureStatus(domain.SourceInstagram)
	if status.Cleanup != "uncertain" || status.DerivationAuthorized {
		t.Fatalf("cleanup uncertainty lost: %+v", status)
	}
	if e.WaitForIdle(time.Millisecond) {
		t.Fatal("IG cleanup uncertainty reported idle")
	}
	if err := e.closeInstagramDaily(ctx); err == nil {
		t.Fatal("owner close erased unverified target cleanup")
	}
}
func awaitSourceBatch(t *testing.T, e *Engine, id string) {
	t.Helper()
	end := time.Now().Add(4 * time.Second)
	for time.Now().Before(end) {
		s, err := e.store.GetSession(context.Background(), id)
		busy := false
		for _, source := range dailySources {
			d := e.dailyController(source)
			d.state.mu.RLock()
			busy = busy || d.state.done != nil
			d.state.mu.RUnlock()
		}
		if err == nil && (s.Status == "completed" || s.Status == "failed") && !busy {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	s, _ := e.store.GetSession(context.Background(), id)
	t.Fatalf("source batch did not finish: %+v X=%+v IG=%+v", s, e.XCaptureStatus(), e.SourceCaptureStatus(domain.SourceInstagram))
}
func TestSourceDailyMixedSessionUsesSharedCoreAndIsolatedReceipts(t *testing.T) {
	for _, order := range [][]domain.Source{{domain.SourceX, domain.SourceInstagram}, {domain.SourceInstagram, domain.SourceX}} {
		t.Run(string(order[0]), func(t *testing.T) {
			e, x := dailyRuntimeFixture(t)
			f := attachInstagramOffline(t, e)
			ctx := context.Background()
			// X's saved grant cannot start IG, including on the same application.
			if err := e.ResumeXDailyScheduling(ctx); err != nil {
				t.Fatal(err)
			}
			if e.dailySourceReady(domain.SourceInstagram) || f.derivations != 0 {
				t.Fatal("X permission authorized Instagram")
			}
			if err := e.StartInstagramDaily(ctx, XDailyStartRequest{true, true}); err != nil {
				t.Fatal(err)
			}
			settings, _ := e.store.GetSettings(ctx)
			settings.ActiveSources = order
			if err := e.store.SaveSettings(ctx, settings); err != nil {
				t.Fatal(err)
			}
			session, err := e.StartVisibleUpdate(ctx, "mixed source fixture")
			if err != nil {
				t.Fatal(err)
			}
			awaitSourceBatch(t, e, session.ID)
			session, err = e.store.GetSession(ctx, session.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(session.Runs) != 2 {
				t.Fatalf("mixed run count=%d", len(session.Runs))
			}
			for _, run := range session.Runs {
				observations, err := e.store.Observations(ctx, run.ID)
				if err != nil || len(observations) != 1 || observations[0].Source != run.Source {
					t.Fatalf("source observations %s: %+v %v", run.Source, observations, err)
				}
				valid, err := e.store.IsDailySourceCapture(ctx, run.ID)
				if err != nil || !valid {
					t.Fatalf("durable source receipt lost %s: %v %v", run.Source, valid, err)
				}
				xOnly, err := e.store.IsDailyXCapture(ctx, run.ID)
				if err != nil || xOnly != (run.Source == domain.SourceX) {
					t.Fatal("X receipt API admitted IG")
				}
			}
			f.mu.Lock()
			if f.derivations != 1 || f.captures != 1 || f.closes != 1 {
				t.Fatalf("IG lifecycle: derive=%d capture=%d close=%d", f.derivations, f.captures, f.closes)
			}
			f.mu.Unlock()
			x.mu.Lock()
			if x.readerCloses != 0 || x.collectorCloses != 1 {
				t.Fatal("X reader/collector lifetime changed")
			}
			x.mu.Unlock()
			if err = e.RevokeInstagramDailyPermission(ctx); err != nil {
				t.Fatal(err)
			}
			if !e.xDailyReady() || e.dailySourceReady(domain.SourceInstagram) {
				t.Fatal("IG revoke crossed source boundary")
			}
		})
	}
}
func TestInstagramDailyRejectsUnqualifiedNativeAndInvalidPairedProof(t *testing.T) {
	e, _ := testEngine(t)
	e.config.Dev = true
	e.config.Capture.InstagramMode = config.XCaptureConcurrent
	if err := e.StartInstagramDaily(context.Background(), XDailyStartRequest{true, true}); err == nil {
		t.Fatal("native IG selected without qualified adapter")
	}
	for _, name := range []string{"native-domain", "wrong-source", "wrong-profile", "wrong-account", "stale", "logged-out", "revision-mismatch", "reader-as-collector", "nonce-replay", "wrong-generation"} {
		t.Run(name, func(t *testing.T) {
			e, _ := dailyRuntimeFixture(t)
			f := attachInstagramOffline(t, e)
			ctx := context.Background()
			if err := e.StartInstagramDaily(ctx, XDailyStartRequest{true, true}); err != nil {
				t.Fatal(err)
			}
			f.mutate = func(v *instagramSyntheticEvidence, collector bool) {
				if !collector {
					return
				}
				switch name {
				case "reader-as-collector":
					v.Role = "reader"
				case "nonce-replay":
					v.Nonce = "synthetic:stale-nonce"
				case "wrong-generation":
					v.Generation++
				case "native-domain":
					v.Domain = "native"
				case "wrong-source":
					v.Source = domain.SourceX
				case "wrong-profile":
					v.ProfileScope = strings.Repeat("a", 64)
				case "wrong-account":
					v.Account = "synthetic:other"
				case "stale":
					v.ObservedAt = v.ObservedAt.Add(-nativePilotAge - time.Second)
				case "logged-out":
					v.Authenticated = false
				case "revision-mismatch":
					v.Revision = "synthetic:changed"
				}
			}
			settings, _ := e.store.GetSettings(ctx)
			settings.ActiveSources = []domain.Source{domain.SourceInstagram}
			if err := e.store.SaveSettings(ctx, settings); err != nil {
				t.Fatal(err)
			}
			session, err := e.StartVisibleUpdate(ctx, "invalid proof fixture")
			if err != nil {
				t.Fatal(err)
			}
			awaitSourceBatch(t, e, session.ID)
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.captures != 0 || f.closes != 1 {
				t.Fatalf("bad proof captured or orphaned collector: %d %d", f.captures, f.closes)
			}
			if e.SourceCaptureStatus(domain.SourceInstagram).DerivationAuthorized {
				t.Fatal("failed proof retained authority")
			}
		})
	}
}
func TestInstagramDailyStopCancelsCaptureAndPreservesOtherSource(t *testing.T) {
	e, _ := dailyRuntimeFixture(t)
	f := attachInstagramOffline(t, e)
	f.hold = make(chan struct{})
	f.started = make(chan struct{})
	ctx := context.Background()
	if err := e.StartInstagramDaily(ctx, XDailyStartRequest{true, true}); err != nil {
		t.Fatal(err)
	}
	settings, _ := e.store.GetSettings(ctx)
	settings.ActiveSources = []domain.Source{domain.SourceInstagram}
	if err := e.store.SaveSettings(ctx, settings); err != nil {
		t.Fatal(err)
	}
	if _, err := e.StartVisibleUpdate(ctx, "cancel fixture"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-f.started:
	case <-time.After(2 * time.Second):
		t.Fatal("IG capture not started")
	}
	if err := e.RevokeInstagramDailyPermission(ctx); err != nil {
		t.Fatal(err)
	}
	if !e.xDailyReady() {
		t.Fatal("IG stop revoked X")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closes != 1 {
		t.Fatal("collector not closed")
	}
}
