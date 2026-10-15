//go:build auth_session_fixture || auth_session_dev

package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

const (
	accountCheckpointFreshnessWindow = 5 * time.Second
)

func syntheticHoldID() (string, error) {
	var nonce [24]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(nonce[:]), nil
}

func encodeCheckpoint(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func (s *Store) CreateSyntheticAccountCheckpoint(ctx context.Context, checkpoint SyntheticAccountCheckpoint) error {
	_, err := s.createSyntheticAccountCheckpoint(ctx, checkpoint)
	return err
}

type syntheticCheckpointRecords struct {
	checkpoint    SyntheticAccountCheckpoint
	checkpointRaw string
	requiredRaw   string
	fenceRaw      string
	fence         accountCheckpointFenceRecord
}

func prepareSyntheticCheckpointRecords(checkpoint SyntheticAccountCheckpoint) (syntheticCheckpointRecords, error) {
	if checkpoint.Domain != syntheticAccountCheckpointDomain || checkpoint.Version != syntheticAccountCheckpointVersion ||
		checkpoint.Generation != 1 || len(checkpoint.AcceptedRevisions) != 0 || checkpoint.VerificationTime != "" ||
		checkpoint.OwnerEpoch != "" || checkpoint.HeadlessGeneration != 0 || checkpoint.OwnerPID != 0 {
		return syntheticCheckpointRecords{}, errors.New("synthetic checkpoint enrollment requires expected-only fixture data")
	}
	checkpoint.ExpectedAccounts = copySyntheticAccounts(checkpoint.ExpectedAccounts)
	checkpoint.AcceptedRevisions = make(map[domain.Source]string)
	if err := validateSyntheticCheckpoint(checkpoint); err != nil {
		return syntheticCheckpointRecords{}, errors.New("synthetic checkpoint enrollment is invalid")
	}
	holdID, err := syntheticHoldID()
	if err != nil {
		return syntheticCheckpointRecords{}, errors.New("synthetic checkpoint enrollment failed")
	}
	checkpointRaw, err := encodeCheckpoint(checkpoint)
	if err != nil {
		return syntheticCheckpointRecords{}, errors.New("synthetic checkpoint enrollment failed")
	}
	checkpointHash := checkpointSHA256(checkpointRaw)
	required := accountCheckpointRequired{
		Domain: syntheticAccountCheckpointDomain, Version: syntheticAccountCheckpointVersion,
		CheckpointID: checkpoint.CheckpointID, Generation: checkpoint.Generation,
	}
	fence := accountCheckpointFenceRecord{
		Domain: syntheticAccountCheckpointDomain, Version: syntheticAccountCheckpointVersion,
		CheckpointID: checkpoint.CheckpointID, Generation: checkpoint.Generation,
		CheckpointSHA256: checkpointHash, Scope: syntheticCheckpointScope(checkpoint.ExpectedAccounts),
		HoldID: holdID, State: "pending",
	}
	requiredRaw, err := encodeCheckpoint(required)
	if err != nil {
		return syntheticCheckpointRecords{}, errors.New("synthetic checkpoint enrollment failed")
	}
	fenceRaw, err := encodeCheckpoint(fence)
	if err != nil {
		return syntheticCheckpointRecords{}, errors.New("synthetic checkpoint enrollment failed")
	}
	return syntheticCheckpointRecords{
		checkpoint: checkpoint, checkpointRaw: checkpointRaw, requiredRaw: requiredRaw,
		fenceRaw: fenceRaw, fence: fence,
	}, nil
}

func (s *Store) createSyntheticAccountCheckpoint(ctx context.Context, checkpoint SyntheticAccountCheckpoint) (SyntheticCheckpointFence, error) {
	s.setAccountCheckpointPermit(nil)
	if err := checkpointContextError(ctx); err != nil {
		return SyntheticCheckpointFence{}, err
	}
	records, err := prepareSyntheticCheckpointRecords(checkpoint)
	if err != nil {
		return SyntheticCheckpointFence{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SyntheticCheckpointFence{}, errors.New("synthetic checkpoint enrollment failed")
	}
	defer tx.Rollback()
	values, err := readAccountCheckpointValuesTx(ctx, tx)
	if err != nil || len(values) != 0 {
		return SyntheticCheckpointFence{}, errors.New("synthetic checkpoint enrollment is already protected or unavailable")
	}
	if err := requireSyntheticCaptureWorkSafe(ctx, tx, records.checkpoint.ExpectedAccounts); err != nil {
		return SyntheticCheckpointFence{}, err
	}
	for _, record := range []struct{ key, value string }{
		{syntheticAccountCheckpointKey, records.checkpointRaw},
		{syntheticAccountCheckpointRequiredKey, records.requiredRaw},
		{syntheticAccountCheckpointFenceKey, records.fenceRaw},
	} {
		if _, err := tx.ExecContext(ctx, `INSERT INTO meta(key,value) VALUES(?,?)`, record.key, record.value); err != nil {
			return SyntheticCheckpointFence{}, errors.New("synthetic checkpoint enrollment failed")
		}
	}
	if err := checkpointContextError(ctx); err != nil {
		return SyntheticCheckpointFence{}, err
	}
	if err := tx.Commit(); err != nil {
		return SyntheticCheckpointFence{}, errors.New("synthetic checkpoint enrollment failed")
	}
	s.resetAccountCheckpointFenceAttempt(records.fence.HoldID)
	return SyntheticCheckpointFence{
		Domain: syntheticAccountCheckpointDomain, Version: syntheticAccountCheckpointVersion,
		CheckpointID: records.checkpoint.CheckpointID, Generation: records.checkpoint.Generation, HoldID: records.fence.HoldID,
		CheckpointSHA256: checkpointSHA256(records.checkpointRaw), Scope: copySyntheticScope(records.fence.Scope),
	}, nil
}

// PrepareSyntheticAccountCheckpoint is the only development startup preparation
// path. Fresh enrollment creates the expected-only checkpoint and initial hold in
// one transaction. Reopen requires an existing exact identity map and creates a
// new pending hold in the same transaction that validates it.
func (s *Store) PrepareSyntheticAccountCheckpoint(ctx context.Context, expected map[domain.Source]SyntheticAccountIdentity, freshEnrollment bool) (SyntheticAccountCheckpoint, SyntheticCheckpointFence, error) {
	s.setAccountCheckpointPermit(nil)
	expected = copySyntheticAccounts(expected)
	request := SyntheticAccountCheckpoint{
		Domain: syntheticAccountCheckpointDomain, Version: syntheticAccountCheckpointVersion,
		CheckpointID: domain.NewID("checkpoint"), Generation: 1,
		ExpectedAccounts: expected, AcceptedRevisions: make(map[domain.Source]string),
	}
	if err := validateSyntheticCheckpoint(request); err != nil {
		return SyntheticAccountCheckpoint{}, SyntheticCheckpointFence{}, errors.New("synthetic checkpoint preparation identities are invalid")
	}
	if freshEnrollment {
		fence, err := s.createSyntheticAccountCheckpoint(ctx, request)
		if err != nil {
			return SyntheticAccountCheckpoint{}, SyntheticCheckpointFence{}, err
		}
		return request, fence, nil
	}
	if err := checkpointContextError(ctx); err != nil {
		return SyntheticAccountCheckpoint{}, SyntheticCheckpointFence{}, err
	}
	holdID, err := syntheticHoldID()
	if err != nil {
		return SyntheticAccountCheckpoint{}, SyntheticCheckpointFence{}, errors.New("synthetic checkpoint fence failed")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SyntheticAccountCheckpoint{}, SyntheticCheckpointFence{}, errors.New("synthetic checkpoint preparation failed")
	}
	defer tx.Rollback()
	checkpoint, checkpointRaw, required, _, _, fenceRaw, err := loadSyntheticCheckpointTx(ctx, tx)
	if err != nil || !syntheticAccountsEqual(checkpoint.ExpectedAccounts, expected) ||
		required.CheckpointID != checkpoint.CheckpointID || required.Generation != checkpoint.Generation {
		return SyntheticAccountCheckpoint{}, SyntheticCheckpointFence{}, errors.New("synthetic checkpoint identity mismatch or unavailable")
	}
	newFence := accountCheckpointFenceRecord{
		Domain: syntheticAccountCheckpointDomain, Version: syntheticAccountCheckpointVersion,
		CheckpointID: checkpoint.CheckpointID, Generation: checkpoint.Generation,
		CheckpointSHA256: checkpointSHA256(checkpointRaw), Scope: syntheticCheckpointScope(checkpoint.ExpectedAccounts),
		HoldID: holdID, State: "pending",
	}
	newRaw, err := encodeCheckpoint(newFence)
	if err != nil {
		return SyntheticAccountCheckpoint{}, SyntheticCheckpointFence{}, errors.New("synthetic checkpoint preparation failed")
	}
	result, err := tx.ExecContext(ctx, `UPDATE meta SET value=? WHERE key=? AND value=?`, newRaw, syntheticAccountCheckpointFenceKey, fenceRaw)
	if err != nil {
		return SyntheticAccountCheckpoint{}, SyntheticCheckpointFence{}, errors.New("synthetic checkpoint preparation failed")
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		return SyntheticAccountCheckpoint{}, SyntheticCheckpointFence{}, errors.New("synthetic checkpoint preparation lost its compare-and-swap")
	}
	if err := checkpointContextError(ctx); err != nil {
		return SyntheticAccountCheckpoint{}, SyntheticCheckpointFence{}, err
	}
	if err := tx.Commit(); err != nil {
		return SyntheticAccountCheckpoint{}, SyntheticCheckpointFence{}, errors.New("synthetic checkpoint preparation failed")
	}
	s.resetAccountCheckpointFenceAttempt(holdID)
	return checkpoint, SyntheticCheckpointFence{
		Domain: syntheticAccountCheckpointDomain, Version: syntheticAccountCheckpointVersion,
		CheckpointID: checkpoint.CheckpointID, Generation: checkpoint.Generation, HoldID: holdID,
		CheckpointSHA256: checkpointSHA256(checkpointRaw), Scope: copySyntheticScope(newFence.Scope),
	}, nil
}

func syntheticAccountsEqual(left, right map[domain.Source]SyntheticAccountIdentity) bool {
	if left == nil || right == nil || len(left) != len(right) {
		return false
	}
	for source, identity := range left {
		if right[source] != identity {
			return false
		}
	}
	return true
}

func (s *Store) BeginSyntheticCheckpointFence(ctx context.Context, checkpointID string, generation uint64) (SyntheticCheckpointFence, error) {
	// Revoke first, even when the database operation itself fails.
	s.setAccountCheckpointPermit(nil)
	if err := checkpointContextError(ctx); err != nil {
		return SyntheticCheckpointFence{}, err
	}
	if !validSyntheticIdentityValue(checkpointID) || generation == 0 || generation > accountCheckpointMaxGeneration {
		return SyntheticCheckpointFence{}, errors.New("synthetic checkpoint fence request is invalid")
	}
	holdID, err := syntheticHoldID()
	if err != nil {
		return SyntheticCheckpointFence{}, errors.New("synthetic checkpoint fence failed")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SyntheticCheckpointFence{}, errors.New("synthetic checkpoint fence failed")
	}
	defer tx.Rollback()
	checkpoint, checkpointRaw, required, fence, _, fenceRaw, err := loadSyntheticCheckpointTx(ctx, tx)
	if err != nil || checkpoint.CheckpointID != checkpointID || checkpoint.Generation != generation ||
		required.CheckpointID != checkpointID || required.Generation != generation || fence.State == "pending" && fence.HoldID == "" {
		return SyntheticCheckpointFence{}, errors.New("synthetic checkpoint fence is stale or unavailable")
	}
	newFence := accountCheckpointFenceRecord{
		Domain: syntheticAccountCheckpointDomain, Version: syntheticAccountCheckpointVersion,
		CheckpointID: checkpoint.CheckpointID, Generation: checkpoint.Generation,
		CheckpointSHA256: checkpointSHA256(checkpointRaw), Scope: syntheticCheckpointScope(checkpoint.ExpectedAccounts),
		HoldID: holdID, State: "pending",
	}
	newRaw, err := encodeCheckpoint(newFence)
	if err != nil {
		return SyntheticCheckpointFence{}, errors.New("synthetic checkpoint fence failed")
	}
	result, err := tx.ExecContext(ctx, `UPDATE meta SET value=? WHERE key=? AND value=?`, newRaw, syntheticAccountCheckpointFenceKey, fenceRaw)
	if err != nil {
		return SyntheticCheckpointFence{}, errors.New("synthetic checkpoint fence failed")
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		return SyntheticCheckpointFence{}, errors.New("synthetic checkpoint fence is stale")
	}
	if err := checkpointContextError(ctx); err != nil {
		return SyntheticCheckpointFence{}, err
	}
	if err := tx.Commit(); err != nil {
		return SyntheticCheckpointFence{}, errors.New("synthetic checkpoint fence failed")
	}
	s.resetAccountCheckpointFenceAttempt(holdID)
	return SyntheticCheckpointFence{
		Domain: syntheticAccountCheckpointDomain, Version: syntheticAccountCheckpointVersion,
		CheckpointID: checkpoint.CheckpointID, Generation: checkpoint.Generation, HoldID: holdID,
		CheckpointSHA256: checkpointSHA256(checkpointRaw), Scope: copySyntheticScope(newFence.Scope),
	}, nil
}

func (s *Store) AuthorizeSyntheticAccountCheckpoint(ctx context.Context, captured SyntheticCheckpointFence, receipt SyntheticVerifiedReceipt) error {
	// Failed attempts remain latched closed. A new capture requires a new fence.
	s.setAccountCheckpointPermit(nil)
	captured.Scope = copySyntheticScope(captured.Scope)
	receipt.Accounts = copySyntheticStringMap(receipt.Accounts)
	receipt.StoreScopeIDs = copySyntheticStringMap(receipt.StoreScopeIDs)
	receipt.ProfileIDs = copySyntheticStringMap(receipt.ProfileIDs)
	receipt.Revisions = copySyntheticStringMap(receipt.Revisions)
	if !s.consumeAccountCheckpointFenceAttempt(captured.HoldID) {
		return errors.New("synthetic checkpoint fence was already attempted")
	}
	if err := checkpointContextError(ctx); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return errors.New("synthetic checkpoint authorization failed")
	}
	defer tx.Rollback()
	checkpoint, checkpointRaw, required, fence, requiredRaw, fenceRaw, err := loadSyntheticCheckpointTx(ctx, tx)
	if err != nil {
		return errors.New("synthetic checkpoint authorization is unavailable")
	}
	values, err := readAccountCheckpointValuesTx(ctx, tx)
	if err != nil {
		return errors.New("account session policy unavailable")
	}
	policy, err := readAccountSessionPolicy(values, checkpoint)
	if err != nil || policy != nil && !policy.Enabled {
		return errors.New("account session policy is not enabled")
	}
	if err := validateCapturedFence(captured, checkpoint, checkpointRaw, required, fence); err != nil {
		return errors.New("synthetic checkpoint fence is stale")
	}
	if err := validateSyntheticReceipt(ctx, s.Now(), captured, checkpoint, receipt); err != nil {
		return err
	}
	if err := requireSyntheticCaptureWorkSafe(ctx, tx, checkpoint.ExpectedAccounts); err != nil {
		return err
	}
	if len(checkpoint.AcceptedRevisions) != 0 {
		for source, prior := range checkpoint.AcceptedRevisions {
			if receipt.Revisions[source] != prior {
				return errors.New("synthetic checkpoint revision changed")
			}
		}
	}
	if checkpoint.Generation >= accountCheckpointMaxGeneration {
		return errors.New("synthetic checkpoint generation is exhausted")
	}
	checkpoint.Generation++
	checkpoint.AcceptedRevisions = copySyntheticRevisions(receipt.Revisions)
	checkpoint.VerificationTime = receipt.CapturedAt.UTC().Format(time.RFC3339Nano)
	checkpoint.OwnerEpoch = receipt.OwnerEpoch
	checkpoint.HeadlessGeneration = receipt.HeadlessGeneration
	checkpoint.OwnerPID = receipt.OwnerPID
	if err := validateSyntheticCheckpoint(checkpoint); err != nil {
		return errors.New("synthetic checkpoint receipt is invalid")
	}
	newCheckpointRaw, err := encodeCheckpoint(checkpoint)
	if err != nil {
		return errors.New("synthetic checkpoint authorization failed")
	}
	newRequiredRaw, err := encodeCheckpoint(accountCheckpointRequired{
		Domain: syntheticAccountCheckpointDomain, Version: syntheticAccountCheckpointVersion,
		CheckpointID: checkpoint.CheckpointID, Generation: checkpoint.Generation,
	})
	if err != nil {
		return errors.New("synthetic checkpoint authorization failed")
	}
	newFence := accountCheckpointFenceRecord{
		Domain: syntheticAccountCheckpointDomain, Version: syntheticAccountCheckpointVersion,
		CheckpointID: checkpoint.CheckpointID, Generation: checkpoint.Generation,
		CheckpointSHA256: checkpointSHA256(newCheckpointRaw), Scope: syntheticCheckpointScope(checkpoint.ExpectedAccounts),
		HoldID: fence.HoldID, State: "authorized", OwnerEpoch: receipt.OwnerEpoch,
		HeadlessGeneration: receipt.HeadlessGeneration, OwnerPID: receipt.OwnerPID,
	}
	newFenceRaw, err := encodeCheckpoint(newFence)
	if err != nil {
		return errors.New("synthetic checkpoint authorization failed")
	}
	for _, update := range []struct{ key, oldValue, newValue string }{
		{syntheticAccountCheckpointKey, checkpointRaw, newCheckpointRaw},
		{syntheticAccountCheckpointRequiredKey, requiredRaw, newRequiredRaw},
		{syntheticAccountCheckpointFenceKey, fenceRaw, newFenceRaw},
	} {
		result, err := tx.ExecContext(ctx, `UPDATE meta SET value=? WHERE key=? AND value=?`, update.newValue, update.key, update.oldValue)
		if err != nil {
			return errors.New("synthetic checkpoint authorization failed")
		}
		changed, _ := result.RowsAffected()
		if changed != 1 {
			return errors.New("synthetic checkpoint authorization lost its compare-and-swap")
		}
	}
	if err := checkpointContextError(ctx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return errors.New("synthetic checkpoint authorization failed")
	}
	s.setAccountCheckpointPermit(&accountCheckpointPermit{
		CheckpointID: checkpoint.CheckpointID, Generation: checkpoint.Generation,
		SHA256: checkpointSHA256(newCheckpointRaw), ScopeSHA256: scopeSHA256(newFence.Scope), HoldID: newFence.HoldID,
	})
	return nil
}

func readAccountCheckpointValuesTx(ctx context.Context, tx *sql.Tx) (map[string]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT key,value FROM meta WHERE substr(key,1,?)=? ORDER BY key`, len(syntheticAccountCheckpointKeyPrefix), syntheticAccountCheckpointKeyPrefix)
	if err != nil {
		return nil, errors.New("account checkpoint metadata lookup failed")
	}
	defer rows.Close()
	values := make(map[string]string, 4)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, errors.New("account checkpoint metadata lookup failed")
		}
		values[key] = value
	}
	if err := rows.Err(); err != nil {
		return nil, errors.New("account checkpoint metadata lookup failed")
	}
	if err := rows.Close(); err != nil {
		return nil, errors.New("account checkpoint metadata lookup failed")
	}
	return values, nil
}

func loadSyntheticCheckpointTx(ctx context.Context, tx *sql.Tx) (SyntheticAccountCheckpoint, string, accountCheckpointRequired, accountCheckpointFenceRecord, string, string, error) {
	values, err := readAccountCheckpointValuesTx(ctx, tx)
	if err != nil || !accountCheckpointRecordCountValid(values) {
		return SyntheticAccountCheckpoint{}, "", accountCheckpointRequired{}, accountCheckpointFenceRecord{}, "", "", errors.New("account checkpoint metadata is incomplete")
	}
	checkpointRaw, checkpointOK := values[syntheticAccountCheckpointKey]
	requiredRaw, requiredOK := values[syntheticAccountCheckpointRequiredKey]
	fenceRaw, fenceOK := values[syntheticAccountCheckpointFenceKey]
	if !checkpointOK || !requiredOK || !fenceOK {
		return SyntheticAccountCheckpoint{}, "", accountCheckpointRequired{}, accountCheckpointFenceRecord{}, "", "", errors.New("account checkpoint metadata is incomplete")
	}
	var checkpoint SyntheticAccountCheckpoint
	var required accountCheckpointRequired
	var fence accountCheckpointFenceRecord
	if strictCheckpointJSON(checkpointRaw, &checkpoint) != nil || strictCheckpointJSON(requiredRaw, &required) != nil ||
		strictCheckpointJSON(fenceRaw, &fence) != nil || validateSyntheticCheckpoint(checkpoint) != nil ||
		required.Domain != syntheticAccountCheckpointDomain || required.Version != syntheticAccountCheckpointVersion ||
		required.CheckpointID != checkpoint.CheckpointID || required.Generation != checkpoint.Generation || !validFence(fence, checkpoint, checkpointRaw) {
		return SyntheticAccountCheckpoint{}, "", accountCheckpointRequired{}, accountCheckpointFenceRecord{}, "", "", errors.New("account checkpoint metadata is invalid")
	}
	checkpoint.ExpectedAccounts = copySyntheticAccounts(checkpoint.ExpectedAccounts)
	if _, err := readAccountSessionPolicy(values, checkpoint); err != nil {
		return SyntheticAccountCheckpoint{}, "", accountCheckpointRequired{}, accountCheckpointFenceRecord{}, "", "", err
	}
	checkpoint.AcceptedRevisions = copySyntheticRevisions(checkpoint.AcceptedRevisions)
	fence.Scope = copySyntheticScope(fence.Scope)
	return checkpoint, checkpointRaw, required, fence, requiredRaw, fenceRaw, nil
}

func validateCapturedFence(captured SyntheticCheckpointFence, checkpoint SyntheticAccountCheckpoint, raw string, required accountCheckpointRequired, stored accountCheckpointFenceRecord) error {
	if captured.Domain != syntheticAccountCheckpointDomain || captured.Version != syntheticAccountCheckpointVersion ||
		captured.CheckpointID != checkpoint.CheckpointID || captured.Generation != checkpoint.Generation ||
		captured.CheckpointSHA256 != checkpointSHA256(raw) || captured.HoldID == "" ||
		captured.CheckpointSHA256 != stored.CheckpointSHA256 || captured.HoldID != stored.HoldID ||
		stored.State != "pending" || required.CheckpointID != captured.CheckpointID || required.Generation != captured.Generation ||
		!syntheticScopesEqual(captured.Scope, stored.Scope) {
		return errors.New("stale checkpoint fence")
	}
	return nil
}

func validateSyntheticReceipt(ctx context.Context, now time.Time, fence SyntheticCheckpointFence, checkpoint SyntheticAccountCheckpoint, receipt SyntheticVerifiedReceipt) error {
	if err := checkpointContextError(ctx); err != nil {
		return err
	}
	if receipt.Domain != syntheticAccountCheckpointDomain || receipt.CheckpointID != fence.CheckpointID || receipt.Generation != fence.Generation || receipt.HoldID != fence.HoldID ||
		!validSyntheticIdentityValue(receipt.OwnerEpoch) || receipt.HeadlessGeneration == 0 || receipt.HeadlessGeneration > accountCheckpointMaxGeneration ||
		receipt.OwnerPID <= 0 || receipt.OwnerPID > math.MaxInt32 || receipt.CapturedAt.IsZero() {
		return errors.New("synthetic checkpoint receipt is incomplete")
	}
	captured := receipt.CapturedAt.UTC()
	now = now.UTC()
	if captured.After(now) || now.Sub(captured) > accountCheckpointFreshnessWindow {
		return errors.New("synthetic checkpoint receipt is stale")
	}
	if len(receipt.Accounts) != len(checkpoint.ExpectedAccounts) || len(receipt.StoreScopeIDs) != len(checkpoint.ExpectedAccounts) ||
		len(receipt.ProfileIDs) != len(checkpoint.ExpectedAccounts) || len(receipt.Revisions) != len(checkpoint.ExpectedAccounts) {
		return errors.New("synthetic checkpoint receipt identity map is incomplete")
	}
	for source, expected := range checkpoint.ExpectedAccounts {
		if !supportedSyntheticAccountSource(source) || receipt.Accounts[source] != expected.AccountID ||
			receipt.StoreScopeIDs[source] != expected.StoreScopeID || receipt.ProfileIDs[source] != expected.ProfileID ||
			!validSyntheticIdentityValue(receipt.Revisions[source]) {
			return errors.New("synthetic checkpoint receipt identity does not match")
		}
	}
	for source := range receipt.Accounts {
		if _, ok := checkpoint.ExpectedAccounts[source]; !ok {
			return errors.New("synthetic checkpoint receipt has an unsupported source")
		}
	}
	for source := range receipt.StoreScopeIDs {
		if _, ok := checkpoint.ExpectedAccounts[source]; !ok {
			return errors.New("synthetic checkpoint receipt has an unsupported source")
		}
	}
	for source := range receipt.ProfileIDs {
		if _, ok := checkpoint.ExpectedAccounts[source]; !ok {
			return errors.New("synthetic checkpoint receipt has an unsupported source")
		}
	}
	for source := range receipt.Revisions {
		if _, ok := checkpoint.ExpectedAccounts[source]; !ok {
			return errors.New("synthetic checkpoint receipt has an unsupported source")
		}
	}
	return nil
}

func requireSyntheticCaptureWorkSafe(ctx context.Context, tx *sql.Tx, expected map[domain.Source]SyntheticAccountIdentity) error {
	var active int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM bridge_commands WHERE status='claimed'`).Scan(&active); err != nil {
		return errors.New("synthetic checkpoint work-state lookup failed")
	}
	if active != 0 {
		return errors.New("synthetic checkpoint requires capture commands to drain")
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_recaptures WHERE status IN ('queued','claimed')`).Scan(&active); err != nil {
		return errors.New("synthetic checkpoint work-state lookup failed")
	}
	if active != 0 {
		return errors.New("synthetic checkpoint does not support active media recaptures")
	}
	rows, err := tx.QueryContext(ctx, `SELECT r.source,c.payload_json FROM bridge_commands c JOIN runs r ON r.id=c.run_id WHERE c.status='queued'`)
	if err != nil {
		return errors.New("synthetic checkpoint work-state lookup failed")
	}
	defer rows.Close()
	for rows.Next() {
		var rawSource, rawPayload string
		if err := rows.Scan(&rawSource, &rawPayload); err != nil {
			return errors.New("synthetic checkpoint work-state lookup failed")
		}
		source := domain.Source(rawSource)
		if _, ok := expected[source]; !ok {
			return errors.New("synthetic checkpoint does not support active capture work")
		}
		payload, err := decodeCapturePayload(rawPayload)
		if err != nil {
			return errors.New("synthetic checkpoint does not support active capture work")
		}
		collector, err := CaptureCollector(payload)
		stamp, stampOK := payload["captureRuntime"].(map[string]any)
		epoch, epochOK := stamp["epoch"].(string)
		generation, generationOK := stamp["generation"].(float64)
		if err != nil || collector != "headless" || !stampOK || stamp["driver"] != "headless" || !epochOK || !validSyntheticIdentityValue(epoch) ||
			!generationOK || math.IsNaN(generation) || math.IsInf(generation, 0) || generation < 1 || generation > float64(accountCheckpointMaxGeneration) || math.Trunc(generation) != generation {
			return errors.New("synthetic checkpoint does not support active capture work")
		}
	}
	if err := rows.Err(); err != nil {
		return errors.New("synthetic checkpoint work-state lookup failed")
	}
	if err := rows.Close(); err != nil {
		return errors.New("synthetic checkpoint work-state lookup failed")
	}
	return nil
}
