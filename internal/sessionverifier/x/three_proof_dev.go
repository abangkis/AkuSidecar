//go:build auth_session_dev && auth_session_fixture

package x

import (
	"sort"
	"strings"
	"time"
)

const (
	ThreeProofEvidenceSchema  = "aku.x.three-proof.fixture.v1"
	ThreeProofLifecycleSchema = "aku.x.lifecycle.fixture.v1"
	FixtureEvidenceProvenance = "synthetic-fixture"
)

type EvidenceKind string

const (
	EvidenceKindSessionSelf EvidenceKind = "browser-self-state"
	EvidenceKindLifecycle   EvidenceKind = "session-lifecycle"
)

type EvidenceFreshness string

const (
	EvidenceFresh   EvidenceFreshness = "fresh"
	EvidenceStale   EvidenceFreshness = "stale"
	EvidenceUnknown EvidenceFreshness = "unknown"
)

type EvidenceCacheDisposition string

const (
	EvidenceCacheBypass        EvidenceCacheDisposition = "bypass"
	EvidenceCacheMiss          EvidenceCacheDisposition = "miss"
	EvidenceCacheHit           EvidenceCacheDisposition = "hit"
	EvidenceCacheRevalidated   EvidenceCacheDisposition = "revalidated"
	EvidenceCacheUnknown       EvidenceCacheDisposition = "unknown"
	EvidenceCacheNotApplicable EvidenceCacheDisposition = "not-applicable"
)

type EvidenceHints struct {
	CookiePresent      bool
	CookieRotationHint bool
}

// EvidenceMetadata is synthetic harness context bound to the one-use Request.
// Hints never establish identity, authentication, or continuity.
type EvidenceMetadata struct {
	Provenance string
	Schema     string
	Kind       EvidenceKind
	EvidenceID string
	Authority  string
	Request    Request
	ObservedAt time.Time
	Freshness  EvidenceFreshness
	Cache      EvidenceCacheDisposition
	Hints      EvidenceHints
}

type ThreeProofEvidence struct {
	Attempt  Attempt
	Metadata EvidenceMetadata
}

// EnrollmentPolicy supplies an identity expectation. Authority describes the
// enrollment source only; it is never current login evidence or counted as an
// evidence authority by this verifier.
type EnrollmentPolicy struct {
	ExpectedAccountID string
	Authority         string
}

type LifecycleGap string

const (
	LifecycleGapNone                     LifecycleGap = ""
	LifecycleGapStartupUnobserved        LifecycleGap = "startup-unobserved"
	LifecycleGapRestart                  LifecycleGap = "restart"
	LifecycleGapDetach                   LifecycleGap = "detach"
	LifecycleGapOverflow                 LifecycleGap = "overflow"
	LifecycleGapUnsupportedSharedWorker  LifecycleGap = "unsupported-shared-worker"
	LifecycleGapUnsupportedServiceWorker LifecycleGap = "unsupported-service-worker"
)

// LifecycleEvidence is a separate synthetic lifecycle witness. Its coverage
// starts at the accepted initial self observation and extends through verifier
// completion. Generation is a fixture-owned lifecycle counter, never a cookie,
// body, process, or probe identifier.
type LifecycleEvidence struct {
	Metadata EvidenceMetadata
	// BaselineEvidenceID identifies the accepted initial self observation; it
	// is not an arbitrary persisted checkpoint or enrollment authority.
	BaselineEvidenceID string
	BaselineAcceptedAt time.Time
	CoveredFrom        time.Time
	CoveredThrough     time.Time
	BaselineGeneration uint64
	CurrentGeneration  uint64
	CoverageComplete   bool
	Gap                LifecycleGap
}

type ThreeProofInput struct {
	Initial   ThreeProofEvidence
	Recheck   ThreeProofEvidence
	Lifecycle *LifecycleEvidence
	Policy    EnrollmentPolicy
}

type IdentityProofState string

const (
	IdentityMatch       IdentityProofState = "match"
	IdentityMismatch    IdentityProofState = "mismatch"
	IdentityUnavailable IdentityProofState = "unavailable"
	IdentityUnsupported IdentityProofState = "unsupported"
)

type IdentityProof struct {
	State         IdentityProofState
	AccountID     string
	InitialID     string
	RecheckID     string
	InitialSource EvidenceReference
	RecheckSource EvidenceReference
	Reason        Reason
}

type EvidenceReference struct {
	EvidenceID string
	Authority  string
	Provenance string
	Schema     string
	Kind       EvidenceKind
}

type AuthenticationProof struct {
	// Initial and Recheck retain the explicit, phase-specific authentication
	// states. State is the two-phase consensus: it is Unknown on disagreement.
	Initial       AuthState
	Recheck       AuthState
	State         AuthState
	InitialProven bool
	RecheckProven bool
	Stable        bool
	InitialSource EvidenceReference
	RecheckSource EvidenceReference
	Reason        Reason
}

type ContinuityProof struct {
	State              Continuity
	Reason             Reason
	BaselineEvidenceID string
	CoveredFrom        time.Time
	CoveredThrough     time.Time
	CoverageComplete   bool
	BaselineGeneration uint64
	CurrentGeneration  uint64
	Source             EvidenceReference
}

// ThreeProofResult reports fixture outcomes only. Its authorization and
// qualification flags are always false, including when every fixture proof
// succeeds.
type ThreeProofResult struct {
	Identity       IdentityProof
	Authentication AuthenticationProof
	Continuity     ContinuityProof
	// EvidenceAuthorityIDs are deduplicated declared fixture labels, not
	// qualified or independent source authorities.
	EvidenceAuthorityIDs []string
	// DistinctEvidenceAuthorities counts those unique declared fixture labels.
	DistinctEvidenceAuthorities int
	CaptureAuthorized           bool
	NativeQualified             bool
	ProductionQualified         bool
}

func (ThreeProofResult) CanAuthorize() bool { return false }

const (
	ReasonEvidenceMetadata  Reason = "evidence_metadata_invalid"
	ReasonEvidenceFreshness Reason = "evidence_freshness_unavailable"
	ReasonEvidenceReplay    Reason = "evidence_replay_or_echo"
	ReasonPolicyInvalid     Reason = "enrollment_policy_invalid"
	ReasonTemporalOrder     Reason = "phase_temporal_order_invalid"
	ReasonLifecycleGap      Reason = "lifecycle_coverage_gap"
	ReasonLifecycleInvalid  Reason = "lifecycle_evidence_invalid"
)

// explicitFixtureAuthState is called only after Observe has consumed and
// validated a one-use challenge, the trusted envelope, freshness, HTTP status,
// bounded JSON, and fixture schema. Authentication is kept separate from the
// optional identity field so a missing ID cannot erase an explicit auth state.
func explicitFixtureAuthState(wire wireResponse, status int) (AuthState, bool) {
	switch wire.State {
	case string(Authenticated):
		return Authenticated, status == 200
	case string(LoggedOut):
		return LoggedOut, status == 401 && wire.Data == nil
	case string(Challenge):
		return Challenge, status == 403 && wire.Data == nil
	case string(Unknown):
		return Unknown, status == 200 && wire.Data == nil
	default:
		return Unsupported, false
	}
}

type acceptedPhase struct {
	evidence    ThreeProofEvidence
	observed    Observation
	valid       bool
	unsupported bool
	reason      Reason
}

func (v *Verifier) VerifyThreeProof(input ThreeProofInput) ThreeProofResult {
	result := ThreeProofResult{
		Identity: IdentityProof{State: IdentityUnavailable, Reason: ReasonEvidenceMetadata},
		Authentication: AuthenticationProof{
			Initial: Unknown, Recheck: Unknown, State: Unknown, Reason: ReasonEvidenceMetadata,
		},
		Continuity: ContinuityProof{State: ContinuityUnavailable, Reason: ReasonLifecycleInvalid},
	}
	if v == nil || v.now == nil || v.pending == nil {
		return result
	}

	// Observe consumes each challenge exactly once and performs the existing
	// strict parser and binding checks. Metadata is considered only afterward.
	initialObserved := v.Observe(input.Initial.Attempt)
	recheckObserved := v.Observe(input.Recheck.Attempt)
	initial := acceptThreeProofPhase(v, input.Initial, initialObserved, PhaseInitial)
	recheck := acceptThreeProofPhase(v, input.Recheck, recheckObserved, PhaseRecheck)

	now := v.now().UTC()
	if initial.valid && !phaseFreshAt(initial, now) {
		initial.valid, initial.reason = false, ReasonStaleEvidence
	}
	if recheck.valid && !phaseFreshAt(recheck, now) {
		recheck.valid, recheck.reason = false, ReasonStaleEvidence
	}
	if initial.valid && recheck.valid &&
		initial.evidence.Metadata.EvidenceID == recheck.evidence.Metadata.EvidenceID {
		recheck.valid, recheck.unsupported, recheck.reason = false, true, ReasonEvidenceReplay
	}
	if initial.valid && recheck.valid &&
		(recheck.observed.observedAt.Before(initial.observed.observedAt) ||
			recheck.evidence.Attempt.Request.IssuedAt.Before(initial.observed.observedAt)) {
		recheck.valid, recheck.reason = false, ReasonTemporalOrder
	}

	result.Identity = identityProof(initial, recheck, input.Policy, v.pin.AccountID)
	result.Authentication = authenticationProof(initial, recheck)
	result.Continuity = continuityProof(v, input, initial, recheck, now)
	result.EvidenceAuthorityIDs = evidenceAuthorities(initial, recheck, result.Continuity)
	result.DistinctEvidenceAuthorities = len(result.EvidenceAuthorityIDs)
	return result
}

func acceptThreeProofPhase(v *Verifier, evidence ThreeProofEvidence, observed Observation, phase Phase) acceptedPhase {
	accepted := acceptedPhase{evidence: evidence, observed: observed, reason: ReasonEvidenceMetadata}
	request := evidence.Attempt.Request
	metadata := evidence.Metadata
	if observed.phase != phase || request.Phase != phase || observed.challenge == "" || observed.challenge != request.Challenge {
		accepted.reason = ReasonPhaseMismatch
		return accepted
	}
	if metadata.Provenance != FixtureEvidenceProvenance || metadata.Schema != ThreeProofEvidenceSchema ||
		metadata.Kind != EvidenceKindSessionSelf || !validEvidenceToken(metadata.EvidenceID) ||
		!validEvidenceToken(metadata.Authority) {
		accepted.unsupported = true
		return accepted
	}
	if !sameRequest(request, metadata.Request) || !envelopeMatches(request, evidence.Attempt.Envelope, v.pin) {
		accepted.unsupported, accepted.reason = true, ReasonBindingMismatch
		return accepted
	}
	if metadata.Freshness != EvidenceFresh ||
		(metadata.Cache != EvidenceCacheBypass && metadata.Cache != EvidenceCacheMiss) {
		accepted.reason = ReasonEvidenceFreshness
		return accepted
	}
	if !metadata.ObservedAt.Equal(observed.observedAt) || !observed.responseValidated || !observed.explicitAuthValid {
		if observed.Reason != ReasonNone {
			accepted.reason = observed.Reason
		} else {
			accepted.reason = ReasonEvidenceMetadata
		}
		accepted.unsupported = observed.explicitAuthState == Unsupported ||
			observed.Reason == ReasonSchemaUnsupported || observed.Reason == ReasonBindingMismatch ||
			observed.Reason == ReasonRedirect || observed.Reason == ReasonWrongOriginOrPath
		return accepted
	}
	accepted.valid, accepted.reason = true, ReasonNone
	return accepted
}

func phaseFreshAt(phase acceptedPhase, now time.Time) bool {
	return phase.valid && stillFresh(phase.observed, now) &&
		metadataFreshAt(phase.evidence.Metadata, now, EvidenceCacheBypass, EvidenceCacheMiss)
}

func metadataFreshAt(metadata EvidenceMetadata, now time.Time, allowed ...EvidenceCacheDisposition) bool {
	if metadata.Freshness != EvidenceFresh || metadata.ObservedAt.IsZero() || metadata.Request.IssuedAt.IsZero() {
		return false
	}
	cacheAllowed := false
	for _, disposition := range allowed {
		if metadata.Cache == disposition {
			cacheAllowed = true
			break
		}
	}
	observedAt := metadata.ObservedAt.UTC()
	issuedAt := metadata.Request.IssuedAt.UTC()
	return cacheAllowed && !issuedAt.After(now) && !observedAt.Before(issuedAt) &&
		!observedAt.After(now) && now.Sub(observedAt) <= MaxEvidenceAge
}

func identityProof(initial, recheck acceptedPhase, policy EnrollmentPolicy, pinnedAccountID string) IdentityProof {
	proof := IdentityProof{State: IdentityUnavailable, Reason: firstPhaseReason(initial, recheck)}
	if !validAccountID(policy.ExpectedAccountID) || policy.ExpectedAccountID != pinnedAccountID {
		proof.State, proof.Reason = IdentityUnsupported, ReasonPolicyInvalid
		return proof
	}
	if initial.unsupported || recheck.unsupported {
		proof.State, proof.Reason = IdentityUnsupported, firstPhaseReason(initial, recheck)
		return proof
	}
	if initial.valid {
		proof.InitialSource = evidenceReference(initial.evidence.Metadata)
		if initial.observed.identityIDValid {
			proof.InitialID = initial.observed.identityID
		}
	}
	if recheck.valid {
		proof.RecheckSource = evidenceReference(recheck.evidence.Metadata)
		if recheck.observed.identityIDValid {
			proof.RecheckID = recheck.observed.identityID
		}
	}
	if !initial.valid || !recheck.valid || proof.InitialID == "" || proof.RecheckID == "" {
		if proof.Reason == ReasonNone {
			proof.Reason = ReasonIdentityMissing
		}
		return proof
	}
	if proof.InitialID != proof.RecheckID || proof.InitialID != policy.ExpectedAccountID {
		proof.State, proof.Reason = IdentityMismatch, ReasonAccountMismatch
		return proof
	}
	proof.State, proof.AccountID, proof.Reason = IdentityMatch, proof.InitialID, ReasonNone
	return proof
}

func authenticationProof(initial, recheck acceptedPhase) AuthenticationProof {
	proof := AuthenticationProof{Initial: Unknown, Recheck: Unknown, State: Unknown, Reason: firstPhaseReason(initial, recheck)}
	if initial.valid && initial.observed.explicitAuthValid {
		proof.Initial = initial.observed.explicitAuthState
		proof.InitialProven = true
		proof.InitialSource = evidenceReference(initial.evidence.Metadata)
	}
	if recheck.valid && recheck.observed.explicitAuthValid {
		proof.Recheck = recheck.observed.explicitAuthState
		proof.RecheckProven = true
		proof.RecheckSource = evidenceReference(recheck.evidence.Metadata)
	}
	if !initial.valid && initial.unsupported {
		proof.Initial = Unsupported
	}
	if !recheck.valid && recheck.unsupported {
		proof.Recheck = Unsupported
	}
	if proof.InitialProven && proof.RecheckProven && proof.Initial == proof.Recheck {
		proof.State, proof.Stable = proof.Recheck, true
		if proof.State == Unknown {
			proof.Reason = ReasonStateUnknown
		} else {
			proof.Reason = ReasonNone
		}
		return proof
	}
	if proof.InitialProven && proof.RecheckProven {
		proof.Reason = ReasonPhaseDisagreement
	}
	return proof
}

func continuityProof(v *Verifier, input ThreeProofInput, initial, recheck acceptedPhase, completedAt time.Time) ContinuityProof {
	proof := ContinuityProof{State: ContinuityUnavailable, Reason: ReasonLifecycleInvalid}
	witness := input.Lifecycle
	if witness == nil {
		return proof
	}
	proof.BaselineEvidenceID = witness.BaselineEvidenceID
	proof.CoveredFrom, proof.CoveredThrough = witness.CoveredFrom.UTC(), witness.CoveredThrough.UTC()
	proof.BaselineGeneration, proof.CurrentGeneration = witness.BaselineGeneration, witness.CurrentGeneration

	meta := witness.Metadata
	if meta.Provenance != FixtureEvidenceProvenance || meta.Schema != ThreeProofLifecycleSchema ||
		meta.Kind != EvidenceKindLifecycle || !validEvidenceToken(meta.EvidenceID) || !validEvidenceToken(meta.Authority) {
		proof.State, proof.Reason = ContinuityUnsupported, ReasonLifecycleInvalid
		return proof
	}
	proof.Source = evidenceReference(meta)
	if !sameRequest(meta.Request, input.Recheck.Attempt.Request) || meta.Request.Phase != PhaseRecheck ||
		!requestMatchesPin(meta.Request, v.pin) {
		proof.State, proof.Reason = ContinuityUnsupported, ReasonBindingMismatch
		return proof
	}
	if meta.EvidenceID == input.Initial.Metadata.EvidenceID || meta.EvidenceID == input.Recheck.Metadata.EvidenceID ||
		meta.EvidenceID == meta.Request.RequestID || meta.EvidenceID == meta.Request.Challenge {
		proof.State, proof.Reason = ContinuityUnsupported, ReasonEvidenceReplay
		return proof
	}
	if meta.Freshness != EvidenceFresh || meta.Cache != EvidenceCacheNotApplicable {
		proof.Reason = ReasonEvidenceFreshness
		return proof
	}
	if !initial.valid || !recheck.valid || initial.observed.explicitAuthState != Authenticated ||
		!initial.observed.identityIDValid || initial.observed.identityID != input.Policy.ExpectedAccountID ||
		input.Policy.ExpectedAccountID != v.pin.AccountID {
		proof.Reason = firstPhaseReason(initial, recheck)
		if proof.Reason == ReasonNone {
			proof.Reason = ReasonIdentityMissing
		}
		return proof
	}
	if !phaseFreshAt(initial, completedAt) || !phaseFreshAt(recheck, completedAt) ||
		!metadataFreshAt(meta, completedAt, EvidenceCacheNotApplicable) {
		proof.Reason = ReasonStaleEvidence
		return proof
	}
	if recheck.observed.observedAt.Before(initial.observed.observedAt) ||
		meta.ObservedAt.IsZero() || !meta.ObservedAt.Equal(proof.CoveredThrough) ||
		!witness.BaselineAcceptedAt.Equal(initial.observed.observedAt) ||
		witness.BaselineEvidenceID != initial.evidence.Metadata.EvidenceID ||
		witness.BaselineGeneration == 0 || witness.BaselineGeneration != v.pin.LineageGeneration ||
		witness.CurrentGeneration == 0 || proof.CoveredFrom.IsZero() || proof.CoveredThrough.IsZero() ||
		proof.CoveredFrom.After(proof.CoveredThrough) ||
		!proof.CoveredFrom.Equal(witness.BaselineAcceptedAt.UTC()) ||
		proof.CoveredThrough.Before(recheck.observed.observedAt) ||
		!proof.CoveredThrough.Equal(completedAt.UTC()) || proof.CoveredThrough.After(completedAt.UTC()) {
		proof.Reason = ReasonLifecycleInvalid
		return proof
	}
	if !witness.CoverageComplete || witness.Gap != LifecycleGapNone {
		if witness.Gap == LifecycleGapUnsupportedSharedWorker || witness.Gap == LifecycleGapUnsupportedServiceWorker ||
			(witness.Gap != LifecycleGapNone && witness.Gap != LifecycleGapStartupUnobserved &&
				witness.Gap != LifecycleGapRestart && witness.Gap != LifecycleGapDetach && witness.Gap != LifecycleGapOverflow) {
			proof.State, proof.Reason = ContinuityUnsupported, ReasonLineageUnsupported
		} else {
			proof.Reason = ReasonLifecycleGap
		}
		return proof
	}
	proof.CoverageComplete = true
	if witness.CurrentGeneration != witness.BaselineGeneration {
		proof.State, proof.Reason = ContinuityChanged, ReasonNone
		return proof
	}
	if recheck.observed.explicitAuthState == LoggedOut ||
		(recheck.observed.identityIDValid && recheck.observed.identityID != initial.observed.identityID) {
		proof.State, proof.Reason = ContinuityChanged, ReasonNone
		return proof
	}
	if recheck.observed.explicitAuthState != Authenticated {
		proof.CoverageComplete = false
		proof.Reason = ReasonStateUnknown
		return proof
	}
	if !recheck.observed.identityIDValid {
		proof.Reason = ReasonIdentityMissing
		proof.CoverageComplete = false
		return proof
	}
	proof.State, proof.Reason = ContinuitySame, ReasonNone
	return proof
}

func requestMatchesPin(request Request, pin Pin) bool {
	return request.ProfileID == pin.ProfileID && request.OwnerID == pin.OwnerID &&
		request.OwnerEpoch == pin.OwnerEpoch && request.OwnerGeneration == pin.OwnerGeneration
}

func evidenceReference(metadata EvidenceMetadata) EvidenceReference {
	return EvidenceReference{
		EvidenceID: metadata.EvidenceID, Authority: metadata.Authority,
		Provenance: metadata.Provenance, Schema: metadata.Schema, Kind: metadata.Kind,
	}
}

func evidenceAuthorities(initial, recheck acceptedPhase, continuity ContinuityProof) []string {
	set := make(map[string]struct{}, 3)
	for _, phase := range []acceptedPhase{initial, recheck} {
		if phase.valid && validEvidenceToken(phase.evidence.Metadata.Authority) {
			set[phase.evidence.Metadata.Authority] = struct{}{}
		}
	}
	if continuity.CoverageComplete && continuity.Source.EvidenceID != "" && validEvidenceToken(continuity.Source.Authority) {
		set[continuity.Source.Authority] = struct{}{}
	}
	result := make([]string, 0, len(set))
	for authority := range set {
		result = append(result, authority)
	}
	sort.Strings(result)
	return result
}

func firstPhaseReason(initial, recheck acceptedPhase) Reason {
	if !initial.valid && initial.reason != ReasonNone {
		return initial.reason
	}
	if !recheck.valid && recheck.reason != ReasonNone {
		return recheck.reason
	}
	return ReasonNone
}

func validEvidenceToken(value string) bool {
	if value == "" || len(value) > 128 || strings.TrimSpace(value) != value {
		return false
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') &&
			!(r >= '0' && r <= '9') && r != '-' && r != '_' && r != '.' && r != ':' {
			return false
		}
	}
	return true
}
