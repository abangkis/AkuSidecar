//go:build auth_session_dev && auth_session_fixture

package engine

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/collection"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/store"
)

// SyntheticPolicyPreview is private fixture UI data, never public runtime status.
// The account map is a copy; confirmation accepts only the one-use ID and decision.
type SyntheticPolicyPreview struct {
	ID        string
	Action    string
	Accounts  map[domain.Source]string
	ExpiresAt time.Time
}

type syntheticPolicyObservation struct {
	owner     *captureruntime.Manager
	snapshot  captureruntime.Snapshot
	pid       int
	epoch     string
	revisions map[domain.Source]string
}

type syntheticPolicyPending struct {
	id, action       string
	expires          time.Time
	observed         syntheticPolicyObservation
	checkpoint       store.SyntheticAccountCheckpoint
	policyGeneration uint64
}

// SyntheticPolicyController orchestrates the private Store policy and existing
// Engine verifier. It is compiled only for the dual-tag synthetic adapter. No
// constructor or action is exposed by normal main, settings, HTTP or native IPC.
type SyntheticPolicyController struct {
	mu         sync.Mutex
	e          *Engine
	capability DevelopmentSessionCapability
	expected   map[domain.Source]store.SyntheticAccountIdentity
	enrollment *store.SyntheticEnrollmentFence
	pending    *syntheticPolicyPending
	running    bool
	stopped    bool
	cancel     context.CancelFunc
	lifetime   context.Context
	clock      func() time.Time
}

func NewSyntheticPolicyController(ctx context.Context, e *Engine, capability DevelopmentSessionCapability, fresh bool) (*SyntheticPolicyController, error) {
	if e == nil || !e.config.Dev || e.store == nil || capability.Provider != "synthetic-fixture" ||
		strings.TrimSpace(capability.StoreScopeID) == "" || strings.TrimSpace(capability.ProfileID) == "" ||
		capability.Authority == nil || capability.Browser == nil || len(capability.Sources) == 0 ||
		len(capability.Sources) != len(capability.ExpectedAccounts) || e.fixtureAdmission != nil ||
		e.collectionRuntime != nil && e.collectionRuntime.ExclusiveSessionBound() {
		return nil, errors.New("synthetic policy controller unavailable")
	}
	capability.Sources = append([]domain.Source(nil), capability.Sources...)
	accounts := make(map[domain.Source]string, len(capability.ExpectedAccounts))
	expected := make(map[domain.Source]store.SyntheticAccountIdentity, len(capability.Sources))
	for _, source := range capability.Sources {
		id := capability.ExpectedAccounts[source]
		if !supportedDevelopmentSource(source) || expected[source].AccountID != "" || strings.TrimSpace(id) == "" || id != strings.TrimSpace(id) {
			return nil, errors.New("synthetic policy identities invalid")
		}
		accounts[source] = id
		expected[source] = store.SyntheticAccountIdentity{AccountID: id, StoreScopeID: capability.StoreScopeID, ProfileID: capability.ProfileID}
	}
	capability.ExpectedAccounts = accounts
	settings, err := e.store.GetSettings(ctx)
	if err != nil || settings.CollectionMode != "headless" {
		return nil, errors.New("synthetic policy requires headless settings")
	}
	for _, source := range settings.ActiveSources {
		if _, ok := expected[source]; !ok {
			return nil, errors.New("synthetic policy source unsupported")
		}
	}
	c := &SyntheticPolicyController{e: e, capability: capability, expected: expected, lifetime: ctx, clock: func() time.Time { return time.Now().UTC() }}
	e.fixtureAdmission = &fixtureAdmissionGate{paused: true, developmentHold: true}
	if fresh {
		fence, err := e.store.BeginSyntheticAccountEnrollment(ctx, capability.StoreScopeID, capability.ProfileID)
		if err != nil {
			return nil, errors.New("synthetic enrollment fence unavailable")
		}
		c.enrollment = &fence
	} else {
		cp, _, _, err := e.store.LoadSyntheticAccountPolicy(ctx)
		if err != nil || !syntheticCheckpointExpectedEqual(cp.ExpectedAccounts, expected) {
			return nil, errors.New("synthetic policy scope mismatch or unavailable")
		}
		if err := e.PrepareDevelopmentSessionCheckpoint(ctx, capability, false); err != nil {
			return nil, errors.New("synthetic policy startup fence unavailable")
		}
	}
	return c, nil
}

func (c *SyntheticPolicyController) hold() {
	c.e.fixtureAdmission.mu.Lock()
	c.e.fixtureAdmission.paused = true
	c.e.fixtureAdmission.developmentHold = true
	c.e.fixtureAdmission.pauseEpoch++
	c.e.fixtureAdmission.mu.Unlock()
}

func (c *SyntheticPolicyController) ownerMatches(o syntheticPolicyObservation) bool {
	return o.owner != nil && o.owner == c.e.captureOwner && o.owner.PID() == o.pid && o.owner.Snapshot() == o.snapshot &&
		o.snapshot.State == captureruntime.Ready && o.snapshot.Driver == "headless" && c.e.Epoch() == o.epoch &&
		(c.e.collectionRuntime == nil || !c.e.collectionRuntime.ExclusiveReaderPending())
}

func (c *SyntheticPolicyController) observe(ctx context.Context, owner *captureruntime.Manager) (syntheticPolicyObservation, error) {
	var zero syntheticPolicyObservation
	if ctx.Err() != nil || owner == nil || owner != c.e.captureOwner {
		return zero, errors.New("synthetic policy owner unavailable")
	}
	o := syntheticPolicyObservation{owner: owner, snapshot: owner.Snapshot(), pid: owner.PID(), epoch: c.e.Epoch(), revisions: map[domain.Source]string{}}
	if !c.ownerMatches(o) {
		return zero, errors.New("synthetic policy owner changed")
	}
	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	first := c.clock()
	for _, source := range c.capability.Sources {
		var authority DevelopmentSessionEvidence
		for _, kind := range []DevelopmentSessionProbeKind{DevelopmentSessionProbeAuthority, DevelopmentSessionProbeBrowser} {
			request := DevelopmentSessionProbeRequest{Version: DevelopmentSessionProbeVersion, Source: source, ProfileID: c.capability.ProfileID, Kind: kind, ProbeID: domain.NewID("policyprobe"), RequestedAt: c.clock()}
			var evidence DevelopmentSessionEvidence
			var err error
			if kind == DevelopmentSessionProbeAuthority {
				evidence, err = c.capability.Authority(probeCtx, request)
			} else {
				var browser DevelopmentBrowserEvidence
				browser, err = c.capability.Browser(probeCtx, request)
				evidence = browser.DevelopmentSessionEvidence
				if browser.EngineEpoch != o.epoch || browser.PID != o.pid || browser.Generation != o.snapshot.Generation || browser.Driver != o.snapshot.Driver {
					return zero, errors.New("synthetic policy browser owner proof rejected")
				}
			}
			now := c.clock()
			if err != nil || probeCtx.Err() != nil || !c.ownerMatches(o) || evidence.Version != request.Version || evidence.Source != source ||
				evidence.Kind != kind || evidence.ProbeID != request.ProbeID || evidence.Domain != "synthetic-fixture" || evidence.ProfileID != request.ProfileID ||
				evidence.AuthState != DevelopmentSessionAuthAuthenticated || evidence.AccountID != c.expected[source].AccountID || strings.TrimSpace(evidence.Revision) == "" ||
				evidence.ObservedAt.Before(request.RequestedAt) || evidence.ObservedAt.After(now) || now.Sub(evidence.ObservedAt) > DevelopmentSessionMaxEvidenceAge {
				return zero, errors.New("synthetic policy observation rejected")
			}
			if kind == DevelopmentSessionProbeAuthority {
				authority = evidence
			} else if authority.AccountID != evidence.AccountID || authority.Revision != evidence.Revision {
				return zero, errors.New("synthetic policy session changed")
			}
			o.revisions[source] = evidence.Revision
		}
	}
	if probeCtx.Err() != nil || !c.ownerMatches(o) || c.clock().Sub(first) > DevelopmentSessionMaxEvidenceAge {
		return zero, errors.New("synthetic policy observation expired")
	}
	return o, nil
}

// Preview observes current synthetic identity while capture stays durably closed.
// Enrollment and enable are separate confirmations. A preview never grants work.
func (c *SyntheticPolicyController) Preview(ctx context.Context, owner *captureruntime.Manager, action string) (SyntheticPolicyPreview, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pending = nil
	if c.stopped || c.lifetime.Err() != nil || c.running || action != "enroll" && action != "enable" && action != "migrate" || (action == "enroll") != (c.enrollment != nil) {
		return SyntheticPolicyPreview{}, errors.New("synthetic policy action unavailable")
	}
	c.hold()
	o, err := c.observe(ctx, owner)
	if err != nil {
		return SyntheticPolicyPreview{}, err
	}
	p := &syntheticPolicyPending{id: domain.NewID("policyconfirm"), action: action, expires: c.clock().Add(2 * time.Minute), observed: o}
	if action != "enroll" {
		p.checkpoint, p.policyGeneration, _, err = c.e.store.LoadSyntheticAccountPolicy(ctx)
		if err != nil || !syntheticCheckpointExpectedEqual(p.checkpoint.ExpectedAccounts, c.expected) {
			return SyntheticPolicyPreview{}, errors.New("synthetic policy checkpoint unavailable")
		}
		if action == "migrate" && p.policyGeneration != 0 || action == "enable" && p.policyGeneration == 0 {
			return SyntheticPolicyPreview{}, errors.New("synthetic policy migration required or already applied")
		}
	}
	c.pending = p
	accounts := map[domain.Source]string{}
	for source, id := range c.capability.ExpectedAccounts {
		accounts[source] = id
	}
	return SyntheticPolicyPreview{ID: p.id, Action: p.action, Accounts: accounts, ExpiresAt: p.expires}, nil
}

// Confirm consumes even rejected/expired decisions. A second fresh observation
// must match the preview before enrollment/enable is written. Startup performs
// the existing independent Engine verification again before any queue resumes.
func (c *SyntheticPolicyController) Confirm(ctx context.Context, id string, accepted bool, coordinator *collection.Coordinator) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	p := c.pending
	c.pending = nil
	if p == nil || c.stopped || c.lifetime.Err() != nil || ctx.Err() != nil || !accepted || id != p.id || !c.clock().Before(p.expires) || !c.ownerMatches(p.observed) {
		return errors.New("synthetic policy confirmation rejected")
	}
	o, err := c.observe(ctx, p.observed.owner)
	if err != nil {
		return err
	}
	for source, revision := range p.observed.revisions {
		if o.revisions[source] != revision {
			return errors.New("synthetic policy session changed before confirmation")
		}
	}
	if p.action == "enroll" {
		if c.enrollment == nil {
			return errors.New("synthetic enrollment unavailable")
		}
		if _, err := c.e.store.CompleteSyntheticAccountEnrollment(ctx, *c.enrollment, c.expected); err != nil {
			return errors.New("synthetic enrollment confirmation failed")
		}
		c.enrollment = nil
		return nil
	}
	if p.action == "migrate" {
		if err := c.e.store.ApplySyntheticAccountPolicy(ctx, p.checkpoint, p.policyGeneration, "migrate"); err != nil {
			return errors.New("synthetic policy migration confirmation failed")
		}
		return nil
	}
	if coordinator == nil {
		return errors.New("synthetic policy coordinator required")
	}
	if err := c.e.store.ApplySyntheticAccountPolicy(ctx, p.checkpoint, p.policyGeneration, "enable"); err != nil {
		return errors.New("synthetic opt-in confirmation failed")
	}
	return c.start(ctx, o.owner, coordinator)
}

func (c *SyntheticPolicyController) start(ctx context.Context, owner *captureruntime.Manager, coordinator *collection.Coordinator) error {
	if err := c.e.PrepareDevelopmentSessionCheckpoint(ctx, c.capability, false); err != nil {
		return errors.New("synthetic policy startup fence unavailable")
	}
	runCtx, cancel := context.WithCancel(c.lifetime)
	// Request cancellation governs startup only. Once confirmation completes,
	// polling belongs to the controller lifetime, not the completed UI request.
	stopRequest := context.AfterFunc(ctx, cancel)
	err := c.e.StartDevelopmentSession(runCtx, owner, coordinator, c.capability, func(context.Context) error {
		c.e.fixtureAdmission.mu.Lock()
		c.e.fixtureAdmission.developmentHold = false
		c.e.fixtureAdmission.mu.Unlock()
		return nil
	})
	stoppedRequest := stopRequest()
	if err != nil || !stoppedRequest || ctx.Err() != nil {
		cancel()
		c.hold()
		c.e.developmentCheckpointMu.Lock()
		revoke := c.e.developmentCheckpointRevoke
		c.e.developmentCheckpointMu.Unlock()
		if revoke != nil {
			cleanupCtx, stop := context.WithTimeout(context.Background(), 2*time.Second)
			_ = revoke(cleanupCtx)
			stop()
		}
		c.stopped = true
		return errors.New("synthetic policy startup verification failed")
	}
	c.cancel = cancel
	c.running = true
	return nil
}

// Resume uses persisted opt-in but never a persisted capture permit. It performs
// fresh Engine proof on the new owner, without re-enrollment or another login.
func (c *SyntheticPolicyController) Resume(ctx context.Context, owner *captureruntime.Manager, coordinator *collection.Coordinator) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopped || c.lifetime.Err() != nil || c.running || c.enrollment != nil || coordinator == nil {
		return errors.New("synthetic policy resume unavailable")
	}
	c.pending = nil
	cp, _, enabled, err := c.e.store.LoadSyntheticAccountPolicy(ctx)
	if err != nil || !enabled || !syntheticCheckpointExpectedEqual(cp.ExpectedAccounts, c.expected) {
		return errors.New("synthetic policy opt-in required")
	}
	return c.start(ctx, owner, coordinator)
}

// Disable commits the durable policy first, drains via the existing binding,
// then stops only this controller's owner. Active reader leases cause a closed
// pending shutdown instead of forcing a window closed. A successor uses a new
// controller/owner, never an automatic re-enable of this stopped controller.
func (c *SyntheticPolicyController) Disable(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pending = nil
	c.hold()
	cp, generation, _, err := c.e.store.LoadSyntheticAccountPolicy(ctx)
	if err != nil {
		return errors.New("synthetic policy opt-out unavailable")
	}
	if err := c.e.store.ApplySyntheticAccountPolicy(ctx, cp, generation, "disable"); err != nil {
		return errors.New("synthetic policy opt-out not persisted")
	}
	if !c.running {
		// A preview may already have created a scoped owner even though no
		// collector has started. Stop that owner only after proving it idle.
		if owner := c.e.captureOwner; owner != nil {
			if owner.Snapshot().ActiveLeases != 0 {
				return errors.New("synthetic policy disabled; owner still leased")
			}
			owner.Terminate()
			select {
			case <-owner.Done():
			case <-ctx.Done():
				return errors.New("synthetic policy disabled; owner shutdown incomplete")
			}
		}
		c.stopped = true
		return nil
	}
	if err := c.e.PauseDevelopmentSession(ctx); err != nil {
		return errors.New("synthetic policy disabled; drain incomplete")
	}
	if c.cancel != nil {
		c.cancel()
	}
	if done := c.e.developmentCoordinatorDone; done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			return errors.New("synthetic policy disabled; polling shutdown incomplete")
		}
	}
	if c.e.captureOwner.Snapshot().ActiveLeases != 0 {
		return errors.New("synthetic policy disabled; owner still leased")
	}
	owner := c.e.captureOwner
	owner.Terminate()
	select {
	case <-owner.Done():
	case <-ctx.Done():
		return errors.New("synthetic policy disabled; owner shutdown incomplete")
	}
	c.running = false
	c.stopped = true
	return nil
}
