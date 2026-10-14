//go:build auth_session_dev

package engine

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/collection"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

// DevelopmentSessionEvidence is private verifier output, not bridge readiness.
// The only supported domain is synthetic-fixture. Real source adapters are absent.
type DevelopmentSessionEvidence struct {
	Domain        string
	Source        domain.Source
	ProfileID     string
	Authenticated bool
	AccountID     string
	Revision      string
}

type DevelopmentBrowserEvidence struct {
	DevelopmentSessionEvidence
	EngineEpoch string
	Generation  uint64
	PID         int
	Driver      string
}

type DevelopmentSessionCapability struct {
	Provider  string
	ProfileID string
	AccountID string
	Sources   []domain.Source
	Authority func(context.Context, domain.Source) (DevelopmentSessionEvidence, error)
	Browser   func(context.Context, domain.Source) (DevelopmentBrowserEvidence, error)
}

// BindDevelopmentSession requires both source authority and current browser proof.
// It must precede startup resume and collection workers on a scoped development Store.
func (e *Engine) BindDevelopmentSession(ctx context.Context, capability DevelopmentSessionCapability) error {
	if !e.config.Dev || capability.Provider != "synthetic-fixture" || strings.TrimSpace(capability.ProfileID) == "" || strings.TrimSpace(capability.AccountID) == "" || len(capability.Sources) == 0 || capability.Authority == nil || capability.Browser == nil {
		return errors.New("development session capability unavailable")
	}
	sources := append([]domain.Source(nil), capability.Sources...)
	allowed := map[domain.Source]bool{}
	for _, source := range sources {
		if (source != domain.SourceX && source != domain.SourceInstagram && source != domain.SourceLinkedIn) || allowed[source] {
			return errors.New("development session source unsupported")
		}
		allowed[source] = true
	}
	settings, err := e.store.GetSettings(ctx)
	if err != nil {
		return err
	}
	if settings.CollectionMode != "headless" {
		return errors.New("development session requires headless collection")
	}
	for _, source := range settings.ActiveSources {
		if !allowed[source] {
			return errors.New("development session source unsupported")
		}
	}
	matches := func(v DevelopmentSessionEvidence, source domain.Source) bool {
		return v.Domain == "synthetic-fixture" && v.Source == source && v.ProfileID == capability.ProfileID && v.Authenticated && v.AccountID == capability.AccountID && strings.TrimSpace(v.Revision) != ""
	}
	probe := func(probeCtx context.Context, browser bool) (sessionEvidence, error) {
		revisions := make([]string, 0, len(sources))
		for _, source := range sources {
			var v DevelopmentSessionEvidence
			if browser {
				if e.captureOwner == nil {
					return sessionEvidence{}, errors.New("development browser owner unavailable")
				}
				before, pid := e.captureOwner.Snapshot(), e.captureOwner.PID()
				proof, probeErr := capability.Browser(probeCtx, source)
				after := e.captureOwner.Snapshot()
				if probeErr != nil || before.State != captureruntime.Ready || before != after || pid <= 0 || e.captureOwner.PID() != pid || proof.EngineEpoch != e.Epoch() || proof.Generation != before.Generation || proof.PID != pid || proof.Driver != before.Driver {
					return sessionEvidence{}, errors.New("development browser owner proof required")
				}
				v = proof.DevelopmentSessionEvidence
			} else {
				authority, probeErr := capability.Authority(probeCtx, source)
				if probeErr != nil {
					return sessionEvidence{}, errors.New("development source authority unavailable")
				}
				v = authority
			}
			if !matches(v, source) {
				return sessionEvidence{}, errors.New("development source identity proof required")
			}
			revisions = append(revisions, v.Revision)
		}
		raw, _ := json.Marshal(revisions)
		return sessionEvidence{Authenticated: true, AccountID: capability.AccountID, Revision: string(raw)}, nil
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
		return err
	}
	active, err := e.store.ActiveSession(ctx)
	if err != nil {
		return err
	}
	if active != nil {
		ids = append(ids, active.ID)
	}
	for _, id := range ids {
		saved, err := e.store.GetSession(ctx, id)
		if err != nil {
			return err
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
	if err := e.bindExclusiveSession(capability.AccountID, func(c context.Context) (sessionEvidence, error) { return probe(c, false) }, func(c context.Context) (sessionEvidence, error) { return probe(c, true) }, true); err != nil {
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
