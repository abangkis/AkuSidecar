package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"strings"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

const (
	syntheticAccountCheckpointKey         = "synthetic_account_checkpoint"
	syntheticAccountCheckpointRequiredKey = "synthetic_account_checkpoint_required"
	syntheticAccountCheckpointFenceKey    = "synthetic_account_checkpoint_fence"
	syntheticAccountCheckpointKeyPrefix   = "synthetic_account_checkpoint"
	syntheticAccountCheckpointDomain      = "synthetic-fixture"
	syntheticAccountCheckpointVersion     = 1
	accountCheckpointMaxGeneration        = uint64(9007199254740991)
)

// SyntheticAccountIdentity records only synthetic account identifiers and
// profile scope. It must never contain cookies, credentials, or browser data.
type SyntheticAccountIdentity struct {
	AccountID    string `json:"accountId"`
	StoreScopeID string `json:"storeScopeId"`
	ProfileID    string `json:"profileId"`
}

// SyntheticAccountCheckpoint is an expected-only account map when enrolled.
// AcceptedRevisions is empty until a bounded synthetic receipt is accepted.
type SyntheticAccountCheckpoint struct {
	Domain             string                                     `json:"domain"`
	Version            int                                        `json:"version"`
	CheckpointID       string                                     `json:"checkpointId"`
	Generation         uint64                                     `json:"generation"`
	ExpectedAccounts   map[domain.Source]SyntheticAccountIdentity `json:"expectedAccounts"`
	AcceptedRevisions  map[domain.Source]string                   `json:"acceptedRevisions"`
	VerificationTime   string                                     `json:"verificationTime"`
	OwnerEpoch         string                                     `json:"ownerEpoch"`
	HeadlessGeneration uint64                                     `json:"headlessGeneration"`
	OwnerPID           int                                        `json:"ownerPid"`
}

// SyntheticCheckpointFence is the immutable capture token returned by
// BeginSyntheticCheckpointFence. Its HoldID is a random nonce, not a clock.
type SyntheticCheckpointFence struct {
	Domain           string                                  `json:"domain"`
	Version          int                                     `json:"version"`
	CheckpointID     string                                  `json:"checkpointId"`
	Generation       uint64                                  `json:"generation"`
	HoldID           string                                  `json:"holdId"`
	CheckpointSHA256 string                                  `json:"checkpointSha256"`
	Scope            map[domain.Source]SyntheticAccountScope `json:"scope"`
}

type SyntheticAccountScope struct {
	StoreScopeID string `json:"storeScopeId"`
	ProfileID    string `json:"profileId"`
}

// SyntheticVerifiedReceipt is a caller-supplied synthetic simulation. It is
// not evidence of Chrome, account, or production authentication state.
type SyntheticVerifiedReceipt struct {
	Domain             string                   `json:"domain"`
	CheckpointID       string                   `json:"checkpointId"`
	Generation         uint64                   `json:"generation"`
	HoldID             string                   `json:"holdId"`
	Accounts           map[domain.Source]string `json:"accounts"`
	StoreScopeIDs      map[domain.Source]string `json:"storeScopeIds"`
	ProfileIDs         map[domain.Source]string `json:"profileIds"`
	Revisions          map[domain.Source]string `json:"revisions"`
	OwnerEpoch         string                   `json:"ownerEpoch"`
	HeadlessGeneration uint64                   `json:"headlessGeneration"`
	OwnerPID           int                      `json:"ownerPid"`
	CapturedAt         time.Time                `json:"capturedAt"`
}

type accountCheckpointRequired struct {
	Domain       string `json:"domain"`
	Version      int    `json:"version"`
	CheckpointID string `json:"checkpointId"`
	Generation   uint64 `json:"generation"`
}

type accountCheckpointFenceRecord struct {
	Domain             string                                  `json:"domain"`
	Version            int                                     `json:"version"`
	CheckpointID       string                                  `json:"checkpointId"`
	Generation         uint64                                  `json:"generation"`
	CheckpointSHA256   string                                  `json:"checkpointSha256"`
	Scope              map[domain.Source]SyntheticAccountScope `json:"scope"`
	HoldID             string                                  `json:"holdId"`
	State              string                                  `json:"state"`
	OwnerEpoch         string                                  `json:"ownerEpoch"`
	HeadlessGeneration uint64                                  `json:"headlessGeneration"`
	OwnerPID           int                                     `json:"ownerPid"`
}

type accountCheckpointPermit struct {
	CheckpointID string
	Generation   uint64
	SHA256       string
	ScopeSHA256  string
	HoldID       string
}

type accountCheckpointClaimSnapshot struct {
	checkpoint SyntheticAccountCheckpoint
	fence      accountCheckpointFenceRecord
	sha256     string
}

type accountCheckpointClaimGate struct {
	protected bool
	allowed   bool
	snapshot  *accountCheckpointClaimSnapshot
	paused    bool
}

func supportedSyntheticAccountSource(source domain.Source) bool {
	return source == domain.SourceX || source == domain.SourceInstagram || source == domain.SourceLinkedIn
}

func copySyntheticAccounts(in map[domain.Source]SyntheticAccountIdentity) map[domain.Source]SyntheticAccountIdentity {
	out := make(map[domain.Source]SyntheticAccountIdentity, len(in))
	for source, identity := range in {
		out[source] = identity
	}
	return out
}

func copySyntheticRevisions(in map[domain.Source]string) map[domain.Source]string {
	out := make(map[domain.Source]string, len(in))
	for source, revision := range in {
		out[source] = revision
	}
	return out
}

func copySyntheticStringMap(in map[domain.Source]string) map[domain.Source]string {
	out := make(map[domain.Source]string, len(in))
	for source, value := range in {
		out[source] = value
	}
	return out
}

func copySyntheticScope(in map[domain.Source]SyntheticAccountScope) map[domain.Source]SyntheticAccountScope {
	out := make(map[domain.Source]SyntheticAccountScope, len(in))
	for source, scope := range in {
		out[source] = scope
	}
	return out
}

func syntheticCheckpointScope(accounts map[domain.Source]SyntheticAccountIdentity) map[domain.Source]SyntheticAccountScope {
	out := make(map[domain.Source]SyntheticAccountScope, len(accounts))
	for source, identity := range accounts {
		out[source] = SyntheticAccountScope{StoreScopeID: identity.StoreScopeID, ProfileID: identity.ProfileID}
	}
	return out
}

func strictCheckpointJSON(raw string, target any) error {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("trailing JSON")
	}
	canonical, err := json.Marshal(target)
	if err != nil || !bytes.Equal(canonical, []byte(raw)) {
		return errors.New("noncanonical JSON")
	}
	return nil
}

func validSyntheticIdentityValue(value string) bool {
	if value == "" || len(value) > 256 || strings.TrimSpace(value) != value {
		return false
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func validateSyntheticCheckpoint(checkpoint SyntheticAccountCheckpoint) error {
	if checkpoint.Domain != syntheticAccountCheckpointDomain || checkpoint.Version != syntheticAccountCheckpointVersion ||
		!validSyntheticIdentityValue(checkpoint.CheckpointID) || checkpoint.Generation == 0 ||
		checkpoint.ExpectedAccounts == nil || len(checkpoint.ExpectedAccounts) == 0 || checkpoint.AcceptedRevisions == nil {
		return errors.New("invalid checkpoint")
	}
	var storeScopeID, profileID string
	firstScope := true
	for source, identity := range checkpoint.ExpectedAccounts {
		if !supportedSyntheticAccountSource(source) || !validSyntheticIdentityValue(identity.AccountID) ||
			!validSyntheticIdentityValue(identity.StoreScopeID) || !validSyntheticIdentityValue(identity.ProfileID) {
			return errors.New("invalid checkpoint")
		}
		if firstScope {
			storeScopeID, profileID, firstScope = identity.StoreScopeID, identity.ProfileID, false
		} else if identity.StoreScopeID != storeScopeID || identity.ProfileID != profileID {
			return errors.New("invalid checkpoint")
		}
	}
	if len(checkpoint.AcceptedRevisions) == 0 {
		if checkpoint.Generation != 1 || checkpoint.VerificationTime != "" || checkpoint.OwnerEpoch != "" || checkpoint.HeadlessGeneration != 0 || checkpoint.OwnerPID != 0 {
			return errors.New("invalid checkpoint")
		}
		return nil
	}
	if len(checkpoint.AcceptedRevisions) != len(checkpoint.ExpectedAccounts) || !validSyntheticIdentityValue(checkpoint.OwnerEpoch) ||
		checkpoint.HeadlessGeneration == 0 || checkpoint.HeadlessGeneration > accountCheckpointMaxGeneration || checkpoint.OwnerPID <= 0 || checkpoint.OwnerPID > math.MaxInt32 {
		return errors.New("invalid checkpoint")
	}
	for source, revision := range checkpoint.AcceptedRevisions {
		if _, exists := checkpoint.ExpectedAccounts[source]; !exists || !validSyntheticIdentityValue(revision) {
			return errors.New("invalid checkpoint")
		}
	}
	verifiedAt, err := time.Parse(time.RFC3339Nano, checkpoint.VerificationTime)
	if err != nil || verifiedAt.Location() != time.UTC || verifiedAt.UTC().Format(time.RFC3339Nano) != checkpoint.VerificationTime {
		return errors.New("invalid checkpoint")
	}
	return nil
}

func checkpointSHA256(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func scopeSHA256(scope map[domain.Source]SyntheticAccountScope) string {
	raw, _ := json.Marshal(scope)
	return checkpointSHA256(string(raw))
}

func validFence(fence accountCheckpointFenceRecord, checkpoint SyntheticAccountCheckpoint, raw string) bool {
	if fence.Domain != syntheticAccountCheckpointDomain || fence.Version != syntheticAccountCheckpointVersion ||
		fence.CheckpointID != checkpoint.CheckpointID || fence.Generation != checkpoint.Generation ||
		fence.CheckpointSHA256 != checkpointSHA256(raw) || fence.HoldID == "" ||
		!syntheticScopesEqual(fence.Scope, syntheticCheckpointScope(checkpoint.ExpectedAccounts)) {
		return false
	}
	switch fence.State {
	case "pending":
		return fence.OwnerEpoch == "" && fence.HeadlessGeneration == 0 && fence.OwnerPID == 0
	case "authorized":
		return len(checkpoint.AcceptedRevisions) == len(checkpoint.ExpectedAccounts) &&
			fence.OwnerEpoch == checkpoint.OwnerEpoch && fence.HeadlessGeneration == checkpoint.HeadlessGeneration &&
			fence.OwnerPID == checkpoint.OwnerPID
	default:
		return false
	}
}

func syntheticScopesEqual(left, right map[domain.Source]SyntheticAccountScope) bool {
	if left == nil || right == nil || len(left) != len(right) {
		return false
	}
	for source, scope := range right {
		if left[source] != scope {
			return false
		}
	}
	return true
}

func (s *Store) currentAccountCheckpointClaimGate(ctx context.Context, tx *sql.Tx) (accountCheckpointClaimGate, error) {
	rows, err := tx.QueryContext(ctx, `SELECT key,value FROM meta WHERE substr(key,1,?)=? ORDER BY key`, len(syntheticAccountCheckpointKeyPrefix), syntheticAccountCheckpointKeyPrefix)
	if err != nil {
		return accountCheckpointClaimGate{}, errors.New("capture checkpoint fence lookup failed")
	}
	values := make(map[string]string, 4)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			rows.Close()
			return accountCheckpointClaimGate{}, errors.New("capture checkpoint fence lookup failed")
		}
		values[key] = value
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return accountCheckpointClaimGate{}, errors.New("capture checkpoint fence lookup failed")
	}
	if err := rows.Close(); err != nil {
		return accountCheckpointClaimGate{}, errors.New("capture checkpoint fence lookup failed")
	}
	if len(values) == 0 {
		return accountCheckpointClaimGate{}, nil
	}
	gate := accountCheckpointClaimGate{protected: true}
	if len(values) != 3 {
		return gate, errors.New("capture checkpoint metadata is incomplete")
	}
	checkpointRaw, checkpointOK := values[syntheticAccountCheckpointKey]
	requiredRaw, requiredOK := values[syntheticAccountCheckpointRequiredKey]
	fenceRaw, fenceOK := values[syntheticAccountCheckpointFenceKey]
	if !checkpointOK || !requiredOK || !fenceOK {
		return gate, errors.New("capture checkpoint metadata is incomplete")
	}
	var checkpoint SyntheticAccountCheckpoint
	var required accountCheckpointRequired
	var fence accountCheckpointFenceRecord
	if strictCheckpointJSON(checkpointRaw, &checkpoint) != nil || strictCheckpointJSON(requiredRaw, &required) != nil ||
		strictCheckpointJSON(fenceRaw, &fence) != nil || validateSyntheticCheckpoint(checkpoint) != nil {
		return gate, errors.New("capture checkpoint metadata is invalid")
	}
	if required.Domain != syntheticAccountCheckpointDomain || required.Version != syntheticAccountCheckpointVersion ||
		required.CheckpointID != checkpoint.CheckpointID || required.Generation != checkpoint.Generation ||
		!validFence(fence, checkpoint, checkpointRaw) {
		return gate, errors.New("capture checkpoint fence is invalid")
	}
	if fence.State != "authorized" {
		return gate, nil
	}
	scope := syntheticCheckpointScope(checkpoint.ExpectedAccounts)
	permit := s.loadAccountCheckpointPermit()
	if permit == nil || permit.CheckpointID != checkpoint.CheckpointID || permit.Generation != checkpoint.Generation ||
		permit.SHA256 != checkpointSHA256(checkpointRaw) || permit.ScopeSHA256 != scopeSHA256(scope) || permit.HoldID != fence.HoldID {
		return gate, nil
	}
	gate.allowed = true
	gate.snapshot = &accountCheckpointClaimSnapshot{checkpoint: checkpoint, fence: fence, sha256: checkpointSHA256(checkpointRaw)}
	return gate, nil
}

func (s *Store) loadAccountCheckpointPermit() *accountCheckpointPermit {
	s.capturePermitMu.RLock()
	defer s.capturePermitMu.RUnlock()
	if s.capturePermit == nil {
		return nil
	}
	copy := *s.capturePermit
	return &copy
}

func (s *Store) setAccountCheckpointPermit(permit *accountCheckpointPermit) {
	s.capturePermitMu.Lock()
	defer s.capturePermitMu.Unlock()
	if permit == nil {
		s.capturePermit = nil
		return
	}
	copy := *permit
	s.capturePermit = &copy
}

func (s *Store) consumeAccountCheckpointFenceAttempt(holdID string) bool {
	if holdID == "" {
		return false
	}
	s.fenceAttemptMu.Lock()
	defer s.fenceAttemptMu.Unlock()
	if s.issuedFenceHoldID != holdID || s.consumedFence {
		return false
	}
	s.consumedFence = true
	return true
}

func (s *Store) resetAccountCheckpointFenceAttempt(holdID string) {
	s.fenceAttemptMu.Lock()
	defer s.fenceAttemptMu.Unlock()
	if holdID != "" {
		s.issuedFenceHoldID = holdID
		s.consumedFence = false
	}
}

func (s *Store) captureCommandClaimGate(ctx context.Context, tx *sql.Tx, runID string) (accountCheckpointClaimGate, error) {
	paused, err := fixtureCaptureClaimPaused(ctx, tx, runID)
	if err != nil {
		return accountCheckpointClaimGate{}, errors.New("capture pause lookup failed")
	}
	if paused {
		return accountCheckpointClaimGate{paused: true}, nil
	}
	return s.currentAccountCheckpointClaimGate(ctx, tx)
}

func (s *Store) captureMediaClaimGate(ctx context.Context, tx *sql.Tx, sessionID string) (accountCheckpointClaimGate, error) {
	if sessionID != "" {
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM meta WHERE key=?`, fixtureCapturePausePrefix+sessionID).Scan(&count); err != nil {
			return accountCheckpointClaimGate{}, errors.New("capture pause lookup failed")
		}
		if count != 0 {
			return accountCheckpointClaimGate{paused: true}, nil
		}
	}
	return s.currentAccountCheckpointClaimGate(ctx, tx)
}

func accountCheckpointCommandEligible(snapshot *accountCheckpointClaimSnapshot, source domain.Source, payload map[string]any) bool {
	if snapshot == nil {
		return true
	}
	if _, exists := snapshot.checkpoint.ExpectedAccounts[source]; !exists {
		return false
	}
	collector, err := CaptureCollector(payload)
	if err != nil || collector != "headless" {
		return false
	}
	stamp, ok := payload["captureRuntime"].(map[string]any)
	if !ok || stamp["driver"] != "headless" || stamp["epoch"] != snapshot.checkpoint.OwnerEpoch {
		return false
	}
	generation, ok := stamp["generation"].(float64)
	return ok && !math.IsNaN(generation) && !math.IsInf(generation, 0) &&
		generation == float64(snapshot.checkpoint.HeadlessGeneration)
}

func checkpointContextError(ctx context.Context) error {
	if ctx.Err() != nil {
		return errors.New("account checkpoint operation cancelled")
	}
	return nil
}
