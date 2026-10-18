//go:build auth_session_dev && auth_session_fixture

// Package x contains an offline-only, synthetic X browser-self verifier
// prototype. Its route, login-state markers, lineage generations, and wire
// schema are invented fixtures, not measured X behavior or claims about X's
// native endpoints. Nothing here is wired to Store, capture admission, a
// browser, or a production provider.
package x

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	FixtureOperation = "x.browser-self.fixture/v1"
	FixtureSchema    = "aku.x.browser-self.fixture.v1"
	FixtureOrigin    = "https://x.com"
	FixturePath      = "/__fixture__/x/account-self/v1"

	MaxBodyBytes         = 4096
	MaxEvidenceAge       = 5 * time.Second
	MaxPendingChallenges = 4
	maxJSONDepth         = 8
)

type AuthState string

const (
	Authenticated AuthState = "authenticated"
	LoggedOut     AuthState = "logged_out"
	Challenge     AuthState = "challenge"
	Unknown       AuthState = "unknown"
	Unsupported   AuthState = "unsupported"
)

type Phase string

const (
	PhaseInitial Phase = "initial"
	PhaseRecheck Phase = "recheck"
)

type Continuity string

const (
	ContinuitySame        Continuity = "same"
	ContinuityChanged     Continuity = "changed"
	ContinuityUnavailable Continuity = "unavailable"
	ContinuityUnsupported Continuity = "unsupported"
)

type Reason string

const (
	ReasonNone               Reason = ""
	ReasonChallengeUnknown   Reason = "challenge_unknown_or_consumed"
	ReasonBindingMismatch    Reason = "binding_mismatch"
	ReasonCoverageIncomplete Reason = "coverage_incomplete"
	ReasonStaleEvidence      Reason = "stale_or_invalid_time"
	ReasonWrongOriginOrPath  Reason = "wrong_origin_or_path"
	ReasonRedirect           Reason = "redirect_unsupported"
	ReasonHTTPStatus         Reason = "http_status_unsupported"
	ReasonBodyTooLarge       Reason = "body_too_large"
	ReasonMalformedBody      Reason = "malformed_body"
	ReasonSchemaUnsupported  Reason = "schema_unsupported"
	ReasonStateUnknown       Reason = "state_unknown"
	ReasonIdentityMissing    Reason = "identity_missing"
	ReasonAccountMismatch    Reason = "account_mismatch"
	ReasonPhaseMismatch      Reason = "phase_mismatch"
	ReasonPhaseDisagreement  Reason = "phase_disagreement"
	ReasonLoggedOut          Reason = "logged_out"
	ReasonChallengeState     Reason = "challenge"
	ReasonUnknownState       Reason = "unknown"
	ReasonLineageUnavailable Reason = "lineage_unavailable"
	ReasonLineageUnsupported Reason = "lineage_unsupported"
	ReasonInvalidWitness     Reason = "synthetic_witness_invalid"
)

// Pin is trusted harness context. It must be supplied out-of-band from the
// response body. The verifier is fixture-only and this pin never authorizes
// capture or persists an identity.
type Pin struct {
	ProfileID         string
	OwnerID           string
	OwnerEpoch        string
	OwnerGeneration   uint64
	AccountID         string
	LineageGeneration uint64
}

// Request is a one-use fixture challenge. Challenge and RequestID are request
// bindings only; neither is an account or session identifier.
type Request struct {
	Version         int
	Source          string
	Operation       string
	Phase           Phase
	Challenge       string
	RequestID       string
	Origin          string
	Path            string
	ProfileID       string
	OwnerID         string
	OwnerEpoch      string
	OwnerGeneration uint64
	IssuedAt        time.Time
}

// Envelope is trusted only as an in-process fixture-harness value. Fields in
// the JSON response cannot populate or override this binding.
type Envelope struct {
	Source           string
	Operation        string
	Phase            Phase
	Challenge        string
	RequestID        string
	RequestOrigin    string
	RequestPath      string
	FinalURL         string
	Redirected       bool
	ProfileID        string
	OwnerID          string
	OwnerEpoch       string
	OwnerGeneration  uint64
	CoverageComplete bool
	StatusCode       int
	ObservedAt       time.Time
}

// Attempt combines a one-use request with its separately trusted synthetic
// envelope and bounded response body.
type Attempt struct {
	Request  Request
	Envelope Envelope
	Lineage  *SyntheticLineageObservation
	Body     []byte
}

// SyntheticLineageObservation is separate trusted fixture context, never a
// field in the response body. Generation is a scenario-owned lifecycle counter
// compared with Pin.LineageGeneration; it is not a cookie fingerprint, probe
// UUID, or claim about a native X session identifier.
type SyntheticLineageObservation struct {
	Provenance       string
	Request          Request
	Generation       uint64
	CoverageComplete bool
	ObservedAt       time.Time
}

type Observation struct {
	State     AuthState
	AccountID string // populated only when it matches the pinned fixture identity
	Reason    Reason

	phase             Phase
	challenge         string
	observedAt        time.Time
	issuedAt          time.Time
	lineageGeneration uint64
	lineageObservedAt time.Time
	lineageReason     Reason
}

type PairResult struct {
	State               AuthState
	AccountID           string
	Continuity          Continuity
	IdentityMatch       bool
	CaptureAuthorized   bool
	ProductionQualified bool
	Reason              Reason
}

// CanAuthorize is deliberately constant: fixture verification results never
// become capture, Store, checkpoint, or provider authorization.
func (PairResult) CanAuthorize() bool { return false }

// Verifier is serial-use fixture machinery. Callers must not issue or consume
// challenges concurrently; the offline harness runs each vector sequentially.
type Verifier struct {
	pin       Pin
	now       func() time.Time
	namespace string
	next      uint64
	pending   map[string]Request
}

func NewVerifier(pin Pin, now func() time.Time) (*Verifier, error) {
	if strings.TrimSpace(pin.ProfileID) == "" || strings.TrimSpace(pin.OwnerID) == "" ||
		strings.TrimSpace(pin.OwnerEpoch) == "" || pin.OwnerGeneration == 0 || !validAccountID(pin.AccountID) || pin.LineageGeneration == 0 {
		return nil, errors.New("synthetic X verifier pin is incomplete")
	}
	if now == nil {
		now = time.Now
	}
	// A per-verifier namespace prevents challenge reuse across verifier
	// instances. It is only for request replay fencing and is never lineage.
	var namespace [16]byte
	if _, err := rand.Read(namespace[:]); err != nil {
		return nil, errors.New("synthetic X verifier challenge namespace unavailable")
	}
	return &Verifier{pin: pin, now: now, namespace: hex.EncodeToString(namespace[:]), pending: make(map[string]Request)}, nil
}

func (v *Verifier) Issue(phase Phase) (Request, error) {
	if v == nil || v.now == nil || v.pending == nil || (phase != PhaseInitial && phase != PhaseRecheck) {
		return Request{}, errors.New("synthetic X verifier unavailable")
	}
	issued := v.now().UTC()
	for challenge, pending := range v.pending {
		if !pending.IssuedAt.IsZero() && issued.After(pending.IssuedAt) && issued.Sub(pending.IssuedAt) > MaxEvidenceAge {
			delete(v.pending, challenge)
		}
	}
	if len(v.pending) >= MaxPendingChallenges {
		return Request{}, errors.New("synthetic X verifier challenge capacity reached")
	}
	v.next++
	sequence := strconv.FormatUint(v.next, 10)
	request := Request{
		Version: 1, Source: "x", Operation: FixtureOperation, Phase: phase,
		Challenge: "fixture-" + v.namespace + "-challenge-" + sequence,
		RequestID: "fixture-" + v.namespace + "-request-" + sequence,
		Origin:    FixtureOrigin, Path: FixturePath, ProfileID: v.pin.ProfileID,
		OwnerID: v.pin.OwnerID, OwnerEpoch: v.pin.OwnerEpoch,
		OwnerGeneration: v.pin.OwnerGeneration, IssuedAt: issued,
	}
	v.pending[request.Challenge] = request
	return request, nil
}

func (v *Verifier) Observe(attempt Attempt) Observation {
	base := Observation{State: Unknown, Reason: ReasonChallengeUnknown}
	if v == nil || v.now == nil || v.pending == nil {
		return base
	}
	request, ok := v.pending[attempt.Request.Challenge]
	if !ok {
		return base
	}
	// Consume before validation: malformed, stale, mismatched, and negative
	// proofs cannot be retried with the same challenge.
	delete(v.pending, attempt.Request.Challenge)
	base.phase = request.Phase
	base.challenge = request.Challenge
	base.issuedAt = request.IssuedAt

	if !sameRequest(request, attempt.Request) || !envelopeMatches(request, attempt.Envelope, v.pin) {
		base.State, base.Reason = Unsupported, ReasonBindingMismatch
		return base
	}
	if !attempt.Envelope.CoverageComplete {
		base.State, base.Reason = Unknown, ReasonCoverageIncomplete
		return base
	}
	if attempt.Envelope.Redirected {
		base.State, base.Reason = Unsupported, ReasonRedirect
		return base
	}
	if !validFinalURL(attempt.Envelope.FinalURL) {
		base.State, base.Reason = Unsupported, ReasonWrongOriginOrPath
		return base
	}
	now := v.now().UTC()
	if request.IssuedAt.IsZero() || request.IssuedAt.After(now) || now.Sub(request.IssuedAt) > MaxEvidenceAge {
		base.State, base.Reason = Unknown, ReasonStaleEvidence
		return base
	}
	observed := attempt.Envelope.ObservedAt.UTC()
	base.observedAt = observed
	if request.IssuedAt.IsZero() || observed.IsZero() || observed.Before(request.IssuedAt) || observed.After(now) ||
		now.Sub(observed) > MaxEvidenceAge {
		base.State, base.Reason = Unknown, ReasonStaleEvidence
		return base
	}
	base.lineageGeneration, base.lineageObservedAt, base.lineageReason = validateLineage(request, attempt.Envelope, attempt.Lineage, now)
	if !validStatus(attempt.Envelope.StatusCode) {
		base.State, base.Reason = Unknown, ReasonHTTPStatus
		return base
	}
	wire, reason := decodeWire(attempt.Body)
	if reason != ReasonNone {
		base.State, base.Reason = Unknown, reason
		return base
	}
	if wire.Schema != FixtureSchema {
		base.State, base.Reason = Unsupported, ReasonSchemaUnsupported
		return base
	}
	switch wire.State {
	case string(Authenticated):
		if attempt.Envelope.StatusCode != 200 || wire.Data == nil || !validAccountID(wire.Data.IDStr) {
			base.State, base.Reason = Unknown, ReasonIdentityMissing
			return base
		}
		if wire.Data.IDStr != v.pin.AccountID {
			base.State, base.Reason = Authenticated, ReasonAccountMismatch
			return base
		}
		base.State, base.AccountID, base.Reason = Authenticated, wire.Data.IDStr, ReasonNone
		return base
	case string(LoggedOut):
		if attempt.Envelope.StatusCode == 401 && wire.Data == nil {
			base.State, base.Reason = LoggedOut, ReasonNone
			return base
		}
	case string(Challenge):
		if attempt.Envelope.StatusCode == 403 && wire.Data == nil {
			base.State, base.Reason = Challenge, ReasonNone
			return base
		}
	case string(Unknown):
		if attempt.Envelope.StatusCode == 200 && wire.Data == nil {
			base.State, base.Reason = Unknown, ReasonStateUnknown
			return base
		}
	default:
		base.State, base.Reason = Unknown, ReasonStateUnknown
		return base
	}
	base.State, base.Reason = Unknown, ReasonHTTPStatus
	return base
}

func (v *Verifier) VerifyPair(first, second Attempt) PairResult {
	result := PairResult{State: Unknown, Continuity: ContinuityUnsupported, Reason: ReasonPhaseMismatch}
	if v == nil || v.now == nil {
		return result
	}
	// Observe both regardless of the first result so both challenges are spent.
	a := v.Observe(first)
	b := v.Observe(second)
	if a.phase != PhaseInitial || b.phase != PhaseRecheck || a.challenge == "" || b.challenge == "" || a.challenge == b.challenge {
		return result
	}
	if a.State != b.State {
		result.Reason = ReasonPhaseDisagreement
		if a.State == LoggedOut || b.State == LoggedOut {
			result.State = LoggedOut
		} else if a.State == Challenge || b.State == Challenge {
			result.State = Challenge
		}
		result.Continuity = ContinuityUnavailable
		return result
	}
	result.State = a.State
	if a.State != Authenticated {
		result.Continuity = ContinuityUnavailable
		result.Reason = firstReason(a, b)
		if result.Reason == ReasonNone {
			switch a.State {
			case LoggedOut:
				result.Reason = ReasonLoggedOut
			case Challenge:
				result.Reason = ReasonChallengeState
			default:
				result.Reason = ReasonUnknownState
			}
		}
		return result
	}
	if a.Reason != ReasonNone || b.Reason != ReasonNone || a.AccountID == "" || a.AccountID != b.AccountID {
		result.Continuity = ContinuityUnavailable
		result.Reason = firstReason(a, b)
		if result.Reason == ReasonNone {
			result.Reason = ReasonPhaseDisagreement
		}
		if a.Reason == ReasonAccountMismatch || b.Reason == ReasonAccountMismatch {
			result.Continuity = ContinuityChanged
			result.Reason = ReasonAccountMismatch
		}
		return result
	}
	result.AccountID, result.IdentityMatch = a.AccountID, true
	verifiedAt := v.now().UTC()
	if !stillFresh(a, verifiedAt) || !stillFresh(b, verifiedAt) {
		result.State, result.Continuity, result.Reason = Unknown, ContinuityUnavailable, ReasonStaleEvidence
		result.AccountID, result.IdentityMatch = "", false
		return result
	}
	if a.lineageReason != ReasonNone || b.lineageReason != ReasonNone {
		result.Reason = firstLineageReason(a, b)
		if a.lineageReason == ReasonStaleEvidence || b.lineageReason == ReasonStaleEvidence {
			result.State, result.AccountID, result.IdentityMatch = Unknown, "", false
			result.Continuity, result.Reason = ContinuityUnavailable, ReasonStaleEvidence
			return result
		}
		if result.Reason == ReasonLineageUnsupported || result.Reason == ReasonInvalidWitness {
			result.Continuity = ContinuityUnsupported
		} else {
			result.Continuity = ContinuityUnavailable
		}
		return result
	}
	if !a.lineageObservedAt.Equal(a.observedAt) || !b.lineageObservedAt.Equal(b.observedAt) ||
		!lineageFresh(a, verifiedAt) || !lineageFresh(b, verifiedAt) {
		result.State, result.AccountID, result.IdentityMatch = Unknown, "", false
		result.Continuity, result.Reason = ContinuityUnavailable, ReasonStaleEvidence
		return result
	}
	if a.lineageGeneration == v.pin.LineageGeneration && b.lineageGeneration == v.pin.LineageGeneration {
		result.Continuity, result.Reason = ContinuitySame, ReasonNone
	} else {
		result.Continuity, result.Reason = ContinuityChanged, ReasonNone
	}
	return result
}

func firstLineageReason(a, b Observation) Reason {
	if a.lineageReason != ReasonNone {
		return a.lineageReason
	}
	return b.lineageReason
}

func firstReason(a, b Observation) Reason {
	if a.Reason != ReasonNone {
		return a.Reason
	}
	return b.Reason
}

func stillFresh(observation Observation, now time.Time) bool {
	return !observation.issuedAt.IsZero() && !observation.observedAt.IsZero() &&
		!observation.issuedAt.After(now) && now.Sub(observation.issuedAt) <= MaxEvidenceAge &&
		!observation.observedAt.Before(observation.issuedAt) && !observation.observedAt.After(now) &&
		now.Sub(observation.observedAt) <= MaxEvidenceAge
}

func lineageFresh(observation Observation, now time.Time) bool {
	return !observation.lineageObservedAt.IsZero() && !observation.lineageObservedAt.After(now) &&
		now.Sub(observation.lineageObservedAt) <= MaxEvidenceAge
}

func validateLineage(request Request, envelope Envelope, lineage *SyntheticLineageObservation, now time.Time) (uint64, time.Time, Reason) {
	if lineage == nil {
		return 0, time.Time{}, ReasonLineageUnsupported
	}
	if lineage.Provenance != "synthetic-fixture" {
		return 0, time.Time{}, ReasonInvalidWitness
	}
	if !lineage.CoverageComplete {
		return 0, lineage.ObservedAt.UTC(), ReasonLineageUnavailable
	}
	if !sameRequest(request, lineage.Request) {
		return 0, time.Time{}, ReasonInvalidWitness
	}
	observedAt := lineage.ObservedAt.UTC()
	if lineage.Generation == 0 {
		return 0, observedAt, ReasonLineageUnavailable
	}
	if observedAt.IsZero() || observedAt.Before(request.IssuedAt) || observedAt.After(now) || now.Sub(observedAt) > MaxEvidenceAge {
		return 0, observedAt, ReasonStaleEvidence
	}
	if !observedAt.Equal(envelope.ObservedAt.UTC()) {
		return 0, observedAt, ReasonInvalidWitness
	}
	return lineage.Generation, observedAt, ReasonNone
}

func sameRequest(a, b Request) bool {
	return a.Version == b.Version && a.Source == b.Source && a.Operation == b.Operation && a.Phase == b.Phase &&
		a.Challenge == b.Challenge && a.RequestID == b.RequestID && a.Origin == b.Origin && a.Path == b.Path &&
		a.ProfileID == b.ProfileID && a.OwnerID == b.OwnerID && a.OwnerEpoch == b.OwnerEpoch &&
		a.OwnerGeneration == b.OwnerGeneration && a.IssuedAt.Equal(b.IssuedAt)
}

func envelopeMatches(request Request, envelope Envelope, pin Pin) bool {
	return envelope.Source == request.Source && envelope.Operation == request.Operation && envelope.Phase == request.Phase &&
		envelope.Challenge == request.Challenge && envelope.RequestID == request.RequestID &&
		envelope.RequestOrigin == request.Origin && envelope.RequestPath == request.Path &&
		envelope.ProfileID == pin.ProfileID && envelope.OwnerID == pin.OwnerID &&
		envelope.OwnerEpoch == pin.OwnerEpoch && envelope.OwnerGeneration == pin.OwnerGeneration
}

func validFinalURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host == "x.com" && u.User == nil && u.Path == FixturePath && u.RawQuery == "" && u.Fragment == ""
}

func validStatus(status int) bool { return status == 200 || status == 401 || status == 403 }

func validAccountID(id string) bool {
	if len(id) == 0 || len(id) > 20 || id[0] == '0' {
		return false
	}
	for i := 0; i < len(id); i++ {
		if id[i] < '0' || id[i] > '9' {
			return false
		}
	}
	return true
}

type wireResponse struct {
	Schema string
	State  string
	Data   *wireIdentity
}

type wireIdentity struct {
	IDStr    string
	Username string
	Name     string
}

func decodeWire(body []byte) (wireResponse, Reason) {
	if len(body) == 0 || len(body) > MaxBodyBytes || !utf8.Valid(body) {
		if len(body) > MaxBodyBytes {
			return wireResponse{}, ReasonBodyTooLarge
		}
		return wireResponse{}, ReasonMalformedBody
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	value, err := parseJSONValue(decoder, 0)
	if err != nil {
		return wireResponse{}, ReasonMalformedBody
	}
	if _, err = decoder.Token(); err != io.EOF {
		return wireResponse{}, ReasonMalformedBody
	}
	root, ok := value.(map[string]any)
	if !ok || !onlyKeys(root, "schema", "state", "data") {
		return wireResponse{}, ReasonMalformedBody
	}
	schema, schemaOK := root["schema"].(string)
	state, stateOK := root["state"].(string)
	if !schemaOK || !stateOK {
		return wireResponse{}, ReasonMalformedBody
	}
	result := wireResponse{Schema: schema, State: state}
	if rawData, exists := root["data"]; exists {
		data, ok := rawData.(map[string]any)
		if !ok || !onlyKeys(data, "id_str", "username", "name") {
			return wireResponse{}, ReasonMalformedBody
		}
		id, ok := data["id_str"].(string)
		if !ok {
			return wireResponse{}, ReasonMalformedBody
		}
		identity := &wireIdentity{IDStr: id}
		if username, exists := data["username"]; exists {
			identity.Username, ok = username.(string)
			if !ok {
				return wireResponse{}, ReasonMalformedBody
			}
		}
		if name, exists := data["name"]; exists {
			identity.Name, ok = name.(string)
			if !ok {
				return wireResponse{}, ReasonMalformedBody
			}
		}
		result.Data = identity
	}
	return result, ReasonNone
}

func onlyKeys(object map[string]any, allowed ...string) bool {
	if len(object) > len(allowed) {
		return false
	}
	for key := range object {
		found := false
		for _, candidate := range allowed {
			if key == candidate {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func parseJSONValue(decoder *json.Decoder, depth int) (any, error) {
	if depth > maxJSONDepth {
		return nil, errors.New("JSON nesting limit exceeded")
	}
	token, err := decoder.Token()
	if err != nil {
		return nil, errors.New("invalid JSON token")
	}
	switch delimiter := token.(type) {
	case json.Delim:
		switch delimiter {
		case '{':
			object := make(map[string]any)
			for decoder.More() {
				keyToken, keyErr := decoder.Token()
				key, keyOK := keyToken.(string)
				if keyErr != nil || !keyOK {
					return nil, errors.New("invalid JSON object key")
				}
				if _, duplicate := object[key]; duplicate {
					return nil, errors.New("duplicate JSON key")
				}
				value, valueErr := parseJSONValue(decoder, depth+1)
				if valueErr != nil {
					return nil, valueErr
				}
				object[key] = value
			}
			closeToken, closeErr := decoder.Token()
			if closeErr != nil || closeToken != json.Delim('}') {
				return nil, errors.New("unterminated JSON object")
			}
			return object, nil
		case '[':
			items := make([]any, 0)
			for decoder.More() {
				value, valueErr := parseJSONValue(decoder, depth+1)
				if valueErr != nil {
					return nil, valueErr
				}
				items = append(items, value)
			}
			closeToken, closeErr := decoder.Token()
			if closeErr != nil || closeToken != json.Delim(']') {
				return nil, errors.New("unterminated JSON array")
			}
			return items, nil
		default:
			return nil, errors.New("unexpected JSON delimiter")
		}
	default:
		return token, nil
	}
}

// fixtureEnvelope is intentionally internal so package consumers cannot
// mistake raw response JSON for the trusted offline transport envelope.
func fixtureEnvelope(request Request, observedAt time.Time, status int) Envelope {
	return Envelope{
		Source: request.Source, Operation: request.Operation, Phase: request.Phase,
		Challenge: request.Challenge, RequestID: request.RequestID,
		RequestOrigin: request.Origin, RequestPath: request.Path,
		FinalURL: FixtureOrigin + FixturePath, ProfileID: request.ProfileID,
		OwnerID: request.OwnerID, OwnerEpoch: request.OwnerEpoch,
		OwnerGeneration: request.OwnerGeneration, CoverageComplete: true,
		StatusCode: status, ObservedAt: observedAt.UTC(),
	}
}

func authenticatedFixtureBody(id, username, name string) []byte {
	return []byte(fmt.Sprintf(`{"schema":%q,"state":"authenticated","data":{"id_str":%q,"username":%q,"name":%q}}`, FixtureSchema, id, username, name))
}

func fixtureBody(state string) []byte {
	return []byte(fmt.Sprintf(`{"schema":%q,"state":%q}`, FixtureSchema, state))
}

func fixtureAttempt(verifier *Verifier, phase Phase, now time.Time, status int, body []byte) (Attempt, error) {
	request, err := verifier.Issue(phase)
	if err != nil {
		return Attempt{}, err
	}
	return Attempt{Request: request, Envelope: fixtureEnvelope(request, now, status), Body: append([]byte(nil), body...)}, nil
}
