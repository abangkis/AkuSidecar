//go:build auth_session_dev && auth_session_fixture

package engine

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sync"
	"time"
)

// pilotConcurrentFixture is an offline fixture boundary, never a native proof.
type pilotConcurrentFixture struct {
	ExplicitOptIn, LimitationAcknowledged, PolicyEnabled bool
	ExpectedAccountID, ScopeID, PolicyID                 string
	PolicyGeneration, ConsentGeneration                  uint64
	Grant                                                pilotFixtureGrant
	Source                                               pilotEvidenceSource
}

type pilotConcurrentAdapter struct {
	mu               sync.Mutex
	fixture          pilotConcurrentFixture
	clock            func() time.Time
	readerProfile    string
	model            *concurrentPilot
	binding          concurrentAuthorizationBinding
	started, revoked bool
	tickets          map[string]pilotWorkTicket
	byWork           map[string]concurrentAuthorizationTicket
}

func newPilotConcurrentController(parent context.Context, cfg concurrentConfig, ports concurrentPorts, f pilotConcurrentFixture) (*concurrentController, error) {
	if !cfg.Enabled {
		return newConcurrentController(parent, cfg, ports)
	}
	if !f.ExplicitOptIn || !f.LimitationAcknowledged || !f.PolicyEnabled || f.PolicyID != concurrentPilotPolicy || f.ExpectedAccountID != cfg.ExpectedAccountID || !isSyntheticID(f.ExpectedAccountID) || !syntheticPilotID(f.ScopeID) || f.PolicyGeneration == 0 || f.ConsentGeneration == 0 || !validPilotGrant(f.Grant) || f.Source == nil {
		return nil, pilotErr(pilotReasonInvalidConfig)
	}
	if cfg.now == nil {
		cfg.now = time.Now
	}
	a := &pilotConcurrentAdapter{fixture: f, clock: cfg.now, readerProfile: cfg.ReaderProfileID, tickets: make(map[string]pilotWorkTicket), byWork: make(map[string]concurrentAuthorizationTicket)}
	return newConcurrentControllerWithAuthorization(parent, cfg, ports, a)
}

func (a *pilotConcurrentAdapter) Start(ctx context.Context, b concurrentAuthorizationBinding) error {
	a.mu.Lock()
	if a.started || a.revoked {
		a.mu.Unlock()
		return pilotErr(pilotReasonConcurrentInvalid)
	}
	a.started = true
	a.mu.Unlock()
	// Reuse only the fixture binding validator; no strict authority or native receipt is borrowed.
	validator := authorizedConcurrentAdapter{clock: a.clock, readerProfile: a.readerProfile, fixture: authorizedConcurrentFixture{AccountID: a.fixture.ExpectedAccountID}}
	if _, err := validator.authorizationConfig(b); err != nil {
		a.Revoke("owner-drift")
		return err
	}
	f := a.fixture
	pair := pilotPair{id: pilotBridgeID("pilot-pair", fmt.Sprintf("%s:%s:%d:%d", b.ControllerID, b.AttemptID, b.Epoch, b.PairGeneration)), attemptID: pilotBridgeID("attempt", b.AttemptID), controllerEpochID: pilotBridgeID("epoch", fmt.Sprint(b.Epoch)), sidecarLifetimeID: pilotBridgeID("sidecar", b.ParentID), generation: b.PairGeneration}
	cfg := pilotConfig{mode: concurrentPilotMode, contractID: concurrentPilotContract, providerID: concurrentPilotProvider, evidenceSchema: concurrentPilotSchema, policyID: f.PolicyID, policyEnabled: f.PolicyEnabled, explicitOptIn: f.ExplicitOptIn, limitationAcknowledged: f.LimitationAcknowledged, expectedAccountID: pilotBridgeID("account", f.ExpectedAccountID), scopeID: f.ScopeID, policyGeneration: f.PolicyGeneration, consentGeneration: f.ConsentGeneration, pair: pair}
	if len(b.ReaderWindows) == 0 {
		b.ReaderWindows = []string{"synthetic:initial-reader-inventory:" + b.Reader.ID}
	}
	cfg.reader = mapPilotConcurrentOwner(b.Reader, pilotReaderRole, pair, a.clock(), pilotBridgeWindows(b.ReaderWindows))
	cfg.collector = mapPilotConcurrentOwner(b.Collector, pilotCollectorRole, pair, a.clock(), []string{pilotBridgeID("collector-inventory", b.Collector.ID)})
	m, err := newConcurrentPilot(cfg, f.Grant, a.clock, f.Source)
	if err != nil {
		return err
	}
	a.mu.Lock()
	if a.revoked {
		a.mu.Unlock()
		m.optOut()
		return pilotErr(pilotReasonOptedOut)
	}
	a.model = m
	a.binding = cloneAuthorizationBinding(b)
	a.mu.Unlock()
	receipt := pilotSeedReceipt{instanceID: m.instanceID, id: pilotBridgeID("pilot-seed", pair.id), contractID: cfg.contractID, providerID: cfg.providerID, evidenceSchema: cfg.evidenceSchema, policyID: cfg.policyID, policyGeneration: cfg.policyGeneration, consentGeneration: cfg.consentGeneration, accountID: cfg.expectedAccountID, pairID: pair.id, attemptID: pair.attemptID, pairGeneration: pair.generation, readerOwnerID: cfg.reader.ownerID, collectorOwnerID: cfg.collector.ownerID, fromRole: pilotReaderRole, toRole: pilotCollectorRole, manifestID: concurrentPilotManifest, seedApplied: true, seededAt: a.clock()}
	return m.enable(ctx, receipt)
}

// All positive identity flags describe these validated synthetic fixture owners only.
func mapPilotConcurrentOwner(o concurrentOwner, r pilotRole, p pilotPair, now time.Time, windows []string) pilotOwner {
	id := pilotBridgeID("pilot-owner", o.ID)
	return pilotOwner{role: r, ownerID: id, profileID: pilotBridgeID("profile", o.ProfileID), directoryID: pilotBridgeID("directory", o.DirectoryID), processID: pilotBridgeID("pid", fmt.Sprint(o.PID)), processCreationID: pilotBridgeID("creation", o.CreationIdentity), processImageID: pilotBridgeID("image", o.Driver), processStartedAt: now, targetID: pilotBridgeID("fixture-target", id), frameID: pilotBridgeID("fixture-frame", id), documentID: pilotBridgeID("fixture-document", id), sidecarLifetimeID: p.sidecarLifetimeID, contextID: pilotBridgeID("fixture-context", o.DirectoryID), epochID: p.controllerEpochID, generation: o.Generation, driverID: pilotBridgeID("driver", o.Driver), directoryPinned: true, profileFresh: r == pilotCollectorRole, disposableContext: r == pilotCollectorRole, processAlive: true, windowInventory: windows}
}
func (a *pilotConcurrentAdapter) current() (*concurrentPilot, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.model == nil || a.revoked {
		return nil, pilotErr(pilotReasonDisabled)
	}
	return a.model, nil
}
func (a *pilotConcurrentAdapter) Check() error {
	m, e := a.current()
	if e != nil {
		return e
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.checkCurrentLocked()
}
func (a *pilotConcurrentAdapter) RenewEvidence(ctx context.Context, b concurrentAuthorizationBinding, validate func(context.Context) error) error {
	a.mu.Lock()
	same := sameAuthorizationBinding(a.binding, b)
	a.mu.Unlock()
	m, e := a.current()
	if e != nil {
		return e
	}
	if !same {
		a.Revoke("owner-drift")
		return pilotErr(pilotReasonOwnerDrift)
	}
	return m.renewVectorWithHook(ctx, false, func() {
		if validate == nil || validate(ctx) != nil {
			m.mu.Lock()
			m.revokeLocked(pilotReasonConcurrentInvalid)
			m.mu.Unlock()
		}
	})
}
func (a *pilotConcurrentAdapter) Ticket(work string) (concurrentAuthorizationTicket, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.revoked || a.model == nil {
		return concurrentAuthorizationTicket{}, pilotErr(pilotReasonDisabled)
	}
	if t, ok := a.byWork[work]; ok {
		return t, nil
	}
	t, e := a.model.dispatch(pilotBridgeID("work", work))
	if e != nil {
		return concurrentAuthorizationTicket{}, e
	}
	out := concurrentAuthorizationTicket{ID: t.id, WorkID: work, EvidenceVersion: t.evidenceVersion}
	a.tickets[t.id] = t
	a.byWork[work] = out
	return out, nil
}
func (a *pilotConcurrentAdapter) resolve(t concurrentAuthorizationTicket) (*concurrentPilot, pilotWorkTicket, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	v, ok := a.tickets[t.ID]
	if a.revoked || a.model == nil || !ok || v.evidenceVersion != t.EvidenceVersion || v.commandID != pilotBridgeID("work", t.WorkID) {
		return nil, pilotWorkTicket{}, pilotErr(pilotReasonStaleTicket)
	}
	return a.model, v, nil
}
func (a *pilotConcurrentAdapter) Claim(t concurrentAuthorizationTicket, commit func() error) error {
	m, v, e := a.resolve(t)
	if e != nil {
		return e
	}
	return m.claimWithCommit(v, commit)
}
func (a *pilotConcurrentAdapter) Cancel(t concurrentAuthorizationTicket, commit func() error) error {
	m, v, e := a.resolve(t)
	if e != nil {
		return e
	}
	return m.cancelWithCommit(v, commit)
}
func (a *pilotConcurrentAdapter) Complete(t concurrentAuthorizationTicket, content string, commit func() error) error {
	m, v, e := a.resolve(t)
	if e != nil {
		return e
	}
	r := pilotResult{id: pilotBridgeID("result", v.id), ticketID: v.id, commandID: v.commandID, scopeID: v.scopeID, sourceID: v.sourceID, operation: v.operation, evidenceVersion: v.evidenceVersion, pairID: v.pairID, pairGeneration: v.pairGeneration, fence: v.fence, collectorOwner: v.collectorOwnerID, content: content}
	return m.acceptResultWithCommit(v, r, commit)
}
func (a *pilotConcurrentAdapter) Revoke(reason string) {
	a.mu.Lock()
	a.revoked = true
	m := a.model
	a.mu.Unlock()
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	r := pilotReason(reason)
	if !validPilotReason(r) {
		r = pilotReasonChannelLost
	}
	m.revokeLocked(r)
}
func (a *pilotConcurrentAdapter) PolicyChanged(p, c uint64, enabled bool) {
	m, e := a.current()
	if e == nil {
		m.policyChanged(p, c, enabled)
	}
}
func (a *pilotConcurrentAdapter) OwnerChanged(r concurrentRole) {
	a.Revoke(string(pilotReasonOwnerDrift))
}
func (a *pilotConcurrentAdapter) UpdateReaderWindows(windows []string) error {
	m, e := a.current()
	if e != nil {
		return e
	}
	mapped := pilotBridgeWindows(windows)
	m.mu.Lock()
	defer m.mu.Unlock()
	if e = m.checkCurrentLocked(); e != nil {
		return e
	}
	if len(mapped) == 0 {
		m.revokeLocked(pilotReasonReaderLost)
		return pilotErr(pilotReasonReaderLost)
	}
	seen := map[string]bool{}
	for _, w := range mapped {
		if seen[w] {
			return pilotErr(pilotReasonInvalidConfig)
		}
		seen[w] = true
	}
	// The controller owns the complete inventory; synchronize the model set without
	// invoking window transition methods while its lock is held.
	m.windowOpen = make(map[string]struct{}, len(mapped))
	for _, w := range mapped {
		m.windowOpen[w] = struct{}{}
	}
	m.reader.windowInventory = mapped
	return nil
}
func (a *pilotConcurrentAdapter) RequestIndependentCollector() error {
	return pilotErr(pilotReasonUnsupported)
}

func pilotBridgeID(label, value string) string {
	return fmt.Sprintf("synthetic-%s-%x", label, sha256.Sum256([]byte(value)))
}
func pilotBridgeWindows(windows []string) []string {
	out := make([]string, len(windows))
	for i, w := range windows {
		out[i] = pilotBridgeID("window", w)
	}
	return out
}
