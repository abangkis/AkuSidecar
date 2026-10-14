//go:build auth_session_dev && auth_session_fixture

package engine

import (
	"context"
	"errors"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func developmentCapability(h *syntheticSidecarHarness) DevelopmentSessionCapability {
	evidence := func(source domain.Source) DevelopmentSessionEvidence {
		return DevelopmentSessionEvidence{Domain: "synthetic-fixture", Source: source, ProfileID: "scoped-fixture-profile", Authenticated: true, AccountID: "alpha", Revision: "synthetic-revision"}
	}
	return DevelopmentSessionCapability{Provider: "synthetic-fixture", ProfileID: "scoped-fixture-profile", AccountID: "alpha", Sources: []domain.Source{domain.SourceX, domain.SourceInstagram},
		Authority: func(_ context.Context, source domain.Source) (DevelopmentSessionEvidence, error) {
			return evidence(source), nil
		},
		Browser: func(_ context.Context, source domain.Source) (DevelopmentBrowserEvidence, error) {
			s := h.m.Snapshot()
			return DevelopmentBrowserEvidence{DevelopmentSessionEvidence: evidence(source), EngineEpoch: h.e.Epoch(), Generation: s.Generation, PID: h.m.PID(), Driver: s.Driver}, nil
		},
	}
}

func bindDevelopmentHarness(t *testing.T, h *syntheticSidecarHarness, capability DevelopmentSessionCapability) {
	t.Helper()
	var initialized atomic.Bool
	if err := h.e.StartDevelopmentSession(h.ctx, h.m, h.c, capability, func(context.Context) error {
		if h.e.fixtureAdmissionAllowed() || !h.c.Status().DevelopmentSession {
			return errors.New("startup recovery preceded closed binding")
		}
		initialized.Store(true)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !initialized.Load() {
		t.Fatal("startup initializer did not run")
	}
	syntheticWait(t, func() bool { return !h.c.Status().BatchPaused }, "dev startup verified admission")
	if h.c.Status().SyntheticExclusive {
		t.Fatal("dev integration mislabeled as legacy fixture")
	}
}

func TestDevelopmentSessionCapabilityDeniesUnknownAndForeignEvidence(t *testing.T) {
	for _, invalid := range []string{"authority_error", "unknown_auth", "source", "profile", "account", "revision", "domain", "browser_generation", "browser_pid", "browser_epoch", "browser_driver", "browser_revision"} {
		t.Run(invalid, func(t *testing.T) {
			h := newSyntheticSidecarHarness(t, "", &syntheticCaptureBackend{})
			cap := developmentCapability(h)
			authority, browser := cap.Authority, cap.Browser
			cap.Authority = func(ctx context.Context, source domain.Source) (DevelopmentSessionEvidence, error) {
				v, err := authority(ctx, source)
				switch invalid {
				case "authority_error":
					return v, errors.New("unknown")
				case "unknown_auth":
					v.Authenticated = false
				case "source":
					v.Source = domain.SourceFacebook
				case "profile":
					v.ProfileID = "foreign"
				case "account":
					v.AccountID = "beta"
				case "revision":
					v.Revision = ""
				case "domain":
					v.Domain = "real"
				}
				return v, err
			}
			cap.Browser = func(ctx context.Context, source domain.Source) (DevelopmentBrowserEvidence, error) {
				v, err := browser(ctx, source)
				switch invalid {
				case "browser_generation":
					v.Generation++
				case "browser_pid":
					v.PID++
				case "browser_epoch":
					v.EngineEpoch = "foreign"
				case "browser_driver":
					v.Driver = "browser"
				case "browser_revision":
					v.Revision = "foreign"
				}
				return v, err
			}
			if err := h.e.BindDevelopmentSession(h.ctx, cap); err == nil {
				t.Fatal("unverified capability bound")
			}
			if h.e.fixtureAdmissionAllowed() {
				t.Fatal("unverified evidence left admission open")
			}
			if _, err := h.e.claimCommandForCollector(h.ctx, "absent", "test", "headless", "headless"); err != nil {
				t.Fatal("closed admission reached command lookup", err)
			}
		})
	}
}

func TestDevelopmentSessionStartupRejectsBeforeRecovery(t *testing.T) {
	for _, invalid := range []string{"missing_provider", "nondev", "owner_not_ready"} {
		t.Run(invalid, func(t *testing.T) {
			h := newSyntheticSidecarHarness(t, "", &syntheticCaptureBackend{})
			cap := developmentCapability(h)
			switch invalid {
			case "missing_provider":
				cap.Provider = ""
			case "nondev":
				h.e.config.Dev = false
			case "owner_not_ready":
				h.m.Terminate()
			}
			called := false
			if err := h.e.StartDevelopmentSession(h.ctx, h.m, h.c, cap, func(context.Context) error { called = true; return nil }); err == nil {
				t.Fatal("invalid startup accepted")
			}
			if called {
				t.Fatal("startup recovery ran before admission proof")
			}
		})
	}
}

func TestDevelopmentSessionRejectsUnsupportedFrozenWorkBeforeResume(t *testing.T) {
	for _, invalid := range []string{"browser_plan", "facebook_plan"} {
		t.Run(invalid, func(t *testing.T) {
			h := newSyntheticSidecarHarness(t, "", &syntheticCaptureBackend{})
			settings, _ := h.state.GetSettings(h.ctx)
			if invalid == "browser_plan" {
				settings.CollectionMode = "browser"
			} else {
				settings.ActiveSources = []domain.Source{domain.SourceFacebook}
			}
			session, err := h.state.CreateUpdateSession(h.ctx, "unsupported old frozen plan", settings, domain.UpdatePolicy{Trigger: domain.UpdateTriggerUser, Delivery: domain.UpdateDeliveryVisible, BudgetAuthority: domain.BudgetAuthorityUser})
			if err != nil {
				t.Fatal(err)
			}
			if err := h.e.BindDevelopmentSession(h.ctx, developmentCapability(h)); err == nil {
				t.Fatal("unsupported frozen work adopted")
			}
			if h.e.fixtureAdmissionAllowed() {
				t.Fatal("unsupported frozen work left admission open")
			}
			saved, err := h.state.GetSession(h.ctx, session.ID)
			if err != nil || saved.Runs[0].Status != "queued" {
				t.Fatal("unsupported work advanced", err)
			}
		})
	}
}

func TestDevelopmentSessionStartupDefersAcceptedReasoningUntilPausedBinding(t *testing.T) {
	h := newSyntheticSidecarHarness(t, "", &syntheticCaptureBackend{})
	session, err := h.e.StartVisibleUpdate(h.ctx, "durable accepted capture before startup")
	if err != nil {
		t.Fatal(err)
	}
	command, err := h.e.claimCommandForCollector(h.ctx, session.Runs[0].ID, "seed", "headless", "headless")
	if err != nil || command == nil {
		t.Fatal("seed command missing", err)
	}
	if err := h.state.SaveObservation(h.ctx, command.ID, session.Runs[0].ID, syntheticObservation(domain.SourceX)); err != nil {
		t.Fatal(err)
	}
	provider := &hybridGateProvider{started: make(chan struct{}), release: make(chan struct{})}
	h.e.provider = provider
	if err := h.e.StartDevelopmentSession(h.ctx, h.m, h.c, developmentCapability(h), func(context.Context) error {
		if h.e.fixtureAdmissionAllowed() {
			return errors.New("startup admission open")
		}
		select {
		case <-provider.started:
			return errors.New("accepted reasoning resumed before initialization")
		default:
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-provider.started:
	case <-time.After(2 * time.Second):
		t.Fatal("accepted reasoning never resumed")
	}
	if !h.c.Status().BatchPaused {
		t.Fatal("reasoning resume opened admission before drain")
	}
	close(provider.release)
	syntheticWait(t, func() bool { return !h.c.Status().BatchPaused }, "startup reasoning drained")
	if h.backend.calls.Load() != 0 {
		t.Fatal("accepted capture reacquired")
	}
	waitSession(t, h.e, session.ID, func(saved domain.Session) bool {
		return saved.Status == "completed" || saved.Status == "partial" || saved.Status == "failed"
	})
	if !h.e.WaitForIdle(time.Second) {
		t.Fatal("resumed reasoning did not finish before Store cleanup")
	}
}

func TestDevelopmentSessionDrainCaptureAndReasoningBeforeReader(t *testing.T) {
	backend := &syntheticCaptureBackend{entered: make(chan struct{}), release: make(chan struct{})}
	h := newSyntheticSidecarHarness(t, "", backend)
	provider := &hybridGateProvider{started: make(chan struct{}), release: make(chan struct{})}
	h.e.provider = provider
	bindDevelopmentHarness(t, h, developmentCapability(h))
	settings, _ := h.state.GetSettings(h.ctx)
	settings.ActiveSources = []domain.Source{domain.SourceX, domain.SourceInstagram}
	if _, err := h.e.SaveSettings(h.ctx, settings); err != nil {
		t.Fatal(err)
	}
	session, err := h.e.StartVisibleUpdate(h.ctx, "dev active capture and reasoning")
	if err != nil {
		t.Fatal(err)
	}
	h.e.StartHeadlessCollection(h.ctx)
	select {
	case <-backend.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("capture not active")
	}
	opened := make(chan error, 1)
	go func() {
		l, r, err := h.e.BorrowNativeReader(h.ctx)
		if err == nil {
			l.Release()
			r()
		}
		opened <- err
	}()
	syntheticWait(t, func() bool { return h.c.Status().BatchPaused }, "dev reader priority")
	if h.initial.closed.Load() {
		t.Fatal("capture owner closed before capture drain")
	}
	close(backend.release)
	select {
	case <-provider.started:
	case <-time.After(2 * time.Second):
		t.Fatal("reasoning not active")
	}
	if h.initial.closed.Load() || h.reader.Load() != nil {
		t.Fatal("reader replaced owner before reasoning drain")
	}
	select {
	case err := <-opened:
		t.Fatal("reader returned before reasoning drain", err)
	case <-time.After(30 * time.Millisecond):
	}
	close(provider.release)
	if err := <-opened; err != nil {
		t.Fatal(err)
	}
	if backend.calls.Load() != 1 {
		t.Fatal("later capture admitted before reader")
	}
	paused, err := h.state.FixtureCaptureSessionPaused(h.ctx, session.ID)
	if err != nil || !paused {
		t.Fatal("durable pause missing", err)
	}
	h.closeLast(t)
	waitSession(t, h.e, session.ID, func(s domain.Session) bool {
		return s.Status == "completed" || s.Status == "partial" || s.Status == "failed"
	})
	if backend.calls.Load() != 2 {
		t.Fatal("never claimed continuation not resumed")
	}
}

func TestDevelopmentSessionAdmissionAndMismatchLatch(t *testing.T) {
	h := newSyntheticSidecarHarness(t, "", &syntheticCaptureBackend{})
	cap := developmentCapability(h)
	browser := cap.Browser
	var mismatch atomic.Bool
	cap.Browser = func(ctx context.Context, s domain.Source) (DevelopmentBrowserEvidence, error) {
		v, err := browser(ctx, s)
		if mismatch.Load() {
			v.AccountID = "beta"
		}
		return v, err
	}
	bindDevelopmentHarness(t, h, cap)
	settings, _ := h.state.GetSettings(h.ctx)
	settings.CollectionMode = "browser"
	if _, err := h.e.SaveSettings(h.ctx, settings); err == nil {
		t.Fatal("browser settings admitted")
	}
	settings.CollectionMode = "headless"
	settings.ActiveSources = []domain.Source{domain.SourceFacebook}
	if _, err := h.e.SaveSettings(h.ctx, settings); err == nil {
		t.Fatal("unsupported source admitted")
	}
	if _, err := h.e.QueueMediaRecapture(h.ctx, "absent", domain.MediaRecaptureMode("manual")); err == nil {
		t.Fatal("media work admitted")
	}
	if _, err := h.e.claimMediaRecaptureForCollector(h.ctx, "absent", "test", "headless", "headless"); err == nil {
		t.Fatal("media claim admitted")
	}
	if _, _, err := h.e.ApplyPassiveXMediaEvidence(h.ctx, "absent", "test", domain.PassiveXMediaEvidence{}); err == nil {
		t.Fatal("passive media route admitted")
	}
	if _, err := h.e.claimCommandForCollector(h.ctx, "absent", "test", "browser", "bridge"); err == nil {
		t.Fatal("browser claim admitted")
	}
	if _, r, err := h.c.BorrowBrowser(h.ctx); err == nil {
		r()
		t.Fatal("browser collector borrow admitted")
	}
	h.borrow(t)
	mismatch.Store(true)
	p := h.reader.Load()
	p.windows.Store(0)
	p.Terminate()
	syntheticWait(t, func() bool {
		return strings.Contains(h.c.Status().Failure, "session verification required") && h.c.Status().Effective == "headless"
	}, "browser source disagreement")
	mismatch.Store(false)
	h.c.Retry()
	time.Sleep(40 * time.Millisecond)
	if !h.c.Status().BatchPaused {
		t.Fatal("mismatch silently recovered")
	}
}

func TestDevelopmentSessionDurableCancellationAndRestart(t *testing.T) {
	h := newSyntheticSidecarHarness(t, "", &syntheticCaptureBackend{})
	bindDevelopmentHarness(t, h, developmentCapability(h))
	session, err := h.e.StartVisibleUpdate(h.ctx, "dev durable queue")
	if err != nil {
		t.Fatal(err)
	}
	h.borrow(t)
	h.cancel()
	h.e.Shutdown()
	h.m.Terminate()
	if err := h.state.Close(); err != nil {
		t.Fatal(err)
	}
	next := newSyntheticSidecarHarness(t, h.path, &syntheticCaptureBackend{})
	paused, err := next.state.FixtureCaptureSessionPaused(next.ctx, session.ID)
	if err != nil || !paused {
		t.Fatal("reopen lost pause")
	}
	if command, err := next.state.ClaimCommandForCollector(next.ctx, session.Runs[0].ID, "premature", "headless"); err != nil || command != nil {
		t.Fatal("SQLite reopen ignored durable marker", err)
	}
	bindDevelopmentHarness(t, next, developmentCapability(next))
	command, err := next.e.claimCommandForCollector(next.ctx, session.Runs[0].ID, "restart", "headless", "headless")
	if err != nil || command == nil {
		t.Fatal("verified restart did not resume", err)
	}
	if command.Payload["captureRuntime"].(map[string]any)["epoch"] != next.e.Epoch() {
		t.Fatal("restart retained stale epoch")
	}
}

func TestDevelopmentSessionCancellationWhilePausedDoesNotResurrect(t *testing.T) {
	h := newSyntheticSidecarHarness(t, "", &syntheticCaptureBackend{})
	bindDevelopmentHarness(t, h, developmentCapability(h))
	session, err := h.e.StartVisibleUpdate(h.ctx, "dev cancel paused queue")
	if err != nil {
		t.Fatal(err)
	}
	h.borrow(t)
	if err := h.e.CancelSession(h.ctx, session.ID); err != nil {
		t.Fatal(err)
	}
	h.closeLast(t)
	saved, err := h.e.Session(h.ctx, session.ID)
	if err != nil || saved.Status != "cancelled" {
		t.Fatal("cancelled session revived", saved.Status, err)
	}
	if command, _ := h.e.claimCommandForCollector(h.ctx, session.Runs[0].ID, "cancelled", "headless", "headless"); command != nil {
		t.Fatal("cancelled command revived")
	}
	paused, err := h.state.FixtureCaptureSessionPaused(h.ctx, session.ID)
	if err != nil || paused {
		t.Fatal("terminal pause marker retained", err)
	}
	if h.backend.calls.Load() != 0 {
		t.Fatal("cancelled queue recaptured")
	}
}
