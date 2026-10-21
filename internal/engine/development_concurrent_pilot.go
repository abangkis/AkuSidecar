//go:build auth_session_dev && auth_session_fixture

package engine

// This file defines an offline, synthetic-only model for the bounded X
// snapshot pilot. It is separate from concurrentAuthorization: it never grants
// native or Store authority and has no browser-operation API.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	concurrentPilotContract              = "aku.x.dev-pilot.snapshot.v1"
	concurrentPilotProvider              = "x-dev-pilot-snapshot"
	concurrentPilotSchema                = "x-dev-pilot-snapshot-evidence.v1"
	concurrentPilotMode                  = "snapshot"
	concurrentPilotPolicy                = "x-dev-pilot.snapshot.v1"
	concurrentPilotManifest              = "synthetic-reviewed-transfer-v1"
	concurrentPilotIdentitySource        = "synthetic-live-current-self.v1"
	concurrentPilotIdentityOrigin        = "https://x.com"
	concurrentPilotSettingsMethod        = "GET"
	concurrentPilotSettingsURL           = "https://api.x.com/1.1/account/settings.json"
	concurrentPilotSettingsRoute         = "api_v1.cookie-auth-rejection.2026-10-20"
	concurrentPilotSettingsQualification = "synthetic-pinned-settings-route.v1"
	concurrentPilotMaxAge                = 5 * time.Second
	concurrentPilotJointMax              = 2 * time.Second
	concurrentPilotMaxLife               = 60 * time.Second
)

type pilotReason string

const (
	pilotReasonDisabled          pilotReason = "admission-disabled"
	pilotReasonUnsupported       pilotReason = "unsupported-mode-or-policy"
	pilotReasonFixtureGrant      pilotReason = "fixture-grant-required"
	pilotReasonInvalidConfig     pilotReason = "invalid-configuration"
	pilotReasonMissingSeed       pilotReason = "derivation-required"
	pilotReasonSeedReplay        pilotReason = "derivation-replayed"
	pilotReasonSeedMismatch      pilotReason = "derivation-mismatch"
	pilotReasonEvidenceInvalid   pilotReason = "evidence-invalid"
	pilotReasonEvidenceExpired   pilotReason = "evidence-expired"
	pilotReasonJointDeadline     pilotReason = "joint-deadline-exceeded"
	pilotReasonClockRegression   pilotReason = "monotonic-clock-regressed"
	pilotReasonAttemptExpired    pilotReason = "attempt-expired"
	pilotReasonIdentityMissing   pilotReason = "identity-unavailable"
	pilotReasonAccountMismatch   pilotReason = "account-mismatch"
	pilotReasonAuthentication    pilotReason = "authentication-rejected"
	pilotReasonSourceUnavailable pilotReason = "source-unavailable"
	pilotReasonOwnerDrift        pilotReason = "owner-drift"
	pilotReasonPolicyChanged     pilotReason = "policy-changed"
	pilotReasonOptedOut          pilotReason = "opted-out"
	pilotReasonReaderLost        pilotReason = "reader-lost"
	pilotReasonParentExited      pilotReason = "parent-exited"
	pilotReasonChannelLost       pilotReason = "evidence-channel-lost"
	pilotReasonCookieMutation    pilotReason = "candidate-cookie-write"
	pilotReasonRenewalBusy       pilotReason = "renewal-already-running"
	pilotReasonConcurrentInvalid pilotReason = "authority-changed-during-renewal"
	pilotReasonStaleTicket       pilotReason = "stale-ticket"
	pilotReasonTicketNotClaimed  pilotReason = "ticket-not-claimed"
	pilotReasonTicketCancelled   pilotReason = "ticket-cancelled"
	pilotReasonDuplicateResult   pilotReason = "duplicate-result"
	pilotReasonDuplicateCommand  pilotReason = "duplicate-command"
	pilotReasonResultMismatch    pilotReason = "result-mismatch"
	pilotReasonUnsupportedEvent  pilotReason = "unsupported-event"
)

type pilotError struct{ reason pilotReason }

func (e pilotError) Error() string      { return string(e.reason) }
func pilotErr(reason pilotReason) error { return pilotError{reason: reason} }

func pilotErrorReason(err error, fallback pilotReason) pilotReason {
	if err == nil {
		return fallback
	}
	var typed pilotError
	if errors.As(err, &typed) && validPilotReason(typed.reason) {
		return typed.reason
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return pilotReasonSourceUnavailable
	}
	return fallback
}

func validPilotReason(reason pilotReason) bool {
	switch reason {
	case pilotReasonDisabled, pilotReasonUnsupported, pilotReasonFixtureGrant, pilotReasonInvalidConfig, pilotReasonMissingSeed, pilotReasonSeedReplay, pilotReasonSeedMismatch, pilotReasonEvidenceInvalid, pilotReasonEvidenceExpired, pilotReasonJointDeadline, pilotReasonClockRegression, pilotReasonAttemptExpired, pilotReasonIdentityMissing, pilotReasonAccountMismatch, pilotReasonAuthentication, pilotReasonSourceUnavailable, pilotReasonOwnerDrift, pilotReasonPolicyChanged, pilotReasonOptedOut, pilotReasonReaderLost, pilotReasonParentExited, pilotReasonChannelLost, pilotReasonCookieMutation, pilotReasonRenewalBusy, pilotReasonConcurrentInvalid, pilotReasonStaleTicket, pilotReasonTicketNotClaimed, pilotReasonTicketCancelled, pilotReasonDuplicateResult, pilotReasonDuplicateCommand, pilotReasonResultMismatch, pilotReasonUnsupportedEvent:
		return true
	default:
		return false
	}
}

type pilotRole string

const (
	pilotReaderRole    pilotRole = "reader"
	pilotCollectorRole pilotRole = "collector"
)

type pilotFixtureGrant struct {
	id                string
	allowOptIn        bool
	allowSeedReceipt  bool
	allowEvidence     bool
	allowCollection   bool
	allowResultCommit bool
}

type pilotPair struct {
	id                string
	attemptID         string
	controllerEpochID string
	sidecarLifetimeID string
	generation        uint64
}

// Owner identity is pinned separately from its tracked window inventory.
type pilotOwner struct {
	role              pilotRole
	ownerID           string
	profileID         string
	directoryID       string
	processID         string
	processCreationID string
	processImageID    string
	processStartedAt  time.Time
	targetID          string
	frameID           string
	documentID        string
	sidecarLifetimeID string
	contextID         string
	epochID           string
	generation        uint64
	driverID          string
	directoryPinned   bool
	profileFresh      bool
	disposableContext bool
	processAlive      bool
	windowInventory   []string
}

type pilotConfig struct {
	mode                   string
	contractID             string
	providerID             string
	evidenceSchema         string
	policyID               string
	policyEnabled          bool
	explicitOptIn          bool
	limitationAcknowledged bool
	expectedAccountID      string
	scopeID                string
	policyGeneration       uint64
	consentGeneration      uint64
	pair                   pilotPair
	reader                 pilotOwner
	collector              pilotOwner
}

// Receipt fields contain only synthetic identifiers and an authorization
// statement; cookie material is never represented in this model.
type pilotSeedReceipt struct {
	instanceID        string
	id                string
	contractID        string
	providerID        string
	evidenceSchema    string
	policyID          string
	policyGeneration  uint64
	consentGeneration uint64
	accountID         string
	pairID            string
	attemptID         string
	pairGeneration    uint64
	readerOwnerID     string
	collectorOwnerID  string
	fromRole          pilotRole
	toRole            pilotRole
	manifestID        string
	seedApplied       bool
	seededAt          time.Time
}

type pilotOperation string

const pilotIdentityAndPinnedSettings pilotOperation = "current-self+settings.pinned"

type pilotChallenge struct {
	instanceID        string
	id                string
	vectorID          string
	contractID        string
	providerID        string
	evidenceSchema    string
	policyID          string
	policyGeneration  uint64
	consentGeneration uint64
	accountID         string
	pair              pilotPair
	role              pilotRole
	owner             pilotOwner
	operation         pilotOperation
	identitySourceID  string
	identityOrigin    string
	settingsMethod    string
	settingsURL       string
	settingsRouteID   string
	settingsQualified string
	targetID          string
	frameID           string
	documentID        string
	requestID         string
	acquisitionStart  time.Time
	previousVersion   uint64
	fence             uint64
}

// Source output is accepted only when challenge, role, owner and source-quality
// markers match exactly. Global continuity is deliberately absent.
type pilotRoleEvidence struct {
	instanceID        string
	challengeID       string
	vectorID          string
	contractID        string
	providerID        string
	evidenceSchema    string
	policyID          string
	policyGeneration  uint64
	consentGeneration uint64
	accountID         string
	pair              pilotPair
	role              pilotRole
	owner             pilotOwner
	operation         pilotOperation
	identitySourceID  string
	identityOrigin    string
	settingsMethod    string
	settingsURL       string
	settingsRouteID   string
	settingsQualified string
	targetID          string
	frameID           string
	documentID        string
	requestID         string
	acquisitionStart  time.Time
	observedAt        time.Time
	validUntil        time.Time
	identityPresent   bool
	identityLive      bool
	settingsSuccess   bool
	authenticated     bool
	cacheFresh        bool
	serviceWorkerOK   bool
	notRedirected     bool
	wellFormed        bool
	correlated        bool
}

type pilotEvidenceSource func(context.Context, pilotChallenge) (pilotRoleEvidence, error)

type pilotPermit struct {
	evidenceVersion   uint64
	expiresAt         time.Time
	readerEvidence    pilotRoleEvidence
	collectorEvidence pilotRoleEvidence
	readerOwner       pilotOwner
	collectorOwner    pilotOwner
	pairGeneration    uint64
	fence             uint64
}

// Work tickets keep the collector identity and original evidence version even
// when later same-attempt checkpoints advance the local evidence chain.
type pilotWorkTicket struct {
	instanceID       string
	id               string
	commandID        string
	scopeID          string
	sourceID         string
	operation        string
	evidenceVersion  uint64
	pairID           string
	pairGeneration   uint64
	fence            uint64
	collectorOwnerID string
	collectorPID     string
	collectorCreate  string
	collectorDriver  string
	collectorContext string
	collectorEpoch   string
	collectorGen     uint64
	dispatchAt       time.Time
}

type pilotWorkStatus string

const (
	pilotWorkPending     pilotWorkStatus = "pending"
	pilotWorkClaimed     pilotWorkStatus = "claimed"
	pilotWorkCompleted   pilotWorkStatus = "completed"
	pilotWorkCancelled   pilotWorkStatus = "cancelled"
	pilotWorkInvalidated pilotWorkStatus = "invalidated"
)

type pilotWorkRecord struct {
	ticket pilotWorkTicket
	status pilotWorkStatus
}

type pilotResult struct {
	id              string
	ticketID        string
	commandID       string
	scopeID         string
	sourceID        string
	operation       string
	evidenceVersion uint64
	pairID          string
	pairGeneration  uint64
	fence           uint64
	collectorOwner  string
	content         string
}

type pilotEvent string

const (
	pilotEventUnclassifiedCookieWrite pilotEvent = "cookie_write_unqualified"
	pilotEventAuthTokenWrite          pilotEvent = "auth_token_write"
	pilotEventCT0Write                pilotEvent = "ct0_write"
	pilotEventChannelDetached         pilotEvent = "channel_detached"
	pilotEventChannelCrashed          pilotEvent = "channel_crashed"
	pilotEventChannelOverflow         pilotEvent = "channel_overflow"
	pilotEventCorrelationAmbiguous    pilotEvent = "correlation_ambiguous"
	pilotEventIdentityChanged         pilotEvent = "identity_changed"
	pilotEventAuthenticationRejected  pilotEvent = "authentication_rejected"
	pilotEventCleanupUncertain        pilotEvent = "cleanup_uncertain"
)

type pilotCounters struct {
	seedReceipts             uint64
	evidenceVectors          uint64
	unclassifiedCookieWrites uint64
	acceptedResults          uint64
	readerWindowOpens        uint64
	readerWindowCloses       uint64
}

type pilotCounterReport struct {
	SeedReceipts             uint64 `json:"seedReceipts"`
	EvidenceVectors          uint64 `json:"evidenceVectors"`
	UnclassifiedCookieWrites uint64 `json:"unclassifiedCookieWrites"`
	AcceptedResults          uint64 `json:"acceptedResults"`
	ReaderWindowOpens        uint64 `json:"readerWindowOpens"`
	ReaderWindowCloses       uint64 `json:"readerWindowCloses"`
}

// This fixed projection contains no account, owner, request, document or error
// text. Native/production/Store authority remains false in every state.
type concurrentPilotReport struct {
	ContractID           string             `json:"contractId"`
	Mode                 string             `json:"mode"`
	PolicyID             string             `json:"policyId"`
	EvidenceVersion      uint64             `json:"evidenceVersion"`
	PilotEligible        bool               `json:"pilotEligible"`
	AdmissionOpen        bool               `json:"admissionOpen"`
	CanAuthorize         bool               `json:"canAuthorize"`
	NativeQualified      bool               `json:"nativeQualified"`
	ProductionQualified  bool               `json:"productionQualified"`
	StorePermit          bool               `json:"storePermit"`
	SnapshotFreshness    string             `json:"snapshotFreshness"`
	GlobalCoverage       string             `json:"globalCoverage"`
	Continuity           string             `json:"continuity"`
	RefreshQualification string             `json:"refreshQualification"`
	AttemptExpired       bool               `json:"attemptExpired"`
	Revoked              bool               `json:"revoked"`
	Reason               string             `json:"reason"`
	Counters             pilotCounterReport `json:"counters"`
}

type pilotClock func() time.Time

var concurrentPilotInstanceSequence atomic.Uint64

type concurrentPilot struct {
	mu sync.Mutex

	instanceID   string
	config       pilotConfig
	grant        pilotFixtureGrant
	clock        pilotClock
	source       pilotEvidenceSource
	attemptStart time.Time
	attemptStop  time.Time
	lastClock    time.Time

	seedReceiptID string
	seedUsed      bool
	active        bool
	admission     bool
	revoked       bool
	reason        pilotReason
	permit        *pilotPermit
	version       uint64
	fence         uint64
	reader        pilotOwner
	collector     pilotOwner
	parentAlive   bool

	renewalID      uint64
	renewalSerial  uint64
	renewalCancel  context.CancelFunc
	challengeSeq   uint64
	vectorSeq      uint64
	workSeq        uint64
	work           map[string]*pilotWorkRecord
	commandTickets map[string]string
	requestIDs     map[string]struct{}
	results        map[string]pilotResult
	resultByTicket map[string]string
	windowOpen     map[string]struct{}
	counters       pilotCounters
}

func newConcurrentPilot(config pilotConfig, grant pilotFixtureGrant, clock pilotClock, source pilotEvidenceSource) (*concurrentPilot, error) {
	if !validPilotGrant(grant) {
		return nil, pilotErr(pilotReasonFixtureGrant)
	}
	if clock == nil || source == nil {
		return nil, pilotErr(pilotReasonInvalidConfig)
	}
	start := clock() // The absolute attempt cap starts before seed/evidence.
	if start.IsZero() {
		return nil, pilotErr(pilotReasonInvalidConfig)
	}
	config = clonePilotConfig(config)
	m := &concurrentPilot{
		instanceID:     fmt.Sprintf("synthetic-pilot-instance-%d", concurrentPilotInstanceSequence.Add(1)),
		config:         config,
		grant:          grant,
		clock:          clock,
		source:         source,
		attemptStart:   start,
		attemptStop:    start.Add(concurrentPilotMaxLife),
		lastClock:      start,
		reader:         clonePilotOwner(config.reader),
		collector:      clonePilotOwner(config.collector),
		parentAlive:    true,
		work:           make(map[string]*pilotWorkRecord),
		commandTickets: make(map[string]string),
		requestIDs:     make(map[string]struct{}),
		results:        make(map[string]pilotResult),
		resultByTicket: make(map[string]string),
		windowOpen:     make(map[string]struct{}),
	}
	for _, id := range m.reader.windowInventory {
		m.windowOpen[id] = struct{}{}
	}
	return m, nil
}

func validPilotGrant(grant pilotFixtureGrant) bool {
	return syntheticPilotID(grant.id) && grant.allowOptIn && grant.allowSeedReceipt && grant.allowEvidence && grant.allowCollection && grant.allowResultCommit
}

func syntheticPilotID(value string) bool {
	if !strings.HasPrefix(value, "synthetic-") || len(value) <= len("synthetic-") || len(value) > 128 {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '_' || char == '.' || char == ':' {
			continue
		}
		return false
	}
	return true
}

func clonePilotConfig(config pilotConfig) pilotConfig {
	config.reader = clonePilotOwner(config.reader)
	config.collector = clonePilotOwner(config.collector)
	return config
}

func clonePilotOwner(owner pilotOwner) pilotOwner {
	owner.windowInventory = append([]string(nil), owner.windowInventory...)
	return owner
}

func clonePilotPair(pair pilotPair) pilotPair { return pair }

func (m *concurrentPilot) enable(ctx context.Context, receipt pilotSeedReceipt) error {
	m.mu.Lock()
	now, err := m.readClockLocked()
	if err != nil {
		m.mu.Unlock()
		return err
	}
	if m.seedUsed {
		m.mu.Unlock()
		return pilotErr(pilotReasonSeedReplay)
	}
	if receipt.id == "" {
		m.mu.Unlock()
		return pilotErr(pilotReasonMissingSeed)
	}
	if m.active || m.admission || m.revoked {
		m.mu.Unlock()
		return pilotErr(pilotReasonUnsupported)
	}
	if !m.parentAlive || !m.reader.processAlive || !m.collector.processAlive {
		m.mu.Unlock()
		return pilotErr(pilotReasonOwnerDrift)
	}
	if err := m.validateActivationLocked(receipt, now); err != nil {
		m.mu.Unlock()
		return err
	}
	m.seedUsed = true
	m.seedReceiptID = receipt.id
	m.counters.seedReceipts++
	m.active = true
	m.reason = ""
	m.mu.Unlock()
	return m.renewVector(ctx, true)
}

func (m *concurrentPilot) validateActivationLocked(receipt pilotSeedReceipt, now time.Time) error {
	c := m.config
	if !c.policyEnabled || !c.explicitOptIn || !c.limitationAcknowledged {
		return pilotErr(pilotReasonDisabled)
	}
	if c.mode != concurrentPilotMode || c.contractID != concurrentPilotContract || c.providerID != concurrentPilotProvider || c.evidenceSchema != concurrentPilotSchema || c.policyID != concurrentPilotPolicy {
		return pilotErr(pilotReasonUnsupported)
	}
	if c.policyGeneration == 0 || c.consentGeneration == 0 || !syntheticPilotID(c.expectedAccountID) || !syntheticPilotID(c.scopeID) || !validPilotPair(c.pair) || !validPilotOwner(m.reader, pilotReaderRole) || !validPilotOwner(m.collector, pilotCollectorRole) {
		return pilotErr(pilotReasonInvalidConfig)
	}
	if m.reader.ownerID == m.collector.ownerID || m.reader.profileID == m.collector.profileID || m.reader.directoryID == m.collector.directoryID || m.reader.processID == m.collector.processID || m.reader.contextID == m.collector.contextID {
		return pilotErr(pilotReasonInvalidConfig)
	}
	if len(m.reader.windowInventory) == 0 || len(m.collector.windowInventory) == 0 || m.reader.sidecarLifetimeID != c.pair.sidecarLifetimeID || m.collector.sidecarLifetimeID != c.pair.sidecarLifetimeID || m.reader.epochID != c.pair.controllerEpochID || m.collector.epochID != c.pair.controllerEpochID {
		return pilotErr(pilotReasonInvalidConfig)
	}
	if m.seedUsed {
		return pilotErr(pilotReasonSeedReplay)
	}
	if !validPilotReceipt(receipt, m.instanceID, c, m.reader, m.collector, m.attemptStart, m.attemptStop, now) {
		return pilotErr(pilotReasonSeedMismatch)
	}
	return nil
}

func validPilotPair(pair pilotPair) bool {
	return syntheticPilotID(pair.id) && syntheticPilotID(pair.attemptID) && syntheticPilotID(pair.controllerEpochID) && syntheticPilotID(pair.sidecarLifetimeID) && pair.generation > 0
}

func validPilotOwner(owner pilotOwner, want pilotRole) bool {
	if owner.role != want || !syntheticPilotID(owner.ownerID) || !syntheticPilotID(owner.profileID) || !syntheticPilotID(owner.directoryID) || !syntheticPilotID(owner.processID) || !syntheticPilotID(owner.processCreationID) || !syntheticPilotID(owner.processImageID) || owner.processStartedAt.IsZero() || !syntheticPilotID(owner.targetID) || !syntheticPilotID(owner.frameID) || !syntheticPilotID(owner.documentID) || !syntheticPilotID(owner.sidecarLifetimeID) || !syntheticPilotID(owner.contextID) || !syntheticPilotID(owner.epochID) || owner.generation == 0 || !syntheticPilotID(owner.driverID) || !owner.directoryPinned || !owner.processAlive || len(owner.windowInventory) == 0 || (want == pilotCollectorRole && (!owner.profileFresh || !owner.disposableContext)) {
		return false
	}
	seen := make(map[string]struct{}, len(owner.windowInventory))
	for _, window := range owner.windowInventory {
		if !syntheticPilotID(window) {
			return false
		}
		if _, duplicate := seen[window]; duplicate {
			return false
		}
		seen[window] = struct{}{}
	}
	return true
}

func validPilotReceipt(receipt pilotSeedReceipt, instanceID string, config pilotConfig, reader, collector pilotOwner, start, stop, now time.Time) bool {
	return receipt.instanceID == instanceID && syntheticPilotID(receipt.id) && receipt.contractID == concurrentPilotContract && receipt.providerID == concurrentPilotProvider && receipt.evidenceSchema == concurrentPilotSchema && receipt.policyID == concurrentPilotPolicy && receipt.policyGeneration == config.policyGeneration && receipt.consentGeneration == config.consentGeneration && receipt.accountID == config.expectedAccountID && receipt.pairID == config.pair.id && receipt.attemptID == config.pair.attemptID && receipt.pairGeneration == config.pair.generation && receipt.readerOwnerID == reader.ownerID && receipt.collectorOwnerID == collector.ownerID && receipt.fromRole == pilotReaderRole && receipt.toRole == pilotCollectorRole && receipt.manifestID == concurrentPilotManifest && receipt.seedApplied && !receipt.seededAt.Before(start) && !receipt.seededAt.After(now) && receipt.seededAt.Before(stop)
}

func (m *concurrentPilot) renew(ctx context.Context) error { return m.renewVector(ctx, false) }

func (m *concurrentPilot) renewVector(parent context.Context, initial bool) error {
	return m.renewVectorWithHook(parent, initial, nil)
}

// The hook is a package-private test seam for blocking publication after all
// source calls complete; normal pilot calls always pass nil.
func (m *concurrentPilot) renewVectorWithHook(parent context.Context, initial bool, beforePublication func()) error {
	if parent == nil {
		parent = context.Background()
	}
	m.mu.Lock()
	now, clockErr := m.readClockLocked()
	if clockErr != nil {
		m.mu.Unlock()
		return clockErr
	}
	if !m.active || m.revoked || !m.parentAlive {
		reason := m.reasonOr(pilotReasonDisabled)
		m.mu.Unlock()
		return pilotErr(reason)
	}
	if m.renewalID != 0 {
		m.mu.Unlock()
		return pilotErr(pilotReasonRenewalBusy)
	}
	if !initial && (m.permit == nil || !m.admission) {
		m.revokeLocked(pilotReasonEvidenceExpired)
		m.mu.Unlock()
		return pilotErr(pilotReasonEvidenceExpired)
	}
	if initial && m.permit != nil {
		m.mu.Unlock()
		return pilotErr(pilotReasonUnsupported)
	}
	start := now
	if !initial && !m.permit.expiresAt.After(now) {
		m.revokeLocked(pilotReasonEvidenceExpired)
		m.mu.Unlock()
		return pilotErr(pilotReasonEvidenceExpired)
	}
	m.renewalSerial++
	renewalID := m.renewalSerial
	m.renewalID = renewalID
	ctx, cancel := context.WithTimeout(parent, concurrentPilotJointMax)
	defer cancel()
	m.renewalCancel = cancel
	m.vectorSeq++
	vectorID := fmt.Sprintf("synthetic-pilot-vector-%d", m.vectorSeq)
	readerChallenge := m.newChallengeLocked(vectorID, pilotReaderRole, m.reader, start)
	collectorChallenge := m.newChallengeLocked(vectorID, pilotCollectorRole, m.collector, start)
	snapshot := m.snapshotLocked()
	m.mu.Unlock()

	readerEvidence, readerErr := m.source(ctx, clonePilotChallenge(readerChallenge))
	collectorEvidence := pilotRoleEvidence{}
	var collectorErr error
	if readerErr == nil {
		collectorEvidence, collectorErr = m.source(ctx, clonePilotChallenge(collectorChallenge))
	} else {
		collectorErr = readerErr
	}
	if beforePublication != nil {
		beforePublication()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.renewalID != renewalID || m.revoked || !m.active || !m.parentAlive || m.fence != snapshot.fence || !pilotOwnerBindingEqual(m.reader, snapshot.reader) || !pilotOwnersEqual(m.collector, snapshot.collector) || m.config.policyGeneration != snapshot.policyGeneration || m.config.consentGeneration != snapshot.consentGeneration || m.config.policyEnabled != snapshot.policyEnabled || m.permitVersionLocked() != snapshot.permitVersion {
		if m.renewalID == renewalID && !m.revoked {
			m.revokeLocked(pilotReasonConcurrentInvalid)
		}
		return pilotErr(pilotReasonConcurrentInvalid)
	}
	// Check cancellation after acquiring the publication lock, so cancellation
	// that occurred while a caller waited cannot publish a late permit.
	if ctx.Err() != nil || parent.Err() != nil {
		m.revokeLocked(pilotReasonSourceUnavailable)
		return pilotErr(m.reason)
	}
	m.renewalID = 0
	m.renewalCancel = nil
	finish, err := m.readClockLocked()
	if err != nil {
		return err
	}
	if readerErr != nil || collectorErr != nil {
		m.revokeLocked(pilotReasonSourceUnavailable)
		return pilotErr(m.reason)
	}
	if finish.Sub(start) < 0 || finish.Sub(start) >= concurrentPilotJointMax {
		m.revokeLocked(pilotReasonJointDeadline)
		return pilotErr(m.reason)
	}
	if err := validatePilotEvidence(readerChallenge, readerEvidence, finish, m.config.expectedAccountID); err != nil {
		m.revokeLocked(pilotErrorReason(err, pilotReasonEvidenceInvalid))
		return pilotErr(m.reason)
	}
	if err := validatePilotEvidence(collectorChallenge, collectorEvidence, finish, m.config.expectedAccountID); err != nil {
		m.revokeLocked(pilotErrorReason(err, pilotReasonEvidenceInvalid))
		return pilotErr(m.reason)
	}
	if readerEvidence.accountID != collectorEvidence.accountID {
		m.revokeLocked(pilotReasonAccountMismatch)
		return pilotErr(m.reason)
	}
	expires := earliestPilotDeadline(
		readerEvidence.validUntil,
		collectorEvidence.validUntil,
		readerChallenge.acquisitionStart.Add(concurrentPilotMaxAge),
		collectorChallenge.acquisitionStart.Add(concurrentPilotMaxAge),
		readerEvidence.observedAt.Add(concurrentPilotMaxAge),
		collectorEvidence.observedAt.Add(concurrentPilotMaxAge),
		m.attemptStop,
	)
	if !expires.After(finish) {
		m.revokeLocked(pilotReasonEvidenceExpired)
		return pilotErr(m.reason)
	}
	readerRequestKey := pilotRequestUseKey(pilotReaderRole, readerChallenge.owner, readerEvidence.requestID)
	collectorRequestKey := pilotRequestUseKey(pilotCollectorRole, collectorChallenge.owner, collectorEvidence.requestID)
	if _, used := m.requestIDs[readerRequestKey]; used {
		m.revokeLocked(pilotReasonEvidenceInvalid)
		return pilotErr(m.reason)
	}
	if _, used := m.requestIDs[collectorRequestKey]; used {
		m.revokeLocked(pilotReasonEvidenceInvalid)
		return pilotErr(m.reason)
	}
	// One more cancellation check is the publication linearization guard.
	if ctx.Err() != nil || parent.Err() != nil {
		m.revokeLocked(pilotReasonSourceUnavailable)
		return pilotErr(m.reason)
	}
	m.requestIDs[readerRequestKey] = struct{}{}
	m.requestIDs[collectorRequestKey] = struct{}{}
	m.version++
	m.counters.evidenceVectors++
	m.permit = &pilotPermit{
		evidenceVersion:   m.version,
		expiresAt:         expires,
		readerEvidence:    clonePilotEvidence(readerEvidence),
		collectorEvidence: clonePilotEvidence(collectorEvidence),
		readerOwner:       clonePilotOwner(m.reader),
		collectorOwner:    clonePilotOwner(m.collector),
		pairGeneration:    m.config.pair.generation,
		fence:             m.fence,
	}
	m.admission = true
	m.reason = ""
	return nil
}

type pilotRenewalSnapshot struct {
	fence             uint64
	permitVersion     uint64
	policyGeneration  uint64
	consentGeneration uint64
	policyEnabled     bool
	reader            pilotOwner
	collector         pilotOwner
}

func (m *concurrentPilot) snapshotLocked() pilotRenewalSnapshot {
	return pilotRenewalSnapshot{
		fence:             m.fence,
		permitVersion:     m.permitVersionLocked(),
		policyGeneration:  m.config.policyGeneration,
		consentGeneration: m.config.consentGeneration,
		policyEnabled:     m.config.policyEnabled,
		reader:            clonePilotOwner(m.reader),
		collector:         clonePilotOwner(m.collector),
	}
}

func (m *concurrentPilot) newChallengeLocked(vectorID string, role pilotRole, owner pilotOwner, start time.Time) pilotChallenge {
	m.challengeSeq++
	return pilotChallenge{
		instanceID:        m.instanceID,
		id:                fmt.Sprintf("synthetic-pilot-challenge-%d", m.challengeSeq),
		vectorID:          vectorID,
		contractID:        concurrentPilotContract,
		providerID:        concurrentPilotProvider,
		evidenceSchema:    concurrentPilotSchema,
		policyID:          concurrentPilotPolicy,
		policyGeneration:  m.config.policyGeneration,
		consentGeneration: m.config.consentGeneration,
		accountID:         m.config.expectedAccountID,
		pair:              clonePilotPair(m.config.pair),
		role:              role,
		owner:             clonePilotOwner(owner),
		operation:         pilotIdentityAndPinnedSettings,
		identitySourceID:  concurrentPilotIdentitySource,
		identityOrigin:    concurrentPilotIdentityOrigin,
		settingsMethod:    concurrentPilotSettingsMethod,
		settingsURL:       concurrentPilotSettingsURL,
		settingsRouteID:   concurrentPilotSettingsRoute,
		settingsQualified: concurrentPilotSettingsQualification,
		targetID:          owner.targetID,
		frameID:           owner.frameID,
		documentID:        owner.documentID,
		requestID:         fmt.Sprintf("synthetic-pilot-request-%d", m.vectorSeq),
		acquisitionStart:  start,
		previousVersion:   m.permitVersionLocked(),
		fence:             m.fence,
	}
}

func clonePilotChallenge(challenge pilotChallenge) pilotChallenge {
	challenge.owner = clonePilotOwner(challenge.owner)
	challenge.pair = clonePilotPair(challenge.pair)
	return challenge
}

func readerErrOrCollector(readerErr, collectorErr error) error {
	if readerErr != nil {
		return readerErr
	}
	return collectorErr
}

func validatePilotEvidence(challenge pilotChallenge, evidence pilotRoleEvidence, now time.Time, expectedAccount string) error {
	if evidence.instanceID != challenge.instanceID || evidence.challengeID != challenge.id || evidence.vectorID != challenge.vectorID || evidence.contractID != concurrentPilotContract || evidence.providerID != concurrentPilotProvider || evidence.evidenceSchema != concurrentPilotSchema || evidence.policyID != challenge.policyID || evidence.policyGeneration != challenge.policyGeneration || evidence.consentGeneration != challenge.consentGeneration || !pilotPairEqual(evidence.pair, challenge.pair) || evidence.role != challenge.role || !pilotOwnersEqual(evidence.owner, challenge.owner) || evidence.operation != pilotIdentityAndPinnedSettings || evidence.identitySourceID != challenge.identitySourceID || evidence.identityOrigin != challenge.identityOrigin || evidence.settingsMethod != challenge.settingsMethod || evidence.settingsURL != challenge.settingsURL || evidence.settingsRouteID != challenge.settingsRouteID || evidence.settingsQualified != challenge.settingsQualified || evidence.targetID != challenge.targetID || evidence.frameID != challenge.frameID || evidence.documentID != challenge.documentID || evidence.requestID != challenge.requestID || evidence.acquisitionStart != challenge.acquisitionStart || evidence.observedAt.Before(challenge.acquisitionStart) || evidence.observedAt.After(now) || evidence.observedAt.IsZero() || !evidence.validUntil.After(evidence.observedAt) {
		return pilotErr(pilotReasonEvidenceInvalid)
	}
	if !evidence.identityPresent || !evidence.identityLive {
		return pilotErr(pilotReasonIdentityMissing)
	}
	if evidence.accountID != expectedAccount {
		return pilotErr(pilotReasonAccountMismatch)
	}
	if !evidence.authenticated || !evidence.settingsSuccess {
		return pilotErr(pilotReasonAuthentication)
	}
	if !evidence.cacheFresh || !evidence.serviceWorkerOK || !evidence.notRedirected || !evidence.wellFormed || !evidence.correlated {
		return pilotErr(pilotReasonEvidenceInvalid)
	}
	if age := now.Sub(evidence.observedAt); age < 0 || age > concurrentPilotMaxAge {
		return pilotErr(pilotReasonEvidenceExpired)
	}
	if !evidence.validUntil.After(now) {
		return pilotErr(pilotReasonEvidenceExpired)
	}
	return nil
}

func clonePilotEvidence(evidence pilotRoleEvidence) pilotRoleEvidence {
	evidence.owner = clonePilotOwner(evidence.owner)
	return evidence
}

func earliestPilotDeadline(deadlines ...time.Time) time.Time {
	var earliest time.Time
	for _, deadline := range deadlines {
		if deadline.IsZero() {
			continue
		}
		if earliest.IsZero() || deadline.Before(earliest) {
			earliest = deadline
		}
	}
	return earliest
}

func pilotPairEqual(a, b pilotPair) bool {
	return a.id == b.id && a.attemptID == b.attemptID && a.controllerEpochID == b.controllerEpochID && a.sidecarLifetimeID == b.sidecarLifetimeID && a.generation == b.generation
}

func pilotOwnersEqual(a, b pilotOwner) bool {
	if a.role != b.role || a.ownerID != b.ownerID || a.profileID != b.profileID || a.directoryID != b.directoryID || a.processID != b.processID || a.processCreationID != b.processCreationID || a.processImageID != b.processImageID || !a.processStartedAt.Equal(b.processStartedAt) || a.targetID != b.targetID || a.frameID != b.frameID || a.documentID != b.documentID || a.sidecarLifetimeID != b.sidecarLifetimeID || a.contextID != b.contextID || a.epochID != b.epochID || a.generation != b.generation || a.driverID != b.driverID || a.directoryPinned != b.directoryPinned || a.profileFresh != b.profileFresh || a.disposableContext != b.disposableContext || a.processAlive != b.processAlive || len(a.windowInventory) != len(b.windowInventory) {
		return false
	}
	for i := range a.windowInventory {
		if a.windowInventory[i] != b.windowInventory[i] {
			return false
		}
	}
	return true
}

func pilotOwnerBindingEqual(a, b pilotOwner) bool {
	a.windowInventory = nil
	b.windowInventory = nil
	return pilotOwnersEqual(a, b)
}

func pilotRequestUseKey(role pilotRole, owner pilotOwner, requestID string) string {
	return string(role) + "\x00" + owner.ownerID + "\x00" + owner.contextID + "\x00" + requestID
}

func (m *concurrentPilot) dispatch(commandID string) (pilotWorkTicket, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.checkCurrentLocked(); err != nil {
		return pilotWorkTicket{}, err
	}
	if !m.grant.allowCollection || !syntheticPilotID(commandID) {
		return pilotWorkTicket{}, pilotErr(pilotReasonInvalidConfig)
	}
	if _, duplicate := m.commandTickets[commandID]; duplicate {
		return pilotWorkTicket{}, pilotErr(pilotReasonDuplicateCommand)
	}
	m.workSeq++
	ticket := pilotWorkTicket{
		instanceID:       m.instanceID,
		id:               fmt.Sprintf("synthetic-pilot-ticket-%d", m.workSeq),
		commandID:        commandID,
		scopeID:          m.config.scopeID,
		sourceID:         concurrentPilotProvider,
		operation:        "snapshot.collect",
		evidenceVersion:  m.permit.evidenceVersion,
		pairID:           m.config.pair.id,
		pairGeneration:   m.config.pair.generation,
		fence:            m.fence,
		collectorOwnerID: m.collector.ownerID,
		collectorPID:     m.collector.processID,
		collectorCreate:  m.collector.processCreationID,
		collectorDriver:  m.collector.driverID,
		collectorContext: m.collector.contextID,
		collectorEpoch:   m.collector.epochID,
		collectorGen:     m.collector.generation,
		dispatchAt:       m.lastClock,
	}
	m.work[ticket.id] = &pilotWorkRecord{ticket: ticket, status: pilotWorkPending}
	m.commandTickets[commandID] = ticket.id
	return ticket, nil
}

func (m *concurrentPilot) claim(ticket pilotWorkTicket) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.checkCurrentLocked(); err != nil {
		return err
	}
	record, err := m.ticketRecordLocked(ticket)
	if err != nil {
		return err
	}
	if record.status == pilotWorkCancelled {
		return pilotErr(pilotReasonTicketCancelled)
	}
	if record.status != pilotWorkPending || ticket.evidenceVersion > m.permit.evidenceVersion {
		return pilotErr(pilotReasonStaleTicket)
	}
	record.status = pilotWorkClaimed
	return nil
}

func (m *concurrentPilot) cancel(ticket pilotWorkTicket) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	record, err := m.ticketRecordLocked(ticket)
	if err != nil {
		return err
	}
	if record.status == pilotWorkCompleted {
		return pilotErr(pilotReasonDuplicateResult)
	}
	if record.status == pilotWorkInvalidated {
		return pilotErr(pilotReasonStaleTicket)
	}
	record.status = pilotWorkCancelled
	return nil
}

func (m *concurrentPilot) acceptResult(ticket pilotWorkTicket, result pilotResult) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.checkCurrentLocked(); err != nil {
		return err
	}
	record, err := m.ticketRecordLocked(ticket)
	if err != nil {
		return err
	}
	if record.status == pilotWorkCancelled {
		return pilotErr(pilotReasonTicketCancelled)
	}
	if record.status == pilotWorkCompleted {
		return pilotErr(pilotReasonDuplicateResult)
	}
	if record.status != pilotWorkClaimed {
		return pilotErr(pilotReasonTicketNotClaimed)
	}
	if ticket.evidenceVersion > m.permit.evidenceVersion || !m.resultMatchesTicket(result, ticket) {
		return pilotErr(pilotReasonResultMismatch)
	}
	if _, exists := m.results[result.id]; exists {
		return pilotErr(pilotReasonDuplicateResult)
	}
	if _, exists := m.resultByTicket[ticket.id]; exists {
		return pilotErr(pilotReasonDuplicateResult)
	}
	// Validation and the memory-sink commit share this lock with every revoke,
	// cancellation and owner/policy transition, so a late result cannot win.
	m.results[result.id] = result
	m.resultByTicket[ticket.id] = result.id
	record.status = pilotWorkCompleted
	m.counters.acceptedResults++
	return nil
}

func (m *concurrentPilot) resultMatchesTicket(result pilotResult, ticket pilotWorkTicket) bool {
	return syntheticPilotID(result.id) && result.ticketID == ticket.id && result.commandID == ticket.commandID && result.scopeID == ticket.scopeID && result.sourceID == ticket.sourceID && result.operation == ticket.operation && result.evidenceVersion == ticket.evidenceVersion && result.pairID == ticket.pairID && result.pairGeneration == ticket.pairGeneration && result.fence == ticket.fence && result.collectorOwner == ticket.collectorOwnerID && len(result.content) <= 16*1024
}

func (m *concurrentPilot) ticketRecordLocked(ticket pilotWorkTicket) (*pilotWorkRecord, error) {
	record, ok := m.work[ticket.id]
	if !ok || ticket.instanceID != m.instanceID || !pilotTicketsEqual(ticket, record.ticket) {
		return nil, pilotErr(pilotReasonStaleTicket)
	}
	return record, nil
}

func pilotTicketsEqual(a, b pilotWorkTicket) bool {
	return a.instanceID == b.instanceID && a.id == b.id && a.commandID == b.commandID && a.scopeID == b.scopeID && a.sourceID == b.sourceID && a.operation == b.operation && a.evidenceVersion == b.evidenceVersion && a.pairID == b.pairID && a.pairGeneration == b.pairGeneration && a.fence == b.fence && a.collectorOwnerID == b.collectorOwnerID && a.collectorPID == b.collectorPID && a.collectorCreate == b.collectorCreate && a.collectorDriver == b.collectorDriver && a.collectorContext == b.collectorContext && a.collectorEpoch == b.collectorEpoch && a.collectorGen == b.collectorGen && a.dispatchAt.Equal(b.dispatchAt)
}

func (m *concurrentPilot) resultCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.results)
}

func (m *concurrentPilot) ticketStatus(ticket pilotWorkTicket) (pilotWorkStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	record, err := m.ticketRecordLocked(ticket)
	if err != nil {
		return "", err
	}
	return record.status, nil
}

func (m *concurrentPilot) observeEvent(event pilotEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := m.readClockLocked(); err != nil {
		return err
	}
	switch event {
	case pilotEventUnclassifiedCookieWrite:
		m.counters.unclassifiedCookieWrites++
		return nil
	case pilotEventAuthTokenWrite, pilotEventCT0Write:
		m.revokeLocked(pilotReasonCookieMutation)
		return pilotErr(m.reason)
	case pilotEventChannelDetached, pilotEventChannelCrashed, pilotEventChannelOverflow, pilotEventCorrelationAmbiguous:
		m.revokeLocked(pilotReasonChannelLost)
		return pilotErr(m.reason)
	case pilotEventIdentityChanged:
		m.revokeLocked(pilotReasonAccountMismatch)
		return pilotErr(m.reason)
	case pilotEventAuthenticationRejected:
		m.revokeLocked(pilotReasonAuthentication)
		return pilotErr(m.reason)
	case pilotEventCleanupUncertain:
		m.revokeLocked(pilotReasonOwnerDrift)
		return pilotErr(m.reason)
	default:
		return pilotErr(pilotReasonUnsupportedEvent)
	}
}

func (m *concurrentPilot) windowOpened(windowID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := m.readClockLocked(); err != nil {
		return err
	}
	if !m.active || m.revoked || !m.admission || !syntheticPilotID(windowID) {
		return pilotErr(pilotReasonDisabled)
	}
	if _, exists := m.windowOpen[windowID]; exists {
		return nil
	}
	m.windowOpen[windowID] = struct{}{}
	m.reader.windowInventory = append(m.reader.windowInventory, windowID)
	m.counters.readerWindowOpens++
	return nil
}

func (m *concurrentPilot) windowClosed(windowID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := m.readClockLocked(); err != nil {
		return err
	}
	if _, exists := m.windowOpen[windowID]; !exists {
		return nil
	}
	delete(m.windowOpen, windowID)
	m.reader.windowInventory = removePilotWindow(m.reader.windowInventory, windowID)
	m.counters.readerWindowCloses++
	if len(m.windowOpen) == 0 {
		m.reader.processAlive = false
		m.revokeLocked(pilotReasonReaderLost)
		return pilotErr(m.reason)
	}
	return nil
}

func removePilotWindow(windows []string, target string) []string {
	for i, window := range windows {
		if window == target {
			return append(windows[:i], windows[i+1:]...)
		}
	}
	return windows
}

func (m *concurrentPilot) optOut() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config.explicitOptIn = false
	m.config.policyEnabled = false
	m.retireCollectorLocked()
	m.revokeLocked(pilotReasonOptedOut)
}

func (m *concurrentPilot) policyChanged(policyGeneration, consentGeneration uint64, enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config.policyGeneration = policyGeneration
	m.config.consentGeneration = consentGeneration
	m.config.policyEnabled = enabled
	m.revokeLocked(pilotReasonPolicyChanged)
}

func (m *concurrentPilot) ownerChanged(role pilotRole, owner pilotOwner) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if role == pilotReaderRole {
		m.reader = clonePilotOwner(owner)
	} else if role == pilotCollectorRole {
		m.collector = clonePilotOwner(owner)
	}
	m.revokeLocked(pilotReasonOwnerDrift)
}

func (m *concurrentPilot) parentExit() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.parentAlive = false
	m.reader.processAlive = false
	m.collector.processAlive = false
	m.reader.windowInventory = nil
	m.collector.windowInventory = nil
	clear(m.windowOpen)
	m.revokeLocked(pilotReasonParentExited)
}

func (m *concurrentPilot) retireCollectorLocked() {
	m.collector.processAlive = false
	m.collector.windowInventory = nil
}

func (m *concurrentPilot) checkCurrentLocked() error {
	now, err := m.readClockLocked()
	if err != nil {
		return err
	}
	if !m.active || !m.admission || m.revoked || m.permit == nil {
		return pilotErr(m.reasonOr(pilotReasonDisabled))
	}
	if !now.Before(m.attemptStop) {
		m.revokeLocked(pilotReasonAttemptExpired)
		return pilotErr(m.reason)
	}
	if !m.permit.expiresAt.After(now) {
		m.revokeLocked(pilotReasonEvidenceExpired)
		return pilotErr(m.reason)
	}
	if !m.reader.processAlive || !m.collector.processAlive || len(m.windowOpen) == 0 || !pilotOwnerBindingEqual(m.permit.readerOwner, m.reader) || !pilotOwnersEqual(m.permit.collectorOwner, m.collector) {
		m.revokeLocked(pilotReasonOwnerDrift)
		return pilotErr(m.reason)
	}
	if !pilotEvidenceFresh(m.permit.readerEvidence, now) || !pilotEvidenceFresh(m.permit.collectorEvidence, now) {
		m.revokeLocked(pilotReasonEvidenceExpired)
		return pilotErr(m.reason)
	}
	return nil
}

func pilotEvidenceFresh(evidence pilotRoleEvidence, now time.Time) bool {
	age := now.Sub(evidence.observedAt)
	return age >= 0 && age <= concurrentPilotMaxAge && evidence.validUntil.After(now)
}

func (m *concurrentPilot) readClockLocked() (time.Time, error) {
	now := m.clock()
	if now.IsZero() || now.Before(m.lastClock) {
		m.revokeLocked(pilotReasonClockRegression)
		return time.Time{}, pilotErr(pilotReasonClockRegression)
	}
	m.lastClock = now
	if !now.Before(m.attemptStop) && !m.revoked {
		m.revokeLocked(pilotReasonAttemptExpired)
		return now, pilotErr(pilotReasonAttemptExpired)
	}
	if m.permit != nil && !m.permit.expiresAt.After(now) && !m.revoked {
		m.revokeLocked(pilotReasonEvidenceExpired)
		return now, pilotErr(pilotReasonEvidenceExpired)
	}
	return now, nil
}

func (m *concurrentPilot) revokeLocked(reason pilotReason) {
	if m.renewalCancel != nil {
		m.renewalCancel()
		m.renewalCancel = nil
	}
	m.renewalID = 0
	if !m.revoked {
		m.fence++
		m.revoked = true
	}
	m.active = false
	m.admission = false
	m.permit = nil
	m.retireCollectorLocked()
	m.reason = reason
	for _, record := range m.work {
		if record.status == pilotWorkPending || record.status == pilotWorkClaimed {
			record.status = pilotWorkInvalidated
		}
	}
}

func (m *concurrentPilot) reasonOr(fallback pilotReason) pilotReason {
	if m.reason != "" {
		return m.reason
	}
	return fallback
}

func (m *concurrentPilot) permitVersionLocked() uint64 {
	if m.permit == nil {
		return 0
	}
	return m.permit.evidenceVersion
}

func (m *concurrentPilot) report() concurrentPilotReport {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, _ = m.readClockLocked()
	now := m.lastClock
	fresh := m.permit != nil && m.permit.expiresAt.After(now) && pilotEvidenceFresh(m.permit.readerEvidence, now) && pilotEvidenceFresh(m.permit.collectorEvidence, now)
	eligible := m.active && m.admission && !m.revoked && fresh && now.Before(m.attemptStop)
	return concurrentPilotReport{
		ContractID:           concurrentPilotContract,
		Mode:                 concurrentPilotMode,
		PolicyID:             concurrentPilotPolicy,
		EvidenceVersion:      m.version,
		PilotEligible:        eligible,
		AdmissionOpen:        eligible,
		CanAuthorize:         false,
		NativeQualified:      false,
		ProductionQualified:  false,
		StorePermit:          false,
		SnapshotFreshness:    pilotFreshnessLabel(fresh),
		GlobalCoverage:       "partial",
		Continuity:           "unknown",
		RefreshQualification: "not_qualified",
		AttemptExpired:       !now.Before(m.attemptStop),
		Revoked:              m.revoked,
		Reason:               string(m.reason),
		Counters: pilotCounterReport{
			SeedReceipts:             m.counters.seedReceipts,
			EvidenceVectors:          m.counters.evidenceVectors,
			UnclassifiedCookieWrites: m.counters.unclassifiedCookieWrites,
			AcceptedResults:          m.counters.acceptedResults,
			ReaderWindowOpens:        m.counters.readerWindowOpens,
			ReaderWindowCloses:       m.counters.readerWindowCloses,
		},
	}
}

func pilotFreshnessLabel(fresh bool) string {
	if fresh {
		return "fresh"
	}
	return "unavailable"
}
