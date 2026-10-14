//go:build auth_session_fixture || auth_session_dev

package store

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

type accountCheckpointTestClock struct{ now time.Time }

func (c *accountCheckpointTestClock) Now() time.Time { return c.now }

func accountCheckpointTestValue(sources ...domain.Source) SyntheticAccountCheckpoint {
	accounts := make(map[domain.Source]SyntheticAccountIdentity, len(sources))
	for _, source := range sources {
		accounts[source] = SyntheticAccountIdentity{
			AccountID: string(source) + "-account", StoreScopeID: "synthetic-store-scope", ProfileID: "synthetic-headless-profile",
		}
	}
	return SyntheticAccountCheckpoint{
		Domain: syntheticAccountCheckpointDomain, Version: syntheticAccountCheckpointVersion,
		CheckpointID: "checkpoint-test", Generation: 1, ExpectedAccounts: accounts,
	}
}

func TestSyntheticAccountCheckpointRequiresOneSharedScope(t *testing.T) {
	checkpoint := accountCheckpointTestValue(domain.SourceX, domain.SourceInstagram)
	checkpoint.ExpectedAccounts[domain.SourceInstagram] = SyntheticAccountIdentity{
		AccountID: "instagram-account", StoreScopeID: "another-store-scope", ProfileID: "another-profile",
	}
	state := openTestStore(t)
	if err := state.CreateSyntheticAccountCheckpoint(context.Background(), checkpoint); err == nil {
		t.Fatal("checkpoint with mixed Store/profile scope was accepted")
	}
	if protected, err := readAccountCheckpointValuesTxForTest(t, state); err != nil || len(protected) != 0 {
		t.Fatalf("invalid scope enrollment left metadata: values=%v err=%v", protected, err)
	}
}

func TestSyntheticAccountCheckpointPrepareIsAtomicAndReopenFenced(t *testing.T) {
	state := openTestStore(t)
	ctx := context.Background()
	identityMap := accountCheckpointTestValue(domain.SourceX, domain.SourceInstagram).ExpectedAccounts
	checkpoint, initialFence, err := state.PrepareSyntheticAccountCheckpoint(ctx, identityMap, true)
	if err != nil {
		t.Fatal(err)
	}
	if checkpoint.Domain != syntheticAccountCheckpointDomain || checkpoint.Version != syntheticAccountCheckpointVersion ||
		checkpoint.Generation != 1 || checkpoint.CheckpointID == "" || len(checkpoint.AcceptedRevisions) != 0 ||
		checkpoint.VerificationTime != "" || checkpoint.OwnerEpoch != "" || checkpoint.HeadlessGeneration != 0 || checkpoint.OwnerPID != 0 ||
		initialFence.HoldID == "" || initialFence.CheckpointID != checkpoint.CheckpointID || initialFence.Generation != 1 {
		t.Fatalf("fresh preparation did not produce expected-only pending state: checkpoint=%+v fence=%+v", checkpoint, initialFence)
	}
	values, err := readAccountCheckpointValuesTxForTest(t, state)
	if err != nil || len(values) != 3 {
		t.Fatalf("fresh preparation did not atomically persist all checkpoint records: keys=%v err=%v", values, err)
	}

	before := cloneAccountCheckpointValues(values)
	wrongIdentity := copySyntheticAccounts(identityMap)
	wrong := wrongIdentity[domain.SourceInstagram]
	wrong.ProfileID = "foreign-profile"
	wrongIdentity[domain.SourceInstagram] = wrong
	if _, _, err = state.PrepareSyntheticAccountCheckpoint(ctx, wrongIdentity, false); err == nil {
		t.Fatal("reopen preparation accepted a different physical profile")
	}
	if after, readErr := readAccountCheckpointValuesTxForTest(t, state); readErr != nil || !reflect.DeepEqual(after, before) {
		t.Fatalf("mismatched reopen changed checkpoint metadata: before=%v after=%v err=%v", before, after, readErr)
	}

	reopened, reopenedFence, err := state.PrepareSyntheticAccountCheckpoint(ctx, identityMap, false)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.CheckpointID != checkpoint.CheckpointID || reopened.Generation != checkpoint.Generation ||
		!reflect.DeepEqual(reopened.ExpectedAccounts, checkpoint.ExpectedAccounts) || reopenedFence.HoldID == initialFence.HoldID ||
		reopenedFence.CheckpointID != initialFence.CheckpointID || reopenedFence.Generation != initialFence.Generation {
		t.Fatalf("reopen did not preserve identity and create a fresh hold: checkpoint=%+v fence=%+v", reopened, reopenedFence)
	}
	if err := state.AuthorizeSyntheticAccountCheckpoint(ctx, initialFence, accountCheckpointReceipt(state, initialFence, checkpoint, "old-owner", 7)); err == nil {
		t.Fatal("superseded preparation hold was accepted")
	}
}

func cloneAccountCheckpointValues(values map[string]string) map[string]string {
	clone := make(map[string]string, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func readAccountCheckpointValuesTxForTest(t *testing.T, s *Store) (map[string]string, error) {
	t.Helper()
	rows, err := s.db.Query(`SELECT key,value FROM meta WHERE substr(key,1,?)=?`, len(syntheticAccountCheckpointKeyPrefix), syntheticAccountCheckpointKeyPrefix)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		values[key] = value
	}
	return values, rows.Err()
}

func accountCheckpointReceipt(s *Store, fence SyntheticCheckpointFence, checkpoint SyntheticAccountCheckpoint, ownerEpoch string, ownerGeneration uint64) SyntheticVerifiedReceipt {
	receipt := SyntheticVerifiedReceipt{
		Domain: syntheticAccountCheckpointDomain, CheckpointID: fence.CheckpointID, Generation: fence.Generation, HoldID: fence.HoldID,
		Accounts:      make(map[domain.Source]string, len(checkpoint.ExpectedAccounts)),
		StoreScopeIDs: make(map[domain.Source]string, len(checkpoint.ExpectedAccounts)),
		ProfileIDs:    make(map[domain.Source]string, len(checkpoint.ExpectedAccounts)),
		Revisions:     make(map[domain.Source]string, len(checkpoint.ExpectedAccounts)),
		OwnerEpoch:    ownerEpoch, HeadlessGeneration: ownerGeneration, OwnerPID: 1248, CapturedAt: s.Now(),
	}
	for source, identity := range checkpoint.ExpectedAccounts {
		receipt.Accounts[source] = identity.AccountID
		receipt.StoreScopeIDs[source] = identity.StoreScopeID
		receipt.ProfileIDs[source] = identity.ProfileID
		receipt.Revisions[source] = string(source) + "-revision-1"
	}
	return receipt
}

func authorizedAccountCheckpointStore(t *testing.T, checkpoint SyntheticAccountCheckpoint, clock Clock) (*Store, SyntheticCheckpointFence, SyntheticVerifiedReceipt) {
	t.Helper()
	settings := domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", true)
	var state *Store
	var err error
	if clock == nil {
		state = openTestStore(t)
	} else {
		state, err = OpenWithClock(filepath.Join(t.TempDir(), "account-checkpoint.db"), settings, clock)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = state.Close() })
	}
	if err := state.CreateSyntheticAccountCheckpoint(context.Background(), checkpoint); err != nil {
		t.Fatal(err)
	}
	fence, err := state.BeginSyntheticCheckpointFence(context.Background(), checkpoint.CheckpointID, checkpoint.Generation)
	if err != nil {
		t.Fatal(err)
	}
	receipt := accountCheckpointReceipt(state, fence, checkpoint, "synthetic-owner-epoch", 41)
	if err := state.AuthorizeSyntheticAccountCheckpoint(context.Background(), fence, receipt); err != nil {
		t.Fatal(err)
	}
	return state, fence, receipt
}

func readAccountCheckpointMetadata(t *testing.T, s *Store) (string, string, string) {
	t.Helper()
	var checkpoint, required, fence string
	for key, target := range map[string]*string{
		syntheticAccountCheckpointKey:         &checkpoint,
		syntheticAccountCheckpointRequiredKey: &required,
		syntheticAccountCheckpointFenceKey:    &fence,
	} {
		if err := s.db.QueryRow(`SELECT value FROM meta WHERE key=?`, key).Scan(target); err != nil {
			t.Fatal(err)
		}
	}
	return checkpoint, required, fence
}

func assertAccountCheckpointQueued(t *testing.T, s *Store, ids ...string) {
	t.Helper()
	for _, id := range ids {
		var status string
		var claimedAt, claimedBy *string
		if err := s.db.QueryRow(`SELECT status,claimed_at,claimed_by FROM bridge_commands WHERE id=?`, id).Scan(&status, &claimedAt, &claimedBy); err != nil {
			t.Fatal(err)
		}
		if status != "queued" || claimedAt != nil || claimedBy != nil {
			t.Fatalf("command %s changed: status=%s claimedAt=%v claimedBy=%v", id, status, claimedAt, claimedBy)
		}
	}
}

func TestSyntheticAccountCheckpointEnrollmentIsImmutableAndExpectedOnly(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	checkpoint := accountCheckpointTestValue(domain.SourceX, domain.SourceInstagram)
	if err := state.CreateSyntheticAccountCheckpoint(ctx, checkpoint); err != nil {
		t.Fatal(err)
	}
	checkpoint.ExpectedAccounts[domain.SourceX] = SyntheticAccountIdentity{AccountID: "mutated", StoreScopeID: "mutated", ProfileID: "mutated"}
	storedRaw, requiredRaw, fenceRaw := readAccountCheckpointMetadata(t, state)
	var stored SyntheticAccountCheckpoint
	if err := json.Unmarshal([]byte(storedRaw), &stored); err != nil {
		t.Fatal(err)
	}
	if stored.ExpectedAccounts[domain.SourceX].AccountID != "x-account" || len(stored.AcceptedRevisions) != 0 || stored.VerificationTime != "" {
		t.Fatalf("enrollment was not immutable expected-only data: %+v", stored)
	}
	var required accountCheckpointRequired
	var fence accountCheckpointFenceRecord
	if err := json.Unmarshal([]byte(requiredRaw), &required); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(fenceRaw), &fence); err != nil {
		t.Fatal(err)
	}
	if required.CheckpointID != stored.CheckpointID || required.Generation != stored.Generation || fence.State != "pending" || fence.HoldID == "" {
		t.Fatalf("enrollment did not atomically persist requirement and hold: required=%+v fence=%+v", required, fence)
	}
	if err := state.CreateSyntheticAccountCheckpoint(ctx, accountCheckpointTestValue(domain.SourceX)); err == nil {
		t.Fatal("checkpoint replacement or re-enrollment succeeded")
	}
	if after, _, _ := readAccountCheckpointMetadata(t, state); after != storedRaw {
		t.Fatal("re-enrollment changed the original checkpoint")
	}
}

func TestSyntheticAccountCheckpointReceiptValidatesIdentityRevisionAndFreshness(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*SyntheticVerifiedReceipt)
	}{
		{name: "account mismatch", mutate: func(r *SyntheticVerifiedReceipt) { r.Accounts[domain.SourceX] = "other-account" }},
		{name: "scope mismatch", mutate: func(r *SyntheticVerifiedReceipt) { r.StoreScopeIDs[domain.SourceX] = "other-scope" }},
		{name: "profile mismatch", mutate: func(r *SyntheticVerifiedReceipt) { r.ProfileIDs[domain.SourceX] = "other-profile" }},
		{name: "missing revision", mutate: func(r *SyntheticVerifiedReceipt) { delete(r.Revisions, domain.SourceX) }},
		{name: "unsupported revision source", mutate: func(r *SyntheticVerifiedReceipt) { r.Revisions[domain.SourceFacebook] = "unexpected" }},
		{name: "empty owner", mutate: func(r *SyntheticVerifiedReceipt) { r.OwnerEpoch = "" }},
		{name: "empty owner process", mutate: func(r *SyntheticVerifiedReceipt) { r.OwnerPID = 0 }},
		{name: "missing headless generation", mutate: func(r *SyntheticVerifiedReceipt) { r.HeadlessGeneration = 0 }},
		{name: "unsafe headless generation", mutate: func(r *SyntheticVerifiedReceipt) { r.HeadlessGeneration = accountCheckpointMaxGeneration + 1 }},
		{name: "future time", mutate: func(r *SyntheticVerifiedReceipt) { r.CapturedAt = r.CapturedAt.Add(time.Second) }},
		{name: "stale time", mutate: func(r *SyntheticVerifiedReceipt) {
			r.CapturedAt = r.CapturedAt.Add(-accountCheckpointFreshnessWindow - time.Second)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			state := openTestStore(t)
			checkpoint := accountCheckpointTestValue(domain.SourceX)
			if err := state.CreateSyntheticAccountCheckpoint(ctx, checkpoint); err != nil {
				t.Fatal(err)
			}
			fence, err := state.BeginSyntheticCheckpointFence(ctx, checkpoint.CheckpointID, checkpoint.Generation)
			if err != nil {
				t.Fatal(err)
			}
			receipt := accountCheckpointReceipt(state, fence, checkpoint, "synthetic-owner-epoch", 41)
			test.mutate(&receipt)
			before := readAccountCheckpointMetadataSnapshot(t, state)
			if err := state.AuthorizeSyntheticAccountCheckpoint(ctx, fence, receipt); err == nil {
				t.Fatal("invalid synthetic receipt was accepted")
			}
			if after := readAccountCheckpointMetadataSnapshot(t, state); !reflect.DeepEqual(after, before) {
				t.Fatalf("failed receipt changed metadata: got=%+v want=%+v", after, before)
			}
		})
	}
	cancelledCtx, cancel := context.WithCancel(context.Background())
	state := openTestStore(t)
	checkpoint := accountCheckpointTestValue(domain.SourceX)
	if err := state.CreateSyntheticAccountCheckpoint(context.Background(), checkpoint); err != nil {
		t.Fatal(err)
	}
	fence, err := state.BeginSyntheticCheckpointFence(context.Background(), checkpoint.CheckpointID, checkpoint.Generation)
	if err != nil {
		t.Fatal(err)
	}
	receipt := accountCheckpointReceipt(state, fence, checkpoint, "synthetic-owner-epoch", 41)
	cancel()
	if err := state.AuthorizeSyntheticAccountCheckpoint(cancelledCtx, fence, receipt); err == nil {
		t.Fatal("cancelled authorization was accepted")
	}
	if err := state.AuthorizeSyntheticAccountCheckpoint(context.Background(), fence, receipt); err == nil {
		t.Fatal("cancelled fence attempt was retried without a fresh capture")
	}
	fence, err = state.BeginSyntheticCheckpointFence(context.Background(), checkpoint.CheckpointID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.AuthorizeSyntheticAccountCheckpoint(context.Background(), fence, accountCheckpointReceipt(state, fence, checkpoint, "synthetic-owner-epoch", 41)); err != nil {
		t.Fatalf("fresh fence after cancelled attempt was not retryable: %v", err)
	}
}

func TestSyntheticAccountCheckpointInvalidHoldCannotResetAttemptLatch(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	checkpoint := accountCheckpointTestValue(domain.SourceX)
	if err := state.CreateSyntheticAccountCheckpoint(ctx, checkpoint); err != nil {
		t.Fatal(err)
	}
	fence, err := state.BeginSyntheticCheckpointFence(ctx, checkpoint.CheckpointID, 1)
	if err != nil {
		t.Fatal(err)
	}
	badReceipt := accountCheckpointReceipt(state, fence, checkpoint, "synthetic-owner-epoch", 41)
	badReceipt.Accounts[domain.SourceX] = "wrong-account"
	if err := state.AuthorizeSyntheticAccountCheckpoint(ctx, fence, badReceipt); err == nil {
		t.Fatal("invalid H1 receipt was accepted")
	}
	forgedFence := fence
	forgedFence.HoldID = "forged-other-hold"
	forgedReceipt := accountCheckpointReceipt(state, forgedFence, checkpoint, "synthetic-owner-epoch", 41)
	if err := state.AuthorizeSyntheticAccountCheckpoint(ctx, forgedFence, forgedReceipt); err == nil {
		t.Fatal("forged hold was accepted")
	}
	if err := state.AuthorizeSyntheticAccountCheckpoint(ctx, fence, accountCheckpointReceipt(state, fence, checkpoint, "synthetic-owner-epoch", 41)); err == nil {
		t.Fatal("corrected H1 bypassed the consumed-attempt latch")
	}
	fence, err = state.BeginSyntheticCheckpointFence(ctx, checkpoint.CheckpointID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.AuthorizeSyntheticAccountCheckpoint(ctx, fence, accountCheckpointReceipt(state, fence, checkpoint, "synthetic-owner-epoch", 41)); err != nil {
		t.Fatalf("new hold did not reset the attempt latch: %v", err)
	}
}

type accountCheckpointMetadataSnapshot struct{ checkpoint, required, fence string }

func readAccountCheckpointMetadataSnapshot(t *testing.T, s *Store) accountCheckpointMetadataSnapshot {
	t.Helper()
	cp, req, fence := readAccountCheckpointMetadata(t, s)
	return accountCheckpointMetadataSnapshot{checkpoint: cp, required: req, fence: fence}
}

func TestSyntheticAccountCheckpointFailedTransactionAndOldFenceStayClosed(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	checkpoint := accountCheckpointTestValue(domain.SourceX)
	insertAccountCheckpointCommand(t, state, "checkpoint-transaction-session", "x", "checkpoint-transaction-run", "checkpoint-transaction-command", "headless", "synthetic-owner-epoch", 41)
	if err := state.CreateSyntheticAccountCheckpoint(ctx, checkpoint); err != nil {
		t.Fatal(err)
	}
	firstFence, err := state.BeginSyntheticCheckpointFence(ctx, checkpoint.CheckpointID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.BeginSyntheticCheckpointFence(ctx, checkpoint.CheckpointID, 99); err == nil {
		t.Fatal("stale generation fence was accepted")
	}
	secondFence, err := state.BeginSyntheticCheckpointFence(ctx, checkpoint.CheckpointID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if firstFence.HoldID == secondFence.HoldID {
		t.Fatal("fresh fence reused its hold nonce")
	}
	wantAfterSecondFence := readAccountCheckpointMetadataSnapshot(t, state)
	wantQueuedAfterSecondFence := loadCapturePauseCommandSnapshot(t, state, "checkpoint-transaction-command")
	staleReceipt := accountCheckpointReceipt(state, firstFence, checkpoint, "synthetic-owner-epoch", 41)
	if err := state.AuthorizeSyntheticAccountCheckpoint(ctx, firstFence, staleReceipt); err == nil {
		t.Fatal("stale HoldID authorization was accepted")
	}
	assertAccountCheckpointQueued(t, state, "checkpoint-transaction-command")
	if got := loadCapturePauseCommandSnapshot(t, state, "checkpoint-transaction-command"); !reflect.DeepEqual(got, wantQueuedAfterSecondFence) {
		t.Fatalf("stale HoldID changed queued command: got=%+v want=%+v", got, wantQueuedAfterSecondFence)
	}
	receipt := accountCheckpointReceipt(state, secondFence, checkpoint, "synthetic-owner-epoch", 41)
	if _, err := state.db.Exec(`CREATE TRIGGER fail_checkpoint_required BEFORE UPDATE ON meta WHEN NEW.key='synthetic_account_checkpoint_required' BEGIN SELECT RAISE(ABORT,'fixture trigger'); END`); err != nil {
		t.Fatal(err)
	}
	if err := state.AuthorizeSyntheticAccountCheckpoint(ctx, secondFence, receipt); err == nil {
		t.Fatal("triggered checkpoint transaction unexpectedly committed")
	}
	if got := readAccountCheckpointMetadataSnapshot(t, state); !reflect.DeepEqual(got, wantAfterSecondFence) {
		// BeginSyntheticCheckpointFence legitimately replaced only the fence;
		// the later authorization failure must preserve that fresh hold and all
		// checkpoint and required-marker bytes.
		t.Fatalf("failed transaction changed protected metadata: got=%+v", got)
	}
	assertAccountCheckpointQueued(t, state, "checkpoint-transaction-command")
	if got := loadCapturePauseCommandSnapshot(t, state, "checkpoint-transaction-command"); !reflect.DeepEqual(got, wantQueuedAfterSecondFence) {
		t.Fatalf("failed transaction changed queued command: got=%+v want=%+v", got, wantQueuedAfterSecondFence)
	}
	if _, err := state.db.Exec(`DROP TRIGGER fail_checkpoint_required`); err != nil {
		t.Fatal(err)
	}
	thirdFence, err := state.BeginSyntheticCheckpointFence(ctx, checkpoint.CheckpointID, 1)
	if err != nil {
		t.Fatal(err)
	}
	receipt = accountCheckpointReceipt(state, thirdFence, checkpoint, "synthetic-owner-epoch", 41)
	if err := state.AuthorizeSyntheticAccountCheckpoint(ctx, thirdFence, receipt); err != nil {
		t.Fatalf("fresh fence after rollback was not retryable: %v", err)
	}
}

func TestSyntheticAccountCheckpointPermitIsSingleHandleAndReauthorizationFenced(t *testing.T) {
	ctx := context.Background()
	clock := &accountCheckpointTestClock{now: time.Date(2026, 10, 14, 3, 0, 0, 0, time.UTC)}
	settings := domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", true)
	path := filepath.Join(t.TempDir(), "account-checkpoint.db")
	first, err := OpenWithClock(path, settings, clock)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := OpenWithClock(path, settings, clock)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	insertAccountCheckpointCommand(t, first, "checkpoint-shared-session", "x", "checkpoint-shared-run", "checkpoint-shared-command", "headless", "same-owner-epoch", 41)
	checkpoint := accountCheckpointTestValue(domain.SourceX)
	if err := first.CreateSyntheticAccountCheckpoint(ctx, checkpoint); err != nil {
		t.Fatal(err)
	}
	fenceA, err := first.BeginSyntheticCheckpointFence(ctx, checkpoint.CheckpointID, 1)
	if err != nil {
		t.Fatal(err)
	}
	receiptA := accountCheckpointReceipt(first, fenceA, checkpoint, "same-owner-epoch", 41)
	if err := first.AuthorizeSyntheticAccountCheckpoint(ctx, fenceA, receiptA); err != nil {
		t.Fatal(err)
	}
	if claim, err := second.ClaimCommandForCollector(ctx, "checkpoint-shared-run", "cold-handle", "headless"); err != nil || claim != nil {
		t.Fatalf("cold Store handle inherited authorization: claim=%+v err=%v", claim, err)
	}
	fenceB, err := second.BeginSyntheticCheckpointFence(ctx, checkpoint.CheckpointID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if fenceA.HoldID == fenceB.HoldID {
		t.Fatal("new authorization cycle reused the old HoldID")
	}
	receiptB := accountCheckpointReceipt(second, fenceB, checkpoint, "same-owner-epoch", 41)
	if err := second.AuthorizeSyntheticAccountCheckpoint(ctx, fenceB, receiptB); err != nil {
		t.Fatal(err)
	}
	if claim, err := first.ClaimCommandForCollector(ctx, "checkpoint-shared-run", "stale-handle", "headless"); err != nil || claim != nil {
		t.Fatalf("old handle permit survived cross-handle fence and reauthorization: claim=%+v err=%v", claim, err)
	}
	claim, err := second.ClaimCommandForCollector(ctx, "checkpoint-shared-run", "current-handle", "headless")
	if err != nil || claim == nil || claim.ID != "checkpoint-shared-command" {
		t.Fatalf("current Store handle did not receive its permit: claim=%+v err=%v", claim, err)
	}
}

func TestSyntheticAccountCheckpointHeadlessOnlyPauseMediaAndLateRoutes(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	mediaTimelineID, _ := insertUnavailableMediaFixture(t, state)
	checkpoint := accountCheckpointTestValue(domain.SourceX)
	insertAccountCheckpointCommand(t, state, "checkpoint-session-x", "x", "checkpoint-run-x", "checkpoint-command-x", "headless", "synthetic-owner-epoch", 41)
	if err := state.CreateSyntheticAccountCheckpoint(ctx, checkpoint); err != nil {
		t.Fatal(err)
	}
	fence, err := state.BeginSyntheticCheckpointFence(ctx, checkpoint.CheckpointID, 1)
	if err != nil {
		t.Fatal(err)
	}
	receipt := accountCheckpointReceipt(state, fence, checkpoint, "synthetic-owner-epoch", 41)
	if err := state.AuthorizeSyntheticAccountCheckpoint(ctx, fence, receipt); err != nil {
		t.Fatal(err)
	}
	if _, err := state.db.Exec(`INSERT INTO meta(key,value) VALUES(?, '1')`, fixtureCapturePausePrefix+"checkpoint-session-x"); err != nil {
		t.Fatal(err)
	}
	if claim, err := state.ClaimCommandForCollector(ctx, "checkpoint-run-x", "paused-owner", "headless"); err != nil || claim != nil {
		t.Fatalf("account permit overrode the session pause: claim=%+v err=%v", claim, err)
	}
	if _, err := state.db.Exec(`DELETE FROM meta WHERE key=?`, fixtureCapturePausePrefix+"checkpoint-session-x"); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		session, source, run, command, collector string
	}{
		{"late-bridge", "x", "late-bridge-run", "late-bridge-command", "bridge"},
		{"late-quiet", "x", "late-quiet-run", "late-quiet-command", "browser_quiet_hidden"},
		{"late-facebook", "facebook", "late-facebook-run", "late-facebook-command", "headless"},
	} {
		insertAccountCheckpointCommand(t, state, item.session, item.source, item.run, item.command, item.collector, "synthetic-owner-epoch", 41)
	}
	media, err := state.CreateOwnedMediaRecapture(ctx, mediaTimelineID, domain.MediaRecaptureBackground, domain.MediaRecaptureMissingMedia, map[string]any{"driver": "headless"}, "headless")
	if err != nil {
		t.Fatal(err)
	}
	mediaBefore, err := state.MediaRecapture(ctx, media.ID)
	if err != nil {
		t.Fatal(err)
	}
	mediaBeforeRow := readAccountCheckpointMediaRow(t, state, media.ID)
	for _, item := range []struct{ run, command, collector string }{
		{"late-bridge-run", "late-bridge-command", "bridge"},
		{"late-quiet-run", "late-quiet-command", "browser_quiet_hidden"},
		{"late-facebook-run", "late-facebook-command", "headless"},
	} {
		claim, err := state.ClaimCommandForCollector(ctx, item.run, "unsupported-consumer", item.collector)
		if err != nil || claim != nil {
			t.Fatalf("late unsupported route/source %s claim=%+v err=%v", item.command, claim, err)
		}
	}
	mediaClaim, err := state.ClaimMediaRecaptureForCollector(ctx, media.ID, "unsupported-media-consumer", "headless")
	if err != nil || mediaClaim.ID != "" {
		t.Fatalf("protected media recapture claim=%+v err=%v", mediaClaim, err)
	}
	mediaAfter, err := state.MediaRecapture(ctx, media.ID)
	if err != nil || mediaAfter.Status != "queued" || !reflect.DeepEqual(mediaAfter.Payload, mediaBefore.Payload) || mediaAfter.ClaimedAt != nil {
		t.Fatalf("media recapture changed after blocked claim: before=%+v after=%+v err=%v", mediaBefore, mediaAfter, err)
	}
	if afterRow := readAccountCheckpointMediaRow(t, state, media.ID); afterRow != mediaBeforeRow {
		t.Fatal("protected media raw row changed")
	}
	claim, err := state.ClaimCommandForCollector(ctx, "checkpoint-run-x", "authorized-headless", "headless")
	if err != nil || claim == nil || claim.ID != "checkpoint-command-x" {
		t.Fatalf("authorized headless route did not claim: claim=%+v err=%v", claim, err)
	}
}

func TestSyntheticAccountCheckpointReceiptRevisionCannotBeRelearned(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	checkpoint := accountCheckpointTestValue(domain.SourceX)
	if err := state.CreateSyntheticAccountCheckpoint(ctx, checkpoint); err != nil {
		t.Fatal(err)
	}
	fence, err := state.BeginSyntheticCheckpointFence(ctx, checkpoint.CheckpointID, 1)
	if err != nil {
		t.Fatal(err)
	}
	receipt := accountCheckpointReceipt(state, fence, checkpoint, "synthetic-owner-epoch", 41)
	if err := state.AuthorizeSyntheticAccountCheckpoint(ctx, fence, receipt); err != nil {
		t.Fatal(err)
	}
	storedRaw, _, _ := readAccountCheckpointMetadata(t, state)
	var stored SyntheticAccountCheckpoint
	if err := json.Unmarshal([]byte(storedRaw), &stored); err != nil {
		t.Fatal(err)
	}
	if stored.Generation != 2 || len(stored.AcceptedRevisions) != 1 || stored.AcceptedRevisions[domain.SourceX] != receipt.Revisions[domain.SourceX] || stored.VerificationTime == "" {
		t.Fatalf("successful synthetic verification was not persisted as an audited checkpoint: %+v", stored)
	}
	fence, err = state.BeginSyntheticCheckpointFence(ctx, checkpoint.CheckpointID, 2)
	if err != nil {
		t.Fatal(err)
	}
	receipt = accountCheckpointReceipt(state, fence, checkpoint, "synthetic-owner-epoch-next", 42)
	receipt.Revisions[domain.SourceX] = "x-revision-changed"
	before := readAccountCheckpointMetadataSnapshot(t, state)
	if err := state.AuthorizeSyntheticAccountCheckpoint(ctx, fence, receipt); err == nil {
		t.Fatal("authorization automatically replaced an accepted revision")
	}
	if after := readAccountCheckpointMetadataSnapshot(t, state); !reflect.DeepEqual(after, before) {
		t.Fatalf("revision mismatch changed checkpoint: got=%+v want=%+v", after, before)
	}
	if err := state.AuthorizeSyntheticAccountCheckpoint(ctx, fence, accountCheckpointReceipt(state, fence, checkpoint, "synthetic-owner-epoch-next", 42)); err == nil {
		t.Fatal("failed authorization was retried without a new fence")
	}
}

func TestSyntheticAccountCheckpointNewOwnerLeavesOldStampedQueueUnchanged(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	insertAccountCheckpointCommand(t, state, "old-owner-session", "x", "old-owner-run", "old-owner-command", "headless", "old-owner-epoch", 11)
	commandBefore := loadCapturePauseCommandSnapshot(t, state, "old-owner-command")
	checkpoint := accountCheckpointTestValue(domain.SourceX)
	if err := state.CreateSyntheticAccountCheckpoint(ctx, checkpoint); err != nil {
		t.Fatal(err)
	}
	fence, err := state.BeginSyntheticCheckpointFence(ctx, checkpoint.CheckpointID, checkpoint.Generation)
	if err != nil {
		t.Fatal(err)
	}
	receipt := accountCheckpointReceipt(state, fence, checkpoint, "new-owner-epoch", 42)
	if err := state.AuthorizeSyntheticAccountCheckpoint(ctx, fence, receipt); err != nil {
		t.Fatalf("fresh receipt with a new owner failed while old queued work remained stamped: %v", err)
	}
	if after := loadCapturePauseCommandSnapshot(t, state, "old-owner-command"); after != commandBefore {
		t.Fatalf("authorization restamped or claimed the old queued command: before=%+v after=%+v", commandBefore, after)
	}
	claim, err := state.ClaimCommandForCollector(ctx, "old-owner-run", "new-owner-handle", "headless")
	if err != nil || claim != nil {
		t.Fatalf("old-stamped command was claimable under the new owner: claim=%+v err=%v", claim, err)
	}
	if after := loadCapturePauseCommandSnapshot(t, state, "old-owner-command"); after != commandBefore {
		t.Fatalf("blocked old-stamped claim changed the queued command: before=%+v after=%+v", commandBefore, after)
	}
}

func TestSyntheticAccountCheckpointAuthorizationRequiresNoActiveClaimsOrMedia(t *testing.T) {
	for _, active := range []string{"claimed command", "queued media"} {
		t.Run(active, func(t *testing.T) {
			ctx := context.Background()
			state := openTestStore(t)
			checkpoint := accountCheckpointTestValue(domain.SourceX)
			if err := state.CreateSyntheticAccountCheckpoint(ctx, checkpoint); err != nil {
				t.Fatal(err)
			}
			fence, err := state.BeginSyntheticCheckpointFence(ctx, checkpoint.CheckpointID, 1)
			if err != nil {
				t.Fatal(err)
			}
			var commandBefore capturePauseCommandSnapshot
			var mediaBefore domain.MediaRecapture
			var mediaBeforeRow capturePauseCommandSnapshot
			if active == "claimed command" {
				insertAccountCheckpointCommand(t, state, "active-claim-session", "x", "active-claim-run", "active-claim-command", "headless", "synthetic-owner-epoch", 41)
				if _, err := state.db.Exec(`UPDATE bridge_commands SET status='claimed',claimed_by='active-owner',claimed_at='now' WHERE id='active-claim-command'`); err != nil {
					t.Fatal(err)
				}
				commandBefore = loadCapturePauseCommandSnapshot(t, state, "active-claim-command")
			} else {
				timelineID, _ := insertUnavailableMediaFixture(t, state)
				createdMedia, createErr := state.CreateOwnedMediaRecapture(ctx, timelineID, domain.MediaRecaptureBackground, domain.MediaRecaptureMissingMedia, map[string]any{"driver": "headless"}, "headless")
				if createErr != nil {
					t.Fatal(createErr)
				}
				mediaBefore, err = state.MediaRecapture(ctx, createdMedia.ID)
				if err != nil {
					t.Fatal(err)
				}
				mediaBeforeRow = readAccountCheckpointMediaRow(t, state, mediaBefore.ID)
			}
			metadataBefore := readAccountCheckpointMetadataSnapshot(t, state)
			receipt := accountCheckpointReceipt(state, fence, checkpoint, "synthetic-owner-epoch", 41)
			if err := state.AuthorizeSyntheticAccountCheckpoint(ctx, fence, receipt); err == nil {
				t.Fatal("authorization proceeded while active capture work remained")
			}
			if got := readAccountCheckpointMetadataSnapshot(t, state); !reflect.DeepEqual(got, metadataBefore) {
				t.Fatalf("blocked authorization changed metadata: got=%+v want=%+v", got, metadataBefore)
			}
			if active == "claimed command" {
				if got := loadCapturePauseCommandSnapshot(t, state, "active-claim-command"); !reflect.DeepEqual(got, commandBefore) {
					t.Fatalf("blocked authorization changed claimed command: got=%+v want=%+v", got, commandBefore)
				}
				return
			}
			mediaAfter, err := state.MediaRecapture(ctx, mediaBefore.ID)
			if err != nil || mediaAfter.Status != "queued" || !reflect.DeepEqual(mediaAfter.Payload, mediaBefore.Payload) || mediaAfter.ClaimedAt != nil {
				t.Fatalf("blocked authorization changed queued media: before=%+v after=%+v err=%v", mediaBefore, mediaAfter, err)
			}
			if afterRow := readAccountCheckpointMediaRow(t, state, mediaBefore.ID); afterRow != mediaBeforeRow {
				t.Fatal("blocked authorization changed media raw row")
			}
		})
	}
}

func TestSyntheticAccountCheckpointCancelledQueueIsNotRevived(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	insertAccountCheckpointCommand(t, state, "cancel-checkpoint-session", "x", "cancel-checkpoint-run", "cancel-checkpoint-command", "headless", "synthetic-owner-epoch", 41)
	if err := state.CancelSession(ctx, "cancel-checkpoint-session"); err != nil {
		t.Fatal(err)
	}
	checkpoint := accountCheckpointTestValue(domain.SourceX)
	if err := state.CreateSyntheticAccountCheckpoint(ctx, checkpoint); err != nil {
		t.Fatal(err)
	}
	fence, err := state.BeginSyntheticCheckpointFence(ctx, checkpoint.CheckpointID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.AuthorizeSyntheticAccountCheckpoint(ctx, fence, accountCheckpointReceipt(state, fence, checkpoint, "synthetic-owner-epoch", 41)); err != nil {
		t.Fatal(err)
	}
	var before capturePauseCommandSnapshot
	before = loadCapturePauseCommandSnapshot(t, state, "cancel-checkpoint-command")
	if before.status != "cancelled" {
		t.Fatalf("cancelled command status=%s", before.status)
	}
	claim, err := state.ClaimCommandForCollector(ctx, "cancel-checkpoint-run", "must-not-revive", "headless")
	if err != nil || claim != nil {
		t.Fatalf("cancelled command claim=%+v err=%v", claim, err)
	}
	if after := loadCapturePauseCommandSnapshot(t, state, "cancel-checkpoint-command"); !reflect.DeepEqual(after, before) {
		t.Fatalf("cancelled command was revived: got=%+v want=%+v", after, before)
	}
}

func TestSyntheticAccountCheckpointFailedOpenDoesNotClearUnknownMetadata(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	if _, err := state.db.Exec(`INSERT INTO meta(key,value) VALUES('synthetic_account_checkpoint_future','future-v7')`); err != nil {
		t.Fatal(err)
	}
	if err := state.CreateSyntheticAccountCheckpoint(ctx, accountCheckpointTestValue(domain.SourceX)); err == nil {
		t.Fatal("enrollment replaced unknown protected metadata")
	}
	var value string
	if err := state.db.QueryRow(`SELECT value FROM meta WHERE key='synthetic_account_checkpoint_future'`).Scan(&value); err != nil || value != "future-v7" {
		t.Fatalf("unknown protected metadata changed: value=%q err=%v", value, err)
	}
}

func TestSyntheticAccountCheckpointInputAndReceiptMapsDoNotAliasStorage(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	checkpoint := accountCheckpointTestValue(domain.SourceX)
	receiptCheckpoint := accountCheckpointTestValue(domain.SourceX)
	inputAccounts := checkpoint.ExpectedAccounts
	if err := state.CreateSyntheticAccountCheckpoint(ctx, checkpoint); err != nil {
		t.Fatal(err)
	}
	inputAccounts[domain.SourceX] = SyntheticAccountIdentity{AccountID: "changed", StoreScopeID: "changed", ProfileID: "changed"}
	fence, err := state.BeginSyntheticCheckpointFence(ctx, checkpoint.CheckpointID, 1)
	if err != nil {
		t.Fatal(err)
	}
	fenceForReceipt := fence
	fenceForReceipt.Scope = copySyntheticScope(fence.Scope)
	fence.Scope[domain.SourceX] = SyntheticAccountScope{StoreScopeID: "changed", ProfileID: "changed"}
	receipt := accountCheckpointReceipt(state, fenceForReceipt, receiptCheckpoint, "synthetic-owner-epoch", 41)
	revisions := receipt.Revisions
	if err := state.AuthorizeSyntheticAccountCheckpoint(ctx, fenceForReceipt, receipt); err != nil {
		t.Fatal(err)
	}
	revisions[domain.SourceX] = "changed-after-return"
	storedRaw, _, _ := readAccountCheckpointMetadata(t, state)
	var stored SyntheticAccountCheckpoint
	if err := json.Unmarshal([]byte(storedRaw), &stored); err != nil {
		t.Fatal(err)
	}
	if stored.ExpectedAccounts[domain.SourceX].AccountID != "x-account" || stored.AcceptedRevisions[domain.SourceX] != "x-revision-1" {
		t.Fatalf("input or receipt map mutation altered storage: %+v", stored)
	}
}
