//go:build auth_session_dev

package engine

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/collection"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

const (
	// DevelopmentSessionProbeVersion versions the synthetic verifier request and response contract.
	DevelopmentSessionProbeVersion = 1
	// DevelopmentSessionMaxEvidenceAge bounds how long one source observation may be reused.
	DevelopmentSessionMaxEvidenceAge = 5 * time.Second
)

type DevelopmentSessionProbeKind string

const (
	DevelopmentSessionProbeAuthority DevelopmentSessionProbeKind = "authority"
	DevelopmentSessionProbeBrowser   DevelopmentSessionProbeKind = "browser"
)

type DevelopmentSessionAuthState string

const (
	DevelopmentSessionAuthAuthenticated DevelopmentSessionAuthState = "authenticated"
	DevelopmentSessionAuthLoggedOut     DevelopmentSessionAuthState = "logged_out"
	DevelopmentSessionAuthChallenge     DevelopmentSessionAuthState = "challenge"
	DevelopmentSessionAuthUnknown       DevelopmentSessionAuthState = "unknown"
	DevelopmentSessionAuthUnsupported   DevelopmentSessionAuthState = "unsupported"
)

// DevelopmentSessionProbeRequest is a one-use, phase-specific challenge for
// the synthetic verifier. Authority is synthetic control evidence that may be
// available before a browser exists; it does not prove a real browser login.
type DevelopmentSessionProbeRequest struct {
	Version     int
	Source      domain.Source
	ProfileID   string
	Kind        DevelopmentSessionProbeKind
	ProbeID     string
	RequestedAt time.Time
}

// DevelopmentSessionEvidence is private verifier output, not bridge readiness.
// The only supported domain is synthetic-fixture. Real source adapters are absent.
type DevelopmentSessionEvidence struct {
	Version    int
	Kind       DevelopmentSessionProbeKind
	ProbeID    string
	ObservedAt time.Time
	Domain     string
	Source     domain.Source
	ProfileID  string
	AuthState  DevelopmentSessionAuthState
	AccountID  string
	Revision   string
}

type DevelopmentBrowserEvidence struct {
	DevelopmentSessionEvidence
	EngineEpoch string
	Generation  uint64
	PID         int
	Driver      string
}

type DevelopmentSessionCapability struct {
	Provider         string
	ProfileID        string
	ExpectedAccounts map[domain.Source]string
	Sources          []domain.Source
	Authority        func(context.Context, DevelopmentSessionProbeRequest) (DevelopmentSessionEvidence, error)
	Browser          func(context.Context, DevelopmentSessionProbeRequest) (DevelopmentBrowserEvidence, error)
	clock            func() time.Time
}

// BindDevelopmentSession requires both source authority and current browser proof.
// It must precede startup resume and collection workers on a scoped development Store.
func (e *Engine) BindDevelopmentSession(ctx context.Context, capability DevelopmentSessionCapability) error {
	if !e.config.Dev || capability.Provider != "synthetic-fixture" || strings.TrimSpace(capability.ProfileID) == "" || len(capability.Sources) == 0 || capability.Authority == nil || capability.Browser == nil {
		return errors.New("development session capability unavailable")
	}
	sources := append([]domain.Source(nil), capability.Sources...)
	expectedAccounts := make(map[domain.Source]string, len(capability.ExpectedAccounts))
	for source, accountID := range capability.ExpectedAccounts {
		expectedAccounts[source] = accountID
	}
	allowed := map[domain.Source]bool{}
	for _, source := range sources {
		if (source != domain.SourceX && source != domain.SourceInstagram && source != domain.SourceLinkedIn) || allowed[source] {
			return errors.New("development session source unsupported")
		}
		allowed[source] = true
	}
	if len(expectedAccounts) != len(allowed) {
		return errors.New("development session expected identities incomplete")
	}
	for source, accountID := range expectedAccounts {
		if !allowed[source] || strings.TrimSpace(accountID) == "" {
			return errors.New("development session expected identities unsupported")
		}
	}
	settings, err := e.store.GetSettings(ctx)
	if err != nil {
		return errors.New("development session settings unavailable")
	}
	if settings.CollectionMode != "headless" {
		return errors.New("development session requires headless collection")
	}
	for _, source := range settings.ActiveSources {
		if !allowed[source] {
			return errors.New("development session source unsupported")
		}
	}
	identitySources := append([]domain.Source(nil), sources...)
	sort.Slice(identitySources, func(i, j int) bool { return identitySources[i] < identitySources[j] })
	identities := make([][2]string, 0, len(identitySources))
	for _, source := range identitySources {
		identities = append(identities, [2]string{string(source), expectedAccounts[source]})
	}
	expectedIdentity, err := json.Marshal(identities)
	if err != nil {
		return errors.New("development session expected identities unavailable")
	}
	now := func() time.Time {
		if capability.clock != nil {
			return capability.clock().UTC()
		}
		return time.Now().UTC()
	}
	newRequest := func(source domain.Source, kind DevelopmentSessionProbeKind) DevelopmentSessionProbeRequest {
		return DevelopmentSessionProbeRequest{
			Version: DevelopmentSessionProbeVersion, Source: source, ProfileID: capability.ProfileID,
			Kind: kind, ProbeID: domain.NewID("devprobe"), RequestedAt: now(),
		}
	}
	matches := func(v DevelopmentSessionEvidence, request DevelopmentSessionProbeRequest, now time.Time) bool {
		return request.Version == DevelopmentSessionProbeVersion && request.ProbeID != "" &&
			v.Version == request.Version && v.Kind == request.Kind && v.ProbeID == request.ProbeID &&
			!v.ObservedAt.IsZero() && !v.ObservedAt.Before(request.RequestedAt) && !v.ObservedAt.After(now) &&
			now.Sub(v.ObservedAt) <= DevelopmentSessionMaxEvidenceAge &&
			v.Domain == "synthetic-fixture" && v.Source == request.Source && v.ProfileID == request.ProfileID &&
			v.AuthState == DevelopmentSessionAuthAuthenticated && v.AccountID == expectedAccounts[request.Source] &&
			strings.TrimSpace(v.Revision) != ""
	}
	probe := func(probeCtx context.Context, browser bool) (sessionEvidence, error) {
		if probeCtx.Err() != nil {
			return sessionEvidence{}, errors.New("development source probe canceled")
		}
		var owner *captureruntime.Manager
		var ownerSnapshot captureruntime.Snapshot
		var ownerPID int
		var ownerEpoch string
		if browser {
			owner = e.captureOwner
			if owner == nil {
				return sessionEvidence{}, errors.New("development browser owner unavailable")
			}
			ownerSnapshot, ownerPID, ownerEpoch = owner.Snapshot(), owner.PID(), e.Epoch()
			if ownerSnapshot.State != captureruntime.Ready || ownerPID <= 0 {
				return sessionEvidence{}, errors.New("development browser owner proof required")
			}
		}
		revisions := make([][2]string, 0, len(sources))
		observations := make([]struct {
			request  DevelopmentSessionProbeRequest
			evidence DevelopmentSessionEvidence
		}, 0, len(sources))
		for _, source := range sources {
			kind := DevelopmentSessionProbeAuthority
			if browser {
				kind = DevelopmentSessionProbeBrowser
			}
			request := newRequest(source, kind)
			var v DevelopmentSessionEvidence
			if browser {
				before, pid := owner.Snapshot(), owner.PID()
				if e.captureOwner != owner || before != ownerSnapshot || pid != ownerPID {
					return sessionEvidence{}, errors.New("development browser owner proof required")
				}
				proof, probeErr := capability.Browser(probeCtx, request)
				after := owner.Snapshot()
				if probeErr != nil || before.State != captureruntime.Ready || before != after || after != ownerSnapshot || pid != ownerPID || e.captureOwner != owner || owner.PID() != ownerPID || proof.EngineEpoch != ownerEpoch || proof.Generation != ownerSnapshot.Generation || proof.PID != ownerPID || proof.Driver != ownerSnapshot.Driver {
					return sessionEvidence{}, errors.New("development browser owner proof required")
				}
				v = proof.DevelopmentSessionEvidence
			} else {
				authority, probeErr := capability.Authority(probeCtx, request)
				if probeErr != nil {
					return sessionEvidence{}, errors.New("development source authority unavailable")
				}
				v = authority
			}
			if probeCtx.Err() != nil {
				return sessionEvidence{}, errors.New("development source probe canceled")
			}
			if !matches(v, request, now()) {
				return sessionEvidence{}, errors.New("development source identity proof required")
			}
			revisions = append(revisions, [2]string{string(source), v.Revision})
			observations = append(observations, struct {
				request  DevelopmentSessionProbeRequest
				evidence DevelopmentSessionEvidence
			}{request: request, evidence: v})
		}
		finalObservedAt := now()
		if probeCtx.Err() != nil {
			return sessionEvidence{}, errors.New("development source probe canceled")
		}
		if browser && (e.captureOwner != owner || owner.Snapshot() != ownerSnapshot || owner.PID() != ownerPID || e.Epoch() != ownerEpoch) {
			return sessionEvidence{}, errors.New("development browser owner changed during evidence collection")
		}
		for _, observation := range observations {
			if !matches(observation.evidence, observation.request, finalObservedAt) {
				return sessionEvidence{}, errors.New("development source evidence expired")
			}
		}
		raw, _ := json.Marshal(revisions)
		return sessionEvidence{Authenticated: true, AccountID: string(expectedIdentity), Revision: string(raw)}, nil
	}
	if e.captureOwner == nil || e.collectionRuntime == nil || e.collectionRuntime.ExclusiveSessionBound() {
		return errors.New("development session startup binding unavailable")
	}
	e.developmentSources = allowed
	e.collectionRuntime.SetDevelopmentSession(true, sources...)
	if e.fixtureAdmission == nil {
		e.fixtureAdmission = &fixtureAdmissionGate{paused: true}
	}
	// Saved settings do not override an older frozen session plan. Reject an
	// unsupported durable route before resume can advance or create its command.
	ids, err := e.store.ListFixturePausedCaptureSessions(ctx)
	if err != nil {
		return errors.New("development session queue unavailable")
	}
	active, err := e.store.ActiveSession(ctx)
	if err != nil {
		return errors.New("development session state unavailable")
	}
	if active != nil {
		ids = append(ids, active.ID)
	}
	for _, id := range ids {
		saved, err := e.store.GetSession(ctx, id)
		if err != nil {
			return errors.New("development session state unavailable")
		}
		if saved.Status != "queued" && saved.Status != "running" {
			continue
		}
		if !hybridCollectionSession(saved) {
			return errors.New("development session cannot adopt non-headless work")
		}
		for _, run := range saved.Runs {
			driver, ok := hybridDriverForRun(saved, run.Source)
			if !allowed[run.Source] || !ok || driver != "headless" {
				return errors.New("development session cannot adopt unsupported source route")
			}
		}
	}
	if err := e.bindExclusiveSession(string(expectedIdentity), func(c context.Context) (sessionEvidence, error) { return probe(c, false) }, func(c context.Context) (sessionEvidence, error) { return probe(c, true) }, true); err != nil {
		return err
	}
	return nil
}

// StartDevelopmentSession is the shared integrated startup boundary. All recovery
// callbacks execute only after binding a paused queue. The scoped owner must be
// ready before this call; initialize configures that verified owner and does not
// create it. No callback may log in. Only accepted-capture reasoning is resumed.
func (e *Engine) StartDevelopmentSession(ctx context.Context, owner *captureruntime.Manager, coordinator *collection.Coordinator, capability DevelopmentSessionCapability, initialize func(context.Context) error) error {
	if owner == nil || coordinator == nil {
		return errors.New("development startup ownership unavailable")
	}
	if e.captureOwner == nil {
		if err := e.AttachCaptureRuntime(ctx, owner); err != nil {
			return err
		}
	} else if e.captureOwner != owner {
		return errors.New("development startup owner changed")
	}
	e.AttachCollectionCoordinator(coordinator)
	if err := e.BindDevelopmentSession(ctx, capability); err != nil {
		return err
	}
	if initialize != nil {
		if err := initialize(ctx); err != nil {
			return err
		}
	}
	if _, err := e.ResumePendingReasoning(ctx); err != nil {
		return err
	}
	coordinator.Request("headless")
	coordinator.Start(ctx)
	return nil
}

// PauseDevelopmentSession prepares a bound, idle headless development runtime for
// a lifecycle test. It grants no reader/foreground capability and has no HTTP route.
func (e *Engine) PauseDevelopmentSession(ctx context.Context) error {
	if e.developmentPause == nil || e.captureOwner == nil || e.collectionRuntime == nil || e.collectionRuntime.ExclusiveReaderPending() {
		return errors.New("development pause unavailable")
	}
	snapshot := e.captureOwner.Snapshot()
	if snapshot.State != captureruntime.Ready || snapshot.Driver != "headless" {
		return errors.New("development pause requires ready headless owner")
	}
	// Keep this explicit lifecycle hold closed through coordinator polling. It
	// grants no window intent; a later process requires a fresh verified binding.
	e.fixtureAdmission.mu.Lock()
	e.fixtureAdmission.developmentHold = true
	e.fixtureAdmission.paused = true
	e.fixtureAdmission.pauseEpoch++
	e.fixtureAdmission.mu.Unlock()
	return e.developmentPause(ctx)
}

func (e *Engine) VerifyDevelopmentReader(ctx context.Context) error {
	if e.developmentVerifyBrowser == nil || e.captureOwner == nil || e.captureOwner.Snapshot().State != captureruntime.Ready || e.captureOwner.Snapshot().Driver != "browser" {
		return errors.New("development reader verification unavailable")
	}
	return e.developmentVerifyBrowser(ctx)
}
