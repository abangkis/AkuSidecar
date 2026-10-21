//go:build auth_session_dev && auth_session_fixture

package engine

// This file is an offline contract model. It is deliberately excluded from
// ordinary builds and accepts only explicitly granted synthetic fixtures.
// It is not a native X provider, a durable Store fence, or a production API.

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

const (
	concurrentAuthorizationContract = "aku.x.concurrent-authorization.v1"
	fixtureProviderID               = "synthetic-x-concurrent"
	fixtureProviderVersion          = "synthetic-provider-v1"
	fixtureEvidenceSchema           = "synthetic-evidence-v1"
	fixtureSourceID                 = "synthetic-x"
	fixtureTransferManifest         = "synthetic-reviewed-transfer-v1"
	fixtureProvenance               = "synthetic-qualified-fixture/v1"
	fixtureAuthentication           = "synthetic-qualified-authenticated/v1"
	fixtureIdentity                 = "synthetic-confirmed-identity/v1"
	fixtureCacheFresh               = "synthetic-cache-fresh"
	fixtureOrigin                   = "synthetic-x://account/session"

	maximumEvidenceAge = 5 * time.Second
	maximumBindingAge  = 2 * time.Second
)

type authorizationReason string

const (
	reasonInvalidConfiguration authorizationReason = "invalid-configuration"
	reasonUnsupportedProvider  authorizationReason = "unsupported-provider"
	reasonFixtureGrant         authorizationReason = "fixture-grant-required"
	reasonDisabled             authorizationReason = "admission-disabled"
	reasonAlreadyEstablished   authorizationReason = "already-established"
	reasonMissingDerivation    authorizationReason = "derivation-required"
	reasonDerivationReplay     authorizationReason = "derivation-replayed"
	reasonInvalidEvidence      authorizationReason = "invalid-evidence"
	reasonEvidenceExpired      authorizationReason = "evidence-expired"
	reasonBindingDeadline      authorizationReason = "binding-deadline-exceeded"
	reasonCoverageGap          authorizationReason = "coverage-gap"
	reasonUnknownAuth          authorizationReason = "authentication-unknown"
	reasonUnknownIdentity      authorizationReason = "identity-unknown"
	reasonUnknownLineage       authorizationReason = "lineage-unknown"
	reasonSessionChanged       authorizationReason = "session-changed"
	reasonClockRegression      authorizationReason = "monotonic-clock-regressed"
	reasonRenewalFailed        authorizationReason = "renewal-failed"
	reasonRevoked              authorizationReason = "revoked"
	reasonOwnerDrift           authorizationReason = "owner-drift"
	reasonPolicyChanged        authorizationReason = "policy-changed"
	reasonOptedOut             authorizationReason = "opted-out"
	reasonReaderLost           authorizationReason = "reader-lost"
	reasonIndependentCollector authorizationReason = "independent-collector-unsupported"
	reasonUnsupportedOperation authorizationReason = "unsupported-operation"
	reasonStaleTicket          authorizationReason = "stale-ticket"
	reasonTicketNotClaimed     authorizationReason = "ticket-not-claimed"
	reasonTicketCancelled      authorizationReason = "ticket-cancelled"
	reasonDuplicateResult      authorizationReason = "duplicate-result"
	reasonContextCancelled     authorizationReason = "context-cancelled"
	reasonConcurrentInvalid    authorizationReason = "authority-changed-during-renewal"
)

type authorizationError struct {
	reason authorizationReason
	cause  error
}

func (e authorizationError) Error() string { return string(e.reason) }
func (e authorizationError) Unwrap() error { return e.cause }

func authError(reason authorizationReason) error { return authorizationError{reason: reason} }

func authErrorWithCause(reason authorizationReason, cause error) error {
	return authorizationError{reason: reason, cause: cause}
}

type role string

const (
	readerRole    role = "reader"
	collectorRole role = "collector"
)

// fixtureGrant is private and exists only in this dual-tagged fixture build.
// Every permission is explicit so a synthetic scenario cannot imply native
// source qualification or production authority.
type fixtureGrant struct {
	id                 string
	allowAuthorization bool
	allowSeed          bool
	allowCollection    bool
	allowRenewal       bool
	allowResultSink    bool
}

type enrollment struct {
	contractID       string
	providerID       string
	providerVersion  string
	evidenceSchema   string
	sourceID         string
	databaseScope    string
	accountID        string
	readerProfileID  string
	policyGeneration uint64
	holdGeneration   uint64
}

type pairIdentity struct {
	id                string
	attemptID         string
	controllerEpochID string
	sidecarLifetimeID string
	generation        uint64
}

type processOwner struct {
	role              role
	ownerID           string
	profileID         string
	directoryID       string
	processID         string
	processCreationID string
	processCreatedAt  time.Time
	processImageID    string
	sidecarLifetimeID string
	contextID         string
	epochID           string
	generation        uint64
	driverID          string
	directoryPinned   bool
	profileFresh      bool
	disposableContext bool
	windowInventory   []string
}

type authorizationConfig struct {
	enrollment    enrollment
	pair          pairIdentity
	reader        processOwner
	collector     processOwner
	policyEnabled bool
}

type derivationReceipt struct {
	instanceID             string
	id                     string
	contractID             string
	providerID             string
	providerVersion        string
	evidenceSchema         string
	databaseScope          string
	sourceID               string
	accountID              string
	pairID                 string
	attemptID              string
	pairGeneration         uint64
	readerOwnerID          string
	collectorOwnerID       string
	readerRevision         string
	collectorRevision      string
	transferManifest       string
	fromRole               role
	toRole                 role
	seedApplied            bool
	containsCookieMaterial bool
	fixtureGrantID         string
}

type evidenceChallenge struct {
	instanceID         string
	id                 string
	vectorID           string
	frameID            string
	requestID          string
	contractID         string
	providerID         string
	providerVersion    string
	evidenceSchema     string
	sourceID           string
	origin             string
	operation          string
	databaseScope      string
	accountID          string
	pairID             string
	attemptID          string
	pairGeneration     uint64
	role               role
	owner              processOwner
	acquisitionStarted time.Time
	previousCoverage   time.Time
	previousVersion    uint64
	fence              uint64
	fixtureGrantID     string
}

// roleEvidence is caller-supplied data and is never trusted without an exact
// match to the issued challenge, qualified fixture markers, and pinned owner.
type roleEvidence struct {
	instanceID          string
	challengeID         string
	vectorID            string
	contractID          string
	providerID          string
	providerVersion     string
	evidenceSchema      string
	sourceID            string
	origin              string
	operation           string
	databaseScope       string
	accountID           string
	pairID              string
	attemptID           string
	pairGeneration      uint64
	role                role
	owner               processOwner
	frameID             string
	requestID           string
	fixtureGrantID      string
	provenance          string
	authentication      string
	identity            string
	cacheState          string
	authenticated       bool
	continuityKnown     bool
	coverageKnown       bool
	coverageContinuous  bool
	coverageGap         bool
	sessionRevision     string
	acquisitionStarted  time.Time
	observedAt          time.Time
	coverageFrom        time.Time
	coverageThrough     time.Time
	authValidUntil      time.Time
	identityValidUntil  time.Time
	coverageValidUntil  time.Time
	providerValidUntil  time.Time
	claimedNativeSource bool
}

type evidenceCollector func(context.Context, evidenceChallenge) (roleEvidence, error)

type permit struct {
	evidenceVersion          uint64
	expiresAt                time.Time
	readerRevision           string
	collectorRevision        string
	readerCoverageThrough    time.Time
	collectorCoverageThrough time.Time
	readerOwner              processOwner
	collectorOwner           processOwner
	pairGeneration           uint64
	fence                    uint64
}

type renewalLink struct {
	fromVersion           uint64
	toVersion             uint64
	pairGeneration        uint64
	readerRevision        string
	collectorRevision     string
	collectorOwnerID      string
	readerCoverageFrom    time.Time
	readerCoverageTo      time.Time
	collectorCoverageFrom time.Time
	collectorCoverageTo   time.Time
}

type workCommand struct {
	id        string
	scope     string
	source    string
	operation string
}

type workTicket struct {
	instanceID       string
	ticketID         string
	commandID        string
	scope            string
	source           string
	operation        string
	evidenceVersion  uint64
	pairID           string
	pairGeneration   uint64
	fence            uint64
	collectorOwnerID string
	dispatchAt       time.Time
}

type workStatus string

const (
	workPending     workStatus = "pending"
	workClaimed     workStatus = "claimed"
	workCompleted   workStatus = "completed"
	workCancelled   workStatus = "cancelled"
	workInvalidated workStatus = "invalidated"
)

type ticketRecord struct {
	ticket workTicket
	status workStatus
}

type fixtureResult struct {
	id               string
	ticketID         string
	commandID        string
	scope            string
	source           string
	operation        string
	evidenceVersion  uint64
	pairID           string
	pairGeneration   uint64
	fence            uint64
	collectorOwnerID string
}

type authorizationView struct {
	nativeQualified    bool
	canAuthorize       bool
	fixturePermitValid bool
	policyEnabled      bool
	evidenceVersion    uint64
	pairGeneration     uint64
	fence              uint64
	expiresAt          time.Time
	reason             authorizationReason
}

type concurrentAuthorization struct {
	mu                 sync.Mutex
	renewalGate        chan struct{}
	clock              func() time.Time
	lastClock          time.Time
	instanceID         string
	config             authorizationConfig
	grant              fixtureGrant
	derivation         *derivationReceipt
	derivationConsumed bool
	permit             *permit
	versionCounter     uint64
	challengeCounter   uint64
	fence              uint64
	pendingRevocation  bool
	revoked            bool
	reason             authorizationReason
	coverageThrough    map[role]time.Time
	versionCoverage    map[uint64]coverageCheckpoint
	renewalLinks       map[uint64]renewalLink
	tickets            map[string]*ticketRecord
	memorySink         map[string]fixtureResult
	sideEffects        uint64
}

type coverageCheckpoint struct {
	reader    time.Time
	collector time.Time
}

var authorizationInstanceSequence atomic.Uint64
var errBindingDeadline = errors.New("synthetic binding deadline elapsed")

func newConcurrentAuthorization(config authorizationConfig, grant fixtureGrant, clock func() time.Time) (*concurrentAuthorization, error) {
	if !validFixtureGrant(grant) {
		return nil, authError(reasonFixtureGrant)
	}
	if clock == nil {
		return nil, authError(reasonInvalidConfiguration)
	}
	if config.enrollment.providerID == "x-concurrent" {
		return nil, authError(reasonUnsupportedProvider)
	}
	if err := validateAuthorizationConfig(config); err != nil {
		return nil, err
	}
	initial := clock()
	if initial.IsZero() {
		return nil, authError(reasonInvalidConfiguration)
	}
	instanceID := fmt.Sprintf("synthetic-instance-%d", authorizationInstanceSequence.Add(1))
	return &concurrentAuthorization{
		renewalGate:     make(chan struct{}, 1),
		clock:           clock,
		lastClock:       initial,
		instanceID:      instanceID,
		config:          cloneConfig(config),
		grant:           grant,
		fence:           1,
		reason:          reasonDisabled,
		coverageThrough: make(map[role]time.Time, 2),
		versionCoverage: make(map[uint64]coverageCheckpoint),
		renewalLinks:    make(map[uint64]renewalLink),
		tickets:         make(map[string]*ticketRecord),
		memorySink:      make(map[string]fixtureResult),
	}, nil
}

func validFixtureGrant(grant fixtureGrant) bool {
	return syntheticID(grant.id) && grant.allowAuthorization && grant.allowSeed && grant.allowCollection && grant.allowRenewal && grant.allowResultSink
}

func validateAuthorizationConfig(config authorizationConfig) error {
	e := config.enrollment
	p := config.pair
	r := config.reader
	c := config.collector
	if e.contractID != concurrentAuthorizationContract || e.providerID != fixtureProviderID || e.providerVersion != fixtureProviderVersion || e.evidenceSchema != fixtureEvidenceSchema || e.sourceID != fixtureSourceID {
		return authError(reasonUnsupportedProvider)
	}
	if !syntheticID(e.databaseScope) || !syntheticID(e.accountID) || !syntheticID(e.readerProfileID) || e.policyGeneration == 0 || e.holdGeneration == 0 {
		return authError(reasonInvalidConfiguration)
	}
	if !syntheticID(p.id) || !syntheticID(p.attemptID) || !syntheticID(p.controllerEpochID) || !syntheticID(p.sidecarLifetimeID) || p.generation == 0 {
		return authError(reasonInvalidConfiguration)
	}
	if err := validateOwner(r, readerRole, p); err != nil {
		return err
	}
	if err := validateOwner(c, collectorRole, p); err != nil {
		return err
	}
	if r.profileID != e.readerProfileID || r.profileID == c.profileID || r.directoryID == c.directoryID || r.contextID == c.contextID || r.processID == c.processID || r.ownerID == c.ownerID || r.driverID == c.driverID {
		return authError(reasonInvalidConfiguration)
	}
	if !c.profileFresh || !c.disposableContext || len(c.windowInventory) == 0 || len(r.windowInventory) == 0 {
		return authError(reasonInvalidConfiguration)
	}
	return nil
}

func validateOwner(owner processOwner, expectedRole role, pair pairIdentity) error {
	if owner.role != expectedRole || !syntheticID(owner.ownerID) || !syntheticID(owner.profileID) || !syntheticID(owner.directoryID) || !syntheticID(owner.processID) || !syntheticID(owner.processCreationID) || owner.processCreatedAt.IsZero() || !syntheticID(owner.processImageID) || !syntheticID(owner.sidecarLifetimeID) || !syntheticID(owner.contextID) || !syntheticID(owner.epochID) || !syntheticID(owner.driverID) || owner.generation == 0 || !owner.directoryPinned {
		return authError(reasonInvalidConfiguration)
	}
	if owner.sidecarLifetimeID != pair.sidecarLifetimeID || owner.epochID != pair.controllerEpochID {
		return authError(reasonInvalidConfiguration)
	}
	if len(owner.windowInventory) == 0 {
		return authError(reasonInvalidConfiguration)
	}
	for _, window := range owner.windowInventory {
		if !syntheticID(window) {
			return authError(reasonInvalidConfiguration)
		}
	}
	return nil
}

func syntheticID(value string) bool {
	return len(value) > len("synthetic-") && value[:len("synthetic-")] == "synthetic-"
}

func cloneConfig(config authorizationConfig) authorizationConfig {
	config.reader = cloneOwner(config.reader)
	config.collector = cloneOwner(config.collector)
	return config
}

func cloneOwner(owner processOwner) processOwner {
	owner.windowInventory = append([]string(nil), owner.windowInventory...)
	return owner
}

func ownerEqual(left, right processOwner) bool {
	return left.role == right.role && left.ownerID == right.ownerID && left.profileID == right.profileID && left.directoryID == right.directoryID && left.processID == right.processID && left.processCreationID == right.processCreationID && left.processCreatedAt.Equal(right.processCreatedAt) && left.processImageID == right.processImageID && left.sidecarLifetimeID == right.sidecarLifetimeID && left.contextID == right.contextID && left.epochID == right.epochID && left.generation == right.generation && left.driverID == right.driverID && left.directoryPinned == right.directoryPinned && left.profileFresh == right.profileFresh && left.disposableContext == right.disposableContext
}

func ownerBindingEqual(left, right processOwner) bool {
	if !ownerEqual(left, right) || len(left.windowInventory) != len(right.windowInventory) {
		return false
	}
	for i := range left.windowInventory {
		if left.windowInventory[i] != right.windowInventory[i] {
			return false
		}
	}
	return true
}

func (a *concurrentAuthorization) recordFixtureDerivation(receipt derivationReceipt) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, err := a.readClockLocked(); err != nil {
		return err
	}
	if !a.config.policyEnabled {
		return authError(reasonDisabled)
	}
	if a.revoked || a.pendingRevocation {
		return authError(reasonRevoked)
	}
	if a.derivation != nil || a.derivationConsumed {
		return authError(reasonDerivationReplay)
	}
	if !a.grant.allowSeed || !a.validDerivation(receipt) {
		return authError(reasonInvalidEvidence)
	}
	copyReceipt := receipt
	a.derivation = &copyReceipt
	return nil
}

func (a *concurrentAuthorization) validDerivation(receipt derivationReceipt) bool {
	e := a.config.enrollment
	p := a.config.pair
	return receipt.instanceID == a.instanceID && syntheticID(receipt.id) && receipt.contractID == e.contractID && receipt.providerID == e.providerID && receipt.providerVersion == e.providerVersion && receipt.evidenceSchema == e.evidenceSchema && receipt.databaseScope == e.databaseScope && receipt.sourceID == e.sourceID && receipt.accountID == e.accountID && receipt.pairID == p.id && receipt.attemptID == p.attemptID && receipt.pairGeneration == p.generation && receipt.readerOwnerID == a.config.reader.ownerID && receipt.collectorOwnerID == a.config.collector.ownerID && syntheticID(receipt.readerRevision) && syntheticID(receipt.collectorRevision) && receipt.transferManifest == fixtureTransferManifest && receipt.fromRole == readerRole && receipt.toRole == collectorRole && receipt.seedApplied && !receipt.containsCookieMaterial && receipt.fixtureGrantID == a.grant.id
}

func (a *concurrentAuthorization) authorize(ctx context.Context, collect evidenceCollector) error {
	if err := a.acquireRenewalSlot(ctx); err != nil {
		return err
	}
	defer a.releaseRenewalSlot()
	if ctx == nil || collect == nil {
		return a.failInitial(reasonInvalidEvidence, nil)
	}
	a.mu.Lock()
	if _, err := a.readClockLocked(); err != nil {
		a.mu.Unlock()
		return err
	}
	if !a.config.policyEnabled {
		a.mu.Unlock()
		return authError(reasonDisabled)
	}
	if a.revoked || a.pendingRevocation {
		a.mu.Unlock()
		return authError(reasonRevoked)
	}
	if a.permit != nil || a.versionCounter != 0 {
		a.mu.Unlock()
		return authError(reasonAlreadyEstablished)
	}
	if a.derivation == nil || a.derivationConsumed {
		a.mu.Unlock()
		return authError(reasonMissingDerivation)
	}
	start := a.lastClock
	challenges := a.makeChallengesLocked(start, 0, map[role]time.Time{readerRole: start, collectorRole: start})
	snapshot := a.snapshotLocked()
	a.mu.Unlock()

	evidence, err := collectVectorBounded(ctx, collect, challenges)
	if err != nil {
		return a.failInitial(collectionErrorReason(err), err)
	}
	if err := contextError(ctx); err != nil {
		return a.failInitial(reasonContextCancelled, err)
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	now, clockErr := a.readClockLocked()
	if clockErr != nil {
		return clockErr
	}
	if !a.snapshotCurrentLocked(snapshot) {
		return a.failInitialLocked(reasonConcurrentInvalid, nil)
	}
	if err := a.validateVectorLocked(evidence, challenges, now, nil); err != nil {
		return a.failInitialLocked(errorReason(err, reasonInvalidEvidence), err)
	}
	if now.Sub(start) >= maximumBindingAge {
		return a.failInitialLocked(reasonBindingDeadline, nil)
	}
	if now.Sub(start) >= maximumEvidenceAge {
		return a.failInitialLocked(reasonEvidenceExpired, nil)
	}
	reader := evidence[readerRole]
	collector := evidence[collectorRole]
	if reader.sessionRevision != a.derivation.readerRevision || collector.sessionRevision != a.derivation.collectorRevision {
		return a.failInitialLocked(reasonSessionChanged, nil)
	}
	expiresAt := earliestDeadline(start.Add(maximumEvidenceAge), reader, collector)
	if !now.Before(expiresAt) {
		return a.failInitialLocked(reasonEvidenceExpired, nil)
	}
	a.versionCounter = 1
	a.permit = &permit{
		evidenceVersion:          1,
		expiresAt:                expiresAt,
		readerRevision:           reader.sessionRevision,
		collectorRevision:        collector.sessionRevision,
		readerCoverageThrough:    reader.coverageThrough,
		collectorCoverageThrough: collector.coverageThrough,
		readerOwner:              cloneOwner(a.config.reader),
		collectorOwner:           cloneOwner(a.config.collector),
		pairGeneration:           a.config.pair.generation,
		fence:                    a.fence,
	}
	a.coverageThrough[readerRole] = reader.coverageThrough
	a.coverageThrough[collectorRole] = collector.coverageThrough
	a.versionCoverage[1] = coverageCheckpoint{reader: reader.coverageThrough, collector: collector.coverageThrough}
	a.derivationConsumed = true
	a.reason = ""
	return nil
}

func (a *concurrentAuthorization) failInitial(reason authorizationReason, cause error) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.failInitialLocked(reason, cause)
}

func (a *concurrentAuthorization) failInitialLocked(reason authorizationReason, cause error) error {
	a.revokeLocked(reason)
	return authErrorWithCause(reason, cause)
}

func (a *concurrentAuthorization) renew(ctx context.Context, collect evidenceCollector) error {
	if err := a.acquireRenewalSlot(ctx); err != nil {
		return err
	}
	defer a.releaseRenewalSlot()
	if ctx == nil || collect == nil {
		return a.failRenewal(nil, reasonInvalidEvidence)
	}

	a.mu.Lock()
	if _, err := a.readClockLocked(); err != nil {
		a.mu.Unlock()
		return err
	}
	if err := a.requireCurrentPermitLocked(); err != nil {
		a.mu.Unlock()
		return err
	}
	if !a.grant.allowRenewal {
		a.mu.Unlock()
		return a.failRenewal(nil, reasonFixtureGrant)
	}
	start := a.lastClock
	previous := *a.permit
	previous.readerOwner = cloneOwner(previous.readerOwner)
	previous.collectorOwner = cloneOwner(previous.collectorOwner)
	snapshot := a.snapshotLocked()
	checkpoints := map[role]time.Time{
		readerRole:    previous.readerCoverageThrough,
		collectorRole: previous.collectorCoverageThrough,
	}
	challenges := a.makeChallengesLocked(start, previous.evidenceVersion, checkpoints)
	a.mu.Unlock()

	evidence, err := collectVectorBounded(ctx, collect, challenges)
	if err != nil {
		return a.failRenewal(err, collectionErrorReason(err))
	}
	if err := contextError(ctx); err != nil {
		return a.failRenewal(err, reasonContextCancelled)
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	now, clockErr := a.readClockLocked()
	if clockErr != nil {
		return clockErr
	}
	if !a.snapshotCurrentLocked(snapshot) || a.permit == nil || a.permit.evidenceVersion != previous.evidenceVersion || !samePermitPredecessor(*a.permit, previous) {
		return authError(reasonConcurrentInvalid)
	}
	if !now.Before(previous.expiresAt) {
		a.expireLocked()
		return authError(reasonEvidenceExpired)
	}
	if err := a.validateVectorLocked(evidence, challenges, now, checkpoints); err != nil {
		return a.failRenewalLocked(errorReason(err, reasonRenewalFailed), err)
	}
	if now.Sub(start) >= maximumBindingAge {
		return a.failRenewalLocked(reasonBindingDeadline, nil)
	}
	if now.Sub(start) >= maximumEvidenceAge {
		return a.failRenewalLocked(reasonEvidenceExpired, nil)
	}
	reader := evidence[readerRole]
	collector := evidence[collectorRole]
	if reader.sessionRevision != previous.readerRevision || collector.sessionRevision != previous.collectorRevision {
		return a.failRenewalLocked(reasonSessionChanged, nil)
	}
	if !ownerEqual(a.config.reader, previous.readerOwner) || !ownerEqual(a.config.collector, previous.collectorOwner) {
		a.revokeLocked(reasonOwnerDrift)
		return authError(reasonOwnerDrift)
	}
	newExpiry := earliestDeadline(start.Add(maximumEvidenceAge), reader, collector)
	if !now.Before(newExpiry) {
		return a.failRenewalLocked(reasonEvidenceExpired, nil)
	}
	fromVersion := previous.evidenceVersion
	toVersion := fromVersion + 1
	if toVersion <= fromVersion {
		return a.failRenewalLocked(reasonInvalidEvidence, nil)
	}
	link := renewalLink{
		fromVersion:           fromVersion,
		toVersion:             toVersion,
		pairGeneration:        a.config.pair.generation,
		readerRevision:        previous.readerRevision,
		collectorRevision:     previous.collectorRevision,
		collectorOwnerID:      previous.collectorOwner.ownerID,
		readerCoverageFrom:    reader.coverageFrom,
		readerCoverageTo:      reader.coverageThrough,
		collectorCoverageFrom: collector.coverageFrom,
		collectorCoverageTo:   collector.coverageThrough,
	}
	// The mutex makes this the single publish point seen by authority readers,
	// claims, completions, opt-out, and revoke.
	a.renewalLinks[fromVersion] = link
	a.versionCounter = toVersion
	a.permit = &permit{
		evidenceVersion:          toVersion,
		expiresAt:                newExpiry,
		readerRevision:           previous.readerRevision,
		collectorRevision:        previous.collectorRevision,
		readerCoverageThrough:    reader.coverageThrough,
		collectorCoverageThrough: collector.coverageThrough,
		readerOwner:              cloneOwner(a.config.reader),
		collectorOwner:           cloneOwner(previous.collectorOwner),
		pairGeneration:           previous.pairGeneration,
		fence:                    previous.fence,
	}
	a.coverageThrough[readerRole] = reader.coverageThrough
	a.coverageThrough[collectorRole] = collector.coverageThrough
	a.versionCoverage[toVersion] = coverageCheckpoint{reader: reader.coverageThrough, collector: collector.coverageThrough}
	a.reason = ""
	return nil
}

func samePermitPredecessor(current, previous permit) bool {
	return current.evidenceVersion == previous.evidenceVersion && current.expiresAt.Equal(previous.expiresAt) && current.readerRevision == previous.readerRevision && current.collectorRevision == previous.collectorRevision && current.pairGeneration == previous.pairGeneration && current.fence == previous.fence && ownerEqual(current.readerOwner, previous.readerOwner) && ownerEqual(current.collectorOwner, previous.collectorOwner)
}

func (a *concurrentAuthorization) failRenewal(cause error, reason authorizationReason) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.failRenewalLocked(reason, cause)
}

func (a *concurrentAuthorization) failRenewalLocked(reason authorizationReason, cause error) error {
	if a.permit != nil && !a.revoked {
		a.revokeLocked(reason)
	}
	return authErrorWithCause(reason, cause)
}

func (a *concurrentAuthorization) acquireRenewalSlot(ctx context.Context) error {
	if ctx == nil {
		return authError(reasonContextCancelled)
	}
	select {
	case a.renewalGate <- struct{}{}:
		return nil
	case <-ctx.Done():
		return authErrorWithCause(reasonContextCancelled, ctx.Err())
	}
}

func (a *concurrentAuthorization) releaseRenewalSlot() { <-a.renewalGate }

func (a *concurrentAuthorization) makeChallengesLocked(start time.Time, previousVersion uint64, checkpoints map[role]time.Time) map[role]evidenceChallenge {
	a.challengeCounter++
	vectorID := fmt.Sprintf("synthetic-vector-%s-%d", a.instanceID, a.challengeCounter)
	makeChallenge := func(r role, owner processOwner) evidenceChallenge {
		id := fmt.Sprintf("synthetic-challenge-%s-%d-%s", a.instanceID, a.challengeCounter, r)
		frameID := fmt.Sprintf("synthetic-frame-%s-%d-%s", a.instanceID, a.challengeCounter, r)
		requestID := fmt.Sprintf("synthetic-request-%s-%d-%s", a.instanceID, a.challengeCounter, r)
		operation := "synthetic-x.observe-reader-session"
		if r == collectorRole {
			operation = "synthetic-x.observe-collector-session"
		}
		return evidenceChallenge{
			instanceID:         a.instanceID,
			id:                 id,
			vectorID:           vectorID,
			frameID:            frameID,
			requestID:          requestID,
			contractID:         a.config.enrollment.contractID,
			providerID:         a.config.enrollment.providerID,
			providerVersion:    a.config.enrollment.providerVersion,
			evidenceSchema:     a.config.enrollment.evidenceSchema,
			sourceID:           a.config.enrollment.sourceID,
			origin:             fixtureOrigin,
			operation:          operation,
			databaseScope:      a.config.enrollment.databaseScope,
			accountID:          a.config.enrollment.accountID,
			pairID:             a.config.pair.id,
			attemptID:          a.config.pair.attemptID,
			pairGeneration:     a.config.pair.generation,
			role:               r,
			owner:              cloneOwner(owner),
			acquisitionStarted: start,
			previousCoverage:   checkpoints[r],
			previousVersion:    previousVersion,
			fence:              a.fence,
			fixtureGrantID:     a.grant.id,
		}
	}
	return map[role]evidenceChallenge{
		readerRole:    makeChallenge(readerRole, a.config.reader),
		collectorRole: makeChallenge(collectorRole, a.config.collector),
	}
}

func collectVector(ctx context.Context, collect evidenceCollector, challenges map[role]evidenceChallenge) (map[role]roleEvidence, error) {
	result := make(map[role]roleEvidence, 2)
	for _, r := range []role{readerRole, collectorRole} {
		if err := contextError(ctx); err != nil {
			return nil, err
		}
		challenge := challenges[r]
		evidence, err := collect(ctx, challenge)
		if err != nil {
			return nil, err
		}
		result[r] = evidence
	}
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	return result, nil
}

// The evidence adapter is called synchronously with a bounded, cooperative
// context. No helper goroutine is left behind if the adapter blocks.
func collectVectorBounded(ctx context.Context, collect evidenceCollector, challenges map[role]evidenceChallenge) (map[role]roleEvidence, error) {
	bounded, cancel := context.WithTimeoutCause(ctx, maximumBindingAge, errBindingDeadline)
	defer cancel()
	vector, err := collectVector(bounded, collect, challenges)
	if err != nil {
		if errors.Is(context.Cause(bounded), errBindingDeadline) {
			return nil, errBindingDeadline
		}
		return nil, err
	}
	if cause := context.Cause(bounded); errors.Is(cause, errBindingDeadline) {
		return nil, errBindingDeadline
	} else if cause != nil {
		return nil, cause
	}
	return vector, nil
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return errors.New("nil context")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func (a *concurrentAuthorization) validateVectorLocked(vector map[role]roleEvidence, challenges map[role]evidenceChallenge, now time.Time, checkpoints map[role]time.Time) error {
	if len(vector) != 2 {
		return authError(reasonInvalidEvidence)
	}
	var lastObserved time.Time
	for _, r := range []role{readerRole, collectorRole} {
		evidence, ok := vector[r]
		challenge, challengeOK := challenges[r]
		if !ok || !challengeOK {
			return authError(reasonInvalidEvidence)
		}
		if evidence.role != r || challenge.role != r {
			return authError(reasonInvalidEvidence)
		}
		if evidence.instanceID != challenge.instanceID || evidence.challengeID != challenge.id || evidence.vectorID != challenge.vectorID || evidence.acquisitionStarted.IsZero() || !evidence.acquisitionStarted.Equal(challenge.acquisitionStarted) {
			return authError(reasonInvalidEvidence)
		}
		if evidence.contractID != challenge.contractID || evidence.providerID != challenge.providerID || evidence.providerVersion != challenge.providerVersion || evidence.evidenceSchema != challenge.evidenceSchema || evidence.sourceID != challenge.sourceID || evidence.origin != challenge.origin || evidence.operation != challenge.operation || evidence.databaseScope != challenge.databaseScope || evidence.accountID != challenge.accountID || evidence.pairID != challenge.pairID || evidence.attemptID != challenge.attemptID || evidence.pairGeneration != challenge.pairGeneration || evidence.fixtureGrantID != challenge.fixtureGrantID || evidence.claimedNativeSource {
			return authError(reasonInvalidEvidence)
		}
		if !ownerBindingEqual(evidence.owner, challenge.owner) {
			return authError(reasonOwnerDrift)
		}
		if evidence.frameID != challenge.frameID || evidence.requestID != challenge.requestID || !syntheticID(evidence.frameID) || !syntheticID(evidence.requestID) || evidence.provenance != fixtureProvenance || evidence.cacheState != fixtureCacheFresh {
			return authError(reasonInvalidEvidence)
		}
		if !evidence.authenticated || evidence.authentication != fixtureAuthentication {
			return authError(reasonUnknownAuth)
		}
		if evidence.identity != fixtureIdentity || evidence.accountID != a.config.enrollment.accountID {
			return authError(reasonUnknownIdentity)
		}
		if !evidence.continuityKnown || !syntheticID(evidence.sessionRevision) {
			return authError(reasonUnknownLineage)
		}
		if !evidence.coverageKnown {
			return authError(reasonUnknownLineage)
		}
		if evidence.coverageGap || !evidence.coverageContinuous {
			return authError(reasonCoverageGap)
		}
		if evidence.observedAt.IsZero() || evidence.observedAt.Before(challenge.acquisitionStarted) || evidence.observedAt.After(now) || (!lastObserved.IsZero() && evidence.observedAt.Before(lastObserved)) {
			return authError(reasonInvalidEvidence)
		}
		lastObserved = evidence.observedAt
		if evidence.coverageFrom.IsZero() || !evidence.coverageThrough.Equal(evidence.observedAt) || evidence.coverageFrom.After(evidence.coverageThrough) {
			return authError(reasonCoverageGap)
		}
		if checkpoints == nil {
			if !evidence.coverageFrom.Equal(challenge.acquisitionStarted) {
				return authError(reasonCoverageGap)
			}
		} else if !evidence.coverageFrom.Equal(checkpoints[r]) || !evidence.coverageFrom.Equal(challenge.previousCoverage) {
			return authError(reasonCoverageGap)
		}
		if !deadlineAfter(evidence.authValidUntil, now) || !deadlineAfter(evidence.identityValidUntil, now) || !deadlineAfter(evidence.coverageValidUntil, now) || !deadlineAfter(evidence.providerValidUntil, now) {
			return authError(reasonEvidenceExpired)
		}
		if evidence.authValidUntil.Before(evidence.observedAt) || evidence.identityValidUntil.Before(evidence.observedAt) || evidence.coverageValidUntil.Before(evidence.observedAt) || evidence.providerValidUntil.Before(evidence.observedAt) {
			return authError(reasonInvalidEvidence)
		}
	}
	if now.Sub(vector[readerRole].acquisitionStarted) >= maximumEvidenceAge {
		return authError(reasonEvidenceExpired)
	}
	return nil
}

func deadlineAfter(deadline, now time.Time) bool { return !deadline.IsZero() && now.Before(deadline) }

func earliestDeadline(evidenceAge time.Time, evidence ...roleEvidence) time.Time {
	deadline := evidenceAge
	for _, item := range evidence {
		for _, candidate := range []time.Time{item.authValidUntil, item.identityValidUntil, item.coverageValidUntil, item.providerValidUntil} {
			if candidate.Before(deadline) {
				deadline = candidate
			}
		}
	}
	return deadline
}

func (a *concurrentAuthorization) snapshotLocked() authorizationSnapshot {
	return authorizationSnapshot{
		config:            cloneConfig(a.config),
		fence:             a.fence,
		policyEnabled:     a.config.policyEnabled,
		derivationID:      derivationID(a.derivation),
		pendingRevocation: a.pendingRevocation,
	}
}

type authorizationSnapshot struct {
	config            authorizationConfig
	fence             uint64
	policyEnabled     bool
	derivationID      string
	pendingRevocation bool
}

func derivationID(receipt *derivationReceipt) string {
	if receipt == nil {
		return ""
	}
	return receipt.id
}

func (a *concurrentAuthorization) snapshotCurrentLocked(snapshot authorizationSnapshot) bool {
	return !a.pendingRevocation && !snapshot.pendingRevocation && !a.revoked && a.fence == snapshot.fence && a.config.policyEnabled == snapshot.policyEnabled && enrollmentEqual(a.config.enrollment, snapshot.config.enrollment) && pairEqual(a.config.pair, snapshot.config.pair) && ownerEqual(a.config.reader, snapshot.config.reader) && ownerEqual(a.config.collector, snapshot.config.collector) && derivationID(a.derivation) == snapshot.derivationID
}

func enrollmentEqual(a, b enrollment) bool {
	return a.contractID == b.contractID && a.providerID == b.providerID && a.providerVersion == b.providerVersion && a.evidenceSchema == b.evidenceSchema && a.sourceID == b.sourceID && a.databaseScope == b.databaseScope && a.accountID == b.accountID && a.readerProfileID == b.readerProfileID && a.policyGeneration == b.policyGeneration && a.holdGeneration == b.holdGeneration
}

func pairEqual(a, b pairIdentity) bool {
	return a.id == b.id && a.attemptID == b.attemptID && a.controllerEpochID == b.controllerEpochID && a.sidecarLifetimeID == b.sidecarLifetimeID && a.generation == b.generation
}

func (a *concurrentAuthorization) requireCurrentPermitLocked() error {
	if a.revoked || a.pendingRevocation || a.permit == nil || !a.config.policyEnabled {
		return authError(a.currentClosedReason())
	}
	now := a.lastClock
	if !now.Before(a.permit.expiresAt) {
		a.expireLocked()
		return authError(reasonEvidenceExpired)
	}
	if a.permit.fence != a.fence || a.permit.pairGeneration != a.config.pair.generation || !ownerEqual(a.permit.readerOwner, a.config.reader) || !ownerEqual(a.permit.collectorOwner, a.config.collector) {
		a.revokeLocked(reasonOwnerDrift)
		return authError(reasonOwnerDrift)
	}
	return nil
}

func (a *concurrentAuthorization) currentClosedReason() authorizationReason {
	if a.reason != "" {
		return a.reason
	}
	if !a.config.policyEnabled {
		return reasonDisabled
	}
	return reasonRevoked
}

func (a *concurrentAuthorization) dispatch(command workCommand) (workTicket, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, err := a.readClockLocked(); err != nil {
		return workTicket{}, err
	}
	if err := a.requireCurrentPermitLocked(); err != nil {
		return workTicket{}, err
	}
	if !a.grant.allowCollection || !syntheticID(command.id) || command.scope != a.config.enrollment.databaseScope || command.source != a.config.enrollment.sourceID || command.operation != "synthetic-x.collect" {
		return workTicket{}, authError(reasonUnsupportedOperation)
	}
	if err := a.validateChainLocked(a.permit.evidenceVersion, a.permit.evidenceVersion); err != nil {
		return workTicket{}, err
	}
	if !a.coverageAtBarrierLocked(a.lastClock) {
		return workTicket{}, authError(reasonCoverageGap)
	}
	if ticketExistsForCommand(a.tickets, command.id) {
		return workTicket{}, authError(reasonDuplicateResult)
	}
	ticketID := fmt.Sprintf("synthetic-ticket-%d", len(a.tickets)+1)
	ticket := workTicket{
		instanceID:       a.instanceID,
		ticketID:         ticketID,
		commandID:        command.id,
		scope:            command.scope,
		source:           command.source,
		operation:        command.operation,
		evidenceVersion:  a.permit.evidenceVersion,
		pairID:           a.config.pair.id,
		pairGeneration:   a.config.pair.generation,
		fence:            a.fence,
		collectorOwnerID: a.config.collector.ownerID,
		dispatchAt:       a.lastClock,
	}
	a.tickets[ticketID] = &ticketRecord{ticket: ticket, status: workPending}
	return ticket, nil
}

func ticketExistsForCommand(tickets map[string]*ticketRecord, commandID string) bool {
	for _, record := range tickets {
		if record.ticket.commandID == commandID {
			return true
		}
	}
	return false
}

func (a *concurrentAuthorization) claim(ticket workTicket) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, err := a.readClockLocked(); err != nil {
		return err
	}
	if err := a.requireCurrentPermitLocked(); err != nil {
		return err
	}
	if !a.coverageAtBarrierLocked(a.lastClock) {
		return authError(reasonCoverageGap)
	}
	record, err := a.ticketRecordLocked(ticket)
	if err != nil {
		return err
	}
	if record.status == workCancelled {
		return authError(reasonTicketCancelled)
	}
	if record.status != workPending {
		return authError(reasonStaleTicket)
	}
	if err := a.validateChainLocked(record.ticket.evidenceVersion, a.permit.evidenceVersion); err != nil {
		return err
	}
	record.status = workClaimed
	a.sideEffects++ // Synthetic fixture capture claim only.
	return nil
}

func (a *concurrentAuthorization) complete(ticket workTicket, result fixtureResult) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, err := a.readClockLocked(); err != nil {
		return err
	}
	if err := a.requireCurrentPermitLocked(); err != nil {
		return err
	}
	if !a.coverageAtBarrierLocked(a.lastClock) {
		return authError(reasonCoverageGap)
	}
	record, err := a.ticketRecordLocked(ticket)
	if err != nil {
		return err
	}
	if record.status == workCancelled {
		return authError(reasonTicketCancelled)
	}
	if record.status != workClaimed {
		if record.status == workCompleted || a.memorySink[record.ticket.ticketID].ticketID != "" {
			return authError(reasonDuplicateResult)
		}
		return authError(reasonTicketNotClaimed)
	}
	canonical := record.ticket
	if !a.grant.allowResultSink || !validResult(result, canonical) {
		return authError(reasonInvalidEvidence)
	}
	if canonical.fence != a.fence || canonical.pairGeneration != a.config.pair.generation || canonical.pairID != a.config.pair.id || canonical.collectorOwnerID != a.config.collector.ownerID {
		return authError(reasonStaleTicket)
	}
	if err := a.validateChainLocked(canonical.evidenceVersion, a.permit.evidenceVersion); err != nil {
		return err
	}
	if _, exists := a.memorySink[canonical.ticketID]; exists {
		return authError(reasonDuplicateResult)
	}
	// Result validation and memory-sink commit share this lock with revoke.
	a.memorySink[canonical.ticketID] = result
	record.status = workCompleted
	a.sideEffects++
	return nil
}

func validResult(result fixtureResult, ticket workTicket) bool {
	return syntheticID(result.id) && result.ticketID == ticket.ticketID && result.commandID == ticket.commandID && result.scope == ticket.scope && result.source == ticket.source && result.operation == ticket.operation && result.evidenceVersion == ticket.evidenceVersion && result.pairID == ticket.pairID && result.pairGeneration == ticket.pairGeneration && result.fence == ticket.fence && result.collectorOwnerID == ticket.collectorOwnerID
}

func (a *concurrentAuthorization) cancel(ticket workTicket) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, err := a.readClockLocked(); err != nil {
		return err
	}
	record, err := a.ticketRecordLocked(ticket)
	if err != nil {
		return err
	}
	if record.status == workCompleted {
		return authError(reasonDuplicateResult)
	}
	if record.status == workCancelled {
		return authError(reasonTicketCancelled)
	}
	record.status = workCancelled
	return nil
}

func (a *concurrentAuthorization) ticketRecordLocked(ticket workTicket) (*ticketRecord, error) {
	if ticket.instanceID != a.instanceID || !syntheticID(ticket.ticketID) {
		return nil, authError(reasonStaleTicket)
	}
	record := a.tickets[ticket.ticketID]
	if record == nil || !ticketEqual(record.ticket, ticket) {
		return nil, authError(reasonStaleTicket)
	}
	return record, nil
}

func ticketEqual(left, right workTicket) bool {
	return left.instanceID == right.instanceID && left.ticketID == right.ticketID && left.commandID == right.commandID && left.scope == right.scope && left.source == right.source && left.operation == right.operation && left.evidenceVersion == right.evidenceVersion && left.pairID == right.pairID && left.pairGeneration == right.pairGeneration && left.fence == right.fence && left.collectorOwnerID == right.collectorOwnerID && left.dispatchAt.Equal(right.dispatchAt)
}

func (a *concurrentAuthorization) validateChainLocked(fromVersion, toVersion uint64) error {
	if a.permit == nil || fromVersion == 0 || toVersion < fromVersion || toVersion != a.permit.evidenceVersion {
		return authError(reasonStaleTicket)
	}
	checkpoint, ok := a.versionCoverage[fromVersion]
	if !ok {
		return authError(reasonStaleTicket)
	}
	version := fromVersion
	for version < toVersion {
		link, ok := a.renewalLinks[version]
		next, checkpointOK := a.versionCoverage[version+1]
		if !ok || !checkpointOK || link.fromVersion != version || link.toVersion != version+1 || link.pairGeneration != a.config.pair.generation || link.readerRevision != a.permit.readerRevision || link.collectorRevision != a.permit.collectorRevision || link.collectorOwnerID != a.config.collector.ownerID || !link.readerCoverageFrom.Equal(checkpoint.reader) || !link.collectorCoverageFrom.Equal(checkpoint.collector) || !link.readerCoverageTo.Equal(next.reader) || !link.collectorCoverageTo.Equal(next.collector) || !link.readerCoverageTo.After(link.readerCoverageFrom) || !link.collectorCoverageTo.After(link.collectorCoverageFrom) {
			return authError(reasonCoverageGap)
		}
		checkpoint = next
		version = link.toVersion
	}
	return nil
}

// A validity deadline bounds how long a lease may live. It does not prove
// lifecycle coverage through a later claim or commit barrier.
func (a *concurrentAuthorization) coverageAtBarrierLocked(barrier time.Time) bool {
	if a.permit == nil {
		return false
	}
	return !a.coverageThrough[readerRole].Before(barrier) && !a.coverageThrough[collectorRole].Before(barrier) && !a.permit.readerCoverageThrough.Before(barrier) && !a.permit.collectorCoverageThrough.Before(barrier)
}

func (a *concurrentAuthorization) check() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, err := a.readClockLocked(); err != nil {
		return err
	}
	if a.permit == nil {
		if a.revoked {
			return authError(a.currentClosedReason())
		}
		return nil
	}
	return a.requireCurrentPermitLocked()
}

func (a *concurrentAuthorization) revoke() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.revokeLocked(reasonRevoked)
}

func (a *concurrentAuthorization) optOut() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.config.policyEnabled = false
	a.revokeLocked(reasonOptedOut)
}

func (a *concurrentAuthorization) policyChanged(policyGeneration, holdGeneration uint64, enabled bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.config.enrollment.policyGeneration = policyGeneration
	a.config.enrollment.holdGeneration = holdGeneration
	a.config.policyEnabled = enabled
	a.revokeLocked(reasonPolicyChanged)
}

func (a *concurrentAuthorization) ownerDrift(changedRole role) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if changedRole != readerRole && changedRole != collectorRole {
		return authError(reasonOwnerDrift)
	}
	a.revokeLocked(reasonOwnerDrift)
	return authError(reasonOwnerDrift)
}

func (a *concurrentAuthorization) readerLost() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.revokeLocked(reasonReaderLost)
}

// updateReaderWindows changes only mutable window inventory. The pinned reader
// process remains the same owner while at least one ordinary reader window is
// present; the last-window transition closes dependent authority.
func (a *concurrentAuthorization) updateReaderWindows(windows []string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, err := a.readClockLocked(); err != nil {
		return err
	}
	if len(windows) == 0 {
		a.revokeLocked(reasonReaderLost)
		return authError(reasonReaderLost)
	}
	for _, window := range windows {
		if !syntheticID(window) {
			return authError(reasonInvalidConfiguration)
		}
	}
	a.config.reader.windowInventory = append([]string(nil), windows...)
	if a.permit != nil {
		a.permit.readerOwner = cloneOwner(a.config.reader)
	}
	return nil
}

func (a *concurrentAuthorization) requestIndependentCollector() error {
	return authError(reasonIndependentCollector)
}

func (a *concurrentAuthorization) revokeLocked(reason authorizationReason) {
	if a.revoked && a.permit == nil {
		return
	}
	a.pendingRevocation = true
	a.fence++
	a.permit = nil
	a.revoked = true
	a.reason = reason
	a.pendingRevocation = false
	for _, record := range a.tickets {
		if record.status == workClaimed {
			record.status = workInvalidated
		}
	}
}

func (a *concurrentAuthorization) expireLocked() {
	a.revokeLocked(reasonEvidenceExpired)
}

func (a *concurrentAuthorization) readClockLocked() (time.Time, error) {
	now := a.clock()
	if now.IsZero() {
		a.revokeLocked(reasonClockRegression)
		return time.Time{}, authError(reasonClockRegression)
	}
	if now.Before(a.lastClock) {
		a.revokeLocked(reasonClockRegression)
		return time.Time{}, authError(reasonClockRegression)
	}
	a.lastClock = now
	if a.permit != nil && !now.Before(a.permit.expiresAt) {
		a.expireLocked()
		return now, authError(reasonEvidenceExpired)
	}
	return now, nil
}

func (a *concurrentAuthorization) view() authorizationView {
	a.mu.Lock()
	defer a.mu.Unlock()
	_, _ = a.readClockLocked()
	view := authorizationView{
		// These remain false even when this development fixture grants its
		// private, synthetic permit semantics.
		nativeQualified: false,
		canAuthorize:    false,
		policyEnabled:   a.config.policyEnabled,
		evidenceVersion: a.versionCounter,
		pairGeneration:  a.config.pair.generation,
		fence:           a.fence,
		reason:          a.reason,
	}
	if a.permit != nil {
		view.fixturePermitValid = true
		view.expiresAt = a.permit.expiresAt
	}
	return view
}

func (a *concurrentAuthorization) ticketStatus(ticket workTicket) (workStatus, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	_, clockErr := a.readClockLocked()
	record, err := a.ticketRecordLocked(ticket)
	if err != nil {
		return "", err
	}
	return record.status, clockErr
}

func (a *concurrentAuthorization) fixtureCounts() (sideEffects, committedResults int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return int(a.sideEffects), len(a.memorySink)
}

func errorReason(err error, fallback authorizationReason) authorizationReason {
	if err == nil {
		return fallback
	}
	var typed authorizationError
	if errors.As(err, &typed) && typed.reason != "" {
		return typed.reason
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return reasonContextCancelled
	}
	return fallback
}

func collectionErrorReason(err error) authorizationReason {
	if errors.Is(err, errBindingDeadline) {
		return reasonBindingDeadline
	}
	return errorReason(err, reasonRenewalFailed)
}
