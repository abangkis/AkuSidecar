//go:build auth_session_fixture || auth_session_dev

package store

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func syntheticEnrollmentExpected(fence SyntheticEnrollmentFence) map[domain.Source]SyntheticAccountIdentity {
	return map[domain.Source]SyntheticAccountIdentity{
		domain.SourceX: {AccountID: "x-account", StoreScopeID: fence.StoreScopeID, ProfileID: fence.ProfileID},
	}
}

func readSyntheticEnrollmentMetadata(t *testing.T, s *Store) map[string]string {
	t.Helper()
	values, err := readAccountCheckpointValuesTxForTest(t, s)
	if err != nil {
		t.Fatal(err)
	}
	return values
}

func assertSyntheticEnrollmentMetadata(t *testing.T, s *Store, want map[string]string) {
	t.Helper()
	if got := readSyntheticEnrollmentMetadata(t, s); !reflect.DeepEqual(got, want) {
		t.Fatalf("enrollment metadata changed: got=%v want=%v", got, want)
	}
}

func TestSyntheticAccountEnrollmentPreEnrollHoldAndDisabledCompletion(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "enrollment.db")
	settings := domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", true)
	state, err := Open(path, settings)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	insertAccountCheckpointCommand(t, state, "enrollment-session", "x", "enrollment-run", "enrollment-command", "headless", "owner-epoch", 41)
	assertAccountCheckpointQueued(t, state, "enrollment-command")

	missing, generation, enabled, err := state.LoadSyntheticAccountPolicy(ctx)
	if err != nil || generation != 0 || enabled || missing.CheckpointID != "" {
		t.Fatalf("empty policy load=%+v generation=%d enabled=%t err=%v", missing, generation, enabled, err)
	}
	if err := state.ApplySyntheticAccountPolicy(ctx, missing, generation, "migrate"); err == nil {
		t.Fatal("policy migration succeeded before enrollment")
	}
	preEnroll := SyntheticEnrollmentFence{HoldID: "unissued-hold", StoreScopeID: "store-scope", ProfileID: "headless-profile"}
	if _, err := state.CompleteSyntheticAccountEnrollment(ctx, preEnroll, syntheticEnrollmentExpected(preEnroll)); err == nil {
		t.Fatal("enrollment completed before a durable hold was issued")
	}
	if got := readSyntheticEnrollmentMetadata(t, state); len(got) != 0 {
		t.Fatalf("pre-enrollment attempts wrote metadata: %v", got)
	}

	fence, err := state.BeginSyntheticAccountEnrollment(ctx, "store-scope", "headless-profile")
	if err != nil {
		t.Fatal(err)
	}
	provisional := readSyntheticEnrollmentMetadata(t, state)
	if len(provisional) != 1 || len(provisional[syntheticEnrollmentHoldKey]) == 0 {
		t.Fatalf("begin did not persist one provisional record: %v", provisional)
	}
	if strings.Contains(provisional[syntheticEnrollmentHoldKey], "accountId") || strings.Contains(provisional[syntheticEnrollmentHoldKey], "expectedAccounts") {
		t.Fatal("provisional hold persisted account identities before completion")
	}
	if _, _, _, err := state.LoadSyntheticAccountPolicy(ctx); err == nil {
		t.Fatal("policy load treated a provisional hold as a complete checkpoint")
	}
	if claim, err := state.ClaimCommandForCollector(ctx, "enrollment-run", "must-stay-closed", "headless"); claim != nil {
		t.Fatalf("provisional hold admitted a capture claim: %+v", claim)
	} else if err == nil {
		t.Fatal("unknown provisional metadata did not fail closed")
	}
	assertAccountCheckpointQueued(t, state, "enrollment-command")

	if _, err := state.BeginSyntheticAccountEnrollment(ctx, "store-scope", "headless-profile"); err == nil {
		t.Fatal("second enrollment replaced an existing provisional hold")
	}
	assertSyntheticEnrollmentMetadata(t, state, provisional)

	checkpoint, err := state.CompleteSyntheticAccountEnrollment(ctx, fence, syntheticEnrollmentExpected(fence))
	if err != nil {
		t.Fatal(err)
	}
	if checkpoint.Domain != syntheticAccountCheckpointDomain || checkpoint.Version != syntheticAccountCheckpointVersion ||
		checkpoint.CheckpointID == "" || checkpoint.Generation != 1 || len(checkpoint.ExpectedAccounts) != 1 ||
		checkpoint.AcceptedRevisions == nil || len(checkpoint.AcceptedRevisions) != 0 || checkpoint.VerificationTime != "" ||
		checkpoint.OwnerEpoch != "" || checkpoint.HeadlessGeneration != 0 || checkpoint.OwnerPID != 0 {
		t.Fatalf("completion did not return an expected-only generation-one checkpoint: %+v", checkpoint)
	}
	completed := readSyntheticEnrollmentMetadata(t, state)
	if len(completed) != 4 {
		t.Fatalf("completion did not atomically install checkpoint, required, fence, and policy: %v", completed)
	}
	if _, exists := completed[syntheticEnrollmentHoldKey]; exists {
		t.Fatal("provisional record survived successful completion")
	}
	var policy accountSessionPolicy
	if err := strictCheckpointJSON(completed[accountSessionPolicyKey], &policy); err != nil || policy.Generation != 1 || policy.Enabled {
		t.Fatalf("initial enrollment policy=%+v err=%v", policy, err)
	}
	loaded, generation, enabled, err := state.LoadSyntheticAccountPolicy(ctx)
	if err != nil || generation != 1 || enabled || !reflect.DeepEqual(loaded, checkpoint) {
		t.Fatalf("completed policy load=%+v generation=%d enabled=%t err=%v", loaded, generation, enabled, err)
	}

	permit := &accountCheckpointPermit{CheckpointID: "read-must-not-clear", Generation: 2, HoldID: "sentinel"}
	state.setAccountCheckpointPermit(permit)
	if _, _, _, err := state.LoadSyntheticAccountPolicy(ctx); err != nil {
		t.Fatal(err)
	}
	if got := state.loadAccountCheckpointPermit(); !reflect.DeepEqual(got, permit) {
		t.Fatalf("read-only policy load changed capture permit: got=%+v want=%+v", got, permit)
	}
	state.setAccountCheckpointPermit(nil)

	if claim, err := state.ClaimCommandForCollector(ctx, "enrollment-run", "same-handle", "headless"); err != nil || claim != nil {
		t.Fatalf("disabled enrollment admitted same-handle claim: claim=%+v err=%v", claim, err)
	}
	assertAccountCheckpointQueued(t, state, "enrollment-command")
	if _, err := state.BeginSyntheticAccountEnrollment(ctx, "store-scope", "headless-profile"); err == nil {
		t.Fatal("completed enrollment was replaced by a new provisional hold")
	}
	assertSyntheticEnrollmentMetadata(t, state, completed)
	newHandle, err := Open(path, settings)
	if err != nil {
		t.Fatal(err)
	}
	if claim, err := newHandle.ClaimCommandForCollector(ctx, "enrollment-run", "new-handle", "headless"); err != nil || claim != nil {
		t.Fatalf("new Store handle admitted disabled enrollment: claim=%+v err=%v", claim, err)
	}
	assertAccountCheckpointQueued(t, newHandle, "enrollment-command")
	if err := newHandle.Close(); err != nil {
		t.Fatal(err)
	}

	if err := state.Close(); err != nil {
		t.Fatal(err)
	}
	cold, err := Open(path, settings)
	if err != nil {
		t.Fatal(err)
	}
	defer cold.Close()
	if claim, err := cold.ClaimCommandForCollector(ctx, "enrollment-run", "cold-open", "headless"); err != nil || claim != nil {
		t.Fatalf("cold Store handle admitted disabled enrollment: claim=%+v err=%v", claim, err)
	}
	assertAccountCheckpointQueued(t, cold, "enrollment-command")
}

func TestSyntheticAccountEnrollmentStaleCancelledAndForeignScopeAreNoOps(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	fence, err := state.BeginSyntheticAccountEnrollment(ctx, "store-scope", "headless-profile")
	if err != nil {
		t.Fatal(err)
	}
	before := readSyntheticEnrollmentMetadata(t, state)
	for _, attempt := range []struct {
		name  string
		fence SyntheticEnrollmentFence
	}{
		{name: "stale hold", fence: SyntheticEnrollmentFence{HoldID: "stale-hold", StoreScopeID: fence.StoreScopeID, ProfileID: fence.ProfileID}},
		{name: "cancelled confirmation", fence: SyntheticEnrollmentFence{StoreScopeID: fence.StoreScopeID, ProfileID: fence.ProfileID}},
		{name: "foreign store scope", fence: SyntheticEnrollmentFence{HoldID: fence.HoldID, StoreScopeID: "foreign-store", ProfileID: fence.ProfileID}},
		{name: "foreign profile", fence: SyntheticEnrollmentFence{HoldID: fence.HoldID, StoreScopeID: fence.StoreScopeID, ProfileID: "foreign-profile"}},
	} {
		t.Run(attempt.name, func(t *testing.T) {
			expected := syntheticEnrollmentExpected(attempt.fence)
			if _, err := state.CompleteSyntheticAccountEnrollment(ctx, attempt.fence, expected); err == nil {
				t.Fatal("stale or foreign confirmation was accepted")
			}
			assertSyntheticEnrollmentMetadata(t, state, before)
		})
	}
	if _, err := state.CompleteSyntheticAccountEnrollment(ctx, fence, syntheticEnrollmentExpected(fence)); err != nil {
		t.Fatalf("failed confirmation changed the valid hold: %v", err)
	}
}

func TestSyntheticAccountEnrollmentCompletionRejectsUnsafeWorkWithoutMutation(t *testing.T) {
	ctx := context.Background()
	for _, item := range []struct {
		name    string
		source  string
		claimed bool
	}{
		{name: "active claimed command", source: "x", claimed: true},
		{name: "unsupported queued source", source: "facebook"},
	} {
		t.Run(item.name, func(t *testing.T) {
			state := openTestStore(t)
			defer state.Close()
			fence, err := state.BeginSyntheticAccountEnrollment(ctx, "store-scope", "headless-profile")
			if err != nil {
				t.Fatal(err)
			}
			insertAccountCheckpointCommand(t, state, "unsafe-session", item.source, "unsafe-run", "unsafe-command", "headless", "owner-epoch", 41)
			if item.claimed {
				if _, err := state.db.Exec(`UPDATE bridge_commands SET status='claimed',claimed_by='active-owner',claimed_at='now' WHERE id='unsafe-command'`); err != nil {
					t.Fatal(err)
				}
			}
			before := readSyntheticEnrollmentMetadata(t, state)
			if _, err := state.CompleteSyntheticAccountEnrollment(ctx, fence, syntheticEnrollmentExpected(fence)); err == nil {
				t.Fatal("completion proceeded with unsafe active work")
			}
			assertSyntheticEnrollmentMetadata(t, state, before)
		})
	}
}

func TestSyntheticAccountEnrollmentCompletionRollsBackProvisionalDeletion(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	fence, err := state.BeginSyntheticAccountEnrollment(ctx, "store-scope", "headless-profile")
	if err != nil {
		t.Fatal(err)
	}
	before := readSyntheticEnrollmentMetadata(t, state)
	if _, err := state.db.Exec(`CREATE TRIGGER fail_enrollment_policy BEFORE INSERT ON meta WHEN NEW.key='synthetic_account_checkpoint_policy' BEGIN SELECT RAISE(ABORT,'synthetic failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := state.CompleteSyntheticAccountEnrollment(ctx, fence, syntheticEnrollmentExpected(fence)); err == nil {
		t.Fatal("triggered failure reported successful completion")
	}
	assertSyntheticEnrollmentMetadata(t, state, before)
	if _, err := state.db.Exec(`DROP TRIGGER fail_enrollment_policy`); err != nil {
		t.Fatal(err)
	}
	if _, err := state.CompleteSyntheticAccountEnrollment(ctx, fence, syntheticEnrollmentExpected(fence)); err != nil {
		t.Fatalf("provisional hold was not retryable after rollback: %v", err)
	}
	var raw string
	if err := state.db.QueryRow(`SELECT value FROM meta WHERE key=?`, accountSessionPolicyKey).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var policy accountSessionPolicy
	if err := json.Unmarshal([]byte(raw), &policy); err != nil || policy.Generation != 1 || policy.Enabled {
		t.Fatalf("rolled-back completion retry policy=%+v err=%v", policy, err)
	}
}
