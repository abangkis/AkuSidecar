//go:build auth_session_dev && auth_session_fixture

package engine

import (
	"context"
	"database/sql"
	"errors"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
)

func developmentCapability(h *syntheticSidecarHarness) DevelopmentSessionCapability {
	evidence := func(request DevelopmentSessionProbeRequest, accountID string) DevelopmentSessionEvidence {
		return DevelopmentSessionEvidence{
			Version: request.Version, Kind: request.Kind, ProbeID: request.ProbeID, ObservedAt: time.Now().UTC(),
			Domain: "synthetic-fixture", Source: request.Source, ProfileID: request.ProfileID,
			AuthState: DevelopmentSessionAuthAuthenticated, AccountID: accountID, Revision: "synthetic-revision",
		}
	}
	return DevelopmentSessionCapability{
		Provider: "synthetic-fixture", ProfileID: "scoped-fixture-profile",
		ExpectedAccounts: map[domain.Source]string{domain.SourceX: "alpha", domain.SourceInstagram: "alpha"},
		Sources:          []domain.Source{domain.SourceX, domain.SourceInstagram},
		Authority: func(_ context.Context, request DevelopmentSessionProbeRequest) (DevelopmentSessionEvidence, error) {
			return evidence(request, "alpha"), nil
		},
		Browser: func(_ context.Context, request DevelopmentSessionProbeRequest) (DevelopmentBrowserEvidence, error) {
			s := h.m.Snapshot()
			return DevelopmentBrowserEvidence{DevelopmentSessionEvidence: evidence(request, "alpha"), EngineEpoch: h.e.Epoch(), Generation: s.Generation, PID: h.m.PID(), Driver: s.Driver}, nil
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
	for _, invalid := range []string{
		"authority_error", "unknown_auth", "logged_out", "challenge", "unsupported_auth",
		"source", "profile", "account", "revision", "domain", "version", "phase", "nonce",
		"zero_observed", "stale_observed", "future_observed", "pre_request_observed",
		"browser_generation", "browser_pid", "browser_epoch", "browser_driver", "browser_revision",
		"browser_version", "browser_phase", "browser_nonce", "browser_zero_observed", "browser_stale_observed", "browser_future_observed", "browser_pre_request_observed",
	} {
		t.Run(invalid, func(t *testing.T) {
			h := newSyntheticSidecarHarness(t, "", &syntheticCaptureBackend{})
			cap := developmentCapability(h)
			authority, browser := cap.Authority, cap.Browser
			cap.Authority = func(ctx context.Context, request DevelopmentSessionProbeRequest) (DevelopmentSessionEvidence, error) {
				v, err := authority(ctx, request)
				switch invalid {
				case "authority_error":
					return v, errors.New("unknown")
				case "unknown_auth":
					v.AuthState = DevelopmentSessionAuthUnknown
				case "logged_out":
					v.AuthState = DevelopmentSessionAuthLoggedOut
				case "challenge":
					v.AuthState = DevelopmentSessionAuthChallenge
				case "unsupported_auth":
					v.AuthState = DevelopmentSessionAuthUnsupported
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
				case "version":
					v.Version++
				case "phase":
					v.Kind = DevelopmentSessionProbeBrowser
				case "nonce":
					v.ProbeID = "foreign-probe"
				case "zero_observed":
					v.ObservedAt = time.Time{}
				case "stale_observed":
					v.ObservedAt = time.Now().Add(-DevelopmentSessionMaxEvidenceAge - time.Millisecond)
				case "future_observed":
					v.ObservedAt = time.Now().Add(time.Minute)
				case "pre_request_observed":
					v.ObservedAt = request.RequestedAt.Add(-time.Nanosecond)
				}
				return v, err
			}
			cap.Browser = func(ctx context.Context, request DevelopmentSessionProbeRequest) (DevelopmentBrowserEvidence, error) {
				v, err := browser(ctx, request)
				switch invalid {
				case "browser_version":
					v.Version++
				case "browser_phase":
					v.Kind = DevelopmentSessionProbeAuthority
				case "browser_nonce":
					v.ProbeID = "foreign-probe"
				case "browser_zero_observed":
					v.ObservedAt = time.Time{}
				case "browser_stale_observed":
					v.ObservedAt = time.Now().Add(-DevelopmentSessionMaxEvidenceAge - time.Millisecond)
				case "browser_future_observed":
					v.ObservedAt = time.Now().Add(time.Minute)
				case "browser_pre_request_observed":
					v.ObservedAt = request.RequestedAt.Add(-time.Nanosecond)
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
			if err := h.e.VerifyDevelopmentReader(h.ctx); err == nil {
				t.Fatal("invalid evidence did not fail closed")
			}
			if _, err := h.e.claimCommandForCollector(h.ctx, "absent", "test", "headless", "headless"); err != nil {
				t.Fatal("closed admission reached command lookup", err)
			}
		})
	}
}

func TestDevelopmentSessionExpectedAccountsAreExactAndSourceScoped(t *testing.T) {
	for _, invalid := range []string{"missing", "extra", "blank", "unsupported_source"} {
		t.Run(invalid, func(t *testing.T) {
			h := newSyntheticSidecarHarness(t, "", &syntheticCaptureBackend{})
			cap := developmentCapability(h)
			switch invalid {
			case "missing":
				delete(cap.ExpectedAccounts, domain.SourceInstagram)
			case "extra":
				cap.ExpectedAccounts[domain.SourceLinkedIn] = "linkedin-account"
			case "blank":
				cap.ExpectedAccounts[domain.SourceX] = "  "
			case "unsupported_source":
				cap.Sources = append(cap.Sources, domain.SourceFacebook)
				cap.ExpectedAccounts[domain.SourceFacebook] = "facebook-account"
			}
			if err := h.e.BindDevelopmentSession(h.ctx, cap); err == nil {
				t.Fatal("inexact expected-account map accepted")
			}
		})
	}

	h := newSyntheticSidecarHarness(t, "", &syntheticCaptureBackend{})
	cap := developmentCapability(h)
	cap.ExpectedAccounts[domain.SourceInstagram] = "instagram-user"
	authority, browser := cap.Authority, cap.Browser
	cap.Authority = func(ctx context.Context, request DevelopmentSessionProbeRequest) (DevelopmentSessionEvidence, error) {
		accountID := cap.ExpectedAccounts[request.Source]
		v, err := authority(ctx, request)
		v.AccountID = accountID
		return v, err
	}
	cap.Browser = func(ctx context.Context, request DevelopmentSessionProbeRequest) (DevelopmentBrowserEvidence, error) {
		accountID := cap.ExpectedAccounts[request.Source]
		v, err := browser(ctx, request)
		v.AccountID = accountID
		return v, err
	}
	bindDevelopmentHarness(t, h, cap)
	h.borrow(t)
	if err := h.e.VerifyDevelopmentReader(h.ctx); err != nil {
		t.Fatalf("distinct source-scoped account IDs were rejected: %v", err)
	}
}

func TestDevelopmentSessionExpectedAccountMapIsDetachedAtBind(t *testing.T) {
	h := newSyntheticSidecarHarness(t, "", &syntheticCaptureBackend{})
	cap := developmentCapability(h)
	authority, browser := cap.Authority, cap.Browser
	var foreign atomic.Bool
	cap.Authority = func(ctx context.Context, request DevelopmentSessionProbeRequest) (DevelopmentSessionEvidence, error) {
		v, err := authority(ctx, request)
		if foreign.Load() {
			v.AccountID = "beta"
		}
		return v, err
	}
	cap.Browser = func(ctx context.Context, request DevelopmentSessionProbeRequest) (DevelopmentBrowserEvidence, error) {
		v, err := browser(ctx, request)
		if foreign.Load() {
			v.AccountID = "beta"
		}
		return v, err
	}
	bindDevelopmentHarness(t, h, cap)
	h.borrow(t)
	if err := h.e.VerifyDevelopmentReader(h.ctx); err != nil {
		t.Fatalf("bound identities did not verify before mutation: %v", err)
	}
	cap.ExpectedAccounts[domain.SourceX] = "beta"
	cap.ExpectedAccounts[domain.SourceInstagram] = "beta"
	foreign.Store(true)
	if err := h.e.VerifyDevelopmentReader(h.ctx); err == nil {
		t.Fatal("post-bind map mutation changed the expected source identities")
	}
}

func TestDevelopmentSessionProbesUseFreshCorrelatedRequests(t *testing.T) {
	h := newSyntheticSidecarHarness(t, "", &syntheticCaptureBackend{})
	cap := developmentCapability(h)
	var mu sync.Mutex
	seen := map[string]bool{}
	check := func(request DevelopmentSessionProbeRequest, kind DevelopmentSessionProbeKind) {
		mu.Lock()
		defer mu.Unlock()
		if request.Version != DevelopmentSessionProbeVersion || request.Kind != kind || request.ProfileID != cap.ProfileID || request.ProbeID == "" || request.RequestedAt.IsZero() || seen[request.ProbeID] {
			t.Errorf("invalid or reused probe request: %+v", request)
		}
		seen[request.ProbeID] = true
	}
	authority, browser := cap.Authority, cap.Browser
	cap.Authority = func(ctx context.Context, request DevelopmentSessionProbeRequest) (DevelopmentSessionEvidence, error) {
		check(request, DevelopmentSessionProbeAuthority)
		return authority(ctx, request)
	}
	cap.Browser = func(ctx context.Context, request DevelopmentSessionProbeRequest) (DevelopmentBrowserEvidence, error) {
		check(request, DevelopmentSessionProbeBrowser)
		return browser(ctx, request)
	}
	bindDevelopmentHarness(t, h, cap)
	mu.Lock()
	requestCount := len(seen)
	mu.Unlock()
	if requestCount < 4 {
		t.Fatalf("expected fresh requests for authority and browser probes, saw %d", requestCount)
	}
}

func TestDevelopmentSessionRejectsEvidenceThatAgesBeforeVectorCompletion(t *testing.T) {
	h := newSyntheticSidecarHarness(t, "", &syntheticCaptureBackend{})
	cap := developmentCapability(h)
	current := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	cap.clock = func() time.Time { return current }
	authority := cap.Authority
	cap.Authority = func(ctx context.Context, request DevelopmentSessionProbeRequest) (DevelopmentSessionEvidence, error) {
		if request.Source == domain.SourceInstagram {
			current = current.Add(DevelopmentSessionMaxEvidenceAge + time.Millisecond)
		}
		v, err := authority(ctx, request)
		v.ObservedAt = current
		return v, err
	}
	if err := h.e.BindDevelopmentSession(h.ctx, cap); err == nil {
		t.Fatal("an earlier source observation remained valid after the full identity vector aged out")
	}
}

func TestDevelopmentSessionBrowserVectorPinsOneOwnerAcrossSources(t *testing.T) {
	h := newSyntheticSidecarHarness(t, "", &syntheticCaptureBackend{})
	cap := developmentCapability(h)
	var armed atomic.Bool
	var firstBrowserResult atomic.Bool
	var browserCalls atomic.Int64
	originalOwner := h.e.captureOwner
	defer func() { h.e.captureOwner = originalOwner }()
	owners := &atomic.Int64{}
	owners.Add(1)
	replacementProcess := &syntheticProfileProcess{mode: "browser", done: make(chan error, 1), owners: owners}
	replacement, err := captureruntime.New(replacementProcess)
	if err != nil {
		t.Fatal(err)
	}
	defer replacement.Terminate()
	cap.clock = func() time.Time {
		if armed.Load() && firstBrowserResult.Swap(false) {
			h.e.captureOwner = replacement
		}
		return time.Now().UTC()
	}
	browser := cap.Browser
	cap.Browser = func(ctx context.Context, request DevelopmentSessionProbeRequest) (DevelopmentBrowserEvidence, error) {
		browserCalls.Add(1)
		v, err := browser(ctx, request)
		activeOwner := h.e.captureOwner
		snapshot := activeOwner.Snapshot()
		v.Generation, v.PID, v.Driver = snapshot.Generation, activeOwner.PID(), snapshot.Driver
		if armed.Load() && request.Source == domain.SourceX {
			firstBrowserResult.Store(true)
		}
		return v, err
	}
	bindDevelopmentHarness(t, h, cap)
	h.borrow(t)
	browserCalls.Store(0)
	armed.Store(true)
	if err := h.e.VerifyDevelopmentReader(h.ctx); err == nil {
		t.Fatal("browser evidence from multiple owner generations was accepted")
	}
	if got := browserCalls.Load(); got != 1 {
		t.Fatalf("replacement owner reached another source probe: browser calls=%d", got)
	}
}

func TestDevelopmentSessionCanceledProbeCannotVerify(t *testing.T) {
	for _, mode := range []string{"before_probe", "during_probe"} {
		t.Run(mode, func(t *testing.T) {
			h := newSyntheticSidecarHarness(t, "", &syntheticCaptureBackend{})
			cap := developmentCapability(h)
			authority := cap.Authority
			var armed atomic.Bool
			var calls atomic.Int64
			ctx, cancel := context.WithCancel(h.ctx)
			defer cancel()
			cap.Authority = func(c context.Context, request DevelopmentSessionProbeRequest) (DevelopmentSessionEvidence, error) {
				if armed.Load() {
					calls.Add(1)
					cancel()
				}
				return authority(c, request)
			}
			bindDevelopmentHarness(t, h, cap)
			h.borrow(t)
			armed.Store(true)
			if mode == "before_probe" {
				cancel()
			}
			if err := h.e.VerifyDevelopmentReader(ctx); err == nil {
				t.Fatal("canceled probe accepted otherwise valid evidence")
			}
			if mode == "before_probe" && calls.Load() != 0 {
				t.Fatal("canceled request reached the adapter")
			}
			if mode == "during_probe" && calls.Load() != 1 {
				t.Fatal("canceled response continued probing other sources")
			}
			if err := h.e.VerifyDevelopmentReader(h.ctx); err == nil {
				t.Fatal("canceled verification silently recovered its failure latch")
			}
		})
	}
}

func TestDevelopmentSessionVerifierErrorsDoNotExposeEvidenceOrBackendText(t *testing.T) {
	h := newSyntheticSidecarHarness(t, "", &syntheticCaptureBackend{})
	cap := developmentCapability(h)
	cap.ProfileID = "C:\\private\\profile\\path"
	cap.Authority = func(_ context.Context, _ DevelopmentSessionProbeRequest) (DevelopmentSessionEvidence, error) {
		return DevelopmentSessionEvidence{AccountID: "private-account", Revision: "private-revision"}, errors.New("private backend response")
	}
	err := h.e.BindDevelopmentSession(h.ctx, cap)
	if err == nil {
		t.Fatal("backend error unexpectedly verified")
	}
	for _, secret := range []string{"private-account", "private-revision", "private backend response", cap.ProfileID} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("verifier error exposed private evidence: %q", err)
		}
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
	pauseProof := make(chan error, 1)
	h.e.fixtureAdmission.mu.Lock()
	h.e.fixtureAdmission.pauseObserver = func(ctx context.Context, id string) error {
		paused, err := h.state.FixtureCaptureSessionPaused(ctx, id)
		if err == nil && (id != session.ID || !paused || h.initial.closed.Load() || h.m.Snapshot().Driver != "headless" || h.m.Snapshot().ActiveLeases < 1) {
			err = errors.New("durable pause observer ran after collector lease release or without SQLite commit")
		}
		pauseProof <- err
		return err
	}
	h.e.fixtureAdmission.mu.Unlock()
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
	if paused, err := h.state.FixtureCaptureSessionPaused(h.ctx, session.ID); err != nil || paused {
		t.Fatal("durable pause before capture drain", paused, err)
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
	if paused, err := h.state.FixtureCaptureSessionPaused(h.ctx, session.ID); err != nil || paused {
		t.Fatal("durable pause before reasoning drain", paused, err)
	}
	select {
	case <-pauseProof:
		t.Fatal("pause observer ran before reasoning drained")
	default:
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
	select {
	case err := <-pauseProof:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("durable pause before lease release not observed")
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
	if !h.e.WaitForIdle(time.Second) {
		t.Fatal("resumed reasoning did not finish before Store cleanup")
	}
}

func TestDevelopmentSessionAdmissionAndMismatchLatch(t *testing.T) {
	h := newSyntheticSidecarHarness(t, "", &syntheticCaptureBackend{})
	cap := developmentCapability(h)
	browser := cap.Browser
	var mismatch atomic.Bool
	cap.Browser = func(ctx context.Context, request DevelopmentSessionProbeRequest) (DevelopmentBrowserEvidence, error) {
		v, err := browser(ctx, request)
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

func TestDevelopmentSessionFailedResumePreservesQueuedCommandAndLatches(t *testing.T) {
	h := newSyntheticSidecarHarness(t, "", &syntheticCaptureBackend{})
	cap := developmentCapability(h)
	browser := cap.Browser
	var mismatch atomic.Bool
	cap.Browser = func(ctx context.Context, request DevelopmentSessionProbeRequest) (DevelopmentBrowserEvidence, error) {
		v, err := browser(ctx, request)
		if mismatch.Load() {
			v.AccountID = "beta"
		}
		return v, err
	}
	bindDevelopmentHarness(t, h, cap)
	h.e.fixtureAdmission.mu.Lock()
	h.e.fixtureAdmission.paused = true
	h.e.fixtureAdmission.mu.Unlock()
	settings, err := h.state.GetSettings(h.ctx)
	if err != nil {
		t.Fatal(err)
	}
	session, err := h.state.CreateUpdateSession(h.ctx, "development failed resume queued row", settings, domain.UpdatePolicy{
		Trigger: domain.UpdateTriggerUser, Delivery: domain.UpdateDeliveryVisible, BudgetAuthority: domain.BudgetAuthorityUser,
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := h.state.AdvanceSession(h.ctx, session.ID)
	if err != nil || run == nil {
		t.Fatal("failed to create queued capture run", err)
	}
	payload := capturePayload(*run, session.ID, settings, 1, nil, "")
	payload["captureRuntime"] = map[string]any{"driver": "headless", "epoch": h.e.Epoch(), "generation": int(h.m.Snapshot().Generation)}
	payload["captureCollector"] = collectorStamp("headless")
	command, err := h.state.StartRun(h.ctx, run.ID, payload)
	if err != nil {
		t.Fatal("failed to persist queued command", err)
	}
	if err := h.state.PauseFixtureCaptureSession(h.ctx, session.ID); err != nil {
		t.Fatal("failed to persist original SQLite pause", err)
	}
	db, err := sql.Open("sqlite", h.path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	type commandRow struct {
		status    string
		payload   string
		claimedAt sql.NullString
		claimedBy sql.NullString
	}
	readCommand := func() commandRow {
		t.Helper()
		var row commandRow
		if err := db.QueryRowContext(h.ctx, `SELECT status,payload_json,claimed_at,claimed_by FROM bridge_commands WHERE id=?`, command.ID).
			Scan(&row.status, &row.payload, &row.claimedAt, &row.claimedBy); err != nil {
			t.Fatal(err)
		}
		return row
	}
	before := readCommand()
	if before.status != "queued" || before.claimedAt.Valid || before.claimedBy.Valid {
		t.Fatalf("original command was not never-claimed and queued: %+v", before)
	}
	h.borrow(t)
	mismatch.Store(true)
	reader := h.reader.Load()
	reader.windows.Store(0)
	reader.Terminate()
	syntheticWait(t, func() bool {
		return strings.Contains(h.c.Status().Failure, "session verification required") && h.c.Status().Effective == "headless"
	}, "failed browser identity resume")
	if after := readCommand(); after != before {
		t.Fatalf("failed proof changed the original queued command: before=%+v after=%+v", before, after)
	}
	paused, err := h.state.FixtureCaptureSessionPaused(h.ctx, session.ID)
	if err != nil || !paused {
		t.Fatalf("failed proof cleared the original SQLite pause: paused=%t err=%v", paused, err)
	}
	saved, err := h.state.GetSession(h.ctx, session.ID)
	if err != nil || saved.Status != "running" || saved.Runs[0].Status != "waiting_for_bridge" {
		t.Fatalf("failed proof changed the session status: status=%s run=%s err=%v", saved.Status, saved.Runs[0].Status, err)
	}
	mismatch.Store(false)
	h.c.Retry()
	time.Sleep(40 * time.Millisecond)
	if !h.c.Status().BatchPaused {
		t.Fatal("corrected proof automatically recovered the latched failure")
	}
	if after := readCommand(); after != before {
		t.Fatalf("retry changed the original queued command: before=%+v after=%+v", before, after)
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
