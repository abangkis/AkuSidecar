//go:build auth_session_fixture || auth_session_dev

package store

import (
	"context"
	"errors"
)

// ApplySyntheticAccountPolicy simulates an explicitly confirmed enrollment
// policy operation. The caller supplies the exact checkpoint and policy revision
// shown for confirmation. It is not a public endpoint or proof of human consent.
// migrate always starts disabled; enable never grants a capture permit.
func (s *Store) ApplySyntheticAccountPolicy(ctx context.Context, expected SyntheticAccountCheckpoint, policyGeneration uint64, action string) error {
	s.setAccountCheckpointPermit(nil)
	if action != "migrate" && action != "enable" && action != "disable" {
		return errors.New("account session policy action invalid")
	}
	if err := checkpointContextError(ctx); err != nil {
		return err
	}
	expected.ExpectedAccounts = copySyntheticAccounts(expected.ExpectedAccounts)
	expected.AcceptedRevisions = copySyntheticRevisions(expected.AcceptedRevisions)
	expectedRaw, err := encodeCheckpoint(expected)
	if err != nil {
		return errors.New("account session confirmation invalid")
	}
	hold, err := syntheticHoldID()
	if err != nil {
		return errors.New("account session fence unavailable")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	cp, cpRaw, _, _, _, fenceRaw, err := loadSyntheticCheckpointTx(ctx, tx)
	if err != nil || cpRaw != expectedRaw {
		return errors.New("account session confirmation stale or mismatched")
	}
	values, err := readAccountCheckpointValuesTx(ctx, tx)
	if err != nil {
		return err
	}
	oldPolicy, err := readAccountSessionPolicy(values, cp)
	if err != nil {
		return err
	}
	if action == "migrate" {
		if oldPolicy != nil || policyGeneration != 0 {
			return errors.New("account session migration already applied")
		}
	} else if oldPolicy == nil || oldPolicy.Generation != policyGeneration || policyGeneration >= accountCheckpointMaxGeneration {
		return errors.New("account session policy confirmation stale")
	}
	if action == "enable" {
		if err := requireSyntheticCaptureWorkSafe(ctx, tx, cp.ExpectedAccounts); err != nil {
			return err
		}
	}
	policy := accountSessionPolicy{Domain: "aku.account-session-policy", Version: 1, Provider: "synthetic-fixture", CheckpointID: cp.CheckpointID,
		ExpectedSHA256: expectedAccountDigest(cp), Generation: policyGeneration + 1, Enabled: action == "enable"}
	policyRaw, err := encodeCheckpoint(policy)
	if err != nil {
		return err
	}
	fence := accountCheckpointFenceRecord{Domain: syntheticAccountCheckpointDomain, Version: syntheticAccountCheckpointVersion,
		CheckpointID: cp.CheckpointID, Generation: cp.Generation, CheckpointSHA256: checkpointSHA256(cpRaw),
		Scope: syntheticCheckpointScope(cp.ExpectedAccounts), HoldID: hold, State: "pending"}
	newFenceRaw, err := encodeCheckpoint(fence)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE meta SET value=? WHERE key=? AND value=?`, newFenceRaw, syntheticAccountCheckpointFenceKey, fenceRaw)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return errors.New("account session fence changed")
	}
	if oldPolicy == nil {
		_, err = tx.ExecContext(ctx, `INSERT INTO meta(key,value) VALUES(?,?)`, accountSessionPolicyKey, policyRaw)
	} else {
		result, err = tx.ExecContext(ctx, `UPDATE meta SET value=? WHERE key=? AND value=?`, policyRaw, accountSessionPolicyKey, values[accountSessionPolicyKey])
		if err == nil {
			if n, _ := result.RowsAffected(); n != 1 {
				return errors.New("account session policy changed")
			}
		}
	}
	if err != nil {
		return err
	}
	if err := checkpointContextError(ctx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	// A policy change invalidates previous receipts; authorization requires a
	// separately issued fresh fence and a fresh verifier observation.
	s.resetAccountCheckpointFenceAttempt(hold)
	return nil
}
