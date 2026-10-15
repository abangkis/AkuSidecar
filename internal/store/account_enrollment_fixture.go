//go:build auth_session_fixture || auth_session_dev

package store

import (
	"context"
	"errors"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

const (
	syntheticEnrollmentHoldKey    = "synthetic_account_checkpoint_enrollment_hold"
	syntheticEnrollmentHoldDomain = "synthetic-fixture-enrollment-hold"
)

// SyntheticEnrollmentFence is the controller confirmation token for one
// synthetic enrollment. It contains scope identifiers only, never accounts or
// authentication material.
type SyntheticEnrollmentFence struct {
	HoldID       string `json:"holdId"`
	StoreScopeID string `json:"storeScopeId"`
	ProfileID    string `json:"profileId"`
}

type syntheticEnrollmentHoldRecord struct {
	Domain       string `json:"domain"`
	Version      int    `json:"version"`
	HoldID       string `json:"holdId"`
	StoreScopeID string `json:"storeScopeId"`
	ProfileID    string `json:"profileId"`
}

// BeginSyntheticAccountEnrollment persists a sole provisional hold. The
// checkpoint guard treats this unknown prefix record as protected metadata,
// which closes capture admission until completion or operator recovery.
func (s *Store) BeginSyntheticAccountEnrollment(ctx context.Context, storeScopeID, profileID string) (SyntheticEnrollmentFence, error) {
	s.setAccountCheckpointPermit(nil)
	if err := checkpointContextError(ctx); err != nil {
		return SyntheticEnrollmentFence{}, err
	}
	if !validSyntheticIdentityValue(storeScopeID) || !validSyntheticIdentityValue(profileID) {
		return SyntheticEnrollmentFence{}, errors.New("synthetic enrollment scope is invalid")
	}
	holdID, err := syntheticHoldID()
	if err != nil {
		return SyntheticEnrollmentFence{}, errors.New("synthetic enrollment hold unavailable")
	}
	fence := SyntheticEnrollmentFence{HoldID: holdID, StoreScopeID: storeScopeID, ProfileID: profileID}
	provisional := syntheticEnrollmentHoldRecord{
		Domain: syntheticEnrollmentHoldDomain, Version: 1, HoldID: holdID,
		StoreScopeID: storeScopeID, ProfileID: profileID,
	}
	raw, err := encodeCheckpoint(provisional)
	if err != nil {
		return SyntheticEnrollmentFence{}, errors.New("synthetic enrollment hold unavailable")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SyntheticEnrollmentFence{}, errors.New("synthetic enrollment hold unavailable")
	}
	defer tx.Rollback()
	values, err := readAccountCheckpointValuesTx(ctx, tx)
	if err != nil || len(values) != 0 {
		return SyntheticEnrollmentFence{}, errors.New("synthetic enrollment is already protected or unavailable")
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO meta(key,value) VALUES(?,?)`, syntheticEnrollmentHoldKey, raw); err != nil {
		return SyntheticEnrollmentFence{}, errors.New("synthetic enrollment hold unavailable")
	}
	if err := checkpointContextError(ctx); err != nil {
		return SyntheticEnrollmentFence{}, err
	}
	if err := tx.Commit(); err != nil {
		return SyntheticEnrollmentFence{}, errors.New("synthetic enrollment hold unavailable")
	}
	return fence, nil
}

// CompleteSyntheticAccountEnrollment atomically replaces the exact provisional
// hold with an expected-only generation-one checkpoint and a disabled policy.
// It never grants a capture permit; a separate synthetic observation is needed.
func (s *Store) CompleteSyntheticAccountEnrollment(ctx context.Context, fence SyntheticEnrollmentFence, expected map[domain.Source]SyntheticAccountIdentity) (SyntheticAccountCheckpoint, error) {
	s.setAccountCheckpointPermit(nil)
	if err := checkpointContextError(ctx); err != nil {
		return SyntheticAccountCheckpoint{}, err
	}
	if !validSyntheticIdentityValue(fence.HoldID) || !validSyntheticIdentityValue(fence.StoreScopeID) || !validSyntheticIdentityValue(fence.ProfileID) {
		return SyntheticAccountCheckpoint{}, errors.New("synthetic enrollment confirmation is invalid")
	}
	expected = copySyntheticAccounts(expected)
	if len(expected) == 0 {
		return SyntheticAccountCheckpoint{}, errors.New("synthetic enrollment identities are invalid")
	}
	for source, identity := range expected {
		if !supportedSyntheticAccountSource(source) || !validSyntheticIdentityValue(identity.AccountID) ||
			identity.StoreScopeID != fence.StoreScopeID || identity.ProfileID != fence.ProfileID {
			return SyntheticAccountCheckpoint{}, errors.New("synthetic enrollment identities do not match the held scope")
		}
	}
	checkpoint := SyntheticAccountCheckpoint{
		Domain: syntheticAccountCheckpointDomain, Version: syntheticAccountCheckpointVersion,
		CheckpointID: domain.NewID("checkpoint"), Generation: 1,
		ExpectedAccounts: expected, AcceptedRevisions: make(map[domain.Source]string),
	}
	records, err := prepareSyntheticCheckpointRecords(checkpoint)
	if err != nil {
		return SyntheticAccountCheckpoint{}, errors.New("synthetic enrollment identities are invalid")
	}
	policyRaw, err := encodeCheckpoint(accountSessionPolicy{
		Domain: "aku.account-session-policy", Version: 1, Provider: "synthetic-fixture",
		CheckpointID: records.checkpoint.CheckpointID, ExpectedSHA256: expectedAccountDigest(records.checkpoint),
		Generation: 1, Enabled: false,
	})
	if err != nil {
		return SyntheticAccountCheckpoint{}, errors.New("synthetic enrollment policy unavailable")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SyntheticAccountCheckpoint{}, errors.New("synthetic enrollment completion failed")
	}
	defer tx.Rollback()
	values, err := readAccountCheckpointValuesTx(ctx, tx)
	if err != nil || len(values) != 1 {
		return SyntheticAccountCheckpoint{}, errors.New("synthetic enrollment hold is stale or unavailable")
	}
	provisionalRaw, exists := values[syntheticEnrollmentHoldKey]
	if !exists {
		return SyntheticAccountCheckpoint{}, errors.New("synthetic enrollment hold is stale or unavailable")
	}
	var provisional syntheticEnrollmentHoldRecord
	if strictCheckpointJSON(provisionalRaw, &provisional) != nil || provisional.Domain != syntheticEnrollmentHoldDomain || provisional.Version != 1 ||
		provisional.HoldID != fence.HoldID || provisional.StoreScopeID != fence.StoreScopeID || provisional.ProfileID != fence.ProfileID {
		return SyntheticAccountCheckpoint{}, errors.New("synthetic enrollment confirmation is stale or mismatched")
	}
	if err := requireSyntheticCaptureWorkSafe(ctx, tx, records.checkpoint.ExpectedAccounts); err != nil {
		return SyntheticAccountCheckpoint{}, err
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM meta WHERE key=? AND value=?`, syntheticEnrollmentHoldKey, provisionalRaw)
	if err != nil {
		return SyntheticAccountCheckpoint{}, errors.New("synthetic enrollment completion failed")
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return SyntheticAccountCheckpoint{}, errors.New("synthetic enrollment hold changed")
	}
	for _, record := range []struct{ key, value string }{
		{syntheticAccountCheckpointKey, records.checkpointRaw},
		{syntheticAccountCheckpointRequiredKey, records.requiredRaw},
		{syntheticAccountCheckpointFenceKey, records.fenceRaw},
		{accountSessionPolicyKey, policyRaw},
	} {
		if _, err := tx.ExecContext(ctx, `INSERT INTO meta(key,value) VALUES(?,?)`, record.key, record.value); err != nil {
			return SyntheticAccountCheckpoint{}, errors.New("synthetic enrollment completion failed")
		}
	}
	if err := checkpointContextError(ctx); err != nil {
		return SyntheticAccountCheckpoint{}, err
	}
	if err := tx.Commit(); err != nil {
		return SyntheticAccountCheckpoint{}, errors.New("synthetic enrollment completion failed")
	}
	return records.checkpoint, nil
}
