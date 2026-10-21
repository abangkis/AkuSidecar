//go:build auth_session_dev && auth_session_fixture

package engine

// This bridge composes the synthetic concurrent controller with the private
// fixture authorization model. It is not registered as a provider and does
// not qualify, inspect, or transfer a native session.

import (
	"context"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

type authorizedConcurrentFixture struct {
	DatabaseScope     string
	AccountID         string
	ReaderRevision    string
	CollectorRevision string
	PolicyGeneration  uint64
	HoldGeneration    uint64
	PolicyEnabled     bool
	Grant             fixtureGrant
	Collect           evidenceCollector
	LifecycleBarrier  syntheticLifecycleBarrier
}

type authorizedConcurrentAdapter struct {
	mu sync.Mutex

	clock           func() time.Time
	fixture         authorizedConcurrentFixture
	readerProfile   string
	authority       *concurrentAuthorization
	started         bool
	starting        bool
	revoked         bool
	baseBinding     concurrentAuthorizationBinding
	ticketsByWork   map[string]concurrentAuthorizationTicket
	authorityTicket map[string]workTicket
}

// newAuthorizedConcurrentController selects the new private authority path.
// Disabled construction remains side-effect-free and delegates to the legacy
// default-off constructor. Enabled construction requires explicit fixture
// grants and a synthetic evidence collector.
func newAuthorizedConcurrentController(parent context.Context, cfg concurrentConfig, ports concurrentPorts, fixture authorizedConcurrentFixture) (*concurrentController, error) {
	if !cfg.Enabled {
		return newConcurrentController(parent, cfg, ports)
	}
	clock := cfg.now
	if clock == nil {
		clock = time.Now
		cfg.now = clock
	}
	if fixture.AccountID != cfg.ExpectedAccountID || !isSyntheticID(fixture.DatabaseScope) || !isSyntheticID(fixture.AccountID) ||
		!isSyntheticID(fixture.ReaderRevision) || !isSyntheticID(fixture.CollectorRevision) || fixture.PolicyGeneration == 0 || fixture.HoldGeneration == 0 ||
		fixture.Collect == nil || !validFixtureGrant(fixture.Grant) {
		return nil, errConcurrentUnavailable
	}
	adapter := &authorizedConcurrentAdapter{
		clock:           clock,
		fixture:         fixture,
		readerProfile:   cfg.ReaderProfileID,
		ticketsByWork:   make(map[string]concurrentAuthorizationTicket),
		authorityTicket: make(map[string]workTicket),
	}
	return newConcurrentControllerWithAuthorization(parent, cfg, ports, adapter)
}

func (a *authorizedConcurrentAdapter) Start(ctx context.Context, binding concurrentAuthorizationBinding) error {
	a.mu.Lock()
	if a.started || a.starting || a.revoked {
		a.mu.Unlock()
		return authError(reasonAlreadyEstablished)
	}
	a.starting = true
	a.mu.Unlock()

	config, err := a.authorizationConfig(binding)
	if err != nil {
		a.mu.Lock()
		a.starting = false
		a.mu.Unlock()
		return err
	}
	authority, err := newConcurrentAuthorization(config, a.fixture.Grant, a.clock)
	if err != nil {
		a.mu.Lock()
		a.starting = false
		a.mu.Unlock()
		return err
	}
	receipt := derivationReceipt{
		instanceID:             authority.instanceID,
		id:                     syntheticIdentity("derivation", binding.ControllerID+":"+fmt.Sprint(binding.PairGeneration)),
		contractID:             config.enrollment.contractID,
		providerID:             config.enrollment.providerID,
		providerVersion:        config.enrollment.providerVersion,
		evidenceSchema:         config.enrollment.evidenceSchema,
		databaseScope:          config.enrollment.databaseScope,
		sourceID:               config.enrollment.sourceID,
		accountID:              config.enrollment.accountID,
		pairID:                 config.pair.id,
		attemptID:              config.pair.attemptID,
		pairGeneration:         config.pair.generation,
		readerOwnerID:          config.reader.ownerID,
		collectorOwnerID:       config.collector.ownerID,
		readerRevision:         mapSyntheticIdentity("reader-revision", a.fixture.ReaderRevision),
		collectorRevision:      mapSyntheticIdentity("collector-revision", a.fixture.CollectorRevision),
		transferManifest:       fixtureTransferManifest,
		fromRole:               readerRole,
		toRole:                 collectorRole,
		seedApplied:            true,
		containsCookieMaterial: false,
		fixtureGrantID:         a.fixture.Grant.id,
	}
	if err := authority.recordFixtureDerivation(receipt); err != nil {
		a.mu.Lock()
		a.starting = false
		a.mu.Unlock()
		return err
	}
	a.mu.Lock()
	if a.revoked || !a.starting {
		a.starting = false
		a.mu.Unlock()
		authority.revoke()
		return authError(reasonRevoked)
	}
	a.started = true
	a.starting = false
	a.authority = authority
	a.baseBinding = cloneAuthorizationBinding(binding)
	a.mu.Unlock()
	if err := authority.authorize(ctx, a.fixture.Collect); err != nil {
		return err
	}
	return nil
}

func (a *authorizedConcurrentAdapter) authorizationConfig(binding concurrentAuthorizationBinding) (authorizationConfig, error) {
	f := a.fixture
	if binding.ControllerID == "" || binding.AttemptID == "" || binding.ParentID == "" || binding.Epoch == 0 || binding.PairGeneration == 0 ||
		binding.AccountID != f.AccountID || binding.Reader.ProfileID != a.readerProfile || binding.Reader.Role != concurrentReader || binding.Collector.Role != concurrentCollector ||
		!validOwner(binding.Reader, concurrentOwnerSpec{Role: concurrentReader, ProfileID: a.readerProfile, DirectoryID: binding.Reader.DirectoryID, ParentID: binding.ParentID, Driver: "synthetic-reader"}) ||
		!validOwner(binding.Collector, concurrentOwnerSpec{Role: concurrentCollector, ProfileID: binding.Collector.ProfileID, DirectoryID: binding.Collector.DirectoryID, ParentID: binding.ParentID, Driver: "synthetic-headless"}) {
		return authorizationConfig{}, authError(reasonInvalidConfiguration)
	}
	if binding.Reader.ProfileID == binding.Collector.ProfileID || binding.Reader.DirectoryID == binding.Collector.DirectoryID ||
		binding.Reader.PID == binding.Collector.PID || binding.Reader.CreationIdentity == binding.Collector.CreationIdentity {
		return authorizationConfig{}, authError(reasonInvalidConfiguration)
	}
	createdAt := a.clock()
	if createdAt.IsZero() {
		return authorizationConfig{}, authError(reasonInvalidConfiguration)
	}
	readerWindows := mapWindowInventory(binding.ReaderWindows)
	if len(readerWindows) == 0 {
		readerWindows = []string{syntheticIdentity("reader-inventory", binding.Reader.ID)}
	}
	collectorWindows := []string{syntheticIdentity("collector-inventory", binding.Collector.ID)}
	reader := mapConcurrentOwner(binding.Reader, readerRole, binding.ParentID, binding.Epoch, createdAt, readerWindows)
	collector := mapConcurrentOwner(binding.Collector, collectorRole, binding.ParentID, binding.Epoch, createdAt, collectorWindows)
	pairKey := fmt.Sprintf("%s|%s|%d|%d", syntheticIdentity("controller", binding.ControllerID), syntheticIdentity("attempt", binding.AttemptID), binding.Epoch, binding.PairGeneration)
	return authorizationConfig{
		enrollment: enrollment{
			contractID: concurrentAuthorizationContract, providerID: fixtureProviderID, providerVersion: fixtureProviderVersion,
			evidenceSchema: fixtureEvidenceSchema, sourceID: fixtureSourceID, databaseScope: mapSyntheticIdentity("database", f.DatabaseScope),
			accountID: mapSyntheticIdentity("account", f.AccountID), readerProfileID: mapSyntheticIdentity("reader-profile", a.readerProfile),
			policyGeneration: f.PolicyGeneration, holdGeneration: f.HoldGeneration,
		},
		pair: pairIdentity{
			id: syntheticIdentity("pair", pairKey), attemptID: mapSyntheticIdentity("attempt", binding.AttemptID),
			controllerEpochID: mapSyntheticIdentity("epoch", fmt.Sprint(binding.Epoch)), sidecarLifetimeID: mapSyntheticIdentity("sidecar", binding.ParentID),
			generation: binding.PairGeneration,
		},
		reader: reader, collector: collector, policyEnabled: f.PolicyEnabled, lifecycleBarrier: f.LifecycleBarrier,
	}, nil
}

func (a *authorizedConcurrentAdapter) RenewEvidence(ctx context.Context, binding concurrentAuthorizationBinding, validate func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, concurrentBindingDeadline)
	defer cancel()
	a.mu.Lock()
	authority := a.authority
	base := a.baseBinding
	revoked := a.revoked
	a.mu.Unlock()
	if authority == nil || revoked || !sameAuthorizationBinding(base, binding) {
		return authError(reasonOwnerDrift)
	}
	return authority.renewValidated(ctx, a.fixture.Collect, validate)
}

func (a *authorizedConcurrentAdapter) Ticket(workID string) (concurrentAuthorizationTicket, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.authority == nil || a.revoked {
		return concurrentAuthorizationTicket{}, authError(reasonDisabled)
	}
	if existing, ok := a.ticketsByWork[workID]; ok {
		return existing, nil
	}
	ticket, err := a.authority.dispatch(workCommand{
		id: mapSyntheticIdentity("work", workID), scope: mapSyntheticIdentity("database", a.fixture.DatabaseScope),
		source: fixtureSourceID, operation: "synthetic-x.collect",
	})
	if err != nil {
		return concurrentAuthorizationTicket{}, err
	}
	result := concurrentAuthorizationTicket{ID: ticket.ticketID, EvidenceVersion: ticket.evidenceVersion, WorkID: workID}
	a.ticketsByWork[workID] = result
	a.authorityTicket[result.ID] = ticket
	return result, nil
}

func (a *authorizedConcurrentAdapter) Claim(ticket concurrentAuthorizationTicket, commit func() error) error {
	authority, canonical, err := a.resolveTicket(ticket)
	if err != nil {
		return err
	}
	return authority.claimWithCommit(canonical, commit)
}

func (a *authorizedConcurrentAdapter) Complete(ticket concurrentAuthorizationTicket, result string, commit func() error) error {
	authority, canonical, err := a.resolveTicket(ticket)
	if err != nil {
		return err
	}
	fixtureResult := fixtureResult{
		id: mapSyntheticIdentity("result", result), ticketID: canonical.ticketID, commandID: canonical.commandID,
		scope: canonical.scope, source: canonical.source, operation: canonical.operation, evidenceVersion: canonical.evidenceVersion,
		pairID: canonical.pairID, pairGeneration: canonical.pairGeneration, fence: canonical.fence, collectorOwnerID: canonical.collectorOwnerID,
	}
	return authority.completeWithCommit(canonical, fixtureResult, commit)
}

func (a *authorizedConcurrentAdapter) Cancel(ticket concurrentAuthorizationTicket, commit func() error) error {
	authority, canonical, err := a.resolveTicket(ticket)
	if err != nil {
		return err
	}
	return authority.cancelWithCommit(canonical, commit)
}

func (a *authorizedConcurrentAdapter) resolveTicket(ticket concurrentAuthorizationTicket) (*concurrentAuthorization, workTicket, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.authority == nil || a.revoked {
		return nil, workTicket{}, authError(reasonRevoked)
	}
	canonical, ok := a.authorityTicket[ticket.ID]
	if !ok || canonical.evidenceVersion != ticket.EvidenceVersion || canonical.commandID != mapSyntheticIdentity("work", ticket.WorkID) {
		return nil, workTicket{}, authError(reasonStaleTicket)
	}
	return a.authority, canonical, nil
}

func (a *authorizedConcurrentAdapter) Check() error {
	a.mu.Lock()
	authority := a.authority
	revoked := a.revoked
	a.mu.Unlock()
	if authority == nil || revoked {
		return authError(reasonDisabled)
	}
	return authority.check()
}

func (a *authorizedConcurrentAdapter) Revoke(reason string) {
	a.mu.Lock()
	a.revoked = true
	authority := a.authority
	a.mu.Unlock()
	if authority != nil {
		authority.revoke()
	}
	_ = reason // Reasons remain controller-local; the authority emits typed fixture outcomes.
}

func (a *authorizedConcurrentAdapter) PolicyChanged(policyGeneration, holdGeneration uint64, enabled bool) {
	a.mu.Lock()
	a.revoked = true
	authority := a.authority
	a.mu.Unlock()
	if authority != nil {
		authority.policyChanged(policyGeneration, holdGeneration, enabled)
	}
}

func (a *authorizedConcurrentAdapter) OwnerChanged(role concurrentRole) {
	a.mu.Lock()
	a.revoked = true
	authority := a.authority
	a.mu.Unlock()
	if authority != nil {
		if role == concurrentReader {
			_ = authority.ownerDrift(readerRole)
		} else if role == concurrentCollector {
			_ = authority.ownerDrift(collectorRole)
		} else {
			authority.revoke()
		}
	}
}

func (a *authorizedConcurrentAdapter) UpdateReaderWindows(windows []string) error {
	a.mu.Lock()
	authority := a.authority
	revoked := a.revoked
	a.mu.Unlock()
	if authority == nil || revoked {
		return authError(reasonRevoked)
	}
	return authority.updateReaderWindows(mapWindowInventory(windows))
}

func (a *authorizedConcurrentAdapter) RequestIndependentCollector() error {
	a.mu.Lock()
	authority := a.authority
	a.mu.Unlock()
	if authority == nil {
		return authError(reasonIndependentCollector)
	}
	return authority.requestIndependentCollector()
}

func cloneAuthorizationBinding(binding concurrentAuthorizationBinding) concurrentAuthorizationBinding {
	binding.ReaderWindows = append([]string(nil), binding.ReaderWindows...)
	return binding
}

func sameAuthorizationBinding(left, right concurrentAuthorizationBinding) bool {
	return left.ControllerID == right.ControllerID && left.AttemptID == right.AttemptID && left.ParentID == right.ParentID &&
		left.Epoch == right.Epoch && left.PairGeneration == right.PairGeneration && left.AccountID == right.AccountID &&
		sameOwner(left.Reader, right.Reader) && sameOwner(left.Collector, right.Collector)
}

func mapConcurrentOwner(owner concurrentOwner, ownerRole role, sidecarID string, epoch uint64, createdAt time.Time, inventory []string) processOwner {
	roleName := string(ownerRole)
	return processOwner{
		role: ownerRole, ownerID: mapSyntheticIdentity(roleName+"-owner", owner.ID),
		profileID: mapSyntheticIdentity(roleName+"-profile", owner.ProfileID), directoryID: mapSyntheticIdentity(roleName+"-directory", owner.DirectoryID),
		processID:         syntheticIdentity("process", fmt.Sprintf("%d:%s", owner.PID, owner.CreationIdentity)),
		processCreationID: mapSyntheticIdentity("creation", owner.CreationIdentity), processCreatedAt: createdAt,
		processImageID: mapSyntheticIdentity("image", owner.Driver), sidecarLifetimeID: mapSyntheticIdentity("sidecar", sidecarID),
		contextID: syntheticIdentity("context", string(ownerRole)+":"+owner.DirectoryID), epochID: mapSyntheticIdentity("epoch", fmt.Sprint(epoch)),
		generation: owner.Generation, driverID: mapSyntheticIdentity("driver", owner.Driver), directoryPinned: true,
		profileFresh: true, disposableContext: true, windowInventory: append([]string(nil), inventory...),
	}
}

func mapWindowInventory(windows []string) []string {
	mapped := make([]string, 0, len(windows))
	for _, window := range windows {
		mapped = append(mapped, mapSyntheticIdentity("window", window))
	}
	return mapped
}

func mapSyntheticIdentity(label, value string) string {
	return syntheticIdentity(label, value)
}

func syntheticIdentity(label, value string) string {
	return "synthetic-" + label + "-" + hex.EncodeToString([]byte(value))
}
